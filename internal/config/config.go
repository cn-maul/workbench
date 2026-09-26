package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	json "encoding/json/v2"
)

type Config struct {
	Workspace    string // 当前工作目录（绝对路径），空表示未指定
	WorkspaceErr string // 指针存在但目录不可用时记录原因
	Port         int    // 上次运行实例绑定的端口（用于重复启动探测）
	path         string // workbench.json 路径
}

type pointerFile struct {
	Workspace string `json:"workspace"`
	Port      int    `json:"port,omitempty"`
}

func ExeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func Load() (*Config, error) {
	dir, err := ExeDir()
	if err != nil {
		return nil, err
	}
	cfg := &Config{path: filepath.Join(dir, "workbench.json")}
	b, err := os.ReadFile(cfg.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	var p pointerFile
	if err := json.Unmarshal(b, &p); err != nil {
		return cfg, nil // 指针损坏：视为未指定，引导用户重选
	}
	cfg.Workspace = p.Workspace
	cfg.Port = p.Port
	if p.Workspace == "" {
		return cfg, nil
	}
	if st, err := os.Stat(p.Workspace); err != nil || !st.IsDir() {
		cfg.WorkspaceErr = fmt.Sprintf("工作目录不可用: %v", err)
	}
	return cfg, nil
}

// SetWorkspace 校验可写、建骨架目录、原子写指针。
func (c *Config) SetWorkspace(dir string) error {
	if dir == "" {
		return errors.New("工作目录不能为空")
	}
	if !filepath.IsAbs(dir) {
		return errors.New("必须使用绝对路径")
	}
	if err := os.MkdirAll(filepath.Join(dir, "monthly"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "custom"), 0o755); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".tmp_write_test")
	if err := os.WriteFile(probe, []byte("x"), 0o644); err != nil {
		return fmt.Errorf("工作目录不可写: %w", err)
	}
	os.Remove(probe)

	c.Workspace = dir
	c.WorkspaceErr = ""
	return c.persist()
}

// SavePort 记录当前实例端口（ best-effort，供重复启动探测）。
func (c *Config) SavePort(port int) {
	c.Port = port
	_ = c.persist()
}

func (c *Config) persist() error {
	b, _ := json.Marshal(pointerFile{Workspace: c.Workspace, Port: c.Port})
	tmp := c.path + ".tmp_" + fmt.Sprint(os.Getpid())
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (c *Config) Usable() bool { return c.Workspace != "" && c.WorkspaceErr == "" }
