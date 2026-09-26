package startup

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	json "encoding/json/v2"

	"workbench/internal/config"
	"workbench/internal/index"
	"workbench/internal/store"
)

const DefaultPort = 8080
const maxPortTries = 30

// AcquirePort 探测+复用策略：
// 指针文件里记录的端口是自家实例 → ErrAlreadyRunning（调用方开浏览器到该端口后退出）；
// 默认端口被别的程序占 → +1 重试。返回已占坑的 listener（交给 http.Serve，无窗口期）。
func AcquirePort(cfg *config.Config) (net.Listener, int, error) {
	if p := cfg.Port; p > 0 && isOwnInstance(p) {
		return nil, p, ErrAlreadyRunning
	}
	port := DefaultPort
	for i := 0; i < maxPortTries; i, port = i+1, port+1 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			slog.Info("端口就绪", "port", port)
			cfg.SavePort(port)
			return ln, port, nil
		}
		if isOwnInstance(port) {
			return nil, port, ErrAlreadyRunning
		}
		slog.Warn("端口被其他程序占用，尝试下一个", "port", port)
	}
	return nil, 0, errors.New("连续 30 个端口均被占用")
}

var ErrAlreadyRunning = errors.New("already running")

func isOwnInstance(port int) bool {
	client := &http.Client{Timeout: 600 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var h struct {
		Port int `json:"port"`
	}
	if err := json.UnmarshalRead(resp.Body, &h); err != nil {
		return false
	}
	return h.Port == port
}

// OpenBrowser 打开系统默认浏览器。
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("打开浏览器失败", "err", err, "url", url)
	}
}

// InitWorkspace 工作目录可用时：清临时文件、开库、重建索引。返回 DB（未就绪时 nil）。
func InitWorkspace(cfg *config.Config) *index.DB {
	if !cfg.Usable() {
		slog.Info("工作目录未就绪，等待引导", "detail", cfg.WorkspaceErr)
		return nil
	}
	store.CleanTemps(cfg.Workspace)
	db, err := index.Open(filepath.Join(cfg.Workspace, "index.db"))
	if err != nil {
		slog.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}
	if err := db.Rebuild(cfg.Workspace); err != nil {
		slog.Error("索引重建失败", "err", err)
		os.Exit(1)
	}
	_ = db.SetSetting("workspace", cfg.Workspace)
	return db
}
