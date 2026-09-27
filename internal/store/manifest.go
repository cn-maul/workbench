package store

import (
	"os"
	"path/filepath"
	"sync"

	"encoding/json/jsontext"
	json "encoding/json/v2"
)

type ManifestEntry struct {
	OriginalName string `json:"original_name"`
	UploadedAt   string `json:"uploaded_at"`
}

type Manifest map[string]ManifestEntry

// LoadManifest 读项目清单；不存在返回空清单，损坏视为不存在（重建时按兜底规则补录）。
func LoadManifest(projDir string) Manifest {
	m := Manifest{}
	b, err := os.ReadFile(filepath.Join(projDir, "assets.json"))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

// SaveManifest 原子写回清单。
func SaveManifest(projDir string, m Manifest) error {
	b, err := json.Marshal(m, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	return AtomicWriteBytes(filepath.Join(projDir, "assets.json"), b)
}

// manifestMu 串行化清单的读-改-写，避免并发更新互相覆盖。
var manifestMu sync.Mutex

// MutateManifest 在互斥锁内完成 读→fn 修改→写回（仅当 fn 返回 dirty 才写）。
func MutateManifest(projDir string, fn func(m Manifest) bool) error {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	m := LoadManifest(projDir)
	if !fn(m) {
		return nil
	}
	return SaveManifest(projDir, m)
}
