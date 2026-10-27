function Test-R313Acknowledged([string]$Path) {
 $stdout=[string](Get-Content $Path -Raw -ErrorAction SilentlyContinue)
 return (($stdout -split '\r?\n') -contains 'DEEP_LEGENDS_UPDATE_STARTED')
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
