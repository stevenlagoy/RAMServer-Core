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
    'lint'     { buf lint; Push-Location server; go vet ./...; Pop-Location }
    'test'     { Push-Location server; go test ./...; Pop-Location }
    'build'    { Push-Location server; go build ./...; Pop-Location }
    'up'       { docker compose up --build }
    'down'     { docker compose down }
}