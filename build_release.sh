#!/bin/bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

echo "============================================================"
echo "[1/3] Building Frontend Web Assets (Vite)..."
echo "============================================================"
cd "$DIR/frontend"
npm run build

echo ""
echo "============================================================"
echo "[2/3] Syncing Frontend Dist to Go Embed Directory..."
echo "============================================================"
rm -rf "$DIR/backend-go/internal/modules/frontend_dist"
cp -r "$DIR/frontend/dist" "$DIR/backend-go/internal/modules/frontend_dist"

echo ""
echo "============================================================"
echo "[3/3] Cross-Compiling Standalone Binaries (Windows & Linux)..."
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
ls -lh "$DIR/backend-go/release/"
