<#
.SYNOPSIS
    Builds renderer.dll (CMake + Ninja) and game.exe (Go + cgo) into build/bin.
.EXAMPLE
    ./build.ps1 -Run
    ./build.ps1 -Config Release
#>
param(
    [ValidateSet('Debug', 'Release')]
    [string] $Config = 'Debug',
    [switch] $Run
)

$ErrorActionPreference = 'Stop'
$root  = $PSScriptRoot
$build = Join-Path $root 'build'
$bin   = Join-Path $build 'bin'

function Invoke-Step([string] $Name, [scriptblock] $Command) {
    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "$Name failed (exit code $LASTEXITCODE)" }
}

Invoke-Step 'Configure renderer' {
    cmake -S (Join-Path $root 'renderer') -B $build -G Ninja "-DCMAKE_BUILD_TYPE=$Config"
}
Invoke-Step 'Build renderer' { cmake --build $build }

Push-Location $root
try {
    $env:CGO_ENABLED = '1'
    Invoke-Step 'Resolve Go modules' { go mod tidy }
    Invoke-Step 'Build game' { go build -o (Join-Path $bin 'game.exe') ./cmd/game }
} finally {
    Pop-Location
}

if ($Run) {
    Push-Location $bin
    try { & (Join-Path $bin 'game.exe') } finally { Pop-Location }
}
