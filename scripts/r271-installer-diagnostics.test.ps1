param([string]$RunSource = (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1'), [string]$HelperSource = (Join-Path $PSScriptRoot 'r271-installer-diagnostics.ps1'))
$ErrorActionPreference = 'Stop'
$fixtureClock = [Diagnostics.Stopwatch]::StartNew()
function Write-R271Stage([string]$Stage) { [Console]::Out.WriteLine(('R271_STAGE {0} {1}ms' -f $Stage, $fixtureClock.ElapsedMilliseconds)); [Console]::Out.Flush() }
Write-R271Stage 'script-start'
[Console]::Out.WriteLine(('R271_ENGINE version={0} platform={1}' -f $PSVersionTable.PSVersion, $PSVersionTable.Platform))
# Instrument only in-memory fixture copies; the installed helper and Run-Setup stay unchanged.
function Add-R271FixtureTrace([string]$Text, [System.Collections.IDictionary]$Markers) {
    foreach ($entry in $Markers.GetEnumerator()) {
        if ($Text.Contains($entry.Key)) {
            $Text = $Text.Replace($entry.Key, ("Write-R271Stage '" + $entry.Value + "-start'`n" + $entry.Key + "`nWrite-R271Stage '" + $entry.Value + "-done'"))
        }
    }
    return $Text
}
$helperText = Get-Content -LiteralPath $HelperSource -Raw
$helperText = Add-R271FixtureTrace $helperText ([ordered]@{
    'Show-R271LogTail ''installer stderr'' ($Prefix + ''-stderr.log'')' = 'stderr-tail'
    'Show-R271LogTail ''installer stdout'' ($Prefix + ''-stdout.log'')' = 'stdout-tail'
    'if (Test-Path -LiteralPath $StartupLog) { Copy-Item -LiteralPath $StartupLog -Destination $startupCopy }' = 'startup-copy'
    'Show-R271LogTail ''installer startup log'' $startupCopy' = 'startup-tail'
    '$events = @(Get-WinEvent -FilterHashtable @{ LogName = ''Application''; StartTime = $now.AddMinutes(-5); Level = 2 } -ErrorAction Stop)' = 'event-query'
    '$related = @(Select-R271ApplicationErrors -Events $events -ProcessID $Process.Id -File $File -StartedAt $StartedAt -Now $now)' = 'event-filter'
    '$json = ConvertTo-R271SafeText ($record | ConvertTo-Json -Depth 10)' = 'diagnostic-json'
    '$json | Set-Content -LiteralPath ($Prefix + ''-application-errors.json'') -Encoding utf8' = 'diagnostic-write'
})
. ([ScriptBlock]::Create($helperText))
Write-R271Stage 'helper-loaded'
function Assert-R271($Condition, [string]$Message) { if (!$Condition) { throw "ASSERTION FAILED: $Message" } }
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('r271-setup-test-' + [Guid]::NewGuid().ToString('N'))
$oldTemp = $env:TEMP; $oldTmp = $env:TMP
$script:fixtureVersion = ''; $script:fixtureExit = 0; $script:flushCalls = 0; $script:queryFails = $false
$script:setupSequence = 0
$evidence = Join-Path $fixture 'evidence'; $install = Join-Path $fixture 'installed'
function Get-CimInstance($ClassName) {
    if ($ClassName -eq 'Win32_Processor') { return [PSCustomObject]@{ Name = 'synthetic Intel Xeon 6973P-C' } }
    return [PSCustomObject]@{ Caption = 'synthetic Windows Server'; Version = '10.0.fixture'; BuildNumber = 'fixture'; OSArchitecture = '64-bit' }
}
function New-R271Event([int]$ID, [string]$PIDText, [string]$Path, [DateTime]$At) {
    $event = [PSCustomObject]@{ Id = $ID; TimeCreated = $At; ProviderName = 'Application Error'; Message = 'RGAPI-fixture-key C:\Users\fixture user\setup.exe'; XmlText = "<Event><EventData><Data Name='ProcessId'>$PIDText</Data><Data Name='AppPath'>$Path</Data></EventData></Event>" }
    $event | Add-Member -MemberType ScriptMethod -Name ToXml -Value { return $this.XmlText }
    return $event
}
function Get-WinEvent($FilterHashtable, $ErrorAction) {
    Assert-R271 ($FilterHashtable.LogName -eq 'Application' -and $FilterHashtable.Level -eq 2) 'query must request Application errors'
    Assert-R271 (((Get-Date) - $FilterHashtable.StartTime).TotalMinutes -lt 5.1) 'query must be bounded to five minutes'
    if ($script:queryFails) { throw 'synthetic event log unavailable' }
    $now = Get-Date
    New-R271Event 1000 '0x141' $script:fixtureFile $script:fixtureStarted
    New-R271Event 1001 '999' $script:fixtureFile $script:fixtureStarted
    New-R271Event 1002 '321' $script:fixtureFile $now.AddMinutes(-6)
}
function Start-Process($FilePath, $ArgumentList, [switch]$PassThru, $RedirectStandardError, $RedirectStandardOutput) {
    Write-R271Stage 'mock-process-start'
    $script:fixtureStarted = Get-Date
    Assert-R271 ($ArgumentList -eq "--update --dest `"$install`"") 'real update/destination arguments must be unchanged'
    Assert-R271 ($FilePath -eq $script:fixtureFile) 'actual installer file must be unchanged'
    Assert-R271 ($RedirectStandardError -and $RedirectStandardOutput -and $RedirectStandardError -ne $RedirectStandardOutput) 'stderr/stdout must be captured separately'
    Assert-R271 ((Split-Path -Leaf $RedirectStandardError).Contains($script:fixtureVersion)) 'capture filename must identify version'
    [IO.File]::WriteAllText($RedirectStandardError, "fatal error: synthetic runtime crash`nRGAPI-fixture-key C:\Users\fixture user\setup.exe")
    [IO.File]::WriteAllText($RedirectStandardOutput, 'synthetic stdout preserved')
    [IO.File]::WriteAllText((Join-Path $env:TMP 'DeepLegendsSetup-startup.log'), '[pid=321] synthetic installer startup')
    $process = [PSCustomObject]@{ Id = 321; ExitCode = $script:fixtureExit }
    $process | Add-Member -MemberType ScriptMethod -Name WaitForExit -Value { param($Timeout); if ($null -eq $Timeout) { $script:flushCalls++; Write-R271Stage 'mock-output-flushed'; return }; return $true }
    Write-R271Stage 'mock-process-ready'
    return $process
}
try {
    $null = New-Item -ItemType Directory -Force $fixture,$evidence,$install
    [IO.File]::WriteAllText((Join-Path $install 'Deep Legends.exe'), 'synthetic installed executable')
    $env:TEMP = Join-Path $fixture 'different-temp'; $null = New-Item -ItemType Directory $env:TEMP; $env:TMP = $fixture
    $source = Get-Content -LiteralPath $RunSource -Raw
    $match = [regex]::Match($source, '(?s)function Run-Setup\(.*?(?=\r?\nfunction Get-R265AppState)')
    Assert-R271 $match.Success 'actual Run-Setup function must be extracted'
    $tracedRun = Add-R271FixtureTrace $match.Value ([ordered]@{
    '$process = Start-Process -FilePath $File -ArgumentList "--update --dest `"$install`"" -PassThru -RedirectStandardError ($prefix + ''-stderr.log'') -RedirectStandardOutput ($prefix + ''-stdout.log'')' = 'setup-launch'
    '$process.WaitForExit() # Flush the redirected readers after the original bounded wait.' = 'setup-flush'
    'Save-R271SetupFailure -File $File -Version $Version -Process $process -StartedAt $started -Prefix $prefix -StartupLog $startupLog' = 'failure-collector'
})
    . ([ScriptBlock]::Create($tracedRun))
    foreach ($name in @('Start-Process','Get-WinEvent','Get-CimInstance')) { Assert-R271 ((Get-Command $name).CommandType -eq 'Function') ('fixture mock must own ' + $name) }
    Write-R271Stage 'fixture-ready'
    $hostLines = @(Save-R271RunnerHost $evidence)
    Write-R271Stage 'host-saved'
    $hostRecord = Get-Content (Join-Path $evidence 'r271-runner-host.json') -Raw | ConvertFrom-Json
    Assert-R271 ($hostRecord.cpu_names[0] -eq 'synthetic Intel Xeon 6973P-C' -and $hostRecord.windows_version -eq '10.0.fixture') 'CPU and Windows version must be preserved'
    foreach ($version in @('0.12.65','0.12.68','0.12.76','0.12.79','0.12.80','0.12.81','0.12.82','0.12.83')) {
        $script:fixtureVersion = $version; $script:fixtureFile = Join-Path $fixture ("Setup-$version.exe"); [IO.File]::WriteAllText($script:fixtureFile, 'unchanged synthetic installer')
        $script:fixtureExit = 0
        Write-R271Stage ('launch-' + $version)
        Run-Setup -File $script:fixtureFile -Version $version | Out-Null
        Write-R271Stage ('completed-' + $version)
        Assert-R271 (($script:lastSetupDeadline - (Get-Date)).TotalSeconds -ge 178 -and ($script:lastSetupDeadline - (Get-Date)).TotalSeconds -le 180) 'original 180 second deadline must remain'
    }
    $script:fixtureVersion = '0.12.82'; $script:fixtureFile = Join-Path $fixture 'Setup-0.12.82.exe'
    $script:fixtureExit = 2
    Write-R271Stage 'nonzero-diagnostics-start'
    Write-R271Stage 'output-list-create-start'
    $output = New-Object 'System.Collections.Generic.List[string]'
    Write-R271Stage 'output-list-create-done'
    $failure = ''
    try { Run-Setup -File $script:fixtureFile -Version '0.12.82' | ForEach-Object { $output.Add([string]$_) } } catch { $failure = $_.Exception.Message }
    Write-R271Stage 'nonzero-call-returned'
    Assert-R271 ($failure -eq 'Real installer failed: 2') 'original nonzero-exit failure must remain'
    $text = $output -join "`n"
    Assert-R271 ($text.Contains('fatal error: synthetic runtime crash') -and $text.Contains('synthetic stdout preserved') -and $text.Contains('synthetic installer startup')) 'all three diagnostic tails must precede throw'
    Assert-R271 (!$text.Contains('RGAPI-fixture-key') -and !$text.Contains('fixture user')) 'diagnostic output must redact key and user path'
    $records = @(Get-ChildItem $evidence -Filter '*-application-errors.json')
    Assert-R271 ($records.Count -eq 1) 'failure must save application error evidence'
    $record = Get-Content $records[0].FullName -Raw | ConvertFrom-Json
    Assert-R271 ($record.application_errors.Count -eq 1 -and $record.application_errors[0].event_id -eq 1000) 'only recent matching PID event may be included'
    Write-R271Stage 'nonzero-diagnostics-completed'
    $script:queryFails = $true
    Write-R271Stage 'query-failure-start'
    try { Run-Setup -File $script:fixtureFile -Version '0.12.82' | Out-Null } catch { $failure = $_.Exception.Message }
    Assert-R271 ($failure -eq 'Real installer failed: 2') 'event log failure must not replace installer failure'
    Assert-R271 ($script:flushCalls -eq 10) 'redirected output must finish before success or failure is handled'
    Write-R271Stage 'assertions-completed'
    Write-Output 'PASS R271 actual Run-Setup redirects eight versions, preserves arguments/deadline, captures failure tails, filters events and redacts output'
} finally { Write-R271Stage 'cleanup-start'; $env:TEMP = $oldTemp; $env:TMP = $oldTmp; Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue; Write-R271Stage 'cleanup-completed' }
