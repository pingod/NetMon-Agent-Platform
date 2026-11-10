$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

Write-Host "============================================================"
Write-Host "Cross-Compiling Standalone Binaries (Windows & Linux)..."
Write-Host "============================================================"

$releaseDir = Join-Path $PSScriptRoot "release"
$rootReleaseDir = Join-Path (Split-Path $PSScriptRoot -Parent) "release"
$frontendDist = Join-Path (Split-Path $PSScriptRoot -Parent) "frontend\dist"
$embedDist = Join-Path $PSScriptRoot "internal\modules\frontend_dist"

Write-Host "[0/4] Syncing frontend dist to embed directory..."
if (Test-Path $frontendDist) {
    if (Test-Path $embedDist) {
        Remove-Item -Path $embedDist -Recurse -Force
    }
    Copy-Item -Path $frontendDist -Destination $embedDist -Recurse -Force
    Write-Host "  -> Synced: $frontendDist -> $embedDist"
} else {
    Write-Host "  -> Warning: $frontendDist does not exist, using existing embed files"
}

if (-not (Test-Path $releaseDir)) {
    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
}
if (-not (Test-Path $rootReleaseDir)) {
    New-Item -ItemType Directory -Path $rootReleaseDir -Force | Out-Null
}

$env:CGO_ENABLED = "0"

# 1. Windows x86_64
Write-Host "[1/4] Compiling for Windows x86_64 (amd64)..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$winOut = Join-Path $releaseDir "itops-server-windows-amd64.exe"
go build -ldflags="-s -w" -o $winOut ./cmd/server
Write-Host "  -> Built: $winOut"

# 2. Windows ARM64
Write-Host "[2/4] Compiling for Windows ARM64 (arm64)..."
$env:GOOS = "windows"
$env:GOARCH = "arm64"
$winArmOut = Join-Path $releaseDir "itops-server-windows-arm64.exe"
go build -ldflags="-s -w" -o $winArmOut ./cmd/server
Write-Host "  -> Built: $winArmOut"

# 3. Linux x86_64
Write-Host "[3/4] Compiling for Linux x86_64 (amd64)..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$linuxOut = Join-Path $releaseDir "itops-server-linux-amd64"
go build -ldflags="-s -w" -o $linuxOut ./cmd/server
Write-Host "  -> Built: $linuxOut"

# 4. Linux ARM64 (Kunpeng / Phytium / ARM64)
Write-Host "[4/4] Compiling for Linux ARM64 (arm64)..."
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$armOut = Join-Path $releaseDir "itops-server-linux-arm64"
go build -ldflags="-s -w" -o $armOut ./cmd/server
Write-Host "  -> Built: $armOut"

Write-Host "Syncing all compiled binaries to root release folder..."
Copy-Item -Path (Join-Path $releaseDir "*") -Destination $rootReleaseDir -Force

Write-Host "============================================================"
Write-Host "SUCCESS! All release binaries have been built:"
Write-Host "============================================================"
Get-ChildItem $rootReleaseDir -Filter "itops-server*" | Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
