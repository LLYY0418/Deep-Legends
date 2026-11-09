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

# R317 exercises the actual upgrade-script helpers, not a duplicate validator.
$upgrade = Get-Content (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1') -Raw
$helpers = [regex]::Match($upgrade, '(?s)function Get-R317ScaleFitEvents.*?(?=\r?\nfunction Run-Setup)')
if (-not $helpers.Success) { throw 'R317 actual persistence helpers missing' }
. ([ScriptBlock]::Create($helpers.Value))
$root = Join-Path ([IO.Path]::GetTempPath()) ('r317-persistence-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $root | Out-Null
$before = '{"x":0,"y":20,"width":1024,"height":680,"maximized":false}'
$after = '{"x":0,"y":0,"width":1024,"height":720,"maximized":false}'
$since = [DateTime]::Parse('2026-11-01T19:09:11Z').ToUniversalTime()
$file = Join-Path $root 'window-bounds.json'
$log = Join-Path $root 'desktop.log'
function Write-R317Event($Event, [string]$At = '2026-11-01T19:09:12.326Z') {
    [IO.File]::AppendAllText($log, "$At window_scale_fit " + ($Event | ConvertTo-Json -Compress -Depth 6) + "`n")
}
function New-R317Clamp {
    return @{event='window_scale_fit';from_scale=1;to_scale=1.25;clamped=$true;before_bounds=($before | ConvertFrom-Json);after_bounds=($after | ConvertFrom-Json);work_area=@{x=0;y=0;width=1024;height=720}}
}
function Reset-R317Fixture {
    [IO.File]::WriteAllText($file,$before)
    [IO.File]::WriteAllText($log,'')
    $script:beforeHash = (Get-FileHash $file -Algorithm SHA256).Hash
}
function Wait-R317Fixture([ScriptBlock]$OnPause = {}) {
    $state = @{ms=0;intervals=@()}
    $clock = { return $state.ms }.GetNewClosure()
    $pause = { param($Milliseconds); $state.intervals+=@($Milliseconds);$state.ms+=$Milliseconds; & $OnPause $state.ms }.GetNewClosure()
    $snapshot = Wait-R317StableBounds $file $root $since (Join-Path $root 'stable.json') -Clock $clock -Pause $pause
    return @{snapshot=$snapshot;ms=$state.ms;intervals=$state.intervals}
}
function Assert-R317Rejected([ScriptBlock]$Action, [string]$Path = $file) {
    $failed = $false
    try { & $Action | Out-Null } catch {
        if ($_.Exception.Message -ne "R238 persistent fixture changed: $Path") { throw }
        $failed = $true
    }
    if (-not $failed) { throw 'R317 invalid persistent file accepted' }
}
try {
    Reset-R317Fixture
    $wait = Wait-R317Fixture
    $record = Assert-R317PersistentFile $file $beforeHash $before $wait.snapshot
    if ($wait.ms -ne 1500 -or $wait.intervals.Count -ne 5 -or @($wait.intervals | Where-Object { $_ -ne 300 }).Count -or $record.explained_by -ne 'unchanged' -or $null -ne $record.matched_event) { throw 'R317 unchanged file or stable interval contract failed' }
    Write-Output 'R317 unchanged window bounds pass after 1500ms at 300ms intervals PASS'

    Reset-R317Fixture
    # File hash and event count change independently; both must reset stability.
    $changing = { param($Ms);if ($Ms -eq 300) { [IO.File]::WriteAllText($file,$after) };if ($Ms -eq 600) { Write-R317Event (New-R317Clamp) } }
    $wait = Wait-R317Fixture $changing
    $record = Assert-R317PersistentFile $file $beforeHash $before $wait.snapshot
    if ($wait.ms -ne 2100 -or $record.explained_by -ne 'window_scale_fit_clamp' -or $record.matched_event.to_scale -ne 1.25 -or $record.matched_event.before_bounds.y -ne 20 -or $record.matched_event.after_bounds.height -ne 720 -or $record.matched_event.work_area.width -ne 1024) { throw 'R317 clamp or event-count stability evidence failed' }
    Write-Output 'R317 only exact stable window_scale_fit clamp passes with matched event evidence PASS'

    foreach ($invalid in @('unrelated','missing-event','wrong-before','wrong-after','not-clamped','outside-work-area','maximized','missing-file','bad-json','missing-maximized','stale-event','latest-event','malformed-latest-event','string-coordinate','missing-scale','invalid-scale')) {
        Reset-R317Fixture
        [IO.File]::WriteAllText($file,$after)
        $event = New-R317Clamp
        switch ($invalid) {
            'unrelated' { [IO.File]::WriteAllText($file,'{"x":4,"y":8,"width":600,"height":400,"maximized":false}') }
            'wrong-before' { $event.before_bounds.y=19 }
            'wrong-after' { $event.after_bounds.height=719 }
            'not-clamped' { $event.clamped=$false }
            'outside-work-area' { $event.work_area.height=719 }
            'maximized' { [IO.File]::WriteAllText($file,$after.Replace('false','true')) }
            'missing-file' { Remove-Item $file }
            'bad-json' { [IO.File]::WriteAllText($file,'{"width":') }
            'missing-maximized' { [IO.File]::WriteAllText($file,'{"x":0,"y":0,"width":1024,"height":720}') }
            'string-coordinate' { $event.before_bounds.x='0' }
            'missing-scale' { $event.Remove('from_scale') }
            'invalid-scale' { $event.to_scale='1.25' }
        }
        if ($invalid -eq 'stale-event') { Write-R317Event $event '2026-11-01T19:09:10Z' }
        elseif ($invalid -ne 'missing-event') { Write-R317Event $event }
        if ($invalid -eq 'latest-event') { $event.clamped=$false;Write-R317Event $event '2026-11-01T19:09:12.400Z' }
        if ($invalid -eq 'malformed-latest-event') { [IO.File]::AppendAllText($log, '2026-11-01T19:09:12.500Z window_scale_fit {bad'+"`n") }
        $wait = Wait-R317Fixture
        Assert-R317Rejected { Assert-R317PersistentFile $file $beforeHash $before $wait.snapshot }
    }
    Write-Output 'R317 unrelated/missing/malformed/maximized/out-of-area/stale/latest-event bounds rejected PASS'

    # Each non-bounds sentinel still requires exact byte identity, even with a valid clamp.
    foreach ($name in @('r238-collection-sentinel.json','r238-cache-sentinel.json','r238-settings-sentinel.json','ui-scale.json')) {
        $other = Join-Path $root $name
        [IO.File]::WriteAllText($other,'{"sentinel":true}')
        $hash = (Get-FileHash $other -Algorithm SHA256).Hash
        $record = Assert-R317PersistentFile $other $hash
        if ($record.before_sha256 -cne $record.after_sha256) { throw 'R317 strict sentinel identity failed' }
        [IO.File]::WriteAllText($other,'{"sentinel":false}')
        Assert-R317Rejected { Assert-R317PersistentFile $other $hash $before $wait.snapshot } $other
        Remove-Item $other
        Assert-R317Rejected { Assert-R317PersistentFile $other $hash $before $wait.snapshot } $other
    }
    Write-Output 'R317 any change or missing file in all four other sentinels fails PASS'

    Reset-R317Fixture
    $state = @{ms=0;intervals=@()}
    $clock = { $state.ms }.GetNewClosure()
    $pause = { param($Milliseconds);$state.intervals+=@($Milliseconds);$state.ms+=$Milliseconds;[IO.File]::AppendAllText($file,' ') }.GetNewClosure()
    Assert-R317Rejected { Wait-R317StableBounds $file $root $since (Join-Path $root 'timeout.json') -Clock $clock -Pause $pause }
    $record = Get-Content (Join-Path $root 'timeout.json') -Raw | ConvertFrom-Json
    if ($state.ms -ne 10000 -or $record.outcome -ne 'failure' -or $record.timeout_ms -ne 10000 -or $state.intervals[-1] -ne 100) { throw 'R317 original 10s ceiling failed' }
    Write-Output 'R317 never-stable bounds fail at 10s without deadline extension PASS'

    # Later candidates use the same validation; existing scale migration and UI guards stay in place.
    if ([regex]::Matches($upgrade,'\$boundsSnapshot = Wait-R317StableBounds').Count -ne 5 -or [regex]::Matches($upgrade,'\$boundsFitSince = \[DateTime\]::UtcNow').Count -ne 5) { throw 'R317 all five candidate chains must wait and scope their events' }
    foreach ($prefix in @('actual079','actual080','actual081','actualPublished082')) {
        if ($upgrade -notmatch ('Assert-R317PersistentFile \$file \$'+$prefix+'Files\[\$file\]')) { throw "R317 missing shared validator: $prefix" }
    }
    foreach ($anchor in @('Published 081 zoom was not migrated to 2','Published 082 zoom was not migrated to 2','Actual 079 persisted file changed after upgrade','Actual 080 persisted file changed after upgrade','Get-R252DiagnosticTimeMilliseconds $_.time')) {
        if (-not $upgrade.Contains($anchor)) { throw "R317 unrelated original assertion missing: $anchor" }
    }
    Write-Output 'R317 all candidate chains share bounds validation; scale migration and unrelated assertions retained PASS'
} finally { Remove-Item $root -Recurse -Force }

# R319：安装器会在 clamp 后追加 no-op window_scale_fit；解释逻辑必须选最后一条
# 真正 clamped 且 before_bounds 等于安装前快照的事件，不能整体豁免 bounds 文件。
$r319Root = Join-Path ([IO.Path]::GetTempPath()) ('r319-bounds-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $r319Root | Out-Null
$r206Source = Get-Content (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1') -Raw
$r317Start = $r206Source.IndexOf('function Test-R317Rectangle')
$r317End = $r206Source.IndexOf('function Run-Setup')
if ($r317Start -lt 0 -or $r317End -le $r317Start) { throw 'R319_WINDOW_BOUNDS: explanation helpers missing' }
. ([ScriptBlock]::Create($r206Source.Substring($r317Start, $r317End - $r317Start)))
function Invoke-R319BoundsCase([object[]]$Events, [string]$BeforeBounds, [string]$CurrentBounds) {
    $dir = Join-Path $r319Root ('r319-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory $dir | Out-Null
    $file = Join-Path $dir 'window-bounds.json'
    [IO.File]::WriteAllText($file, $CurrentBounds)
    $hash = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash
    $snapshot = [pscustomobject]@{read_error=$null;sha256=$hash;text=$CurrentBounds;events=$Events}
    return Assert-R317PersistentFile $file 'BEFORE' $BeforeBounds $snapshot
}
$before = '{"width":1024,"height":680,"x":0,"y":20,"maximized":false}'
$current = '{"width":1024,"height":720,"x":0,"y":0,"maximized":false}'
$clamped = @{at='2026-11-08T15:58:12.806Z';value=@{event='window_scale_fit';from_scale=1;to_scale=1.25;before_bounds=@{x=0;y=20;width=1024;height=680};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped=$true}}
$noop = @{at='2026-11-08T15:58:12.838Z';value=@{event='window_scale_fit';from_scale=1.25;to_scale=1.25;before_bounds=@{x=0;y=0;width=1024;height=720};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped=$false}}
$record = Invoke-R319BoundsCase @($noop,$clamped,$noop) $before $current
if ($record.explained_by -ne 'window_scale_fit_clamp' -or $record.matched_event.from_scale -ne 1 -or $record.matched_event.to_scale -ne 1.25 -or $record.trailing_noops -ne 1) { throw 'R319_ASSERT_CLAMP: clamped event/trailing noops not selected' }
$leading = @{at='2026-11-08T15:58:12.700Z';value=@{event='window_scale_fit';from_scale=1;to_scale=1;before_bounds=@{x=0;y=20;width=1024;height=680};after_bounds=@{x=0;y=20;width=1024;height=680};work_area=@{x=0;y=0;width=1024;height=720};clamped=$false}}
$record = Invoke-R319BoundsCase @($leading,$clamped,$noop) $before $current
if ($record.trailing_noops -ne 1 -or $record.matched_event.from_scale -ne 1) { throw 'R319_ASSERT_LEADING: leading no-op changed the explanation' }
$rejected = $false
try { $null = Invoke-R319BoundsCase @($noop) $before $current } catch { if ($_.Exception.Message -notmatch 'persistent fixture changed') { throw }; $rejected = $true }
if (-not $rejected) { throw 'R319_ASSERT_NOOP: no-op event accepted as explanation' }
$wrongBefore = @{at='x';value=@{event='window_scale_fit';from_scale=1;to_scale=1.25;before_bounds=@{x=0;y=0;width=1024;height=680};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped=$true}}
$rejected = $false
try { $null = Invoke-R319BoundsCase @($wrongBefore) $before $current } catch { if ($_.Exception.Message -notmatch 'persistent fixture changed') { throw }; $rejected = $true }
if (-not $rejected) { throw 'R319_ASSERT_BEFORE: mismatched before_bounds accepted' }
function Assert-R319Rejected($Events) {
    $failed = $false
    try { $null = Invoke-R319BoundsCase $Events $before $current } catch { if ($_.Exception.Message -notmatch 'persistent fixture changed') { throw }; $failed = $true }
    if (-not $failed) { throw 'R319 invalid trailing/no-op case accepted' }
}
$latest = @{at='2026-11-08T15:58:12.900Z';value=@{event='window_scale_fit';from_scale=1;to_scale=1.25;before_bounds=@{x=0;y=20;width=1024;height=680};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped=$false}}
$badNoops = @(
    @{at='a';value=@{event='window_scale_fit';from_scale=1.25;to_scale=1.25;before_bounds=@{x=0;y=0;width=1024;height=720};after_bounds=@{x=0;y=0;width=1024;height=721};work_area=@{x=0;y=0;width=1024;height=720};clamped=$false}},
    @{at='b';value=@{event='window_scale_fit';from_scale=1.25;to_scale=1.3;before_bounds=@{x=0;y=0;width=1024;height=720};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped=$false}},
    @{at='c';value=@{event='window_scale_fit';from_scale=1.25;to_scale=1.25;before_bounds=@{x=0;y=0;width=800;height=720};after_bounds=@{x=0;y=0;width=800;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped=$false}},
    @{at='d';value=@{event='window_scale_fit';from_scale=1.25;to_scale=1.25;before_bounds=@{x=0;y=0;width=1024;height=720};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720};clamped='false'}},
    @{at='e';value=@{event='window_scale_fit';from_scale=1.25;to_scale=1.25;before_bounds=@{x=0;y=0;width=1024;height=720};after_bounds=@{x=0;y=0;width=1024;height=720};work_area=@{x=0;y=0;width=1024;height=720}}}
)
foreach ($bad in $badNoops) { Assert-R319Rejected @($clamped,$bad) }
Assert-R319Rejected @($clamped,$noop,$latest)
Assert-R319Rejected @($noop)
Assert-R319Rejected @($wrongBefore,$noop)
Write-Output 'R319 trailing no-op matrix (valid no-op, leading no-op, malformed/mismatch/latest/no-clamp) PASS'
# 变异：只检查 clamped=false，latest-event 必须被误接受并使负例失败。
function Test-R319TrueNoOp($Value, $Current) {
    return ($null -ne $Value -and $Value.clamped -is [bool] -and $Value.clamped -eq $false)
}
$killed = $false
try { Assert-R319Rejected @($clamped,$latest) } catch { if ($_.Exception.Message -notmatch 'R319 invalid trailing/no-op case accepted') { throw }; $killed = $true }
if (-not $killed) { throw 'R319_ASSERT_MUTATION: clamped-only no-op condition survived' }
Write-Output 'R319 clamped-only mutation rejected latest-event PASS'
Remove-Item $r319Root -Recurse -Force
Write-Output 'R319 clamped/after-noop explanation selection and mismatched-before rejection PASS'
