#!/usr/bin/env bash
# IDCBoxLine 绿色包构建脚本（v1.0，Q3 拍板：双形态、绿色包为交付默认）
#
# ============================================================================
# 这个脚本解决什么问题
# ============================================================================
#
# 产品规划 §8 的交付形态要求：「解压即用、**含运行时**、启动脚本」，
# 因为「实施现场常常只有一台跳板机」，而且国企跳板机往往：
#   - 没有 Docker（所以 Docker 只是备选形态）
#   - 没有 Python 或版本太老（Kylin 自带 3.6/3.8 是常态）
#   - **完全断网**（AC-8：断网环境一条命令完成部署并可用）
#
# 「venv 不可移植」是绿色包最常见的翻车点：venv 里全是绝对路径与
# 平台相关的二进制，打包带走必坏。所以这里的形态是：
#
#   1. **Python 解释器本身装进包**（python-build-standalone，
#      install_only_stripped 变体：linux 31MB + windows 21MB）
#   2. **第三方依赖装成跨平台合并的 site-packages**（uv --target 分别按
#      linux / windows 解析安装，再合并 —— 两平台的二进制文件名后缀不同
#      （*.cpython-312-x86_64-linux-gnu.so vs *.cp312-win_amd64.pyd），
#      实测可共存互不覆盖；纯 Python 文件两边相同，覆盖无害）
#   3. **idcbox 以源码入包，靠 PYTHONPATH 使用**，不走 pip install：
#      免掉构建隔离（hatchling 在目标机不可用），而且 `python -m uvicorn`
#      本来就不需要 entry point
#   4. 启动只有两个脚本：run.sh（Linux/麒麟/CentOS）与 run.bat（Windows），
#      **全程零 pip、零网络** —— 这条要在验收时证明（见文末自检清单）
#
# ============================================================================
# 版本为什么全部钉死（改一个都要过五条验收）
# ============================================================================
#
# - 依赖版本 = pyproject `api`/`infra` extras = 当前验收环境实测版本。
#   1328 用例 + 导出铁线 0 格差异是在这组版本上过的。
# - Python 3.12.11 + PBS tag 20250918 + sha256 双平台指纹写死在下面。
#   打包要求可复现：同一脚本跑两次产物一致（除时间戳）。
# - requires-python 是 >=3.12，取 3.12 是为兼容性最好（现场若自带 3.13+
#   的纯 Python 包也兼容，但我们自带解释器，不依赖现场）。
#
# ============================================================================
# 下载源
# ============================================================================
#
# GitHub Release 直连在本机被 CDN 节流（HEAD 200 但 body 超时），
# gh-proxy 系镜像全灭，TUNA 未同步该仓库 —— 最终用 npmmirror 的
# binary 镜像（uv 官方文档推荐的 UV_PYTHON_INSTALL_MIRROR 同源）。
# 镜像可能漂移，所以：`--python-mirror` 可覆盖，且 sha256 指纹兜底 ——
# 从任何源下载，指纹不对就中止。**不校验哈希的自动下载 = 供应链投毒入口**，
# 这套东西是要进客户内网的。
#
# ============================================================================
# 用法
# ============================================================================
#
#   bash scripts/package_green.sh                 # 完整构建（需联网一次）
#   bash scripts/package_green.sh --skip-python   # 复用缓存的 PBS 归档
#   bash scripts/package_green.sh --fast          # 跳过前端构建与依赖下载
#
# 产物：build/IDCBoxLine-<版本>-offline/            （目录形态）
#       build/IDCBoxLine-<版本>-offline-<日期>.tar.gz（交付形态）
#
# 交付后现场自检（AC-8）：
#   tar xzf IDCBoxLine-*.tar.gz && cd IDCBoxLine-*
#   ./run.sh           # Linux；Windows 双击 run.bat
#   浏览器 http://127.0.0.1:8000
set -euo pipefail

# ----------------------------------------------------------------------------
# 钉死的版本与指纹
# ----------------------------------------------------------------------------
PBS_TAG="20250918"
PBS_PY="3.12.11"
PBS_LINUX_ASSET="cpython-${PBS_PY}+${PBS_TAG}-x86_64-unknown-linux-gnu-install_only_stripped.tar.gz"
PBS_WIN_ASSET="cpython-${PBS_PY}+${PBS_TAG}-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"
# 指纹在开发机对 npmmirror 实际下载结果计算并钉死（见 commit message）。
PBS_LINUX_SHA="d8f71e55f8fd6a4cc9d18ce697969b3326a3325615147e70a9c4b1fa1c3698a8"
PBS_WIN_SHA="891b85c5904bab8c33719fd6ad0c40526a4d9c7d223dacfdb6c9c05cdf4c6771"
DEFAULT_PY_MIRROR="https://registry.npmmirror.com/-/binary/python-build-standalone"
# wheels 的 index：官方 pypi.org 在本机时好时坏（connection reset），
# npmmirror 的 pypi 镜像同源同步。`--pip-mirror` 可覆盖。
DEFAULT_PIP_MIRROR="https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple/"

# 运行时依赖：与 backend/pyproject.toml 的 [infra,api] 逐字一致。
# 在这里重复一份而不是从 pyproject 读，是因为 uv pip install --target
# 装第三方时不带本地包（idcbox 以源码入包）—— 但**两处必须同版本**，
# 改 pyproject 时忘改这里，绿色包就会装出另一套依赖。
RUNTIME_DEPS=(
  "fastapi==0.121.2"
  "uvicorn==0.41.0"
  "sqlalchemy==2.0.44"
  "pydantic==2.14.1"
  "openpyxl==3.1.5"
)

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PY_MIRROR="$DEFAULT_PY_MIRROR"
PIP_MIRROR="$DEFAULT_PIP_MIRROR"
SKIP_PYTHON=0
FAST=0
OUT_DIR="$ROOT/build"
CACHE_DIR="$ROOT/.scratch/pbs-cache"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-python)   SKIP_PYTHON=1; shift ;;
    --fast)          FAST=1; shift ;;
    --python-mirror) PY_MIRROR="$2"; shift 2 ;;
    --pip-mirror)    PIP_MIRROR="$2"; shift 2 ;;
    --out)           OUT_DIR="$2"; shift 2 ;;
    *) echo "未知参数：$1" >&2; exit 2 ;;
  esac
done

VERSION="$(cd "$ROOT/backend" && env -u PYTHONHOME -u PYTHONPATH .venv/bin/python -c \
  'from importlib.metadata import version; print(version("idcbox"))' 2>/dev/null || echo "0.1.0")"
BUILD_DATE="$(date +%Y%m%d)"
PKG_NAME="IDCBoxLine-${VERSION}-offline"
STAGE="$OUT_DIR/$PKG_NAME"

say() { echo -e "\n\033[1==> \033[0m$*"; }
die() { echo "错误：$*" >&2; exit 1; }

# ----------------------------------------------------------------------------
# 0) 前置检查
# ----------------------------------------------------------------------------
say "前置检查"
command -v uv   >/dev/null || die "需要 uv（pip install uv 或 curl -LsSf https://astral.sh/uv/install.sh | sh）"
command -v curl >/dev/null || die "需要 curl"
command -v tar  >/dev/null || die "需要 tar"
[[ -x "$ROOT/web/node_modules/.bin/vite" ]] || die "缺 $ROOT/web/node_modules/.bin/vite（先 pnpm install）"
[[ -f "$ROOT/web/dist/index.html" || $FAST -eq 0 ]] || die "--fast 要求 web/dist 已存在"

# ----------------------------------------------------------------------------
# 1) 前端构建（AGENTS 铁律：不用 pnpm build，直接 vite）
# ----------------------------------------------------------------------------
if [[ $FAST -eq 0 ]]; then
  say "构建前端"
  ( cd "$ROOT/web" && ./node_modules/.bin/vite build >/dev/null && echo "   dist 完成：$(du -sh dist | cut -f1)" )
fi

# ----------------------------------------------------------------------------
# 2) 布局 staging
# ----------------------------------------------------------------------------
#
# **包内相对布局必须与仓库一致**：`api/main.py` 用
# `Path(__file__).parents[3]` 定位仓库根（data/、web/dist、信息港部署上架/
# 都从那算）。改这个布局 = 静默找不到源文件。
say "组装 staging：$STAGE"
rm -rf "$STAGE"
mkdir -p "$STAGE"

# 用 `git ls-files` 从**工作树**复制被跟踪文件 —— 天然排除
# .venv/node_modules/.scratch/data；不用 `git archive HEAD` 是因为那会把
# 尚未提交的修复（比如 pyproject 的 extras）留在包外：验收的是工作树状态，
# 包必须与验收状态一致。
( cd "$ROOT" && git ls-files -z -- \
    backend/idcbox backend/pyproject.toml backend/scripts \
    "信息港部署上架" projects rules ) > /tmp/green-filelist.z
tar --null -C "$ROOT" -cf - -T /tmp/green-filelist.z | tar -xf - -C "$STAGE"
rm -f /tmp/green-filelist.z

mkdir -p "$STAGE/web"
cp -r "$ROOT/web/dist" "$STAGE/web/dist"
mkdir -p "$STAGE/data"
touch "$STAGE/data/.gitkeep"

# ----------------------------------------------------------------------------
# 3) 第三方依赖：双平台 uv --target 后合并
# ----------------------------------------------------------------------------
if [[ $FAST -eq 0 || ! -d "$STAGE/vendor/python" ]]; then
  say "安装跨平台依赖（vendor/python）"
  TMPV="$(mktemp -d)"
  for plat in "x86_64-unknown-linux-gnu" "x86_64-pc-windows-msvc"; do
    UV_CACHE_DIR="$ROOT/.scratch/uvcache" UV_DEFAULT_INDEX="$PIP_MIRROR" uv pip install \
      --target "$TMPV/$plat" \
      --python-version 3.12 --python-platform "$plat" \
      --only-binary=:all: -q "${RUNTIME_DEPS[@]}" \
      || die "依赖解析失败（$plat）。试 --pip-mirror https://pypi.org/simple/"
  done
  mkdir -p "$STAGE/vendor"
  # 合并顺序固定（先 linux 后 windows），产物才可复现。
  cp -r "$TMPV/x86_64-unknown-linux-gnu" "$STAGE/vendor/python"
  cp -r "$TMPV/x86_64-pc-windows-msvc/." "$STAGE/vendor/python/"
  rm -rf "$TMPV"
  echo "   vendor：$(du -sh "$STAGE/vendor" | cut -f1)"
fi

# ----------------------------------------------------------------------------
# 4) 嵌入式 Python 运行时（python-build-standalone）
# ----------------------------------------------------------------------------
ensure_pbs_cache() {  # $1=资产名 $2=期望sha256
  local asset="$1" want="$2"
  if [[ -f "$CACHE_DIR/$asset" ]]; then
    echo "   缓存命中：$asset"
  elif [[ $SKIP_PYTHON -eq 1 ]]; then
    die "--skip-python 但缓存缺 $asset —— 先不带该参数跑一次"
  else
    mkdir -p "$CACHE_DIR"
    local enc="${asset//+/%2B}"
    echo "   下载：$asset"
    curl -fsSL --retry 3 --max-time 600 -o "$CACHE_DIR/$asset" \
      "$PY_MIRROR/$PBS_TAG/$enc" \
      || die "下载失败：$asset（试 --python-mirror 换一个源）"
  fi
  # 指纹校验无条件执行 —— 缓存命中也要，缓存本身可能被污染。
  echo "$want  $CACHE_DIR/$asset" | sha256sum -c - \
    || die "sha256 不匹配：$asset。中止 —— 这是进客户内网的包。"
}

# ## --skip-python 的语义（这里翻过一次车）
#
# 它只意味着「**不联网下载**」，不意味着「runtime 不进包」——
# staging 每次 rm -rf 重建，跳过解包会产出没有解释器的残缺包，
# 而且第一次残缺包差点被当成验收产物。缓存缺失时直接报错中止。
say "组装嵌入式 Python（runtime/）"
mkdir -p "$STAGE/runtime/linux" "$STAGE/runtime/windows"
ensure_pbs_cache "$PBS_LINUX_ASSET" "$PBS_LINUX_SHA"
ensure_pbs_cache "$PBS_WIN_ASSET"  "$PBS_WIN_SHA"
# install_only 归档的顶层就是 python/，直接解到 runtime/<os>/ 下
tar xzf "$CACHE_DIR/$PBS_LINUX_ASSET" -C "$STAGE/runtime/linux"
tar xzf "$CACHE_DIR/$PBS_WIN_ASSET"  -C "$STAGE/runtime/windows"
[[ -x "$STAGE/runtime/linux/python/bin/python3" ]] || die "linux 解释器缺失"
[[ -f "$STAGE/runtime/windows/python/python.exe" ]] || die "windows 解释器缺失"
"$STAGE/runtime/linux/python/bin/python3" --version | sed 's/^/   /'

# 残缺包防线：核心成员任何一个缺失都中止（上一版让一个缺 runtime 的
# staging 溜过了校验 —— 因为它检查在「产物结构校验」之前是死代码路径）
for must in runtime/linux/python/bin/python3 runtime/windows/python/python.exe \
            vendor/python/fastapi web/dist/index.html backend/idcbox/api/main.py; do
  [[ -e "$STAGE/$must" ]] || die "包残缺：$must 不在"
done

# ----------------------------------------------------------------------------
# 5) 启动脚本（进包的，与仓库 scripts/run.sh 是两回事）
# ----------------------------------------------------------------------------
say "生成启动脚本与交付文档"

cat > "$STAGE/run.sh" <<'RUNSH'
#!/usr/bin/env bash
# IDCBoxLine 绿色包启动（Linux / 麒麟 / CentOS）
#
# ## 为什么用 PYTHONPATH 而不是 pip install
#
# 断网跳板机上 pip 的构建隔离不可用（拿不到 hatchling），而
# `python -m uvicorn idcbox.api.main:create_app` 本来就不需要安装 ——
# 源码 + 依赖树都齐在包里，指路即可。**全程不触网、不写系统目录。**
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PY="$DIR/runtime/linux/python/bin/python3"
[[ -x "$PY" ]] || { echo "错误：缺 runtime/linux/python（包不完整？）" >&2; exit 1; }

# 与开发环境同款兜底：PYTHONHOME 污染是本项目老坑（AGENTS §1）
export PYTHONHOME= PYTHONPATH="$DIR/vendor/python:$DIR/backend"
HOST="${IDCBOX_HOST:-127.0.0.1}"
PORT="${IDCBOX_PORT:-8000}"
echo "IDCBoxLine 启动中：http://$HOST:$PORT  （Ctrl+C 停止）"
cd "$DIR/backend"
exec "$PY" -m uvicorn idcbox.api.main:create_app --factory --host "$HOST" --port "$PORT" --log-level warning
RUNSH
chmod +x "$STAGE/run.sh"

# ## bat 必须是 CRLF
#
# LF-only 的 .bat 里 `if (...)` 多行块会被 cmd 解析成「命令语法不正确」，
# 而且不同 Windows 版本行为不一致 —— 这是能毁掉「双击即用」第一印象的坑。
# heredoc 写出来的是 LF，随后 sed 统一转 CRLF。
cat > "$STAGE/run.bat" <<'RUNBAT'
@echo off
rem IDCBoxLine 绿色包启动（Windows x64）
rem 中文乱码时执行：chcp 65001（多数终端无需设置）
chcp 65001 >nul
setlocal
set DIR=%~dp0
set PY=%DIR%runtime\windows\python\python.exe
if not exist "%PY%" (
  echo 错误：缺 runtime\windows\python（包不完整？） 1>&2
  exit /b 1
)
if "%IDCBOX_PORT%"=="" set IDCBOX_PORT=8000
if "%IDCBOX_HOST%"=="" set IDCBOX_HOST=127.0.0.1
set PYTHONHOME=
set PYTHONPATH=%DIR%vendor\python;%DIR%backend
echo IDCBoxLine 启动中：http://%IDCBOX_HOST%:%IDCBOX_PORT%  （Ctrl+C 停止）
cd /d "%DIR%backend"
"%PY%" -m uvicorn idcbox.api.main:create_app --factory --host %IDCBOX_HOST% --port %IDCBOX_PORT% --log-level warning
RUNBAT
sed -i 's/$/\r/' "$STAGE/run.bat"   # LF → CRLF

cat > "$STAGE/自检.sh" <<'SELFCHK'
#!/usr/bin/env bash
# 现场自检：证明「解压 → 数据完整 → 服务可用」全链路不触网。
# AC-8 的验收动作就是这个脚本跑通。
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PY="$DIR/runtime/linux/python/bin/python3"
export PYTHONHOME= PYTHONPATH="$DIR/vendor/python:$DIR/backend"

echo "1) 嵌入式解释器：$("$PY" --version)"
echo "2) 依赖树导入：$(cd "$DIR/backend" && "$PY" -c 'import fastapi,uvicorn,sqlalchemy,openpyxl,pydantic;print("OK")')"
echo "3) 导出保真铁线（约 40 秒）："
(cd "$DIR/backend" && "$PY" scripts/verify_export.py | tail -2)
echo "4) 服务启动 + 接口探测："
# 自检用 8123 而不是 8000：现场机器上很可能已经有一个 IDCBoxLine 在跑
# （开发机更是），撞上会 bind 失败 —— 第一次自检就栽在这里。
#
# 所有 curl 都带 `--noproxy *`：本机探测被公司代理劫持过（测试时把
# http_proxy 指向死端口模拟断网，curl 也跟着走代理 → 永远探测失败，
# 误报「服务未就绪」）。**对 127.0.0.1 的请求本来就不该走代理。**
SELFPORT="${IDCBOX_SELFCHECK_PORT:-8123}"
# 8123 是**自检专用端口**：上面若坐着 idcbox 进程，必然是前几轮自检的孤儿
# —— 按「uvicorn + 该端口」签名清理是准确的（第一次失败轮留下过占端口的
# 僵尸服务，第二次自检被自己的残留挡住）。
pkill -f "uvicorn idcbox.api.main.*--port $SELFPORT" 2>/dev/null || true
sleep 0.5
if curl -sf -o /dev/null --noproxy '*' --max-time 2 "http://127.0.0.1:$SELFPORT/api/health"; then
  echo "   端口 $SELFPORT 上已有服务 —— 换端口重试：IDCBOX_SELFCHECK_PORT=8999 ./自检.sh"
  exit 1
fi
LOG=$(mktemp -t idcbox-selfcheck.XXXXXX)
# ## 为什么必须是 `( … exec ./run.sh ) &`
#
# 写成 `( cd … && ./run.sh ) &` 时，`$!` 拿到的是 **subshell** 的 pid，
# run.sh 里 `exec python` 替换的是 subshell 的**子进程** —— trap 杀错了人，
# uvicorn 变孤儿继续占端口（第一轮实测就留了一个，把第二次自检挡死）。
# 子 shell 里显式 `exec` 让 run.sh 直接顶替 subshell，pid 才对齐。
( cd "$DIR" && exec env IDCBOX_PORT=$SELFPORT ./run.sh ) >"$LOG" 2>&1 &
echo $! >"$LOG.pid"
trap 'kill "$(cat "$LOG.pid")" 2>/dev/null; rm -f "$LOG" "$LOG.pid" 2>/dev/null || true' EXIT
# 60 秒：冷启动要在现场慢机器上也能过（首次加载 V13 约 5~15 秒，
# 自检第 3 步刚跑过全量计算，缓存是热的，但别依赖这一点）
for i in $(seq 1 60); do
  curl -sf -o /dev/null --noproxy '*' --max-time 2 "http://127.0.0.1:$SELFPORT/api/health" && break
  sleep 1
done
curl -sf -o /dev/null --noproxy '*' --max-time 3 "http://127.0.0.1:$SELFPORT/api/health" \
  || { echo "   服务 60 秒内未就绪，启动日志："; cat "$LOG"; exit 1; }
M=$(curl -s --noproxy '*' "http://127.0.0.1:$SELFPORT/api/meta")
SC=$(curl -s --noproxy '*' "http://127.0.0.1:$SELFPORT/api/scene")
# 断言数据真的全量加载（不是起了个空服务）：5127 行连线 + 3D 场景 362 柜
echo "$M" | grep -q '"effective":5127' || { echo "   meta 行数不对：$M"; exit 1; }
echo "$SC" | grep -q '"racks":362' || { echo "   scene 柜数不对"; exit 1; }
echo "   /api/health OK · /api/meta 5127 行 · /api/scene 362 柜"
echo "✅ 自检全部通过：http://127.0.0.1:$SELFPORT"
SELFCHK
chmod +x "$STAGE/自检.sh"

# VERSION + 交付说明
cat > "$STAGE/VERSION" <<EOF
product:  IDCBoxLine
version:  $VERSION
build:    $BUILD_DATE
python:   CPython $PBS_PY (python-build-standalone $PBS_TAG, install_only_stripped)
deps:     ${RUNTIME_DEPS[*]}
sources:  仓库 $(git -C "$ROOT" rev-parse --short HEAD) + web/dist
EOF

cat > "$STAGE/使用说明.txt" <<EOF
IDCBoxLine 机柜上架规划与布线自动化 —— 离线绿色包
====================================================

版本 $(date +%F) 构建 · 适用系统：Linux x86_64（麒麟/CentOS 等）与 Windows x64

【启动】
  Linux：   终端里执行   ./run.sh
  Windows： 双击        run.bat
  浏览器打开 http://127.0.0.1:8000

  十二个页签：平面与机柜 / 接线设计 / 布线表 / 数据体检 / 端口与策略 /
  耗材清单 / 项目与版本 / 版本对比 / 设备台账 / 设备型号库 / 线缆标签 / 3D 机房。

【换端口 / 换数据】
  IDCBOX_PORT=8123 ./run.sh
  IDCBOX_SOURCE=/路径/V13xxx.xlsx ./run.sh
  IDCBOX_INTENT=/路径/intent.json ./run.sh

【自检】（验收时执行；证明解压即可用）
  ./自检.sh

【数据与项目】
  - 客户源数据在「信息港部署上架/」，只读，勿改。
  - 项目库在 data/idcbox.db（首次启动自动创建）。要预置 V13 全量意图，
    把 projects/v13/intent_full.json 用「项目与版本」页导入，或用命令行：
    runtime/linux/python/bin/python3 -m idcbox.cli project-save --name V13 \\
      --intent projects/v13/intent_full.json   （先 cd backend 并设 PYTHONPATH，参考 自检.sh）
  - 整个目录可拷到任意位置，不需要管理员权限，不写注册表。

【常见故障】
  - 端口被占：换个端口（见上）。
  - 页面空白：确认用 127.0.0.1 而不是 localhost（某些内网 DNS 解析慢）。
  - 3D 页黑屏/报错：浏览器设置里开启「硬件加速」；无显卡时系统会走软件
    渲染（实测可用），若仍失败请改用「平面与机柜」页。
EOF

# ----------------------------------------------------------------------------
# 6) 校验包内跨平台文件真的共存（合并步骤的回归位）
# ----------------------------------------------------------------------------
say "产物结构校验"
[[ -f "$STAGE/backend/idcbox/api/main.py" ]] || die "backend 源码缺失"
[[ -d "$STAGE/web/dist" ]] || die "web/dist 缺失"
[[ -f "$STAGE/信息港部署上架/V13(内部)甘肃信息港-设备上架规划-20260714.xlsx" ]] || die "V13 源数据缺失"
[[ -f "$STAGE/rules/len_empirical_v1.json" ]] || die "经验表缺失"
[[ -f "$STAGE/vendor/python/pydantic_core/_pydantic_core.cpython-312-x86_64-linux-gnu.so" ]] || die "linux pydantic_core 缺失"
[[ -f "$STAGE/vendor/python/pydantic_core/_pydantic_core.cp312-win_amd64.pyd" ]] || die "windows pydantic_core 缺失"
echo "   两平台二进制共存校验：OK"
# bat 是 LF 的话 cmd 解析多行 `if (...)` 块直接报「命令语法不正确」，
# 而这个问题**在 Linux 上永远测不出来** —— 必须在构建期断言 CRLF。
head -c 4096 "$STAGE/run.bat" | grep -q $'\r' || die "run.bat 不是 CRLF（cmd 下必坏）"
grep -q $'\r$' "$STAGE/run.bat" || die "run.bat 行尾缺 CR"
echo "   run.bat CRLF 校验：OK"

# ----------------------------------------------------------------------------
# 7) 清单与压缩
# ----------------------------------------------------------------------------
( cd "$STAGE" && find . -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS.txt )

TARBALL="$OUT_DIR/$PKG_NAME-$BUILD_DATE.tar.gz"
say "打包 $TARBALL"
# mtime 固定 + gzip 无时间戳：同一提交跑两次，tar 内容一致（时间戳类字段除外）
tar --sort=name --mtime="1970-01-01 00:00:00Z" --owner=0 --group=0 -C "$OUT_DIR" -cf - "$PKG_NAME" \
  | gzip -n > "$TARBALL"

say "完成"
{
  echo "目录：$STAGE"
  echo "归档：$TARBALL（$(du -h "$TARBALL" | cut -f1)）"
  echo "归档 sha256：$(sha256sum "$TARBALL" | cut -d' ' -f1)"
  echo
  echo "交付后现场验收（AC-8，全程不需要网络）："
  echo "  tar xzf $(basename "$TARBALL") && cd $PKG_NAME && ./自检.sh"
} | tee "$OUT_DIR/$PKG_NAME-$BUILD_DATE.txt"
