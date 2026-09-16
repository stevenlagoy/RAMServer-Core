#Requires -Version 5.1
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('generate', 'lint', 'test', 'build', 'up', 'down')]
    [string]$Command
)

$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

switch ($Command) {
    'generate' { buf generate }
    'lint' {
        buf lint
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        Push-Location server
        try {
            go vet ./...
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        } finally {
            Pop-Location
        }
    }
    'test' {
        Push-Location server
        try {
            go test -race ./...
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        } finally {
            Pop-Location
        }
    }
    'build' {
        Push-Location server
        try {
            go build ./...
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        } finally {
            Pop-Location
        }
    }
    'up'       { docker compose up --build }
    'down'     { docker compose down }
}