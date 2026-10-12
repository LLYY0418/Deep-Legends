param([string]$RunSource = (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1'))
$ErrorActionPreference = 'Stop'
function Assert-R276($Condition, [string]$Message) { if (!$Condition) { throw "ASSERTION FAILED: $Message" } }
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('r276-legacy-retry-' + [Guid]::NewGuid().ToString('N'))
$evidence = Join-Path $fixture 'evidence'; $install = Join-Path $fixture 'installed'
$oldTmp = $env:TMP; $env:TMP = $fixture
$candidateVersion = '0.12.83'
$script:setupSequence = 0
$script:starts = 0; $script:stops = 0; $script:flushes = 0
$script:legacyRetryRecords = @()
$signature = "runtime: bad pointer in frame main.shellWndProc`nfatal error: invalid pointer found on stack"
function ConvertTo-R271SafeText($Text) { return $Text }
function Save-R271SetupFailure { }
function Stop-InstalledApp { $script:stops++; Assert-R276 ($script:flushes -eq $script:starts) 'cleanup must follow true exit/output flush' }
function Start-Process($FilePath, $ArgumentList, [switch]$PassThru, $RedirectStandardError, $RedirectStandardOutput) {
    $script:starts++
    $index = [Math]::Min($script:starts - 1, $script:exits.Count - 1)
    Assert-R276 ($FilePath -eq 'exact-published-or-candidate.exe') 'retry must use unchanged executable'
    Assert-R276 ($ArgumentList -eq "--update --dest `"$install`"") 'installer arguments unchanged'
    [IO.File]::WriteAllText($RedirectStandardError, $script:stderr)
    [IO.File]::WriteAllText($RedirectStandardOutput, $signature)
    $process = [PSCustomObject]@{ Id = 321; ExitCode = $script:exits[$index] }
    $process | Add-Member ScriptMethod WaitForExit { param($Timeout); if ($null -eq $Timeout) { $script:flushes++; return }; return $true }
    return $process
}
function Invoke-Fixture([string]$Version, [string]$ErrorText, [int[]]$Exits, [bool]$OptIn, [int]$ExpectedStarts, [bool]$ExpectedPass) {
    $script:starts = 0; $script:stops = 0; $script:flushes = 0; $script:legacyRetryRecords = @()
    $script:stderr = $ErrorText; $script:exits = $Exits
    $failure = ''
    try { Run-Setup 'exact-published-or-candidate.exe' -Version $Version -PublishedLegacy:$OptIn | Out-Null } catch { $failure = $_.Exception.Message }
    Assert-R276 ($script:starts -eq $ExpectedStarts) "starts mismatch $Version expected=$ExpectedStarts actual=$script:starts"
    if (!$ExpectedPass) { Assert-R276 ($failure -eq 'Real installer failed: 2') "original failure cannot be replaced version=$Version text=$ErrorText failure=$failure" }
    Assert-R276 (($failure -eq '') -eq $ExpectedPass) "main outcome mismatch $Version $failure"
    Assert-R276 ($script:stops -eq ($ExpectedStarts - 1)) 'cleanup exactly once per permitted retry'
    Assert-R276 ($script:flushes -eq $ExpectedStarts) 'each attempt flushed stderr'
    if ($ExpectedStarts -eq 2) {
        Assert-R276 ($script:legacyRetryRecords.Count -eq 1) 'exactly one retry evidence record'
        $record = $script:legacyRetryRecords[0]
        Assert-R276 ($record.phase -eq 'published-legacy-setup' -and $record.version -eq $Version -and $record.retry_count -eq 1 -and $record.attempt_count -eq 2 -and $record.completed) 'retry evidence identity and limit'
        Assert-R276 ($record.first_stderr -ne $record.second_stderr -and (Test-Path $record.first_stderr) -and (Test-Path $record.second_stderr)) 'both original stderr paths preserved'
        Assert-R276 ((Get-Content $record.first_stderr -Raw).Contains('invalid pointer found on stack')) 'first original signature preserved'
        $saved = Get-Content $record.evidence_json -Raw | ConvertFrom-Json
        Assert-R276 ($saved.second_exit_code -eq $Exits[1]) 'second outcome evidence preserved'
    } else { Assert-R276 ($script:legacyRetryRecords.Count -eq 0) 'no unauthorized retry evidence' }
}
try {
    New-Item -ItemType Directory -Force $fixture,$install,$evidence | Out-Null
    [IO.File]::WriteAllText((Join-Path $install 'Deep Legends.exe'), 'anonymous fixture')
    $source = Get-Content $RunSource -Raw
    $match = [regex]::Match($source, '(?s)function Run-Setup\(.*?(?=\r?\nfunction Get-R265AppState)')
    Assert-R276 $match.Success 'extract actual production-test script function'
    . ([ScriptBlock]::Create($match.Value))
    foreach ($version in @('0.12.65','0.12.68','0.12.76','0.12.80','0.12.81')) { Invoke-Fixture $version $signature @(2,0) $true 2 $true }
    Invoke-Fixture '0.12.81' $signature @(2,2,0) $true 2 $false
    foreach ($text in @('invalid pointer found on stack','main.shellWndProc','unrelated failure','')) { Invoke-Fixture '0.12.81' $text @(2,0) $true 1 $false }
    foreach ($version in @('0.12.79','0.12.82','0.12.83')) { Invoke-Fixture $version $signature @(2,0) $true 1 $false }
    # Even a future candidate matching the historical whitelist must not retry.
    $candidateVersion = '0.12.81'; Invoke-Fixture '0.12.81' $signature @(2,0) $true 1 $false; $candidateVersion = '0.12.83'
    Invoke-Fixture '0.12.81' $signature @(2,0) $false 1 $false
    Invoke-Fixture '0.12.81' $signature @(0) $true 1 $true
    # Exact callsite opt-in is part of the policy; candidate remains unmarked.
    $candidateCalls = [regex]::Matches($source, 'Run-Setup[^\r\n]*-Version \$candidateVersion[^\r\n]*')
    Assert-R276 ($candidateCalls.Count -eq 5) 'all five real candidate callsites are checked'
    foreach ($call in $candidateCalls) { Assert-R276 (!$call.Value.Contains('-PublishedLegacy')) 'candidate calls cannot opt in' }
    $calls = [regex]::Matches($source, 'Run-Setup \$(?:old|legacy|published076|published080|published081) -Version ''0\.12\.(?:65|68|76|80|81)'' -PublishedLegacy')
    Assert-R276 ($calls.Count -eq 7) 'all seven authorized old-installer callsites opt in'
    Write-Output 'PASS R276 precise stderr-only legacy retry, one retry maximum, candidate/other failures fatal, cleanup and both attempt evidence preserved'
} finally { $env:TMP = $oldTmp; Remove-Item $fixture -Recurse -Force }
