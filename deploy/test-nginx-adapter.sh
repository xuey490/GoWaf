#!/usr/bin/env bash
#
# 宝塔 Nginx 适配器单测：不启服务、不需要 root。
# 覆盖实测会踩到的坑：server/{ 分行、http/{ 分行、$server_port !~ 443 无锚点。
#
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck disable=SC1091
source "${ROOT}/deploy/install.sh"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf '  OK  %s\n' "$*"; }

UPSTREAM_PORT=8080
TAKE_443=1
PANEL=bt
ENGINE=nginx

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

# ---------- 1. 宝塔分行 server { listen 80/443/IPv6 ----------
cat > "${WORKDIR}/bt-site.conf" <<'EOF'
server
{
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name demo.local www.demo.local;
    root /www/wwwroot/demo.local;
    #HTTP_TO_HTTPS_START
    if ($server_port !~ 443){
        rewrite ^(/.*)$ https://$host$1 permanent;
    }
    #HTTP_TO_HTTPS_END
    location / { try_files $uri $uri/ =404; }
}
EOF

nginx_retarget_file "${WORKDIR}/bt-site.conf" 1 || true
got=$(cat "${WORKDIR}/bt-site.conf")

echo "$got" | grep -q 'listen 127.0.0.1:8080;' \
  || fail "宝塔分行 server 的 listen 80 没有改写到 8080：$got"
echo "$got" | grep -qE '^[[:space:]]*listen[[:space:]]+80' \
  && fail "listen 80 还在：$got"
echo "$got" | grep -q '# \[GOWAF\].*listen \[::\]:80' \
  || fail "IPv6 listen 80 没有注释：$got"
echo "$got" | grep -q '# \[GOWAF\].*listen 443 ssl' \
  || fail "同块 443 listen 没有注释：$got"
echo "$got" | grep -q 'server_name demo.local' \
  || fail "server_name 被改坏了：$got"
pass "宝塔分行 server { listen 80/443/IPv6 改写"

# ---------- 2. 强制 HTTPS 死循环（宝塔无锚点模板） ----------
VHOST_DIR="$WORKDIR"
NGINX_CONF="${WORKDIR}/nginx.conf"
cat > "$NGINX_CONF" <<'EOF'
http
{
    include mime.types;
}
EOF
# 上一步的 bt-site.conf 还在 WORKDIR
nginx_fix_https_loop
grep -q 'http_x_forwarded_proto' "${WORKDIR}/bt-site.conf" \
  || fail "宝塔 if (\$server_port !~ 443) 没有改成 X-Forwarded-Proto"
grep -q 'server_port !~ 443' "${WORKDIR}/bt-site.conf" \
  && fail "旧的 \$server_port 判断还在"
pass "宝塔强制 HTTPS 跳转改写（无 ^\$ 锚点）"

# ---------- 3. 幂等 ----------
cp "${WORKDIR}/bt-site.conf" "${WORKDIR}/bt-site.conf.bak"
nginx_retarget_file "${WORKDIR}/bt-site.conf" 1 || true
cmp -s "${WORKDIR}/bt-site.conf" "${WORKDIR}/bt-site.conf.bak" \
  || fail "第二次改写不是幂等"
pass "listen 改写幂等"

# ---------- 4. 同行 server {（原生 nginx） ----------
cat > "${WORKDIR}/native.conf" <<'EOF'
server {
    listen 80 default_server;
    server_name shop.local;
}
EOF
nginx_retarget_file "${WORKDIR}/native.conf" 0 || true
grep -q 'listen 127.0.0.1:8080;' "${WORKDIR}/native.conf" \
  || fail "同行 server { 没有改写"
grep -q 'default_server' "${WORKDIR}/native.conf" \
  && fail "default_server 没有摘掉（多站点会冲突）"
pass "同行 server { + 摘除 default_server"

# ---------- 5. 仅 443 的 server 块（不能整段注释，否则默认绑 *:80） ----------
cat > "${WORKDIR}/ssl-only.conf" <<'EOF'
server {
    listen 443 ssl;
    server_name sslonly.local;
}
EOF
nginx_retarget_file "${WORKDIR}/ssl-only.conf" 1 || true
grep -q 'listen 127.0.0.1:8080;' "${WORKDIR}/ssl-only.conf" \
  || fail "仅 443 的块没有改成 8080 明文：$got"
grep -qE '^[[:space:]]*listen[[:space:]]+443' "${WORKDIR}/ssl-only.conf" \
  && fail "仅 443 的 listen 还在"
pass "仅 443 server 块改成 8080 明文"

# ---------- 6. 宝塔分行 http { 注入 realip ----------
NGINX_CONF="${WORKDIR}/nginx.conf"
nginx_inject_realip
grep -q 'GOWAF-MANAGED BEGIN' "$NGINX_CONF" || fail "realip 没有注入"
grep -q 'set_real_ip_from 127.0.0.1;' "$NGINX_CONF" || fail "缺少 set_real_ip_from"
awk '
  /http[[:space:]]*$/ { n=1 }
  n && /\{/ { found=1 }
  /GOWAF-MANAGED BEGIN/ { if (!found) exit 2; exit 0 }
  END { if (!found) exit 1 }
' "$NGINX_CONF" || fail "realip 没有插进 http { 块内"
# 幂等
nginx_inject_realip
cnt=$(grep -c 'GOWAF-MANAGED BEGIN' "$NGINX_CONF")
[[ $cnt -eq 1 ]] || fail "realip 注入不幂等，出现 $cnt 次"
pass "宝塔分行 http { 注入 realip 且幂等"

# ---------- 7. 同行 http { ----------
cat > "${WORKDIR}/nginx-same.conf" <<'EOF'
http {
    include mime.types;
}
EOF
NGINX_CONF="${WORKDIR}/nginx-same.conf"
nginx_inject_realip
grep -q 'set_real_ip_from 127.0.0.1;' "$NGINX_CONF" || fail "同行 http { 没有注入 realip"
pass "同行 http { 注入 realip"

printf '\n全部适配器单测通过\n'
