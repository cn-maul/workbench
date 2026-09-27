package handler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"workbench/internal/index"
	"workbench/internal/model"
	"workbench/internal/store"
)

func (s *Server) HandleListProjects(c *gin.Context) {
	projects, err := s.DB.ListProjects()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	type node struct {
		model.Project
		Pending bool `json:"pending"`
	}
	cur := index.CurrentMonth()
	var current *node
	monthly := []node{}
	custom := []node{}
	for _, p := range projects {
		n := node{Project: p}
		switch {
		case p.Type == model.TypeMonthly && p.YearMonth == cur:
			cp := n
			current = &cp
		case p.Type == model.TypeMonthly:
			monthly = append(monthly, n)
		default:
			custom = append(custom, n)
		}
	}
	if current == nil {
		current = &node{
			Project: model.Project{ID: model.MonthlyKey(cur), Name: cur, Type: model.TypeMonthly, YearMonth: cur},
			Pending: true,
		}
	}
	c.JSON(200, gin.H{"current_monthly": current, "history_monthly": monthly, "custom": custom})
}

func (s *Server) HandleCreateProject(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "需要 JSON: {\"name\": \"...\"}")
		return
	}
	name := cleanProjectName(req.Name)
	if name == "" {
		fail(c, 400, "项目名为空或清洗后为空")
		return
	}
	if _, err := s.DB.FindCustom(name); err == nil {
		fail(c, 400, "已存在同名项目")
		return
	}
	key, err := s.newProjectKey(name)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	dir := filepath.Join(s.Cfg.Workspace, "custom", store.ProjectDirName(key, name))
	if err := os.MkdirAll(filepath.Join(dir, "records"), 0o755); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		os.RemoveAll(dir)
		fail(c, 500, err.Error())
		return
	}
	rowID, err := s.DB.InsertProject(key, name, model.TypeCustom, "")
	if err != nil {
		os.RemoveAll(dir)
		fail(c, 500, err.Error())
		return
	}
	p, err := s.DB.GetProjectByRowID(rowID)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	c.JSON(200, p)
}

// newProjectKey 取一个库里和磁盘上都没用过的短码。
func (s *Server) newProjectKey(name string) (string, error) {
	for i := 0; i < 50; i++ {
		k, err := store.NewProjectKey()
		if err != nil {
			return "", err
		}
		taken, err := s.DB.ProjectKeyExists(k)
		if err != nil {
			return "", err
		}
		if taken {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.Cfg.Workspace, "custom", store.ProjectDirName(k, name))); err == nil {
			continue
		}
		return k, nil
	}
	return "", fmt.Errorf("短码生成失败，请重试")
}

// HandleRenameProject 仅支持自定义项目：目录名 <短码>_<name> 与库记录同步改。
func (s *Server) HandleRenameProject(c *gin.Context) {
	p, _, err := s.resolveProject(c.Param("id"))
	if err != nil {
		fail(c, 404, "项目不存在")
		return
	}
	if p.Type != model.TypeCustom {
		fail(c, 400, "只有自定义项目可以改名")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "需要 JSON: {\"name\": \"...\"}")
		return
	}
	name := cleanProjectName(req.Name)
	if name == "" {
		fail(c, 400, "项目名为空或清洗后为空")
		return
	}
	if name == p.Name {
		c.JSON(200, p)
		return
	}
	if other, err := s.DB.FindCustom(name); err == nil && other.ID != p.ID {
		fail(c, 400, "已存在同名项目")
		return
	}
	oldDir, err := s.projectDir(p)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	newDir := filepath.Join(filepath.Dir(oldDir), store.ProjectDirName(p.ID, name))
	if _, err := os.Stat(newDir); err == nil {
		fail(c, 400, "目标目录已存在，请先处理后再改名")
		return
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := s.DB.RenameProject(p.RowID, name); err != nil {
		os.Rename(newDir, oldDir)
		fail(c, 500, err.Error())
		return
	}
	p.Name = name
	c.JSON(200, p)
}

// HandleDeleteProject 仅支持自定义项目：目录连同内容直接删除（工作区数据不做回收站）。
func (s *Server) HandleDeleteProject(c *gin.Context) {
	p, _, err := s.resolveProject(c.Param("id"))
	if err != nil {
		fail(c, 404, "项目不存在")
		return
	}
	if p.Type != model.TypeCustom {
		fail(c, 400, "只有自定义项目可以删除")
		return
	}
	dir, err := s.projectDir(p)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := s.DB.DeleteProject(p.RowID); err != nil {
		fail(c, 500, err.Error())
		return
	}
	c.Status(204)
}

func (s *Server) HandleCurrentMonthly(c *gin.Context) {
	ym := index.CurrentMonth()
	if p, err := s.DB.FindMonthly(ym); err == nil {
		c.JSON(200, gin.H{"project": p, "pending": false})
		return
	}
	c.JSON(200, gin.H{
		"project": model.Project{ID: model.MonthlyKey(ym), Name: ym, Type: model.TypeMonthly, YearMonth: ym},
		"pending": true,
	})
}

func (s *Server) HandleGetProject(c *gin.Context) {
	p, pending, err := s.resolveProject(c.Param("id"))
	if err != nil {
		fail(c, 404, "项目不存在")
		return
	}
	c.JSON(200, gin.H{"project": p, "pending": pending})
}

func (s *Server) HandleListAssets(c *gin.Context) {
	p, pending, err := s.resolveProject(c.Param("id"))
	if err != nil {
		fail(c, 404, "项目不存在")
		return
	}
	if pending {
		c.JSON(200, []model.Asset{})
		return
	}
	category := c.Query("category")
	if category != "" && category != model.CatRecord && category != model.CatFile {
		fail(c, 400, "category 只允许 record 或 file")
		return
	}
	assets, err := s.DB.ListAssets(p.RowID, category)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if assets == nil {
		assets = []model.Asset{}
	}
	c.JSON(200, assets)
}
