# Synthetic fixtures exercise the real script and collector, never a real installer.
[CmdletBinding()]
param([string]$ScriptPath = (Join-Path $PSScriptRoot 'r82-startup-ab.ps1'))
$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('r82-ab-script-' + [Guid]::NewGuid().ToString('N'))
$oldTemp = $env:TEMP
$oldMode = [Environment]::GetEnvironmentVariable('DEEP_LEGENDS_STARTUP_PREWARM', 'Process')
$script:checks = 0
# The invoked .ps1 has its own script scope. Share only fixture state explicitly
# in this disposable PowerShell process so the mocked cmdlets can see it there.
$global:r82ABFixtureState = [PSCustomObject]@{ Current = $null; Launches = 0 }

function Assert-Fixture($Condition, [string]$Message) {
    if (!$Condition) { throw "ASSERTION FAILED: $Message" }
}
function Complete-Check([string]$Name) {
    $script:checks++
    Write-Output "PASS $Name"
}
function New-Fixture([string]$Name = 'Deep Legends Setup 0.12.1.exe') {
    $directory = Join-Path $root ([Guid]::NewGuid().ToString('N'))
    $null = New-Item -ItemType Directory -Path $directory
    $installer = Join-Path $directory $Name
    [IO.File]::WriteAllText($installer, 'synthetic installer bytes ' + [Guid]::NewGuid())
    $assets = [ordered]@{}
    $assets['Deep Legends Setup 0.12.1.exe'] = (Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant()
    $receipt = Join-Path $directory 'release-build.json'
    [ordered]@{ schema = 1; version = '0.12.1'; fingerprint = 'abcdef012345'; assets = $assets } |
        ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $receipt -Encoding UTF8
    return @{ Installer = $installer; Receipt = $receipt; Results = (Join-Path $directory 'results.json'); Diagnostics = (Join-Path $directory 'diagnostics.jsonl') }
}

# Scoped to this test process. The child script sees these functions; it cannot
# launch an exe or consult/modify the user's real running application.
function Get-Process { param($Name, $ErrorAction) }
function Start-Process {
    param($FilePath, [switch]$PassThru)
    Assert-Fixture ($FilePath -eq $global:r82ABFixtureState.Current.Installer) 'launch used the wrong installer path'
    $global:r82ABFixtureState.Launches++
    $time = [DateTime]::UtcNow.ToString('o')
    $runID = [Guid]::NewGuid().ToString('N')
    $records = @(
        @{ event = 'app_start'; build_fingerprint = 'abcdef012345'; run_id = $runID; time = $time },
        @{ event = 'desktop_startup_phases_ms'; run_id = $runID; time = $time; log_seq = 2;
            phases_ms = @{ process_to_js = 1600; spawn_to_ready = 1500; total = 3600; splash_window_shown = 1800 } }
    )
    $diagnostics = ($records | ForEach-Object { $_ | ConvertTo-Json -Depth 4 -Compress }) -join "`n"
    # File.WriteAllText emits UTF-8 without a BOM on both Windows PowerShell 5.1
    # and PowerShell 7, matching the application's real JSONL/startup log files.
    [IO.File]::WriteAllText($global:r82ABFixtureState.Current.Diagnostics, $diagnostics)
    $prewarm = if ($env:DEEP_LEGENDS_STARTUP_PREWARM -eq '1') {
        'enabled=true files=2 failures=0 timed_out=false elapsed_ms=700'
    } else { 'enabled=false elapsed_ms=0' }
    [IO.File]::WriteAllText((Join-Path $env:TEMP 'DeepLegendsSetup-startup.log'),
        "[pid=321] startup prewarm $prewarm`n[pid=321] application handoff visible=true elapsed_ms=2100")
    $process = [PSCustomObject]@{ Id = 321; ExitCode = 0 }
    $process | Add-Member -MemberType ScriptMethod -Name WaitForExit -Value { param($Timeout); return $true }
    return $process
}
function Invoke-Fixture($Fixture, [string]$Group = 'control') {
    $global:r82ABFixtureState.Current = $Fixture
    $global:r82ABFixtureState.Launches = 0
    try {
        & $script:copy -Installer $Fixture.Installer -Group $Group -Results $Fixture.Results -DiagnosticsPath $Fixture.Diagnostics | Out-Null
        return $null
    } catch { return $_.Exception.Message }
}
function Assert-Rejected($Fixture, [string]$ErrorText, [string]$Label) {
    $failure = Invoke-Fixture $Fixture
    Assert-Fixture ($failure -and $failure.Contains($ErrorText)) "$Label must report the expected error (got: $failure)"
    Assert-Fixture ($global:r82ABFixtureState.Launches -eq 0) "$Label must fail before launch"
    Assert-Fixture (!(Test-Path -LiteralPath $Fixture.Results)) "$Label must not save a measurement"
    Complete-Check $Label
}

function Write-GroupSamples($Fixture, [string]$Group, [int]$Count) {
    $rows = @(for ($i = 0; $i -lt $Count; $i++) {
        @{ group = $Group; fingerprint = ('{0:x12}' -f $i); run_id = "old-$i";
            phases_ms = @{ process_to_js = 1600; spawn_to_ready = 1500; total = 3600; splash_window_shown = 1800 };
            prewarm_ms = $(if ($Group -eq 'control') { 0 } else { 700 }); handoff_ms = 2100;
            prewarm_failures = 0; prewarm_timed_out = $false }
    })
    [IO.File]::WriteAllText($Fixture.Results, (ConvertTo-Json -InputObject $rows -Depth 5))
}

try {
    $null = New-Item -ItemType Directory -Path $root
    $env:TEMP = $root
    $env:DEEP_LEGENDS_STARTUP_PREWARM = 'fixture-original'
    $source = Get-Content -LiteralPath $ScriptPath -Raw -Encoding UTF8
    # Only the OS gate is removed in a disposable copy. Hashing, receipt lookup,
    # deduplication, collector arguments and the real Node collector stay intact.
    $gate = "if ([Environment]::OSVersion.Platform -ne 'Win32NT') { throw 'This measurement requires Windows.' }"
    Assert-Fixture ($source.Contains($gate)) 'platform gate changed; review the test adapter'
    $script:copy = Join-Path $root 'r82-startup-ab.ps1'
    $source.Replace($gate, '') | Set-Content -LiteralPath $script:copy -Encoding UTF8
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'r82-startup-ab-report.cjs') -Destination $root

    $fixture = New-Fixture
    $failure = Invoke-Fixture $fixture
    Assert-Fixture (!$failure) "versioned installer must collect successfully (got: $failure)"
    $saved = @(Get-Content -LiteralPath $fixture.Results -Raw -Encoding UTF8 | ConvertFrom-Json)
    Assert-Fixture ($global:r82ABFixtureState.Launches -eq 1 -and $saved.Count -eq 1 -and $saved[0].fingerprint -eq 'abcdef012345') 'versioned installer must save the receipt fingerprint'
    Assert-Fixture ($env:DEEP_LEGENDS_STARTUP_PREWARM -eq 'fixture-original') 'measurement must restore the prewarm environment'
    Complete-Check 'version filename and receipt fingerprint reach the real collector'

    $measured = $fixture
    $fixture = New-Fixture '[archive] any filename.exe'
    $failure = Invoke-Fixture $fixture
    Assert-Fixture (!$failure -and $global:r82ABFixtureState.Launches -eq 1) "renamed content must match by hash (got: $failure)"
    Complete-Check 'renamed content matches a differently named receipt asset'

    $fixture = $measured
    $before = [IO.File]::ReadAllText($fixture.Results)
    $renamed = Join-Path (Split-Path -Parent $fixture.Installer) '[archive] renamed installer.exe'
    Move-Item -LiteralPath $fixture.Installer -Destination $renamed
    $fixture.Installer = $renamed
    $failure = Invoke-Fixture $fixture 'prewarm'
    Assert-Fixture ($failure -match 'fingerprint has already been measured') 'duplicate fingerprint must be rejected after rename and group change'
    Assert-Fixture ($global:r82ABFixtureState.Launches -eq 0) 'duplicate fingerprint must fail before launch'
    Assert-Fixture ([IO.File]::ReadAllText($fixture.Results) -eq $before) 'duplicate must not alter existing measurements'
    Complete-Check 'duplicate fingerprint rejected before launch across filenames and groups'

    $fixture = New-Fixture
    [IO.File]::WriteAllText($fixture.Installer, 'unrelated content under the original filename')
    Assert-Rejected $fixture '安装包内容与 release-build.json 不匹配' 'mismatched content'

    $fixture = New-Fixture
    Remove-Item -LiteralPath $fixture.Receipt
    Assert-Rejected $fixture '把整个 dist/desktop 目录一起带过来' 'missing receipt'

    $fixture = New-Fixture
    [IO.File]::WriteAllText($fixture.Receipt, '{broken json')
    Assert-Rejected $fixture '无法读取 release-build.json' 'malformed receipt'

    $fixture = New-Fixture
    $receipt = Get-Content -LiteralPath $fixture.Receipt -Raw -Encoding UTF8 | ConvertFrom-Json
    $receipt.fingerprint = 'not-a-fingerprint'
    $receipt | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $fixture.Receipt -Encoding UTF8
    Assert-Rejected $fixture '缺少有效的 fingerprint 或 assets' 'invalid fingerprint'

    $fixture = New-Fixture
    $receipt = Get-Content -LiteralPath $fixture.Receipt -Raw -Encoding UTF8 | ConvertFrom-Json
    $receipt.assets = @()
    $receipt | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $fixture.Receipt -Encoding UTF8
    Assert-Rejected $fixture '缺少有效的 fingerprint 或 assets' 'invalid assets'

    foreach ($group in @('control', 'prewarm')) {
        $otherGroup = if ($group -eq 'control') { 'prewarm' } else { 'control' }
        foreach ($count in @(2, 3, 4)) {
            $fixture = New-Fixture
            Write-GroupSamples $fixture $group $count
            $before = [IO.File]::ReadAllText($fixture.Results)
            $failure = Invoke-Fixture $fixture $group
            if ($count -ge 3) {
                Assert-Fixture ($global:r82ABFixtureState.Launches -eq 0) "$group/$count full group must fail before launch"
                Assert-Fixture ($failure -and $failure.Contains("$group 组已有 $count 个有效样本") -and $failure.Contains("-Group $otherGroup")) "$group/$count must explain the full group and suggest the other group (got: $failure)"
                Assert-Fixture ([IO.File]::ReadAllText($fixture.Results) -eq $before) 'full group must preserve saved results'
            } else {
                Assert-Fixture (!$failure -and $global:r82ABFixtureState.Launches -eq 1) "$group/$count must allow the third sample (got: $failure)"
                $saved = @(Get-Content -LiteralPath $fixture.Results -Raw -Encoding UTF8 | ConvertFrom-Json)
                Assert-Fixture ($saved.Count -eq 3) 'third sample must reach the real collector'
            }
            Assert-Fixture ($env:DEEP_LEGENDS_STARTUP_PREWARM -eq 'fixture-original') 'preflight must preserve the environment'
            Complete-Check "$group count $count boundary"
        }
        $fixture = New-Fixture
        Write-GroupSamples $fixture $group 3
        $failure = Invoke-Fixture $fixture $otherGroup
        Assert-Fixture (!$failure -and $global:r82ABFixtureState.Launches -eq 1) "full $group must not block empty $otherGroup (got: $failure)"
        $saved = @(Get-Content -LiteralPath $fixture.Results -Raw -Encoding UTF8 | ConvertFrom-Json)
        Assert-Fixture (@($saved | Where-Object { $_.group -eq $otherGroup }).Count -eq 1) 'other group sample must reach the real collector'
        Complete-Check "full $group allows empty $otherGroup"
    }

    Assert-Fixture ($script:checks -eq 16) 'not all fixture checks ran'
    Write-Output 'PASS all 16 fixture checks'
} finally {
    $env:TEMP = $oldTemp
    [Environment]::SetEnvironmentVariable('DEEP_LEGENDS_STARTUP_PREWARM', $oldMode, 'Process')
    Remove-Variable -Name r82ABFixtureState -Scope Global -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
