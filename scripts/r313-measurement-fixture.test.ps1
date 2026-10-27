$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'r313-measurement-fixture.ps1')
$file=Join-Path ([IO.Path]::GetTempPath()) ('r313-ack-'+[guid]::NewGuid()+'.txt')
try {
 foreach($value in @('',"other output`n",'DEEP_LEGENDS_UPDATE_STARTE')) {
  [IO.File]::WriteAllText($file,$value)
  if(Test-R313Acknowledged $file){throw 'Empty, partial or unrelated stdout acknowledged the parent'}
 }
 [IO.File]::WriteAllText($file,"DEEP_LEGENDS_UPDATE_STARTED`r`n")
 if(-not(Test-R313Acknowledged $file)){throw 'Compatible ACK was not accepted'}
 $replacement=$file+'.next';[IO.File]::WriteAllText($replacement,'new timing snapshot')
 if((Read-R313ProbeText $file) -ne "DEEP_LEGENDS_UPDATE_STARTED`r`n"){throw 'Probe changed the snapshot bytes'}
 [IO.File]::Move($replacement,$file,$true);if((Read-R313ProbeText $file) -ne 'new timing snapshot'){throw 'A completed snapshot read leaked a file handle'}
 [IO.File]::WriteAllText($file,'[pid=123] installer page ready; WebView visible')
 if(Test-R313PageReady $file 456){throw 'Another installer acknowledged page readiness'}
 if(-not(Test-R313PageReady $file 123)){throw 'Current installer page readiness was not recognized'}
 Write-Output 'R313 empty stdout and exact protocol regression PASS'
}finally{Remove-Item $file -Force -ErrorAction SilentlyContinue}
