# Test orchestration only. No settings writes, fixture injection or product changes.
$script:R279UpgradeStages = @()
function Invoke-R279UpgradeStage {
    param([string]$Name, [string[]]$DependsOn = @(), [scriptblock]$Action, [string]$Output)
    $blocked = @($DependsOn | Where-Object { $dependency = $_; -not @($script:R279UpgradeStages | Where-Object { $_.name -eq $dependency -and $_.status -eq 'passed' }).Count })
    $started = [Diagnostics.Stopwatch]::StartNew()
    $row = [ordered]@{ name = $Name; status = 'passed'; exitCode = 0; dependencies = $blocked; error = $null; duration_ms = 0 }
    if ($blocked.Count) { $row.status = 'skipped-dependency-failed'; $row.exitCode = $null }
    else {
        try { & $Action }
        catch { $row.status = 'failed'; $row.exitCode = 1; $row.error = ConvertTo-R271SafeText $_.Exception.Message; Write-Output ("R279_STAGE_FAILURE ${Name}: " + $row.error) }
    }
    $row.duration_ms = $started.Elapsed.TotalMilliseconds
    $script:R279UpgradeStages += @($row)
    ConvertTo-Json -InputObject @($script:R279UpgradeStages) -Depth 8 | Set-Content -LiteralPath $Output -Encoding utf8
}
function Assert-R279UpgradeStages {
    $failed = @($script:R279UpgradeStages | Where-Object { $_.status -ne 'passed' })
    if ($failed.Count) { throw ('R279 upgrade failed stages: ' + (($failed | ForEach-Object { $_.name + ':' + $_.status + ':' + $_.error }) -join '; ')) }
}
