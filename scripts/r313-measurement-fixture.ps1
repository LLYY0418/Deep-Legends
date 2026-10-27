function Open-R313ProbeFile([string]$Path) {
 # Share all access and close each snapshot promptly. On Windows the readiness
 # probe waits for the producer's log event before opening its timing snapshot.
 return [IO.File]::Open($Path,[IO.FileMode]::Open,[IO.FileAccess]::Read,([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete))
}
function Read-R313ProbeText([string]$Path) {
 if(-not[IO.File]::Exists($Path)){return ''}
 $stream=$null;$reader=$null
 try{$stream=Open-R313ProbeFile $Path;$reader=[IO.StreamReader]::new($stream);return [string]$reader.ReadToEnd()}
 catch [IO.FileNotFoundException]{return ''}
 finally{if($reader){$reader.Dispose()}elseif($stream){$stream.Dispose()}}
}
function Test-R313Acknowledged([string]$Path) {
 $stdout=Read-R313ProbeText $Path
 return (($stdout -split '\r?\n') -contains 'DEEP_LEGENDS_UPDATE_STARTED')
}
function Test-R313PageReady([string]$LogPath,[int]$ChildID) {
 $log=Read-R313ProbeText $LogPath
 return ($log -match ('(?m)^\[pid='+$ChildID+'\].*installer page ready; WebView visible'))
}
function Wait-R313ParentReady([string]$Ready,$Parent) {
 $deadline=[DateTime]::UtcNow.AddSeconds(30)
 while(-not(Test-Path $Ready)) {
  if($Parent.HasExited -or [DateTime]::UtcNow -ge $deadline){throw 'Mock parent did not become ready'}
  Start-Sleep -Milliseconds 10
 }
}
function Stop-R313WebViewProfile([string]$Profile) {
 $deadline=[DateTime]::UtcNow.AddSeconds(10)
 do {
  $owned=@(Get-CimInstance Win32_Process|Where-Object{$_.Name -eq 'msedgewebview2.exe' -and $_.CommandLine -and $_.CommandLine.IndexOf($Profile,[StringComparison]::OrdinalIgnoreCase) -ge 0})
  if($owned.Count -eq 0){break}
  foreach($process in $owned){Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue}
  if([DateTime]::UtcNow -ge $deadline){throw 'Owned WebView profile process cleanup did not finish'}
  Start-Sleep -Milliseconds 50
 }while($true)
 do {
  if(-not(Test-Path $Profile)){return}
  try{Remove-Item -Recurse -Force $Profile;return}catch{$code=$_.Exception.HResult -band 0xffff;if($code -notin @(32,33) -or [DateTime]::UtcNow -ge $deadline){throw};Start-Sleep -Milliseconds 50}
 }while($true)
}
