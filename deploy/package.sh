#!/usr/bin/env bash
#
# GoWAF Linux 打包：交叉编译二进制并打成可直接上传的 tar.gz。
# 对应 Windows 侧的 deploy/package.ps1。
#
# 用法：
#   bash deploy/package.sh              # linux/amd64
#   bash deploy/package.sh arm64        # linux/arm64
#
set -Eeuo pipefail

ARCH=${1:-amd64}
[[ $ARCH == amd64 || $ARCH == arm64 ]] || { echo "Arch 只接受 amd64 或 arm64" >&2; exit 1; }

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
[[ -f go.mod ]] || { echo "找不到 go.mod" >&2; exit 1; }

step() { printf '\n==> %s\n' "$*"; }
ok()   { printf '    [OK] %s\n' "$*"; }

step '检查构建工具'
command -v go >/dev/null || { echo "找不到 go" >&2; exit 1; }
ok "$(go version)"

step '准备前端产物（go:embed 需要 internal/web/dist）'
if [[ ! -f internal/web/dist/index.html ]]; then
  echo "internal/web/dist 缺少 index.html（仓库应自带前端产物）" >&2
  exit 1
fi
ok '使用仓库自带的 internal/web/dist'

step "交叉编译 linux/${ARCH}"
VER=$(git describe --tags --always --dirty 2>/dev/null || echo "dev-$(date -u +%Y%m%d-%H%M)")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo none)
BUILT_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X 'main.version=${VER}' -X 'main.commit=${COMMIT}' -X 'main.buildTime=${BUILT_AT}'"

mkdir -p dist
EXE_NAME="gowaf-linux-${ARCH}"
CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build -trimpath -ldflags "$LDFLAGS" -o "dist/${EXE_NAME}" ./cmd/gowaf
SIZE=$(du -h "dist/${EXE_NAME}" | awk '{print $1}')
ok "${EXE_NAME}  ${SIZE}  (${VER})"

step '组装部署包'
STAGE_NAME="gowaf-deploy-${VER}-linux-${ARCH}"
STAGE="dist/${STAGE_NAME}"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp "dist/${EXE_NAME}" "$STAGE/"
cp rules/builtin.yaml "$STAGE/"
cp deploy/install.sh "$STAGE/"
chmod +x "$STAGE/install.sh" "$STAGE/${EXE_NAME}"

RULE_COUNT=$(grep -c '^- id:' rules/builtin.yaml || true)
cat > "${STAGE}/README.txt" <<EOF
GoWAF 部署包  ${VER}  (linux/${ARCH})
================================================================

包含：${EXE_NAME} / install.sh / builtin.yaml（规则 ${RULE_COUNT} 条）

第一步：先体检（只读，不改任何东西）
  tar -xzf ${STAGE_NAME}.tar.gz
  cd ${STAGE_NAME}
  bash install.sh doctor

安装（自动探测宝塔/1Panel/原生 + Apache/Nginx）：
  bash install.sh install
  bash install.sh install --engine nginx     # 同机双引擎时显式指定
  bash install.sh install --http-only        # 只接管 80
  bash install.sh install --cloudflare       # 站点在 Cloudflare 后面

出问题立刻恢复原状：
  bash install.sh rollback

访问控制台：
  ssh -L 8899:127.0.0.1:8899 root@<服务器IP>
  浏览器打开 http://127.0.0.1:8899
EOF
ok "已组装 ${STAGE_NAME}"

step '打包 tar.gz'
tar -czf "dist/${STAGE_NAME}.tar.gz" -C dist "$STAGE_NAME"
TAR_SIZE=$(du -h "dist/${STAGE_NAME}.tar.gz" | awk '{print $1}')
ok "${STAGE_NAME}.tar.gz  ${TAR_SIZE}"

HASH=$(sha256sum "dist/${STAGE_NAME}.tar.gz" | awk '{print $1}')
printf '%s  %s\n' "$HASH" "${STAGE_NAME}.tar.gz" > "dist/${STAGE_NAME}.tar.gz.sha256"
ok "SHA256 ${HASH}"

printf '\n================================================================\n'
printf ' 打包完成\n'
printf '================================================================\n'
printf ' 文件   dist/%s.tar.gz\n' "$STAGE_NAME"
printf ' 大小   %s\n' "$TAR_SIZE"
printf ' 规则   %s 条\n' "$RULE_COUNT"
printf '\n 上传后：\n'
printf '    tar -xzf %s.tar.gz && cd %s && bash install.sh install\n' "$STAGE_NAME" "$STAGE_NAME"
