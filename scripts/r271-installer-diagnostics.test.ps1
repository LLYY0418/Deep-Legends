param([string]$RunSource = (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1'), [string]$HelperSource = (Join-Path $PSScriptRoot 'r271-installer-diagnostics.ps1'))
$ErrorActionPreference = 'Stop'
. $HelperSource
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
    $script:fixtureStarted = Get-Date
    Assert-R271 ($ArgumentList -eq "--update --dest `"$install`"") 'real update/destination arguments must be unchanged'
    Assert-R271 ($FilePath -eq $script:fixtureFile) 'actual installer file must be unchanged'
    Assert-R271 ($RedirectStandardError -and $RedirectStandardOutput -and $RedirectStandardError -ne $RedirectStandardOutput) 'stderr/stdout must be captured separately'
    Assert-R271 ((Split-Path -Leaf $RedirectStandardError).Contains($script:fixtureVersion)) 'capture filename must identify version'
    [IO.File]::WriteAllText($RedirectStandardError, "fatal error: synthetic runtime crash`nRGAPI-fixture-key C:\Users\fixture user\setup.exe")
    [IO.File]::WriteAllText($RedirectStandardOutput, 'synthetic stdout preserved')
    [IO.File]::WriteAllText((Join-Path $env:TMP 'DeepLegendsSetup-startup.log'), '[pid=321] synthetic installer startup')
    $process = [PSCustomObject]@{ Id = 321; ExitCode = $script:fixtureExit }
    $process | Add-Member -MemberType ScriptMethod -Name WaitForExit -Value { param($Timeout); if ($null -eq $Timeout) { $script:flushCalls++; return }; return $true }
    return $process
}
try {
    $null = New-Item -ItemType Directory -Force $fixture,$evidence,$install
    [IO.File]::WriteAllText((Join-Path $install 'Deep Legends.exe'), 'synthetic installed executable')
    $env:TEMP = Join-Path $fixture 'different-temp'; $null = New-Item -ItemType Directory $env:TEMP; $env:TMP = $fixture
    $source = Get-Content -LiteralPath $RunSource -Raw
    $match = [regex]::Match($source, '(?s)function Run-Setup\(.*?(?=\r?\nfunction Get-R265AppState)')
    Assert-R271 $match.Success 'actual Run-Setup function must be extracted'
    . ([ScriptBlock]::Create($match.Value))
    $hostLines = @(Save-R271RunnerHost $evidence)
    $hostRecord = Get-Content (Join-Path $evidence 'r271-runner-host.json') -Raw | ConvertFrom-Json
    Assert-R271 ($hostRecord.cpu_names[0] -eq 'synthetic Intel Xeon 6973P-C' -and $hostRecord.windows_version -eq '10.0.fixture') 'CPU and Windows version must be preserved'
    foreach ($version in @('0.12.65','0.12.68','0.12.76','0.12.79','0.12.80','0.12.81','0.12.82')) {
        $script:fixtureVersion = $version; $script:fixtureFile = Join-Path $fixture ("Setup-$version.exe"); [IO.File]::WriteAllText($script:fixtureFile, 'unchanged synthetic installer')
        $script:fixtureExit = 0
        Run-Setup -File $script:fixtureFile -Version $version | Out-Null
        Assert-R271 (($script:lastSetupDeadline - (Get-Date)).TotalSeconds -ge 178 -and ($script:lastSetupDeadline - (Get-Date)).TotalSeconds -le 180) 'original 180 second deadline must remain'
    }
    $script:fixtureExit = 2
    $output = New-Object 'System.Collections.Generic.List[string]'
    $failure = ''
    try { Run-Setup -File $script:fixtureFile -Version '0.12.82' | ForEach-Object { $output.Add([string]$_) } } catch { $failure = $_.Exception.Message }
    Assert-R271 ($failure -eq 'Real installer failed: 2') 'original nonzero-exit failure must remain'
    $text = $output -join "`n"
    Assert-R271 ($text.Contains('fatal error: synthetic runtime crash') -and $text.Contains('synthetic stdout preserved') -and $text.Contains('synthetic installer startup')) 'all three diagnostic tails must precede throw'
    Assert-R271 (!$text.Contains('RGAPI-fixture-key') -and !$text.Contains('fixture user')) 'diagnostic output must redact key and user path'
    $records = @(Get-ChildItem $evidence -Filter '*-application-errors.json')
    Assert-R271 ($records.Count -eq 1) 'failure must save application error evidence'
    $record = Get-Content $records[0].FullName -Raw | ConvertFrom-Json
    Assert-R271 ($record.application_errors.Count -eq 1 -and $record.application_errors[0].event_id -eq 1000) 'only recent matching PID event may be included'
    $script:queryFails = $true
    try { Run-Setup -File $script:fixtureFile -Version '0.12.82' | Out-Null } catch { $failure = $_.Exception.Message }
    Assert-R271 ($failure -eq 'Real installer failed: 2') 'event log failure must not replace installer failure'
    Assert-R271 ($script:flushCalls -eq 9) 'redirected output must finish before success or failure is handled'
    Write-Output 'PASS R271 actual Run-Setup redirects seven versions, preserves arguments/deadline, captures failure tails, filters events and redacts output'
} finally { $env:TEMP = $oldTemp; $env:TMP = $oldTmp; Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue }
