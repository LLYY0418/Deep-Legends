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
function Run-Setup([string]$File) {
    # These are the actual Go installer shells. --update auto-starts their
    # existing progress flow; /S alone would leave the Go setup page waiting.
    $process = Start-Process -FilePath $File -ArgumentList "--update --dest `"$install`"" -PassThru
    if (-not $process.WaitForExit(180000)) { Stop-Process -Id $process.Id -Force; throw "Real installer timed out" }
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
    $desktopLink = Join-Path ([Environment]::GetFolderPath('Desktop')) "Deep Legends.lnk"
    $menuLink = Join-Path ([Environment]::GetFolderPath('Programs')) "Deep Legends.lnk"
    $created = @{}
    foreach ($link in @($desktopLink,$menuLink)) { if (-not (Test-Path $link)) {throw "Old installation shortcut missing"}; $created[$link] = (Get-Item $link).CreationTimeUtc.Ticks }
    Run-Setup (Resolve-Path $Setup)
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
    @{old_version='0.12.65';key_mode='public';stages=$order.Count;icon_location_stable=$true;created_time_changed=$false;total_ms=$timing.total_ms;uninstall_old_ms=$timing.uninstall_old_ms;copy_ms=$timing.copy_ms} | ConvertTo-Json | Set-Content (Join-Path $evidence 'real-upgrade-summary.json')
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
