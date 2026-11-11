#!/usr/bin/env bash
# 开发环境：为规划引擎创建独立 venv（Python 3.12）。
# 生产环境不需要本脚本：Docker 镜像已内置依赖，绿色包自带运行时。
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PY="${PLANNING_BOOTSTRAP_PYTHON:-python3.12}"

if ! command -v "$PY" >/dev/null 2>&1; then
  echo "未找到 $PY；请用 PLANNING_BOOTSTRAP_PYTHON 指定 Python 3.12 解释器" >&2
  exit 1
fi

cd "$DIR/backend"
"$PY" -m venv .venv
.venv/bin/python -m pip install --upgrade pip
# 直接按 pyproject 的 [infra,api] 钉版安装。不做 editable 安装：
# 引擎以源码目录为 cwd 启动（python -m uvicorn），不依赖 site-packages 元数据。
.venv/bin/python -m pip install \
  "fastapi==0.121.2" "uvicorn==0.41.0" "sqlalchemy==2.0.44" \
  "pydantic==2.14.1" "openpyxl==3.1.5"
# 可选：跑回归时再装
# .venv/bin/python -m pip install pytest==9.1.1 pytest-cov==7.1.0 import-linter==2.7

echo "规划引擎已就绪：$DIR/backend/.venv"
echo "启动 NetMon 后将自动发现；也可单独运行：bash $DIR/scripts/run.sh"
