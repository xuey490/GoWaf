#!/usr/bin/env bash
#
# 搭一套「无面板」原生 Debian/Ubuntu Apache 或 Nginx 实验室，
# 供 install.sh 走 PANEL=system。会藏起宝塔/小皮标记，避免误判。
#
# 用法：
#   sudo bash deploy/lab-system.sh apache
#   sudo bash deploy/lab-system.sh nginx
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

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
WWW=/var/www
CERT=/etc/gowaf/certs

hide_path() {
  local p=$1
  if [[ -e $p && ! -e ${p}.gowaf-lab-off ]]; then
    mv "$p" "${p}.gowaf-lab-off"
  fi
}

stop_lab_web() {
  /etc/init.d/nginx stop >/dev/null 2>&1 || true
  /etc/init.d/httpd stop >/dev/null 2>&1 || true
  /etc/init.d/apache2 stop >/dev/null 2>&1 || true
  apache2ctl stop >/dev/null 2>&1 || true
  if [[ -x /usr/local/bin/gowaf ]]; then
    /usr/local/bin/gowaf stop >/dev/null 2>&1 || true
  fi
  pkill -f '/www/server/gowaf/bin/gowaf' >/dev/null 2>&1 || true
  pkill -x nginx >/dev/null 2>&1 || true
  pkill -x apache2 >/dev/null 2>&1 || true
  sleep 0.4
}

step "准备原生 Linux ${ENGINE} 实验室（PANEL=system）"

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
if [[ $ENGINE == apache ]]; then
  apt-get install -y -qq apache2 iproute2 curl openssl ca-certificates >/dev/null
else
  apt-get install -y -qq nginx iproute2 curl openssl ca-certificates >/dev/null
fi

stop_lab_web

# 藏起面板标记，否则 detect_panel 会走宝塔/小皮
hide_path /usr/local/phpstudy
hide_path /www/server/panel
hide_path /www/server/nginx/sbin/nginx
hide_path /www/server/nginx/conf/nginx.conf
hide_path /www/server/apache/conf/httpd.conf
# 小皮留下的 httpd SysV 脚本会抢走原生 apache2
if [[ -x /etc/init.d/httpd ]]; then
  hide_path /etc/init.d/httpd
fi
# 还原 Debian 的 nginx init.d（实验室曾被宝塔/小皮包装器覆盖）
if [[ -f /etc/init.d/nginx.apt.bak ]]; then
  cp -a /etc/init.d/nginx.apt.bak /etc/init.d/nginx
  chmod 755 /etc/init.d/nginx
fi

install -d -m 755 \
  "${WWW}/demo.local" "${WWW}/shop.local" \
  "${CERT}/demo.local" \
  /var/log/apache2 /var/log/nginx \
  /run/lock/apache2 /run/apache2
# Debian apache2 的 lock 目录：/var/lock 常常是指向 /run/lock 的符号链接
mkdir -p /run/lock/apache2 /var/lock/apache2 2>/dev/null || true

if [[ ! -f ${CERT}/demo.local/fullchain.pem ]]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout "${CERT}/demo.local/privkey.pem" \
    -out "${CERT}/demo.local/fullchain.pem" \
    -subj "/CN=demo.local" \
    -addext "subjectAltName=DNS:demo.local,DNS:www.demo.local" >/dev/null 2>&1
  chmod 640 "${CERT}/demo.local/privkey.pem"
  chown root:www-data "${CERT}/demo.local/privkey.pem" 2>/dev/null \
    || chown root:nginx "${CERT}/demo.local/privkey.pem" 2>/dev/null \
    || true
fi
cp "${CERT}/demo.local/fullchain.pem" /usr/local/share/ca-certificates/demo.local.crt
update-ca-certificates >/dev/null 2>&1 || true

printf '<html><body>demo-ok</body></html>\n' > "${WWW}/demo.local/index.html"
printf '<html><body>shop-ok</body></html>\n' > "${WWW}/shop.local/index.html"
grep -qE '[[:space:]]demo\.local' /etc/hosts || echo '127.0.0.1 demo.local www.demo.local shop.local' >> /etc/hosts

install -m 755 "${HERE}/lab-systemctl.sh" /usr/local/sbin/systemctl
install -m 755 "${HERE}/lab-systemctl.sh" /usr/local/sbin/journalctl
hash -r 2>/dev/null || true

setup_apache() {
  step '写 Debian 原生 Apache 站点（sites-available + a2ensite）'
  a2enmod ssl rewrite headers setenvif remoteip >/dev/null 2>&1 || true
  a2dissite 000-default >/dev/null 2>&1 || true
  a2dissite default-ssl >/dev/null 2>&1 || true

  cat > /etc/apache2/sites-available/demo.local.conf <<EOF
<VirtualHost *:80>
    ServerName demo.local
    ServerAlias www.demo.local
    DocumentRoot ${WWW}/demo.local
    ErrorLog \${APACHE_LOG_DIR}/demo.local-error.log
    CustomLog \${APACHE_LOG_DIR}/demo.local-access.log combined

    #HTTP_TO_HTTPS_START
    RewriteEngine On
    RewriteCond %{SERVER_PORT} !^443$
    RewriteRule ^(.*)\$ https://%{SERVER_NAME}\$1 [L,R=301]
    #HTTP_TO_HTTPS_END
</VirtualHost>

<VirtualHost *:443>
    ServerName demo.local
    ServerAlias www.demo.local
    DocumentRoot ${WWW}/demo.local
    SSLEngine on
    SSLCertificateFile ${CERT}/demo.local/fullchain.pem
    SSLCertificateKeyFile ${CERT}/demo.local/privkey.pem
    ErrorLog \${APACHE_LOG_DIR}/demo.local-ssl-error.log
    CustomLog \${APACHE_LOG_DIR}/demo.local-ssl-access.log combined
</VirtualHost>
EOF

  cat > /etc/apache2/sites-available/shop.local.conf <<EOF
<VirtualHost *:80>
    ServerName shop.local
    DocumentRoot ${WWW}/shop.local
    ErrorLog \${APACHE_LOG_DIR}/shop.local-error.log
    CustomLog \${APACHE_LOG_DIR}/shop.local-access.log combined
</VirtualHost>
EOF

  a2ensite demo.local >/dev/null
  a2ensite shop.local >/dev/null
  chown -R www-data:www-data "${WWW}/demo.local" "${WWW}/shop.local"
  apache2ctl configtest
  /etc/init.d/apache2 start
}

setup_nginx() {
  step '写 Debian 原生 Nginx 站点（保留无后缀 default + *.conf）'
  # 故意留下 sites-enabled/default（无 .conf），验证 install.sh 会改到它
  cat > /etc/nginx/sites-available/demo.local.conf <<EOF
server {
    listen 80;
    listen [::]:80;
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name demo.local www.demo.local;
    root ${WWW}/demo.local;
    index index.html;

    ssl_certificate     ${CERT}/demo.local/fullchain.pem;
    ssl_certificate_key ${CERT}/demo.local/privkey.pem;

    #HTTP_TO_HTTPS_START
    if (\$server_port !~ 443){
        rewrite ^(/.*)\$ https://\$host\$1 permanent;
    }
    #HTTP_TO_HTTPS_END

    location / { try_files \$uri \$uri/ =404; }
}
EOF
  cat > /etc/nginx/sites-available/shop.local.conf <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name shop.local;
    root ${WWW}/shop.local;
    index index.html;
    location / { try_files \$uri \$uri/ =404; }
}
EOF
  ln -sfn /etc/nginx/sites-available/demo.local.conf /etc/nginx/sites-enabled/demo.local.conf
  ln -sfn /etc/nginx/sites-available/shop.local.conf /etc/nginx/sites-enabled/shop.local.conf
  # default 仍是无后缀符号链接，保持 Debian 包装原样
  nginx -t
  /etc/init.d/nginx start
}

if [[ $ENGINE == apache ]]; then
  setup_apache
else
  setup_nginx
fi

sleep 0.6
ss -lntp | grep -E ':80|:443' || true
curl -sS -o /dev/null -w 'demo.local HTTP %{http_code} redirect=%{redirect_url}\n' -H 'Host: demo.local' http://127.0.0.1/ || true
curl -sk -o /dev/null -w 'demo.local HTTPS %{http_code}\n' --resolve demo.local:443:127.0.0.1 https://demo.local/ || true
curl -sS -o /dev/null -w 'shop.local HTTP %{http_code}\n' -H 'Host: shop.local' http://127.0.0.1/ || true
ok "原生 Linux ${ENGINE} 实验室已就绪（PANEL=system ENGINE=${ENGINE}）"
printf '\n下一步：\n  bash install.sh doctor\n  bash install.sh install --force\n'
