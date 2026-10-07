param([Parameter(Mandatory=$true)][string]$Setup)
$ErrorActionPreference = "Stop"
$root = Join-Path $env:RUNNER_TEMP "deep-legends-r206-real-upgrade"
$install = Join-Path $root "installed"
$data = Join-Path $root "data"
$evidence = Join-Path $root "evidence"
New-Item -ItemType Directory -Force $install,$data,$evidence | Out-Null
$env:LOL_LOOT_DATA_DIR = $data
$old = Join-Path $root "Deep-Legends-Setup-0.12.65-public.exe"
Invoke-WebRequest "https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.65/Deep-Legends-Setup-0.12.65-public.exe" -OutFile $old
if ((Get-FileHash $old -Algorithm SHA256).Hash.ToLowerInvariant() -ne "bf97221a0c8465b78d57bf32f15c746eeec72deb7f3ec4923323b095c16c70ac") { throw "Old public setup checksum mismatch" }
function Run-Setup([string]$File, [switch]$MonitorLegacy) {
    # These are the actual Go installer shells. --update auto-starts their
    # existing progress flow; /S alone would leave the Go setup page waiting.
    $process = Start-Process -FilePath $File -ArgumentList "--update --dest `"$install`"" -PassThru
    $deadline=(Get-Date).AddSeconds(180)
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
    if ($MonitorLegacy) { ConvertTo-Json -InputObject @($snapshots) -Depth 8 | Set-Content (Join-Path $evidence 'legacy-0.12.68-nsis-snapshots.json') }
    if ($process.ExitCode -ne 0) { throw "Real installer failed: $($process.ExitCode)" }
    if (-not (Test-Path (Join-Path $install "Deep Legends.exe"))) { throw "Installed executable missing" }
}
function Stop-InstalledApp {
    Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($install, [StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Milliseconds 500
}
try {
    Run-Setup $old
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
    try { Run-Setup $legacy -MonitorLegacy } finally { $env:TEMP=$originalTemp; $env:TMP=$originalTmp }
    Start-Sleep -Seconds 2
    $legacyDiagnostics=Join-Path $data 'logs/diagnostics.jsonl'
    if (Test-Path $legacyDiagnostics) { Get-Content $legacyDiagnostics | ForEach-Object {try {$row=$_ | ConvertFrom-Json; if ($row.event -eq 'update_install_timing') {$_}} catch {}} | Set-Content (Join-Path $evidence 'legacy-install-timing.jsonl') }
    Stop-InstalledApp
    # R238: install the exact published 0.12.76 public release before the candidate.
    $published076 = Join-Path $root 'Deep-Legends-Setup-0.12.76-public.exe'
    Invoke-WebRequest 'https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.76/Deep-Legends-Setup-0.12.76-public.exe' -OutFile $published076
    if ((Get-FileHash $published076 -Algorithm SHA256).Hash.ToLowerInvariant() -ne '01014312b60e591a05bfc87f86e098adf6c5fc5e59520db5aedac5b5dba02ff3') { throw 'Published 0.12.76 setup checksum mismatch' }
    Run-Setup $published076
    Stop-InstalledApp
    $userData = Join-Path $env:APPDATA 'Deep Legends'
    $sentinels = @{}
    # Distinct safe fixtures in real persistent directories, without a real account.
    foreach ($entry in @(@($data,'season-stats/r238-cache-sentinel.json','{"schemaVersion":3,"r238":"season-cache"}'),@($data,'snapshots/r238-collection-sentinel.json','{"r238":"collection"}'),@($userData,'r238-settings-sentinel.json','{"r238":"preferences"}'),@($userData,'window-bounds.json','{"x":40,"y":40,"width":1050,"height":750,"maximized":false}'),@($userData,'ui-scale.json','{"mode":"fixed","value":1.25,"defaultAuto":1}'))) {
        $file=Join-Path $entry[0] $entry[1]
        New-Item -ItemType Directory -Force (Split-Path -Parent $file) | Out-Null
        [IO.File]::WriteAllText($file,$entry[2],(New-Object Text.UTF8Encoding($false)))
        $sentinels[$file]=(Get-FileHash $file -Algorithm SHA256).Hash
    }
    # Production updater Check/Download/Apply, using a candidate HTTP fixture:
    # 0.12.77 is still a draft, so the anonymous Latest endpoint stays 0.12.76.
    $env:R238_UPGRADE_SETUP=(Resolve-Path $Setup).Path
    $env:R238_UPGRADE_INSTALL=$install
    $env:R238_UPGRADE_EVIDENCE=$evidence
    & go test -count=1 -run '^TestR238DefaultOnline076To077$' ./backend *> (Join-Path $evidence 'online-076-077-test.log')
    if ($LASTEXITCODE -ne 0) { throw 'R238 candidate online updater check/download/hash/handoff failed' }
    $online=Get-Content -Raw (Join-Path $evidence 'online-076-077.json') | ConvertFrom-Json
    if ((Get-FileHash $online.download -Algorithm SHA256).Hash.ToLowerInvariant() -ne $online.sha256) { throw 'R238 verified online download changed before installation' }
    $desktopLink = Join-Path ([Environment]::GetFolderPath('Desktop')) "Deep Legends.lnk"
    $menuLink = Join-Path ([Environment]::GetFolderPath('Programs')) "Deep Legends.lnk"
    $created = @{}
    foreach ($link in @($desktopLink,$menuLink)) { if (-not (Test-Path $link)) {throw "Old installation shortcut missing"}; $created[$link] = (Get-Item $link).CreationTimeUtc.Ticks }
    Run-Setup $online.download
    Stop-InstalledApp
    foreach ($file in $sentinels.Keys) { if (-not (Test-Path $file) -or (Get-FileHash $file -Algorithm SHA256).Hash -ne $sentinels[$file]) { throw "R238 persistent fixture changed: $file" } }
    $sentinels | ConvertTo-Json | Set-Content (Join-Path $evidence 'retained-persistent-fixtures.json')
    Start-Process (Join-Path $install 'Deep Legends.exe') | Out-Null
    $stages = @{}
    Get-Content (Join-Path $data "update-install-stages.txt") | ForEach-Object { $pair=$_ -split '=',2; $stages[$pair[0]]=[long]$pair[1] }
    $order = @('installer_start','parent_exited','uninstall_old_start','uninstall_old_done','extract_start','extract_done','copy_done','relaunch')
    $previous=0L
    foreach ($stage in $order) { if (-not $stages.ContainsKey($stage) -or $stages[$stage] -lt $previous) {throw "Missing/non-monotonic stage: $stage"}; $previous=$stages[$stage] }
    $raw = Get-Content (Join-Path $data "update-install-nsis-stages.txt")
    if (-not ($raw -match '^uninstall_old_done=')) {throw "NSIS did not write uninstall_old_done"}
    $deadline=(Get-Date).AddSeconds(30)
    do {
        $rows = @(Get-Content (Join-Path $data 'logs/diagnostics.jsonl') | ForEach-Object {try {$_ | ConvertFrom-Json} catch {}})
        $timing = @($rows | Where-Object event -eq 'update_install_timing') | Select-Object -Last 1
        if ($timing.result -eq 'ok') {break}; Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)
    if ($timing.result -ne 'ok' -or $null -eq $timing.uninstall_old_ms) {throw "Eight stages not imported as result=ok"}
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
    @{old_version='0.12.65';intermediate_version='0.12.68';upgrade_from='0.12.76';upgrade_to='0.12.77';candidate_online_transport='real loopback HTTP fixture';anonymous_latest_077='pending publication';persistent_sentinels_retained=$true;key_mode='public';stages=$order.Count;icon_location_stable=$true;created_time_changed=$false;total_ms=$timing.total_ms;uninstall_old_ms=$timing.uninstall_old_ms;copy_ms=$timing.copy_ms} | ConvertTo-Json | Set-Content (Join-Path $evidence 'real-upgrade-summary.json')
    Get-Content (Join-Path $evidence 'real-upgrade-summary.json')
} finally {
    foreach ($name in @('update-install-stages.txt','update-install-nsis-stages.txt','update-install-timing.json')) {
        $path=Join-Path $data $name
        if (Test-Path $path) { Copy-Item $path $evidence -Force }
    }
    $diagnostics=Join-Path $data 'logs/diagnostics.jsonl'
    if (Test-Path $diagnostics) {
        Get-Content $diagnostics | ForEach-Object {try {$row=$_ | ConvertFrom-Json; if ($row.event -in @('update_install_timing','update_shortcut_state')) {$_}} catch {}} | Set-Content (Join-Path $evidence 'install-diagnostics.jsonl')
    }
    Stop-InstalledApp
}
