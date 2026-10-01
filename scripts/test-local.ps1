$ErrorActionPreference='Stop'
Set-Location -LiteralPath (Split-Path -Parent $PSScriptRoot)
& go test -race ./...;if($LASTEXITCODE -ne 0){throw 'Race tests failed'}
& go vet ./...;if($LASTEXITCODE -ne 0){throw 'Go vet failed'}
Push-Location web
try{& npm.cmd ci;if($LASTEXITCODE -ne 0){throw 'Dependencies failed'};& npm.cmd test;if($LASTEXITCODE -ne 0){throw 'Frontend tests failed'};& npm.cmd run build;if($LASTEXITCODE -ne 0){throw 'Frontend build failed'};& npm.cmd run typecheck;if($LASTEXITCODE -ne 0){throw 'Typecheck failed'}}finally{Pop-Location}
Write-Output 'All local checks passed'
