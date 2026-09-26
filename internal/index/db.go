package index

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"workbench/internal/model"
)

type DB struct{ *sql.DB }

func Open(dbPath string) (*DB, error) {
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
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			year_month TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_type_ym ON projects(type, year_month)`,
		`CREATE TABLE IF NOT EXISTS assets (
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
		`CREATE INDEX IF NOT EXISTS idx_assets_project ON assets(project_id, category)`,
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// --- projects ---

func (d *DB) ListProjects() ([]model.Project, error) {
	rows, err := d.Query(`SELECT id,name,type,year_month,created_at FROM projects ORDER BY type DESC, year_month DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Project
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.YearMonth, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetProject(id int64) (*model.Project, error) {
	p := &model.Project{}
	err := d.QueryRow(`SELECT id,name,type,year_month,created_at FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.Type, &p.YearMonth, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (d *DB) FindMonthly(ym string) (*model.Project, error) {
	p := &model.Project{}
	err := d.QueryRow(`SELECT id,name,type,year_month,created_at FROM projects WHERE type='monthly' AND year_month=?`, ym).
		Scan(&p.ID, &p.Name, &p.Type, &p.YearMonth, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (d *DB) InsertProject(name, ptype, ym string, id int64) (int64, error) {
	now := time.Now().Format(time.RFC3339)
	if id > 0 {
		_, err := d.Exec(`INSERT INTO projects(id,name,type,year_month,created_at) VALUES(?,?,?,?,?)`,
			id, name, ptype, ym, now)
		return id, err
	}
	r, err := d.Exec(`INSERT INTO projects(name,type,year_month,created_at) VALUES(?,?,?,?)`,
		name, ptype, ym, now)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

// NextCustomID 返回 custom 下不被占用的最小 id（从 1 起）。
func (d *DB) NextCustomID() (int64, error) {
	rows, err := d.Query(`SELECT id FROM projects ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	used := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		used[id] = true
	}
	for id := int64(1); ; id++ {
		if !used[id] {
			return id, nil
		}
	}
}

func (d *DB) MaxProjectID() (int64, error) {
	var id int64
	err := d.QueryRow(`SELECT COALESCE(MAX(id),0) FROM projects`).Scan(&id)
	return id, err
}

// --- assets ---

func (d *DB) ListAssets(projectID int64, category string) ([]model.Asset, error) {
	q := `SELECT id,project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at
		FROM assets WHERE project_id=?`
	args := []any{projectID}
	if category != "" {
		q += ` AND category=?`
		args = append(args, category)
	}
	q += ` ORDER BY uploaded_at DESC, id DESC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Category, &a.OriginalName, &a.StoredName,
			&a.StoredPath, &a.Ext, &a.Size, &a.UploadedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) GetAsset(id int64) (*model.Asset, error) {
	a := &model.Asset{}
	err := d.QueryRow(`SELECT id,project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at
		FROM assets WHERE id=?`, id).
		Scan(&a.ID, &a.ProjectID, &a.Category, &a.OriginalName, &a.StoredName,
			&a.StoredPath, &a.Ext, &a.Size, &a.UploadedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) InsertAsset(a *model.Asset) (int64, error) {
	r, err := d.Exec(`INSERT INTO assets(project_id,category,original_name,stored_name,stored_path,ext,size,uploaded_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		a.ProjectID, a.Category, a.OriginalName, a.StoredName, a.StoredPath, a.Ext, a.Size, a.UploadedAt)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (d *DB) UpdateAssetSize(id int64, size int64) error {
	_, err := d.Exec(`UPDATE assets SET size=? WHERE id=?`, size, id)
	return err
}

func (d *DB) ClearAssets() error {
	_, err := d.Exec(`DELETE FROM assets`)
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
