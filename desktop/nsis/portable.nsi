# Deep Legends 定制便携版启动器（构建时覆盖 electron-builder 内置 portable.nsi）。
#
# 内置模板的行为是：每次双击都把整个应用（约 400MB）解压到 %TEMP%，
# 退出后再整体删除，导致每次启动都要等待数秒到数十秒。
# 本模板改为版本化缓存：
#   1. 首次启动解压到 %LOCALAPPDATA%\<产品名>\app-<版本>，期间显示解压进度窗口；
#   2. 缓存完整时后续启动完全静默，直接运行缓存内主程序，实现秒开；
#   3. 版本升级时清理旧缓存后重新解压，解压完成才写入就绪标记，
#      中途失败下次会自动重新解压。
#
# 依赖 electron-builder 生成的宏：PRODUCT_NAME、VERSION、APP_FILENAME、
# APP_EXECUTABLE_FILENAME（common.nsh）、extractEmbeddedAppPackage（zip 分支）。

!include "common.nsh"
!include "extractAppPackage.nsh"

# https://github.com/electron-userland/electron-builder/issues/3972#issuecomment-505171582
CRCCheck off
WindowIcon Off
AutoCloseWindow True
RequestExecutionLevel ${REQUEST_EXECUTION_LEVEL}
Caption "${PRODUCT_NAME}"

Var CacheHit

Function .onInit
  StrCpy $INSTDIR "$LOCALAPPDATA\${APP_FILENAME}\app-${VERSION}"
  StrCpy $CacheHit "0"
  ${If} ${FileExists} "$INSTDIR\${APP_EXECUTABLE_FILENAME}"
  ${AndIf} ${FileExists} "$INSTDIR\.deep-legends-ready"
    StrCpy $CacheHit "1"
    SetSilent silent
  ${EndIf}

  !insertmacro check64BitAndSetRegView
FunctionEnd

Section
  InitPluginsDir

  ${If} $CacheHit != "1"
    # 清理当前用户的旧版本缓存，再解压当前版本。
    RMDir /r "$LOCALAPPDATA\${APP_FILENAME}"
    SetOutPath $INSTDIR
    # 本项目只构建 x64 单架构：electron-builder 此时提供 APP_DIR_64
    # （文件直接随安装器压缩，解压带真实进度条）；未提供时回退到
    # 上游的内嵌压缩包方案。
    !ifdef APP_DIR_64
      File /r "${APP_DIR_64}\*.*"
    !else
      !insertmacro extractEmbeddedAppPackage
    !endif
    FileOpen $0 "$INSTDIR\.deep-legends-ready" w
    FileWrite $0 "${VERSION}"
    FileClose $0
  ${EndIf}

  System::Call 'Kernel32::SetEnvironmentVariable(t, t)i ("PORTABLE_EXECUTABLE_DIR", "$EXEDIR").r0'
  System::Call 'Kernel32::SetEnvironmentVariable(t, t)i ("PORTABLE_EXECUTABLE_FILE", "$EXEPATH").r0'
  System::Call 'Kernel32::SetEnvironmentVariable(t, t)i ("PORTABLE_EXECUTABLE_APP_FILENAME", "${APP_FILENAME}").r0'
  ${StdUtils.GetAllParameters} $R0 0

  # 用 Exec 而非 ExecWait：启动器立即退出，不驻留进程，
  # 也不再在退出后删除缓存目录。
  SetOutPath $INSTDIR
  Exec '"$INSTDIR\${APP_EXECUTABLE_FILENAME}" $R0'
SectionEnd
