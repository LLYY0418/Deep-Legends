# ConvertFrom-Json can already return DateTime or DateTimeOffset. Parsing such
# an object as a string drops fractional seconds through culture formatting.
function Get-R252DiagnosticTimeMilliseconds($Value) {
    if ($Value -is [DateTimeOffset]) { return $Value.ToUnixTimeMilliseconds() }
    if ($Value -is [DateTime]) { return ([DateTimeOffset]$Value).ToUnixTimeMilliseconds() }
    return ([DateTimeOffset]::Parse([string]$Value, [Globalization.CultureInfo]::InvariantCulture)).ToUnixTimeMilliseconds()
}
