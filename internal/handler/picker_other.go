//go:build !windows

package handler

import "errors"

func pickFolderCmd() (string, error) {
	return "", errors.New("仅支持在 Windows 上原生选择目录")
}
