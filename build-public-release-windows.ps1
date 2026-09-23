param(
    [string]$Version = "0.12.19"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

if ($env:OS -ne "Windows_NT") { throw "Run this script on Windows." }
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object -TypeName Security.Principal.WindowsPrincipal -ArgumentList $identity
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -and $env:GITHUB_ACTIONS -ne "true") {
    throw "Open PowerShell as Administrator before running this script; Windows symlink tests require it."
}

# An elevated PowerShell may find an old per-user Node before the system Node.
$systemNode = "C:\Program Files\nodejs\node.exe"
if (Test-Path $systemNode) {
    $env:Path = "$(Split-Path -Parent $systemNode);$env:Path"
}
$nodeVersion = (& node --version).Trim()
if ($LASTEXITCODE -ne 0) { throw "Node.js 22 or newer is required." }
if ($nodeVersion -notmatch '^v(\d+)\.') { throw "Cannot parse Node.js version: $nodeVersion." }
$nodeMajor = [int]($nodeVersion -replace '^v(\d+)\..*$', '$1')
if ($nodeMajor -lt 22) { throw "Node.js 22 or newer is required; found $nodeVersion." }

if ($env:GITHUB_ACTIONS -ne "true" -and (-not $env:GOPROXY -or $env:GOPROXY -eq "https://proxy.golang.org,direct")) {
    $env:GOPROXY = "https://goproxy.cn|https://proxy.golang.org|direct"
}

Push-Location $projectRoot
try {
    & (Join-Path $projectRoot "build-desktop-windows.ps1") -Version $Version -KeyMode public
    if (-not $?) { throw "Windows desktop build failed." }

    & node (Join-Path $projectRoot "scripts\make-release.cjs")
    if ($LASTEXITCODE -ne 0) { throw "Release file generation failed." }

    & node --test (Join-Path $projectRoot "scripts\make-release.test.cjs") (Join-Path $projectRoot "desktop\release-build.test.cjs")
    if ($LASTEXITCODE -ne 0) { throw "Release tests failed." }
    & go test -run TestUpdate ./...
    if ($LASTEXITCODE -ne 0) { throw "Update tests failed." }

    $releaseDir = Join-Path $projectRoot "dist\release"
    $setupName = "Deep-Legends-Setup-$Version-public.exe"
    $expected = @($setupName, "latest.json", "SHA256SUMS-public.txt")
    $actual = @(Get-ChildItem -Path $releaseDir -File | ForEach-Object { $_.Name } | Sort-Object)
    if (($actual -join "`n") -ne (($expected | Sort-Object) -join "`n")) {
        throw "dist/release must contain exactly the three public release files; found: $($actual -join ', ')"
    }
    $manifest = Get-Content -Raw -Encoding UTF8 (Join-Path $releaseDir "latest.json") | ConvertFrom-Json
    if ($manifest.version -ne $Version -or $manifest.asset.name -ne $setupName) {
        throw "Release manifest version or asset name does not match $Version."
    }
    $expectedUrl = "https://github.com/LLYY0418/Deep-Legends/releases/download/v$Version/$setupName"
    if ($manifest.asset.url -ne $expectedUrl) { throw "Release asset URL does not match the expected URL." }
    $sumLines = @(Get-Content (Join-Path $releaseDir "SHA256SUMS-public.txt") | Where-Object { $_ })
    foreach ($name in @($setupName, "latest.json")) {
        $hash = (Get-FileHash -Algorithm SHA256 (Join-Path $releaseDir $name)).Hash.ToLowerInvariant()
        if ($sumLines -notcontains "$hash  $name") { throw "SHA-256 mismatch: $name" }
        if ($name -eq $setupName -and $manifest.asset.sha256 -ne $hash) { throw "Manifest SHA-256 mismatch: $name" }
    }
    if ($sumLines.Count -ne 2) { throw "Unexpected lines in SHA256SUMS-public.txt." }

    Write-Host "Public release build passed: $releaseDir"
    Write-Host "Source fingerprint: $($manifest.fingerprint)"
    Write-Host "Setup SHA-256: $($manifest.asset.sha256)"
} finally {
    Pop-Location
}
