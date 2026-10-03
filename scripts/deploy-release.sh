#!/bin/bash
# deploy-release.sh — 在服务器上把某个 GitHub Release 装成新的版本目录并滚动切换。
#
# 用法：deploy-release.sh v0.1.27
#
# 只从 GitHub Release 取包，并强制校验 checksums.txt。装到独立的
# /opt/chanshui-<ver>，不覆盖当前目录；再按 docs/deploy/rolling-upgrade.md
# 的顺序排空旧实例后切换软链接。
set -euo pipefail

tag="${1:-}"
if [ -z "$tag" ]; then
  echo "用法: $0 <tag，例如 v0.1.27>" >&2
  exit 1
fi
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "tag 必须形如 v0.1.27" >&2
  exit 1
fi

base="https://github.com/kiss-kedaya/chanzhushui/releases/download/${tag}"
ver="${tag#v}"                       # v0.1.27 -> 0.1.27
target="/opt/chanshui-${ver}"
archive="chanshui_${tag}_linux-amd64.tar.gz"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cd "$work"
curl -fsSL -o pkg.tar.gz "${base}/${archive}"
curl -fsSL -o checksums.txt "${base}/checksums.txt"
# 校验失败必须中止：包和清单对不上说明下载损坏或被人动过。
grep " ${archive}\$" checksums.txt | sha256sum -c -

sudo rm -rf "$target"
sudo mkdir -p "$target"
sudo tar -xzf pkg.tar.gz -C "$target" --strip-components=1
sudo chown -R chanshui:chanshui "$target"
sudo chmod 755 "$target/chanshui"

echo "VERSION=$(sudo cat "$target/VERSION")"
test -x "$target/chanshui"
test -f "$target/web/index.html"

# 排空旧实例：SIGUSR1 让它停止接收新 Job，等 queued/running 归零。
pid=$(systemctl show -p MainPID --value chanshui || true)
echo "OLD_PID=${pid:-none}"
if [ -n "${pid:-}" ] && [ "$pid" != "0" ]; then
  sudo kill -USR1 "$pid"
  ok=0
  for _ in $(seq 1 60); do
    h=$(curl -fsS http://127.0.0.1:8770/api/health || true)
    code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8770/api/ready || true)
    if echo "$h" | grep -q '"queued_jobs":0' && echo "$h" | grep -q '"active_jobs":0' && [ "$code" = "503" ]; then
      ok=1
      break
    fi
    sleep 2
  done
  if [ "$ok" != 1 ]; then
    echo "DRAIN_NOT_READY：仍有任务在跑，未切换。稍后重试。" >&2
    exit 2
  fi
fi

sudo systemctl stop chanshui
sudo ln -sfn "$target" /opt/chanshui-current
sudo systemctl start chanshui
sleep 1
systemctl is-active chanshui
readlink -f /opt/chanshui-current
curl -fsS http://127.0.0.1:8770/api/health; echo
curl -fsS -o /dev/null -w 'ready %{http_code}\n' http://127.0.0.1:8770/api/ready
curl -fsS -o /dev/null -w 'local_home %{http_code}\n' http://127.0.0.1:8770/
