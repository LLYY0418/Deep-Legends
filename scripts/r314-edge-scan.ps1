param([long]$Window,[int]$ProcessID,[int]$Width,[int]$Height,[string]$Output,[string]$Label)
$ErrorActionPreference='Stop'
New-Item -ItemType Directory -Force (Split-Path $Output)|Out-Null
Add-Type @'
using System;using System.Collections.Generic;using System.Runtime.InteropServices;
public static class R314HitTest {
 public delegate bool EnumProc(IntPtr h,IntPtr d);
 [DllImport("user32.dll")]static extern bool EnumWindows(EnumProc cb,IntPtr d);
 [DllImport("user32.dll")]static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll")]static extern uint GetWindowThreadProcessId(IntPtr h,out uint p);
 [DllImport("user32.dll")]static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
 public static long Find(int process,int width,int height){long found=0;EnumWindows((h,d)=>{uint p;GetWindowThreadProcessId(h,out p);if(p==process&&IsWindowVisible(h))found=h.ToInt64();return true;},IntPtr.Zero);if(found==0)throw new Exception("Visible window not found for exact process "+process);SetWindowPos(new IntPtr(found),IntPtr.Zero,40,40,width,height,0x0040);return found;}

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
if($ProcessID){$Window=[R314HitTest]::Find($ProcessID,$Width,$Height);Start-Sleep -Milliseconds 300}
$result=[R314HitTest]::Scan($Window);$result['label']=$Label
$result|ConvertTo-Json -Depth 10|Set-Content -Encoding utf8 $Output
