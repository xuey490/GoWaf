#!/usr/bin/env bash
#
# GoWAF 一键部署 / 运维脚本（Ubuntu + 宝塔 Apache）
#
# 核心事实：GoWAF 是「反向代理型」WAF，它必须站在 Web 服务器前面。
# 所以部署动作只有一件事——把 80/443 交给 gowaf，让 Apache 退到 127.0.0.1:8080。
# 其余都是围绕这件事的安全措施：备份、configtest、失败自动回滚、端口漂移自愈。
#
# 用法：
#   ./install.sh install              # 一键部署（默认 observe 模式，只记录不拦截）
#   ./install.sh start|stop|restart   # 服务控制
#   ./install.sh status               # 状态 + 端口 + 最近日志
#   ./install.sh observe              # 切观察模式（不拦截）
#   ./install.sh protect              # 切实时拦截模式
#   ./install.sh resync               # 修复被宝塔面板改回的 Apache 端口
#   ./install.sh cloudflare on|off|update|status   # Cloudflare 回源 IP 识别
#   ./install.sh console open [--ip 1.2.3.4|all] [--hours 2]   # 临时对外暴露控制台
#   ./install.sh console close|status              # 立刻收回 / 查看当前状态
#   ./install.sh rollback             # 还原 Apache 配置并停用 gowaf
#   ./install.sh uninstall            # 彻底卸载（含配置与数据，会二次确认）
#   ./install.sh doctor               # 只体检，不改任何东西
#
# 参数：
#   --binary PATH   指定 gowaf 二进制（默认找脚本同目录的 gowaf-linux-amd64 / bin/gowaf）
#   --rules PATH    指定规则文件（不指定则保留服务器上已有的，避免覆盖你的调优）
#   --upstream-port N   Apache 回退端口，默认 8080
#   --http-only     只接管 80，443 完全不动（最保守的上线方式）
#   --cloudflare    站点挂在 Cloudflare 小云朵后面：把 CF 回源网段写入 trusted_proxies，
#                   使限流/封禁/日志按真实访客 IP 计算（否则会按 CF 节点 IP，一封一大片）
#   --force         跳过交互确认
#   --ip A.B.C.D    仅 console open 用：允许访问控制台的来源（可逗号分隔，all = 全网）
#                   不填则自动取当前 SSH 客户端 IP，只给「你这一台」开门
#   --hours N       仅 console open 用：N 小时后自动收回（默认 2）
#   --quiet         resync 用，只在真的修了东西时输出
#
set -Eeuo pipefail

# ---------------------------------------------------------------- 常量

GW_HOME="/www/server/gowaf"
GW_BIN="${GW_HOME}/bin/gowaf"
GW_CONF="${GW_HOME}/configs/config.yaml"
GW_RULES="${GW_HOME}/rules/builtin.yaml"
GW_BACKUP="${GW_HOME}/backup"
GW_SYNC_OFF="${GW_HOME}/.port-sync-off"     # 存在则 resync 定时器不动作

BT_PANEL="/www/server/panel"
BT_CERT_DIR="${BT_PANEL}/vhost/cert"
BT_VHOST_DIR="${BT_PANEL}/vhost/apache"
BT_APACHE="/www/server/apache"

UPSTREAM_PORT=8080
ADMIN_PORT=8899
FORCE=0
QUIET=0
HTTP_ONLY=0
BINARY_SRC=""
RULES_SRC=""
CF_MODE=0                   # 1 = 站点挂在 Cloudflare 小云朵（代理）后面

# Cloudflare 回源 IP 段。CF 只是「IP 随机、网段固定」，
# 所以这里列的是网段而不是单个 IP；安装时会尝试联网拉官方最新值，失败则用内置兜底。
# 内置值来自 https://www.cloudflare.com/ips-v4 与 /ips-v6
CF_V4_FALLBACK="173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22
141.101.64.0/18 108.162.192.0/18 190.93.240.0/20 188.114.96.0/20
197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13
104.24.0.0/14 172.64.0.0/13 131.0.72.0/22"
CF_V6_FALLBACK="2400:cb00::/32 2606:4700::/32 2803:f800::/32 2405:b500::/32
2405:8100::/32 2a06:98c0::/29 2c0f:f248::/32"
CF_CIDRS=""                 # 运行期填充，空格分隔
CF_ACTION=""                # cloudflare 子命令的第二个词：on/off/update/status

# 运行期探测结果
APACHE_SVC=""
APACHE_CONF=""
APACHE_CTL=""
VHOST_DIR=""
CERT_DIR=""
TAKE_443=0
DEFAULT_DOMAIN=""
CERT_NAMES=""               # 证书目录下「有 fullchain+privkey」的条目，空格分隔
CERT_SANS_INFO=""           # 每行：目录|SAN 列表|覆盖的 vhost 域名数
VHOST_NAMES=""              # 所有 vhost 的 ServerName + ServerAlias（小写去重）
COVERED_NAMES=""            # 至少被一张证书覆盖的域名
MISSING_NAMES=""            # 没有任何证书覆盖的域名（CF 回源会 526）
SELFCHECK_HOST=""           # 自检用的 SNI（必须是真实域名，不能是 112.5.37.34 这类站点名）

# ---------------------------------------------------------------- 输出

c_red=$'\033[31m'; c_grn=$'\033[32m'; c_yel=$'\033[33m'; c_dim=$'\033[2m'; c_off=$'\033[0m'
step() { [[ $QUIET -eq 1 ]] || printf '\n%s==>%s %s\n' "$c_dim" "$c_off" "$*"; }
info() { [[ $QUIET -eq 1 ]] || printf '    %s\n' "$*"; }
ok()   { [[ $QUIET -eq 1 ]] || printf '    %s✓%s %s\n' "$c_grn" "$c_off" "$*"; }
warn() { printf '    %s!%s %s\n' "$c_yel" "$c_off" "$*" >&2; }
die()  { printf '\n%s✗ %s%s\n\n' "$c_red" "$*" "$c_off" >&2; exit 1; }

need_root() { [[ $EUID -eq 0 ]] || die "必须用 root 运行（sudo -i 后再执行）"; }

# 交互确认；--force 或非交互环境（CI/管道）需显式 --force
confirm() {
  local msg=$1 ans
  [[ $FORCE -eq 1 ]] && return 0
  [[ -t 0 ]] || die "非交互环境，请加 --force 表示已知晓风险"
  read -r -p "${msg} [y/N] " ans
  [[ ${ans,,} == y* ]] || die "已取消"
}

# ---------------------------------------------------------------- 证书工具

# 汇总所有 vhost 的 ServerName / ServerAlias（小写、去端口、去重、去掉 _ 与 * 占位）
collect_vhost_names() {
  local f
  for f in "$VHOST_DIR"/*.conf; do
    [[ -f $f ]] || continue
    awk 'tolower($1) ~ /^server(name|alias)$/ { for (i = 2; i <= NF; i++) print tolower($i) }' \
      "$f" 2>/dev/null || true
  done | sed -E 's/:[0-9]+$//' | grep -vE '^(_|\*|)$' | LC_ALL=C sort -u | tr '\n' ' ' || true
}

# 读一张证书实际覆盖的域名。宝塔的目录名是「站点名」不是域名，
# 目录名形如 112.5.37.34 也可能装的是 *.phpframe.org 通配证书，所以必须看 SAN。
cert_sans() {
  local dir=$1 out=""
  if command -v openssl >/dev/null 2>&1 && [[ -f "${CERT_DIR}/${dir}/fullchain.pem" ]]; then
    out=$(openssl x509 -in "${CERT_DIR}/${dir}/fullchain.pem" -noout -ext subjectAltName 2>/dev/null \
          | grep -o 'DNS:[^,[:space:]]*' | sed 's/^DNS://' | tr 'A-Z' 'a-z' | tr '\n' ' ' || true)
  fi
  if [[ -z ${out// /} ]]; then
    out=$(printf '%s' "$dir" | tr 'A-Z' 'a-z')
  fi
  printf '%s' "$out"
}

# 证书是否覆盖某个域名（支持泛域名，且泛域名只匹配一级）
name_covered() {
  local name=$1 sans=$2 s suffix prefix
  for s in $sans; do
    [[ $s == "$name" ]] && return 0
    [[ $s == \*.* ]] || continue
    suffix=${s#\*}
    prefix=${name%"$suffix"}
    if [[ $name == *"$suffix" && -n $prefix && $prefix != *.* ]]; then
      return 0
    fi
  done
  return 1
}

# 选 default_cert：取「覆盖 vhost 域名最多」的那张，而不是字典序第一
# （字典序会把 112.5.37.34 这类站点名排到最前，跟证书实际内容无关）。
pick_cert() {
  local d sans score best_dir="" best_score=-1 best_sans="" all="" n
  local missing="" covered=""

  CERT_NAMES=""
  CERT_SANS_INFO=""
  COVERED_NAMES=""
  MISSING_NAMES=""
  DEFAULT_DOMAIN=""
  SELFCHECK_HOST=""

  for d in $(find "$CERT_DIR" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' 2>/dev/null | LC_ALL=C sort); do
    [[ -f "${CERT_DIR}/${d}/fullchain.pem" && -f "${CERT_DIR}/${d}/privkey.pem" ]] || continue
    CERT_NAMES+="${d} "
    sans=$(cert_sans "$d")
    all+="${sans} "
    score=0
    for n in $VHOST_NAMES; do
      name_covered "$n" "$sans" && score=$((score + 1))
    done
    CERT_SANS_INFO+="${d}|${sans}|${score}"$'\n'
    if (( score > best_score )); then
      best_score=$score
      best_dir=$d
      best_sans=$sans
    fi
  done

  CERT_NAMES=${CERT_NAMES% }
  DEFAULT_DOMAIN=$best_dir

  for n in $VHOST_NAMES; do
    # 占位域名和纯 IP 没有证书是正常的，不参与告警。
    # ssl.* 与 8 位十六进制前缀是宝塔自动生成的站点名，永远不会作为 SNI 出现。
    case $n in
      bt.default.com|localhost|*.local|ssl.*) continue ;;
    esac
    [[ $n =~ ^[0-9a-f]{8}\. ]] && continue
    [[ $n =~ ^[0-9.]+$ ]] && continue
    [[ $n == *:* ]] && continue
    if name_covered "$n" "$all"; then covered+="${n} "; else missing+="${n} "; fi
  done
  COVERED_NAMES=${covered% }
  MISSING_NAMES=${missing% }

  # 自检用的 SNI：优先取「default_cert 真正覆盖的 vhost 域名」
  for n in $VHOST_NAMES; do
    case $n in
      bt.default.com|localhost|*.local) continue ;;
    esac
    [[ $n =~ ^[0-9.]+$ ]] && continue
    [[ $n == *:* ]] && continue
    if name_covered "$n" "$best_sans"; then
      SELFCHECK_HOST=$n
      break
    fi
  done
  # 注意：这里必须是 if，不能写成 [[ -z ... ]] && ...，
  # 否则条件不成立时函数返回 1，在 set -e 下会静默终止整个脚本。
  if [[ -z $SELFCHECK_HOST ]]; then
    SELFCHECK_HOST=${COVERED_NAMES%% *}
  fi
}

# ---------------------------------------------------------------- 环境探测

detect_env() {
  step "探测运行环境"

  [[ -f /etc/os-release ]] || die "无法识别系统，本脚本只支持 Ubuntu/Debian"
  # shellcheck disable=SC1091
  . /etc/os-release
  info "系统：${PRETTY_NAME:-unknown}"

  # --- Apache ---
  if [[ -f "${BT_APACHE}/conf/httpd.conf" ]]; then
    APACHE_CONF="${BT_APACHE}/conf/httpd.conf"
    VHOST_DIR="${BT_VHOST_DIR}"
    CERT_DIR="${BT_CERT_DIR}"
    info "Web 服务器：宝塔 Apache（${APACHE_CONF}）"
    info "站点目录：${VHOST_DIR}"
  elif [[ -f /etc/apache2/apache2.conf ]]; then
    APACHE_CONF="/etc/apache2/apache2.conf"
    VHOST_DIR="/etc/apache2/sites-enabled"
    CERT_DIR="${CERT_DIR:-/etc/gowaf/certs}"
    info "Web 服务器：系统 Apache（${APACHE_CONF}）"
  else
    die "没找到 Apache 配置。本脚本面向 Apache 环境；Nginx 请勿直接运行（会改错文件）"
  fi

  for s in httpd apache2; do
    if systemctl list-unit-files "${s}.service" >/dev/null 2>&1; then
      APACHE_SVC="$s"; break
    fi
  done
  [[ -n $APACHE_SVC ]] || die "找不到 httpd/apache2 的 systemd 服务"
  info "Apache 服务名：${APACHE_SVC}"

  # 宝塔的 apachectl 不在 PATH 里，必须显式找
  for cand in \
    "${BT_APACHE}/bin/apachectl" \
    "$(command -v apachectl 2>/dev/null || true)" \
    "$(command -v apache2ctl 2>/dev/null || true)" \
    /usr/sbin/apache2ctl
  do
    if [[ -n $cand && -x $cand ]]; then APACHE_CTL="$cand"; break; fi
  done
  [[ -n $APACHE_CTL ]] || die "找不到 apachectl / apache2ctl，无法校验配置"
  info "配置校验器：${APACHE_CTL}"

  # --- 端口占用 ---
  # 8080 被占用有两种完全不同的含义，不能一刀切：
  #   a) 首次部署，别的程序占着 → 必须换端口，否则 Apache 退不下来
  #   b) 已经部署过，Apache 正按上次的要求蹲在 8080 上 → 这是稳态，重装时必须放行
  # （重装时最容易被 b 误伤，报「无法作为 Apache 回退端口」然后让人白停服务）
  if port_listening "$UPSTREAM_PORT"; then
    if [[ -f /etc/systemd/system/gowaf.service && -f $GW_CONF ]] && port_held_by_apache "$UPSTREAM_PORT"; then
      info "端口 ${UPSTREAM_PORT}：Apache 已按上次部署退到这里（重装正常态，无需停服务）"
    else
      local who
      who=$(port_holder "$UPSTREAM_PORT" | head -2 || true)
      die "端口 ${UPSTREAM_PORT} 已被占用，无法作为 Apache 回退端口：
${who}
    换一个端口：--upstream-port 8090；或先停掉占用它的程序"
    fi
  fi

  if ss -lntp 2>/dev/null | grep -qE ":${ADMIN_PORT}\b"; then
    [[ -f $GW_BIN ]] || die "控制台端口 ${ADMIN_PORT} 被其他程序占用"
  fi

  # --- 证书：决定是否接管 443 ---
  if [[ -d $CERT_DIR ]]; then
    VHOST_NAMES=$(collect_vhost_names)
    pick_cert
    if [[ -n $DEFAULT_DOMAIN ]]; then
      TAKE_443=1
      info "证书目录：${CERT_NAMES}"
      info "TLS 默认证书：${DEFAULT_DOMAIN}（覆盖 $(echo "$CERT_SANS_INFO" | awk -F'|' -v d="$DEFAULT_DOMAIN" '$1==d{print $2}')）"
      if [[ -n $MISSING_NAMES ]]; then
        warn "这些 vhost 域名没有任何证书覆盖：${MISSING_NAMES}"
        warn "它们经 Cloudflare 回源会 526（SNI 对不上任何证书）。用 install.sh doctor 可看完整对照表"
      fi
    else
      info "证书目录 ${CERT_DIR} 下没有 fullchain.pem + privkey.pem：只接管 80，HTTPS 仍由 Apache 处理"
    fi
  else
    info "没有证书目录：只接管 80，HTTPS 仍由 Apache 处理（WAF 不覆盖 HTTPS）"
  fi

  if [[ $HTTP_ONLY -eq 1 && $TAKE_443 -eq 1 ]]; then
    TAKE_443=0
    info "--http-only 生效：本次只接管 80，443 完全不动（HTTPS 流量暂不经过 WAF）"
  fi

  for cmd in curl ss systemctl; do
    command -v "$cmd" >/dev/null 2>&1 || die "缺少命令：${cmd}（apt-get install -y ${cmd}）"
  done
}

find_binary() {
  [[ -n $BINARY_SRC ]] && return
  local d
  d=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  for cand in \
    "${d}/gowaf-linux-amd64" \
    "${d}/gowaf" \
    "${d}/../bin/gowaf-linux-amd64" \
    "${d}/../bin/gowaf"
  do
    if [[ -f $cand ]]; then BINARY_SRC="$cand"; return; fi
  done
  die "找不到 gowaf 二进制。请先在本机执行 make build-all，把 bin/gowaf-linux-amd64
    和本脚本一起传到服务器同一目录，或显式指定：--binary /tmp/gowaf"

  # 说明：不接受「服务器上 go build」，生产容器缺 git/前端产物，构建必失败。
}

# ---------------------------------------------------------------- 备份

BK_DIR=""
backup_apache() {
  local ts
  ts=$(date +%Y%m%d-%H%M%S)
  BK_DIR="${GW_BACKUP}/${ts}"
  mkdir -p "$BK_DIR"

  cp -a "$APACHE_CONF" "${BK_DIR}/" 2>/dev/null || true
  if [[ -d $VHOST_DIR ]]; then
    mkdir -p "${BK_DIR}/vhost"
    cp -a "${VHOST_DIR}/." "${BK_DIR}/vhost/" 2>/dev/null || true
  fi

  # 记录还原所需的最小信息，rollback 时不必猜
  cat > "${BK_DIR}/meta.env" <<EOF
APACHE_CONF=${APACHE_CONF}
VHOST_DIR=${VHOST_DIR}
APACHE_SVC=${APACHE_SVC}
UPSTREAM_PORT=${UPSTREAM_PORT}
TAKE_443=${TAKE_443}
EOF

  ln -sfn "$BK_DIR" "${GW_BACKUP}/latest"

  # 只留最近 10 份：resync 每次纠正都会备份，不清理会慢慢吃满磁盘
  find "$GW_BACKUP" -mindepth 1 -maxdepth 1 -type d -name '20*' -printf '%T@ %p\n' 2>/dev/null \
    | sort -rn | tail -n +11 | cut -d' ' -f2- | xargs -r rm -rf

  ok "Apache 配置已备份到 ${BK_DIR}"
}

latest_backup() {
  local d
  d=$(readlink -f "${GW_BACKUP}/latest" 2>/dev/null || true)
  [[ -n $d && -d $d ]] || return 1
  echo "$d"
}

# ---------------------------------------------------------------- Apache 改造

apache_test() {
  local out
  if [[ -z $APACHE_CTL ]]; then
    echo "APACHE_CTL 未设置，无法检查 Apache 配置" >&2
    return 1
  fi
  if ! out=$("$APACHE_CTL" configtest 2>&1); then
    printf '%s\n' "$out" >&2
    return 1
  fi
  return 0
}

# 宝塔的 Apache 由 SysV 脚本托管（systemctl 显示 active (exited)），
# systemctl reload/restart 对它经常不生效或半生效，优先走 /etc/init.d。
apache_ctl() {
  local action=$1
  local init="/etc/init.d/${APACHE_SVC}"
  if [[ -x $init ]]; then
    "$init" "$action" >/dev/null 2>&1
    return $?
  fi
  systemctl "$action" "$APACHE_SVC" >/dev/null 2>&1
}

apache_reload() {
  if ! apache_ctl reload; then
    apache_ctl restart
  fi
}

# ---------------------------------------------------------------- 端口检查
# 只匹配精确端口：:80 不能命中 :8080
port_listening() {
  ss -lnt 2>/dev/null | awk -v p=":$1" 'NR > 1 { n = split($4, a, ":"); if (":" a[n] == p) f = 1 } END { exit(f ? 0 : 1) }'
}

# 打印占用某端口的进程（ss 的 users:((...)) 字段），没有输出表示没人占
port_holder() {
  ss -lntp 2>/dev/null | awk -v p=":$1" 'NR > 1 { n = split($4, a, ":"); if (":" a[n] == p) print "    " $4 "  " $6 }'
}

# 该端口是不是被 Apache 自己占着（重装时 8080 归 httpd 是正常态）
port_held_by_apache() {
  ss -lntp 2>/dev/null | awk -v p=":$1" 'NR > 1 { n = split($4, a, ":"); if (":" a[n] == p && $0 ~ /httpd|apache2/) f = 1 } END { exit(f ? 0 : 1) }'
}

# 该端口是不是被 gowaf 占着。doctor 用它判断 443 到底有没有经过 WAF：
# 只看「443 是否空闲」不够，443 由 Apache 占着和由 gowaf 占着是两种完全不同的形态。
port_held_by_gowaf() {
  ss -lntp 2>/dev/null | awk -v p=":$1" 'NR > 1 { n = split($4, a, ":"); if (":" a[n] == p && $0 ~ /gowaf/) f = 1 } END { exit(f ? 0 : 1) }'
}

# 读已部署配置里的 tls.enabled。比看端口持有者更可靠：gowaf 挂掉时端口无人监听，
# 但配置里仍然写着它该不该接管 443。
gw_tls_enabled() {
  [[ -f $GW_CONF ]] || return 1
  awk '
    /^tls:/ { t = 1; next }
    t && /^[a-z_]+:/ { exit }
    t && /^[[:space:]]+enabled:[[:space:]]*true/ { f = 1; exit }
    END { exit(f ? 0 : 1) }
  ' "$GW_CONF"
}

wait_port_listening() {
  local p=$1 n=${2:-20} i
  for ((i = 0; i < n; i++)); do
    port_listening "$p" && return 0
    sleep 0.5
  done
  return 1
}

wait_port_free() {
  local p=$1 n=${2:-20} i
  for ((i = 0; i < n; i++)); do
    port_listening "$p" || return 0
    sleep 0.5
  done
  return 1
}

dump_listen_evidence() {
  local p=$1 el="${BT_APACHE}/logs/error_log"
  printf '\n--- 端口 %s 的占用者 ---\n' "$p" >&2
  port_holder "$p" >&2 || true
  printf '\n--- Apache 进程 ---\n' >&2
  ps -eo pid,ppid,args 2>/dev/null | grep -E '[h]ttpd' | head -8 >&2 || true
  printf '\n--- systemctl status ---\n' >&2
  systemctl status --no-pager -l "$APACHE_SVC" 2>/dev/null | head -12 >&2 || true
  if [[ -f $el ]]; then
    printf '\n--- %s 末尾 ---\n' "$el" >&2
    tail -15 "$el" >&2 || true
  fi
}

# 宝塔除 httpd.conf 外，conf/extra 等目录也可能写 Listen，只注释主文件不够
comment_stray_listen() {
  local port=$1 f n=0
  while IFS= read -r f; do
    [[ -n $f && $f != "$APACHE_CONF" ]] || continue
    sed -i -E "s/^([[:space:]]*Listen[[:space:]]+(\[::\]:|0\.0\.0\.0:|127\.0\.0\.1:)?${port}[[:space:]]*)\$/# [GOWAF] \1/" "$f"
    n=$((n + 1))
    info "注释 $(basename "$f") 里的 Listen ${port}"
  done < <(grep -rlE "^[[:space:]]*Listen[[:space:]]+(\[::\]:|0\.0\.0\.0:|127\.0\.0\.1:)?${port}[[:space:]]*$" \
             "$(dirname "$APACHE_CONF")" 2>/dev/null || true)
  [[ $n -eq 0 ]] || warn "共注释 ${n} 处额外的 Listen ${port}"
}

# 注释掉 Listen 80 / Listen 443（含 IPv6 与显式 IP 形式）
comment_listen() {
  local port=$1
  sed -i -E "s/^([[:space:]]*Listen[[:space:]]+(\[::\]:|0\.0\.0\.0:|127\.0\.0\.1:)?${port}[[:space:]]*)\$/# [GOWAF] \1/" "$APACHE_CONF"
}

inject_httpd_block() {
  grep -q 'GOWAF-MANAGED' "$APACHE_CONF" && { info "httpd.conf 已注入过 GOWAF 块，跳过"; return; }

  # mod_remoteip 缺失会导致 Apache 日志里全是 127.0.0.1，宝塔的日志/封IP 功能全废
  if [[ $APACHE_CONF == /etc/apache2/apache2.conf ]]; then
    # Debian/Ubuntu 的系统 Apache 用 a2enmod，直接写 LoadModule 会与 mods-enabled 重复加载而报错
    if a2enmod remoteip >/dev/null 2>&1; then
      ok "启用 mod_remoteip（真实 IP 透传）"
    fi
  elif ! grep -qE '^[[:space:]]*LoadModule[[:space:]]+remoteip_module' "$APACHE_CONF"; then
    local so
    for so in "${BT_APACHE}/modules/mod_remoteip.so" /usr/lib/apache2/modules/mod_remoteip.so; do
      if [[ -f $so ]]; then
        printf '\nLoadModule remoteip_module %s\n' "$so" >> "$APACHE_CONF"
        ok "启用 mod_remoteip（真实 IP 透传）"
        break
      fi
    done
  fi

  cat >> "$APACHE_CONF" <<EOF

# ===== GOWAF-MANAGED BEGIN =====
# 由 gowaf 部署脚本写入。手工修改会在 gowaf resync 时被覆盖。
Listen 127.0.0.1:${UPSTREAM_PORT}

# WAF 与 Apache 同机，只信任本机传来的 X-Forwarded-For
RemoteIPHeader X-Forwarded-For
RemoteIPInternalProxy 127.0.0.1

# TLS 已在 WAF 终止，告诉 PHP/Apache "这是 HTTPS"，
# 否则站点里的 RewriteRule .* https://%{HTTP_HOST}%{REQUEST_URI} 会无限 301。
SetEnvIf X-Forwarded-Proto "^https\$" HTTPS=on
# ===== GOWAF-MANAGED END =====
EOF
  ok "httpd.conf 注入 Listen ${UPSTREAM_PORT} + 真实 IP + HTTPS 环境变量"
}

# 把 <VirtualHost *:443> 整段注释掉（TLS 已经由 WAF 终止，这段再留着会造成重复 ServerName）
disable_ssl_vhosts() {
  local f changed
  for f in "$VHOST_DIR"/*.conf; do
    [[ -f $f ]] || continue
    grep -qE '^[[:space:]]*<VirtualHost[^>]*:443>' "$f" || continue
    changed=0
    awk '
      /^[[:space:]]*<VirtualHost[^>]*:443[^>]*>/ { in443 = 1 }
      {
        if (in443) { print "# [GOWAF] " $0 } else { print }
      }
      in443 && /<\/VirtualHost>/ { in443 = 0; changed = 1 }
      END { exit(changed ? 0 : 3) }
    ' "$f" > "${f}.gowaf.tmp" || { rm -f "${f}.gowaf.tmp"; continue; }
    if ! cmp -s "$f" "${f}.gowaf.tmp"; then
      mv "${f}.gowaf.tmp" "$f"
      ok "已禁用 SSL vhost：$(basename "$f")"
    else
      rm -f "${f}.gowaf.tmp"
    fi
  done
}

retarget_vhosts() {
  local f n=0 before
  for f in "$VHOST_DIR"/*.conf; do
    [[ -f $f ]] || continue
    before=$(md5sum "$f" | awk '{print $1}')
    # 直接 sed 全部替换，已经改过的行不会匹配，天然幂等；
    # 不能用「文件里已有 8080 就跳过整文件」——宝塔可能往同一文件追加新的 *:80。
    # 锚行首：注释掉的 <VirtualHost *:443> 不能被误改。
    sed -i -E "s/^([[:space:]]*)<VirtualHost[[:space:]]+\*:80[[:space:]]*>/\\1<VirtualHost 127.0.0.1:${UPSTREAM_PORT}>/g" "$f"
    sed -i -E "s/^([[:space:]]*)<VirtualHost[[:space:]]+\*:8080[[:space:]]*>/\\1<VirtualHost 127.0.0.1:${UPSTREAM_PORT}>/g" "$f"
    [[ $(md5sum "$f" | awk '{print $1}') != "$before" ]] && n=$((n + 1))
  done
  [[ $n -eq 0 ]] && info "所有 vhost 端口已经就位" || ok "改写 ${n} 个 vhost 文件到 127.0.0.1:${UPSTREAM_PORT}"
}

# 安装中途失败统一收尾：停掉 gowaf、关掉自愈定时器、还原 Apache
abort_install() {
  local msg=$1
  touch "$GW_SYNC_OFF" 2>/dev/null || true
  systemctl disable --now gowaf 2>/dev/null || true
  systemctl disable --now gowaf-resync.timer 2>/dev/null || true
  restore_apache_from "$BK_DIR" || true
  die "$msg"
}

# 主配置 + 零散 Listen + 全部 vhost 的指纹，用来判断这次是不是真的改动了 Apache
apache_fingerprint() {
  {
    cat "$APACHE_CONF" 2>/dev/null || true
    cat "${BT_APACHE}"/conf/extra/*.conf 2>/dev/null || true
    cat "$VHOST_DIR"/*.conf 2>/dev/null || true
  } | md5sum | awk '{print $1}'
}

patch_apache() {
  step "改造 Apache：让出 80${TAKE_443:+/443}，退到 127.0.0.1:${UPSTREAM_PORT}"

  local before after
  before=$(apache_fingerprint)

  comment_listen 80
  comment_stray_listen 80
  if [[ $TAKE_443 -eq 1 ]]; then
    comment_listen 443
    comment_stray_listen 443
  else
    info "未接管 443，保持 Apache 原样"
  fi
  inject_httpd_block
  retarget_vhosts
  if [[ $TAKE_443 -eq 1 ]]; then
    disable_ssl_vhosts
  fi

  if ! apache_test; then
    warn "Apache 配置校验失败，立即回滚"
    restore_apache_from "$BK_DIR"
    die "已回滚 Apache 配置，站点未受影响。请把上面的报错发给开发者"
  fi
  ok "Apache 配置校验通过"

  # 重装/升级时配置通常一个字节都没变，这时重启 Apache 纯属制造 5 秒 502。
  # 只有真的改动了文件（或 Apache 根本没在跑）才走「停 → 等 80 真空 → 起」。
  after=$(apache_fingerprint)
  if [[ $before == "$after" ]] && systemctl is-active --quiet "$APACHE_SVC"; then
    ok "Apache 配置无变化（已在 127.0.0.1:${UPSTREAM_PORT}），跳过重启，站点零中断"
    return 0
  fi

  # Listen 变更必须「先停、确认 80 真空了、再起」。
  # 宝塔的 Apache 由 SysV 脚本托管，systemctl restart 经常变成空操作
  # （stop 没真正停，start 报 already running 却返回成功），
  # 结果就是 Apache 仍用旧配置占着 80，gowaf 绑定时才暴露成 bind: address already in use。
  apache_stop_and_verify
  apache_start_and_verify
}

apache_stop_and_verify() {
  info "停止 Apache（Listen 变动只能靠重启生效，约 2~5 秒内站点短暂不可用）"
  apache_ctl stop || true
  if ! wait_port_free 80 12; then
    "$APACHE_CTL" -k stop >/dev/null 2>&1 || true
  fi
  if ! wait_port_free 80 20; then
    warn "Apache 已 stop，但 80 仍然被占用"
    dump_listen_evidence 80
    restore_apache_from "$BK_DIR"
    die "80 没有释放，无法交给 WAF。请按上面的占用者信息处理后重试"
  fi
  if [[ $TAKE_443 -eq 1 ]] && ! wait_port_free 443 10; then
    warn "443 仍被占用"
    dump_listen_evidence 443
    restore_apache_from "$BK_DIR"
    die "443 没有释放，无法交给 WAF"
  fi
  ok "Apache 已停止，80${TAKE_443:+/443} 已让出"
}

apache_start_and_verify() {
  local el="${BT_APACHE}/logs/error_log"
  apache_ctl start || true
  if ! wait_port_listening "$UPSTREAM_PORT" 20; then
    warn "systemctl start 之后没看到 ${UPSTREAM_PORT}，改用 apachectl -k restart 再试"
    "$APACHE_CTL" -k restart >/dev/null 2>&1 || true
  fi
  if ! wait_port_listening "$UPSTREAM_PORT" 20; then
    warn "Apache 启动了，但没有监听 127.0.0.1:${UPSTREAM_PORT}"
    dump_listen_evidence "$UPSTREAM_PORT"
    if [[ -f $el ]]; then
      printf '\n--- %s 末尾 ---\n' "$el" >&2
      tail -25 "$el" >&2 || true
    fi
    restore_apache_from "$BK_DIR"
    die "Apache 未能切到 ${UPSTREAM_PORT}，已回滚。上面是 error_log"
  fi
  ok "Apache 已监听 127.0.0.1:${UPSTREAM_PORT}"
  if port_listening 80; then
    warn "80 仍有监听（可能是别的程序）："
    port_holder 80 >&2 || true
  fi
}

# 启动 gowaf 前的最后一道闸：端口不是 Apache 让出来的，就不能怪 gowaf 绑不上
ensure_port_free() {
  local p=$1
  port_listening "$p" || return 0
  warn "端口 ${p} 仍被占用，gowaf 无法监听："
  port_holder "$p" >&2 || true
  abort_install "端口 ${p} 没有被释放（占用者见上）。Apache 配置已还原，站点未受影响"
}

restore_apache_from() {
  local bd=$1
  [[ -d $bd ]] || { warn "备份目录不存在：$bd"; return 1; }

  # 路径变量为空时必须立刻停：cp 到空目标 = 破坏文件系统当前状态，
  # 静默失败比大声失败更危险（调用方会以为还原成功了）。
  if [[ -z $APACHE_CONF || -z $VHOST_DIR || -z $APACHE_SVC ]]; then
    warn "还原参数不完整：APACHE_CONF=[$APACHE_CONF] VHOST_DIR=[$VHOST_DIR] APACHE_SVC=[$APACHE_SVC]"
    warn "detect_env 未运行或 meta.env 缺失，拒绝在参数未知时动 Apache"
    return 1
  fi

  step "从备份还原 Apache 配置（${bd}）"
  # 不再用 || true：还原失败必须让 rollback 整体失败，
  # 否则会出现「看似回滚了，其实端口还卡在 8080」的假成功。
  cp -a "${bd}/$(basename "$APACHE_CONF")" "$APACHE_CONF" || return 1
  if [[ -d "${bd}/vhost" ]]; then
    cp -a "${bd}/vhost/." "$VHOST_DIR/" || return 1
  fi
  if apache_test; then
    apache_reload
    ok "Apache 已还原并重载"
  else
    die "还原后配置仍有错误，请人工介入：$APACHE_CONF"
  fi
}

# ---------------------------------------------------------------- 站点配置

collect_sites_yaml() {
  local f name alias line hosts=() first=1 seen=" "

  printf 'sites:\n'
  for f in "$VHOST_DIR"/*.conf; do
    [[ -f $f ]] || continue
    name=$(awk '/^[[:space:]]*ServerName[[:space:]]/{print $2; exit}' "$f" 2>/dev/null || true)
    [[ -n $name ]] || continue
    name=${name,,}
    case $name in
      localhost|127.0.0.1|default|_|"") continue ;;
    esac
    [[ $seen == *" $name "* ]] && continue
    seen+="$name "

    hosts=("$name")
    # 同一个文件里的 :80 与 :443 两个 vhost 常写同一个 ServerAlias，这里去重
    local seen_host=" ${name} "
    while IFS= read -r alias; do
      alias=${alias,,}
      [[ -z $alias ]] && continue
      [[ $seen_host == *" ${alias} "* ]] && continue
      seen_host+="${alias} "
      hosts+=("$alias")
    done < <(awk '/^[[:space:]]*ServerAlias[[:space:]]/{for(i=2;i<=NF;i++) print $i}' "$f" 2>/dev/null || true)

    if [[ $first -eq 0 ]]; then printf '\n'; fi
    first=0
    printf '  - name: "%s"\n' "$name"
    printf '    hosts: ['
    local i
    for i in "${!hosts[@]}"; do
      [[ $i -gt 0 ]] && printf ', '
      printf '"%s"' "${hosts[$i]}"
    done
    printf ']\n'
    printf '    upstream: "http://127.0.0.1:%s"\n' "$UPSTREAM_PORT"
  done

  # 兜底：未知域名也不至于 404
  [[ $first -eq 0 ]] && printf '\n'
  printf '  - name: "兜底站点"\n'
  printf '    hosts: ["*"]\n'
  printf '    upstream: "http://127.0.0.1:%s"\n' "$UPSTREAM_PORT"
}

# 拉取 Cloudflare 官方回源网段；失败返回非 0，由调用方回退到内置列表。
fetch_cf_cidrs() {
  local v4 v6
  v4=$(curl -fsS --max-time 6 https://www.cloudflare.com/ips-v4 2>/dev/null \
        | tr -d '\r' | grep -E '^[0-9]' | tr '\n' ' ') || return 1
  v6=$(curl -fsS --max-time 6 https://www.cloudflare.com/ips-v6 2>/dev/null \
        | tr -d '\r' | grep -E '^[0-9a-fA-F]' | tr '\n' ' ') || return 1
  [[ -n ${v4// /} ]] || return 1
  CF_CIDRS="${v4} ${v6}"
  return 0
}

resolve_cf_cidrs() {
  if fetch_cf_cidrs; then
    info "已从 Cloudflare 拉取最新回源网段：$(wc -w <<<"$CF_CIDRS") 条"
  else
    CF_CIDRS="${CF_V4_FALLBACK} ${CF_V6_FALLBACK}"
    warn "拉取 Cloudflare 网段失败（服务器无外网？），使用内置列表 $(wc -w <<<"$CF_CIDRS") 条"
  fi
}

# 生成 server.trusted_proxies 段。回环始终保留（本地探活要用）。
render_trusted_proxies() {
  printf '  trusted_proxies:\n'
  printf '    - "127.0.0.1/32"\n'
  printf '    - "::1/128"\n'
  local c
  for c in $CF_CIDRS; do
    printf '    - "%s"\n' "$c"
  done
}

render_config() {
  local mode=$1
  step "生成配置（engine.mode=${mode}）"

  # 用标记文件记住「这台机器在 CDN 后面」，否则 switch_mode 重渲染会把网段丢掉
  if [[ -f "${GW_HOME}/.cloudflare" ]]; then
    CF_MODE=1
  fi
  if [[ $CF_MODE -eq 1 && -z $CF_CIDRS ]]; then
    resolve_cf_cidrs
  fi

  local guard_lines
  if [[ $mode == "observe" ]]; then
    # 观察期限流必须放到极宽——限流不受 observe 约束，会真的 429
    guard_lines="  global_rps: 0
  ip_rps: 500
  ip_burst: 1000
  ip_per_minute: 0
  auto_ban: false"
  else
    guard_lines="  global_rps: 0
  ip_rps: 50
  ip_burst: 100
  ip_per_minute: 2000
  auto_ban: true"
  fi

  local trusted_block
  trusted_block="$(render_trusted_proxies)"

  local tls_block
  if [[ $TAKE_443 -eq 1 ]]; then
    tls_block="tls:
  enabled: true
  cert_dir: \"${CERT_DIR}\"
  default_cert: \"${DEFAULT_DOMAIN}\"
  min_version: \"1.2\"
  hsts: false          # 首次上线先别开 HSTS，避免证书问题被浏览器记住"
  else
    tls_block="tls:
  enabled: false       # 未接管 443：HTTPS 仍直连 Apache，WAF 不检测这部分流量"
  fi

  cat > "$GW_CONF" <<EOF
# GoWAF 生产配置（由 deploy/install.sh 生成于 $(date '+%F %T')）
# 修改后需 systemctl restart gowaf 生效；控制台里改的设置优先于本文件的 sites。

server:
  listen: "0.0.0.0:80"
  tls_listen: "$([[ $TAKE_443 -eq 1 ]] && echo '0.0.0.0:443' || echo '')"
  admin_listen: "127.0.0.1:${ADMIN_PORT}"
  read_header_timeout: 5s
  read_timeout: 30s
  write_timeout: 0s
  idle_timeout: 90s
  max_header_bytes: 32768
  shutdown_timeout: 15s
${trusted_block}

${tls_block}

storage:
  dsn: "${GW_HOME}/data/waf.db"
  retain_days: 30
  sample_rate: 0.01

$(collect_sites_yaml)

engine:
  mode: "${mode}"
  block_score: 60
  challenge_score: 30
  max_body_inspect: 262144
  max_targets: 64
  regex_timeout_ms: 50
  rules_file: "${GW_RULES}"

guard:
${guard_lines}
  challenge_per_minute: 300
  ban_per_minute: 600
  ban_1h_per_minute: 1200
  # 自动封禁：ban_window 内命中 ban_threshold 次 → 封 ban_ttl；
  # 解封后再触发算一次连击，第 ban_max_strikes 次直接永久（expires_at=0）；
  # 超过 ban_strike_reset 没有新命中，连击数清零。都可以在控制台热改。
  ban_window: 10m
  ban_threshold: 5
  ban_ttl: 1h
  ban_max_ttl: 24h
  ban_max_strikes: 3
  ban_strike_reset: 24h
  whitelist_static: true

admin:
  allowed_ips: ["127.0.0.1/32"]
  jwt_secret_file: "${GW_HOME}/data/jwt.key"
  access_ttl: 2h
  refresh_ttl: 168h
  login_max_fail: 5
  login_lock: 15m
  cookie_secure: false          # 控制台只监听回环，通过 SSH 隧道以 http 访问；
                                # 置 true 会导致浏览器不发送 cookie，直接登录不了
  cookie_domain: ""
  cors:
    enabled: false
  csrf:
    enabled: true
    header: "X-CSRF-Token"
    cookie_name: "waf_csrf"

alert:
  webhook: ""
  block_threshold: 500
  window: 5m

log:
  level: "info"
  file: "${GW_HOME}/logs/waf.log"
  max_size_mb: 100
  max_backups: 14
EOF
  ok "配置已写入 ${GW_CONF}"
}

# ---------------------------------------------------------------- 安装件

install_files() {
  step "安装文件到 ${GW_HOME}"
  mkdir -p "${GW_HOME}"/{bin,configs,rules,data,logs,backup}
  chmod 750 "${GW_HOME}/data"

  install -m 755 "$BINARY_SRC" "$GW_BIN"
  ok "二进制：$("$GW_BIN" -v 2>/dev/null || echo gowaf)"

  local d self_src
  d=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  # 把自己也装一份：console 的定时收回由 systemd timer 调用它。
  # 从 /usr/local/bin/gowaf 跑 install 时目录里没有 install.sh，退回用脚本自身
  self_src="${d}/install.sh"
  [[ -f $self_src ]] || self_src="${BASH_SOURCE[0]}"
  if [[ -f $self_src ]]; then
    install -m 755 "$self_src" "${GW_HOME}/bin/install.sh"
  fi
  if [[ -n $RULES_SRC ]]; then
    install -m 644 "$RULES_SRC" "$GW_RULES"
    ok "规则文件已更新：${RULES_SRC}"
  elif [[ -f $GW_RULES ]]; then
    # 升级时不覆盖：你在生产上调过的规则比仓库里的新
    info "保留已有规则文件：${GW_RULES}（要覆盖请加 --rules）"
  elif [[ -f "${d}/../rules/builtin.yaml" ]]; then
    install -m 644 "${d}/../rules/builtin.yaml" "$GW_RULES"
  elif [[ -f "${d}/builtin.yaml" ]]; then
    install -m 644 "${d}/builtin.yaml" "$GW_RULES"
  else
    die "找不到 rules/builtin.yaml，请用 --rules 指定"
  fi
  ok "规则：$(grep -c '^- id:' "$GW_RULES" 2>/dev/null || echo '?') 条"
}

install_systemd() {
  step "安装 systemd 服务"

  cat > /etc/systemd/system/gowaf.service <<EOF
[Unit]
Description=GoWAF (反向代理 + 规则检测)
Documentation=file://${GW_HOME}
After=network-online.target ${APACHE_SVC}.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${GW_HOME}
# 先校验配置，配错了宁可不启动，也不要带着错规则跑
ExecStartPre=${GW_BIN} -t -c ${GW_CONF}
ExecStart=${GW_BIN} -c ${GW_CONF}
Restart=always
RestartSec=3s
TimeoutStopSec=45
LimitNOFILE=65535
KillSignal=SIGTERM
# 需要 bind 80/443，并读取宝塔证书私钥（root-only），故以 root 运行
User=root

[Install]
WantedBy=multi-user.target
EOF

  # 端口漂移自愈：宝塔面板保存站点/改 SSL 会把 vhost 端口写回 80，
  # 此时 WAF 占着 80，Apache 会起不来 → 站点直接挂。定时纠正比挂着好。
  cat > /etc/systemd/system/gowaf-resync.service <<EOF
[Unit]
Description=GoWAF 纠正 Apache 端口漂移

[Service]
Type=oneshot
ExecStart=/usr/local/bin/gowaf resync --quiet
EOF

  cat > /etc/systemd/system/gowaf-resync.timer <<'EOF'
[Unit]
Description=每 60 秒检查一次 Apache 是否抢回 80/443

[Timer]
OnBootSec=90
OnUnitActiveSec=60
AccuracySec=10

[Install]
WantedBy=timers.target
EOF

  cat > /etc/logrotate.d/gowaf <<EOF
${GW_HOME}/logs/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
EOF

  systemctl daemon-reload
  systemctl enable "${APACHE_SVC}" >/dev/null 2>&1 || true
  systemctl enable gowaf.service >/dev/null
  systemctl enable --now gowaf-resync.timer >/dev/null
  ok "gowaf.service / gowaf-resync.timer / logrotate 已就位"
}

install_cli() {
  local self
  self=$(readlink -f "${BASH_SOURCE[0]}")
  if [[ $self != "/usr/local/bin/gowaf" ]]; then
    install -m 755 "$self" /usr/local/bin/gowaf
    ok "管理命令：gowaf（start/stop/restart/status/logs/observe/protect/resync/rollback）"
  fi
}

# ---------------------------------------------------------------- 服务控制

svc_start() {
  need_root
  systemctl start gowaf
  sleep 1
  systemctl is-active --quiet gowaf || die "gowaf 启动失败：journalctl -u gowaf -n 50 --no-pager"
  ok "gowaf 已启动（规则 $(grep -c '^- id:' "$GW_RULES" 2>/dev/null || echo '?') 条）"
}

svc_stop() {
  need_root
  systemctl stop gowaf
  ok "gowaf 已停止"
  warn "gowaf 停止后 80/443 无人监听，站点将不可访问。要让站点恢复请执行：gowaf rollback"
}

svc_restart() {
  need_root
  systemctl restart gowaf
  sleep 1
  systemctl is-active --quiet gowaf || die "gowaf 重启失败：journalctl -u gowaf -n 50 --no-pager"
  ok "gowaf 已重启"
}

svc_status() {
  step "服务状态"
  if systemctl is-active --quiet gowaf; then
    ok "gowaf running"
  else
    warn "gowaf 未运行"
  fi
  systemctl status gowaf --no-pager -n 0 2>/dev/null | sed -n '1,6p' | sed 's/^/    /' || true

  step "端口"
  ss -lntp 2>/dev/null | grep -E ':(80|443|'"${UPSTREAM_PORT}"'|'"${ADMIN_PORT}"')[[:space:]]' | sed 's/^/    /' || info "无相关监听"

  step "模式"
  local m
  m=$(awk '/^  mode:/{print $2}' "$GW_CONF" 2>/dev/null | tr -d '"')
  info "engine.mode = ${m:-unknown}"

  step "最近日志"
  journalctl -u gowaf -n 8 --no-pager 2>/dev/null | sed 's/^/    /' || true
}

switch_mode() {
  local mode=$1
  need_root
  [[ -f $GW_CONF ]] || die "配置不存在，请先 install"
  sed -i -E "s/^  mode: .*/  mode: \"${mode}\"/" "$GW_CONF"
  if [[ $mode == "observe" ]]; then
    sed -i -E 's/^  ip_rps: .*/  ip_rps: 500/; s/^  ip_burst: .*/  ip_burst: 1000/; s/^  ip_per_minute: .*/  ip_per_minute: 0/; s/^  auto_ban: .*/  auto_ban: false/' "$GW_CONF"
  else
    sed -i -E 's/^  ip_rps: .*/  ip_rps: 50/; s/^  ip_burst: .*/  ip_burst: 100/; s/^  ip_per_minute: .*/  ip_per_minute: 2000/; s/^  auto_ban: .*/  auto_ban: true/' "$GW_CONF"
  fi
  "$GW_BIN" -t -c "$GW_CONF" >/dev/null || die "配置校验失败，未重启"
  systemctl restart gowaf
  sleep 1
  systemctl is-active --quiet gowaf || die "重启失败：journalctl -u gowaf -n 50"
  ok "已切换到 ${mode} 模式"
  [[ $mode == "protect" ]] && warn "protect 会真的拦截请求。建议先跑 observe 观察 3~7 天，确认没有误杀再切"
  return 0
}

# ---------------------------------------------------------------- Cloudflare

# 原地重写 server.trusted_proxies 段，其余配置一字不动。
rewrite_trusted_proxies() {
  local conf=$GW_CONF newf tmp
  [[ -f $conf ]] || die "配置不存在：$conf"
  newf="${conf}.trusted.new"
  tmp="${conf}.trusted.tmp"
  render_trusted_proxies > "$newf"
  awk -v repl="$newf" '
    BEGIN { while ((getline line < repl) > 0) blk = blk line "\n"; close(repl) }
    /^  trusted_proxies:[[:space:]]*$/ { printf "%s", blk; skip = 1; next }
    skip && /^    - / { next }
    { skip = 0; print }
  ' "$conf" > "$tmp"
  mv "$tmp" "$conf"
  rm -f "$newf"
}

cmd_cloudflare() {
  local action=${CF_ACTION:-status}
  need_root
  case $action in
    on|enable)
      mkdir -p "$GW_HOME"
      touch "${GW_HOME}/.cloudflare"
      CF_MODE=1; CF_CIDRS=""
      resolve_cf_cidrs
      rewrite_trusted_proxies
      "$GW_BIN" -t -c "$GW_CONF" >/dev/null || die "配置校验失败，未重启（原配置已备份为 ${GW_CONF}.trusted.tmp）"
      systemctl restart gowaf
      sleep 1
      systemctl is-active --quiet gowaf || die "重启失败：journalctl -u gowaf -n 50"
      ok "已启用 Cloudflare 回源识别（$(wc -w <<<"$CF_CIDRS") 个网段）"
      warn "Cloudflare 侧 SSL/TLS 模式必须是 Full 或 Full (strict)；Flexible 会导致 301 死循环，且 WAF 拿不到 HTTPS 信息"
      ;;
    off|disable)
      rm -f "${GW_HOME}/.cloudflare"
      CF_MODE=0; CF_CIDRS=""
      rewrite_trusted_proxies
      "$GW_BIN" -t -c "$GW_CONF" >/dev/null || die "配置校验失败，未重启"
      systemctl restart gowaf
      ok "已关闭 Cloudflare 回源识别（只信任回环，适用于 WAF 直接对公网）"
      ;;
    update|refresh)
      [[ -f "${GW_HOME}/.cloudflare" ]] || die "当前未启用 Cloudflare 模式，先执行：$0 cloudflare on"
      CF_MODE=1; CF_CIDRS=""
      resolve_cf_cidrs
      rewrite_trusted_proxies
      "$GW_BIN" -t -c "$GW_CONF" >/dev/null || die "配置校验失败，未重启"
      systemctl restart gowaf
      ok "网段已更新，gowaf 已重启"
      ;;
    status)
      if [[ -f "${GW_HOME}/.cloudflare" ]]; then
        info "Cloudflare 模式：已启用"
      else
        info "Cloudflare 模式：未启用（只信任 127.0.0.1 / ::1）"
      fi
      if [[ -f $GW_CONF ]]; then
        info "配置中可信网段数：$(awk '/^  trusted_proxies:/{f=1;next} f&&/^    - /{n++;next} f&&!/^    - /{exit} END{print n+0}' "$GW_CONF")"
      fi
      ;;
    *) die "用法：$0 cloudflare on|off|update|status" ;;
  esac
}

# ---------------------------------------------------------------- 漂移纠正

cmd_resync() {
  need_root
  [[ -f $GW_SYNC_OFF ]] && return 0
  [[ -f $GW_CONF ]] || return 0
  # gowaf 没在跑就别动 Apache：此时 80 由 Apache 持有是正常状态，
  # 强行改回 8080 会造成 80 无人监听、站点直接挂。
  systemctl is-active --quiet gowaf || return 0

  # 漂移检测与纠正都依赖 APACHE_CONF/VHOST_DIR/APACHE_SVC，
  # 必须先探测；缺了它们会误判漂移、备份出空壳（2026-11-03 生产事故：
  # 每分钟生成空备份，把真正的回滚点挤出保留轮换）。
  detect_env

  local drift=0

  # 1) httpd.conf 里出现未注释的 Listen 80/443
  if grep -qE '^[[:space:]]*Listen[[:space:]]+(\[::\]:|0\.0\.0\.0:)?(80|443)[[:space:]]*$' "$APACHE_CONF" 2>/dev/null; then
    drift=1
  fi

  # 2) vhost 回到 *:80 / *:443（必须锚行首，否则会匹配到我们自己注释掉的 SSL 块）
  if grep -lqE '^[[:space:]]*<VirtualHost[[:space:]]+\*:(80|443)>' "$VHOST_DIR"/*.conf 2>/dev/null; then
    drift=1
  fi

  [[ $drift -eq 0 ]] && { [[ $QUIET -eq 1 ]] || info "端口正常，无需处理"; return 0; }

  warn "检测到 Apache 端口漂移（多半是宝塔面板保存了站点设置），正在纠正"
  backup_apache
  patch_apache
  [[ $QUIET -eq 1 ]] || ok "已纠正并重启 Apache"
  logger -t gowaf "端口漂移已纠正" 2>/dev/null || true
}

# ---------------------------------------------------------------- 控制台临时外放
#
# 控制台默认只听 127.0.0.1，正规用法是 SSH 隧道：
#   ssh -L 8899:127.0.0.1:8899 root@服务器
# 想直接在浏览器里连，就要同时改动三样东西，少一样都连不上：
#   1. admin_listen    127.0.0.1 -> 0.0.0.0（并声明 i_know_what_i_am_doing）
#   2. allowed_ips     只留你当前出口 IP（默认取 SSH 客户端 IP，不用手打）
#   3. 主机防火墙      放行 8899（云厂商安全组还得你自己在控制台放行）
# 因为「临时」，所以顺带挂一个 systemd timer，到点自动改回回环并撤掉防火墙规则。

CONSOLE_ACTION=""
CONSOLE_IP=""
CONSOLE_HOURS=2
CONSOLE_UNIT="gowaf-console-close"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"   # 仅测试时覆盖，生产恒为 /etc/systemd/system
CONSOLE_STATE="${GW_HOME}/.console-open"

# 改写 admin: 段里的某个键（键不存在就插到该段末尾），保持缩进两格
yaml_admin_set() {
  local key=$1 line=$2 conf=$GW_CONF tmp
  [[ -f $conf ]] || die "配置不存在：$conf"
  grep -qE '^admin:[[:space:]]*$' "$conf" || die "配置里找不到 admin: 段，请先重新 install"
  tmp=$(mktemp)
  awk -v key="$key" -v line="$line" '
    /^admin:[[:space:]]*$/ { inadm = 1; print; next }
    inadm && /^[^[:space:]#]/ { if (!done) { print line; done = 1 } inadm = 0 }
    inadm && $0 ~ ("^[[:space:]]+" key ":") { if (!done) { print line; done = 1 } ; next }
    { print }
  ' "$conf" > "$tmp"
  mv "$tmp" "$conf"
}

console_listen_now() {
  awk '/^[[:space:]]*admin_listen:/{gsub(/[",]/,"",$2); print $2; exit}' "$GW_CONF" 2>/dev/null || true
}

# 服务器自身的出口地址：不需要联网，比 curl ipify 可靠
server_addr() {
  local a
  a=$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}') || true
  [[ -z $a ]] && a=$(hostname -I 2>/dev/null | awk '{print $1}') || true
  printf '%s' "${a:-<服务器IP>}"
}

fw_engine() {
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then
    printf 'ufw'
  elif systemctl is-active --quiet firewalld 2>/dev/null; then
    printf 'firewalld'
  elif command -v iptables >/dev/null 2>&1; then
    printf 'iptables'
  else
    printf 'none'
  fi
}

fw_allow() {  # port cidr
  local port=$1 cidr=$2
  case $(fw_engine) in
    ufw)
      ufw allow from "$cidr" to any port "$port" proto tcp >/dev/null 2>&1 || true ;;
    firewalld)
      if [[ $cidr == *:* ]]; then
        firewall-cmd -q --permanent --add-rich-rule="rule family=ipv6 source address=${cidr} port port=${port} protocol=tcp accept" >/dev/null 2>&1 || true
      else
        firewall-cmd -q --permanent --add-rich-rule="rule family=ipv4 source address=${cidr} port port=${port} protocol=tcp accept" >/dev/null 2>&1 || true
      fi
      firewall-cmd -q --reload >/dev/null 2>&1 || true ;;
    iptables)
      iptables -C INPUT -p tcp --dport "$port" -s "$cidr" -m comment --comment gowaf-console -j ACCEPT 2>/dev/null \
        || iptables -I INPUT 1 -p tcp --dport "$port" -s "$cidr" -m comment --comment gowaf-console -j ACCEPT 2>/dev/null \
        || warn "iptables 放行失败（$cidr -> $port），请手动放行" ;;
  esac
}

fw_deny() {  # port cidr
  local port=$1 cidr=$2
  case $(fw_engine) in
    ufw)
      ufw delete allow from "$cidr" to any port "$port" proto tcp >/dev/null 2>&1 || true ;;
    firewalld)
      if [[ $cidr == *:* ]]; then
        firewall-cmd -q --permanent --remove-rich-rule="rule family=ipv6 source address=${cidr} port port=${port} protocol=tcp accept" >/dev/null 2>&1 || true
      else
        firewall-cmd -q --permanent --remove-rich-rule="rule family=ipv4 source address=${cidr} port port=${port} protocol=tcp accept" >/dev/null 2>&1 || true
      fi
      firewall-cmd -q --reload >/dev/null 2>&1 || true ;;
    iptables)
      while iptables -C INPUT -p tcp --dport "$port" -s "$cidr" -m comment --comment gowaf-console -j ACCEPT 2>/dev/null; do
        iptables -D INPUT -p tcp --dport "$port" -s "$cidr" -m comment --comment gowaf-console -j ACCEPT 2>/dev/null || break
      done ;;
  esac
}

console_timer_stop() {
  systemctl stop "${CONSOLE_UNIT}.timer" >/dev/null 2>&1 || true
  systemctl disable "${CONSOLE_UNIT}.timer" >/dev/null 2>&1 || true
  rm -f "${UNIT_DIR}/${CONSOLE_UNIT}.timer" "${UNIT_DIR}/${CONSOLE_UNIT}.service"
  systemctl daemon-reload >/dev/null 2>&1 || true
}

console_timer_arm() {  # hours
  cat > "${UNIT_DIR}/${CONSOLE_UNIT}.service" <<EOF
[Unit]
Description=GoWAF 控制台临时外放到期收回

[Service]
Type=oneshot
ExecStart=/bin/bash ${GW_HOME}/bin/install.sh console close --force --quiet
EOF
  cat > "${UNIT_DIR}/${CONSOLE_UNIT}.timer" <<EOF
[Unit]
Description=${1} 小时后收回 GoWAF 控制台外放

[Timer]
OnActiveSec=${1}h
AccuracySec=1min
Unit=${CONSOLE_UNIT}.service

[Install]
WantedBy=timers.target
EOF
  systemctl daemon-reload
  systemctl start "${CONSOLE_UNIT}.timer" >/dev/null 2>&1 || warn "定时收回启动失败，记得手动 console close"
}

normalize_src() {  # 1.2.3.4 -> 1.2.3.4/32 ；::1 -> ::1/128 ；已带掩码原样
  local s=$1
  if [[ $s == */* ]]; then printf '%s' "$s"; return; fi
  if [[ $s == *:* ]]; then printf '%s/128' "$s"; else printf '%s/32' "$s"; fi
}

console_open() {
  [[ -f $GW_CONF ]] || die "还没安装：${GW_CONF} 不存在"
  local -a srcs=()
  local ssh_ip="" s yaml_list

  ssh_ip=$(awk '{print $1}' <<<"${SSH_CLIENT:-}")

  if [[ -z $CONSOLE_IP ]]; then
    if [[ -n $ssh_ip ]]; then
      srcs=("$(normalize_src "$ssh_ip")")
      info "未指定 --ip，默认只放行你当前的 SSH 来源：${srcs[0]}"
    else
      srcs=("0.0.0.0/0" "::/0")
      warn "拿不到 SSH 来源 IP，退化为对全网放行——强烈建议改用 --ip 你的公网IP"
    fi
  elif [[ $CONSOLE_IP == all || $CONSOLE_IP == "0.0.0.0/0" ]]; then
    srcs=("0.0.0.0/0" "::/0")
  else
    for s in ${CONSOLE_IP//,/ }; do srcs+=("$(normalize_src "$s")"); done
  fi

  # 只在「从回环改为对外开放」时留一份原始配置；重复 open 不要覆盖它，
  # 否则 close 出问题时手上就没有干净的回环配置可退。
  if [[ $(console_listen_now) != "0.0.0.0:${ADMIN_PORT}" ]]; then
    cp -p "$GW_CONF" "${GW_CONF}.console.bak"
  fi

  sed -i -E "s#^([[:space:]]*admin_listen:[[:space:]]*).*#\1\"0.0.0.0:${ADMIN_PORT}\"#" "$GW_CONF"
  yaml_admin_set i_know_what_i_am_doing "  i_know_what_i_am_doing: true   # console open 临时打开，close 时移除"

  yaml_list=""
  yaml_list="\"127.0.0.1/32\""
  for s in "${srcs[@]}"; do yaml_list+=", \"${s}\""; done
  yaml_admin_set allowed_ips "  allowed_ips: [${yaml_list}]"

  if ! "$GW_BIN" -t -c "$GW_CONF" >/dev/null 2>&1; then
    "$GW_BIN" -t -c "$GW_CONF" || true
    cp -p "${GW_CONF}.console.bak" "$GW_CONF"
    die "配置校验失败，已还原（未重启服务）"
  fi

  systemctl restart gowaf
  sleep 1
  systemctl is-active --quiet gowaf || die "gowaf 启动失败：journalctl -u gowaf -n 50"

  local code
  code=$(curl -s -o /dev/null -m 5 -w '%{http_code}' "http://127.0.0.1:${ADMIN_PORT}/healthz" 2>/dev/null) || true
  [[ ${code:-000} == 200 ]] || warn "控制面自检返回 ${code:-000}（端口没起来？看 journalctl -u gowaf -n 50）"

  for s in "${srcs[@]}"; do fw_allow "$ADMIN_PORT" "$s"; done

  printf '%s\n' "${srcs[@]}" > "$CONSOLE_STATE"
  console_timer_arm "$CONSOLE_HOURS"

  printf '\n%s控制台已临时对外开放%s\n' "$c_grn" "$c_off"
  printf '  地址      http://%s:%s\n' "$(server_addr)" "$ADMIN_PORT"
  printf '  放行来源  %s\n' "${srcs[*]}"
  printf '  防火墙    %s（云厂商安全组还要在控制台放行 %s）\n' "$(fw_engine)" "$ADMIN_PORT"
  printf '  自动收回  %s 小时后（systemd timer: %s）\n' "$CONSOLE_HOURS" "$CONSOLE_UNIT"
  printf '  手动收回  %s console close\n' "${GW_HOME}/bin/install.sh"
  printf '  登录账号  admin（密码见 %s/data/initial-password.txt，改过就用新密码）\n' "$GW_HOME"
  printf '\n%s注意：控制台现在是以 http 裸奔在公网上，密码错误 5 次会锁 15 分钟。\n' "$c_yel"
  printf '长期方案用 Cloudflare Tunnel 或 SSH 隧道，别长期开着。%s\n' "$c_off"
}

console_close() {
  [[ -f $GW_CONF ]] || return 0
  local -a srcs=()
  local s is_open=0

  if [[ -f $CONSOLE_STATE ]]; then
    while IFS= read -r s; do [[ -n $s ]] && srcs+=("$s"); done < "$CONSOLE_STATE"
  fi
  # 状态文件丢了也要能收回：兜底撤掉全网放行规则
  [[ ${#srcs[@]} -eq 0 ]] && srcs=("0.0.0.0/0" "::/0")

  for s in "${srcs[@]}"; do fw_deny "$ADMIN_PORT" "$s"; done

  # 注意：这里必须用 if，不能写 [[ ]] && is_open=1 ——
  # 条件不成立时整个 && 列表返回 1，在 set -e 下会静默退出（本项目已踩过这个坑）
  if [[ $(console_listen_now) == "0.0.0.0:${ADMIN_PORT}" ]]; then is_open=1; fi

  if [[ $is_open -eq 1 ]]; then
    sed -i -E "s#^([[:space:]]*admin_listen:[[:space:]]*).*#\1\"127.0.0.1:${ADMIN_PORT}\"#" "$GW_CONF"
    yaml_admin_set allowed_ips "  allowed_ips: [\"127.0.0.1/32\"]"
    # 把 i_know_what_i_am_doing 那行删掉（连同注释），避免长期留着这个「知情开关」
    sed -i -E '/^[[:space:]]*i_know_what_i_am_doing:/d' "$GW_CONF"

    if "$GW_BIN" -t -c "$GW_CONF" >/dev/null 2>&1; then
      # gowaf 可能本来就没在跑（比如已回滚、Apache 已收回 80/443）。
      # 此时绝不能 systemctl restart：绑定失败 + Restart=always 会陷入崩溃循环。
      if systemctl is-active --quiet gowaf; then
        systemctl restart gowaf
        sleep 1
        if systemctl is-active --quiet gowaf; then
          ok "控制台已收回 127.0.0.1:${ADMIN_PORT}（现在只能用 SSH 隧道访问）"
        else
          warn "gowaf 重启失败，尝试用备份配置恢复"
          if [[ -f ${GW_CONF}.console.bak ]]; then
            cp -p "${GW_CONF}.console.bak" "$GW_CONF"
            systemctl restart gowaf || true
          fi
        fi
      else
        ok "控制台已收回（gowaf 当前未运行，配置已改好，下次启动即生效）"
      fi
    else
      warn "收回后的配置校验不通过，保留原样（未重启），请手动检查 ${GW_CONF}"
    fi
  else
    ok "控制台本来就只听回环，只清理了防火墙规则"
  fi

  console_timer_stop
  rm -f "$CONSOLE_STATE"
}

console_status() {
  [[ -f $GW_CONF ]] || die "还没安装：${GW_CONF} 不存在"
  info "监听地址：$(console_listen_now)"
  info "允许来源：$(awk '/^[[:space:]]*allowed_ips:/{print; exit}' "$GW_CONF" | sed 's/^[[:space:]]*//')"
  info "防火墙引擎：$(fw_engine)"
  if port_listening "$ADMIN_PORT"; then
    info "端口 ${ADMIN_PORT}：监听中"
    port_holder "$ADMIN_PORT" || true
  else
    info "端口 ${ADMIN_PORT}：未监听"
  fi
  if systemctl list-timers --all 2>/dev/null | grep -q "$CONSOLE_UNIT"; then
    info "自动收回：$(systemctl list-timers --all 2>/dev/null | grep "$CONSOLE_UNIT" | sed 's/^[[:space:]]*//' | awk '{print $1, $2, $3}')"
  else
    info "自动收回：未启用"
  fi
  if [[ $(console_listen_now) == 127.0.0.1:* ]]; then
    ok "当前处于安全状态（仅回环）"
  else
    warn "当前控制台对公网开放，用完请执行：$0 console close"
  fi
}

cmd_console() {
  need_root
  case ${CONSOLE_ACTION:-status} in
    open|on)    console_open ;;
    close|off)  console_close ;;
    status)     console_status ;;
    *)          die "用法：$0 console open [--ip 1.2.3.4|all] [--hours 2] | close | status" ;;
  esac
}

# ---------------------------------------------------------------- 主流程

do_install() {
  need_root
  find_binary
  detect_env

  local TAKE443_LABEL="" TAKE443_LABEL2=""
  if [[ $TAKE_443 -eq 1 ]]; then
    TAKE443_LABEL="/443"
    TAKE443_LABEL2="，并禁用 SSL vhost（TLS 转由 WAF 终止）"
  fi

  if [[ -f /etc/systemd/system/gowaf.service ]]; then
    if [[ $FORCE -eq 0 ]]; then
      printf '\n检测到已安装 gowaf。继续将重新生成配置并重启服务。\n'
      confirm '继续？'
    fi
  else
    printf '\n即将执行以下动作：\n'
    printf '  1. 备份 %s 与全部 vhost 到 %s\n' "$APACHE_CONF" "$GW_BACKUP"
    printf '  2. 注释 Apache 的 Listen 80%s，改为监听 127.0.0.1:%s\n' \
      "$TAKE443_LABEL" "$UPSTREAM_PORT"
    printf '  3. 改写各站点 vhost 端口%s\n' "$TAKE443_LABEL2"
    printf '  4. 安装 gowaf.service，以 observe 模式（只记录不拦截）启动\n'
    printf '  （任一环节失败会自动回滚 Apache 配置，站点不会长时间中断）\n'
    confirm '继续？'
  fi

  # 替换二进制不能直接覆盖正在运行的文件（Linux 会报 ETXTBSY）。
  # 但也别为此提前停服务：把旧文件改名，老进程继续用旧 inode 服务 80，站点零中断；
  # 新二进制落到原路径，等最后一次性 restart 切过去（中断只剩下一秒级）。
  if systemctl is-active --quiet gowaf && [[ -f $GW_BIN ]]; then
    step "准备替换二进制（老进程继续服务，站点不中断）"
    mv -f "$GW_BIN" "${GW_BIN}.prev" 2>/dev/null || true
  fi

  install_files
  install_cli

  # 重装/升级不偷偷改模式：已经是 protect 就保持 protect
  local mode="observe"
  if [[ -f $GW_CONF ]]; then
    local old_mode
    old_mode=$(awk '/^  mode:/{gsub(/"/, "", $2); print $2; exit}' "$GW_CONF" 2>/dev/null || true)
    [[ $old_mode == "protect" ]] && mode="protect"
  fi
  if [[ $CF_MODE -eq 1 ]]; then
    touch "${GW_HOME}/.cloudflare"
  fi
  render_config "$mode"

  backup_apache
  patch_apache
  install_systemd

  step "切换到新二进制（约 1~3 秒无法访问）"
  # 到这里才停：停 → 让出 80 → 立刻起。中断窗口从「整段安装」压到「一次重启」
  systemctl stop gowaf 2>/dev/null || true
  ensure_port_free 80
  if [[ $TAKE_443 -eq 1 ]]; then
    ensure_port_free 443
  fi
  systemctl start gowaf
  sleep 1
  if ! systemctl is-active --quiet gowaf; then
    journalctl -u gowaf -n 30 --no-pager >&2 || true
    warn "gowaf 启动失败，正在回滚 Apache 以免站点中断"
    abort_install "已回滚。请把上面的日志发给开发者"
  fi
  ok "gowaf 运行中"
  rm -f "${GW_BIN}.prev"
  # 上次失败回滚会留下这个标记（resync 看到它就不动作），安装成功必须清掉，
  # 否则宝塔改回 vhost 端口后自愈定时器永远不会生效。
  rm -f "$GW_SYNC_OFF"

  healthcheck
  print_summary
}

healthcheck() {
  step "连通性自检"
  local code host tls_host
  # 自检用的 Host 必须是「真实域名」：default_cert 的目录名可能是 112.5.37.34 这类站点名，
  # 拿它做 SNI 会匹配不到任何证书，反而误报失败。
  host=${SELFCHECK_HOST:-}
  [[ -z $host ]] && host=${DEFAULT_DOMAIN:-}
  [[ -z $host ]] && host=$(awk '/^  - name:/{print $3; exit}' "$GW_CONF" | tr -d '"')

  if [[ -n $host ]]; then
    # curl -w 在连接失败时自己也输出 000，所以不能再 || echo 000（会拼成 000000）
    code=$(curl -s -o /dev/null -m 8 -w '%{http_code}' -H "Host: ${host}" http://127.0.0.1/ 2>/dev/null) || true
    code=${code:-000}
    case $code in
      000|502) warn "HTTP 自检返回 ${code}（Host: ${host}）—— 检查 Apache 是否在 ${UPSTREAM_PORT} 上、站点是否正常" ;;
      *)       ok "HTTP 自检 ${code}（Host: ${host}）" ;;
    esac
  fi

  if [[ $TAKE_443 -eq 1 && -n $host ]]; then
    # 去掉 -k：要验证的正是「这张证书能不能配上这个域名」
    tls_host=$host
    code=$(curl -s -o /dev/null -m 8 -w '%{http_code}' --resolve "${tls_host}:443:127.0.0.1" "https://${tls_host}/" 2>/dev/null) || true
    code=${code:-000}
    case $code in
      000) warn "HTTPS 自检失败（SNI: ${tls_host}）—— 没有证书覆盖这个域名，或 WAF 未在 443 监听" ;;
      502) warn "HTTPS 自检 502（SNI: ${tls_host}）—— TLS 已通，但 Apache 上游异常" ;;
      *)   ok "HTTPS 自检 ${code}（SNI: ${tls_host}，证书校验通过）" ;;
    esac
  fi
}

print_summary() {
  local status80="接管" status443="未接管（HTTPS 直连 Apache，不经过 WAF）"
  if [[ $TAKE_443 -eq 1 ]]; then
    status443="接管（TLS 由 WAF 终止，证书取自宝塔）"
  fi
  local cur_mode
  cur_mode=$(awk '/^  mode:/{gsub(/"/, "", $2); print $2; exit}' "$GW_CONF" 2>/dev/null || echo observe)
  local mode_desc="observe —— 只记录，不拦截"
  [[ $cur_mode == "protect" ]] && mode_desc="protect —— 命中即拦截"

  printf '\n%s================================================%s\n' "$c_grn" "$c_off"
  printf ' GoWAF 部署完成\n'
  printf '%s================================================%s\n' "$c_grn" "$c_off"
  printf ' 数据面   80 %s / 443 %s\n' "$status80" "$status443"
  printf ' 后端     127.0.0.1:%s（Apache）\n' "$UPSTREAM_PORT"
  printf ' 控制台   127.0.0.1:%s（仅回环）\n' "$ADMIN_PORT"
  printf ' 模式     %s\n' "$mode_desc"
  printf ' 规则     %s 条\n' "$(grep -c '^- id:' "$GW_RULES" 2>/dev/null || echo '?')"

  if [[ -f "${GW_HOME}/data/initial-password.txt" ]]; then
    printf '\n 初始账号 admin / %s\n' "$(cat "${GW_HOME}/data/initial-password.txt" 2>/dev/null)"
    printf ' （保存在 %s/data/initial-password.txt，登录改密后请删除）\n' "$GW_HOME"
  fi

  cat <<EOF

 访问控制台（本机不开外网端口，用 SSH 隧道）：
   ssh -L ${ADMIN_PORT}:127.0.0.1:${ADMIN_PORT} root@<服务器IP>
   浏览器打开 http://127.0.0.1:${ADMIN_PORT}

 观察 3~7 天后确认没有误杀，再切实时拦截：
   gowaf protect

 宝塔面板里如果再点「保存」站点设置，Apache 端口会被改回 80，
 定时器会自动纠正；也可以手动：gowaf resync

 出问题需要立刻恢复原状：
   gowaf rollback

EOF
}

do_rollback() {
  need_root
  local bd
  bd=$(latest_backup) || die "没有可用备份，无法回滚"

  # 必须先 detect_env：不跑的话 $APACHE_CONF / $VHOST_DIR / $APACHE_CTL 全是空，
  # restore_apache_from 里的 cp 因目标为空失败，又被 || true 静默吞掉，
  # 最后 apache_test 用空命令报 ': command not found' —— 配置没还原还报"配置有错"。
  # （2026-11-03 线上事故根因：rollback 只停了 gowaf，还原一步都没执行。）
  detect_env

  # 备份时写下的 meta.env 记录了当时的真实路径与服务名，优先用它覆盖
  # detect_env 的推断值：即使 Apache 服务名/目录后来变过，还原仍按部署时原样进行。
  local meta="${bd}/meta.env"
  if [[ -f $meta ]]; then
    source "$meta"
  else
    warn "备份里没有 meta.env，按当前探测结果还原"
  fi

  step "回滚：还原 Apache 并停用 gowaf"
  systemctl disable --now gowaf 2>/dev/null || true
  systemctl disable --now gowaf-resync.timer 2>/dev/null || true
  touch "$GW_SYNC_OFF"

  # 先还原配置，Apache 才能重新拿回 80/443；gowaf 已停，端口是空的
  restore_apache_from "$bd"

  ok "已还原到部署前状态（备份 ${bd}）"
  warn "gowaf 服务与文件仍在，只是不运行。彻底清除用：gowaf uninstall"
}

do_uninstall() {
  need_root
  confirm '将删除服务、配置、数据库与日志（不可恢复）。确认？'
  do_rollback || true
  systemctl disable --now gowaf 2>/dev/null || true
  rm -f /etc/systemd/system/gowaf.service /etc/systemd/system/gowaf-resync.service \
        /etc/systemd/system/gowaf-resync.timer /etc/logrotate.d/gowaf
  systemctl daemon-reload
  rm -rf "$GW_HOME"
  rm -f /usr/local/bin/gowaf
  ok "已彻底卸载"
}

do_doctor() {
  need_root
  detect_env

  # ---------------- 当前部署形态：直接回答「有没有用 --http-only」
  step "当前部署形态（只读）"
  local installed=0 tls_on=0 holders=""
  # 一律用 if，不用 [[ ]] && x=1：条件不成立时 && 列表返回 1，
  # 在 set -e 下有静默退出的风险（本项目已踩过，见 cmd_console 里的注释）。
  if [[ -f $GW_CONF || -x $GW_BIN ]]; then installed=1; fi
  if gw_tls_enabled; then tls_on=1; fi

  if (( installed == 0 )); then
    info "尚未部署：${GW_HOME} 下没有 gowaf（本次 install 会首次安装）"
    if [[ -n $CERT_NAMES ]]; then
      info "按当前证书情况，install 会接管 80 + 443（HTTPS 也经过 WAF）"
    else
      info "证书目录为空，install 只会接管 80（HTTPS 直连 Apache，不过 WAF）"
    fi
  else
    info "已安装：${GW_CONF}"
    if (( tls_on == 1 )); then
      ok "形态：全接管 —— 80 与 443 都经过 WAF"
    else
      warn "形态：仅接管 80 —— 443 不经过 WAF（HTTPS 流量 WAF 看不到）"
      if [[ -n $CERT_NAMES ]]; then
        info "证书是有的，但配置里 tls.enabled=false → 当初用了 --http-only，或装完后手动关掉了 443"
        info "要让 HTTPS 也进 WAF：去掉 --http-only 重跑 install，或 reference docs 改 config 后 restart"
      else
        info "证书目录为空 → 装的时候就没法接管 443，不是 --http-only 的问题"
      fi
    fi

    # 实际监听情况，用于交叉验证配置（配置说接管、实际没监听 = 进程挂了或换过配置没重启）
    local p
    for p in 80 443; do
      if port_held_by_gowaf "$p"; then
        holders+="${p}=gowaf "
      elif port_held_by_apache "$p"; then
        holders+="${p}=apache "
      elif port_listening "$p"; then
        holders+="${p}=其他进程 "
      else
        holders+="${p}=无监听 "
      fi
    done
    info "实际监听：${holders% }"
    if (( tls_on == 1 )) && ! port_held_by_gowaf 443; then
      warn "配置要求接管 443，但 443 上没有 gowaf → 服务未运行或启动失败，先看 install.sh logs"
    fi
  fi

  step "证书与站点域名（只读）"
  local d f names line mark
  if [[ -n $CERT_NAMES ]]; then
    info "宝塔证书目录 ${CERT_DIR}（★ 为 default_cert，即 SNI 对不上时的兜底）："
    printf '        %-2s %-20s %-44s %s\n' "" "目录" "证书覆盖的域名（SAN）" "命中 vhost"
    while IFS='|' read -r d names line; do
      [[ -n $d ]] || continue
      mark=" "
      [[ $d == "$DEFAULT_DOMAIN" ]] && mark="★"
      printf '        %-2s %-20s %-44s %s 个\n' "$mark" "$d" "${names:-（无 SAN）}" "$line"
    done <<<"$CERT_SANS_INFO"
  else
    info "证书目录里没有可用证书（需要 fullchain.pem + privkey.pem）：${CERT_DIR}"
  fi
  info "各 vhost 的 ServerName / ServerAlias："
  for f in "$VHOST_DIR"/*.conf; do
    [[ -f $f ]] || continue
    # 注意：grep 无匹配会返回 1，在 set -Eeuo pipefail 下会让整个脚本静默退出
    # （0.site_total_log_format.conf 这类非 vhost 文件就会触发），必须 || true
    names=$(grep -hE '^[[:space:]]*Server(Name|Alias)' "$f" 2>/dev/null \
            | sed -E 's/^[[:space:]]*//' | tr '\n' ' ' || true)
    [[ -n ${names// /} ]] || continue
    printf '      %-34s %s\n' "$(basename "$f")" "$names"
  done
  if [[ -n $MISSING_NAMES ]]; then
    warn "没有任何证书覆盖的域名：${MISSING_NAMES}"
    warn "经 Cloudflare 回源时 SNI 是域名，对不上任何证书 → Full (strict) 会报 526"
  elif [[ -n $COVERED_NAMES ]]; then
    ok "vhost 域名全部有证书覆盖：${COVERED_NAMES}"
  fi

  # 站点 → 具体哪张证书。直接回答「服务器上到底几个站点、每个站点用哪张证书」，
  # 省得对着上面两张表自己配对。
  local vh hit all_total real_total
  all_total=$(printf '%s\n' $VHOST_NAMES | grep -c . || true)
  # 先滤掉空行再统计：VHOST_NAMES 为空时 printf 会吐一个空行，
  # 不滤掉的话「真实站点数」会算成 1。
  real_total=$(printf '%s\n' $VHOST_NAMES | grep -v '^[[:space:]]*$' \
    | grep -cEv '^(bt\.default\.com|localhost|.*\.local|ssl\.)' || true)
  step "站点清单（只读）"
  if (( all_total == 0 )); then
    info "没有找到任何 vhost 域名（${VHOST_DIR}）"
  else
    info "共 ${all_total} 个 vhost 域名，其中真实站点 ${real_total} 个："
    printf '      %-34s %-24s %s\n' "域名" "证书目录" "备注"
    for vh in $VHOST_NAMES; do
      case $vh in
        bt.default.com|localhost|*.local|ssl.*)
          printf '      %-34s %-24s %s\n' "$vh" "-" "占位域名，忽略"
          continue ;;
      esac
      [[ $vh =~ ^[0-9a-f]{8}\. ]] && { printf '      %-34s %-24s %s\n' "$vh" "-" "宝塔自动生成，忽略"; continue; }
      if [[ $vh =~ ^[0-9.]+$ ]]; then
        printf '      %-34s %-24s %s\n' "$vh" "-" "纯 IP 访问，无 SNI"
        continue
      fi
      hit=""
      while IFS='|' read -r d names line; do
        [[ -n $d ]] || continue
        name_covered "$vh" "$names" && { hit=$d; break; }
      done <<<"$CERT_SANS_INFO"
      if [[ -n $hit ]]; then
        if [[ $hit == "$DEFAULT_DOMAIN" ]]; then
          printf '      %-34s %-24s %s\n' "$vh" "$hit" "★ default_cert"
        else
          printf '      %-34s %-24s %s\n' "$vh" "$hit" "SNI 精确匹配"
        fi
      else
        printf '      %-34s %-24s %s\n' "$vh" "-" "无证书覆盖"
      fi
    done
  fi

  step "端口占用（只读）"
  local p
  for p in 80 443 "$UPSTREAM_PORT" "$ADMIN_PORT"; do
    if port_listening "$p"; then
      printf '      %-5s 监听中\n' "$p"
      port_holder "$p" || true
    else
      printf '      %-5s 空闲\n' "$p"
    fi
  done
  if port_listening 80 && [[ ! -f $GW_CONF ]]; then
    warn "80 现在由 Apache 占用是正常的（还没部署）；install 会先让 Apache 停到 ${UPSTREAM_PORT}"
  fi

  step "只读体检"
  apache_test && ok "Apache 配置语法正常"
  if systemctl is-active --quiet gowaf; then ok "gowaf 运行中"; else warn "gowaf 未运行"; fi
  [[ -f $GW_CONF ]] && "$GW_BIN" -t -c "$GW_CONF" >/dev/null 2>&1 && ok "gowaf 配置校验通过"
  healthcheck
}

usage() {
  sed -n '/^# 用法：/,/^#   --quiet/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

main() {
  local cmd="install"
  if [[ $# -gt 0 && $1 != --* ]]; then
    cmd=$1
    shift
  fi
  if [[ $cmd == "cloudflare" && $# -gt 0 && $1 != --* ]]; then
    CF_ACTION=$1
    shift
  fi
  if [[ $cmd == "console" && $# -gt 0 && $1 != --* ]]; then
    CONSOLE_ACTION=$1
    shift
  fi

  while [[ $# -gt 0 ]]; do
    case $1 in
      --binary)        BINARY_SRC=$2; shift 2 ;;
      --rules)         RULES_SRC=$2; shift 2 ;;
      --upstream-port) UPSTREAM_PORT=$2; shift 2 ;;
      --ip)            CONSOLE_IP=$2; shift 2 ;;
      --hours)         CONSOLE_HOURS=$2; shift 2 ;;
      --http-only)     HTTP_ONLY=1; shift ;;
      --cloudflare)    CF_MODE=1; shift ;;
      --force)         FORCE=1; shift ;;
      --quiet)         QUIET=1; shift ;;
      -h|--help)       usage; exit 0 ;;
      *)               die "未知参数：$1（--help 查看用法）" ;;
    esac
  done

  case $cmd in
    install)   do_install ;;
    start)     svc_start ;;
    stop)      svc_stop ;;
    restart)   svc_restart ;;
    status)    svc_status ;;
    logs)      journalctl -u gowaf -f -n 100 --no-pager ;;
    observe)   switch_mode observe ;;
    protect)   switch_mode protect ;;
    cloudflare) cmd_cloudflare ;;
    console)   cmd_console ;;
    resync)    cmd_resync ;;
    rollback)  do_rollback ;;
    uninstall) do_uninstall ;;
    doctor)    do_doctor ;;
    *)         die "未知命令：$cmd（--help 查看用法）" ;;
  esac
}

main "$@"
