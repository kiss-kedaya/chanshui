param([int]$Port=8770,[switch]$SkipBuild)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$local=Join-Path $root '.local'
New-Item -ItemType Directory -Path $local -Force | Out-Null
if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) { throw "Port $Port already in use; no existing process was stopped." }
if(-not $SkipBuild){
 Push-Location (Join-Path $root 'web')
 try { & npm.cmd ci; if($LASTEXITCODE -ne 0){throw 'npm ci failed'}; & npm.cmd run build; if($LASTEXITCODE -ne 0){throw 'Next.js build failed'} } finally {Pop-Location}
 & go test ./...; if($LASTEXITCODE -ne 0){throw 'Go tests failed'}
 & go build -o .local/chanshui.exe .; if($LASTEXITCODE -ne 0){throw 'Go build failed'}
}
$exe=Join-Path $local 'chanshui.exe'
if(-not(Test-Path -LiteralPath $exe)){throw 'Missing executable; rerun without -SkipBuild'}
if(-not(Test-Path -LiteralPath (Join-Path $root 'web/out-build/index.html'))){throw 'Missing static export'}
$p=Start-Process -FilePath $exe -ArgumentList @('-addr',"127.0.0.1:$Port") -WorkingDirectory $root -WindowStyle Hidden -RedirectStandardOutput (Join-Path $local 'server.stdout.log') -RedirectStandardError (Join-Path $local 'server.stderr.log') -PassThru
[System.IO.File]::WriteAllText((Join-Path $local 'server.pid'),[string]$p.Id)
for($i=0;$i -lt 30;$i++){
 try{$resp=Invoke-WebRequest -Uri "http://127.0.0.1:$Port/api/health" -TimeoutSec 2;if($resp.StatusCode -eq 200){Write-Output "Local app: http://127.0.0.1:$Port ; PID=$($p.Id)";return}}catch{Start-Sleep -Milliseconds 300}
}
throw 'Local health check did not become ready; see .local/server.stderr.log'
