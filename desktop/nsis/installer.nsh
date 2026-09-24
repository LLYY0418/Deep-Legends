; Deep Legends / white native setup. Keep electron-builder's install/upgrade/uninstall
; sections, but expose only a destination and a launch choice to new users.
!include nsDialogs.nsh
!include LogicLib.nsh
!include FileFunc.nsh
!include WinMessages.nsh

; System-DPI-aware native controls: avoid Windows bitmap virtualization.
ManifestDPIAware true

!ifndef BUILD_UNINSTALLER
!define MUI_CUSTOMFUNCTION_GUIINIT DLGuiInit

; Installer-only palette; keep the uninstaller on stock Windows styling.
!define MUI_BGCOLOR "FFFFFF"
!define MUI_TEXTCOLOR "20242B"
!define MUI_INSTFILESPAGE_COLORS "20242B F5F6F8"
!define MUI_INSTFILESPAGE_PROGRESSBAR "smooth"
!endif

!define MUI_ABORTWARNING

; Keep registry/shortcut targets unchanged. The stock NSIS uninstaller remains
; the only implementation of file, registry and shortcut removal.
!define DL_UNINSTALL_SHELL "${PROJECT_DIR}\uninstall-shell.exe"
!macro customInstall
  ; Internal payload, not a second user-facing uninstaller. The shell copies
  ; this PE to its temporary workspace before launching the stock NSIS logic.
  CreateDirectory "$INSTDIR\resources"
  Delete "$INSTDIR\resources\uninstall-core.dat"
  ClearErrors
  Rename "$INSTDIR\${UNINSTALL_FILENAME}" "$INSTDIR\resources\uninstall-core.dat"
  IfErrors 0 +2
    Abort "Unable to prepare the uninstaller payload"
  File "/oname=$INSTDIR\${UNINSTALL_FILENAME}" "${DL_UNINSTALL_SHELL}"
  Delete "$INSTDIR\Uninstall Deep Legends Core.exe"
!macroend

!macro customInstallMode
  !ifndef BUILD_UNINSTALLER
    ; Honor updater mode flags, then reuse an existing installation. If both
    ; scopes exist, prefer the current user's copy instead of prompting.
    ${If} ${isForAllUsers}
      StrCpy $isForceMachineInstall "1"
    ${ElseIf} ${isForCurrentUser}
      StrCpy $isForceCurrentInstall "1"
    ${ElseIf} $hasPerMachineInstallation == "1"
    ${AndIf} $hasPerUserInstallation != "1"
      StrCpy $isForceMachineInstall "1"
    ${Else}
      StrCpy $isForceCurrentInstall "1"
    ${EndIf}
  !endif
!macroend

!ifndef BUILD_UNINSTALLER
Var DLDialog
Var DLDirectory
Var DLLaunch
Var DLFont
Var DLBodyFont
Var DLIcon
Var DLIconHandle
Var DLStepFont
Var DLButtonFont
Var DLStep
Var DLDpi
Var DLFooter
Var DLFooterText
Var DLSpace
Var DLProgressIcon

!macro customPageAfterChangeDir
  Page custom DLDirectoryPage DLDirectoryLeave
  !define MUI_PAGE_CUSTOMFUNCTION_SHOW DLInstFilesShow
  !define MUI_PAGE_CUSTOMFUNCTION_LEAVE DLInstFilesLeave
!macroend
!macro customFinishPage
  Page custom DLFinishPage DLFinishLeave
!macroend

; The builder emits its custom include and plugin paths asynchronously.
; Defer executable functions until installer.nsi calls customHeader, after
; all plugin directories and standard helper variables have been registered.
!define DL_INSTALLER_ICON "${MUI_ICON}"
 ; Coordinates are design pixels at 96 DPI, shared by all three pages.
!macro DLPosition HWND X Y W H
  System::Call 'kernel32::MulDiv(i ${X}, i $DLDpi, i 96)i.r5'
  System::Call 'kernel32::MulDiv(i ${Y}, i $DLDpi, i 96)i.r6'
  System::Call 'kernel32::MulDiv(i ${W}, i $DLDpi, i 96)i.r7'
  System::Call 'kernel32::MulDiv(i ${H}, i $DLDpi, i 96)i.r8'
  System::Call 'user32::SetWindowPos(p ${HWND}, p 0, i r5, i r6, i r7, i r8, i 0x14)'
!macroend

; Native static controls also work on MUI's existing instfiles dialog. Do not
; call nsDialogs::Create there: it would replace the native install lifecycle.
!macro DLLabel PARENT X Y W H TEXT FONT INK BG
  System::Call 'user32::CreateWindowExW(i 0, w "STATIC", w "${TEXT}", i 0x50000200, i 0, i 0, i 0, i 0, p ${PARENT}, p 0, p 0, p 0)p.r0'
  SendMessage $0 ${WM_SETFONT} ${FONT} 1
  SetCtlColors $0 ${INK} ${BG}
  !insertmacro DLPosition $0 ${X} ${Y} ${W} ${H}
!macroend

; Native Win32 push buttons cannot be recoloured. DefWindowProc ignores the
; brush returned for WM_CTLCOLORBTN on BS_PUSHBUTTON whether or not visual
; styles are on, so SetCtlColors does nothing for them. Two attempts to force
; it have now been shipped and neither changed anything on real hardware:
; blanking the theme (SetWindowTheme " " " ") and asking for the undocumented
; "DarkMode_Explorer" class -- the latter also needs a process-wide dark mode
; opt-in through unexported uxtheme ordinals that shift between Windows builds.
;
; So stop fighting the platform: the buttons stay stock, and the strip they sit
; on is a light surface instead. Light system buttons on a light footer read as
; deliberate wizard chrome; the same buttons floating on near-black read as
; broken, and the disabled "取消" was grey-on-near-black, which is what
; "看不清楚" was about. The three pages use white; the footer remains a light gray surface.
!define DL_FOOTER_BG "EDEFF3"
!define DL_FOOTER_INK "5A6377"

!macro customHeader
Function DLGuiInit
  SetCtlColors $HWNDPARENT 0x20242B 0xFFFFFF
  StrCpy $DLDpi 96
  System::Call 'user32::GetDC(p $HWNDPARENT)p.r0'
  System::Call 'gdi32::GetDeviceCaps(p r0, i 90)i.r1'
  System::Call 'user32::ReleaseDC(p $HWNDPARENT, p r0)'
  ${If} $1 > 0
    StrCpy $DLDpi $1
  ${EndIf}
  ; Resize the client, retaining the original window origin and native chrome.
  System::Call 'kernel32::MulDiv(i 700, i $DLDpi, i 96)i.r1'
  System::Call 'kernel32::MulDiv(i 480, i $DLDpi, i 96)i.r2'
  System::Call '*(i 0, i 0, i r1, i r2)p.r0'
  System::Call 'user32::GetWindowLongW(p $HWNDPARENT, i -16)i.r3'
  System::Call 'user32::GetWindowLongW(p $HWNDPARENT, i -20)i.r4'
  System::Call 'user32::AdjustWindowRectEx(p r0, i r3, i 0, i r4)'
  System::Call '*$0(i.r1, i.r2, i.r3, i.r4)'
  System::Free $0
  IntOp $3 $3 - $1
  IntOp $4 $4 - $2
  System::Call 'user32::SetWindowPos(p $HWNDPARENT, p 0, i 0, i 0, i r3, i r4, i 6)'
  Call DLHideStock
  StrCpy $DLDialog $HWNDPARENT
  Call DLFonts
  !insertmacro DLLabel $HWNDPARENT 0 420 700 60 "" $DLBodyFont ${DL_FOOTER_INK} ${DL_FOOTER_BG}
  StrCpy $DLFooter $0
  System::Call 'user32::SetWindowPos(p $DLFooter, p 1, i 0, i 0, i 0, i 0, i 3)'
  ; 1px rule so the light strip reads as a seam against the white page.
  !insertmacro DLLabel $HWNDPARENT 0 420 700 1 "" $DLStepFont D8DCE4 D8DCE4
  !insertmacro DLLabel $HWNDPARENT 30 444 320 20 "" $DLStepFont ${DL_FOOTER_INK} ${DL_FOOTER_BG}
  StrCpy $DLFooterText $0
  ; Buttons keep the system look on purpose (see the note above); only the font
  ; and position are ours, so they line up with the design grid.
  GetDlgItem $0 $HWNDPARENT 1
  SendMessage $0 ${WM_SETFONT} $DLButtonFont 1
  !insertmacro DLPosition $0 578 434 92 34
  GetDlgItem $0 $HWNDPARENT 2
  SendMessage $0 ${WM_SETFONT} $DLButtonFont 1
  !insertmacro DLPosition $0 476 434 92 34
  GetDlgItem $0 $HWNDPARENT 3
  SendMessage $0 ${WM_SETFONT} $DLButtonFont 1
  ShowWindow $0 ${SW_HIDE}
FunctionEnd

Function DLHideStock
  GetDlgItem $0 $HWNDPARENT 1028
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 1256
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 1035
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 1037
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 1038
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 1039
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 1045
  ShowWindow $0 ${SW_HIDE}
  GetDlgItem $0 $HWNDPARENT 3
  ShowWindow $0 ${SW_HIDE}
FunctionEnd

Function DLFonts
  ${If} $DLFont == ""
    CreateFont $DLFont "Microsoft YaHei UI" 20 700
    CreateFont $DLBodyFont "Microsoft YaHei UI" 11 400
    CreateFont $DLStepFont "Microsoft YaHei UI" 10 400
    CreateFont $DLButtonFont "Microsoft YaHei UI" 11 600
  ${EndIf}
FunctionEnd

Function DLTheme
  Call DLFonts
  Call DLHideStock
  SetCtlColors $DLDialog 20242B FFFFFF
  !insertmacro DLPosition $DLDialog 0 0 700 420
  !insertmacro DLLabel $DLDialog 30 18 540 38 "DEEP LEGENDS" $DLFont 8A641F FFFFFF
  !insertmacro DLLabel $DLDialog 30 56 640 20 "生涯 · 对局 · 收藏" $DLStepFont 626C7A FFFFFF
  !insertmacro DLLabel $DLDialog 30 82 640 1 "" $DLStepFont D9A441 D9A441
  !insertmacro DLLabel $DLDialog 30 94 130 24 "①  选择位置" $DLStepFont 626C7A FFFFFF
  ${If} $DLStep == 1
    SetCtlColors $0 8A641F FFFFFF
  ${Else}
    ${NSD_SetText} $0 "✓  选择位置"
  ${EndIf}
  !insertmacro DLLabel $DLDialog 160 104 165 1 "" $DLStepFont E3E7ED E3E7ED
  ${If} $DLStep > 1
    SetCtlColors $0 8A6A2E 8A6A2E
  ${EndIf}
  !insertmacro DLLabel $DLDialog 337 94 75 24 "②  安装" $DLStepFont 626C7A FFFFFF
  ${If} $DLStep == 2
    SetCtlColors $0 8A641F FFFFFF
  ${ElseIf} $DLStep == 3
    ${NSD_SetText} $0 "✓  安装"
  ${EndIf}
  !insertmacro DLLabel $DLDialog 412 104 180 1 "" $DLStepFont E3E7ED E3E7ED
  ${If} $DLStep == 3
    SetCtlColors $0 8A6A2E 8A6A2E
  ${EndIf}
  !insertmacro DLLabel $DLDialog 604 94 66 24 "③  完成" $DLStepFont 626C7A FFFFFF
  ${If} $DLStep == 3
    SetCtlColors $0 8A641F FFFFFF
  ${EndIf}
  InitPluginsDir
  File /oname=$PLUGINSDIR\deep-legends.ico "${DL_INSTALLER_ICON}"
  System::Call 'user32::CreateWindowExW(i 0, w "STATIC", w "", i 0x50000003, i 0, i 0, i 0, i 0, p $DLDialog, p 0, p 0, p 0)p.s'
  Pop $DLIcon
  !insertmacro DLPosition $DLIcon 626 18 44 48
  System::Call 'kernel32::MulDiv(i 44, i $DLDpi, i 96)i.r1'
  System::Call 'user32::LoadImageW(p 0, w "$PLUGINSDIR\deep-legends.ico", i 1, i r1, i r1, i 0x10)p.s'
  Pop $DLIconHandle
  ; STM_SETICON (0x0170) expects its handle in wParam, not lParam.
  SendMessage $DLIcon ${STM_SETIMAGE} 1 $DLIconHandle
FunctionEnd

Function DLInstFilesShow
  StrCpy $DLStep 2
  StrCpy $DLDialog $mui.InstFilesPage
  SetCtlColors $mui.InstFilesPage 0x20242B 0xFFFFFF
  ShowWindow $mui.InstFilesPage.Log ${SW_HIDE}
  ShowWindow $mui.InstFilesPage.ShowLogButton ${SW_HIDE}
  ShowWindow $mui.InstFilesPage.Text ${SW_HIDE}
  Call DLTheme
  StrCpy $DLProgressIcon $DLIconHandle
  !insertmacro DLLabel $DLDialog 30 164 640 24 "正在写入程序文件…" $DLBodyFont 20242B FFFFFF
  System::Call 'uxtheme::SetWindowTheme(p $mui.InstFilesPage.ProgressBar, w " ", w " ")'
  !insertmacro DLPosition $mui.InstFilesPage.ProgressBar 30 198 640 10
  ; Native progress messages use BGR COLORREF: RGB E3E7ED -> EDE7E3.
  SendMessage $mui.InstFilesPage.ProgressBar ${PBM_SETBKCOLOR} 0 0xEDE7E3
  SendMessage $mui.InstFilesPage.ProgressBar ${PBM_SETBARCOLOR} 0 0x41A4D9
  !insertmacro DLLabel $DLDialog 30 224 640 20 "✓  校验安装包" $DLStepFont 626C7A FFFFFF
  !insertmacro DLLabel $DLDialog 30 252 640 20 "✓  创建安装目录" $DLStepFont 626C7A FFFFFF
  !insertmacro DLLabel $DLDialog 30 280 640 20 "›  写入程序文件" $DLStepFont 8A641F FFFFFF
  !insertmacro DLLabel $DLDialog 30 308 640 20 "○  创建快捷方式" $DLStepFont 626C7A FFFFFF
  ${NSD_SetText} $DLFooterText "这一步通常需要 10 ~ 30 秒"
  GetDlgItem $0 $HWNDPARENT 1
  EnableWindow $0 0
  GetDlgItem $0 $HWNDPARENT 2
  EnableWindow $0 1
FunctionEnd

Function DLInstFilesLeave
  ${NSD_FreeIcon} $DLProgressIcon
  StrCpy $DLProgressIcon 0
  Call DLHideStock
FunctionEnd

Function DLDirectoryPage
  ${If} ${isUpdated}
    Abort
  ${EndIf}
  nsDialogs::Create 1044
  Pop $DLDialog
  ${If} $DLDialog == error
    Abort
  ${EndIf}
  StrCpy $DLStep 1
  Call DLTheme
  !insertmacro DLLabel $DLDialog 30 140 640 24 "安装到哪里" $DLButtonFont 20242B FFFFFF
  !insertmacro DLLabel $DLDialog 30 168 640 22 "默认装在当前用户目录下，不需要管理员权限。" $DLStepFont 626C7A FFFFFF
  ${NSD_CreateDirRequest} 0 0 1 1 "$INSTDIR"
  Pop $DLDirectory
  SendMessage $DLDirectory ${WM_SETFONT} $DLBodyFont 1
  SetCtlColors $DLDirectory 20242B F5F6F8
  !insertmacro DLPosition $DLDirectory 30 204 554 34
  ${NSD_OnChange} $DLDirectory DLDirectoryChanged
  ${NSD_CreateBrowseButton} 0 0 1 1 "浏览…"
  Pop $0
  SendMessage $0 ${WM_SETFONT} $DLButtonFont 1
  !insertmacro DLPosition $0 592 204 78 34
  ${NSD_OnClick} $0 DLBrowse
  !insertmacro DLLabel $DLDialog 30 250 640 20 "" $DLStepFont 626C7A FFFFFF
  StrCpy $DLSpace $0
  Call DLUpdateSpace
  !insertmacro DLLabel $DLDialog 30 286 640 40 "   仅安装到本机，不需要账号密码，不写入游戏目录。" $DLStepFont 626C7A F5F6F8
  !insertmacro DLLabel $DLDialog 30 286 2 40 "" $DLStepFont D9A441 D9A441
  ${NSD_SetText} $DLFooterText ""
  GetDlgItem $0 $HWNDPARENT 1
  ${NSD_SetText} $0 "开始安装"
  nsDialogs::Show
  ${NSD_FreeIcon} $DLIconHandle
FunctionEnd

Function DLDirectoryChanged
  Pop $0
  Call DLUpdateSpace
FunctionEnd

Function DLUpdateSpace
  StrCpy $1 0
  StrCpy $2 0
  ClearErrors
  ${Do}
    SectionGetSize $1 $3
    ${If} ${Errors}
      ${ExitDo}
    ${EndIf}
    IntOp $2 $2 + $3
    IntOp $1 $1 + 1
  ${Loop}
  IntOp $2 $2 + 1023
  IntOp $2 $2 / 1024
  ${NSD_GetText} $DLDirectory $3
  StrCpy $3 $3 3
  System::Call 'kernel32::GetDiskFreeSpaceExW(w r3, *l.r4, p 0, p 0)i.r0'
  ${If} $0 != 0
    System::Int64Op $4 / 1048576
    Pop $4
    ${NSD_SetText} $DLSpace "需要空间 $2 MB    可用空间 $4 MB"
  ${Else}
    ${NSD_SetText} $DLSpace "需要空间 $2 MB    可用空间未知"
  ${EndIf}
FunctionEnd

Function DLBrowse
  Pop $0
  ${NSD_GetText} $DLDirectory $1
  nsDialogs::SelectFolderDialog "选择安装位置" "$1"
  Pop $0
  ${If} $0 != error
    ; Browsing selects a parent directory; never double-append our leaf.
    ClearErrors
    GetFullPathName $1 "$0"
    IfErrors dl_browse_done
    StrCpy $0 $1
    dl_trim_selected_directory:
      StrLen $1 $0
      ${If} $1 > 3
        StrCpy $2 $0 1 -1
        ${If} $2 == "\"
          StrCpy $0 $0 -1
          Goto dl_trim_selected_directory
        ${EndIf}
      ${EndIf}
    ${GetFileName} "$0" $1
    StrCmp $1 "Deep Legends" dl_selected_directory_ready
    StrCpy $2 $0 1 -1
    ${If} $2 == "\"
      StrCpy $0 "$0Deep Legends"
    ${Else}
      StrCpy $0 "$0\Deep Legends"
    ${EndIf}
    dl_selected_directory_ready:
    ${NSD_SetText} $DLDirectory $0
  ${EndIf}
  dl_browse_done:
FunctionEnd

Function DLDirectoryLeave
  ${NSD_GetText} $DLDirectory $0
  ; Local absolute drive paths only; never install into a drive root.
  StrCpy $1 $0 3
  StrCpy $2 $1 2 1
  ${If} $2 != ":\"
    MessageBox MB_OK|MB_ICONEXCLAMATION "请选择本机磁盘中的安装文件夹。"
    Abort
  ${EndIf}
  ClearErrors
  ; NSIS GetFullPathName additionally checks the final component exists.
  ; Win32 canonicalizes a not-yet-created destination without that check.
  System::Call 'kernel32::GetFullPathNameW(w r0, i ${NSIS_MAX_STRLEN}, w .r1, p 0)i.r2'
  ${If} $2 == 0
  ${OrIf} $2 >= ${NSIS_MAX_STRLEN}
    MessageBox MB_OK|MB_ICONEXCLAMATION "安装路径无效，请重新选择。"
    Abort
  ${EndIf}
  StrLen $2 $1
  ${If} $2 <= 3
    MessageBox MB_OK|MB_ICONEXCLAMATION "请在磁盘中选择一个文件夹，不要直接安装到磁盘根目录。"
    Abort
  ${EndIf}
  ; Check/create the chosen folder only after the user clicks Install. A
  ; unique empty probe catches file paths, inaccessible drives and denied
  ; write access on this page instead of failing halfway through extraction.
  ClearErrors
  CreateDirectory "$1"
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONEXCLAMATION "无法创建安装文件夹，请选择其他位置。"
    Abort
  ${EndIf}
  ClearErrors
  GetTempFileName $2 "$1"
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONEXCLAMATION "安装文件夹不可写，请选择其他位置。"
    Abort
  ${EndIf}
  Delete "$2"
  ; Validate the canonical path, including C:\folder\.. resolving to C:\.
  StrCpy $INSTDIR $1
FunctionEnd

Function DLFinishPage
  nsDialogs::Create 1044
  Pop $DLDialog
  ${If} $DLDialog == error
    Abort
  ${EndIf}
  StrCpy $DLStep 3
  Call DLTheme
  !insertmacro DLLabel $DLDialog 30 142 44 48 "✓" $DLFont 8A641F FAF3E3
  ; Hexagonal completion badge. SetWindowRgn owns the region on success.
  System::Call 'user32::SetWindowLongW(p r0, i -16, i 0x50000201)'
  IntOp $1 $7 / 2
  IntOp $2 $8 / 4
  IntOp $3 $8 - $2
  System::Call '*(i r1, i 0, i r7, i r2, i r7, i r3, i r1, i r8, i 0, i r3, i 0, i r2)p.r4'
  System::Call 'gdi32::CreatePolygonRgn(p r4, i 6, i 1)p.r1'
  System::Free $4
  System::Call 'user32::SetWindowRgn(p r0, p r1, i 1)i.r2'
  ${If} $2 == 0
    System::Call 'gdi32::DeleteObject(p r1)'
  ${EndIf}
  !insertmacro DLLabel $DLDialog 30 207 640 24 "安装完成" $DLButtonFont 20242B FFFFFF
  !insertmacro DLLabel $DLDialog 30 236 640 22 "你的下一场，从这里开始。" $DLStepFont 626C7A FFFFFF
  !insertmacro DLLabel $DLDialog 30 258 640 22 "桌面与开始菜单已创建「Deep Legends」快捷方式。" $DLStepFont 626C7A FFFFFF
  ${NSD_CreateCheckbox} 0 0 1 1 "立即启动 Deep Legends"
  Pop $DLLaunch
  SendMessage $DLLaunch ${WM_SETFONT} $DLBodyFont 1
  ; A checkbox is a BUTTON too, but unlike a push button its *label* is drawn
  ; through WM_CTLCOLORSTATIC, so SetCtlColors does reach the text here and the
  ; caption stays readable on the white page. Only the small glyph stays system
  ; coloured.
  SetCtlColors $DLLaunch 20242B FFFFFF
  !insertmacro DLPosition $DLLaunch 30 305 640 24
  ${NSD_Check} $DLLaunch
  ${NSD_SetText} $DLFooterText "已安装 ${VERSION}"
  GetDlgItem $0 $HWNDPARENT 1
  ${NSD_SetText} $0 "完成"
  GetDlgItem $0 $HWNDPARENT 2
  ShowWindow $0 ${SW_HIDE}
  nsDialogs::Show
  ${NSD_FreeIcon} $DLIconHandle
FunctionEnd

Function DLFinishLeave
  ${NSD_GetState} $DLLaunch $0
  ; Take the window off screen BEFORE launching anything. Every way of starting
  ; the app from here -- StdUtils.ExecShellAsUser, ExecShell, Exec -- ends up
  ; inside a ShellExecute/CreateProcess call that does not return until Windows
  ; has finished creating the new process, and creating a process from a
  ; just-written unsigned executable means waiting out the antivirus scan of the
  ; new image (measured 2217ms on the reporter's machine as process_to_js).
  ; UAC::Unload in .onGUIEnd can add more. Doing it with the wizard still up is
  ; what made "完成" feel like it froze for several seconds; hiding first means
  ; the window disappears on the click and the wait happens off screen.
  ; The checkbox is read first because the hidden dialog is about to be torn down.
  ShowWindow $HWNDPARENT ${SW_HIDE}
  ${If} $0 == ${BST_CHECKED}
    StrCpy $1 ""
    ${If} ${isUpdated}
      StrCpy $1 "--updated"
    ${EndIf}
    ${StdUtils.ExecShellAsUser} $0 "$launchLink" "open" "$1"
  ${EndIf}
FunctionEnd
!macroend
!endif
