param([long]$Window, [string]$Output)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms
Add-Type -ReferencedAssemblies System.Drawing,System.Windows.Forms -TypeDefinition @'
using System;using System.IO;using System.Drawing;using System.Drawing.Imaging;using System.Diagnostics;using System.Threading;using System.Runtime.InteropServices;using System.Collections.Generic;
public class R312Capture {
 [StructLayout(LayoutKind.Sequential)] public struct Rect {public int left,top,right,bottom;}
 [StructLayout(LayoutKind.Sequential)] public struct Point {public int x,y;}
 [DllImport("user32.dll")] static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
 [DllImport("user32.dll")] static extern bool GetClientRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")] static extern bool ClientToScreen(IntPtr h,ref Point p);
 [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h,out Rect r);
 [StructLayout(LayoutKind.Sequential)] struct MouseInput {public int dx,dy;public uint mouseData,flags,time;public UIntPtr extra;}
 [StructLayout(LayoutKind.Sequential)] struct Input {public uint type;public MouseInput mouse;}
 [DllImport("user32.dll",SetLastError=true)] static extern uint SendInput(uint count,Input[] inputs,int size);
 [DllImport("user32.dll")] static extern IntPtr SendMessage(IntPtr h,uint msg,IntPtr w,IntPtr l);
 static void Mouse(int x,int y,uint flags,int sw,int sh){var input=new Input{type=0,mouse=new MouseInput{dx=(int)Math.Round(x*65535.0/(sw-1)),dy=(int)Math.Round(y*65535.0/(sh-1)),flags=flags}};if(SendInput(1,new[]{input},Marshal.SizeOf(typeof(Input)))!=1)throw new Exception("SendInput failed "+Marshal.GetLastWin32Error());}
 [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] static extern bool SetProcessDPIAware();
 public static string Run(long handle,string output){
  SetProcessDPIAware();var h=new IntPtr(handle);Directory.CreateDirectory(output);SetForegroundWindow(h);
  var screen=System.Windows.Forms.Screen.PrimaryScreen.Bounds;if(screen.Width<1560||screen.Height<900)throw new Exception("Desktop too small: "+screen);
  SetWindowPos(h,new IntPtr(-1),20,20,800,850,0x0040);Thread.Sleep(1000);
  Rect windowRect;GetWindowRect(h,out windowRect);int mouseX=windowRect.right-2,mouseY=(windowRect.top+windowRect.bottom)/2;
  bool hit=false;for(int offset=1;offset<=12;offset++){mouseX=windowRect.right-offset;var packed=new IntPtr((mouseY<<16)|(mouseX&65535));if(SendMessage(h,0x84,IntPtr.Zero,packed).ToInt64()==11){hit=true;break;}}
  if(!hit)throw new Exception("Right resize border hit test must be HTRIGHT");
  Mouse(mouseX,mouseY,0x8001,screen.Width,screen.Height);Thread.Sleep(150);Mouse(mouseX,mouseY,0x8002,screen.Width,screen.Height);Thread.Sleep(100);
  var frames=new List<Bitmap>();var rows=new List<string>();var clock=Stopwatch.StartNew();long total=0,bright=0;int previousRight=0;
  Rect rect;Point origin=new Point();GetClientRect(h,out rect);ClientToScreen(h,ref origin);previousRight=origin.x+rect.right;
  try { for(int i=0;i<120;i++){
   int width=i<60?800+(i+1)*12:1520-(i-59)*12;
   double target=i*16.0;while(clock.Elapsed.TotalMilliseconds<target)Thread.Sleep(0);
   mouseX+=i<60?12:-12;Mouse(mouseX,mouseY,0xA001,screen.Width,screen.Height);Thread.Sleep(3);
   GetClientRect(h,out rect);origin=new Point();ClientToScreen(h,ref origin);int right=origin.x+rect.right;
   var bitmap=new Bitmap(rect.right,rect.bottom,PixelFormat.Format32bppArgb);
   using(var g=Graphics.FromImage(bitmap))g.CopyFromScreen(origin.x,origin.y,0,0,bitmap.Size,CopyPixelOperation.SourceCopy);
   long white=0,pixels=0;
   if(right>previousRight){int left=Math.Max(0,previousRight-origin.x);for(int y=70;y<bitmap.Height;y++)for(int x=left;x<bitmap.Width;x++){var c=bitmap.GetPixel(x,y);if(.2126*c.R+.7152*c.G+.0722*c.B>200)white++;pixels++;}total+=pixels;bright+=white;}
   rows.Add("{\"frame\":"+i+",\"atMs\":"+clock.Elapsed.TotalMilliseconds.ToString(System.Globalization.CultureInfo.InvariantCulture)+",\"width\":"+width+",\"pixels\":"+pixels+",\"bright\":"+white+"}");frames.Add(bitmap);previousRight=right;
  }
  } finally {Mouse(mouseX,mouseY,0x8004,screen.Width,screen.Height);}
  double elapsed=clock.Elapsed.TotalSeconds;double fps=120/elapsed;
  for(int i=0;i<frames.Count;i++){frames[i].Save(Path.Combine(output,"frame-"+i.ToString("D3")+".png"),ImageFormat.Png);frames[i].Dispose();}
  File.WriteAllText(Path.Combine(output,"frames.json"),"["+string.Join(",",rows)+"]");
  if(fps<30)throw new Exception("Capture below required 30fps: "+fps);
  return "{\"input\":\"SendInput-held-right-border\",\"frames\":120,\"expandFrames\":60,\"shrinkFrames\":60,\"stepPx\":12,\"fps\":"+fps.ToString(System.Globalization.CultureInfo.InvariantCulture)+",\"pixels\":"+total+",\"bright\":"+bright+",\"whiteRatio\":"+((double)bright/total).ToString(System.Globalization.CultureInfo.InvariantCulture)+",\"desktopWidth\":"+screen.Width+",\"desktopHeight\":"+screen.Height+"}";
 }
}
'@
[R312Capture]::Run($Window,$Output) | Set-Content -Encoding utf8 (Join-Path $Output 'capture.json')
