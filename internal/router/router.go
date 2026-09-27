package router

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"workbench/internal/handler"
	"workbench/internal/web"
)

const HeaderInstance = "X-Workbench"

func New(s *handler.Server, port int) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.Header(HeaderInstance, "1")
		c.JSON(http.StatusOK, gin.H{"port": port})
	})

	api := r.Group("/api/v1")
	api.GET("/auth/status", s.HandleAuthStatus)
	api.POST("/auth/login", s.HandleLogin)
	api.POST("/auth/logout", s.HandleLogout)

	need := api.Group("", s.RequireAuth())
	{
		need.GET("/settings", s.HandleGetSettings)
		need.PUT("/settings", s.HandlePutSettings)
		need.GET("/pick-folder", s.HandlePickFolder)
	}

	auth := api.Group("", s.RequireAuth(), s.RequireWorkspace())
	{
		auth.PUT("/access", s.HandlePutAccess)
		auth.GET("/projects", s.HandleListProjects)
		auth.POST("/projects", s.HandleCreateProject)
		auth.GET("/projects/current-monthly", s.HandleCurrentMonthly)
		auth.GET("/projects/:id", s.HandleGetProject)
		auth.PUT("/projects/:id", s.HandleRenameProject)
		auth.DELETE("/projects/:id", s.HandleDeleteProject)
		auth.GET("/projects/:id/assets", s.HandleListAssets)
		auth.POST("/projects/:id/upload", s.HandleUpload)
		auth.POST("/projects/:id/records/blank", s.HandleBlankRecord)
		auth.GET("/assets/:id/download", s.HandleDownload)
		auth.GET("/batch/download", s.HandleBatchDownload)
		auth.POST("/batch/delete", s.HandleBatchDelete)
		auth.GET("/assets/:id/text", s.HandleGetText)
		auth.PUT("/assets/:id/text", s.HandlePutText)
		auth.GET("/records/:id/references", s.HandleReferences)
		auth.GET("/search", s.HandleSearch)
	}

	Dist := web.Dist()
	r.NoRoute(func(c *gin.Context) {
		p := path.Clean("/" + strings.TrimPrefix(c.Request.URL.Path, "/"))
		if strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		rel := strings.TrimPrefix(p, "/")
		if rel != "" {
			if f, err := Dist.Open(rel); err == nil {
				if st, err := f.Stat(); err == nil && !st.IsDir() {
					serveFS(c, rel, st.Size(), Dist)
					return
				}
			}
		}
		serveFS(c, "index.html", 0, Dist)
	})
	return r
}

var mimeByExt = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".woff2": "font/woff2",
}

func serveFS(c *gin.Context, name string, _ int64, fsys fs.FS) {
	if mt, ok := mimeByExt[path.Ext(name)]; ok {
		c.Header("Content-Type", mt)
	}
	if name == "index.html" {
		c.Header("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		// vite 产物文件名带内容 hash，可永久缓存
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	}
	f, err := fsys.Open(name)
	if err != nil {
		c.String(http.StatusInternalServerError, "static error")
		return
	}
	defer f.Close()
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, path.Base(name), modZero, rs)
		return
	}
	b, err := io.ReadAll(f)
	if err != nil {
		c.String(http.StatusInternalServerError, "static read error")
		return
	}
	c.Data(http.StatusOK, contentTypeOr(c.Writer.Header().Get("Content-Type"), path.Ext(name)), b)
}

func contentTypeOr(hint, ext string) string {
	if hint != "" {
		return hint
	}
	if mimeByExt[ext] != "" {
		return mimeByExt[ext]
	}
	return "application/octet-stream"
}

var modZero = time.Time{}
