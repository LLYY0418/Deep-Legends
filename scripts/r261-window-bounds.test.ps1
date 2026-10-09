$ErrorActionPreference = 'Stop'
$source = Get-Content (Join-Path $PSScriptRoot 'r261-window-bounds.ps1') -Raw
$implementation = [ScriptBlock]::Create($source)
$root = Join-Path ([IO.Path]::GetTempPath()) ('r261-bounds-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $root | Out-Null
function Assert-CloseRace([ScriptBlock]$Implementation, [string]$Initial) {
    & {
        param($Implementation,$Initial,$Root)
        . $Implementation
        $file = Join-Path $Root 'bounds.json'
        $evidence = Join-Path $Root 'wait.json'
        $state = @{step=0;closes=0}
        $process = [pscustomobject]@{Id=37;MainWindowHandle=0;MainWindowTitle='';Fixture=$state}
        $process | Add-Member ScriptMethod CloseMainWindow { $this.Fixture.closes++; return $true }
        $enumerate = {
            param($Executable)
            switch ($state.step++) {
                0 { [IO.File]::WriteAllText($file,$Initial); return $process }
                1 { [IO.File]::WriteAllText($file,''); $process.MainWindowHandle=1; $process.MainWindowTitle='Deep Legends'; return $process }
                2 { [IO.File]::WriteAllText($file,'{"width":'); return $process }
                3 { [IO.File]::WriteAllText($file,'{"width":800,"height":600,"x":1,"y":2}'); return $process }
                4 { [IO.File]::WriteAllText($file,'{"width":900,"height":700,"x":3,"y":4}'); return }
                default { throw 'Unexpected extra poll' }
            }
        }.GetNewClosure()
        $result = Wait-R261PublishedWindowBounds -ProcessPath 'fixture.exe' -BoundsFile $file -Deadline (Get-Date).AddSeconds(10) -EvidencePath $evidence -ListProcesses $enumerate
        $value = $result | ConvertFrom-Json
        if ($state.step -ne 5 -or $state.closes -ne 1 -or $value.width -ne 900 -or $value.x -ne 3) { throw 'R261_ASSERT_NORMAL_EXIT: returned before final close persistence' }
        $record = Get-Content $evidence -Raw | ConvertFrom-Json
        if ($record.outcome -ne 'success' -or @($record.observations | Where-Object { $_.bounds_state -eq 'empty' }).Count -eq 0 -or @($record.observations | Where-Object { $_.bounds_state -eq 'invalid_json' }).Count -eq 0) { throw 'R261_ASSERT_EVIDENCE: transient writes were not recorded' }
    } $Implementation $Initial $root
}
try {
    Assert-CloseRace $implementation ''
    Assert-CloseRace $implementation '{"width":500,"height":400,"x":0,"y":0}'
    Write-Output 'R261 empty/partial/stale-valid file waits for normal close and final snapshot PASS'
    . $implementation
    foreach ($invalid in @('', '{"width":', '{"width":0,"height":600,"x":1,"y":2}', '{"width":800,"height":600,"x":1}')) {
        $file = Join-Path $root 'invalid.json'
        [IO.File]::WriteAllText($file,$invalid)
        $state = @{step=0}
        $process = [pscustomobject]@{Id=41;MainWindowHandle=1;MainWindowTitle='Deep Legends'}
        $process | Add-Member ScriptMethod CloseMainWindow { return $true }
        $enumerate = { param($Executable); if ($state.step++ -eq 0) { return $process } }.GetNewClosure()
        $rejected = $false
        try { $null = Wait-R261PublishedWindowBounds 'fixture.exe' $file (Get-Date).AddSeconds(10) (Join-Path $root 'invalid-wait.json') $enumerate } catch {
            if ($_.Exception.Message -notmatch '^Invalid actual 0.12.76 window bounds after normal close:') { throw }
            $rejected = $true
        }
        if (-not $rejected) { throw 'R261_ASSERT_INVALID: invalid final bounds accepted' }
    }
    Write-Output 'R261 empty/malformed/nonpositive/missing-coordinate final bounds rejected PASS'
    $live = [pscustomobject]@{Id=43;MainWindowHandle=0;MainWindowTitle='not the main window'}
    $live | Add-Member ScriptMethod CloseMainWindow { throw 'Wrong window must not be closed' }
    $enumerate = { param($Executable); return $live }.GetNewClosure()
    $rejected = $false
    $deadline = (Get-Date).AddMilliseconds(150)
    try { $null = Wait-R261PublishedWindowBounds 'fixture.exe' (Join-Path $root 'bounds.json') $deadline (Join-Path $root 'timeout.json') $enumerate } catch {
        if ($_.Exception.Message -notmatch '^Timed out waiting for normal 0.12.76 window close') { throw }; $rejected = $true
    }
    if (-not $rejected) { throw 'R261_ASSERT_TIMEOUT: process that never closes was accepted' }
    $record = Get-Content (Join-Path $root 'timeout.json') -Raw | ConvertFrom-Json
    if ([DateTime]$record.deadline -ne $deadline.ToUniversalTime()) { throw 'R261_ASSERT_DEADLINE: caller deadline changed' }
    $rejected = $false
    try { $null = Wait-R261PublishedWindowBounds 'fixture.exe' (Join-Path $root 'bounds.json') (Get-Date).AddSeconds(-1) (Join-Path $root 'expired.json') { throw 'Expired deadline must not poll' } } catch {
        if ($_.Exception.Message -notmatch '^Timed out waiting for normal 0.12.76 window close') { throw }; $rejected = $true
    }
    if (-not $rejected) { throw 'R261_ASSERT_DEADLINE: expired deadline extended' }
    Write-Output 'R261 real-window filtering and existing deadline retained PASS'
    $oldCondition = '$closed.Count -gt 0 -and $processes.Count -eq 0'
    $mutated = $source.Replace($oldCondition,'(Test-Path -LiteralPath $BoundsFile)')
    if ($mutated -eq $source) { throw 'Mutation did not change implementation' }
    $killed = $false
    try { Assert-CloseRace ([ScriptBlock]::Create($mutated)) '{"width":500,"height":400,"x":0,"y":0}' } catch {
        if ($_.Exception.Message -notmatch '^R261_ASSERT_NORMAL_EXIT:') { throw }
        $killed = $true
    }
    if (-not $killed) { throw 'R261 existence-only mutation survived' }
    $upgrade = Get-Content (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1') -Raw
    if ($upgrade -notmatch 'Wait-R261PublishedWindowBounds' -or $upgrade -notmatch '-Deadline \$script:lastSetupDeadline' -or $upgrade -notmatch 'AddSeconds\(180\)') { throw 'R261 real upgrade must use helper and original setup deadline' }
    Write-Output 'R261 old existence-only assertion mutation killed PASS'
} finally { Remove-Item $root -Recurse -Force }
