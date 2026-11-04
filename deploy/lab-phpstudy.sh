#!/usr/bin/env bash
#
# 在本机搭一套「小皮 Linux」目录布局实验室，供 install.sh 走 PANEL=phpstudy。
# 官方 xp.cn 安装包解压到 /usr/local/phpstudy（不是宝塔的 /www/server/panel）。
# 本镜像 PID 1 是 tini、不能装完整小皮面板，但路径、vhost、init.d、证书按官方习惯放置。
#
# 用法：
#   sudo bash deploy/lab-phpstudy.sh apache
#   sudo bash deploy/lab-phpstudy.sh nginx
#
set -Eeuo pipefail

[[ $EUID -eq 0 ]] || { echo "必须用 root 运行" >&2; exit 1; }

ENGINE=${1:-}
[[ $ENGINE == apache || $ENGINE == nginx ]] || {
  echo "用法：$0 apache|nginx" >&2
  exit 1
}

step() { printf '\n==> %s\n' "$*"; }
ok()   { printf '    ✓ %s\n' "$*"; }

XP=/usr/local/phpstudy
XP_NGINX="${XP}/soft/nginx/nginx-1.15"
XP_HTTPD="${XP}/soft/apache/httpd-2.4.39"
WWWROOT=/www/wwwroot
WWWLOGS=/www/wwwlogs
MOD=/usr/lib/apache2/modules
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

hide_conf() {
  local f=$1
  if [[ -f $f && ! -f ${f}.gowaf-lab-off ]]; then
    mv "$f" "${f}.gowaf-lab-off"
  fi
}

unhide_conf() {
  local f=$1
  if [[ -f ${f}.gowaf-lab-off && ! -f $f ]]; then
    mv "${f}.gowaf-lab-off" "$f"
  fi
}

stop_lab_web() {
  /etc/init.d/nginx stop >/dev/null 2>&1 || true
  /etc/init.d/httpd stop >/dev/null 2>&1 || true
  /etc/init.d/apache2 stop >/dev/null 2>&1 || true
  apache2ctl stop >/dev/null 2>&1 || true
  # 卸掉上一轮 GoWAF，避免占着 80/443
  if [[ -x /usr/local/bin/gowaf ]]; then
    /usr/local/bin/gowaf stop >/dev/null 2>&1 || true
  fi
  pkill -f '/www/server/gowaf/bin/gowaf' >/dev/null 2>&1 || true
  pkill -f '/www/server/nginx/conf/nginx.conf' >/dev/null 2>&1 || true
  pkill -f "${XP_NGINX}/conf/nginx.conf" >/dev/null 2>&1 || true
  pkill -x apache2 >/dev/null 2>&1 || true
  pkill -x nginx >/dev/null 2>&1 || true
  sleep 0.4
}

step "准备小皮 Linux ${ENGINE} 实验室（官方路径 ${XP}）"

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
if [[ $ENGINE == apache ]]; then
  apt-get install -y -qq apache2 iproute2 curl openssl ca-certificates >/dev/null
else
  apt-get install -y -qq nginx iproute2 curl openssl ca-certificates >/dev/null
fi

stop_lab_web
# 系统 apache2/nginx 让路
systemctl stop nginx >/dev/null 2>&1 || true
systemctl disable nginx >/dev/null 2>&1 || true
systemctl stop apache2 >/dev/null 2>&1 || true
systemctl disable apache2 >/dev/null 2>&1 || true

id www >/dev/null 2>&1 || useradd -r -s /usr/sbin/nologin www

install -d -m 755 \
  "${XP}/system" \
  "${XP}/vhost/nginx" \
  "${XP}/vhost/apache" \
  "${XP}/vhost/cert/demo.local" \
  "${XP_NGINX}"/{sbin,conf,conf.d,logs,html} \
  "${XP_HTTPD}"/{bin,conf,conf/extra,logs,run,modules} \
  "${WWWROOT}"/{demo.local,shop.local} \
  "${WWWLOGS}"

# 面板标记：install.sh 用 /usr/local/phpstudy/soft 或 phpstudyctl 判定 PANEL=phpstudy
cat > "${XP}/system/phpstudyctl" <<'EOF'
#!/bin/bash
echo "phpstudyctl lab stub: $*"
EOF
chmod 755 "${XP}/system/phpstudyctl"

# 去掉宝塔主程序标记，避免仍被判成 PANEL=bt
rm -f /www/server/panel/BT-Panel
rm -rf /www/server/panel/class
# 宝塔同构引擎配置必须藏起来，否则 phpstudy 探测会双引擎拒绝或改错文件
hide_conf /www/server/nginx/conf/nginx.conf
hide_conf /www/server/apache/conf/httpd.conf

# 签发演示证书
if [[ ! -f ${XP}/vhost/cert/demo.local/fullchain.pem ]]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout "${XP}/vhost/cert/demo.local/privkey.pem" \
    -out "${XP}/vhost/cert/demo.local/fullchain.pem" \
    -subj "/CN=demo.local" \
    -addext "subjectAltName=DNS:demo.local,DNS:www.demo.local" >/dev/null 2>&1
  chmod 600 "${XP}/vhost/cert/demo.local/privkey.pem"
fi
cp "${XP}/vhost/cert/demo.local/fullchain.pem" /usr/local/share/ca-certificates/demo.local.crt
update-ca-certificates >/dev/null 2>&1 || true
chown www:www "${XP}/vhost/cert/demo.local/privkey.pem" "${XP}/vhost/cert/demo.local/fullchain.pem" || true

printf '<html><body>demo-ok</body></html>\n' > "${WWWROOT}/demo.local/index.html"
printf '<html><body>shop-ok</body></html>\n' > "${WWWROOT}/shop.local/index.html"
grep -qE '[[:space:]]demo\.local' /etc/hosts || echo '127.0.0.1 demo.local www.demo.local shop.local' >> /etc/hosts

# systemctl 替身（本镜像无 systemd dbus）
install -m 755 "${HERE}/lab-systemctl.sh" /usr/local/sbin/systemctl
install -m 755 "${HERE}/lab-systemctl.sh" /usr/local/sbin/journalctl
mkdir -p /etc/systemd/system
hash -r 2>/dev/null || true

setup_nginx() {
  step '包装小皮 nginx 二进制（系统 nginx + 官方 prefix）'
  unhide_conf "${XP_NGINX}/conf/nginx.conf"
  hide_conf "${XP_HTTPD}/conf/httpd.conf"

  cat > "${XP_NGINX}/sbin/nginx" <<EOF
#!/usr/bin/env bash
exec /usr/sbin/nginx -p ${XP_NGINX} -c ${XP_NGINX}/conf/nginx.conf "\$@"
EOF
  chmod 755 "${XP_NGINX}/sbin/nginx"
  cp -a /etc/nginx/mime.types "${XP_NGINX}/conf/mime.types"

  # 官方解压包是库存 nginx.conf：http { 同行，不是宝塔分行
  cat > "${XP_NGINX}/conf/nginx.conf" <<EOF
user  www www;
worker_processes auto;
error_log  ${WWWLOGS}/nginx_error.log  crit;
pid        ${XP_NGINX}/logs/nginx.pid;
worker_rlimit_nofile 51200;

events {
    worker_connections 51200;
}

http {
    include       mime.types;
    default_type  application/octet-stream;
    sendfile      on;
    keepalive_timeout 65;
    server_tokens off;
    access_log off;

    server {
        listen 80;
        server_name _;
        index index.html;
        root ${XP_NGINX}/html;
    }

    include ${XP}/vhost/nginx/*.conf;
}
EOF
  printf '<html><body>xp-nginx-default</body></html>\n' > "${XP_NGINX}/html/index.html"

  # 小皮 Linux 站点常用 server { 同行 + 强制 HTTPS（无锚点）
  cat > "${XP}/vhost/nginx/demo.local.conf" <<EOF
server {
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name demo.local www.demo.local;
    index index.html index.htm;
    root ${WWWROOT}/demo.local;

    ssl_certificate    ${XP}/vhost/cert/demo.local/fullchain.pem;
    ssl_certificate_key    ${XP}/vhost/cert/demo.local/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    #HTTP_TO_HTTPS_START
    if (\$server_port !~ 443){
        rewrite ^(/.*)\$ https://\$host\$1 permanent;
    }
    #HTTP_TO_HTTPS_END

    location / {
        try_files \$uri \$uri/ =404;
    }

    access_log  ${WWWLOGS}/demo.local.log;
    error_log   ${WWWLOGS}/demo.local.error.log;
}
EOF

  # 小皮 Windows 风格的 _80.conf 命名，Linux 版也偶尔出现
  cat > "${XP}/vhost/nginx/shop.local_80.conf" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name shop.local;
    index index.html;
    root ${WWWROOT}/shop.local;
    location / {
        try_files \$uri \$uri/ =404;
    }
    access_log  ${WWWLOGS}/shop.local.log;
    error_log   ${WWWLOGS}/shop.local.error.log;
}
EOF

  chown -R www:www "$WWWROOT" "${XP_NGINX}/html" "$WWWLOGS" "${XP_NGINX}/logs"

  if [[ -f /etc/init.d/nginx && ! -f /etc/init.d/nginx.apt.bak ]]; then
    cp -a /etc/init.d/nginx /etc/init.d/nginx.apt.bak
  fi
  cat > /etc/init.d/nginx <<EOF
#!/bin/bash
### BEGIN INIT INFO
# Provides:          nginx
# Required-Start:    \$network
# Required-Stop:     \$network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: phpstudy-layout nginx (GoWAF lab)
### END INIT INFO
DAEMON=${XP_NGINX}/sbin/nginx
PID=${XP_NGINX}/logs/nginx.pid

start() {
  if [[ -f \$PID ]] && kill -0 "\$(cat "\$PID")" 2>/dev/null; then
    return 0
  fi
  "\$DAEMON"
}
stop() {
  if [[ -f \$PID ]] && kill -0 "\$(cat "\$PID")" 2>/dev/null; then
    "\$DAEMON" -s stop
    for _ in 1 2 3 4 5; do
      [[ -f \$PID ]] || return 0
      sleep 0.2
    done
    kill "\$(cat "\$PID")" 2>/dev/null || true
  fi
  pkill -f '${XP_NGINX}/conf/nginx.conf' 2>/dev/null || true
  return 0
}
reload() {
  [[ -f \$PID ]] && "\$DAEMON" -s reload
}
status() {
  [[ -f \$PID ]] && kill -0 "\$(cat "\$PID")" 2>/dev/null
}

case "\$1" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  reload) reload ;;
  status) status ;;
  *) echo "Usage: \$0 {start|stop|restart|reload|status}" >&2; exit 1 ;;
esac
EOF
  chmod 755 /etc/init.d/nginx

  "${XP_NGINX}/sbin/nginx" -t
  /etc/init.d/nginx start
}

setup_apache() {
  step '包装小皮 apachectl（系统 apache2 + 官方 ServerRoot）'
  [[ -d $MOD ]] || { echo "找不到 ${MOD}，请先 apt-get install apache2" >&2; exit 1; }
  unhide_conf "${XP_HTTPD}/conf/httpd.conf"
  hide_conf "${XP_NGINX}/conf/nginx.conf"

  load() {
    local name=$1 file=$2
    # Debian/Ubuntu 把 unixd、log_config 编进 apache2，没有对应 .so
    if [[ ! -f ${MOD}/${file} ]]; then
      printf '    skip %s (compiled-in or absent)\n' "$file" >&2
      return 0
    fi
    printf 'LoadModule %s %s/%s\n' "$name" "$MOD" "$file"
  }

  {
    cat <<EOF
ServerRoot "${XP_HTTPD}"
DefaultRuntimeDir ${XP_HTTPD}/run
PidFile logs/httpd.pid
Mutex file:${XP_HTTPD}/logs default
ServerName phpstudy.local
User www
Group www
Timeout 60
KeepAlive On
MaxKeepAliveRequests 100
KeepAliveTimeout 5

Listen 80
Listen 443

EOF
    load mpm_event_module mod_mpm_event.so
    load authz_core_module mod_authz_core.so
    load authz_host_module mod_authz_host.so
    load unixd_module mod_unixd.so
    load mime_module mod_mime.so
    load log_config_module mod_log_config.so
    load dir_module mod_dir.so
    load alias_module mod_alias.so
    load rewrite_module mod_rewrite.so
    load headers_module mod_headers.so
    load setenvif_module mod_setenvif.so
    load env_module mod_env.so
    load socache_shmcb_module mod_socache_shmcb.so
    load ssl_module mod_ssl.so
    load remoteip_module mod_remoteip.so
    cat <<EOF

TypesConfig /etc/mime.types
DirectoryIndex index.html index.htm

<Directory />
    AllowOverride None
    Require all denied
</Directory>
<Directory ${WWWROOT}>
    Options FollowSymLinks
    AllowOverride All
    Require all granted
</Directory>

ErrorLog ${WWWLOGS}/httpd_error.log
LogLevel warn
LogFormat "%h %l %u %t \\"%r\\" %>s %b" combined
CustomLog ${WWWLOGS}/httpd_access.log combined

SSLRandomSeed startup builtin
SSLRandomSeed connect builtin
SSLSessionCache shmcb:${XP_HTTPD}/logs/ssl_scache(512000)
SSLSessionCacheTimeout 300

Include ${XP}/vhost/apache/*.conf
EOF
  } > "${XP_HTTPD}/conf/httpd.conf"

  cat > "${XP}/vhost/apache/demo.local.conf" <<EOF
<VirtualHost *:80>
    ServerName demo.local
    ServerAlias www.demo.local
    DocumentRoot ${WWWROOT}/demo.local
    ErrorLog ${WWWLOGS}/demo.local-error_log
    CustomLog ${WWWLOGS}/demo.local-access_log combined

    #HTTP_TO_HTTPS_START
    RewriteEngine On
    RewriteCond %{SERVER_PORT} !^443$
    RewriteRule ^(.*)\$ https://%{SERVER_NAME}\$1 [L,R=301]
    #HTTP_TO_HTTPS_END
</VirtualHost>

<VirtualHost *:443>
    ServerName demo.local
    ServerAlias www.demo.local
    DocumentRoot ${WWWROOT}/demo.local
    SSLEngine on
    SSLCertificateFile ${XP}/vhost/cert/demo.local/fullchain.pem
    SSLCertificateKeyFile ${XP}/vhost/cert/demo.local/privkey.pem
    ErrorLog ${WWWLOGS}/demo.local-ssl-error_log
    CustomLog ${WWWLOGS}/demo.local-ssl-access_log combined
</VirtualHost>
EOF

  cat > "${XP}/vhost/apache/shop.local.conf" <<EOF
<VirtualHost *:80>
    ServerName shop.local
    DocumentRoot ${WWWROOT}/shop.local
    ErrorLog ${WWWLOGS}/shop.local-error_log
    CustomLog ${WWWLOGS}/shop.local-access_log combined
</VirtualHost>
EOF

  cat > "${XP_HTTPD}/bin/apachectl" <<EOF
#!/usr/bin/env bash
ROOT=${XP_HTTPD}
CONF=\${ROOT}/conf/httpd.conf
BIN=/usr/sbin/apache2
case \${1:-} in
  configtest)
    exec "\$BIN" -d "\$ROOT" -f "\$CONF" -t
    ;;
  start)
    exec "\$BIN" -d "\$ROOT" -f "\$CONF" -k start
    ;;
  stop)
    exec "\$BIN" -d "\$ROOT" -f "\$CONF" -k stop
    ;;
  restart)
    exec "\$BIN" -d "\$ROOT" -f "\$CONF" -k restart
    ;;
  reload|graceful)
    exec "\$BIN" -d "\$ROOT" -f "\$CONF" -k graceful
    ;;
  *)
    exec "\$BIN" -d "\$ROOT" -f "\$CONF" "\$@"
    ;;
esac
EOF
  chmod 755 "${XP_HTTPD}/bin/apachectl"

  chown -R www:www "$WWWROOT" "$WWWLOGS" "${XP_HTTPD}/logs" "${XP_HTTPD}/run"

  cat > /etc/init.d/httpd <<EOF
#!/bin/bash
### BEGIN INIT INFO
# Provides:          httpd
# Required-Start:    \$network
# Required-Stop:     \$network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: phpstudy-layout httpd (GoWAF lab)
### END INIT INFO
CTL=${XP_HTTPD}/bin/apachectl
PID=${XP_HTTPD}/logs/httpd.pid

start() {
  if [[ -f \$PID ]] && kill -0 "\$(cat "\$PID")" 2>/dev/null; then
    return 0
  fi
  "\$CTL" start
}
stop() {
  if [[ -f \$PID ]] && kill -0 "\$(cat "\$PID")" 2>/dev/null; then
    "\$CTL" stop || true
    for _ in 1 2 3 4 5 6 7 8; do
      kill -0 "\$(cat "\$PID" 2>/dev/null)" 2>/dev/null || break
      sleep 0.2
    done
  fi
  pkill -f '${XP_HTTPD}/conf/httpd.conf' 2>/dev/null || true
  return 0
}
reload() {
  [[ -f \$PID ]] && "\$CTL" graceful
}
status() {
  [[ -f \$PID ]] && kill -0 "\$(cat "\$PID")" 2>/dev/null
}

case "\$1" in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  reload) reload ;;
  status) status ;;
  *) echo "Usage: \$0 {start|stop|restart|reload|status}" >&2; exit 1 ;;
esac
EOF
  chmod 755 /etc/init.d/httpd

  "${XP_HTTPD}/bin/apachectl" configtest
  /etc/init.d/httpd start
}

if [[ $ENGINE == apache ]]; then
  setup_apache
else
  setup_nginx
fi

sleep 0.6
ss -lntp | grep -E ':80|:443' || true
curl -sS -o /tmp/demo.body -w 'demo.local HTTP %{http_code} redirect=%{redirect_url}\n' -H 'Host: demo.local' http://127.0.0.1/ || true
curl -sk -o /dev/null -w 'demo.local HTTPS %{http_code}\n' --resolve demo.local:443:127.0.0.1 https://demo.local/ || true
curl -sS -o /dev/null -w 'shop.local HTTP %{http_code}\n' -H 'Host: shop.local' http://127.0.0.1/ || true
ok "小皮 Linux ${ENGINE} 实验室已就绪（PANEL=phpstudy ENGINE=${ENGINE}）"
printf '\n下一步：\n  bash install.sh doctor\n  bash install.sh install --force\n'
