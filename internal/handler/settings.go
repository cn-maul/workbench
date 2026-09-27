package handler

import (
	"path/filepath"

	"github.com/gin-gonic/gin"

	"workbench/internal/index"
	"workbench/internal/store"
)

func (s *Server) HandleGetSettings(c *gin.Context) {
	c.JSON(200, gin.H{
		"workspace":        s.Cfg.Workspace,
		"workspace_exists": s.Cfg.Usable(),
		"needs_select":     !s.Cfg.Usable(),
		"has_password":     s.Cfg.PasswordHash != "",
		"lan_enabled":      s.Cfg.Lan,
		"port":             s.Cfg.Port,
	})
}

// HandlePickFolder 弹系统原生文件夹选择框（引导页/设置页用），取消返回空 path。
func (s *Server) HandlePickFolder(c *gin.Context) {
	path, err := pickFolderCmd()
	if err != nil && path == "" {
		fail(c, 500, "打开文件夹选择框失败: "+err.Error())
		return
	}
	c.JSON(200, gin.H{"path": path})
}

// HandlePutSettings 指定/更换工作目录：迁移旧数据 → 写指针 → 重开库 → 全量重建。
func (s *Server) HandlePutSettings(c *gin.Context) {
	var req struct {
		Workspace string `json:"workspace"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Workspace == "" {
		fail(c, 400, "需要 JSON: {\"workspace\": \"D:\\\\我的工作台\"}")
		return
	}
	old := s.Cfg.Workspace
	next := filepath.Clean(req.Workspace)
	if old != "" {
		old = filepath.Clean(old)
		if store.SamePath(next, old) { // 同一个目录（Windows 下含大小写差异）：没什么可迁可重开，直接回当前状态
			s.HandleGetSettings(c)
			return
		}
		if store.Nested(next, old) || store.Nested(old, next) {
			fail(c, 400, "新目录与旧目录嵌套会互相复制，请选一个独立目录")
			return
		}
	}
	if err := s.Cfg.Prepare(next); err != nil {
		fail(c, 400, err.Error())
		return
	}
	var migrated store.MigrateStats
	if old != "" {
		st, err := store.MigrateWorkspace(old, next)
		if err != nil {
			// 复制失败：指针仍在旧目录，两边都完好
			fail(c, 500, err.Error())
			return
		}
		migrated = st
	}
	// 指针只在数据搬完之后才移动：中途断电时旧目录仍是当前目录，
	// 新目录里的半成品会在下一次「切换」靠「同名跳过」补齐。
	if err := s.Cfg.SetWorkspace(next); err != nil {
		fail(c, 400, err.Error())
		return
	}
	if err := s.swapDB(); err != nil {
		// swapDB 失败时 s.DB 仍是旧目录的连接，指针退回去就行，不用重开库。
		if old != "" {
			_ = s.Cfg.SetWorkspace(old)
		}
		fail(c, 500, "打开新工作目录失败: "+err.Error())
		return
	}
	c.JSON(200, gin.H{
		"workspace":        s.Cfg.Workspace,
		"workspace_exists": true,
		"needs_select":     false,
		"has_password":     s.Cfg.PasswordHash != "",
		"lan_enabled":      s.Cfg.Lan,
		"port":             s.Cfg.Port,
		"migrated":         migrated.Copied,
		"skipped":          migrated.Skipped,
	})
}

// swapDB 关闭旧连接，打开新工作目录的 index.db 并重建索引。
func (s *Server) swapDB() error {
	db, err := index.Open(filepath.Join(s.Cfg.Workspace, "index.db"))
	if err != nil {
		return err
	}
	if err := db.Rebuild(s.Cfg.Workspace); err != nil {
		db.Close()
		return err
	}
	// 先换指针再关旧库：在途请求最多命中旧库一次，不会命中已关闭的库。
	old := s.DB
	s.DB = db
	if old != nil {
		old.Close()
	}
	return nil
}
