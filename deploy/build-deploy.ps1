# build-deploy.ps1 — assemble deploy/pack and build cadguanliq-installer.exe
# Run on the dev machine:  powershell -File build-deploy.ps1
# Result: deploy\cadguanliq-installer.exe  (single file, copy to any Windows machine)

$ErrorActionPreference = 'Stop'
$RepoRoot = Split-Path -Parent $PSScriptRoot          # repo root (deploy\..)
$PackDir  = Join-Path $PSScriptRoot 'pack'
$Packer   = Join-Path $PSScriptRoot 'packer.exe'
$VsVars   = 'D:\exe\vs\VC\Auxiliary\Build\vcvars64.bat'
$PgSetup  = Join-Path $RepoRoot 'postgresql-18.6-1-windows-x64.exe'
$UvPython = Join-Path $env:APPDATA 'uv\python\cpython-3.14.0-windows-x86_64-none'

function Copy-Tree($source, $dest, $excludeDirs = @(), $excludeFiles = @()) {
    robocopy $source $dest /E /NFL /NDL /NJH /NJS /NP /R:1 /W:1 /XD @excludeDirs /XF @excludeFiles | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "robocopy failed for $source (code $LASTEXITCODE)" }
}

if (-not (Test-Path $Packer)) { throw "packer.exe not found, build it first (cl /std:c++17 ...)" }
if (-not (Test-Path $PgSetup)) { throw "PostgreSQL setup not found: $PgSetup" }
if (-not (Test-Path (Join-Path $RepoRoot 'go\.venv\Scripts\python.exe'))) { throw "go\.venv missing, create it first" }
if (-not (Test-Path $UvPython)) { throw "uv python runtime missing: $UvPython" }

if (Test-Path $PackDir) { Remove-Item -Recurse -Force $PackDir }
New-Item -ItemType Directory -Force $PackDir | Out-Null

Write-Host '==> Building backend exe'
$goDir = Join-Path $RepoRoot 'go'
Push-Location $goDir
go build -trimpath -o (Join-Path $PackDir 'cadguanliq.exe') ./cmd/server
if ($LASTEXITCODE -ne 0) { Pop-Location; throw 'go build failed' }
Pop-Location

Write-Host '==> Copying tools (exb_probe, wheels, CAXA plugin)'
Copy-Tree (Join-Path $RepoRoot 'tools') (Join-Path $PackDir 'tools') @('exb2dxf\plugin', 'exb2dxf\ok\examples') @('cad2x.exe', 'cad2x_real.exe', 'sample-output.dwg', 'sample-test.dxf')

Write-Host '==> Copying migrations'
Copy-Tree (Join-Path $RepoRoot 'go\database\migrations') (Join-Path $PackDir 'database\migrations')

Write-Host '==> Copying venv'
Copy-Tree (Join-Path $RepoRoot 'go\.venv') (Join-Path $PackDir '.venv')

Write-Host '==> Copying portable python runtime'
Copy-Tree $UvPython (Join-Path $PackDir 'runtime\python')

Write-Host '==> Copying PostgreSQL installer'
Copy-Item $PgSetup $PackDir

Write-Host '==> Copying fallback scripts'
Copy-Item (Join-Path $PSScriptRoot 'install.ps1') $PackDir
Copy-Item (Join-Path $PSScriptRoot 'install.bat') $PackDir

Write-Host '==> Packing installer'
$installer = Join-Path $PSScriptRoot 'cadguanliq-installer.exe'
& $Packer pack $PackDir $installer | Select-Object -Last 2
if ($LASTEXITCODE -ne 0) { throw 'pack failed' }

$size = (Get-Item $installer).Length / 1MB
Write-Host ("==> DONE: {0}  ({1:N0} MB)" -f $installer, $size)
Write-Host '    Target machine: copy this single exe, double-click, everything installs offline.'
