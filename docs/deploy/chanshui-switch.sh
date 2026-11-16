#!/usr/bin/env bash
# Run on the server after the new blue/green instance is healthy.
# Usage: chanshui-switch.sh <new_port> <new_unit> <old_unit>
set -euo pipefail

new_port=${1:?new port}
new_unit=${2:?new unit}
old_unit=${3:?old unit}
case "$new_unit" in *@*) ;; *) new_unit="chanshui-bluegreen@$new_unit" ;; esac
case "$old_unit" in *@*) ;; *) old_unit="chanshui-bluegreen@$old_unit" ;; esac
upstream=/etc/nginx/conf.d/chanshui-upstream.conf
backup="/etc/nginx/conf.d/chanshui-upstream.conf.bak-$(date -u +%Y%m%dT%H%M%SZ)"

# Keep every hashed asset ever shipped. A tab opened before this deploy still
# asks for its old chunk names; deleting them turns the page into 404s.
unit=${new_unit#chanshui-bluegreen@}
env_file="/etc/chanshui/bluegreen-${unit}.env"
if [ -f "$env_file" ]; then
  new_root=$(sed -n 's/^CHANSHUI_ROOT=//p' "$env_file" | tail -n1)
  if [ -n "$new_root" ] && [ -d "$new_root/web/_next/static" ]; then
    for old in /opt/chanshui-*/web/_next/static; do
      [ "$old" = "$new_root/web/_next/static" ] && continue
      [ -d "$old" ] || continue
      cp -an "$old/." "$new_root/web/_next/static/" 2>/dev/null || true
    done
    chown -R chanshui:chanshui "$new_root/web/_next/static"
  fi
fi

curl -fsS "http://127.0.0.1:${new_port}/api/ready" >/dev/null
old_port=$(awk '/server 127.0.0.1:/ {gsub(/;/, "", $2); split($2, a, ":"); print a[2]}' "$upstream" | head -n1)
test -n "$old_port"
test "$old_port" != "$new_port"

cp -a "$upstream" "$backup"
sed -i "s/server 127.0.0.1:${old_port};/server 127.0.0.1:${new_port};/" "$upstream"
if ! nginx -t; then
  cp -a "$backup" "$upstream"
  echo "NGINX_RESTORED=$backup"
  exit 1
fi
systemctl reload nginx
echo "UPSTREAM=${new_port} NEW_UNIT=${new_unit} OLD_PORT=${old_port}"

for i in $(seq 1 900); do
  h=$(curl -fsS "http://127.0.0.1:${old_port}/api/health" || true)
  if echo "$h" | grep -q '"active_jobs":0'; then
    systemctl stop "$old_unit"
    echo "SWITCH_OK old_unit_stopped=${old_unit}"
    exit 0
  fi
  sleep 2
done
echo "OLD_INSTANCE_STILL_BUSY old_port=${old_port}"
exit 2