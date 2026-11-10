# syntax=docker/dockerfile:1

# ── Stage 1: 构建前端 (Vite) ──
FROM node:22-alpine AS frontend
WORKDIR /build/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

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
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/itops-server ./cmd/server

# ── Stage 3: 运行时 ──
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata iputils curl \
    && addgroup -S itops && adduser -S -G itops itops \
    && mkdir -p /app/data /app/logs \
    && chown -R itops:itops /app

WORKDIR /app
COPY --from=backend --chown=itops:itops /out/itops-server /app/itops-server

ENV NETMON_PACKAGED=true \
    NODE_ENV=production \
    PORT=8088 \
    DATABASE_PATH=/app/data/app.db \
    UPLOAD_DIR=/app/data/uploads \
    TZ=Asia/Shanghai

USER itops
EXPOSE 8088
VOLUME ["/app/data", "/app/logs"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8088/api/health || exit 1

ENTRYPOINT ["/app/itops-server"]
