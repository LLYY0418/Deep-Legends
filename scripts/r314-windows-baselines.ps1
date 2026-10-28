param([string]$Output)
$ErrorActionPreference='Stop'
New-Item -ItemType Directory -Force $Output|Out-Null
$env:RIOT_API_KEY='RGAPI-00000000-0000-0000-0000-000000000000'
foreach($version in @('0.13.0','0.13.2')){
 $dir=Join-Path $Output $version;New-Item -ItemType Directory -Force $dir|Out-Null
 gh release download "v$version" --pattern '*public.exe' --dir $dir
 if($LASTEXITCODE -ne 0){throw 'published baseline download failed'}
 $setup=(Get-ChildItem "$dir/*.exe").FullName
 Get-FileHash $setup -Algorithm SHA256|ConvertTo-Json|Set-Content "$dir/sha256.json"
 $install=Join-Path $dir 'installed';New-Item -ItemType Directory -Force $install|Out-Null
 $env:LOL_LOOT_DATA_DIR=Join-Path $dir 'install-data';$env:WEBVIEW2_USER_DATA_FOLDER=Join-Path $dir 'webview-profile'
 $child=Start-Process $setup -ArgumentList "--update --dest `"$install`"" -PassThru -RedirectStandardOutput "$dir/install-stdout.log" -RedirectStandardError "$dir/install-stderr.log"
 if(-not $child.WaitForExit(180000)){Stop-Process -Id $child.Id -Force;throw 'baseline install timeout'};$child.WaitForExit();if($child.ExitCode -ne 0){throw 'baseline install failed'}
 Get-CimInstance Win32_Process|Where-Object{$_.ExecutablePath -and $_.ExecutablePath.StartsWith($install,[StringComparison]::OrdinalIgnoreCase)}|ForEach-Object{Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue}
 Remove-Item Env:WEBVIEW2_USER_DATA_FOLDER -ErrorAction SilentlyContinue
 node ./scripts/r314-windows-edges.cjs "$install/Deep Legends.exe" "$dir/edges" $version
 if($LASTEXITCODE -ne 0){throw 'baseline edge scan failed'}
}
