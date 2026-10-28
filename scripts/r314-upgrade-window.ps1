param([Parameter(Mandatory=$true)][string]$Setup,[Parameter(Mandatory=$true)][string]$Output,[int]$Count=10,[switch]$Warmup,[switch]$Enforce,[switch]$WaitForPage)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'r313-measurement-fixture.ps1')
New-Item -ItemType Directory -Force $Output | Out-Null
Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class R314Windows {
 public delegate bool EnumProc(IntPtr hwnd, IntPtr data);
 [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc callback, IntPtr data);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr hwnd,StringBuilder text,int count);
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hwnd,out uint pid);
 [DllImport("winmm.dll")] public static extern uint timeBeginPeriod(uint period);
 [DllImport("winmm.dll")] public static extern uint timeEndPeriod(uint period);
 public static bool Visible(int process) {bool found=false;EnumWindows((h,d)=>{uint p;GetWindowThreadProcessId(h,out p);var t=new StringBuilder(256);GetWindowText(h,t,256);if(p==process&&IsWindowVisible(h)&&t.ToString()=="Deep Legends 安装程序")found=true;return true;},IntPtr.Zero);return found;}
}
'@
$rows=@();$setupPath=(Resolve-Path $Setup).Path
go build -o (Join-Path $Output 'r314-launch.exe') ./scripts/r314-installer-launch
if($LASTEXITCODE -ne 0){throw 'cmd.Start helper build failed'}
[R314Windows]::timeBeginPeriod(1) | Out-Null
try {
 for($i=1;$i -le $Count;$i++) {
  $trial=Join-Path $Output "trial-$i";New-Item -ItemType Directory -Force $trial | Out-Null
  # A distinct newly written file for every sample, never reused across trials.
  $trialSetup=Join-Path $trial ('Setup-'+[Guid]::NewGuid().ToString('N')+'.exe');Copy-Item $setupPath $trialSetup
  $warmMs=$null
  if($Warmup){$warmWatch=[Diagnostics.Stopwatch]::StartNew();$warm=Start-Process -FilePath $trialSetup -ArgumentList '--warmup' -PassThru -WindowStyle Hidden;if(-not $warm.WaitForExit(30000)){Stop-Process -Id $warm.Id -Force;throw 'warmup timed out'};$warm.WaitForExit();if($warm.ExitCode -ne 0){throw 'warmup failed'};$warmMs=$warmWatch.Elapsed.TotalMilliseconds}
  $env:LOL_LOOT_DATA_DIR=$trial
  $profile=Join-Path $Output 'webview-profile';$env:WEBVIEW2_USER_DATA_FOLDER=$profile
  $dest=Join-Path $trial 'installed';New-Item -ItemType Directory -Force $dest | Out-Null
  $ready=Join-Path $trial 'parent-ready';$escaped=$ready.Replace("'","''");$code="[IO.File]::WriteAllText('$escaped','ready');Start-Sleep -Seconds 90";$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($code))
  $parent=Start-Process pwsh -ArgumentList '-NoProfile','-EncodedCommand',$encoded -PassThru -WindowStyle Hidden
  $child=$null;$launcher=$null
  try {
   Wait-R313ParentReady $ready $parent
   $startupLog=Join-Path $env:TEMP 'DeepLegendsSetup-startup.log';$logStart=if(Test-Path $startupLog){(Get-Item $startupLog).Length}else{0}
   $receipt=Join-Path $trial 'spawn-receipt.json'
   $helper=Join-Path $Output 'r314-launch.exe'
   if(-not(Test-Path $helper)){throw 'actual cmd.Start measurement helper missing'}
   $launcher=Start-Process -FilePath $helper -ArgumentList "`"$trialSetup`" `"$receipt`" `"$(Join-Path $trial 'stdout.txt')`" `"$(Join-Path $trial 'stderr.txt')`" --update --parent-first --parent-pid $($parent.Id) --dest `"$dest`"" -PassThru -WindowStyle Hidden
   $deadline=[DateTime]::UtcNow.AddSeconds(30)
   while(-not(Test-Path $receipt)){if($launcher.HasExited -or [DateTime]::UtcNow -gt $deadline){throw 'Go cmd.Start helper failed'};Start-Sleep -Milliseconds 5}
   $spawn=Get-Content $receipt -Raw|ConvertFrom-Json;$startUtc=[long]$spawn.start_utc_ms;$spawnReturned=$spawn.spawn_returned_ms
   $child=Get-Process -Id $spawn.pid;$watch=[Diagnostics.Stopwatch]::StartNew()

   while(-not [R314Windows]::Visible($child.Id)) {if($watch.Elapsed.TotalSeconds -gt 30 -or $child.HasExited){throw "No visible installer at trial $i"};Start-Sleep -Milliseconds 10}
   $window=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()-$startUtc
   $deadline=[DateTime]::UtcNow.AddSeconds(25)
   do {Start-Sleep -Milliseconds 10;$acknowledged=Test-R313Acknowledged (Join-Path $trial 'stdout.txt')} while(-not $acknowledged -and [DateTime]::UtcNow -lt $deadline)
   if(-not $acknowledged){throw "Missing compatible ACK trial $i"}
   $timing=$null;$file=Join-Path $trial 'update-install-timing.json'
   if($WaitForPage) {
    $pageDeadline=[DateTime]::UtcNow.AddSeconds(25)
    # Do not poll a file that the producer is atomically replacing: even a
    # delete-sharing reader can briefly deny MoveFileEx replacement on Windows.
    # The current PID's ready log is emitted after the timing write completes.
    do {Start-Sleep -Milliseconds 10;$ready=Test-R313PageReady $startupLog $child.Id $logStart} while(-not $ready -and [DateTime]::UtcNow -lt $pageDeadline)
    if($ready){$timing=(Read-R313ProbeText $file)|ConvertFrom-Json}
    if(-not $timing.webview_ready){throw "Candidate WebView did not finish initializing at trial $i"}
   } elseif(Test-Path $file){$timing=Get-Content $file -Raw|ConvertFrom-Json}
   $createdUtc=([DateTimeOffset]$child.StartTime.ToUniversalTime()).ToUnixTimeMilliseconds()
   $mainMs=if($null -ne $timing.process_created_to_main_ms){$createdUtc+$timing.process_created_to_main_ms-$startUtc}else{$null}
   $pageStage=if($null -ne $timing.page_visible){$timing.page_visible}else{$timing.webview_ready}
   $rows+=@{trial=$i;new_file_bytes=(Get-Item $trialSetup).Length;warmup_ms=$warmMs;spawn_returned_ms=$spawnReturned;spawn_to_shell_ms=$window;spawn_to_main_ms=$mainMs;legacy_installer_start_proxy_ms=$timing.installer_start-$startUtc;spawn_to_window_ms=$pageStage-$startUtc;page_boundary=if($timing.page_visible){'page_visible'}else{'legacy_webview_ready'};timing=$timing}
   $rows|ConvertTo-Json -Depth 10|Set-Content (Join-Path $Output 'samples.json') -Encoding utf8
  } finally {
   if($child -and -not $child.HasExited){& taskkill /PID $child.Id /T /F 2>$null | Out-Null}
   if($child -and -not $child.WaitForExit(5000)){throw 'Owned installer did not exit after cleanup'}
   if(-not $parent.HasExited){Stop-Process -Id $parent.Id -Force};$parent.WaitForExit(5000)|Out-Null
   if($launcher -and -not $launcher.WaitForExit(5000)){Stop-Process -Id $launcher.Id -Force}
   # Match production's fixed WebView DataPath. Only Setup is newly written;
   # an unrelated fresh browser profile per launch is not a first-execution test.
   $profileDeadline=[DateTime]::UtcNow.AddSeconds(10)
   do{$owned=@(Get-CimInstance Win32_Process|Where-Object{$_.Name -eq 'msedgewebview2.exe' -and $_.CommandLine -and $_.CommandLine.IndexOf($profile,[StringComparison]::OrdinalIgnoreCase) -ge 0});foreach($ownedProcess in $owned){if($ownedProcess -and $ownedProcess.ProcessId -gt 0){Stop-Process -Id $ownedProcess.ProcessId -Force -ErrorAction SilentlyContinue}};if($owned.Count -eq 0){break};if([DateTime]::UtcNow -ge $profileDeadline){throw 'WebView process cleanup timed out'};Start-Sleep -Milliseconds 30}while($true)
   Remove-Item $trialSetup -Force -ErrorAction SilentlyContinue
   if(Test-Path "$env:TEMP/DeepLegendsSetup-startup.log"){Get-Content "$env:TEMP/DeepLegendsSetup-startup.log" -Tail 100|Set-Content (Join-Path $trial 'startup.log')}
  }
 }
} finally {if($profile){$deadline=[DateTime]::UtcNow.AddSeconds(10);do{try{Remove-Item -Recurse -Force $profile -ErrorAction Stop;break}catch{if(-not(Test-Path $profile)){break};if([DateTime]::UtcNow -ge $deadline){throw};Start-Sleep -Milliseconds 50}}while($true)};[R314Windows]::timeEndPeriod(1)|Out-Null;Remove-Item Env:LOL_LOOT_DATA_DIR -ErrorAction SilentlyContinue;Remove-Item Env:WEBVIEW2_USER_DATA_FOLDER -ErrorAction SilentlyContinue}
$values=@($rows.spawn_to_window_ms|Sort-Object)
$result=@{profile_policy='one-production-style-profile-per-group; fresh-Setup-per-trial';setup_bytes=(Get-Item $setupPath).Length;sha256=(Get-FileHash $setupPath -Algorithm SHA256).Hash.ToLowerInvariant();count=$rows.Count;median_ms=$values[[int][Math]::Floor($values.Count/2)];p95_ms=$values[[Math]::Ceiling($values.Count*.95)-1];rows=$rows}
$result|ConvertTo-Json -Depth 12|Set-Content (Join-Path $Output 'result.json') -Encoding utf8
$result|ConvertTo-Json -Depth 3
if($Enforce -and ($result.median_ms -gt 900 -or $result.p95_ms -gt 1500)){throw 'R314 progress page budget exceeded (median<=900ms, p95<=1500ms)'}
