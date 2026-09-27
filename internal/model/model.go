package model

import "time"

const (
	TypeMonthly = "monthly"
	TypeCustom  = "custom"

	CatRecord = "record"
	CatFile   = "file"
)

type Project struct {
	RowID     int64  `json:"-"`  // projects.id 内部主键，只给 assets.project_id 用
	ID        string `json:"id"` // 对外标识：custom=目录短码，monthly="-YYYYMM"
	Name      string `json:"name"`
	Type      string `json:"type"`
	YearMonth string `json:"year_month,omitempty"`
	CreatedAt string `json:"created_at"`
}

// MonthlyKey 月度项目的对外 id：'-' + YYYYMM，天然跨工作台唯一。
func MonthlyKey(ym string) string { return "-" + ym[:4] + ym[5:7] }

type Asset struct {
	ID           int64  `json:"id"`
	ProjectRowID int64  `json:"-"`
	ProjectID    string `json:"project_id"` // 对外项目标识（前端按它跳转/取列表）
	Category     string `json:"category"`
	OriginalName string `json:"original_name"`
	StoredName   string `json:"stored_name"`
	StoredPath   string `json:"stored_path"`
	Ext          string `json:"ext"`
	Size         int64  `json:"size"`
	UploadedAt   string `json:"uploaded_at"`
}

func nowRFC3339() string { return time.Now().Format(time.RFC3339) }

// SearchHit 搜索命中的资产（带所属项目名，不含路径等内部字段）。
type SearchHit struct {
	ID           int64  `json:"id"`
	ProjectRowID int64  `json:"-"`
	ProjectID    string `json:"project_id"`
	ProjectName  string `json:"project_name"`
	Category     string `json:"category"`
	OriginalName string `json:"original_name"`
	Ext          string `json:"ext"`
	Size         int64  `json:"size"`
	UploadedAt   string `json:"uploaded_at"`
}
