//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--internal-probe-child" {
		if err := runActor(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	if len(os.Args) != 1 {
		return
	} // No user-supplied target HWND/PID modes.
	if err := runController(); err != nil {
		user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(wide(err.Error()))), uintptr(unsafe.Pointer(wide("窗口测试启动失败"))), 0x10)
	}
}

type uiNotice struct{ kind, text, folder string }

func runController() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var h, start, cancelButton, folderButton, output uintptr
	var running, closing bool
	var cancel context.CancelFunc
	var reportFolder string
	notices := make(chan uiNotice, 32)
	post := func(n uiNotice) {
		select {
		case notices <- n:
			postMessage.Call(h, wmApp, 0, 0)
		default:
		}
	}
	stop := func() {
		if cancel != nil {
			cancel()
		}
		textWindow(output, "正在停止并回收模拟窗口，请稍候……\r\n不会修改其他程序窗口。")
		enableWindow.Call(cancelButton, 0)
	}
	cb := syscall.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
		switch msg {
		case wmCommand:
			switch wp & 0xffff {
			case 1:
				if running {
					return 0
				}
				running = true
				ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
				cancel = c
				enableWindow.Call(start, 0)
				enableWindow.Call(cancelButton, 1)
				textWindow(output, "正在创建独立模拟窗口……\r\n请暂时不要操作其他窗口。")
				go func() {
					defer c()
					runExperiment(ctx, h, func(text string) { post(uiNotice{kind: "status", text: text}) },
						func(text, folder string) { post(uiNotice{kind: "done", text: text, folder: folder}) })
				}()
			case 2:
				if running {
					stop()
				}
			case 3:
				if reportFolder != "" {
					r, _, _ := systemDLL("shell32.dll").NewProc("ShellExecuteW").Call(h, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(reportFolder))), 0, 0, 1)
					if r <= 32 {
						textWindow(output, "自动打开目录失败，请按显示的路径手动打开：\r\n"+reportFolder)
					}
				}
			}
			return 0
		case wmApp:
			for {
				select {
				case n := <-notices:
					if n.kind == "status" {
						if !closing {
							textWindow(output, n.text)
						}
						continue
					}
					running, reportFolder = false, n.folder
					enableWindow.Call(cancelButton, 0)
					if reportFolder != "" {
						enableWindow.Call(folderButton, 1)
					}
					textWindow(output, n.text+"\r\n\r\n报告目录：\r\n"+reportFolder+"\r\n\r\n请发送目录里的 WindowProbe-Report.zip。\r\n如实际看到闪烁，也请一并说明。")
					if closing {
						destroyWindow.Call(hwnd)
					}
				default:
					return 0
				}
			}
		case wmClose:
			if running {
				closing = true
				stop()
			} else {
				destroyWindow.Call(hwnd)
			}
			return 0
		case wmDestroy:
			if cancel != nil {
				cancel()
			}
			postQuit.Call(0)
			return 0
		}
		r, _, _ := defaultProc.Call(hwnd, msg, wp, lp)
		return r
	})
	var err error
	h, err = createTopWindow(fmt.Sprintf("DLProbeController-%d", os.Getpid()), "Deep Legends · 独立窗口防弹出测试 v"+version, cb, 760, 570, 0)
	if err != nil {
		return err
	}
	control(h, "STATIC", "只测试模拟窗口，不启动或连接 LOL", 0, 24, 20, 690, 26, 0)
	control(h, "STATIC", "点击开始后约 10 秒。基线与恢复阶段会故意弹出一个测试窗口。\r\n运行中请暂时不要操作鼠标键盘；需要中止时按 Esc、关闭窗口或点击停止。\r\n不改客户端文件/配置、不注入、不联网、不记录其他程序标题或画面。", 0, 24, 54, 690, 78, 0)
	start = control(h, "BUTTON", "开始测试", 0x10000, 24, 148, 145, 34, 1)
	cancelButton = control(h, "BUTTON", "停止并恢复", 0x10000, 184, 148, 145, 34, 2)
	folderButton = control(h, "BUTTON", "打开报告文件夹", 0x10000, 344, 148, 175, 34, 3)
	enableWindow.Call(cancelButton, 0)
	enableWindow.Call(folderButton, 0)
	output = control(h, "EDIT", "准备就绪。\r\n\r\n请点击“开始测试”。\r\n本工具不会修改正式项目的自动接受功能。", 0x0800|0x0004|0x0040|0x00200000|0x00800000, 24, 202, 690, 270, 4) // Read-only multiline, autoscroll, vertical scroll, border.
	control(h, "STATIC", "测试成功仅说明模拟结果，不等于真实客户端已经修复。", 0, 24, 484, 690, 24, 0)
	if start == 0 || cancelButton == 0 || folderButton == 0 || output == 0 {
		destroyWindow.Call(h)
		return fmt.Errorf("无法创建测试控件")
	}
	showWindow.Call(h, 5)
	return messageLoop()
}
