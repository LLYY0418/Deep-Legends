# Diagnostic collection only; installer arguments, files and deadlines are owned by Run-Setup.
function ConvertTo-R271SafeText([string]$Text) {
    return ($Text -replace 'RGAPI-[A-Za-z0-9-]+', '[riot-key]' -replace '(?i)([A-Z]:\\+Users\\+|/Users/|/home/)[^\\/\r\n"]+', '$1[user]')
}
function Show-R271LogTail([string]$Label, [string]$Path) {
    Write-Output "$Label (last 80 lines)"
    if (Test-Path -LiteralPath $Path) {
        Get-Content -LiteralPath $Path -Tail 80 | ForEach-Object { ConvertTo-R271SafeText ([string]$_) }
    } else { Write-Output '[log not created]' }
}
function Save-R271RunnerHost([string]$Evidence) {
    $cpus = @(Get-CimInstance Win32_Processor | Select-Object -ExpandProperty Name)
    $os = Get-CimInstance Win32_OperatingSystem
    $record = [ordered]@{ captured_at = [DateTime]::UtcNow.ToString('o'); cpu_names = $cpus; windows_caption = $os.Caption; windows_version = $os.Version; windows_build = $os.BuildNumber; windows_architecture = $os.OSArchitecture }
    $json = $record | ConvertTo-Json -Depth 4
    $json | Set-Content -LiteralPath (Join-Path $Evidence 'r271-runner-host.json') -Encoding utf8
    Write-Output ('R271_RUNNER_HOST ' + (ConvertTo-R271SafeText ($record | ConvertTo-Json -Depth 4 -Compress)))
}
function Select-R271ApplicationErrors($Events, [int]$ProcessID, [string]$File, [DateTime]$StartedAt, [DateTime]$Now) {
    foreach ($event in $Events) {
        if ($event.TimeCreated -lt $Now.AddMinutes(-5) -or $event.TimeCreated -lt $StartedAt -or $event.TimeCreated -gt $Now) { continue }
        $xml = [xml]$event.ToXml()
        $fields = @{}
        foreach ($item in $xml.Event.EventData.Data) { if ($item.Name) { $fields[[string]$item.Name] = [string]$item.'#text' } }
        $idText = $fields['ProcessId']
        if (!$idText) { $idText = $fields['FaultingProcessId'] }
        $related = $false
        if ($idText) {
            try { $idValue = if ($idText -match '^0x') { [Convert]::ToInt64($idText.Substring(2), 16) } else { [long]$idText }; $related = $idValue -eq $ProcessID } catch {}
        } else {
            $appPath = $fields['AppPath']
            if (!$appPath) { $appPath = $fields['FaultingApplicationPath'] }
            $related = $appPath -and [string]::Equals($appPath, $File, [StringComparison]::OrdinalIgnoreCase)
        }
        if ($related) {
            [ordered]@{ time = $event.TimeCreated.ToUniversalTime().ToString('o'); event_id = $event.Id; provider = $event.ProviderName; process_id = $ProcessID; fields = $fields; message = $event.Message; xml = $event.ToXml() }
        }
    }
}
function Save-R271SetupFailure([string]$File, [string]$Version, $Process, [DateTime]$StartedAt, [string]$Prefix, [string]$StartupLog) {
    Show-R271LogTail 'installer stderr' ($Prefix + '-stderr.log')
    Show-R271LogTail 'installer stdout' ($Prefix + '-stdout.log')
    $startupCopy = $Prefix + '-startup.log'
    if (Test-Path -LiteralPath $StartupLog) { Copy-Item -LiteralPath $StartupLog -Destination $startupCopy }
    Show-R271LogTail 'installer startup log' $startupCopy
    $now = Get-Date
    $queryStatus = 'ok'; $queryError = $null; $events = @()
    try {
        $events = @(Get-WinEvent -FilterHashtable @{ LogName = 'Application'; StartTime = $now.AddMinutes(-5); Level = 2 } -ErrorAction Stop)
    } catch {
        if ($_.FullyQualifiedErrorId -match 'NoMatchingEventsFound') { $queryStatus = 'no_matching_events' }
        else { $queryStatus = 'query_failed'; $queryError = $_.Exception.Message }
    }
    $related = @(Select-R271ApplicationErrors -Events $events -ProcessID $Process.Id -File $File -StartedAt $StartedAt -Now $now)
    $record = [ordered]@{ version = $Version; installer = $File; process_id = $Process.Id; exit_code = $Process.ExitCode; started_at = $StartedAt.ToUniversalTime().ToString('o'); captured_at = $now.ToUniversalTime().ToString('o'); query_start = $now.AddMinutes(-5).ToUniversalTime().ToString('o'); query_status = $queryStatus; query_error = $queryError; application_errors = $related }
    $json = ConvertTo-R271SafeText ($record | ConvertTo-Json -Depth 10)
    $json | Set-Content -LiteralPath ($Prefix + '-application-errors.json') -Encoding utf8
    Write-Output 'related Windows Application errors from the last five minutes'
    Write-Output $json
}
