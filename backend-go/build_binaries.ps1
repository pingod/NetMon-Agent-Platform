$ErrorActionPreference = "Stop"

Write-Host "============================================================"
Write-Host "Compiling Go Binaries for Windows and Linux..."
Write-Host "============================================================"

$releaseDir = Join-Path $PSScriptRoot "release"
$rootReleaseDir = Join-Path $PSScriptRoot "..\release"

if (-not (Test-Path $releaseDir)) {
    New-Item -ItemType Directory -Path $releaseDir | Out-Null
}
if (-not (Test-Path $rootReleaseDir)) {
    New-Item -ItemType Directory -Path $rootReleaseDir | Out-Null
}

$env:CGO_ENABLED = "0"

# 1. Windows amd64
Write-Host "[1/4] Compiling Windows AMD64 (x86_64)..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o (Join-Path $releaseDir "itops-server-windows-amd64.exe") .\cmd\server
Copy-Item -Force (Join-Path $releaseDir "itops-server-windows-amd64.exe") (Join-Path $rootReleaseDir "itops-server-windows-amd64.exe")
Write-Host "  -> Done: itops-server-windows-amd64.exe"

# 2. Windows arm64
Write-Host "[2/4] Compiling Windows ARM64..."
$env:GOOS = "windows"
$env:GOARCH = "arm64"
go build -ldflags="-s -w" -o (Join-Path $releaseDir "itops-server-windows-arm64.exe") .\cmd\server
Copy-Item -Force (Join-Path $releaseDir "itops-server-windows-arm64.exe") (Join-Path $rootReleaseDir "itops-server-windows-arm64.exe")
Write-Host "  -> Done: itops-server-windows-arm64.exe"

# 3. Linux amd64
Write-Host "[3/4] Compiling Linux AMD64 (x86_64)..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o (Join-Path $releaseDir "itops-server-linux-amd64") .\cmd\server
Copy-Item -Force (Join-Path $releaseDir "itops-server-linux-amd64") (Join-Path $rootReleaseDir "itops-server-linux-amd64")
Write-Host "  -> Done: itops-server-linux-amd64"

# 4. Linux arm64
Write-Host "[4/4] Compiling Linux ARM64 (Kunpeng / Phytium / ARM)..."
$env:GOOS = "linux"
$env:GOARCH = "arm64"
go build -ldflags="-s -w" -o (Join-Path $releaseDir "itops-server-linux-arm64") .\cmd\server
Copy-Item -Force (Join-Path $releaseDir "itops-server-linux-arm64") (Join-Path $rootReleaseDir "itops-server-linux-arm64")
Write-Host "  -> Done: itops-server-linux-arm64"

Write-Host "============================================================"
Write-Host "SUCCESS! All binaries built and copied to release directories:"
Write-Host "  - $releaseDir"
Write-Host "  - $rootReleaseDir"
Write-Host "============================================================"

Get-ChildItem $releaseDir
