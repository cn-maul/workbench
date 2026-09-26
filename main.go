package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"workbench/internal/config"
	"workbench/internal/handler"
	"workbench/internal/router"
	"workbench/internal/startup"
)

// logFile 在 exe 旁打开 workbench.log（windowsgui 构建下 stdout 不可见，靠它排查问题）。
func logFile() (*os.File, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(filepath.Dir(exe), "workbench.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func main() {
	logOut := io.Writer(os.Stdout)
	if f, err := logFile(); err == nil {
		defer f.Close()
		logOut = io.MultiWriter(os.Stdout, f)
	}
	logger := slog.New(slog.NewTextHandler(logOut, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("读取配置失败", "err", err)
		os.Exit(1)
	}

	ln, port, err := startup.AcquirePort(cfg)
	if errors.Is(err, startup.ErrAlreadyRunning) {
		slog.Info("工作台已在运行，打开浏览器", "port", port)
		startup.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d", port))
		return
	}
	if err != nil {
		slog.Error("无法获取端口", "err", err)
		os.Exit(1)
	}

	db := startup.InitWorkspace(cfg)
	srv := &handler.Server{Cfg: cfg, DB: db}
	engine := router.New(srv, port)

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	slog.Info("工作台启动", "url", url, "workspace", cfg.Workspace)
	startup.OpenBrowser(url)

	if err := http.Serve(ln, engine); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("服务异常", "err", err)
		os.Exit(1)
	}
}
