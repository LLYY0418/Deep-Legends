# WORKLIST-R83-ADDENDUM-EXECUTE-TIMING · 执行预热没赶上：目标目录太晚才"定稳"

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。
数据来源：用户真机新构建（指纹 `00d37a7f7892`）真实安装一次，
`%TEMP%\DeepLegendsSetup-startup-c45d9aff.log`（第 197-211 行）。日期：2026-09-12。

## 结论先行

R83 的执行预热（把"读一遍"升级成"真的跑一遍"）这次**一次都没有真正跑完**，
两个目标全部 `reason=still_running`、`execute_valid=0`，最终静默回退成了和上一版
完全一样的读预热。日志证据：

```
[pid=26180] ... startup warm mode=execute file=主程序 ready_ms=10678 ... elapsed_ms=262 voided=false attempted=true reason=still_running
[pid=26180] ... startup warm mode=execute file=后端 ready_ms=10928 ... elapsed_ms=12 voided=false attempted=true reason=still_running
[pid=26180] ... startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=32 execute_valid=0 fallback_files=2
[pid=26180] ... application launch pid=21028 error=<nil> elapsed_ms=1879
[pid=26180] ... application handoff visible=true elapsed_ms=3801
```

这不是失败/超时（`w.ctx` 没有到 8 秒预算上限），是**两个目标在"最终安装目录"里
定稳的时间点，几乎正好卡在 NSIS 退出前的最后一瞬间**：主程序在安装开始后
10.678 秒才定稳，后端 10.928 秒才定稳，而整个安装（从点"安装"到 NSIS 退出）
一共只用了约 10.94 秒——**留给执行预热的窗口只有 250 毫秒左右，两个进程都刚起步
就被 `Finish()` 里的 `Cancel()` 叫停了**。

（这次的 `elapsed_ms=1879`/`3801` 是第一次拿到的毫秒级精确值，比 R83 之前用
秒级日志反推出的"3.0~3.9 秒"区间更快——但这份改善不能记在这次执行预热的功劳上，
因为它压根没跑完，实际发生的是和上一版完全相同的读预热(`elapsed_ms=32`，和历史
读预热的 35~48ms 同一量级)。这台机器今天已经连续装了十几次不同指纹的构建，
不排除是 Windows 对这类程序的判断速度本身在变快，与这次代码改动无关——
不要把这次的直接测量结果当成"执行预热已经生效"的证据。）

## 根因：NSIS 的两段式拷贝是"整体搬运"，不是"边解压边到位"

查了 electron-builder 用的 NSIS 模板
`desktop/node_modules/app-builder-lib/templates/nsis/include/extractAppPackage.nsh`：

```nsis
!macro extractUsing7za FILE
  CreateDirectory "$PLUGINSDIR\7z-out"
  SetOutPath "$PLUGINSDIR\7z-out"
  Nsis7z::Extract "${FILE}"        ; 一次性把整个压缩包解到 7z-out
  ...
  CopyFiles /SILENT "$PLUGINSDIR\7z-out\*" $OUTDIR   ; 再一次性整体搬到最终目录
```

`Nsis7z::Extract` 是**一次调用解压整个压缩包**，`CopyFiles` 也是**一次调用把
所有文件搬到最终目录**——都不是"一个文件一个文件慢慢写"。我们现在
`installer/execute_prewarm.go` 的 `inspectWarmExecutable` 盯的是**最终安装目录**
（`installer/install_windows.go:135` 附近 `a.startupWarm = newExecutionWarmup(dest, ...)`
的 `dest` 就是 `$OUTDIR`），所以只能等到那次"整体搬运"完全结束才能看到文件——
而这次整体搬运显然是压在整个 10.94 秒流程的最后一小段才完成的。

`installer/install_windows.go:220` 的 `extractedBytes()` 早就在读
`<temp>\ns*.tmp\7z-out` 这个暂存目录来算进度条，说明这条路径在代码里已经有先例，
只是执行预热没有复用它。

## 要做什么

**把执行预热盯的目录从"最终安装目录"改成解压暂存目录 `7z-out`，并且直接从暂存
目录里的文件执行/读取**，而不是等它被搬到最终目录。具体：

1. 复用 `extractedBytes()` 里已经写好的定位逻辑（在 `temp` 下找创建时间不早于
   本次安装开始、且匹配 `ns*.tmp` 的目录，取其 `7z-out` 子目录）来算出本次安装
   实际使用的暂存目录路径。
2. `newExecutionWarmup` 现在只接收 `dest`（最终目录）来拼两个目标路径
   （`installer/prewarm.go:19` 的 `startupPrewarmPaths`）。改成同时能感知暂存目录
   里对应的相对路径（`Deep Legends.exe`、`resources/app.asar.unpacked/backend/loot-service.exe`
   在压缩包内的相对结构，需要你实际确认打包产物内部布局是否就是这个相对路径——
   不要想当然，用一次真实构建解压出来看一眼 `7z-out` 里到底长什么样）。
3. 稳定性判定（现有的"400ms 内大小/mtime/文件身份不变"）逻辑不变，只是现在
   应该能在暂存目录里更早地定稳。
4. **必须处理的风险**：执行预热是在 NSIS 还没把这个文件从暂存目录搬到最终目录
   之前，就直接执行暂存目录里的这份拷贝。这时 `CopyFiles` 可能还没读取/搬运
   这个文件（也可能正好在搬运中）。请评估并测试：
   - 我们的子进程在暂存目录里执行这份文件时，会不会因为触发杀毒软件对它的扫描
     /锁定，反而拖慢或干扰 NSIS 自己那次 `CopyFiles` 读取同一份文件？
     （这正是我们要绕开的同一种扫描行为，两边可能互相打架，需要真实测一下，
     不要假设没事。）
   - 如果暂存目录里的文件后续被 `RMDir /r "$PLUGINSDIR\7z-out"` 删除（重试路径
     才会走到，见 `extractAppPackage.nsh:125`），我们的子进程如果还在跑，
     需要确认不会因为源文件被删除而出现异常/崩溃，且不能影响安装本身。
5. **仍然要在最终目录里做收尾复核**：现有 `Finish()` 阶段"执行完之后文件有没有
   被改写"的复核（`installer/execute_prewarm.go` 的 `voided` 逻辑），改成盯的对象
   变化后，这条复核语义要重新想清楚——万一暂存目录的文件在我们执行完之后、
   NSIS 搬运之前又被覆盖过（理论上不太可能，但工单一贯的原则是不假设），
   要有对应的判断，不能假装没这回事。
6. 保留现有的读预热兜底：暂存目录侦测失败、执行没跑完、暂存目录的文件其实和
   最终目录不一致等任何意外，一律回退到现有的读预热（`runStartupPrewarm`），
   不改变这条兜底逻辑本身。

## 验收判据

1. 用真实构建重新走一次「用户实测三步」（装一次、发回
   `%TEMP%\DeepLegendsSetup-startup.log`），日志里两个目标的 `reason` 应该变成
   `ok`（真正执行完成），`execute_valid=2`，而不是这次的 `still_running`。
2. 如果这次的瓶颈是"7z 解压本身也要跑到接近整个流程末尾"（即暂存目录同样
   定稳得很晚），那就如实报告"改了也没有明显提前多少"，不要为了完成任务硬凑
   一个不真实的改善数字。**这轮的验证方式不变**：装一次、发一个日志文件，
   判定标准还是看 `application launch ... elapsed_ms=` 这一个数（同 R83：
   低于 1000ms 达标；1000~2500ms 算部分改善；高于 2500ms 说明还没解决主要问题）。
3. 第 4 点里"执行预热是否干扰 NSIS 自己的文件拷贝"，请在验收报告里明确写清楚
   怎么排查/排除的，不能只说"应该没事"。

## 不要做的事

- 不要因为这次单个样本测出 `elapsed_ms=1879` 就当成"R83 执行预热已经证明有效"
  写进任何文档——这次的实际行为是读预热（`execute_valid=0`），跟执行预热无关，
  见上面"结论先行"里的说明。
- 不要恢复六次 A/B 实验协议，验证方式沿用"装一次、发一个日志文件"。
- 不要改动 `installer/execute_prewarm.go` 里已经验证过的非阻塞收尾
  （`Finish()` 不等待卡死进程）、8 秒预算、失败/超时/回退这些逻辑本身，
  只改"盯哪个目录、从哪里执行"这一件事。
