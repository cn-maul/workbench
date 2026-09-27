package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"workbench/internal/model"
)

type DB struct{ *sql.DB }

// Open 打开工作目录的 index.db。库是可丢弃缓存，每次开都先删掉重建，
// 于是代码里只需要「当前这一份」表结构，没有 schema 兼容分支。
// 代价：资产 id 每次启动从 1 重新分配，复制出去的 /assets/:id 链接会漂；
// 项目身份是目录短码，不受影响。
func Open(dbPath string) (*DB, error) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		p := dbPath + suffix
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("删除旧索引缓存失败 %s: %w", p, err)
		}
	}
	sqlDB, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1) // 单写者，避免锁竞争
	d := &DB{sqlDB}
	if err := d.migrate(); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			year_month TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			category TEXT NOT NULL,
			original_name TEXT NOT NULL,
			stored_name TEXT NOT NULL,
			stored_path TEXT NOT NULL,
			ext TEXT NOT NULL,
			size INTEGER NOT NULL,
			uploaded_at TEXT NOT NULL
		)`,
		// 部分索引：月度按 year_month 防重复；custom 的 year_month 恒为空串，
		// 若与 type 组成单一无索会在第二个自定义项目上撞 ('custom','')。
		`CREATE UNIQUE INDEX idx_projects_monthly_ym ON projects(year_month) WHERE type='monthly'`,
		`CREATE UNIQUE INDEX idx_projects_custom_name ON projects(name) WHERE type='custom'`,
		// key = custom 目录前缀短码，也是对外项目 id；月度的 key 恒为空串，所以只索 custom。
		`CREATE UNIQUE INDEX idx_projects_custom_key ON projects(key) WHERE type='custom'`,
		`CREATE INDEX idx_assets_project ON assets(project_id, category)`,
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

// --- projects ---

// 月度的对外 id 由 year_month 推导，custom 的直接用目录短码。
func publicProjectKey(key, ptype, ym string) string {
	if ptype == model.TypeMonthly {
		return model.MonthlyKey(ym)
	}
	return key
}

func scanProject(row scanner) (model.Project, error) {
	var p model.Project
	var key string
	err := row.Scan(&p.RowID, &key, &p.Name, &p.Type, &p.YearMonth, &p.CreatedAt)
	p.ID = publicProjectKey(key, p.Type, p.YearMonth)
	return p, err
}

const projectCols = `id,key,name,type,year_month,created_at`

func (d *DB) ListProjects() ([]model.Project, error) {
	rows, err := d.Query(`SELECT ` + projectCols + ` FROM projects ORDER BY type DESC, year_month DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetProjectByRowID(id int64) (*model.Project, error) {
	row := d.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id=?`, id)
	p, err := scanProject(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) FindMonthly(ym string) (*model.Project, error) {
	row := d.QueryRow(`SELECT `+projectCols+` FROM projects WHERE type='monthly' AND year_month=?`, ym)
	p, err := scanProject(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) FindCustom(name string) (*model.Project, error) {
	row := d.QueryRow(`SELECT `+projectCols+` FROM projects WHERE type='custom' AND name=?`, name)
	p, err := scanProject(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByKey 按目录短码取自定义项目（对外 id 就是这个短码）。
func (d *DB) FindByKey(key string) (*model.Project, error) {
	row := d.QueryRow(`SELECT `+projectCols+` FROM projects WHERE type='custom' AND key=?`, key)
	p, err := scanProject(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ProjectKeyExists 短码是否已入库（新建项目时排重用）。
func (d *DB) ProjectKeyExists(key string) (bool, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM projects WHERE key=?`, key).Scan(&n)
	return n > 0, err
}

// InsertProject 写入项目并返回内部 rowid；月度传空 key，自定义传目录短码。
func (d *DB) InsertProject(key, name, ptype, ym string) (int64, error) {
	now := time.Now().Format(time.RFC3339)
	r, err := d.Exec(`INSERT INTO projects(key,name,type,year_month,created_at) VALUES(?,?,?,?,?)`,
		key, name, ptype, ym, now)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (d *DB) RenameProject(id int64, name string) error {
	_, err := d.Exec(`UPDATE projects SET name=? WHERE id=?`, name, id)
	return err
}

// --- assets ---

// 资产一律联项目取出：对外 project_id 是短码/月度键，内部 rowid 只用来做外键。
const assetCols = `a.id,a.project_id,p.key,p.type,p.year_month,a.category,a.original_name,
	a.stored_name,a.stored_path,a.ext,a.size,a.uploaded_at`
const assetFrom = ` FROM assets a JOIN projects p ON p.id = a.project_id`

func scanAsset(row scanner) (model.Asset, error) {
	var a model.Asset
	var key, ptype, ym string
	err := row.Scan(&a.ID, &a.ProjectRowID, &key, &ptype, &ym, &a.Category, &a.OriginalName,
		&a.StoredName, &a.StoredPath, &a.Ext, &a.Size, &a.UploadedAt)
	a.ProjectID = publicProjectKey(key, ptype, ym)
	return a, err
}

func (d *DB) ListAssets(projectRowID int64, category string) ([]model.Asset, error) {
	q := `SELECT ` + assetCols + assetFrom + ` WHERE a.project_id=?`
	args := []any{projectRowID}
	if category != "" {
		q += ` AND a.category=?`
		args = append(args, category)
	}
	q += ` ORDER BY a.uploaded_at DESC, a.id DESC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) GetAsset(id int64) (*model.Asset, error) {
	row := d.QueryRow(`SELECT `+assetCols+assetFrom+` WHERE a.id=?`, id)
	a, err := scanAsset(row)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func (d *DB) GetAssetsByIds(ids []int64) ([]*model.Asset, error) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := d.Query(`SELECT `+assetCols+assetFrom+` WHERE a.id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (d *DB) DeleteAssets(ids []int64) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := d.Exec(`DELETE FROM assets WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	return err
}

// SearchAssets 按文件名模糊搜索；projectRowID>0 时限定项目。ESCAPE 防止 %/_ 被当通配符。
func (d *DB) SearchAssets(q string, projectRowID int64) ([]model.SearchHit, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	base := `SELECT a.id, a.project_id, p.key, p.type, p.year_month, p.name, a.category, a.original_name,
		a.ext, a.size, a.uploaded_at
		FROM assets a JOIN projects p ON p.id = a.project_id
		WHERE a.original_name LIKE ? ESCAPE '\'`
	args := []any{like}
	if projectRowID > 0 {
		base += ` AND a.project_id = ?`
		args = append(args, projectRowID)
	}
	rows, err := d.Query(base+` ORDER BY a.uploaded_at DESC LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SearchHit
	for rows.Next() {
		var h model.SearchHit
		var key, ptype, ym string
		if err := rows.Scan(&h.ID, &h.ProjectRowID, &key, &ptype, &ym, &h.ProjectName, &h.Category,
			&h.OriginalName, &h.Ext, &h.Size, &h.UploadedAt); err != nil {
			return nil, err
		}
		h.ProjectID = publicProjectKey(key, ptype, ym)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (d *DB) InsertAsset(a *model.Asset) (int64, error) {
	r, err := d.Exec(`INSERT INTO assets(project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		a.ProjectRowID, a.Category, a.OriginalName, a.StoredName, a.StoredPath, a.Ext, a.Size, a.UploadedAt)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (d *DB) UpdateAssetSize(id int64, size int64) error {
	_, err := d.Exec(`UPDATE assets SET size=? WHERE id=?`, size, id)
	return err
}

func (d *DB) DeleteProject(id int64) error {
	_, err := d.Exec(`DELETE FROM assets WHERE project_id=?`, id)
	if err != nil {
		return err
	}
	_, err = d.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}
