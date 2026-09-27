package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 回归：清单不分类别，对账必须整体做一次。
// 曾有 bug：按 record/file 分轮各自 LoadManifest 对账，第一轮把 files/ 的条目
// 误判为「清单有、磁盘没」删掉并写回，original_name 退化为兜底名。
func TestRebuildPreservesManifestAcrossCategories(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "monthly", "2026", "2026-09")
	recDir, fileDir := filepath.Join(proj, "records"), filepath.Join(proj, "files")
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(recDir, "20260927_笔记.md"), "# 笔记\n")
	write(filepath.Join(fileDir, "20260927_检查表.pdf"), "%PDF-fake")
	write(filepath.Join(fileDir, "20260927_检查表_2.pdf"), "%PDF-fake-2")

	// 清单含全部三类条目（含重名 _2），original_name 均为上传时保存的真名
	manifest := `{
  "20260927_笔记.md": { "original_name": "笔记.md", "uploaded_at": "2026-09-27T10:00:00+08:00" },
  "20260927_检查表.pdf": { "original_name": "检查表.pdf", "uploaded_at": "2026-09-27T10:01:00+08:00" },
  "20260927_检查表_2.pdf": { "original_name": "检查表_原始.pdf", "uploaded_at": "2026-09-27T10:02:00+08:00" }
}`
	write(filepath.Join(proj, "assets.json"), manifest)

	db, err := Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Rebuild(root); err != nil {
		t.Fatal(err)
	}

	// 清单必须原样保留（含 files 条目的 original_name，不得退化）
	b, err := os.ReadFile(filepath.Join(proj, "assets.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := string(b)
	for _, want := range []string{`"笔记.md"`, `"检查表.pdf"`, `"检查表_原始.pdf"`} {
		if !strings.Contains(m, want) {
			t.Fatalf("清单丢失 original_name %s，实际内容：\n%s", want, m)
		}
	}

	// 缓存表：3 条资产，类别正确
	assets, err := db.ListAssets(1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 3 {
		t.Fatalf("期望 3 条资产，实际 %d", len(assets))
	}
	cats := map[string]int{"record": 0, "file": 0}
	for _, a := range assets {
		cats[a.Category]++
		if a.Category == "record" && a.OriginalName != "笔记.md" {
			t.Errorf("record original_name = %q, 期望 笔记.md", a.OriginalName)
		}
		if a.Category == "file" && a.StoredName == "20260927_检查表_2.pdf" && a.OriginalName != "检查表_原始.pdf" {
			t.Errorf("file original_name = %q, 期望 检查表_原始.pdf", a.OriginalName)
		}
	}
	if cats["record"] != 1 || cats["file"] != 2 {
		t.Fatalf("类别分布错误: %v", cats)
	}
}
