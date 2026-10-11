param([Parameter(Mandatory=$true)][string]$Setup)
$ErrorActionPreference = "Stop"
# All installed versions inherit the fixed fake key, including published builds.
$env:RIOT_API_KEY = 'RGAPI-00000000-0000-0000-0000-000000000000'
. (Join-Path $PSScriptRoot 'r252-diagnostic-time.ps1')
. (Join-Path $PSScriptRoot 'r261-window-bounds.ps1')
. (Join-Path $PSScriptRoot 'r271-installer-diagnostics.ps1')
# The candidate is whatever desktop/package.json declares, so a version bump
# needs no edits here.
$candidateVersion = (& node -p "require('./desktop/package.json').version").Trim()
if ($LASTEXITCODE -ne 0 -or $candidateVersion -notmatch '^\d+\.\d+\.\d+$' -or $candidateVersion -eq '0.12.76') { throw "Invalid candidate version: $candidateVersion" }
$root = Join-Path $env:RUNNER_TEMP "deep-legends-r206-real-upgrade"
$install = Join-Path $root "installed"
$data = Join-Path $root "data"
$evidence = Join-Path $root "evidence"
New-Item -ItemType Directory -Force $install,$data,$evidence | Out-Null
Save-R271RunnerHost $evidence
$setupSequence = 0
$appDirectories = @($install, (Join-Path $root 'installed-079'), (Join-Path $root 'installed-080'), (Join-Path $root 'installed-081-2.25'), (Join-Path $root 'installed-081-2.5'))
$debugPorts = @()
$cleanupSequence = 0
$launchSequence = 0
$env:LOL_LOOT_DATA_DIR = $data
$old = Join-Path $root "Deep-Legends-Setup-0.12.65-public.exe"
Invoke-WebRequest "https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.65/Deep-Legends-Setup-0.12.65-public.exe" -OutFile $old
if ((Get-FileHash $old -Algorithm SHA256).Hash.ToLowerInvariant() -ne "bf97221a0c8465b78d57bf32f15c746eeec72deb7f3ec4923323b095c16c70ac") { throw "Old public setup checksum mismatch" }
function Show-PersistedFailure([string]$Log) {
    Get-Content -LiteralPath $Log -Tail 80 | ForEach-Object {
        ($_ -replace 'RGAPI-[A-Za-z0-9-]+', '[riot-key]' -replace '(?i)([A-Z]:\\Users\\|/Users/|/home/)[^\\/\s]+', '$1[user]')
    }
}
function Run-Setup([string]$File, [Parameter(Mandatory=$true)][string]$Version, [switch]$MonitorLegacy) {
    # These are the actual Go installer shells. --update auto-starts their
    # existing progress flow; /S alone would leave the Go setup page waiting.
    $script:setupSequence++
    $prefix = Join-Path $evidence ("r271-setup-" + $Version + '-' + $script:setupSequence)
    $started = Get-Date
    $tempDirectory = if ($env:TMP) { $env:TMP } elseif ($env:TEMP) { $env:TEMP } else { [IO.Path]::GetTempPath() }
    $startupLog = Join-Path $tempDirectory 'DeepLegendsSetup-startup.log'
    $process = Start-Process -FilePath $File -ArgumentList "--update --dest `"$install`"" -PassThru -RedirectStandardError ($prefix + '-stderr.log') -RedirectStandardOutput ($prefix + '-stdout.log')
    [ordered]@{ version = $Version; installer = $File; process_id = $process.Id; started_at = $started.ToUniversalTime().ToString('o'); stderr = $prefix + '-stderr.log'; stdout = $prefix + '-stdout.log'; startup_log = $startupLog } | ConvertTo-Json | ForEach-Object { ConvertTo-R271SafeText $_ } | Set-Content -LiteralPath ($prefix + '-launch.json') -Encoding utf8
    $deadline=(Get-Date).AddSeconds(180)
    $script:lastSetupDeadline=$deadline
    $snapshots=@(); $last=""
    while (-not $process.WaitForExit(50)) {
        if ((Get-Date) -gt $deadline) { Stop-Process -Id $process.Id -Force; throw "Real installer timed out" }
        if ($MonitorLegacy) {
            Get-ChildItem $env:TEMP -Filter 'update-install-stages.txt' -Recurse -ErrorAction SilentlyContinue | ForEach-Object {
                try {
                    $lines=@(Get-Content $_.FullName -ErrorAction Stop)
                    $text=$lines -join "`n"
                    if ($text -ne $last) { $snapshots+=@{observed_at=(Get-Date).ToUniversalTime().ToString('o');stage_lines=$lines}; $last=$text }
                } catch {}
            }
        }
    }
    $process.WaitForExit() # Flush the redirected readers after the original bounded wait.
    if ($MonitorLegacy) { ConvertTo-Json -InputObject @($snapshots) -Depth 8 | Set-Content (Join-Path $evidence 'legacy-0.12.68-nsis-snapshots.json') }
    $exitCode = $process.ExitCode
    $result = [ordered]@{ version = $Version; installer = $File; process_id = $process.Id; exit_code = $exitCode; completed_at = [DateTime]::UtcNow.ToString('o'); stderr_bytes = (Get-Item -LiteralPath ($prefix + '-stderr.log')).Length; stdout_bytes = (Get-Item -LiteralPath ($prefix + '-stdout.log')).Length }
    $result | ConvertTo-Json | ForEach-Object { ConvertTo-R271SafeText $_ } | Set-Content -LiteralPath ($prefix + '-result.json') -Encoding utf8
    Write-Output ('R271_SETUP_RESULT ' + (ConvertTo-R271SafeText ($result | ConvertTo-Json -Compress)))
    if (Test-Path -LiteralPath $startupLog) { Copy-Item -LiteralPath $startupLog -Destination ($prefix + '-startup.log') }
    if ($exitCode -ne 0) {
        try { Save-R271SetupFailure -File $File -Version $Version -Process $process -StartedAt $started -Prefix $prefix -StartupLog $startupLog }
        catch { Write-Output ('Installer diagnostic collection failed: ' + (ConvertTo-R271SafeText $_.Exception.Message)) }
        throw "Real installer failed: $exitCode"
    }
    if (-not (Test-Path (Join-Path $install "Deep Legends.exe"))) { throw "Installed executable missing" }
}
function Get-R265AppState {
    $all = @(Get-CimInstance Win32_Process)
    $owned = @($all | Where-Object {
        $executable = $_.ExecutablePath
        $inside = $false
        foreach ($directory in $appDirectories) {
            if ($executable -and $executable.StartsWith($directory.TrimEnd('\')+'\', [StringComparison]::OrdinalIgnoreCase)) { $inside = $true }
        }
        $inside -and $_.Name -in @('Deep Legends.exe','loot-service.exe')
    })
    $ports = @(47391) + @($debugPorts)
    $listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -in $ports -or $_.OwningProcess -in @($owned.ProcessId) })
    $locks = @()
    if ($userData -and (Test-Path $userData)) {
        foreach ($relative in @('SingletonLock','SingletonCookie','SingletonSocket','Local Storage/leveldb/LOCK')) {
            $lockPath = Join-Path $userData $relative
            if (Test-Path -LiteralPath $lockPath) { $item=Get-Item -LiteralPath $lockPath -Force; $locks+=@{path=$item.FullName;last_write_utc=$item.LastWriteTimeUtc.ToString('o');bytes=$item.Length} }
        }
    }
    # Include other installs by name and every port owner, but never kill an
    # unrelated installation. Its presence must fail the isolated fixture.
    $related = @($all | Where-Object { $_.Name -in @('Deep Legends.exe','loot-service.exe') -or $_.ProcessId -in @($listeners.OwningProcess) })
    return @{at=(Get-Date).ToUniversalTime().ToString('o');install=$install;user_data=$userData;owned=@($owned | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CreationDate);processes=@($related | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CreationDate);listeners=@($listeners | Select-Object LocalAddress,LocalPort,OwningProcess);lock_files=$locks}
}
function Save-R265State([string]$Name) {
    $state = Get-R265AppState
    $state | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $evidence $Name) -Encoding utf8
    return $state
}
function Stop-InstalledApp([switch]$ObserveOnly) {
    $script:cleanupSequence++
    $started=Get-Date; $deadline=$started.AddSeconds(5)
    $observations=@(); $previous=''; $outcome='failure'
    try {
        do {
            $state=Get-R265AppState
            $key=($state.processes | ConvertTo-Json -Compress -Depth 4)+'|'+($state.listeners | ConvertTo-Json -Compress -Depth 4)
            if ($key -ne $previous) { $observations+=@($state); $previous=$key }
            if ($state.processes.Count -eq 0 -and $state.listeners.Count -eq 0) { $outcome='success'; return }
            if (-not $ObserveOnly) {
                # Re-snapshot after killing: a renderer/backend can be spawned
                # while the old main process is being terminated.
                foreach ($process in $state.owned) { Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue }
            }
            Start-Sleep -Milliseconds 100
        } while ((Get-Date) -lt $deadline)
        if ($ObserveOnly) { throw 'Product processes or ports remain after normal close; no cleanup retry permitted' }
        throw 'Installed fixture did not stop completely, or a foreign instance owns its ports'
    } finally {
        @{started_at=$started.ToUniversalTime().ToString('o');ended_at=(Get-Date).ToUniversalTime().ToString('o');elapsed_ms=[int]((Get-Date)-$started).TotalMilliseconds;observe_only=[bool]$ObserveOnly;outcome=$outcome;observations=$observations} | ConvertTo-Json -Depth 12 | Set-Content (Join-Path $evidence ("r265-stop-"+$cleanupSequence+".json")) -Encoding utf8
    }
}
function Wait-R265InstallerLaunch {
    # The installer hands off asynchronously. Do not mistake a pre-launch empty
    # process snapshot for a completed stop, then race its auto-start for the
    # shared Electron single-instance lock. Keep the existing installer deadline.
    $started=Get-Date; $observations=@(); $outcome='failure'
    try {
        do {
            $state=Get-R265AppState
            $main=@($state.owned | Where-Object { $_.ExecutablePath -eq (Join-Path $install 'Deep Legends.exe') })
            $backendPids=@($state.owned | Where-Object { $_.Name -eq 'loot-service.exe' -and $_.ExecutablePath.StartsWith($install.TrimEnd('\')+'\', [StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { $_.ProcessId })
            $observations+=@(@{at=$state.at;main_pids=@($main.ProcessId);backend_pids=$backendPids;listeners=$state.listeners})
            if ($main.Count -gt 0 -and @($state.listeners | Where-Object { $_.OwningProcess -in $backendPids }).Count -gt 0) { $outcome='success'; return }
            Start-Sleep -Milliseconds 100
        } while ((Get-Date) -lt $script:lastSetupDeadline)
        throw 'Installer auto-start did not acquire its own backend within the original installer deadline'
    } finally {
        @{started_at=$started.ToUniversalTime().ToString('o');ended_at=(Get-Date).ToUniversalTime().ToString('o');installer_deadline=$script:lastSetupDeadline.ToUniversalTime().ToString('o');install=$install;outcome=$outcome;observations=$observations} | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $evidence ("r265-installer-launch-"+$cleanupSequence+".json")) -Encoding utf8
    }
}
function Assert-InstalledVersion([string]$Version) {
    $exe = Get-Item (Join-Path $install 'Deep Legends.exe')
    $package = (& node -e "console.log(require('./desktop/node_modules/@electron/asar').extractFile(process.argv[1],'package.json').toString())" (Join-Path $install 'resources/app.asar')) | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or -not $package.version) { throw 'Cannot read installed Electron package version' }
    $record = @{expected=$Version;path=$exe.FullName;product_version=$exe.VersionInfo.ProductVersion;file_version=$exe.VersionInfo.FileVersion;asar_version=$package.version;exe_sha256=(Get-FileHash $exe.FullName -Algorithm SHA256).Hash.ToLowerInvariant();last_write_utc=$exe.LastWriteTimeUtc.ToString('o')}
    $record | ConvertTo-Json | Set-Content (Join-Path $evidence ("installed-version-"+$Version+".json"))
    # electron-builder writes a four-component PE ProductVersion, while the
    # FileVersion and package version are the release's exact three components.
    if ($record.product_version -ne "$Version.0" -or $record.file_version -ne $Version -or $record.asar_version -ne $Version) { throw ("Unexpected installed version: "+($record | ConvertTo-Json -Compress)) }
}
try {
    Run-Setup $old -Version '0.12.65'
    Stop-InstalledApp
    # Reproduce the reported 0.12.65 -> 0.12.68 path before upgrading to R206.
    # Capture its private TEMP stage file while the real installer is running;
    # the old wrapper removes that workspace at completion.
    $legacy = Join-Path $root 'Deep-Legends-Setup-0.12.68-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.68/Deep-Legends-Setup-0.12.68-public.exe' -OutFile $legacy
    if ((Get-FileHash $legacy -Algorithm SHA256).Hash.ToLowerInvariant() -ne '8fa7dd821cfb5318634bfca14c26f922553d5c3ab62ddf8f194910be6f4b7b76') { throw 'Legacy public setup checksum mismatch' }
    $originalTemp=$env:TEMP; $originalTmp=$env:TMP
    $env:TEMP=Join-Path $root 'legacy-temp'; $env:TMP=$env:TEMP
    New-Item -ItemType Directory -Force $env:TEMP | Out-Null
    try { Run-Setup $legacy -Version '0.12.68' -MonitorLegacy } finally { $env:TEMP=$originalTemp; $env:TMP=$originalTmp }
    Start-Sleep -Seconds 2
    $legacyDiagnostics=Join-Path $data 'logs/diagnostics.jsonl'
    if (Test-Path $legacyDiagnostics) { Get-Content $legacyDiagnostics | ForEach-Object {try {$row=$_ | ConvertFrom-Json; if ($row.event -eq 'update_install_timing') {$_}} catch {}} | Set-Content (Join-Path $evidence 'legacy-install-timing.jsonl') }
    Stop-InstalledApp
    # R238: install the exact published 0.12.76 public release before the candidate.
    $published076 = Join-Path $root 'Deep-Legends-Setup-0.12.76-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.76/Deep-Legends-Setup-0.12.76-public.exe' -OutFile $published076
    if ((Get-FileHash $published076 -Algorithm SHA256).Hash.ToLowerInvariant() -ne '01014312b60e591a05bfc87f86e098adf6c5fc5e59520db5aedac5b5dba02ff3') { throw 'Published 0.12.76 setup checksum mismatch' }
    Run-Setup $published076 -Version '0.12.76'
    Assert-InstalledVersion '0.12.76'
    Stop-InstalledApp
    $published080 = Join-Path $root 'Deep-Legends-Setup-0.12.80-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.80/Deep-Legends-Setup-0.12.80-public.exe' -OutFile $published080
    if ((Get-FileHash $published080 -Algorithm SHA256).Hash.ToLowerInvariant() -ne 'e62a12f286cef2a5b8dabaaf99dcd8de50557ba34f5c1b84cde3fa0c86744aaf') { throw 'Published 0.12.80 setup checksum mismatch' }
    Run-Setup $published080 -Version '0.12.80'
    Assert-InstalledVersion '0.12.80'
    Stop-InstalledApp
    $published081 = Join-Path $root 'Deep-Legends-Setup-0.12.81-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.81/Deep-Legends-Setup-0.12.81-public.exe' -OutFile $published081
    if ((Get-FileHash $published081 -Algorithm SHA256).Hash.ToLowerInvariant() -ne '7dd3e6eb22aeb75c9e62deddd090231f8c93fa62e92de90bfc1baa174085862d') { throw 'Published 0.12.81 setup checksum mismatch' }
    Run-Setup $published081 -Version '0.12.81'
    Assert-InstalledVersion '0.12.81'
    $userDataName = (& node -e "const p=JSON.parse(require('./desktop/node_modules/@electron/asar').extractFile(process.argv[1],'package.json'));console.log(p.productName||p.name)" (Join-Path $install 'resources/app.asar')).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $userDataName) { throw 'Cannot determine installed Electron userData name' }
    $userData = Join-Path $env:APPDATA $userDataName
    # Use the state actually persisted by 0.12.76 on this display. An invented
    # 1050x750 rectangle can be clamped by the CI desktop and legitimately saved
    # at another size; that does not measure whether the upgrade retained it.
    $boundsFile = Join-Path $userData 'window-bounds.json'
    # Wait for a real normal close and for all Electron writers to exit before
    # reading the final snapshot. Existence alone also matches a truncated file.
    $publishedBounds = Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline $script:lastSetupDeadline -EvidencePath (Join-Path $evidence '076-window-bounds-wait.json')
    Stop-InstalledApp -ObserveOnly
    if (-not (Test-Path $userData)) { throw '0.12.76 did not initialize its actual Electron userData directory' }
    if (-not (Test-Path $boundsFile)) { throw '0.12.76 did not persist its actual window bounds' }
    $publishedBoundsValue = $publishedBounds | ConvertFrom-Json
    if ($publishedBoundsValue.width -le 0 -or $publishedBoundsValue.height -le 0 -or $null -eq $publishedBoundsValue.x -or $null -eq $publishedBoundsValue.y) { throw 'Invalid actual 0.12.76 window bounds' }
    [IO.File]::WriteAllText((Join-Path $evidence '076-window-bounds-before.json'),$publishedBounds,(New-Object Text.UTF8Encoding($false)))
    $sentinels = @{}
    # Distinct safe fixtures in real persistent directories, without a real account.
    foreach ($entry in @(@($data,'season-stats/r238-cache-sentinel.json','{"schemaVersion":3,"r238":"season-cache"}'),@($data,'snapshots/r238-collection-sentinel.json','{"r238":"collection"}'),@($userData,'r238-settings-sentinel.json','{"r238":"preferences"}'),@($userData,'window-bounds.json',$publishedBounds),@($userData,'ui-scale.json','{"mode":"fixed","value":1.25,"defaultAuto":1}'))) {
        $file=Join-Path $entry[0] $entry[1]
        New-Item -ItemType Directory -Force (Split-Path -Parent $file) | Out-Null
        [IO.File]::WriteAllText($file,$entry[2],(New-Object Text.UTF8Encoding($false)))
        $sentinels[$file]=(Get-FileHash $file -Algorithm SHA256).Hash
    }
    # Production updater Check/Download/Apply, using a candidate HTTP fixture:
    # The candidate is not published, so the anonymous Latest endpoint stays 0.12.76.
    $env:R238_UPGRADE_SETUP=(Resolve-Path $Setup).Path
    $env:R238_UPGRADE_INSTALL=$install
    $env:R238_UPGRADE_DATA=$data
    $env:R238_UPGRADE_EVIDENCE=$evidence
    & go test -count=1 -run '^TestR238DefaultOnline076To077$' ./backend *> (Join-Path $evidence 'online-076-077-test.log')
    if ($LASTEXITCODE -ne 0) { throw 'R238 candidate online updater check/download/hash/handoff failed' }
    $online=Get-Content -Raw (Join-Path $evidence 'online-076-077.json') | ConvertFrom-Json
    if ((Get-FileHash $online.download -Algorithm SHA256).Hash.ToLowerInvariant() -ne $online.sha256) { throw 'R238 verified online download changed before installation' }
    $desktopLink = Join-Path ([Environment]::GetFolderPath('Desktop')) "Deep Legends.lnk"
    $menuLink = Join-Path ([Environment]::GetFolderPath('Programs')) "Deep Legends.lnk"
    $created = @{}
    foreach ($link in @($desktopLink,$menuLink)) { if (-not (Test-Path $link)) {throw "Old installation shortcut missing"}; $created[$link] = (Get-Item $link).CreationTimeUtc.Ticks }
    $diagnosticFile=Join-Path $data 'logs/diagnostics.jsonl'
    $expectedFingerprint = (& node desktop/source-fingerprint.cjs).Trim()
    if ($LASTEXITCODE -ne 0 -or $expectedFingerprint -notmatch '^[0-9a-f]{12}$') { throw 'Cannot determine candidate fingerprint' }
    foreach ($stageFile in @('update-install-stages.txt','update-install-nsis-stages.txt','update-install-timing.json')) {
        $stagePath=Join-Path $data $stageFile
        if (Test-Path $stagePath) { Copy-Item $stagePath (Join-Path $evidence ("076-before-"+$stageFile)); Remove-Item $stagePath }
    }
    Run-Setup $online.download -Version $candidateVersion
    # Keep the real installer handoff alive until its once-only buffered timing
    # event is flushed. Killing/relaunching it here can destroy that evidence.
    Assert-InstalledVersion $candidateVersion
    $stages = @{}
    Get-Content (Join-Path $data "update-install-stages.txt") | ForEach-Object { $pair=$_ -split '=',2; $stages[$pair[0]]=[long]$pair[1] }
    $order = @('installer_start','parent_exited','uninstall_old_start','uninstall_old_done','extract_start','extract_done','copy_done','relaunch')
    $previous=0L
    foreach ($stage in $order) { if (-not $stages.ContainsKey($stage) -or $stages[$stage] -lt $previous) {throw "Missing/non-monotonic stage: $stage"}; $previous=$stages[$stage] }
    $raw = Get-Content (Join-Path $data "update-install-nsis-stages.txt")
    if (-not ($raw -match '^uninstall_old_done=')) {throw "NSIS did not write uninstall_old_done"}
    $deadline=(Get-Date).AddSeconds(30)
    do {
        $rows = @()
        # A line offset becomes invalid after rotation. Read current/archived
        # generations and accept only this fingerprint AND this exact install.
        Get-ChildItem (Join-Path $data 'logs') -Filter 'diagnostics*.jsonl' -ErrorAction SilentlyContinue | Where-Object { $_.Name -match '^diagnostics(?:\.[1-5])?\.jsonl$' } | ForEach-Object {
            $rows += @(Get-Content $_.FullName | Where-Object { $_ -match '"event"\s*:\s*"update_install_timing"' } | ForEach-Object {try {$_ | ConvertFrom-Json} catch {}})
        }
        $timing = @($rows | Where-Object { $_.event -eq 'update_install_timing' -and $_.build_fingerprint -eq $expectedFingerprint -and $_.total_ms -eq ($stages['relaunch'] - $stages['installer_start']) -and (Get-R252DiagnosticTimeMilliseconds $_.time) -ge $stages['relaunch'] }) | Select-Object -Last 1
        if ($timing.result -eq 'ok') {break}; Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)
    if ($timing.result -ne 'ok' -or $null -eq $timing.uninstall_old_ms) {throw "Eight stages not imported as result=ok"}
    $comparisons = @()
    foreach ($file in $sentinels.Keys) {
        $afterHash = $null
        if (Test-Path $file) { $afterHash = (Get-FileHash $file -Algorithm SHA256).Hash }
        $comparisons += @{path=$file;before_sha256=$sentinels[$file];after_sha256=$afterHash}
    }
    $comparisons | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $evidence 'persistent-fixture-comparison.json')
    $comparisons | ConvertTo-Json -Depth 5 | Write-Output
    foreach ($file in $sentinels.Keys) { if (-not (Test-Path $file) -or (Get-FileHash $file -Algorithm SHA256).Hash -ne $sentinels[$file]) { throw "R238 persistent fixture changed: $file" } }
    $sentinels | ConvertTo-Json | Set-Content (Join-Path $evidence 'retained-persistent-fixtures.json')
    $shell = New-Object -ComObject WScript.Shell
    $icon = Join-Path $env:LOCALAPPDATA 'deep-legends/app.ico'
    if (-not (Test-Path $icon)) {throw "Stable icon missing"}
    foreach ($link in @($desktopLink,$menuLink)) {
        if ((Get-Item $link).CreationTimeUtc.Ticks -ne $created[$link]) {throw "Shortcut creation time changed"}
        $shortcut=$shell.CreateShortcut($link)
        if ($shortcut.IconLocation -ne "$icon,0") {throw "Shortcut icon location not stable"}
        if ($shortcut.TargetPath -ne (Join-Path $install 'Deep Legends.exe')) {throw "Shortcut target changed"}
    }
    $timing | ConvertTo-Json -Depth 8 | Set-Content (Join-Path $evidence 'update-install-timing-event.json')
    Copy-Item (Join-Path $data 'update-install-stages.txt'),(Join-Path $data 'update-install-nsis-stages.txt') $evidence
    @{old_version='0.12.65';intermediate_version='0.12.68';upgrade_from='0.12.81';chain=@('0.12.65','0.12.68','0.12.76','0.12.80','0.12.81',$candidateVersion);upgrade_to=$candidateVersion;candidate_online_transport='real loopback HTTP fixture';anonymous_latest_candidate='pending publication';persistent_sentinels_retained=$true;key_mode='public';stages=$order.Count;icon_location_stable=$true;created_time_changed=$false;total_ms=$timing.total_ms;uninstall_old_ms=$timing.uninstall_old_ms;copy_ms=$timing.copy_ms} | ConvertTo-Json | Set-Content (Join-Path $evidence 'real-upgrade-summary.json')
    Get-Content (Join-Path $evidence 'real-upgrade-summary.json')
    # Additional, independent published 0.12.79 -> candidate chain. Preferences
    # are written through actual installed 079 UI handlers, never injected as
    # sentinel files or restored into candidate localStorage by this harness.
    Stop-InstalledApp
    $install = Join-Path $root 'installed-079'
    New-Item -ItemType Directory -Force $install | Out-Null
    $published079 = Join-Path $root 'Deep-Legends-Setup-0.12.79-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.79/Deep-Legends-Setup-0.12.79-public.exe' -OutFile $published079
    if ((Get-FileHash $published079 -Algorithm SHA256).Hash.ToLowerInvariant() -ne 'cabacc1b4e8bc8dcf51469e1d1e84f0ef6ff8f82090084bf0c3fd6b0bb794d96') { throw 'Published 0.12.79 setup checksum mismatch' }
    Run-Setup $published079 -Version '0.12.79'
    Assert-InstalledVersion '0.12.79'
    Wait-R265InstallerLaunch
    Stop-InstalledApp
    function Start-R265DebugApp {
        Stop-InstalledApp -ObserveOnly
        $script:launchSequence++
        Save-R265State ("r265-before-launch-"+$launchSequence+".json") | Out-Null
        $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback,0)
        $listener.Start(); $debugPort=$listener.LocalEndpoint.Port; $listener.Stop()
        if ($debugPort -eq 47391) { throw 'CDP port conflicts with the stable backend origin' }
        $script:debugPorts+=@($debugPort)
        $launched=Start-Process -FilePath (Join-Path $install 'Deep Legends.exe') -ArgumentList "--remote-debugging-port=$debugPort" -PassThru
        $env:R265_EXPECTED_ELECTRON_PID=[string]$launched.Id
        @{at=(Get-Date).ToUniversalTime().ToString('o');pid=$launched.Id;install=$install;debug_port=$debugPort;backend_port=47391;user_data=$userData} | ConvertTo-Json | Set-Content (Join-Path $evidence ("r265-launch-"+$launchSequence+".json")) -Encoding utf8
        return $debugPort
    }
    $debugPort=Start-R265DebugApp
    node scripts/r265-persisted-ui.cjs $debugPort write-settings $evidence *> (Join-Path $evidence '079-unmodified-settings-write.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $evidence '079-unmodified-settings-write.log'); throw 'Unmodified published 079 UI did not write settings' }
    node scripts/r265-persisted-ui.cjs $debugPort write-presets $evidence *> (Join-Path $evidence '079-format-presets-write.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $evidence '079-format-presets-write.log'); throw 'Same-tag missing-resource fixture did not save preset format' }
    $actual079Bounds=Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $evidence '079-normal-close.json')
    Stop-InstalledApp -ObserveOnly
    $actual079Files=@{}
    foreach ($name in @('window-bounds.json','ui-scale.json')) {
        $file=Join-Path $userData $name
        if (-not (Test-Path $file)) { throw "Actual 079 persistent file missing: $name" }
        Copy-Item $file (Join-Path $evidence ("079-actual-before-"+$name)) -Force
        $actual079Files[$file]=(Get-FileHash $file -Algorithm SHA256).Hash
    }
    $candidateSetupPath=(Resolve-Path $Setup).Path
    Run-Setup $candidateSetupPath -Version $candidateVersion
    Assert-InstalledVersion $candidateVersion
    Wait-R265InstallerLaunch
    Stop-InstalledApp
    $debugPort=Start-R265DebugApp
    node scripts/r265-persisted-ui.cjs $debugPort read $evidence *> (Join-Path $evidence '080-actual-ui-read.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $evidence '080-actual-ui-read.log'); throw 'Actual 079 UI-written preferences changed after candidate upgrade' }
    $actual080Bounds=Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $evidence '080-normal-close.json')
    Stop-InstalledApp -ObserveOnly
    $debugPort=Start-R265DebugApp
    node scripts/r265-persisted-ui.cjs $debugPort restart $evidence *> (Join-Path $evidence '080-preset-restart.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $evidence '080-preset-restart.log'); throw 'Renamed preset was not retained after candidate restart' }
    node scripts/r265-persisted-ui.cjs $debugPort corrupt $evidence *> (Join-Path $evidence '080-corrupt-preset.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $evidence '080-corrupt-preset.log'); throw 'Corrupt preset damaged other settings or was not discarded' }
    Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $evidence '080-second-normal-close.json') | Out-Null
    Stop-InstalledApp -ObserveOnly
    $actual079Comparisons=@()
    foreach ($file in $actual079Files.Keys) {
        $after=(Get-FileHash $file -Algorithm SHA256).Hash
        $actual079Comparisons+=@{name=(Split-Path $file -Leaf);before_sha256=$actual079Files[$file];after_sha256=$after}
        if ($after -ne $actual079Files[$file]) { throw "Actual 079 persisted file changed after upgrade: $file" }
        Copy-Item $file (Join-Path $evidence ("080-actual-after-"+(Split-Path $file -Leaf))) -Force
    }
    $actual079Comparisons | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $evidence '079-to-080-persistent-files.json')
    @{upgrade_from='0.12.79';upgrade_to=$candidateVersion;key_mode='public';setup079_sha256='cabacc1b4e8bc8dcf51469e1d1e84f0ef6ff8f82090084bf0c3fd6b0bb794d96';settings_written_by='unmodified published 079 renderer UI';preset_scope='format compatibility only: published 079 cannot save presets; exact same-tag missing-script fixture used only before upgrade';preset_script_sha256='74699f9576ccd8686fce0e95c38b718b96718baa46bd68e9068cf947f7ea38f3';candidate_resource_fixture=$false;player_data='synthetic demo';preferences_equal=$true;persisted_files_equal=$true;preset_apply_rename_save_restart=$true;corrupt_preset_isolated=$true} | ConvertTo-Json | Set-Content (Join-Path $evidence '079-to-080-real-upgrade-summary.json')
    Get-Content (Join-Path $evidence '079-to-080-real-upgrade-summary.json')
    $r266Evidence=Join-Path $evidence '080-to-081'
    New-Item -ItemType Directory -Force $r266Evidence | Out-Null
    # Additional, independent published 0.12.80 -> candidate chain. Preferences
    # are written through actual installed 080 UI handlers, never injected as
    # sentinel files or restored into candidate localStorage by this harness.
    Stop-InstalledApp
    $install = Join-Path $root 'installed-080'
    New-Item -ItemType Directory -Force $install | Out-Null
    $published080 = Join-Path $root 'Deep-Legends-Setup-0.12.80-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.80/Deep-Legends-Setup-0.12.80-public.exe' -OutFile $published080
    if ((Get-FileHash $published080 -Algorithm SHA256).Hash.ToLowerInvariant() -ne 'e62a12f286cef2a5b8dabaaf99dcd8de50557ba34f5c1b84cde3fa0c86744aaf') { throw 'Published 0.12.80 setup checksum mismatch' }
    Run-Setup $published080 -Version '0.12.80'
    Assert-InstalledVersion '0.12.80'
    Wait-R265InstallerLaunch
    Stop-InstalledApp
    $debugPort=Start-R265DebugApp
    node scripts/r266-persisted-ui.cjs $debugPort write-settings $r266Evidence *> (Join-Path $r266Evidence '080-unmodified-settings-write.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r266Evidence '080-unmodified-settings-write.log'); throw 'Unmodified published 080 UI did not write settings' }
    node scripts/r266-persisted-ui.cjs $debugPort write-presets $r266Evidence *> (Join-Path $r266Evidence '080-format-presets-write.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r266Evidence '080-format-presets-write.log'); throw 'Published 080 UI did not save presets' }
    $actual080Bounds=Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $r266Evidence '080-normal-close.json')
    Stop-InstalledApp -ObserveOnly
    $actual080Files=@{}
    foreach ($name in @('window-bounds.json','ui-scale.json')) {
        $file=Join-Path $userData $name
        if (-not (Test-Path $file)) { throw "Actual 080 persistent file missing: $name" }
        Copy-Item $file (Join-Path $r266Evidence ("080-actual-before-"+$name)) -Force
        $actual080Files[$file]=(Get-FileHash $file -Algorithm SHA256).Hash
    }
    $candidateSetupPath=(Resolve-Path $Setup).Path
    Run-Setup $candidateSetupPath -Version $candidateVersion
    Assert-InstalledVersion $candidateVersion
    Wait-R265InstallerLaunch
    Stop-InstalledApp
    $debugPort=Start-R265DebugApp
    node scripts/r266-persisted-ui.cjs $debugPort read $r266Evidence *> (Join-Path $r266Evidence '081-actual-ui-read.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r266Evidence '081-actual-ui-read.log'); throw 'Actual 080 UI-written preferences changed after candidate upgrade' }
    $actual081Bounds=Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $r266Evidence '081-normal-close.json')
    Stop-InstalledApp -ObserveOnly
    $debugPort=Start-R265DebugApp
    node scripts/r266-persisted-ui.cjs $debugPort restart $r266Evidence *> (Join-Path $r266Evidence '081-preset-restart.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r266Evidence '081-preset-restart.log'); throw 'Renamed preset was not retained after candidate restart' }
    node scripts/r266-persisted-ui.cjs $debugPort corrupt $r266Evidence *> (Join-Path $r266Evidence '081-corrupt-preset.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r266Evidence '081-corrupt-preset.log'); throw 'Corrupt preset damaged other settings or was not discarded' }
    Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $r266Evidence '081-second-normal-close.json') | Out-Null
    Stop-InstalledApp -ObserveOnly
    $actual080Comparisons=@()
    foreach ($file in $actual080Files.Keys) {
        $after=(Get-FileHash $file -Algorithm SHA256).Hash
        $actual080Comparisons+=@{name=(Split-Path $file -Leaf);before_sha256=$actual080Files[$file];after_sha256=$after}
        if ($after -ne $actual080Files[$file]) { throw "Actual 080 persisted file changed after upgrade: $file" }
        Copy-Item $file (Join-Path $r266Evidence ("081-actual-after-"+(Split-Path $file -Leaf))) -Force
    }
    $actual080Comparisons | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $r266Evidence '080-to-081-persistent-files.json')
    @{upgrade_from='0.12.80';upgrade_to=$candidateVersion;key_mode='public';setup080_sha256='e62a12f286cef2a5b8dabaaf99dcd8de50557ba34f5c1b84cde3fa0c86744aaf';settings_written_by='unmodified published 080 renderer UI';preset_scope='actual published 080 UI-written presets, no resource substitution';candidate_resource_fixture=$false;player_data='synthetic demo';preferences_equal=$true;persisted_files_equal=$true;preset_apply_rename_save_restart=$true;corrupt_preset_isolated=$true} | ConvertTo-Json | Set-Content (Join-Path $r266Evidence '080-to-081-real-upgrade-summary.json')
    Get-Content (Join-Path $r266Evidence '080-to-081-real-upgrade-summary.json')
    foreach ($legacyScale in @('2.25','2.5')) {
    $env:R269_LEGACY_SCALE=$legacyScale
    $r269Evidence=Join-Path $evidence ("081-to-082-"+$legacyScale)
    New-Item -ItemType Directory -Force $r269Evidence | Out-Null
    # Additional, independent published 0.12.81 -> candidate chain. Preferences
    # are written through actual installed 081 UI handlers, never injected as
    # sentinel files or restored into candidate localStorage by this harness.
    Stop-InstalledApp
    $install = Join-Path $root ("installed-081-"+$legacyScale)
    New-Item -ItemType Directory -Force $install | Out-Null
    $published081 = Join-Path $root 'Deep-Legends-Setup-0.12.81-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.81/Deep-Legends-Setup-0.12.81-public.exe' -OutFile $published081
    if ((Get-FileHash $published081 -Algorithm SHA256).Hash.ToLowerInvariant() -ne '7dd3e6eb22aeb75c9e62deddd090231f8c93fa62e92de90bfc1baa174085862d') { throw 'Published 0.12.81 setup checksum mismatch' }
    Run-Setup $published081 -Version '0.12.81'
    Assert-InstalledVersion '0.12.81'
    Wait-R265InstallerLaunch
    Stop-InstalledApp
    $debugPort=Start-R265DebugApp
    node scripts/r269-persisted-ui.cjs $debugPort write-settings $r269Evidence *> (Join-Path $r269Evidence '081-unmodified-settings-write.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r269Evidence '081-unmodified-settings-write.log'); throw 'Unmodified published 081 UI did not write settings' }
    node scripts/r269-persisted-ui.cjs $debugPort write-presets $r269Evidence *> (Join-Path $r269Evidence '081-format-presets-write.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r269Evidence '081-format-presets-write.log'); throw 'Published 081 UI did not save presets' }
    $actual081Bounds=Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $r269Evidence '081-normal-close.json')
    Stop-InstalledApp -ObserveOnly
    $actual081Files=@{}
    foreach ($name in @('window-bounds.json','ui-scale.json')) {
        $file=Join-Path $userData $name
        if (-not (Test-Path $file)) { throw "Actual 081 persistent file missing: $name" }
        Copy-Item $file (Join-Path $r269Evidence ("081-actual-before-"+$name)) -Force
        $actual081Files[$file]=(Get-FileHash $file -Algorithm SHA256).Hash
    }
    $candidateSetupPath=(Resolve-Path $Setup).Path
    Run-Setup $candidateSetupPath -Version $candidateVersion
    Assert-InstalledVersion $candidateVersion
    Wait-R265InstallerLaunch
    Stop-InstalledApp
    $debugPort=Start-R265DebugApp
    node scripts/r269-persisted-ui.cjs $debugPort read $r269Evidence *> (Join-Path $r269Evidence '082-actual-ui-read.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r269Evidence '082-actual-ui-read.log'); throw 'Actual 081 UI-written preferences changed after candidate upgrade' }
    $actual082Bounds=Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $r269Evidence '082-normal-close.json')
    Stop-InstalledApp -ObserveOnly
    $debugPort=Start-R265DebugApp
    node scripts/r269-persisted-ui.cjs $debugPort restart $r269Evidence *> (Join-Path $r269Evidence '082-preset-restart.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r269Evidence '082-preset-restart.log'); throw 'Renamed preset was not retained after candidate restart' }
    node scripts/r269-persisted-ui.cjs $debugPort corrupt $r269Evidence *> (Join-Path $r269Evidence '082-corrupt-preset.log')
    if ($LASTEXITCODE -ne 0) { Show-PersistedFailure (Join-Path $r269Evidence '082-corrupt-preset.log'); throw 'Corrupt preset damaged other settings or was not discarded' }
    Wait-R261PublishedWindowBounds -ProcessPath (Join-Path $install 'Deep Legends.exe') -BoundsFile $boundsFile -Deadline (Get-Date).AddSeconds(60) -EvidencePath (Join-Path $r269Evidence '082-second-normal-close.json') | Out-Null
    Stop-InstalledApp -ObserveOnly
    $actual081Comparisons=@()
    foreach ($file in $actual081Files.Keys) {
        $after=(Get-FileHash $file -Algorithm SHA256).Hash
        $actual081Comparisons+=@{name=(Split-Path $file -Leaf);before_sha256=$actual081Files[$file];after_sha256=$after}
        if ((Split-Path $file -Leaf) -eq 'ui-scale.json') {
            $scale=Get-Content $file -Raw | ConvertFrom-Json
            if ($scale.mode -ne 'fixed' -or $scale.value -ne 2) { throw 'Published 081 zoom was not migrated to 2' }
        } elseif ($after -ne $actual081Files[$file]) { throw "Actual 081 persisted file changed after upgrade: $file" }
        Copy-Item $file (Join-Path $r269Evidence ("082-actual-after-"+(Split-Path $file -Leaf))) -Force
    }
    $actual081Comparisons | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $r269Evidence '081-to-082-persistent-files.json')
    @{upgrade_from='0.12.81';upgrade_to=$candidateVersion;key_mode='public';setup081_sha256='7dd3e6eb22aeb75c9e62deddd090231f8c93fa62e92de90bfc1baa174085862d';settings_written_by='unmodified published 081 renderer UI';preset_scope='actual published 081 UI-written presets, no resource substitution';candidate_resource_fixture=$false;player_data='synthetic demo';preferences_equal_except_scale=$true;scale_before=$legacyScale;scale_after=2;window_bounds_equal=$true;preset_apply_rename_save_restart=$true;corrupt_preset_isolated=$true} | ConvertTo-Json | Set-Content (Join-Path $r269Evidence '081-to-082-real-upgrade-summary.json')
    Get-Content (Join-Path $r269Evidence '081-to-082-real-upgrade-summary.json')
    }
    Remove-Item Env:R269_LEGACY_SCALE

} catch {
    # Capture before final cleanup, including foreign install paths/port owners.
    Save-R265State 'r265-failure-processes-and-ports.json' | Out-Null
    throw
} finally {
    if ($userData -and (Test-Path $userData)) {
        foreach ($name in @('window-bounds.json','ui-scale.json','r238-settings-sentinel.json')) {
            $path=Join-Path $userData $name
            if (Test-Path $path) { Copy-Item $path (Join-Path $evidence ("077-after-"+$name)) -Force }
        }
        Get-ChildItem (Join-Path $userData 'logs') -Filter 'desktop*.log' -ErrorAction SilentlyContinue | ForEach-Object { Copy-Item $_.FullName (Join-Path $evidence $_.Name) }
    }
    foreach ($name in @('update-install-stages.txt','update-install-nsis-stages.txt','update-install-timing.json')) {
        $path=Join-Path $data $name
        if (Test-Path $path) { Copy-Item $path $evidence -Force }
    }
    Get-ChildItem (Join-Path $data 'logs') -Filter 'diagnostics*.jsonl' -ErrorAction SilentlyContinue | Where-Object { $_.Name -match '^diagnostics(?:\.[1-5])?\.jsonl$' } | ForEach-Object { Copy-Item $_.FullName (Join-Path $evidence $_.Name) }
    $diagnostics=Join-Path $data 'logs/diagnostics.jsonl'
    if (Test-Path $diagnostics) {
        Get-Content $diagnostics | ForEach-Object {try {$row=$_ | ConvertFrom-Json; if ($row.event -in @('update_install_timing','update_shortcut_state')) {$_}} catch {}} | Set-Content (Join-Path $evidence 'install-diagnostics.jsonl')
    }
    Stop-InstalledApp
}
