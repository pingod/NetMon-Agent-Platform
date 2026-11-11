#!/bin/bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

echo "============================================================"
echo "[1/4] Building Frontend Web Assets (Vite)..."
echo "============================================================"
cd "$DIR/frontend"
npm run build

echo ""
echo "============================================================"
echo "[2/4] Building IDC Planning Web Assets (Vite)..."
echo "============================================================"
cd "$DIR/planning/web"
if [ ! -d node_modules ]; then
  pnpm install --frozen-lockfile
fi
# pnpm 10/11 对构建脚本的默认策略不一致；显式重建 esbuild，避免 Vite 启动失败
pnpm rebuild esbuild
VITE_BASE=/planning-app/ VITE_API_BASE=/api/v1/planning pnpm build

echo ""
echo "============================================================"
echo "[3/4] Syncing Web Dist to Go Embed Directories..."
echo "============================================================"
rm -rf "$DIR/backend-go/internal/modules/frontend_dist"
cp -r "$DIR/frontend/dist" "$DIR/backend-go/internal/modules/frontend_dist"
rm -rf "$DIR/backend-go/internal/modules/planning/dist"
mkdir -p "$DIR/backend-go/internal/modules/planning/dist"
cp -r "$DIR/planning/web/dist/." "$DIR/backend-go/internal/modules/planning/dist/"

echo ""
echo "============================================================"
echo "[4/4] Cross-Compiling Standalone Binaries (Windows & Linux)..."
echo "============================================================"
cd "$DIR/backend-go"
mkdir -p "$DIR/backend-go/release"

export CGO_ENABLED=0

echo "- Compiling for Windows x86_64..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$DIR/backend-go/release/itops-server-windows-amd64.exe" ./cmd/server

echo "- Compiling for Windows ARM64..."
GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o "$DIR/backend-go/release/itops-server-windows-arm64.exe" ./cmd/server

echo "- Compiling for Linux x86_64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$DIR/backend-go/release/itops-server-linux-amd64" ./cmd/server

echo "- Compiling for Linux ARM64 (Kunpeng / Phytium / ARM)..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$DIR/backend-go/release/itops-server-linux-arm64" ./cmd/server

echo ""
echo "============================================================"
echo "SUCCESS! All release binaries have been built to:"
echo "$DIR/backend-go/release/"
echo "============================================================"
echo "提示：独立二进制需配套规划引擎运行环境（Python 3.12 或绿色包）。"
echo "      将 planning/ 放到二进制同级目录即可自动发现；"
echo "      或用 PLANNING_ROOT / PLANNING_PYTHON 显式指定。"
ls -lh "$DIR/backend-go/release/"
