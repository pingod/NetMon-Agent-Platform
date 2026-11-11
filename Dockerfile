# syntax=docker/dockerfile:1

# ── Stage 1: 构建主前端 (Vite) ──
FROM node:22-alpine AS frontend
WORKDIR /build/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ── Stage 1b: 构建 IDC 上架规划前端（嵌入 Go） ──
FROM node:22-alpine AS planning-web
RUN npm install -g pnpm@10
WORKDIR /build/planning-web
COPY planning/web/package.json planning/web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile \
    && pnpm rebuild esbuild
COPY planning/web/ ./
# base/API 前缀必须与 Go 的 /planning-app、/api/v1/planning 路由一致
RUN VITE_BASE=/planning-app/ VITE_API_BASE=/api/v1/planning pnpm build

# ── Stage 2: 编译 Go 后端（前端产物内嵌进二进制） ──
FROM golang:1.25-alpine AS backend
ARG GOPROXY=https://goproxy.cn,https://goproxy.io,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY backend-go/go.mod backend-go/go.sum ./
RUN go mod download
COPY backend-go/ ./
# 用最新前端构建产物替换仓库内的历史产物
RUN rm -rf internal/modules/frontend_dist \
    && mkdir -p internal/modules/frontend_dist
COPY --from=frontend /build/frontend/dist/ ./internal/modules/frontend_dist/
RUN rm -rf internal/modules/planning/dist \
    && mkdir -p internal/modules/planning/dist
COPY --from=planning-web /build/planning-web/dist/ ./internal/modules/planning/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/itops-server ./cmd/server

# ── Stage 3: 运行时（Go 主服务 + 规划引擎） ──
FROM python:3.12-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r itops && useradd -r -g itops -d /app -s /sbin/nologin itops \
    && mkdir -p /app/data /app/logs \
    && chown -R itops:itops /app

# 规划引擎依赖：与 planning/backend/pyproject.toml 的 [infra,api] 钉版一致。
# 国内构建可传 --build-arg PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ARG PIP_INDEX_URL=https://pypi.org/simple
ENV PIP_INDEX_URL=${PIP_INDEX_URL} PIP_DEFAULT_TIMEOUT=120
RUN pip install --no-cache-dir --retries 10 --timeout 120 \
    fastapi==0.121.2 \
    uvicorn==0.41.0 \
    sqlalchemy==2.0.44 \
    pydantic==2.14.1 \
    openpyxl==3.1.5

WORKDIR /app
COPY --from=backend --chown=itops:itops /out/itops-server /app/itops-server

# 引擎源码与数据资产；前端由 Go 二进制内嵌，不再单独提供
COPY --chown=itops:itops planning/backend/idcbox /app/planning/backend/idcbox
COPY --chown=itops:itops planning/data /app/planning/data
COPY --chown=itops:itops planning/projects /app/planning/projects
COPY --chown=itops:itops planning/rules /app/planning/rules
COPY --chown=itops:itops planning/shared /app/planning/shared
COPY --chown=itops:itops planning/信息港部署上架 /app/planning/信息港部署上架

ENV NETMON_PACKAGED=true \
    NODE_ENV=production \
    PORT=8088 \
    DATABASE_PATH=/app/data/app.db \
    UPLOAD_DIR=/app/data/uploads \
    PLANNING_ROOT=/app/planning \
    PLANNING_DB=/app/data/planning.db \
    PLANNING_PYTHON=/usr/local/bin/python3 \
    TZ=Asia/Shanghai

USER itops
EXPOSE 8088
VOLUME ["/app/data", "/app/logs"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8088/api/health || exit 1

ENTRYPOINT ["/app/itops-server"]
