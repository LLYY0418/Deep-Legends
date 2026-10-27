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
 Write-Output 'R313 empty stdout and exact protocol regression PASS'
}finally{Remove-Item $file -Force -ErrorAction SilentlyContinue}
