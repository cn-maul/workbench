package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"workbench/internal/model"
	"workbench/internal/store"
)

var refRe = regexp.MustCompile(`\[\[(.+?)\]\]`)

type refResult struct {
	Name      string        `json:"name"`
	Exists    bool          `json:"exists"`
	Matched   *model.Asset  `json:"matched,omitempty"`
	Candidates []model.Asset `json:"candidates"`
}

// HandleReferences GET /records/:id/references —— 扫描 md 中 [[文件名]] 并按 2.2 语义匹配。
func (s *Server) HandleReferences(c *gin.Context) {
	a, ok := s.assetOr404(c)
	if !ok {
		return
	}
	if a.Category != model.CatRecord {
		fail(c, 400, "只有记录支持引用扫描")
		return
	}
	abs, err := s.absPath(a)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		fail(c, 404, "文件已不存在")
		return
	}
	assets, err := s.DB.ListAssets(a.ProjectID, model.CatFile)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	byName := map[string][]model.Asset{}
	for _, f := range assets {
		key := strings.ToLower(filepath.Base(f.OriginalName))
		byName[key] = append(byName[key], f)
	}

	seen := map[string]bool{}
	results := []refResult{}
	for _, m := range refRe.FindAllStringSubmatch(string(b), -1) {
		name := strings.TrimSpace(m[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		res := s.matchRef(name, byName)
		results = append(results, res)
	}
	c.JSON(200, results)
}

// matchRef 精确匹配 → 未命中剥 _N 重试；多命中取最新（uploaded_at 降序，ListAssets 已排序）。
func (s *Server) matchRef(name string, byName map[string][]model.Asset) refResult {
	res := refResult{Name: name, Candidates: []model.Asset{}}
	cands, ok := byName[strings.ToLower(name)]
	if !ok {
		if stripped, changed := store.StripTrailDup(name); changed {
			cands, ok = byName[strings.ToLower(stripped)]
		}
	}
	if !ok || len(cands) == 0 {
		return res
	}
	res.Exists = true
	res.Matched = &cands[0]
	res.Candidates = cands
	return res
}
