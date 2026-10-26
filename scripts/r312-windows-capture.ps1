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
 [DllImport("user32.dll")] static extern bool GetCursorPos(out Point p);
 [DllImport("user32.dll")] static extern short GetAsyncKeyState(int key);
 [DllImport("user32.dll")] static extern IntPtr WindowFromPoint(Point p);
 [DllImport("user32.dll")] static extern IntPtr GetAncestor(IntPtr h,uint flags);
 [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h,out Rect r);
 [StructLayout(LayoutKind.Sequential)] struct MouseInput {public int dx,dy;public uint mouseData,flags,time;public UIntPtr extra;}
 [StructLayout(LayoutKind.Sequential)] struct Input {public uint type;public MouseInput mouse;}
 [DllImport("user32.dll",SetLastError=true)] static extern uint SendInput(uint count,Input[] inputs,int size);
 [DllImport("user32.dll")] static extern IntPtr SendMessage(IntPtr h,uint msg,IntPtr w,IntPtr l);
 static void Mouse(int x,int y,uint flags,int sw,int sh){var input=new Input{type=0,mouse=new MouseInput{dx=(int)Math.Round(x*65535.0/(sw-1)),dy=(int)Math.Round(y*65535.0/(sh-1)),flags=flags}};if(SendInput(1,new[]{input},Marshal.SizeOf(typeof(Input)))!=1)throw new Exception("SendInput failed "+Marshal.GetLastWin32Error());}
 [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] static extern bool SetProcessDPIAware();
 [DllImport("user32.dll",EntryPoint="SystemParametersInfoW")] static extern bool SetSystemParameter(uint action,uint parameter,IntPtr value,uint flags);
 [DllImport("user32.dll",EntryPoint="SystemParametersInfoW")] static extern bool GetSystemParameter(uint action,uint parameter,ref int value,uint flags);
 [DllImport("winmm.dll")] static extern uint timeBeginPeriod(uint period);
 [DllImport("winmm.dll")] static extern uint timeEndPeriod(uint period);
 public static string Run(long handle,string output){
  int previousDrag=0;GetSystemParameter(0x26,0,ref previousDrag,0);if(!SetSystemParameter(0x25,1,IntPtr.Zero,2))throw new Exception("Enable full-window dragging failed");
  timeBeginPeriod(1);SetProcessDPIAware();var h=new IntPtr(handle);Directory.CreateDirectory(output);SetForegroundWindow(h);
  var screen=System.Windows.Forms.Screen.PrimaryScreen.Bounds;if(screen.Width<1560||screen.Height<900)throw new Exception("Desktop too small: "+screen);
  SetWindowPos(h,new IntPtr(-1),20,20,800,850,0x0040);Thread.Sleep(1000);
  using(var full=new Bitmap(800,850)){using(var g=Graphics.FromImage(full))g.CopyFromScreen(20,20,0,0,full.Size,CopyPixelOperation.SourceCopy);full.Save(Path.Combine(output,"desktop-before.png"),ImageFormat.Png);}
  File.WriteAllText(Path.Combine(output,"ready.txt"),"ready");var readyClock=Stopwatch.StartNew();while(!File.Exists(Path.Combine(output,"go.txt"))){if(readyClock.Elapsed.TotalSeconds>20)throw new Exception("Capture start handshake timed out");Thread.Sleep(5);}
  Rect windowRect;GetWindowRect(h,out windowRect);int mouseX=windowRect.right-8,mouseY=(windowRect.top+windowRect.bottom)/2;
  bool hit=false;for(int offset=8;offset<=16;offset++){mouseX=windowRect.right-offset;var packed=new IntPtr((mouseY<<16)|(mouseX&65535));if(SendMessage(h,0x84,IntPtr.Zero,packed).ToInt64()==11){hit=true;break;}}
  if(!hit)throw new Exception("Right resize border hit test must be HTRIGHT");
  Mouse(mouseX,mouseY,0x8001,screen.Width,screen.Height);Thread.Sleep(150);Mouse(mouseX,mouseY,0x8002,screen.Width,screen.Height);Thread.Sleep(100);
  var frames=new List<Bitmap>();var rows=new List<string>();var clock=Stopwatch.StartNew();long total=0,bright=0;int previousRight=0;Point cursor;GetCursorPos(out cursor);File.WriteAllText(Path.Combine(output,"input-start.txt"),"target="+mouseX+","+mouseY+" cursor="+cursor.x+","+cursor.y+" down="+GetAsyncKeyState(1)+" hwnd="+h+" screen="+screen);
  Rect rect;Point origin=new Point();GetClientRect(h,out rect);ClientToScreen(h,ref origin);previousRight=origin.x+rect.right;
  int inputStep=0;long inputStartMs=DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),inputEndMs=0;Exception inputFailure=null;
  var inputThread=new Thread(()=>{try{for(int i=0;i<120;i++){double target=i*(1000.0/60);while(clock.Elapsed.TotalMilliseconds<target)Thread.Sleep(0);mouseX+=i<60?12:-12;Mouse(mouseX,mouseY,0xA001,screen.Width,screen.Height);Volatile.Write(ref inputStep,i+1);}}catch(Exception e){inputFailure=e;}finally{Mouse(mouseX,mouseY,0x8004,screen.Width,screen.Height);inputEndMs=DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();}});
  inputThread.Start();
  double sampledSeconds=0;try{while(inputThread.IsAlive){
   GetClientRect(h,out rect);origin=new Point();ClientToScreen(h,ref origin);int right=origin.x+rect.right;
   int added=Math.Max(0,right-previousRight),stripWidth=Math.Min(rect.right,added>0?added:12);
   var bitmap=new Bitmap(stripWidth,Math.Max(1,rect.bottom-70),PixelFormat.Format32bppArgb);
   using(var g=Graphics.FromImage(bitmap))g.CopyFromScreen(right-stripWidth,origin.y+70,0,0,bitmap.Size,CopyPixelOperation.SourceCopy);
   long white=0,pixels=0;
   if(added>0){var locked=bitmap.LockBits(new Rectangle(0,0,bitmap.Width,bitmap.Height),ImageLockMode.ReadOnly,PixelFormat.Format32bppArgb);try{var bytes=new byte[Math.Abs(locked.Stride)*bitmap.Height];Marshal.Copy(locked.Scan0,bytes,0,bytes.Length);for(int y=0;y<bitmap.Height;y++)for(int x=0;x<bitmap.Width;x++){int k=y*Math.Abs(locked.Stride)+x*4;if(.2126*bytes[k+2]+.7152*bytes[k+1]+.0722*bytes[k]>200)white++;pixels++;}}finally{bitmap.UnlockBits(locked);}total+=pixels;bright+=white;}
   GetCursorPos(out cursor);rows.Add("{\"actualRight\":"+right+",\"cursorX\":"+cursor.x+",\"leftButton\":"+GetAsyncKeyState(1)+",\"frame\":"+frames.Count+",\"inputStep\":"+Volatile.Read(ref inputStep)+",\"atMs\":"+clock.Elapsed.TotalMilliseconds.ToString(System.Globalization.CultureInfo.InvariantCulture)+",\"pixels\":"+pixels+",\"bright\":"+white+"}");frames.Add(bitmap);previousRight=right;
  }sampledSeconds=clock.Elapsed.TotalSeconds;}finally{inputThread.Join();Mouse(mouseX,mouseY,0x8004,screen.Width,screen.Height);timeEndPeriod(1);SetSystemParameter(0x25,(uint)previousDrag,IntPtr.Zero,2);}
  double elapsed=sampledSeconds;double fps=frames.Count/elapsed;
  using(var full=new Bitmap(800,850)){using(var g=Graphics.FromImage(full))g.CopyFromScreen(20,20,0,0,full.Size,CopyPixelOperation.SourceCopy);full.Save(Path.Combine(output,"desktop-after.png"),ImageFormat.Png);}
  for(int i=0;i<frames.Count;i++){frames[i].Save(Path.Combine(output,"frame-"+i.ToString("D3")+".png"),ImageFormat.Png);frames[i].Dispose();}
  File.WriteAllText(Path.Combine(output,"frames.json"),"["+string.Join(",",rows)+"]");if(inputFailure!=null)throw inputFailure;
  return "{\"inputStartMs\":"+inputStartMs+",\"inputEndMs\":"+inputEndMs+",\"input\":\"SendInput-held-right-border\",\"inputFrames\":120,\"frames\":"+frames.Count+",\"expandFrames\":60,\"shrinkFrames\":60,\"stepPx\":12,\"fps\":"+fps.ToString(System.Globalization.CultureInfo.InvariantCulture)+",\"pixels\":"+total+",\"bright\":"+bright+",\"whiteRatio\":"+(total==0?0:(double)bright/total).ToString(System.Globalization.CultureInfo.InvariantCulture)+",\"desktopWidth\":"+screen.Width+",\"desktopHeight\":"+screen.Height+"}";
 }
}
'@
[R312Capture]::Run($Window,$Output) | Set-Content -Encoding utf8 (Join-Path $Output 'capture.json')
