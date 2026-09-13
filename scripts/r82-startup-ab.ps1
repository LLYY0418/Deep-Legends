# Runs a user-built installer and collects one cold sample; never builds or packages.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Installer,
    [Parameter(Mandatory = $true)][ValidateSet('control', 'prewarm')][string]$Group,
    [string]$Results = (Join-Path $PSScriptRoot '..\output\r82-ab.json'),
    [string]$DiagnosticsPath = ''
)
$ErrorActionPreference = 'Stop'
if ([Environment]::OSVersion.Platform -ne 'Win32NT') { throw 'This measurement requires Windows.' }
$node = (Get-Command node -ErrorAction Stop).Source
$setupPath = (Resolve-Path -LiteralPath $Installer).Path
$receiptPath = Join-Path (Split-Path -Parent $setupPath) 'release-build.json'
if (!(Test-Path -LiteralPath $receiptPath -PathType Leaf)) {
    throw '找不到 release-build.json，无法确认这是不是一次全新构建；把整个 dist/desktop 目录一起带过来，不要只拷贝安装包本身'
}
try {
    $receipt = Get-Content -LiteralPath $receiptPath -Raw -Encoding UTF8 | ConvertFrom-Json
} catch {
    throw '无法读取 release-build.json，请从同一次完整构建重新复制 dist/desktop 目录'
}
if ($receipt.fingerprint -isnot [string] -or $receipt.fingerprint -notmatch '^[0-9a-f]{12}$' -or
    $receipt.assets -isnot [System.Management.Automation.PSCustomObject]) {
    throw 'release-build.json 缺少有效的 fingerprint 或 assets，请重新复制同一次完整构建的记录'
}
$setupHash = (Get-FileHash -LiteralPath $setupPath -Algorithm SHA256).Hash
# Match content, not a filename: versioned and renamed artifacts are both valid.
$matchingAsset = $receipt.assets.PSObject.Properties | Where-Object { $_.Value -is [string] -and $_.Value -eq $setupHash }
if (!$matchingAsset) {
    throw '安装包内容与 release-build.json 不匹配；请使用同一次构建的安装包和记录，不要混用新旧文件'
}
$fingerprint = $receipt.fingerprint.ToLowerInvariant()
if (Get-Process -Name 'Deep Legends', 'loot-service' -ErrorAction SilentlyContinue) {
    throw 'Close Deep Legends completely before a cold run. This script never kills the application.'
}
if (Test-Path -LiteralPath $Results) {
    $previous = Get-Content -LiteralPath $Results -Raw | ConvertFrom-Json
    if ($previous | Where-Object { $_.fingerprint -eq $fingerprint }) { throw 'This fingerprint has already been measured.' }
    $groupCount = @($previous | Where-Object { $_.group -eq $Group }).Count
    if ($groupCount -ge 3) {
        $otherGroup = if ($Group -eq 'control') { 'prewarm' } else { 'control' }
        throw "$Group 组已有 $groupCount 个有效样本（上限 3 个）；如需测另一组，请改用 -Group $otherGroup。未启动安装器。"
    }
}
if (!$DiagnosticsPath) {
    $dataRoot = if ($env:LOL_LOOT_DATA_DIR) { $env:LOL_LOOT_DATA_DIR } else { Join-Path $env:LOCALAPPDATA 'LOLLootAssistant' }
    $DiagnosticsPath = Join-Path $dataRoot 'logs\diagnostics.jsonl'
}
$since = [DateTime]::UtcNow.ToString('o')
$oldMode = [Environment]::GetEnvironmentVariable('DEEP_LEGENDS_STARTUP_PREWARM', 'Process')
try {
    $env:DEEP_LEGENDS_STARTUP_PREWARM = if ($Group -eq 'prewarm') { '1' } else { '0' }
    Write-Host "Run $Group / $fingerprint. Complete installation normally; do not reopen the app during collection."
    $setup = Start-Process -FilePath $setupPath -PassThru
} finally {
    [Environment]::SetEnvironmentVariable('DEEP_LEGENDS_STARTUP_PREWARM', $oldMode, 'Process')
}
if (!$setup.WaitForExit(900000)) { throw 'Installer has not exited after 15 minutes; preserve logs. No processes were killed.' }
if ($setup.ExitCode -ne 0) { throw "Installer failed with exit code $($setup.ExitCode); preserve startup log." }
& $node (Join-Path $PSScriptRoot 'r82-startup-ab-report.cjs') --collect $Group $fingerprint $setup.Id $since $DiagnosticsPath (Join-Path $env:TEMP 'DeepLegendsSetup-startup.log') $Results
if ($LASTEXITCODE -ne 0) { throw 'Collection failed; see the error above.' }
