$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'r279-upgrade-stages.ps1')
function ConvertTo-R271SafeText([string]$Value) { return $Value }
$out = Join-Path ([IO.Path]::GetTempPath()) ('r279-stage-' + [guid]::NewGuid() + '.json')
try {
    Invoke-R279UpgradeStage -Name 'prepare' -Output $out -Action { throw 'controlled target failure' }
    Invoke-R279UpgradeStage -Name 'dependent' -DependsOn @('prepare') -Output $out -Action { throw 'dependency must never run' }
    Invoke-R279UpgradeStage -Name 'independent' -Output $out -Action { $script:independentRan = $true }
    $rows = @(Get-Content $out -Raw | ConvertFrom-Json)
    if ($rows.Count -ne 3 -or $rows[0].status -ne 'failed' -or $rows[1].status -ne 'skipped-dependency-failed' -or $rows[2].status -ne 'passed' -or -not $script:independentRan) { throw 'collection/dependency mutation survived' }
    $failed = $false
    try { Assert-R279UpgradeStages } catch { $failed = $_.Exception.Message -match 'prepare:failed' }
    if (-not $failed) { throw 'swallow-failure mutation survived' }
    $script:R279UpgradeStages = @()
    Invoke-R279UpgradeStage -Name 'pass' -Output $out -Action {}
    Assert-R279UpgradeStages
    Write-Output 'R279 stage collection, dependency and terminal failure proofs PASS'
} finally { Remove-Item -LiteralPath $out -Force -ErrorAction SilentlyContinue }

# Exercise the real corrupt-stage action with an already-running installed app.
# The original lifecycle keeps that app alive; the Node probe reloads its document itself.
$source = Get-Content (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1') -Raw
$match = [regex]::Match($source, "Invoke-R279UpgradeStage -Name 'corrupt'[^\r\n]+-Action \{\r?\n([\s\S]*?)\r?\n    \}")
if (-not $match.Success) { throw 'actual corrupt action not found' }
$action = [scriptblock]::Create($match.Groups[1].Value)
function Start-R265DebugApp { throw 'original ObserveOnly rejects an app that is still running' }
function node { $global:LASTEXITCODE = 0; $script:corruptProbeRan = $true }
$debugPort = 12345
$evidence = Join-Path ([IO.Path]::GetTempPath()) ('r279-corrupt-action-' + [guid]::NewGuid())
New-Item -ItemType Directory -Force $evidence | Out-Null
$out = Join-Path $evidence 'stages.json'
try {
    $script:R279UpgradeStages = @(@{ name = 'write-presets'; status = 'passed' })
    Invoke-R279UpgradeStage -Name 'corrupt' -DependsOn @('write-presets') -Output $out -Action $action
    if (-not $script:corruptProbeRan -or $script:R279UpgradeStages[-1].status -ne 'passed') { throw 'live-app corrupt action regression' }
    Invoke-R279UpgradeStage -Name 'old-corrupt-mutant' -Output $out -Action { Start-R265DebugApp; & $action }
    if ($script:R279UpgradeStages[-1].status -ne 'failed' -or $script:R279UpgradeStages[-1].error -notmatch 'ObserveOnly') { throw 'old redundant restart mutant survived' }
    Invoke-R279UpgradeStage -Name 'dependent-chain' -DependsOn @('old-corrupt-mutant') -Output $out -Action { throw 'contaminated shared profile must not run' }
    if ($script:R279UpgradeStages[-1].status -ne 'skipped-dependency-failed') { throw 'shared profile dependency was not skipped' }
    Write-Output 'R279 real corrupt action PASS; redundant restart mutant FAIL; dependent profile chain skipped'
} finally { Remove-Item -LiteralPath $evidence -Recurse -Force -ErrorAction SilentlyContinue }
