param([Parameter(Mandatory=$true)][string]$Setup,[Parameter(Mandatory=$true)][string]$Output)
$ErrorActionPreference='Stop'
$root=Join-Path $env:RUNNER_TEMP 'r313-published-check';$install=Join-Path $root 'installed';$env:LOL_LOOT_DATA_DIR=Join-Path $root 'data'
New-Item -ItemType Directory -Force $install,$Output|Out-Null
function Stop-Owned{Get-CimInstance Win32_Process|Where-Object{$_.ExecutablePath -and $_.ExecutablePath.StartsWith($install+'\',[StringComparison]::OrdinalIgnoreCase)}|ForEach-Object{Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue}}
try{
 $initial=Start-Process -FilePath (Resolve-Path $Setup) -ArgumentList "--update --dest `"$install`"" -PassThru -RedirectStandardOutput (Join-Path $Output 'install-stdout.log') -RedirectStandardError (Join-Path $Output 'install-stderr.log')
 if(-not $initial.WaitForExit(180000)){Stop-Process $initial.Id -Force;throw 'Installation timed out'};$initial.WaitForExit();if($initial.ExitCode -ne 0){throw 'Installation failed'}
 $exe=Join-Path $install 'Deep Legends.exe';if((Get-Item $exe).VersionInfo.FileVersion -ne '0.13.1'){throw 'Not the actual published 0.13.1 client'}
 Start-Sleep -Seconds 2;Stop-Owned;Start-Sleep -Milliseconds 300
 $listener=New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback,0);$listener.Start();$port=$listener.LocalEndpoint.Port;$listener.Stop()
 $client=Start-Process $exe -ArgumentList "--remote-debugging-port=$port" -PassThru;$env:R265_EXPECTED_ELECTRON_PID=[string]$client.Id
 node scripts/r313-published-check.cjs $port $Output
 if($LASTEXITCODE -ne 0){throw 'Actual published 0.13.1 update detection failed'}
}finally{Stop-Owned;Remove-Item -Recurse -Force $root -ErrorAction SilentlyContinue;Remove-Item Env:LOL_LOOT_DATA_DIR -ErrorAction SilentlyContinue;Remove-Item Env:R265_EXPECTED_ELECTRON_PID -ErrorAction SilentlyContinue}
