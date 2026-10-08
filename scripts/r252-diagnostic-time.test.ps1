$ErrorActionPreference = 'Stop'
$source = Get-Content (Join-Path $PSScriptRoot 'r252-diagnostic-time.ps1') -Raw
$fixed = [ScriptBlock]::Create($source)
$old = [ScriptBlock]::Create('function Get-R252DiagnosticTimeMilliseconds($Value) { return ([DateTimeOffset]::Parse($Value)).ToUnixTimeMilliseconds() }')
function Assert-TimingFractions([ScriptBlock]$Implementation) {
    & {
        param($Implementation)
        . $Implementation
        $iso = '2026-10-08T02:29:51.5344331Z'
        $json = '{"time":"2026-10-08T02:29:51.5344331Z"}' | ConvertFrom-Json
        foreach ($culture in @('en-US','zh-CN')) {
            [Threading.Thread]::CurrentThread.CurrentCulture = [Globalization.CultureInfo]::GetCultureInfo($culture)
            foreach ($value in @($iso,[DateTimeOffset]::Parse($iso),[DateTime]::Parse($iso,[Globalization.CultureInfo]::InvariantCulture,[Globalization.DateTimeStyles]::RoundtripKind),$json.time)) {
                $ms = Get-R252DiagnosticTimeMilliseconds $value
                if ($ms -ne 1791426591534L) { throw "R252_ASSERT_FRACTION: $culture/$($value.GetType().Name) -> $ms" }
                if ($ms -lt 1791426591181L) { throw 'R252_ASSERT_INSTALL_SELECTOR: this exact CI install was rejected' }
            }
            if ((Get-R252DiagnosticTimeMilliseconds '2026-10-08T02:29:51.180Z') -ge 1791426591181L) { throw 'R252_ASSERT_OLD_INSTALL: earlier event was accepted' }
        }
    } $Implementation
}
$originalCulture = [Threading.Thread]::CurrentThread.CurrentCulture
try {
    Assert-TimingFractions $fixed
    $killed = $false
    try { Assert-TimingFractions $old } catch {
        if ($_.Exception.Message -notmatch '^R252_ASSERT_FRACTION:') { throw }
        Write-Output ("R252 old Parse(object) assertion mutation killed: " + $_.Exception.Message)
        $killed = $true
    }
    if (-not $killed) { throw 'R252 mutation did not fail its fractional timestamp assertion' }
    $upgrade = Get-Content (Join-Path $PSScriptRoot 'r206-real-upgrade-windows.ps1') -Raw
    if ($upgrade -notmatch 'Get-R252DiagnosticTimeMilliseconds \$_\.time') { throw 'Real upgrade selector must use the verified conversion' }
    Write-Output 'R252 native diagnostic timestamp fractions PASS; exact install and earlier-event rejection retained'
} finally { [Threading.Thread]::CurrentThread.CurrentCulture = $originalCulture }
