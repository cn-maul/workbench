package handler

import (
	"log/slog"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"workbench/internal/index"
)

func (s *Server) HandleGetSettings(c *gin.Context) {
	c.JSON(200, gin.H{
		"workspace":        s.Cfg.Workspace,
		"workspace_exists": s.Cfg.Usable(),
		"needs_select":     !s.Cfg.Usable(),
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

// HandlePutSettings 指定/更换工作目录：写指针 → 重开库 → 全量重建。
func (s *Server) HandlePutSettings(c *gin.Context) {
	var req struct {
		Workspace string `json:"workspace"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Workspace == "" {
		fail(c, 400, "需要 JSON: {\"workspace\": \"D:\\\\我的工作台\"}")
		return
	}
	old := s.Cfg.Workspace
	if err := s.Cfg.SetWorkspace(req.Workspace); err != nil {
		fail(c, 400, err.Error())
		return
	}
	if err := s.swapDB(); err != nil {
		// 回滚指针
		if old != "" {
			_ = s.Cfg.SetWorkspace(old)
			_ = s.swapDB()
		}
		fail(c, 500, "打开新工作目录失败: "+err.Error())
		return
	}
	c.JSON(200, gin.H{
		"workspace":        s.Cfg.Workspace,
		"workspace_exists": true,
		"needs_select":     false,
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
	if err := db.SetSetting("workspace", s.Cfg.Workspace); err != nil {
		slog.Warn("写 settings.workspace 失败", "err", err)
	}
	if s.DB != nil {
		s.DB.Close()
	}
	s.DB = db
	return nil
}
