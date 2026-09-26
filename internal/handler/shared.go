package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"workbench/internal/config"
	"workbench/internal/index"
	"workbench/internal/model"
	"workbench/internal/store"
)

type Server struct {
	Cfg *config.Config
	DB  *index.DB
}

func virtualID(ym string) int64 {
	n, _ := strconv.ParseInt(ym[:4]+ym[5:7], 10, 64)
	return -n
}

// resolveProject 支持真实 id 与虚拟月度 id（-YYYYMM）。
func (s *Server) resolveProject(raw string) (*model.Project, bool, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, false, fmt.Errorf("非法项目 id")
	}
	if n < 0 {
		ym := fmt.Sprintf("%04d-%02d", -n/100, -n%100)
		if p, err := s.DB.FindMonthly(ym); err == nil {
			return p, false, nil
		}
		return &model.Project{ID: n, Name: ym, Type: model.TypeMonthly, YearMonth: ym}, true, nil
	}
	p, err := s.DB.GetProject(n)
	return p, false, err
}

// ensureProject 保证项目目录与库记录存在（月度项目由此落盘）。
func (s *Server) ensureProject(p *model.Project, pending bool) (*model.Project, error) {
	if !pending {
		return p, nil
	}
	if p.Type == model.TypeMonthly {
		if existing, err := s.DB.FindMonthly(p.YearMonth); err == nil {
			return existing, s.ensureDirs(existing)
		}
	}
	var id int64
	var err error
	if p.Type == model.TypeCustom {
		id, err = s.DB.NextCustomID()
		if err != nil {
			return nil, err
		}
		id, err = s.DB.InsertProject(p.Name, p.Type, "", id)
	} else {
		id, err = s.DB.InsertProject(p.Name, p.Type, p.YearMonth, 0)
	}
	if err != nil {
		return nil, err
	}
	proj, err := s.DB.GetProject(id)
	if err != nil {
		return nil, err
	}
	return proj, s.ensureDirs(proj)
}

func (s *Server) ensureDirs(p *model.Project) error {
	dir, err := s.projectDir(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "records"), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(dir, "files"), 0o755)
}

func (s *Server) projectDir(p *model.Project) (string, error) {
	return store.ProjectDir(s.Cfg.Workspace, p)
}

func (s *Server) RequireWorkspace() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.Cfg.Usable() {
			c.AbortWithStatusJSON(http.StatusPreconditionRequired, gin.H{"error": "workspace_required"})
			return
		}
		c.Next()
	}
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

func cleanProjectName(name string) string {
	t := strings.TrimSpace(name)
	t = filepath.Base(filepath.ToSlash(t))
	out := make([]rune, 0, len(t))
	for _, r := range t {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			continue
		}
		out = append(out, r)
	}
	res := strings.TrimSpace(string(out))
	if len([]rune(res)) > 50 {
		res = string([]rune(res)[:50])
	}
	if res == "." || res == ".." {
		return ""
	}
	return res
}
