param([Parameter(Mandatory=$true)][string]$Output)
$ErrorActionPreference='Stop'
New-Item -ItemType Directory -Force (Split-Path $Output)|Out-Null
Add-Type @'
using System;using System.Runtime.InteropServices;
public static class R314Display {
 [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)]public struct Mode {
  [MarshalAs(UnmanagedType.ByValTStr,SizeConst=32)]public string device;
  public short spec,driver,size,extra;public int fields,x,y,orientation,fixedOutput;
  public short color,duplex,yResolution,ttOption,collate;
  [MarshalAs(UnmanagedType.ByValTStr,SizeConst=32)]public string form;
  public short logPixels;public int bits,width,height,flags,frequency,icmMethod,icmIntent,media,dither,reserved1,reserved2,panningWidth,panningHeight;
 }
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern bool EnumDisplaySettings(string device,int number,ref Mode mode);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern int ChangeDisplaySettingsEx(string device,ref Mode mode,IntPtr hwnd,uint flags,IntPtr parameters);
 [DllImport("user32.dll")]static extern int GetSystemMetrics(int index);
 [DllImport("user32.dll")]static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 static Mode Current(){var m=new Mode();m.size=(short)Marshal.SizeOf(typeof(Mode));if(!EnumDisplaySettings(null,-1,ref m))throw new Exception("Current display mode unavailable");return m;}
 public static object Resize(){var prior=SetThreadDpiAwarenessContext(new IntPtr(-4));try{
  var before=Current();var target=before;target.width=1920;target.height=1080;target.fields=0x00080000|0x00100000;
  int test=ChangeDisplaySettingsEx(null,ref target,IntPtr.Zero,2,IntPtr.Zero),apply=test;
  if(test==0)apply=ChangeDisplaySettingsEx(null,ref target,IntPtr.Zero,0,IntPtr.Zero);
  var after=Current();return new{before=new{before.width,before.height},after=new{after.width,after.height},test,apply,screenWidth=GetSystemMetrics(0),screenHeight=GetSystemMetrics(1),dpiChanged=false,modeBytes=Marshal.SizeOf(typeof(Mode))};
 }finally{SetThreadDpiAwarenessContext(prior);}}
}
'@
$result=[R314Display]::Resize();$result|ConvertTo-Json -Depth 5|Set-Content -Encoding utf8 $Output
$result|ConvertTo-Json -Depth 5
if($result.apply -ne 0 -or $result.screenWidth -lt 1920 -or $result.screenHeight -lt 1080){throw 'Runner display cannot contain the required real windows and 24px physical drag distance'}
