#!/usr/bin/env bash
#
# 在本机搭一套「宝塔 Nginx 目录布局」实验室，供 install.sh 走 PANEL=bt + ENGINE=nginx。
# 不安装完整宝塔面板（需要绑定账号、体积大），但路径、主配置语法、vhost 模板、
# init.d 控制、证书目录都按宝塔习惯放置，install.sh 探测结果与真机一致。
#
set -Eeuo pipefail

[[ $EUID -eq 0 ]] || { echo "必须用 root 运行" >&2; exit 1; }

step() { printf '\n==> %s\n' "$*"; }
ok()   { printf '    ✓ %s\n' "$*"; }

PREFIX=/www/server/nginx
PANEL=/www/server/panel
WWWROOT=/www/wwwroot
WWWLOGS=/www/wwwlogs

step '安装依赖（nginx / iproute2 / curl / openssl）'
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq nginx iproute2 curl openssl ca-certificates >/dev/null
# 系统 nginx 让路，后面用宝塔路径上的包装器
systemctl stop nginx >/dev/null 2>&1 || true
systemctl disable nginx >/dev/null 2>&1 || true
ok "nginx $(nginx -v 2>&1 | awk '{print $NF}')"

step '创建 www 用户与宝塔目录'
id www >/dev/null 2>&1 || useradd -r -s /usr/sbin/nologin www
install -d -m 755 \
  "$PREFIX"/{sbin,conf,conf.d,logs,html} \
  "$PANEL"/{class,vhost/nginx,vhost/cert/demo.local,vhost/rewrite} \
  "$WWWROOT"/{demo.local,shop.local} \
  "$WWWLOGS"
# 面板主程序标记：install.sh 用 BT-Panel 或 class/ 判定 PANEL=bt
: > "${PANEL}/BT-Panel"
ok "PANEL=${PANEL} PREFIX=${PREFIX}"

step '包装 nginx 二进制（系统 nginx + 宝塔 prefix）'
cat > "${PREFIX}/sbin/nginx" <<'EOF'
#!/usr/bin/env bash
exec /usr/sbin/nginx -p /www/server/nginx -c /www/server/nginx/conf/nginx.conf "$@"
EOF
chmod 755 "${PREFIX}/sbin/nginx"
cp -a /etc/nginx/mime.types "${PREFIX}/conf/mime.types"
ok "sbin/nginx 包装器"

step '写宝塔风格主配置（http 与 { 分行，带默认 server）'
cat > "${PREFIX}/conf/nginx.conf" <<'EOF'
user  www www;
worker_processes auto;
error_log  /www/wwwlogs/nginx_error.log  crit;
pid        /www/server/nginx/logs/nginx.pid;
worker_rlimit_nofile 51200;

events
    {
        use epoll;
        worker_connections 51200;
    }

http
    {
        include       mime.types;
        default_type  application/octet-stream;
        server_names_hash_bucket_size 512;
        client_header_buffer_size 32k;
        large_client_header_buffers 4 32k;
        client_max_body_size 50m;
        sendfile   on;
        tcp_nopush on;
        keepalive_timeout 60;
        gzip on;
        server_tokens off;
        access_log off;

        server
        {
            listen 80;
            listen [::]:80;
            server_name _;
            index index.html;
            root /www/server/nginx/html;
        }

        include /www/server/panel/vhost/nginx/*.conf;
    }
EOF
printf '<html><body>bt-nginx-default</body></html>\n' > "${PREFIX}/html/index.html"

step '签发演示证书（demo.local / www.demo.local）'
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout "${PANEL}/vhost/cert/demo.local/privkey.pem" \
  -out "${PANEL}/vhost/cert/demo.local/fullchain.pem" \
  -subj "/CN=demo.local" \
  -addext "subjectAltName=DNS:demo.local,DNS:www.demo.local" >/dev/null 2>&1
chmod 600 "${PANEL}/vhost/cert/demo.local/privkey.pem"
# 把演示证书加入本机信任，install.sh 的 HTTPS 自检故意不加 -k
cp "${PANEL}/vhost/cert/demo.local/fullchain.pem" /usr/local/share/ca-certificates/demo.local.crt
update-ca-certificates >/dev/null 2>&1 || true
ok "证书 ${PANEL}/vhost/cert/demo.local"

step '写宝塔风格站点（server 与 { 分行 + 强制 HTTPS）'
cat > "${PANEL}/vhost/nginx/demo.local.conf" <<'EOF'
server
{
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name demo.local www.demo.local;
    index index.html index.htm default.html;
    root /www/wwwroot/demo.local;

    ssl_certificate    /www/server/panel/vhost/cert/demo.local/fullchain.pem;
    ssl_certificate_key    /www/server/panel/vhost/cert/demo.local/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    #HTTP_TO_HTTPS_START
    if ($server_port !~ 443){
        rewrite ^(/.*)$ https://$host$1 permanent;
    }
    #HTTP_TO_HTTPS_END

    location / {
        try_files $uri $uri/ =404;
    }

    access_log  /www/wwwlogs/demo.local.log;
    error_log   /www/wwwlogs/demo.local.error.log;
}
EOF

cat > "${PANEL}/vhost/nginx/shop.local.conf" <<'EOF'
server
{
    listen 80;
    listen [::]:80;
    server_name shop.local;
    index index.html;
    root /www/wwwroot/shop.local;
    location / {
        try_files $uri $uri/ =404;
    }
    access_log  /www/wwwlogs/shop.local.log;
    error_log   /www/wwwlogs/shop.local.error.log;
}
EOF

printf '<html><body>demo-ok</body></html>\n' > "${WWWROOT}/demo.local/index.html"
printf '<html><body>shop-ok</body></html>\n' > "${WWWROOT}/shop.local/index.html"
chown -R www:www "$WWWROOT" "$PREFIX/html" "$WWWLOGS" "$PREFIX/logs"

grep -qE '[[:space:]]demo\.local' /etc/hosts || echo '127.0.0.1 demo.local www.demo.local shop.local' >> /etc/hosts
ok "站点 demo.local / shop.local"

step '安装宝塔风格 /etc/init.d/nginx'
if [[ -f /etc/init.d/nginx && ! -f /etc/init.d/nginx.apt.bak ]]; then
  cp -a /etc/init.d/nginx /etc/init.d/nginx.apt.bak
fi
cat > /etc/init.d/nginx <<'EOF'
#!/bin/bash
### BEGIN INIT INFO
# Provides:          nginx
# Required-Start:    $network
# Required-Stop:     $network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: BT-layout nginx (GoWAF lab)
### END INIT INFO
DAEMON=/www/server/nginx/sbin/nginx
PID=/www/server/nginx/logs/nginx.pid

start() {
  if [[ -f $PID ]] && kill -0 "$(cat "$PID")" 2>/dev/null; then
    return 0
  fi
  "$DAEMON"
}
stop() {
  if [[ -f $PID ]] && kill -0 "$(cat "$PID")" 2>/dev/null; then
    "$DAEMON" -s stop
    for _ in 1 2 3 4 5; do
      [[ -f $PID ]] || return 0
      sleep 0.2
    done
    kill "$(cat "$PID")" 2>/dev/null || true
  fi
  # 兜底：按配置文件杀掉还在监听的 master
  pkill -f '/www/server/nginx/conf/nginx.conf' 2>/dev/null || true
  return 0
}
reload() {
  [[ -f $PID ]] && "$DAEMON" -s reload
}
status() {
  [[ -f $PID ]] && kill -0 "$(cat "$PID")" 2>/dev/null
}

case "$1" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  reload) reload ;;
  status) status ;;
  *) echo "Usage: $0 {start|stop|restart|reload|status}" >&2; exit 1 ;;
esac
EOF
chmod 755 /etc/init.d/nginx

cat > /etc/systemd/system/nginx.service <<'EOF'
[Unit]
Description=BT-layout nginx (GoWAF lab)
After=network-online.target
Wants=network-online.target

[Service]
Type=forking
PIDFile=/www/server/nginx/logs/nginx.pid
ExecStart=/etc/init.d/nginx start
ExecReload=/etc/init.d/nginx reload
ExecStop=/etc/init.d/nginx stop
KillMode=mixed

[Install]
WantedBy=multi-user.target
EOF
# 本镜像 PID 1 是 tini，没有 systemd dbus。装一个 systemctl 替身，
# 让 install.sh 的 gowaf.service / timer 路径能跑通。
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
install -m 755 "${HERE}/lab-systemctl.sh" /usr/local/sbin/systemctl
install -m 755 "${HERE}/lab-systemctl.sh" /usr/local/sbin/journalctl
mkdir -p /etc/systemd/system
# 真 systemd 若存在也不要抢 PATH：sudo 的 secure_path 里 /usr/local/sbin 在最前
hash -r 2>/dev/null || true
systemctl daemon-reload >/dev/null 2>&1 || true
systemctl enable nginx >/dev/null 2>&1 || true

step '语法检查并启动'
"${PREFIX}/sbin/nginx" -t
/etc/init.d/nginx start
sleep 0.5
ss -lntp | grep -E ':80|:443' || true
curl -sS -o /tmp/demo.body -w 'demo.local HTTP %{http_code} redirect=%{redirect_url}\n' -H 'Host: demo.local' http://127.0.0.1/ || true
curl -sk -o /dev/null -w 'demo.local HTTPS %{http_code}\n' --resolve demo.local:443:127.0.0.1 https://demo.local/ || true
curl -sS -o /dev/null -w 'shop.local HTTP %{http_code}\n' -H 'Host: shop.local' http://127.0.0.1/ || true
ok "宝塔 Nginx 实验室已就绪（PANEL=bt ENGINE=nginx）"
printf '\n下一步：\n  bash install.sh doctor\n  bash install.sh install --force\n'
