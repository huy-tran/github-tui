#requires -Version 5.1
<#
.SYNOPSIS
    Build and install the gh-tui binary into the Go bin directory (on PATH).

.DESCRIPTION
    Builds the current module and drops `gh-tui.exe` into $env:GOBIN, or
    "$(go env GOPATH)\bin" when GOBIN is unset. That directory is normally on
    PATH for Go installs, so `gh-tui` becomes runnable from anywhere.

.EXAMPLE
    .\install.ps1
#>
[CmdletBinding()]
param(
    # Name of the installed executable (without extension).
    [string]$Name = 'gh-tui'
)

$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

# Resolve the install directory: GOBIN wins, else GOPATH\bin.
$binDir = (go env GOBIN)
if ([string]::IsNullOrWhiteSpace($binDir)) {
    $binDir = Join-Path (go env GOPATH) 'bin'
}

if (-not (Test-Path -LiteralPath $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

$target = Join-Path $binDir "$Name.exe"
$repoExe = Join-Path $PSScriptRoot "$Name.exe"

# 1. Build the repo-local binary.
Write-Host "Building $Name -> $repoExe" -ForegroundColor Cyan
go build -o $repoExe .
if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
}
Write-Host "Built $repoExe" -ForegroundColor Green

# 2. Copy it onto PATH so `gh-tui` runs from anywhere (identical binary).
try {
    Copy-Item -LiteralPath $repoExe -Destination $target -Force -ErrorAction Stop
    Write-Host "Installed $target" -ForegroundColor Green
} catch [System.IO.IOException] {
    Write-Warning "Could not update $target - it looks like gh-tui is currently running."
    Write-Warning "Quit the running gh-tui and re-run .\install.ps1 to update the PATH copy."
    Write-Host "(The repo-local $repoExe is up to date.)" -ForegroundColor Green
}

# Warn if the install dir is not on PATH so the user knows to fix it.
$onPath = ($env:PATH -split ';') -contains $binDir
if (-not $onPath) {
    Write-Warning "$binDir is not on your PATH. Add it, e.g.:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', `"`$env:Path;$binDir`", 'User')" -ForegroundColor Yellow
} else {
    Write-Host "Run it from anywhere with: $Name" -ForegroundColor Green
}
