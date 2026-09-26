package store

import (
	"os"
	"path/filepath"

	json "encoding/json/v2"
	"encoding/json/jsontext"
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
