# deploy-release.ps1 — 把某个 GitHub Release 发到服务器（私有仓库可用）。
#
# 用法（在仓库根目录执行）：
#   ./scripts/deploy-release.ps1 -Tag v0.1.27
#   ./scripts/deploy-release.ps1 -Tag v0.1.27 -SshHost kedaya-new
#
# 为什么分两段：仓库是私有的，服务器直连 Release 会 404。所以在本机用已登录的
# gh 下载包，校验 sha256 后再 scp 上传；服务器只负责校验同一个 hash 并切换。
# GitHub 凭据不落到服务器上。
#
# 切换按 docs/deploy/rolling-upgrade.md：先 SIGUSR1 排空，确认 queued/active 归零
# 且 /api/ready 返回 503，再停服务、切软链接、启动。
param(
  [Parameter(Mandatory = $true)][string]$Tag,
  [string]$SshHost = 'kedaya-new',
  [string]$Repo = 'kiss-kedaya/chanshui',
  [switch]$SkipDeploy
)
$ErrorActionPreference = 'Stop'
if ($Tag -notmatch '^v\d+\.\d+\.\d+$') { throw "Tag 必须形如 v0.1.27，收到：$Tag" }
$ver = $Tag.TrimStart('v')
$archive = "chanshui_${Tag}_linux-amd64.tar.gz"

$work = Join-Path $env:TEMP "chanshui-deploy-$ver"
if (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force }
New-Item -ItemType Directory -Path $work -Force | Out-Null

Write-Output "1/5 下载 $Tag 的 linux-amd64 包与校验清单"
& gh release download $Tag --repo $Repo --pattern $archive --pattern 'checksums.txt' --dir $work
if ($LASTEXITCODE -ne 0) { throw 'gh release download 失败；确认 gh 已登录且有该仓库权限' }

Write-Output '2/5 本机校验 sha256'
$line = (Select-String -LiteralPath (Join-Path $work 'checksums.txt') -Pattern " $([regex]::Escape($archive))$").Line
if (-not $line) { throw "checksums.txt 里没有 $archive" }
$want = ($line -split '\s+')[0].ToLower()
# 不依赖 Get-FileHash：某些精简环境里 Microsoft.PowerShell.Utility 没加载。
# certutil 是系统自带，输出里取第一段十六进制即可。
$certOut = & certutil -hashfile (Join-Path $work $archive) SHA256 2>&1
$hexLine = $certOut | Where-Object { $_ -match '^[0-9a-fA-F\s]{32,}$' } | Select-Object -First 1
if (-not $hexLine) { throw "certutil 没有返回哈希：$certOut" }
$got = ($hexLine -replace '\s', '').ToLower()
if ($want -ne $got) { throw "本机校验失败：期望 $want，实际 $got" }
Write-Output "    OK $got"

Write-Output "3/5 上传到 $SshHost"
& scp -o ConnectTimeout=20 (Join-Path $work $archive) (Join-Path $work 'checksums.txt') "${SshHost}:/tmp/"
if ($LASTEXITCODE -ne 0) { throw 'scp 失败' }

Write-Output '4/5 服务器校验并解到新版本目录'
$remote = @"
set -euo pipefail
cd /tmp
grep " $archive\$" checksums.txt | sha256sum -c -
target=/opt/chanshui-$ver
sudo rm -rf "`$target"; sudo mkdir -p "`$target"
sudo tar -xzf $archive -C "`$target" --strip-components=1
sudo chown -R chanshui:chanshui "`$target"; sudo chmod 755 "`$target/chanshui"
cat "`$target/VERSION"
test -x "`$target/chanshui"; test -f "`$target/web/index.html"
echo PKG_OK
"@
& ssh -o ConnectTimeout=20 $SshHost $remote
if ($LASTEXITCODE -ne 0) { throw '服务器安装新版本目录失败' }

if ($SkipDeploy) { Write-Output '已跳过切换（-SkipDeploy）：新版本目录就绪，未改动运行中的服务'; return }

Write-Output '5/5 排空并切换'
$switch = @"
set -euo pipefail
pid=`$(systemctl show -p MainPID --value chanshui)
echo "OLD_PID=`$pid"
if [ -n "`$pid" ] && [ "`$pid" != "0" ]; then
  sudo kill -USR1 "`$pid"
  ok=0
  for i in `$(seq 1 60); do
    h=`$(curl -fsS http://127.0.0.1:8770/api/health || true)
    code=`$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8770/api/ready || true)
    if echo "`$h" | grep -q '\"queued_jobs\":0' && echo "`$h" | grep -q '\"active_jobs\":0' && [ "`$code" = "503" ]; then ok=1; break; fi
    sleep 2
  done
  if [ "`$ok" != 1 ]; then echo DRAIN_NOT_READY; exit 2; fi
fi
sudo systemctl stop chanshui
sudo ln -sfn /opt/chanshui-$ver /opt/chanshui-current
sudo systemctl start chanshui
sleep 2
systemctl is-active chanshui
readlink -f /opt/chanshui-current
curl -fsS http://127.0.0.1:8770/api/health; echo
curl -fsS -o /dev/null -w 'ready %{http_code}\n' http://127.0.0.1:8770/api/ready
curl -fsS -o /dev/null -w 'home %{http_code}\n' http://127.0.0.1:8770/
"@
& ssh -o ConnectTimeout=20 $SshHost $switch
if ($LASTEXITCODE -ne 0) { throw '切换失败；新版本目录已就绪，当前仍指向旧版本' }
Write-Output "$Tag 部署完成"
