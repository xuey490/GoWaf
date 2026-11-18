# GoWAF 容器镜像
#
# 关键事实：前端构建产物已提交在 internal/web/dist（go:embed 内嵌），
# 镜像构建不需要 node/pnpm，克隆仓库 + docker build 即可。
# 数据面 CGO_ENABLED=0 纯静态编译，运行镜像用最小 alpine + CA 证书
# （ACME 签发、威胁情报源拉取都要 HTTPS 出网）。

# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
ARG VERSION=docker
ARG COMMIT=none
WORKDIR /src
# 先拷 go.mod/go.sum 单独 download：依赖不变时利用层缓存
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X 'main.version=${VERSION}' -X 'main.commit=${COMMIT}' -X 'main.buildTime=unknown'" \
    -o /out/gowaf ./cmd/gowaf

FROM alpine:3.20
# ca-certificates：ACME/情报源 HTTPS 出网；tzdata：日志与聚合窗口按时区
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 -h /data waf \
    && mkdir -p /data /etc/gowaf && chown -R waf:waf /data
COPY --from=build /out/gowaf /usr/local/bin/gowaf
COPY rules/builtin.yaml /app/rules/builtin.yaml
COPY deploy/config.docker.yaml /etc/gowaf/config.yaml

# 数据卷：SQLite 数据库、证书目录、JWT 密钥、ACME 账户状态全部落在 /data
VOLUME ["/data"]
EXPOSE 80 443 8899
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8899/healthz >/dev/null 2>&1 || exit 1
USER waf
ENTRYPOINT ["gowaf", "-c", "/etc/gowaf/config.yaml"]
