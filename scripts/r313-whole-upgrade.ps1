param([Parameter(Mandatory=$true)][string]$Baseline,[Parameter(Mandatory=$true)][string]$Candidate,[Parameter(Mandatory=$true)][string]$Output,[int]$Pairs=3)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'r313-measurement-fixture.ps1')
New-Item -ItemType Directory -Force $Output|Out-Null
$baselinePath=(Resolve-Path $Baseline).Path;$candidatePath=(Resolve-Path $Candidate).Path
$env:RIOT_API_KEY='RGAPI-00000000-0000-0000-0000-000000000000'
Add-Type @'
using System;using System.Runtime.InteropServices;
public static class R313AppWindow {
 public delegate bool EnumProc(IntPtr h,IntPtr d);
 [DllImport("user32.dll")]public static extern bool EnumWindows(EnumProc cb,IntPtr d);
 [DllImport("user32.dll")]public static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll")]public static extern uint GetWindowThreadProcessId(IntPtr h,out uint p);
 public static bool Visible(uint p){bool found=false;EnumWindows((h,d)=>{uint own;GetWindowThreadProcessId(h,out own);if(own==p&&IsWindowVisible(h))found=true;return true;},IntPtr.Zero);return found;}
}
'@
function Get-Owned([string]$Directory){return @(Get-CimInstance Win32_Process|Where-Object{$_.ExecutablePath -and $_.ExecutablePath.StartsWith($Directory.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)})}
function Stop-Owned([string]$Directory){$deadline=[DateTime]::UtcNow.AddSeconds(10);do{$owned=Get-Owned $Directory;if($owned.Count -eq 0){return};foreach($p in $owned){Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue};Start-Sleep -Milliseconds 50}while([DateTime]::UtcNow -lt $deadline);throw 'Owned application cleanup did not finish'}
function Remove-OwnedInstall([string]$Directory){$deadline=[DateTime]::UtcNow.AddSeconds(10);do{Stop-Owned $Directory;try{Remove-Item -Recurse -Force $Directory;return}catch{$code=$_.Exception.HResult -band 0xffff;if($code -notin @(32,33) -or [DateTime]::UtcNow -ge $deadline){throw};Start-Sleep -Milliseconds 50}}while($true)}
function Wait-App([string]$Directory){$deadline=[DateTime]::UtcNow.AddSeconds(40);do{$owned=Get-Owned $Directory;$visible=@($owned|Where-Object{$_.Name -eq 'Deep Legends.exe' -and [R313AppWindow]::Visible($_.ProcessId)});$backend=@($owned|Where-Object{$_.Name -eq 'loot-service.exe'});if($visible.Count -gt 0 -and $backend.Count -gt 0){return};Start-Sleep -Milliseconds 50}while([DateTime]::UtcNow -lt $deadline);throw 'Installed application never became visible with its backend'}
$rows=@()
try {
 for($pair=1;$pair -le $Pairs;$pair++) {foreach($mode in @('baseline','candidate')){
  $trial=Join-Path $Output "$pair-$mode";$install=Join-Path $trial 'installed';$data=Join-Path $trial 'data';New-Item -ItemType Directory -Force $install,$data|Out-Null;$env:LOL_LOOT_DATA_DIR=$data
  $initialProfile=Join-Path $trial 'initial-webview-profile';$profile=Join-Path $trial 'upgrade-webview-profile';$env:WEBVIEW2_USER_DATA_FOLDER=$initialProfile
  $initial=Start-Process -FilePath $baselinePath -ArgumentList "--update --dest `"$install`"" -PassThru -RedirectStandardOutput (Join-Path $trial 'initial-stdout.log') -RedirectStandardError (Join-Path $trial 'initial-stderr.log')
  if(-not $initial.WaitForExit(180000)) {Stop-Process $initial.Id -Force;throw 'Baseline installation timed out'};$initial.WaitForExit();if($initial.ExitCode -ne 0){throw 'Baseline installation failed'};Wait-App $install;Stop-Owned $install
  Stop-R313WebViewProfile $initialProfile;$env:WEBVIEW2_USER_DATA_FOLDER=$profile
  $signal=Join-Path $trial 'parent-exit.signal';$escaped=$signal.Replace("'","''");$code="while(-not(Test-Path '$escaped')){Start-Sleep -Milliseconds 10};Start-Sleep -Milliseconds 300";$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($code))
  $parent=Start-Process pwsh -ArgumentList '-NoProfile','-EncodedCommand',$encoded -WindowStyle Hidden -PassThru
  $setup=if($mode -eq 'candidate'){$candidatePath}else{$baselinePath};$child=$null
  try {
   $watch=[Diagnostics.Stopwatch]::StartNew();$stdout=Join-Path $trial 'upgrade-stdout.log'
   $child=Start-Process -FilePath $setup -ArgumentList "--update --parent-first --parent-pid $($parent.Id) --dest `"$install`" --recovery-exe `"$install/Deep Legends.exe`"" -PassThru -RedirectStandardOutput $stdout -RedirectStandardError (Join-Path $trial 'upgrade-stderr.log')
   do {Start-Sleep -Milliseconds 10;$acknowledged=Test-R313Acknowledged $stdout;if($watch.Elapsed.TotalSeconds -gt 30 -or $child.HasExited){throw 'Installer did not acknowledge upgrade'}}while(-not $acknowledged)
   $ackMs=$watch.Elapsed.TotalMilliseconds;New-Item -ItemType File $signal|Out-Null
   if(-not $child.WaitForExit(180000)){throw 'Whole upgrade timed out'};$child.WaitForExit();if($child.ExitCode -ne 0){throw 'Upgrade installer failed'}
   Wait-App $install;$readyMs=$watch.Elapsed.TotalMilliseconds
   $version=(Get-Item (Join-Path $install 'Deep Legends.exe')).VersionInfo.FileVersion
   $expected=if($mode -eq 'candidate'){(& node -p "require('./desktop/package.json').version").Trim()}else{'0.13.1'}
   if($version -ne $expected){throw "Installed version $version does not match $expected"}
   $rows+=@{pair=$pair;mode=$mode;ack_ms=$ackMs;spawn_to_app_ready_ms=$readyMs;version=$version;parent_flush_ms=300}
   $rows|ConvertTo-Json -Depth 10|Set-Content (Join-Path $Output 'samples.json') -Encoding utf8
   $stages=Join-Path $data 'update-install-stages.txt';if(Test-Path $stages){Copy-Item $stages (Join-Path $trial 'stages.txt')}
  }finally{if($child -and -not $child.HasExited){& taskkill /PID $child.Id /T /F 2>$null|Out-Null};if(-not $parent.HasExited){Stop-Process -Id $parent.Id -Force};Stop-R313WebViewProfile $profile;Remove-OwnedInstall $install}
 }}
 $old=@($rows|Where-Object mode -eq baseline|ForEach-Object spawn_to_app_ready_ms|Sort-Object);$new=@($rows|Where-Object mode -eq candidate|ForEach-Object spawn_to_app_ready_ms|Sort-Object)
 $result=@{rows=$rows;baseline_median_ms=$old[[Math]::Floor($old.Count/2)];candidate_median_ms=$new[[Math]::Floor($new.Count/2)]}
 $result|ConvertTo-Json -Depth 10|Set-Content (Join-Path $Output 'result.json') -Encoding utf8;$result|ConvertTo-Json -Depth 10
 if($result.candidate_median_ms -gt $result.baseline_median_ms){throw 'Whole upgrade is slower than 0.13.1 (first round medians)'}
}finally{Remove-Item Env:RIOT_API_KEY -ErrorAction SilentlyContinue;Remove-Item Env:LOL_LOOT_DATA_DIR -ErrorAction SilentlyContinue;Remove-Item Env:WEBVIEW2_USER_DATA_FOLDER -ErrorAction SilentlyContinue}
