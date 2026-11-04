#!/usr/bin/env bash
# Cloud Agent 实验室的 systemctl / journalctl 替身。
# 本镜像 PID 1 是 tini，没有 systemd dbus；install.sh 仍按生产路径调用 systemctl。
set -euo pipefail
STATEDIR=/run/lab-systemd
mkdir -p "$STATEDIR" /www/server/gowaf/logs 2>/dev/null || true

unit_base() {
  local u=$1
  u=${u#*/}                 # foo.service -> already
  u=${u%.service}
  u=${u%.timer}
  printf '%s' "$u"
}

is_running() {
  local u pidfile
  u=$(unit_base "$1")
  case $u in
    nginx)
      for pidfile in /run/nginx.pid \
                     /var/run/nginx.pid \
                     /www/server/nginx/logs/nginx.pid \
                     /usr/local/phpstudy/soft/nginx/*/logs/nginx.pid; do
        [[ -f $pidfile ]] || continue
        kill -0 "$(cat "$pidfile")" 2>/dev/null && return 0
      done
      return 1 ;;
    httpd|apache2)
      for pidfile in /www/server/apache/logs/httpd.pid \
                     /usr/local/phpstudy/soft/apache/*/logs/httpd.pid \
                     /run/apache2/apache2.pid \
                     /var/run/apache2/apache2.pid; do
        [[ -f $pidfile ]] || continue
        kill -0 "$(cat "$pidfile")" 2>/dev/null && return 0
      done
      return 1 ;;
    gowaf)
      pidfile=/run/gowaf.pid
      [[ -f $pidfile ]] && kill -0 "$(cat "$pidfile")" 2>/dev/null ;;
    gowaf-resync)
      pidfile=/run/gowaf-resync.pid
      [[ -f $pidfile ]] && kill -0 "$(cat "$pidfile")" 2>/dev/null ;;
    *) return 1 ;;
  esac
}

start_unit() {
  local u pidfile
  u=$(unit_base "$1")
  case $u in
    nginx)
      /etc/init.d/nginx start
      ;;
    httpd|apache2)
      if [[ -x /etc/init.d/httpd ]]; then
        /etc/init.d/httpd start
      elif [[ -x /etc/init.d/apache2 ]]; then
        /etc/init.d/apache2 start
      fi
      ;;
    gowaf)
      local bin=/www/server/gowaf/bin/gowaf
      local conf=/www/server/gowaf/configs/config.yaml
      [[ -x $bin && -f $conf ]] || return 1
      "$bin" -t -c "$conf" >/dev/null
      if is_running gowaf; then return 0; fi
      nohup "$bin" -c "$conf" >>/www/server/gowaf/logs/service.log 2>&1 &
      echo $! > /run/gowaf.pid
      sleep 0.4
      is_running gowaf
      ;;
    gowaf-resync)
      if is_running gowaf-resync; then return 0; fi
      nohup bash -c 'while true; do /usr/local/bin/gowaf resync --quiet >/dev/null 2>&1 || true; sleep 60; done' \
        >>/www/server/gowaf/logs/resync.log 2>&1 &
      echo $! > /run/gowaf-resync.pid
      ;;
    *) return 0 ;;
  esac
}

stop_unit() {
  local u pidfile
  u=$(unit_base "$1")
  case $u in
    nginx) /etc/init.d/nginx stop || true ;;
    httpd|apache2)
      if [[ -x /etc/init.d/httpd ]]; then
        /etc/init.d/httpd stop || true
      elif [[ -x /etc/init.d/apache2 ]]; then
        /etc/init.d/apache2 stop || true
      fi
      pkill -x apache2 2>/dev/null || true
      pkill -x httpd 2>/dev/null || true
      ;;
    gowaf)
      pidfile=/run/gowaf.pid
      if [[ -f $pidfile ]] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
        kill "$(cat "$pidfile")" 2>/dev/null || true
        for _ in 1 2 3 4 5 6 7 8 9 10; do
          kill -0 "$(cat "$pidfile")" 2>/dev/null || break
          sleep 0.2
        done
        kill -9 "$(cat "$pidfile")" 2>/dev/null || true
      fi
      pkill -f '/www/server/gowaf/bin/gowaf' 2>/dev/null || true
      rm -f "$pidfile"
      ;;
    gowaf-resync)
      pidfile=/run/gowaf-resync.pid
      if [[ -f $pidfile ]]; then
        kill "$(cat "$pidfile")" 2>/dev/null || true
        rm -f "$pidfile"
      fi
      ;;
  esac
  return 0
}

# 丢掉 journalctl / systemctl 的装饰参数，剩下 action + units
ARGS=()
SKIP=0
PROG=$(basename "$0")
for a in "$@"; do
  if (( SKIP )); then SKIP=0; continue; fi
  case $a in
    --quiet|-q|--no-pager|--now|--all|--user|--system|--full|--no-legend|--plain) continue ;;
    -n|--lines|-o|--output|--type|--property|-p) SKIP=1; continue ;;
    --lines=*|--output=*|--type=*|--property=*) continue ;;
    *) ARGS+=("$a") ;;
  esac
done

if [[ $PROG == journalctl ]]; then
  unit=""
  n=50
  follow=0
  set -- "$@"
  while [[ $# -gt 0 ]]; do
    case $1 in
      -u|--unit) unit=$2; shift 2 ;;
      -n) n=$2; shift 2 ;;
      -f|--follow) follow=1; shift ;;
      --no-pager) shift ;;
      *) shift ;;
    esac
  done
  log=/www/server/gowaf/logs/service.log
  [[ -f $log ]] || { echo "(no gowaf service log)"; exit 0; }
  if [[ $follow -eq 1 ]]; then
    tail -n "$n" -f "$log"
  else
    tail -n "$n" "$log"
  fi
  exit 0
fi

ACTION=${ARGS[0]:-}
UNITS=("${ARGS[@]:1}")

case $ACTION in
  daemon-reload) exit 0 ;;
  list-unit-files)
    # 空参数：给一个最小列表；带参数：该 unit 存在即打印
    if [[ ${#UNITS[@]} -eq 0 ]]; then
      printf 'nginx.service\nenabled\nhttpd.service\nenabled\napache2.service\nenabled\ngowaf.service\nenabled\n'
      exit 0
    fi
    for u in "${UNITS[@]}"; do
      case $(unit_base "$u") in
        nginx|httpd|apache2|gowaf|gowaf-resync) printf '%s\tenabled\n' "$u" ;;
        *) exit 1 ;;
      esac
    done
    exit 0
    ;;
  list-timers)
    if is_running gowaf-resync; then
      echo "gowaf-console-close.timer  (lab shim)"
    fi
    exit 0
    ;;
  is-active)
    [[ ${#UNITS[@]} -ge 1 ]] || exit 1
    if is_running "${UNITS[0]}"; then
      [[ ${QUIET:-0} -eq 1 ]] || echo active
      exit 0
    fi
    exit 3
    ;;
  is-enabled)
    exit 0
    ;;
  enable)
    # --now 已被剥掉，若原 argv 含 --now 则同时 start
    for a in "$@"; do [[ $a == --now ]] && NOW=1; done
    if [[ ${NOW:-0} -eq 1 ]]; then
      for u in "${UNITS[@]}"; do start_unit "$u" || true; done
    fi
    exit 0
    ;;
  disable)
    for a in "$@"; do [[ $a == --now ]] && NOW=1; done
    if [[ ${NOW:-0} -eq 1 ]]; then
      for u in "${UNITS[@]}"; do stop_unit "$u"; done
    fi
    exit 0
    ;;
  start)
    for u in "${UNITS[@]}"; do start_unit "$u"; done
    ;;
  stop)
    for u in "${UNITS[@]}"; do stop_unit "$u"; done
    ;;
  restart)
    for u in "${UNITS[@]}"; do stop_unit "$u"; start_unit "$u"; done
    ;;
  reload)
    for u in "${UNITS[@]}"; do
      case $(unit_base "$u") in
        nginx) /etc/init.d/nginx reload || true ;;
        httpd|apache2)
          if [[ -x /etc/init.d/httpd ]]; then /etc/init.d/httpd reload || true
          elif [[ -x /etc/init.d/apache2 ]]; then /etc/init.d/apache2 reload || true
          fi
          ;;
        gowaf) stop_unit gowaf; start_unit gowaf ;;
      esac
    done
    ;;
  status)
    u=${UNITS[0]:-gowaf}
    if is_running "$u"; then
      echo "${u}.service - lab shim (active)"
      exit 0
    fi
    echo "${u}.service - lab shim (inactive)"
    exit 3
    ;;
  *)
    # 未知子命令当成功，避免挡住 install
    exit 0
    ;;
esac
