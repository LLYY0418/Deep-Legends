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
