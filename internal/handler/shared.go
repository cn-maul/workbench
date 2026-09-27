package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"workbench/internal/config"
	"workbench/internal/index"
	"workbench/internal/model"
	"workbench/internal/store"
)

type Server struct {
	Cfg      *config.Config
	DB       *index.DB
	SessMu   sync.Mutex
	Sessions map[string]time.Time // token → 登录时间，超过 sessionTTL 失效
}

// resolveProject 解析对外项目 id：custom 短码，或月度的 -YYYYMM（可 pending）。
func (s *Server) resolveProject(raw string) (*model.Project, bool, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "-") {
		ym, err := ymFromKey(raw)
		if err != nil {
			return nil, false, err
		}
		if p, err := s.DB.FindMonthly(ym); err == nil {
			return p, false, nil
		}
		return &model.Project{ID: raw, Name: ym, Type: model.TypeMonthly, YearMonth: ym}, true, nil
	}
	if !store.ValidProjectKey(raw) {
		return nil, false, fmt.Errorf("非法项目 id")
	}
	p, err := s.DB.FindByKey(raw)
	return p, false, err
}

// ymFromKey 把 -YYYYMM 还原为 2006-01；月份越界视为非法。
func ymFromKey(key string) (string, error) {
	digits := key[1:]
	if len(digits) != 6 {
		return "", fmt.Errorf("非法项目 id")
	}
	y, err := strconv.Atoi(digits[:4])
	if err != nil {
		return "", fmt.Errorf("非法项目 id")
	}
	m, err := strconv.Atoi(digits[4:])
	if err != nil || m < 1 || m > 12 {
		return "", fmt.Errorf("非法项目 id")
	}
	return fmt.Sprintf("%04d-%02d", y, m), nil
}

// ensureProject 保证月度项目落盘（自定义项目在创建接口里就建好了，不会是 pending）。
func (s *Server) ensureProject(p *model.Project, pending bool) (*model.Project, error) {
	if !pending {
		return p, nil
	}
	if p.Type != model.TypeMonthly {
		return nil, fmt.Errorf("非法项目 id")
	}
	if existing, err := s.DB.FindMonthly(p.YearMonth); err == nil {
		return existing, s.ensureDirs(existing)
	}
	rowID, err := s.DB.InsertProject("", p.Name, p.Type, p.YearMonth)
	if err != nil {
		return nil, err
	}
	proj, err := s.DB.GetProjectByRowID(rowID)
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
