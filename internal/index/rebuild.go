package index

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"workbench/internal/model"
	"workbench/internal/store"
)

var ymRe = regexp.MustCompile(`^\d{4}-\d{2}$`)
var yearRe = regexp.MustCompile(`^\d{4}$`)
var customDirRe = regexp.MustCompile(`^(\d+)_(.+)$`)

type projKey struct {
	Type string
	YM   string // 月度用；自定义为空
	Name string // 自定义用
}

// Rebuild 以文件系统+清单为真相，全量重建 projects/assets 两张表。
// 旧库可用时按 stored_path / 项目定位键沿用原 id，保证重建后 id 稳定。
func (d *DB) Rebuild(root string) error {
	// 旧 id 快照（库损坏或空表时忽略）
	projIDByKey := map[projKey]int64{}
	assetIDByPath := map[string]int64{}
	if rows, err := d.Query(`SELECT id,name,type,year_month FROM projects`); err == nil {
		for rows.Next() {
			var k projKey
			var id int64
			if rows.Scan(&id, &k.Name, &k.Type, &k.YM) == nil {
				if k.Type == model.TypeMonthly {
					k.Name = "" // 月度的 name 恒等于 year_month，不参与定位键
				}
				projIDByKey[k] = id
			}
		}
		rows.Close()
	}
	if rows, err := d.Query(`SELECT id,stored_path FROM assets`); err == nil {
		for rows.Next() {
			var id int64
			var sp string
			if rows.Scan(&id, &sp) == nil {
				assetIDByPath[sp] = id
			}
		}
		rows.Close()
	}

	newProjects := map[projKey]int64{} // key -> 期望id（自定义带号，月度0=自动）
	projDirs := map[projKey]string{}

	// 1. 月度项目
	years, err := os.ReadDir(filepath.Join(root, "monthly"))
	if err == nil {
		for _, y := range years {
			if !y.IsDir() || !yearRe.MatchString(y.Name()) {
				continue
			}
			months, _ := os.ReadDir(filepath.Join(root, "monthly", y.Name()))
			for _, m := range months {
				if !m.IsDir() || !ymRe.MatchString(m.Name()) {
					continue
				}
				k := projKey{Type: model.TypeMonthly, YM: m.Name()}
				newProjects[k] = 0
				projDirs[k] = filepath.Join(root, "monthly", y.Name(), m.Name())
			}
		}
	}

	// 2. 自定义项目
	customs, err := os.ReadDir(filepath.Join(root, "custom"))
	if err == nil {
		for _, c := range customs {
			if !c.IsDir() {
				continue
			}
			m := customDirRe.FindStringSubmatch(c.Name())
			if m == nil {
				slog.Warn("忽略不合规的 custom 目录", "dir", c.Name())
				continue
			}
			id, _ := strconv.ParseInt(m[1], 10, 64)
			k := projKey{Type: model.TypeCustom, Name: m[2]}
			newProjects[k] = id
			projDirs[k] = filepath.Join(root, "custom", c.Name())
		}
	}

	// 3. 清空重建
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM assets`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM projects`); err != nil {
		return err
	}

	insProj := func(id int64, name, ptype, ym string) (int64, error) {
		now := time.Now().Format(time.RFC3339)
		if id > 0 {
			_, err := tx.Exec(`INSERT INTO projects(id,name,type,year_month,created_at) VALUES(?,?,?,?,?)`, id, name, ptype, ym, now)
			return id, err
		}
		res, err := tx.Exec(`INSERT INTO projects(name,type,year_month,created_at) VALUES(?,?,?,?)`, name, ptype, ym, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}

	insAsset := func(projectID int64, category, original, stored, relPath, ext string, size int64, uploaded string) error {
		id := assetIDByPath[relPath]
		var err error
		if id > 0 {
			_, err = tx.Exec(`INSERT INTO assets(id,project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at)
				VALUES(?,?,?,?,?,?,?,?,?)`, id, projectID, category, original, stored, relPath, ext, size, uploaded)
		} else {
			_, err = tx.Exec(`INSERT INTO assets(project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at)
				VALUES(?,?,?,?,?,?,?,?)`, projectID, category, original, stored, relPath, ext, size, uploaded)
		}
		return err
	}

	changedProjects := 0
	// 4. 逐项目插入并对账清单
	for k, wantID := range newProjects {
		dir := projDirs[k]
		name := k.YM
		if k.Type == model.TypeCustom {
			name = k.Name
		}
		pid, err := insProj(max(wantID, projIDByKey[k]), name, k.Type, k.YM)
		if err != nil {
			return err
		}
		changedProjects++

		for _, category := range []string{model.CatRecord, model.CatFile} {
			catDir := store.CategoryDir(dir, category)
			entries, err := os.ReadDir(catDir)
			if err != nil {
				continue // 目录不存在 = 没有该类别资产
			}
			manifest := store.LoadManifest(dir)
			onDisk := map[string]os.DirEntry{}
			for _, e := range entries {
				if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				onDisk[e.Name()] = e
			}
			// 清单有、磁盘没 → 从清单删除
			dirty := false
			for stored := range manifest {
				if _, ok := onDisk[stored]; !ok {
					delete(manifest, stored)
					dirty = true
				}
			}
			// 磁盘有、清单没 → 兜底规则补录
			for stored, e := range onDisk {
				if _, ok := manifest[stored]; ok {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				orig, uploaded := store.FallbackMeta(stored, info.ModTime())
				manifest[stored] = store.ManifestEntry{OriginalName: orig, UploadedAt: uploaded}
				dirty = true
			}
			if dirty {
				if err := store.SaveManifest(dir, manifest); err != nil {
					slog.Error("写清单失败", "dir", dir, "err", err)
				}
			}
			for stored, entry := range manifest {
				info, err := onDisk[stored].Info()
				if err != nil {
					continue
				}
				if err := insAsset(pid, category, entry.OriginalName, stored,
					store.RelPath(root, dir, category, stored),
					strings.ToLower(strings.TrimPrefix(filepath.Ext(stored), ".")), info.Size(), entry.UploadedAt); err != nil {
					return err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("索引重建完成", "projects", changedProjects, "root", root)
	return nil
}

// CurrentMonth 返回当月字符串。
func CurrentMonth() string { return time.Now().Format("2006-01") }
