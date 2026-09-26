package model

import "time"

const (
	TypeMonthly = "monthly"
	TypeCustom  = "custom"

	CatRecord = "record"
	CatFile   = "file"
)

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	YearMonth string `json:"year_month,omitempty"`
	CreatedAt string `json:"created_at"`
}

type Asset struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	Category     string `json:"category"`
	OriginalName string `json:"original_name"`
	StoredName   string `json:"stored_name"`
	StoredPath   string `json:"stored_path"`
	Ext          string `json:"ext"`
	Size         int64  `json:"size"`
	UploadedAt   string `json:"uploaded_at"`
}

func nowRFC3339() string { return time.Now().Format(time.RFC3339) }
