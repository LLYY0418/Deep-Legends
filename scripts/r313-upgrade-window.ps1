param([Parameter(Mandatory=$true)][string]$Setup,[Parameter(Mandatory=$true)][string]$Output,[int]$Count=15,[switch]$Enforce,[switch]$WaitForPage)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'r313-measurement-fixture.ps1')
New-Item -ItemType Directory -Force $Output | Out-Null
Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class R313Windows {
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
[R313Windows]::timeBeginPeriod(1) | Out-Null
try {
 for($i=1;$i -le $Count;$i++) {
  $trial=Join-Path $Output "trial-$i";New-Item -ItemType Directory -Force $trial | Out-Null
  $env:LOL_LOOT_DATA_DIR=$trial
  $profile=Join-Path $trial 'webview-profile';$env:WEBVIEW2_USER_DATA_FOLDER=$profile
  $dest=Join-Path $trial 'installed';New-Item -ItemType Directory -Force $dest | Out-Null
  $ready=Join-Path $trial 'parent-ready';$escaped=$ready.Replace("'","''");$code="[IO.File]::WriteAllText('$escaped','ready');Start-Sleep -Seconds 90";$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($code))
  $parent=Start-Process pwsh -ArgumentList '-NoProfile','-EncodedCommand',$encoded -PassThru -WindowStyle Hidden
  $child=$null
  try {
   Wait-R313ParentReady $ready $parent
   $watch=[Diagnostics.Stopwatch]::StartNew()
   $child=Start-Process -FilePath $setupPath -ArgumentList "--update --parent-first --parent-pid $($parent.Id) --dest `"$dest`"" -PassThru -RedirectStandardOutput (Join-Path $trial 'stdout.txt') -RedirectStandardError (Join-Path $trial 'stderr.txt')
   $spawnReturned=$watch.Elapsed.TotalMilliseconds
   while(-not [R313Windows]::Visible($child.Id)) {if($watch.Elapsed.TotalSeconds -gt 30 -or $child.HasExited){throw "No visible installer at trial $i"};Start-Sleep -Milliseconds 10}
   $window=$watch.Elapsed.TotalMilliseconds
   $deadline=[DateTime]::UtcNow.AddSeconds(25)
   do {Start-Sleep -Milliseconds 10;$acknowledged=Test-R313Acknowledged (Join-Path $trial 'stdout.txt')} while(-not $acknowledged -and [DateTime]::UtcNow -lt $deadline)
   if(-not $acknowledged){throw "Missing compatible ACK trial $i"}
   $timing=$null;$file=Join-Path $trial 'update-install-timing.json'
   if($WaitForPage) {
    $pageDeadline=[DateTime]::UtcNow.AddSeconds(25)
    do {Start-Sleep -Milliseconds 10;if(Test-Path $file){$timing=(Read-R313ProbeText $file)|ConvertFrom-Json}} while(-not $timing.webview_ready -and [DateTime]::UtcNow -lt $pageDeadline)
    if(-not $timing.webview_ready){throw "Candidate WebView did not finish initializing at trial $i"}
   } elseif(Test-Path $file){$timing=Get-Content $file -Raw|ConvertFrom-Json}
   $rows+=@{trial=$i;spawn_returned_ms=$spawnReturned;spawn_to_window_ms=$window;timing=$timing}
  } finally {
   if($child -and -not $child.HasExited){& taskkill /PID $child.Id /T /F 2>$null | Out-Null}
   if($child -and -not $child.WaitForExit(5000)){throw 'Owned installer did not exit after cleanup'}
   if(-not $parent.HasExited){Stop-Process -Id $parent.Id -Force};$parent.WaitForExit(5000)|Out-Null
   Stop-R313WebViewProfile $profile
   if(Test-Path "$env:TEMP/DeepLegendsSetup-startup.log"){Get-Content "$env:TEMP/DeepLegendsSetup-startup.log" -Tail 100|Set-Content (Join-Path $trial 'startup.log')}
  }
 }
} finally {[R313Windows]::timeEndPeriod(1)|Out-Null;Remove-Item Env:LOL_LOOT_DATA_DIR -ErrorAction SilentlyContinue;Remove-Item Env:WEBVIEW2_USER_DATA_FOLDER -ErrorAction SilentlyContinue}
$values=@($rows.spawn_to_window_ms|Sort-Object)
$result=@{setup_bytes=(Get-Item $setupPath).Length;sha256=(Get-FileHash $setupPath -Algorithm SHA256).Hash.ToLowerInvariant();count=$rows.Count;median_ms=$values[[int][Math]::Floor($values.Count/2)];p95_ms=$values[[Math]::Ceiling($values.Count*.95)-1];rows=$rows}
$result|ConvertTo-Json -Depth 12|Set-Content (Join-Path $Output 'result.json') -Encoding utf8
$result|ConvertTo-Json -Depth 3
if($Enforce -and ($result.median_ms -gt 700 -or $result.p95_ms -gt 1500)){throw 'R313 native window budget exceeded (median<=700ms, p95<=1500ms)'}
