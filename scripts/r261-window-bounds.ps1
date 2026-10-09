# The published app writes this file in place. File existence (or even a valid
# earlier snapshot) is not evidence that its normal close handler has finished.
function Wait-R261PublishedWindowBounds {
    param(
        [Parameter(Mandatory=$true)][string]$ProcessPath,
        [Parameter(Mandatory=$true)][string]$BoundsFile,
        [Parameter(Mandatory=$true)][DateTime]$Deadline,
        [Parameter(Mandatory=$true)][string]$EvidencePath,
        [ScriptBlock]$ListProcesses = {
            param($Executable)
            Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $Executable }
        }
    )
    $started = Get-Date
    $closed = @{}
    $observations = @()
    $previous = ''
    $polls = 0
    $outcome = 'failure'
    $failure = $null
    try {
        while ((Get-Date) -lt $Deadline) {
            $polls++
            $processes = @(& $ListProcesses $ProcessPath)
            foreach ($process in $processes) {
                if ($process.MainWindowHandle -ne 0 -and $process.MainWindowTitle -eq 'Deep Legends' -and -not $closed.ContainsKey($process.Id)) {
                    $accepted = $process.CloseMainWindow()
                    $observations += @{at=(Get-Date).ToUniversalTime().ToString('o');event='close_requested';pid=$process.Id;accepted=$accepted}
                    if ($accepted) { $closed[$process.Id] = $true }
                }
            }
            $state = 'missing'
            $text = $null
            $bytes = $null
            $readError = $null
            if (Test-Path -LiteralPath $BoundsFile) {
                try {
                    $text = [IO.File]::ReadAllText($BoundsFile)
                    $bytes = [Text.Encoding]::UTF8.GetByteCount($text)
                    $state = 'empty'
                    if (-not [string]::IsNullOrWhiteSpace($text)) {
                        $state = 'invalid_json'
                        $value = $text | ConvertFrom-Json -ErrorAction Stop
                        $state = 'invalid_bounds'
                        if ($null -ne $value -and $value.width -gt 0 -and $value.height -gt 0 -and $null -ne $value.x -and $null -ne $value.y) { $state = 'valid' }
                    }
                } catch { $readError = $_.Exception.Message }
            }
            $ids = @($processes | ForEach-Object { $_.Id } | Sort-Object)
            $key = "$($ids -join ',')|$state|$bytes|$readError"
            if ($key -ne $previous) {
                $observations += @{at=(Get-Date).ToUniversalTime().ToString('o');event='observed';process_ids=$ids;bounds_state=$state;bytes=$bytes;read_error=$readError}
                $previous = $key
            }
            # All Electron processes must have exited after an accepted close.
            # Never force-kill a writer merely because its path appeared.
            if ($closed.Count -gt 0 -and $processes.Count -eq 0) {
                if ($state -ne 'valid') { throw "Invalid actual 0.12.76 window bounds after normal close: $state" }
                $outcome = 'success'
                return $text
            }
            Start-Sleep -Milliseconds 50
        }
        throw 'Timed out waiting for normal 0.12.76 window close and valid bounds'
    } catch {
        $failure = $_.Exception.Message
        throw
    } finally {
        @{started_at=$started.ToUniversalTime().ToString('o');ended_at=(Get-Date).ToUniversalTime().ToString('o');deadline=$Deadline.ToUniversalTime().ToString('o');polls=$polls;outcome=$outcome;failure=$failure;process_path=$ProcessPath;bounds_path=$BoundsFile;closed_process_ids=@($closed.Keys);observations=$observations} |
            ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $EvidencePath -Encoding utf8
    }
}
