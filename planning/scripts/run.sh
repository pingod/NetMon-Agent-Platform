#!/usr/bin/env bash
# IDCBoxLine 一键启动（离线单机）
#
# 用 `bash scripts/run.sh` 启动后用浏览器打开 http://127.0.0.1:8000
#
# ## 为什么一个进程同时服务前端和 API
#
# 部署目标是「离线内网、双击就能用」。分两个进程（nginx + uvicorn）
# 意味着用户要起两个东西、两个端口、两份日志。FastAPI 托管
# 前端静态产物后只有一个进程，一条命令。实测静态响应 <5ms。
#
# ## 为什么 PYTHONHOME 要 unset
#
# 某些发行版（含本开发环境）把 PYTHONHOME 指向了一个不匹配的解释器，
# 会让 venv 里的 python 报 "Failed to import encodings module"。
# 这不是本项目的问题，但启动脚本里必须兜住 —— 用户不该为此调试半小时。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOST="${IDCBOX_HOST:-127.0.0.1}"
PORT="${IDCBOX_PORT:-8000}"
BACKEND="$ROOT/backend"
WEB="$ROOT/web"

echo "==> IDCBoxLine 启动中"

# 1) 确认后端依赖
PY="$BACKEND/.venv/bin/python"
if [[ ! -x "$PY" ]]; then
  echo "错误：未找到 $PY" >&2
  echo "请先运行：uv venv backend/.venv && uv pip install --python backend/.venv/bin/python -e 'backend[infra,api]'" >&2
  exit 1
fi

# 2) 前端产物缺失时构建（离线环境需先 pnpm install）
if [[ ! -f "$WEB/dist/index.html" ]]; then
  echo "==> 前端尚未构建，尝试构建"
  if [[ ! -d "$WEB/node_modules" ]]; then
    echo "错误：$WEB/node_modules 不存在，请先执行 pnpm install --offline" >&2
    exit 1
  fi
  ( cd "$WEB" && pnpm rebuild esbuild && pnpm build )
fi

# 3) 启动
echo "==> 打开 http://$HOST:$PORT"
cd "$BACKEND"
exec env -u PYTHONHOME -u PYTHONPATH \
  "$PY" -m uvicorn idcbox.api.main:create_app \
  --factory --host "$HOST" --port "$PORT" --log-level warning
