$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$pidPath=Join-Path $root '.local/server.pid'
if(-not(Test-Path -LiteralPath $pidPath)){Write-Output 'No local server PID file';return}
$serverPid=[int](Get-Content -LiteralPath $pidPath -Raw)
$p=Get-Process -Id $serverPid -ErrorAction SilentlyContinue
$expected=[IO.Path]::GetFullPath((Join-Path $root '.local/chanshui.exe'))
if($p){if($p.Path -ne $expected){throw 'PID belongs to another executable; refusing to stop'};Stop-Process -Id $serverPid;$p.WaitForExit(10000) | Out-Null;Write-Output "Stopped only local project server: $serverPid"}else{Write-Output 'Local server already stopped'}
