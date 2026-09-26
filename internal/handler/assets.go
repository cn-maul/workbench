package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"workbench/internal/model"
	"workbench/internal/store"
)

// HandleUpload multipart 字段: category + file。流式落盘 → 清单 → 入库。
func (s *Server) HandleUpload(c *gin.Context) {
	p, pending, err := s.resolveProject(c.Param("id"))
	if err != nil {
		fail(c, 404, "项目不存在")
		return
	}
	p, err = s.ensureProject(p, pending)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	category := c.PostForm("category")
	if category != model.CatRecord && category != model.CatFile {
		fail(c, 400, "category 只允许 record 或 file")
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, "缺少 file 字段: "+err.Error())
		return
	}
	original, err := store.CleanBaseName(fh.Filename)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	if err := store.CheckExt(category, original); err != nil {
		fail(c, 415, err.Error())
		return
	}

	projDir, err := s.projectDir(p)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	catDir := store.CategoryDir(projDir, category)
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		fail(c, 500, err.Error())
		return
	}
	stored, err := store.AllocateStoredName(catDir, original)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}

	src, err := fh.Open()
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	defer src.Close()

	final := filepath.Join(catDir, stored)
	tmpPath, size, err := streamToTemp(catDir, src)
	if err != nil {
		fail(c, 500, "写入失败: "+err.Error())
		return
	}
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		fail(c, 500, err.Error())
		return
	}

	manifest := store.LoadManifest(projDir)
	uploadedAt := time.Now().Format(time.RFC3339)
	manifest[stored] = store.ManifestEntry{OriginalName: original, UploadedAt: uploadedAt}
	if err := store.SaveManifest(projDir, manifest); err != nil {
		os.Remove(final)
		fail(c, 500, err.Error())
		return
	}

	a := &model.Asset{
		ProjectID: p.ID, Category: category, OriginalName: original, StoredName: stored,
		StoredPath: store.RelPath(s.Cfg.Workspace, projDir, category, stored),
		Ext:        strings.ToLower(strings.TrimPrefix(filepath.Ext(stored), ".")),
		Size:       size, UploadedAt: uploadedAt,
	}
	id, err := s.DB.InsertAsset(a)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	a.ID = id
	c.JSON(200, a)
}

// streamToTemp 流式写临时文件 + fsync，返回临时路径与大小（原子重命名交给调用方，避免双临时）。
func streamToTemp(dir string, r io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(dir, ".tmp_*")
	if err != nil {
		return "", 0, err
	}
	size, err := io.Copy(tmp, r)
	if err == nil {
		err = tmp.Sync()
	}
	name := tmp.Name()
	tmp.Close()
	if err != nil {
		os.Remove(name)
		return "", 0, err
	}
	return name, size, nil
}

// HandleBlankRecord 新建空白 md：{ "title": "..." }
func (s *Server) HandleBlankRecord(c *gin.Context) {
	p, pending, err := s.resolveProject(c.Param("id"))
	if err != nil {
		fail(c, 404, "项目不存在")
		return
	}
	p, err = s.ensureProject(p, pending)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		fail(c, 400, "需要 JSON: {\"title\": \"...\"}")
		return
	}
	title := store.SanitizeTitle(req.Title)
	original := title + ".md"

	projDir, err := s.projectDir(p)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	catDir := store.CategoryDir(projDir, model.CatRecord)
	stored, err := store.AllocateStoredName(catDir, original)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	final := filepath.Join(catDir, stored)
	content := fmt.Sprintf("# %s\n\n", title)
	if err := store.AtomicWriteBytes(final, []byte(content)); err != nil {
		fail(c, 500, err.Error())
		return
	}

	manifest := store.LoadManifest(projDir)
	uploadedAt := time.Now().Format(time.RFC3339)
	manifest[stored] = store.ManifestEntry{OriginalName: original, UploadedAt: uploadedAt}
	if err := store.SaveManifest(projDir, manifest); err != nil {
		os.Remove(final)
		fail(c, 500, err.Error())
		return
	}
	a := &model.Asset{
		ProjectID: p.ID, Category: model.CatRecord, OriginalName: original, StoredName: stored,
		StoredPath: store.RelPath(s.Cfg.Workspace, projDir, model.CatRecord, stored),
		Ext:        "md", Size: int64(len(content)), UploadedAt: uploadedAt,
	}
	id, err := s.DB.InsertAsset(a)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	a.ID = id
	c.JSON(200, a)
}

func (s *Server) assetOr404(c *gin.Context) (*model.Asset, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, 400, "非法资产 id")
		return nil, false
	}
	a, err := s.DB.GetAsset(id)
	if err != nil {
		fail(c, 404, "资产不存在")
		return nil, false
	}
	return a, true
}

// absPath 解析资产磁盘路径，并防目录穿越。
func (s *Server) absPath(a *model.Asset) (string, error) {
	abs := filepath.Join(s.Cfg.Workspace, filepath.FromSlash(a.StoredPath))
	cleanRoot := filepath.Clean(s.Cfg.Workspace) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(abs)+string(filepath.Separator), cleanRoot) {
		return "", fmt.Errorf("非法路径")
	}
	return abs, nil
}

func (s *Server) HandleDownload(c *gin.Context) {
	a, ok := s.assetOr404(c)
	if !ok {
		return
	}
	abs, err := s.absPath(a)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if _, err := os.Stat(abs); err != nil {
		fail(c, 404, "文件已不存在")
		return
	}
	extra := `attachment; filename*=UTF-8''` + url.PathEscape(a.OriginalName)
	c.Header("Content-Disposition", extra)
	c.Header("Content-Type", "application/octet-stream")
	c.File(abs)
}

func (s *Server) HandleGetText(c *gin.Context) {
	a, ok := s.assetOr404(c)
	if !ok {
		return
	}
	if a.Category != model.CatRecord {
		fail(c, 400, "只有记录（md）支持文本读取")
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
	if st := statOf(abs); st > 0 {
		_ = s.DB.UpdateAssetSize(a.ID, st)
	}
	c.Header("X-File-Size", strconv.FormatInt(int64(len(b)), 10))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", b)
}

// HandlePutText 保存 md：原子写回；If-Match 防覆盖（客户端回传读到的 X-File-Size）。
func (s *Server) HandlePutText(c *gin.Context) {
	a, ok := s.assetOr404(c)
	if !ok {
		return
	}
	if a.Category != model.CatRecord {
		fail(c, 400, "只有记录（md）支持文本写入")
		return
	}
	abs, err := s.absPath(a)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	st, err := os.Stat(abs)
	if err != nil {
		fail(c, 404, "文件已不存在")
		return
	}
	if m := c.GetHeader("If-Match"); m != "" {
		if m != strconv.FormatInt(st.Size(), 10) {
			fail(c, http.StatusConflict, "文件在外部已变化，请重新加载后再保存")
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 32<<20))
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	if err := store.AtomicWriteBytes(abs, body); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if st := statOf(abs); st > 0 {
		_ = s.DB.UpdateAssetSize(a.ID, st)
	}
	c.JSON(200, gin.H{"ok": true, "size": len(body)})
}

func statOf(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.Size()
	}
	return -1
}
