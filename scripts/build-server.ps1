param([string]$Output='.artifacts/release-linux-amd64')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
& ./scripts/test-local.ps1
if($LASTEXITCODE -ne 0){throw 'Tests failed'}
$target=[IO.Path]::GetFullPath((Join-Path $root $Output))
if(-not $target.StartsWith($root+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Output must stay within this workspace'}
New-Item -ItemType Directory -Path $target -Force | Out-Null
$goos=$env:GOOS;$goarch=$env:GOARCH;$cgo=$env:CGO_ENABLED
try{$env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0';& go build -trimpath -ldflags '-s -w' -o (Join-Path $target 'chanshui') .;if($LASTEXITCODE -ne 0){throw 'Linux build failed'};& go build -trimpath -ldflags '-s -w' -o (Join-Path $target 'feedback-export') ./cmd/feedback-export;if($LASTEXITCODE -ne 0){throw 'Feedback export build failed'}}finally{$env:GOOS=$goos;$env:GOARCH=$goarch;$env:CGO_ENABLED=$cgo}
$webTarget=Join-Path $target 'web'
New-Item -ItemType Directory -Path $webTarget -Force | Out-Null
Get-ChildItem -LiteralPath web/out-build -Force | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination $webTarget -Recurse -Force}
Copy-Item -LiteralPath docs/server-preparation.md -Destination $target
Copy-Item -LiteralPath README.md -Destination $target
Copy-Item -LiteralPath LICENSE -Destination $target
Copy-Item -LiteralPath SECURITY.md -Destination $target
Copy-Item -LiteralPath CONTRIBUTING.md -Destination $target
Copy-Item -LiteralPath THIRD_PARTY_NOTICES.md -Destination $target
Copy-Item -LiteralPath .env.example -Destination $target
$version = if($env:RELEASE_VERSION){$env:RELEASE_VERSION}else{try{git describe --tags --always --dirty 2>$null}catch{'dev-local'}}
$releaseNote = Join-Path $root "docs/release-notes/$version.md"
if(Test-Path -LiteralPath $releaseNote){Copy-Item -LiteralPath $releaseNote -Destination $target}
[IO.File]::WriteAllText((Join-Path $target 'VERSION'),[string]$version)
$hashLines=@()
Get-ChildItem -LiteralPath $target -File -Recurse | ForEach-Object {if($_.Name -ne 'checksums.txt'){$hash=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower();$relative=$_.FullName.Substring($target.Length+1).Replace('\','/');$hashLines += "$hash  $relative"}}
$hashLines | Set-Content -LiteralPath (Join-Path $target 'checksums.txt') -Encoding utf8
Write-Output "Local deployment bundle ready: $target ; not uploaded or deployed"
