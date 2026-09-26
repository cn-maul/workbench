package handler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"workbench/internal/index"
	"workbench/internal/model"
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
			Project: model.Project{ID: virtualID(cur), Name: cur, Type: model.TypeMonthly, YearMonth: cur},
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
	id, err := s.DB.NextCustomID()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	dir := filepath.Join(s.Cfg.Workspace, "custom", fmt.Sprintf("%d_%s", id, name))
	if err := os.MkdirAll(filepath.Join(dir, "records"), 0o755); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if _, err := s.DB.InsertProject(name, model.TypeCustom, "", id); err != nil {
		os.RemoveAll(dir)
		fail(c, 500, err.Error())
		return
	}
	p, _ := s.DB.GetProject(id)
	c.JSON(200, p)
}

func (s *Server) HandleCurrentMonthly(c *gin.Context) {
	ym := index.CurrentMonth()
	if p, err := s.DB.FindMonthly(ym); err == nil {
		c.JSON(200, gin.H{"project": p, "pending": false})
		return
	}
	c.JSON(200, gin.H{
		"project": model.Project{ID: virtualID(ym), Name: ym, Type: model.TypeMonthly, YearMonth: ym},
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
	assets, err := s.DB.ListAssets(p.ID, category)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if assets == nil {
		assets = []model.Asset{}
	}
	c.JSON(200, assets)
}
