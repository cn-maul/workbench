package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"workbench/internal/model"
)

// 文件库扩展名白名单
var FileWhitelist = map[string]bool{
	"jpg": true, "jpeg": true, "png": true,
	"doc": true, "docx": true,
	"ppt": true, "pptx": true,
	"xls": true, "xlsx": true,
	"pdf": true,
	"zip": true, "rar": true, "7z": true,
}

var datePrefixRe = regexp.MustCompile(`^\d{8}_`)
var trailSuffixRe = regexp.MustCompile(`_(\d+)$`)
var titleIllegalRe = regexp.MustCompile(`[\\/:*?"<>|]`)

const MaxTitleRunes = 50

// ProjectDir 返回项目根目录。
func ProjectDir(root string, p *model.Project) (string, error) {
	switch p.Type {
	case model.TypeMonthly:
		return filepath.Join(root, "monthly", p.YearMonth[:4], p.YearMonth), nil
	case model.TypeCustom:
		entries, err := os.ReadDir(filepath.Join(root, "custom"))
		if err != nil {
			return "", err
		}
		prefix := fmt.Sprintf("%d_", p.ID)
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
				return filepath.Join(root, "custom", e.Name()), nil
			}
		}
		return "", fmt.Errorf("找不到 id=%d 的自定义项目目录", p.ID)
	}
	return "", fmt.Errorf("未知项目类型 %q", p.Type)
}

func CategoryDir(projDir, category string) string {
	if category == model.CatRecord {
		return filepath.Join(projDir, "records")
	}
	return filepath.Join(projDir, "files")
}

func RelPath(root, projDir, category, storedName string) string {
	rel, _ := filepath.Rel(root, filepath.Join(projDir, subdir(category), storedName))
	return filepath.ToSlash(rel)
}

func subdir(category string) string {
	if category == model.CatRecord {
		return "records"
	}
	return "files"
}

// CleanBaseName 取纯文件名并清洗（防路径穿越、拒空、强制 UTF-8）。
// 非 UTF-8 文件名（老 Windows 客户端 GBK 字节）做合法化清洗，保证清单可序列化。
func CleanBaseName(name string) (string, error) {
	n := filepath.Base(filepath.ToSlash(strings.TrimSpace(name)))
	if n == "" || n == "." || n == "/" || n == "\\" {
		return "", errors.New("文件名为空")
	}
	if !utf8.ValidString(n) {
		// 老 Windows 客户端会把文件名按本地代码页(GBK)发送，尝试转码
		if decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes([]byte(n)); err == nil && utf8.Valid(decoded) {
			n = string(decoded)
		} else {
			n = strings.ToValidUTF8(n, "?")
		}
	}
	if r := []rune(n); len(r) > 150 {
		ext := filepath.Ext(n)
		n = string(r[:150-len([]rune(ext))]) + ext
	}
	return n, nil
}

// SanitizeTitle 清洗记录标题：去非法字符与尾部 .md，限长 50，空则兜底。
func SanitizeTitle(title string) string {
	t := strings.TrimSpace(title)
	t = strings.ReplaceAll(filepath.Base(filepath.ToSlash(t)), "\\", "")
	t = titleIllegalRe.ReplaceAllString(t, "")
	t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(t, ".md"), ".MD"))
	if r := []rune(t); len(r) > MaxTitleRunes {
		t = string(r[:MaxTitleRunes])
	}
	if t == "" {
		t = "未命名记录" + time.Now().Format("20060102")
	}
	return t
}

// CheckExt 校验扩展名是否允许进入指定库。
func CheckExt(category, baseName string) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(baseName), "."))
	if category == model.CatRecord {
		if ext != "md" {
			return errors.New("记录库只允许 .md 文件")
		}
		return nil
	}
	if ext == "" {
		return errors.New("缺少扩展名")
	}
	if !FileWhitelist[ext] {
		return fmt.Errorf("不允许的文件类型: %s", ext)
	}
	return nil
}

// AllocateStoredName 按 5.4 规则生成存储名并在目录内排重。
func AllocateStoredName(catDir, originalName string) (string, error) {
	if originalName == "" {
		return "", errors.New("原始名为空")
	}
	ext := filepath.Ext(originalName)
	stem := strings.TrimSuffix(originalName, ext)
	base := time.Now().Format("20060102") + "_" + stem
	for i := 1; ; i++ {
		cand := base + ext
		if i > 1 {
			cand = fmt.Sprintf("%s_%d%s", base, i, ext)
		}
		if _, err := os.Stat(filepath.Join(catDir, cand)); errors.Is(err, os.ErrNotExist) {
			return cand, nil
		} else if err != nil {
			return "", err
		}
	}
}

// AtomicWrite 临时文件 → fsync → 原子重命名。
func AtomicWrite(finalPath string, r io.Reader) (int64, error) {
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(dir, ".tmp_*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	size, err := io.Copy(tmp, r)
	if err == nil {
		err = tmp.Sync()
	}
	tmp.Close()
	if err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	return size, nil
}

// AtomicWriteBytes 便捷版。
func AtomicWriteBytes(finalPath string, b []byte) error {
	_, err := AtomicWrite(finalPath, strings.NewReader(string(b)))
	return err
}

// CleanTemps 删除目录树内残留的 .tmp_* 文件。
func CleanTemps(root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasPrefix(info.Name(), ".tmp_") {
			os.Remove(path)
		}
		return nil
	})
}

// FallbackMeta 清单缺失时按存储名推断原始名。
func FallbackMeta(storedName string, modTime time.Time) (originalName string, uploadedAt string) {
	originalName = storedName
	if datePrefixRe.MatchString(storedName) {
		originalName = datePrefixRe.ReplaceAllString(storedName, "")
	}
	return originalName, modTime.Format(time.RFC3339)
}

// StripTrailDup 去掉原始名stem尾部的 _N（用于引用兜底匹配）。
func StripTrailDup(name string) (string, bool) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	m := trailSuffixRe.FindStringSubmatch(stem)
	if m == nil {
		return name, false
	}
	return strings.TrimSuffix(stem, m[0]) + ext, true
}
