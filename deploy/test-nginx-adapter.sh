#!/usr/bin/env bash
#
# 面板适配器单测：不启服务、不需要 root。
# 覆盖实测会踩到的坑：宝塔 Nginx server/{ 分行、http/{ 分行、$server_port !~ 443 无锚点；
# 小皮 Linux 路径 glob；Apache VirtualHost 改写与强制 HTTPS。
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

# ---------- 8. 小皮 Linux 版本目录 glob 取最新 ----------
mkdir -p "${WORKDIR}/soft/nginx/nginx-1.15/conf" "${WORKDIR}/soft/nginx/nginx-1.18/conf"
touch "${WORKDIR}/soft/nginx/nginx-1.15/conf/nginx.conf" "${WORKDIR}/soft/nginx/nginx-1.18/conf/nginx.conf"
got=$(xp_latest_file "${WORKDIR}/soft/nginx/*/conf/nginx.conf")
[[ $got == *nginx-1.18* ]] || fail "xp_latest_file 没有取到较新版本：$got"
if xp_latest_file "${WORKDIR}/soft/missing/*/conf/nginx.conf" >/dev/null 2>&1; then
  fail "不存在的 glob 应该返回失败"
fi
pass "小皮 Linux 版本目录 glob 取最新"

# ---------- 9. Apache VirtualHost *:80 + 强制 HTTPS ----------
PANEL=phpstudy
ENGINE=apache
VHOST_DIR="$WORKDIR"
cat > "${WORKDIR}/demo.apache.conf" <<'EOF'
<VirtualHost *:80>
    ServerName demo.local
    DocumentRoot /www/wwwroot/demo.local
    #HTTP_TO_HTTPS_START
    RewriteEngine On
    RewriteCond %{SERVER_PORT} !^443$
    RewriteRule ^(.*)$ https://%{SERVER_NAME}$1 [L,R=301]
    #HTTP_TO_HTTPS_END
</VirtualHost>
<VirtualHost *:443>
    ServerName demo.local
    SSLEngine on
</VirtualHost>
EOF
retarget_vhosts
grep -q '<VirtualHost 127.0.0.1:8080>' "${WORKDIR}/demo.apache.conf" \
  || fail "Apache *:80 没有改写到 127.0.0.1:8080"
grep -qE '^[[:space:]]*<VirtualHost[[:space:]]+\*:80>' "${WORKDIR}/demo.apache.conf" \
  && fail "Apache *:80 还在"
grep -qF 'RewriteCond %{HTTP:X-Forwarded-Proto} !^https$' "${WORKDIR}/demo.apache.conf" \
  || fail "Apache 强制 HTTPS 没有补 X-Forwarded-Proto"
grep -qF '<VirtualHost *:443>' "${WORKDIR}/demo.apache.conf" \
  || fail "未接管前 *:443 段不该动"
# 幂等
cp "${WORKDIR}/demo.apache.conf" "${WORKDIR}/demo.apache.conf.bak"
retarget_vhosts
cmp -s "${WORKDIR}/demo.apache.conf" "${WORKDIR}/demo.apache.conf.bak" \
  || fail "Apache retarget 第二次不是幂等"
pass "Apache VirtualHost *:80 改写 + 强制 HTTPS 幂等"

# ---------- 10. Debian sites-enabled/default 无后缀 + 符号链接备份 ----------
SITE="${WORKDIR}/sites-enabled"
AVAIL="${WORKDIR}/sites-available"
mkdir -p "$SITE" "$AVAIL"
cat > "${AVAIL}/default" <<'EOF'
server {
    listen 80 default_server;
    server_name _;
}
EOF
ln -s "${AVAIL}/default" "${SITE}/default"
cat > "${AVAIL}/shop.local.conf" <<'EOF'
server {
    listen 80;
    server_name shop.local;
}
EOF
ln -s "${AVAIL}/shop.local.conf" "${SITE}/shop.local.conf"
VHOST_DIR="$SITE"
got=$(iter_vhost_files)
echo "$got" | grep -Fxq "${AVAIL}/default" || fail "iter_vhost_files 没有跟到 sites-available/default：$got"
echo "$got" | grep -q 'shop.local.conf' || fail "iter_vhost_files 漏掉 .conf：$got"
BK_DIR="${WORKDIR}/bk"
backup_vhost_files
[[ -L ${BK_DIR}/vhost/default ]] && fail "备份还是符号链接，没有跟内容"
grep -q 'listen 80 default_server' "${BK_DIR}/vhost/default" \
  || fail "备份没有 default 原文"
nginx_retarget_file "${SITE}/default" 0 || true
grep -q 'listen 127.0.0.1:8080' "${AVAIL}/default" \
  || fail "改写符号链接没有写到 sites-available"
restore_vhost_files "$BK_DIR"
grep -q 'listen 80 default_server' "${AVAIL}/default" \
  || fail "restore 没有把 default 写回 sites-available"
pass "Debian sites-enabled/default 无后缀 + 符号链接备份还原"

# ---------- 11. 发布包同目录的 gowaf-linux-arm64 能被找到 ----------
ARMDIR="${WORKDIR}/arm-pack"
mkdir -p "$ARMDIR"
cp "${ROOT}/deploy/install.sh" "${ARMDIR}/install.sh"
: > "${ARMDIR}/gowaf-linux-arm64"
got=$(bash -c '
  source "$1"
  BINARY_SRC=
  find_binary
  printf "%s\n" "$BINARY_SRC"
' _ "${ARMDIR}/install.sh")
[[ $got == "${ARMDIR}/gowaf-linux-arm64" ]] || fail "arm64 发布包没找到同目录二进制：$got"
pass "同目录 gowaf-linux-arm64 可被 find_binary 找到"

# 本机架构优先：同目录同时有 amd64 和 arm64 时拿 uname 对应的那份
: > "${ARMDIR}/gowaf-linux-amd64"
got=$(bash -c '
  source "$1"
  BINARY_SRC=
  find_binary
  printf "%s\n" "$BINARY_SRC"
' _ "${ARMDIR}/install.sh")
want_arch=$(uname -m)
case $want_arch in
  x86_64|amd64)  expect="${ARMDIR}/gowaf-linux-amd64" ;;
  aarch64|arm64) expect="${ARMDIR}/gowaf-linux-arm64" ;;
  *) expect="" ;;
esac
if [[ -n $expect ]]; then
  [[ $got == "$expect" ]] || fail "双二进制时没有优先本机架构：got=$got expect=$expect"
  pass "同目录双架构时优先本机 $(basename "$expect")"
fi

printf '\n全部适配器单测通过\n'
