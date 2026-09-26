//go:build windows

package handler

import (
	"os/exec"
	"strings"
	"syscall"
)

// pickFolderCmd 用 PowerShell 弹原生文件夹选择框，返回选中的绝对路径。
// 隐藏窗口、STA 单线程套间（FolderBrowserDialog 要求）；取消时返回空串。
func pickFolderCmd() (string, error) {
	ps := `
Add-Type -AssemblyName System.Windows.Forms
$f = New-Object System.Windows.Forms.FolderBrowserDialog
$f.Description = '选择工作目录'
$f.ShowNewFolderButton = $true
if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $f.SelectedPath }
`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
