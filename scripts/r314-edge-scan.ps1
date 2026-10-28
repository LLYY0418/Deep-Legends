param([long]$Window,[int]$ProcessID,[int]$Width,[int]$Height,[string]$Output,[string]$Label,[switch]$Explorer,[switch]$VerifyDrag)
$ErrorActionPreference='Stop'
New-Item -ItemType Directory -Force (Split-Path $Output)|Out-Null
Add-Type @'
using System;using System.Collections.Generic;using System.Runtime.InteropServices;
public static class R314HitTest {
 public delegate bool EnumProc(IntPtr h,IntPtr d);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern IntPtr FindWindow(string cls,string title);
 public static long FindExplorer(int width,int height){var h=FindWindow("CabinetWClass",null);if(h==IntPtr.Zero)return 0;SetWindowPos(h,IntPtr.Zero,40,40,width,height,0x0040);return h.ToInt64();}
 [DllImport("user32.dll")]static extern bool EnumWindows(EnumProc cb,IntPtr d);
 [DllImport("user32.dll")]static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll")]static extern uint GetWindowThreadProcessId(IntPtr h,out uint p);
 [DllImport("user32.dll")]static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")]static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
 [DllImport("user32.dll")]static extern bool SetForegroundWindow(IntPtr h);
 public static object[] Drags(long handle){var h=new IntPtr(handle);Rect original;GetWindowRect(h,out original);var results=new List<object>();foreach(var edge in new[]{"left","right","bottom","bottomleft","bottomright"})foreach(int offset in new[]{-6,2}){
  SetWindowPos(h,new IntPtr(-1),original.left,original.top,original.right-original.left,original.bottom-original.top,0x0040);SetForegroundWindow(h);System.Threading.Thread.Sleep(80);
  Rect r;DwmGetWindowAttribute(h,9,out r,Marshal.SizeOf(typeof(Rect)));int x=(r.left+r.right)/2,y=(r.top+r.bottom)/2,dx=0,dy=0;
  if(edge=="left"||edge=="bottomleft"){x=r.left+offset;dx=-24;}if(edge=="right"||edge=="bottomright"){x=r.right-1-offset;dx=24;}if(edge.StartsWith("bottom")){y=r.bottom-1-offset;dy=24;}
  Rect before;GetWindowRect(h,out before);SetCursorPos(x,y);System.Threading.Thread.Sleep(30);mouse_event(2,0,0,0,UIntPtr.Zero);System.Threading.Thread.Sleep(60);SetCursorPos(x+dx,y+dy);System.Threading.Thread.Sleep(120);mouse_event(4,0,0,0,UIntPtr.Zero);System.Threading.Thread.Sleep(60);Rect after;GetWindowRect(h,out after);
  bool changed=(dx==0||Math.Abs((after.right-after.left)-(before.right-before.left))>=12)&&(dy==0||Math.Abs((after.bottom-after.top)-(before.bottom-before.top))>=12);results.Add(new{edge,offset,x,y,changed,before,after});
 }SetWindowPos(h,IntPtr.Zero,original.left,original.top,original.right-original.left,original.bottom-original.top,0x0040);return results.ToArray();}
 [DllImport("user32.dll")]static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
 [DllImport("user32.dll",EntryPoint="GetWindowLongPtrW")]static extern IntPtr GetWindowLongPtr(IntPtr h,int index);
 public static long Find(int process,int width,int height){long found=0,largest=0;EnumWindows((h,d)=>{uint p;GetWindowThreadProcessId(h,out p);Rect r;GetWindowRect(h,out r);long area=(long)(r.right-r.left)*(r.bottom-r.top);if(p==process&&IsWindowVisible(h)&&(GetWindowLongPtr(h,-16).ToInt64()&0x00040000)!=0&&area>largest){largest=area;found=h.ToInt64();}return true;},IntPtr.Zero);if(found==0)throw new Exception("Visible resizable main window not found for exact process "+process);if(!SetWindowPos(new IntPtr(found),IntPtr.Zero,40,40,width,height,0x0040))throw new Exception("SetWindowPos failed");Rect actual;GetWindowRect(new IntPtr(found),out actual);if(actual.right-actual.left!=width||actual.bottom-actual.top!=height)throw new Exception("Requested main window size not applied");return found;}

 [StructLayout(LayoutKind.Sequential)]public struct Rect{public int left,top,right,bottom;}
 [DllImport("user32.dll")]static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("user32.dll")]public static extern bool GetWindowRect(IntPtr h,out Rect r);
 [DllImport("dwmapi.dll")]static extern int DwmGetWindowAttribute(IntPtr h,int attribute,out Rect r,int size);
 [DllImport("user32.dll")]static extern uint GetDpiForWindow(IntPtr h);
 [DllImport("user32.dll")]static extern IntPtr SendMessage(IntPtr h,uint msg,IntPtr w,IntPtr l);
 public static Dictionary<string,object> Scan(long handle){
  var prior=SetThreadDpiAwarenessContext(new IntPtr(-4));try{
   var h=new IntPtr(handle);Rect outer,visible;GetWindowRect(h,out outer);int hr=DwmGetWindowAttribute(h,9,out visible,Marshal.SizeOf(typeof(Rect)));if(hr!=0)throw new Exception("DWM frame bounds "+hr);
   var rows=new List<object>();
   foreach(var frame in new[]{"visible","outer"}){var r=frame=="visible"?visible:outer;
    foreach(var edge in new[]{"left","right","bottom","bottomleft","bottomright"})foreach(var fraction in new[]{.2,.5,.8})for(int offset=-10;offset<=10;offset++){
     int x,y;
     if(edge=="left"||edge=="right"){x=edge=="left"?r.left+offset:r.right-1-offset;y=r.top+(int)((r.bottom-r.top)*fraction);}
     else if(edge=="bottom"){x=r.left+(int)((r.right-r.left)*fraction);y=r.bottom-1-offset;}
     else{x=edge=="bottomleft"?r.left+offset:r.right-1-offset;y=r.bottom-1-offset;}
     long code=SendMessage(h,0x84,IntPtr.Zero,new IntPtr((y<<16)|(x&65535))).ToInt64();
     rows.Add(new{frame,edge,fraction,offset,x,y,code});
    }
   }
   return new Dictionary<string,object>{{"dpi",GetDpiForWindow(h)},{"outer",outer},{"visible",visible},{"rows",rows}};
  }finally{SetThreadDpiAwarenessContext(prior);}
 }
}
'@
if($Explorer){Start-Process explorer.exe -ArgumentList 'C:\Windows';$deadline=[DateTime]::UtcNow.AddSeconds(20);do{$Window=[R314HitTest]::FindExplorer($Width,$Height);if($Window){break};Start-Sleep -Milliseconds 100}while([DateTime]::UtcNow -lt $deadline);if(-not $Window){throw 'Explorer window not created'}}
if($ProcessID){$Window=[R314HitTest]::Find($ProcessID,$Width,$Height);Start-Sleep -Milliseconds 300}
$result=[R314HitTest]::Scan($Window);$result['label']=$Label
if($VerifyDrag){$result['drags']=[R314HitTest]::Drags($Window)}
$result|ConvertTo-Json -Depth 10|Set-Content -Encoding utf8 $Output

if($VerifyDrag){
 $bad=@($result.rows|Where-Object{$_.frame -eq 'visible' -and $_.offset -ge -6 -and $_.offset -le 2 -and $_.code -notin @(10,11,12,13,14,15,16,17)})
 if($bad.Count -gt 0){throw 'Native resize zone does not cover every required pixel'}
 if(@($result.drags|Where-Object{-not $_.changed}).Count -gt 0){throw 'Real pointer drag failed at a required border pixel'}
}
