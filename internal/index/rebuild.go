package index

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"workbench/internal/model"
	"workbench/internal/store"
)

var ymRe = regexp.MustCompile(`^\d{4}-\d{2}$`)
var yearRe = regexp.MustCompile(`^\d{4}$`)

// projRow 是「磁盘扫出来的一个项目」，Key 为 custom 短码（月度空）。
type projRow struct {
	Type string
	YM   string
	Name string
	Key  string
	Dir  string
}

// fixCustomDir 把目录改名为 <key>_<name> 并回写扫描结果。
func fixCustomDir(pr *projRow, key, name string) error {
	newDir := filepath.Join(filepath.Dir(pr.Dir), store.ProjectDirName(key, name))
	if err := os.Rename(pr.Dir, newDir); err != nil {
		return err
	}
	pr.Key, pr.Name, pr.Dir = key, name, newDir
	return nil
}

// healCustomConflicts 处理合并两个工作目录带来的重复短码/重名：短码随机重生成、
// 项目名加序号，磁盘目录跟着改。自愈失败就丢掉这个项目——
// 撞 UNIQUE 会让整库重建失败，少一个项目只是少一次可见性。
func healCustomConflicts(projs []projRow) []projRow {
	seenKey := map[string]bool{}
	seenName := map[string]bool{}
	keep := projs[:0]
	for i := range projs {
		pr := projs[i]
		if pr.Type != model.TypeCustom {
			keep = append(keep, pr)
			continue
		}
		for attempt := 0; seenKey[pr.Key] && attempt < 50; attempt++ {
			key, err := store.NewProjectKey()
			if err != nil || seenKey[key] {
				continue
			}
			if err := fixCustomDir(&pr, key, pr.Name); err != nil {
				slog.Error("重复短码目录改名失败", "dir", pr.Dir, "err", err)
				break
			}
		}
		for seenName[pr.Name] {
			renamed := false
			for n := 2; n < 100; n++ {
				cand := fmt.Sprintf("%s %d", pr.Name, n)
				if seenName[cand] {
					continue
				}
				if err := fixCustomDir(&pr, pr.Key, cand); err != nil {
					slog.Error("重名目录改名失败", "dir", pr.Dir, "err", err)
					break
				}
				renamed = true
				break
			}
			if !renamed {
				break
			}
		}
		if seenKey[pr.Key] || seenName[pr.Name] {
			slog.Error("自定义项目短码或名称无法去重，本次重建忽略", "key", pr.Key, "name", pr.Name, "dir", pr.Dir)
			continue
		}
		seenKey[pr.Key], seenName[pr.Name] = true, true
		keep = append(keep, pr)
	}
	return keep
}

// Rebuild 以文件系统+清单为真相，全量重建 projects/assets 两张表。
// 项目的对外身份是目录短码（custom）或 -YYYYMM（月度），内部 rowid 每次重建都可以变。
// 资产 id 同理：Open 每次启动都删掉 index.db，重建后 id 一律从 1 重新分配，
// 复制出去的 /assets/:id 下载链接会漂——这是「库即缓存」设计的已知代价（见计划书 5.5）。
func (d *DB) Rebuild(root string) error {
	var projs []projRow

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
				projs = append(projs, projRow{
					Type: model.TypeMonthly, YM: m.Name(), Name: m.Name(),
					Dir: filepath.Join(root, "monthly", y.Name(), m.Name()),
				})
			}
		}
	}

	// 2. 自定义项目：目录名 <短码>_<项目名>
	customs, err := os.ReadDir(filepath.Join(root, "custom"))
	if err == nil {
		for _, c := range customs {
			if !c.IsDir() {
				continue
			}
			key, name, ok := store.ParseCustomDir(c.Name())
			if !ok {
				slog.Warn("忽略不合规的 custom 目录", "dir", c.Name())
				continue
			}
			projs = append(projs, projRow{
				Type: model.TypeCustom, Name: name, Key: key,
				Dir: filepath.Join(root, "custom", c.Name()),
			})
		}
	}

	// 3. 合并两份工作台时可能出现重复短码或重复项目名：改名自愈，别把整个重建搞失败
	projs = healCustomConflicts(projs)

	// 4. 稳定顺序（短码/年月），让内部 rowid 可复现
	sort.Slice(projs, func(i, j int) bool {
		if projs[i].Type != projs[j].Type {
			return projs[i].Type > projs[j].Type
		}
		if projs[i].YM != projs[j].YM {
			return projs[i].YM > projs[j].YM
		}
		return projs[i].Key < projs[j].Key
	})

	// 5. 清空重建
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

	insProj := func(pr projRow) (int64, error) {
		now := time.Now().Format(time.RFC3339)
		res, err := tx.Exec(`INSERT INTO projects(key,name,type,year_month,created_at) VALUES(?,?,?,?,?)`,
			pr.Key, pr.Name, pr.Type, pr.YM, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}

	insAsset := func(projectRowID int64, category, original, stored, relPath, ext string, size int64, uploaded string) error {
		_, err := tx.Exec(`INSERT INTO assets(project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at)
			VALUES(?,?,?,?,?,?,?,?)`, projectRowID, category, original, stored, relPath, ext, size, uploaded)
		return err
	}

	changedProjects := 0
	// 6. 逐项目插入并对账清单。rowid 全部由 SQLite 自增分配，绝不写显式 id。
	for _, pr := range projs {
		pid, err := insProj(pr)
		if err != nil {
			return err
		}
		dir := pr.Dir
		changedProjects++

		// 清单不分类别（key 是存储文件名），对账必须整体做一次：
		// 按「文件实际所在的子目录」推断 category。若按类别分轮各自对账同一份清单，
		// 第一轮会把另一类别（files/）的条目误判为「清单有、磁盘没」整批删掉。
		manifest := store.LoadManifest(dir)
		type diskFile struct {
			category string
			entry    os.DirEntry
		}
		onDisk := map[string]diskFile{}
		for _, category := range []string{model.CatRecord, model.CatFile} {
			entries, err := os.ReadDir(store.CategoryDir(dir, category))
			if err != nil {
				continue // 目录不存在 = 没有该类别资产
			}
			for _, e := range entries {
				if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				onDisk[e.Name()] = diskFile{category: category, entry: e}
			}
		}
		// 清单有、磁盘没 → 从清单删除；磁盘有、清单没 → 兜底规则补录
		dirty := false
		for stored := range manifest {
			if _, ok := onDisk[stored]; !ok {
				delete(manifest, stored)
				dirty = true
			}
		}
		for stored, df := range onDisk {
			if _, ok := manifest[stored]; ok {
				continue
			}
			info, err := df.entry.Info()
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
		// 入库以磁盘为准（category 由所在子目录决定），元数据取对账后的清单
		for stored, df := range onDisk {
			entry, ok := manifest[stored]
			if !ok {
				continue
			}
			info, err := df.entry.Info()
			if err != nil {
				continue
			}
			if err := insAsset(pid, df.category, entry.OriginalName, stored,
				store.RelPath(root, dir, df.category, stored),
				strings.ToLower(strings.TrimPrefix(filepath.Ext(stored), ".")), info.Size(), entry.UploadedAt); err != nil {
				return err
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
