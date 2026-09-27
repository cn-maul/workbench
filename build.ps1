# 工作台构建脚本：
#   [1/2] 构建前端（vite -> internal/web/dist）
#   [2/2] Go 内嵌打包 -> 项目根目录 Workbench.exe
#
# 用法：pwsh -File build.ps1   （PowerShell 7；Windows PowerShell 5.1 亦兼容，
# 为此本文件保存为 UTF-8 带 BOM——5.1 读无 BOM 的 UTF-8 会按 GBK 误解中文。）

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

foreach ($cmd in 'node', 'npm', 'go') {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        Write-Host "缺少命令：$cmd，请先安装后重试" -ForegroundColor Red
        exit 1
    }
}

Write-Host '[1/2] 构建前端（vite build -> internal/web/dist）…'
Push-Location frontend
try {
    if (-not (Test-Path node_modules)) {
        Write-Host '    未找到 node_modules，先执行 npm install…'
        npm install
        if ($LASTEXITCODE -ne 0) { throw 'npm install 失败' }
    }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'npm run build 失败' }
} finally {
    Pop-Location
}

Write-Host '[2/2] 构建后端（go build -> Workbench.exe）…'
go build -trimpath -ldflags '-s -w -H windowsgui' -o Workbench.exe .
if ($LASTEXITCODE -ne 0) { throw 'go build 失败' }

$exe = Get-Item (Join-Path $PSScriptRoot 'Workbench.exe')
$mb = [math]::Round($exe.Length / 1MB, 1)
Write-Host "完成：$($exe.FullName)（$mb MB）" -ForegroundColor Green
