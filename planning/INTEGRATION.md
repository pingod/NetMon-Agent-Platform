# 规划模块集成契约

本目录原为独立仓库 `IDCBoxLine`，现作为 NetMon-Agent-Platform 的内置模块运行。
本文只记录**与主平台的接缝**；域层、公式与验收标准仍以 `docs/HANDOFF.md` 为准。

## 进程模型

```
Browser
  └─ /planning            NetMon React 路由（iframe）
       └─ /planning-app/  Go 内嵌的规划前端（Vite 产物）
            └─ /api/v1/planning/*  Go 认证代理
                 └─ 127.0.0.1:18001  Python 引擎（uvicorn + FastAPI）
```

- Go 侧实现：`backend-go/internal/modules/planning/`。
- 引擎**只绑定 127.0.0.1**，不对宿主机暴露端口；认证由 NetMon JWT 完成。
- 引擎不可用时：`/api/v1/planning-status` 返回状态与原因，代理返回 503；主平台其余功能不受影响。
- 退出信号会先停 HTTP、再停止引擎（SIGINT），最多等待 5 秒。

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PLANNING_ENABLED` | `true` | `false` 时完全跳过引擎 |
| `PLANNING_ROOT` | 自动查找 `./planning` / 二进制同级 `planning/` | 引擎根目录 |
| `PLANNING_ENGINE_DIR` | `<root>/backend` | `idcbox` 包所在目录 |
| `PLANNING_PYTHON` | 自动：绿色包 runtime → `backend/.venv` → `python3` | 解释器 |
| `PLANNING_PORT` | `18001`（占用时顺延） | 本地端口 |
| `PLANNING_DB` | `<root>/data/idcbox.db` | 项目版本库；不存在时从种子库复制 |
| `PLANNING_STATIC_DIR` | 空 | 指向 `planning/web/dist` 可替代嵌入产物（开发用） |

绿色包（`scripts/package_green.sh` 产物）自带 `runtime/` 与 `vendor/`：
把 `PLANNING_ROOT` 指向解压目录即可，Go 会自动设置 `PYTHONHOME=` / `PYTHONPATH=vendor:backend`。

## 构建

```bash
# 规划前端 → Go embed 目录
cd planning/web
pnpm install --frozen-lockfile
pnpm rebuild esbuild                      # pnpm 10/11 构建脚本策略不一致
VITE_BASE=/planning-app/ VITE_API_BASE=/api/v1/planning pnpm build
rm -rf ../../backend-go/internal/modules/planning/dist
cp -r dist/. ../../backend-go/internal/modules/planning/dist/
```

`build_release.sh` 与 `Dockerfile` 已包含上述步骤。Docker 运行时基于 `python:3.12-slim`，
按 `backend/pyproject.toml` 的 `[infra,api]` 钉版安装依赖；非 Docker 独立二进制需自备
Python 3.12 或使用绿色包。

## 数据

规划项目库独立为 `planning.db`：它保存版本化意图文档（`intent_document`）与从 Excel 反解的
规范表，生命周期与 NetMon `app.db` 的资产表不同。两者同处 `data/` 目录、同一备份策略，
但不做跨库外键 —— 避免一次 Schema 迁移同时改两套模型。

- 种子库：`planning/data/idcbox.db`（V13 / 信息港二期，含意图版本）。
- 运行库：默认 `planning/data/idcbox.db`；容器内为 `/app/data/planning.db`。
- 源 Excel：`planning/信息港部署上架/`（只读，用于 `N/O` 几何与导出铁线）。
- 意图：`planning/projects/v13/intent*.json`；规则：`planning/rules/`；黄金向量：`planning/shared/golden/`。

## 验收

```bash
cd planning/backend
env -u PYTHONHOME -u PYTHONPATH .venv/bin/python -m pytest tests/ -p no:warnings
env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/verify_export.py   # 必须 0 格差异
cd ../web && ./node_modules/.bin/vitest run && ./node_modules/.bin/tsc --noEmit
```

主平台侧：

```bash
cd backend-go && go build ./cmd/server
curl -s -H "Authorization: Bearer <token>" http://127.0.0.1:8088/api/v1/planning-status
curl -s -H "Authorization: Bearer <token>" http://127.0.0.1:8088/api/v1/planning/api/meta
```
