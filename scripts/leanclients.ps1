# Opens several leanclients in one Windows Terminal window, one pane each, named Player1..PlayerN.
# Usage: scripts\leanclients.ps1 [count] [leanclient flags...]
#   scripts\leanclients.ps1              3 clients
#   scripts\leanclients.ps1 4 -addr 127.0.0.1:9001
# Close them all by closing the Windows Terminal window.
$ErrorActionPreference = 'Stop'

$count = 3
$extra = @($args)
if ($extra.Count -gt 0 -and $extra[0] -match '^\d+$') {
    $count = [int]$extra[0]
    $extra = @($extra | Select-Object -Skip 1)
}
if ($count -lt 1) {
    Write-Error "usage: leanclients.ps1 [count] [leanclient flags...]"
    exit 1
}
if (-not (Get-Command wt.exe -ErrorAction SilentlyContinue)) {
    Write-Error "Windows Terminal (wt.exe) is not installed"
    exit 1
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "go is not installed"
    exit 1
}

# Build once so every pane starts immediately instead of compiling with go run
$bin = Join-Path $env:TEMP 'ramserver-leanclient.exe'
Push-Location (Join-Path $PSScriptRoot '..\server')
try {
    go build -o $bin ./cmd/leanclient
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
finally {
    Pop-Location
}

$shell = (Get-Process -Id $PID).Path
$startDir = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

function Quote([string]$s) { "'" + $s.Replace("'", "''") + "'" }

# Command line for client $n, with extra flags quoted safely. It is passed as a
# base64-encoded command so no further quoting is needed when handing it to wt.
function Get-EncodedClientCommand([int]$n) {
    $quotedExtra = @($extra | ForEach-Object { Quote $_ })
    $parts = @((Quote $bin), '-name', "Player$n") + $quotedExtra
    $cmd = '& ' + ($parts -join ' ')
    [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($cmd))
}

function Get-PaneArgs([int]$n) {
    "-d `"$startDir`" --title Player$n --suppressApplicationTitle `"$shell`" -NoLogo -NoExit -EncodedCommand $(Get-EncodedClientCommand $n)"
}

$wtArgs = "-w new new-tab $(Get-PaneArgs 1)"
for ($i = 2; $i -le $count; $i++) {
    $wtArgs += " ; split-pane $(Get-PaneArgs $i)"
}

Start-Process wt.exe -ArgumentList $wtArgs