@echo off
echo ============================================================
echo [1/3] Building Frontend Web Assets (Vite)...
echo ============================================================
cd /d "%~dp0frontend"
call npm run build
if %errorlevel% neq 0 (
    echo [ERROR] Frontend build failed!
    pause
    exit /b %errorlevel%
)

echo.
echo ============================================================
echo [2/3] Syncing Frontend Dist to Go Embed Directory...
echo ============================================================
if exist "%~dp0backend-go\internal\modules\frontend_dist" (
    rmdir /s /q "%~dp0backend-go\internal\modules\frontend_dist"
)
xcopy /E /I /Y "%~dp0frontend\dist" "%~dp0backend-go\internal\modules\frontend_dist"

echo.
echo ============================================================
echo [3/3] Cross-Compiling Standalone Binaries (Windows and Linux)...
echo ============================================================
cd /d "%~dp0backend-go"
if not exist "%~dp0backend-go\release" mkdir "%~dp0backend-go\release"

set CGO_ENABLED=0

:: 1. Windows x86_64
echo - Compiling for Windows x86_64...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o "%~dp0backend-go\release\itops-server-windows-amd64.exe" .\cmd\server

:: 2. Windows ARM64
echo - Compiling for Windows ARM64...
set GOOS=windows
set GOARCH=arm64
go build -ldflags="-s -w" -o "%~dp0backend-go\release\itops-server-windows-arm64.exe" .\cmd\server

:: 3. Linux x86_64
echo - Compiling for Linux x86_64...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o "%~dp0backend-go\release\itops-server-linux-amd64" .\cmd\server

:: 4. Linux ARM64 (Kunpeng / Phytium / ARM)
echo - Compiling for Linux ARM64...
set GOOS=linux
set GOARCH=arm64
go build -ldflags="-s -w" -o "%~dp0backend-go\release\itops-server-linux-arm64" .\cmd\server

:: Copy to root release directory if exists
if exist "%~dp0release" (
    copy /Y "%~dp0backend-go\release\*" "%~dp0release\"
)

echo.
echo ============================================================
echo SUCCESS! All release binaries have been built to:
echo %~dp0release\  and  %~dp0backend-go\release\
echo ============================================================
dir "%~dp0backend-go\release"
if "%~1" neq "--no-pause" pause

