# R80 / R81 独立验收报告

验收人：Claude（只读验收，**未修改仓库任何文件**；全部变异测试在 `/tmp` 副本中进行）。
日期：2026-09-11。对象：`WORKLIST-R80-INSTALLER-SHELL.md`（安装器 A~G + 卸载器 H）、
`WORKLIST-R81-AUTO-UPDATE.md`（A~G）。

---

## 一、总评

**四组全部真实落地，不是假绿。** 我自己设计并执行了 32 处独立变异（不复用 GPT 自带的
`scripts/r80-uninstall-mutations.py` / `r81-mutations.py`），其中 31 处被现有测试杀掉。

GPT 自写的两份 ACCEPTANCE-REPORT **内容异常诚实**——主动标注了 F3 未上传、16 项真机待测、
以及 Riot Key 的发布冲突。抽查下来没有发现虚报或夸大。

有 **2 个真问题**需要你决策，其中 1 个是用户可见的界面偏差。

### 基线（先确认为绿，否则所有变异结论作废）

| 套件 | 结果 |
|---|---|
| 根模块 `go test ./...` | ok，66.2s |
| `installer` 模块 `go test ./...` | ok（含 `uninstall` 子包） |
| `cd desktop && node --test` | **187 pass / 0 fail** |
| 交叉编译安装器/卸载器外壳 | 均为 PE32+ **GUI** 子系统（非 console，不会闪黑窗） |

### 变异测试汇总

| 组 | 变异数 | 被杀 | 漏网 |
|---|---|---|---|
| R80 安装器 A~G | 13 | 13 | 0 |
| R80 卸载器 H | 6 | 6 | 0 |
| R81 更新后端 A/B | 13 | 12 | **1** |
| R81 前端/交接 C/D/E/G | 9 | 9 | 0 |
| **合计** | **41** | **40** | **1** |

---

## 二、需要你决策的问题

### P1：升级图标尺寸——已确认，无需改动

~~我原以为 GPT 把方向做反了~~：用户复核后确认现状（30px + 现有箭头）就是对的，**不用改**。

你给我的原话是 **"图标整体再大一点点，箭头再小一点点"**，我据此把工单 E 节定稿为
**26px 图标 + 收窄的箭头**（`M12 15.7V9.9` / `M9.5 12.4 12 9.9l2.5 2.5`）。

但 GPT 的报告把诉求转述成 **"图标再大点、箭头也大点"**，实现成
**30px + 恢复成旧的大箭头**：

| 位置 | 仓库现状 | 工单定稿 |
|---|---|---|
| `web/app.css:981` | `width:30px; height:30px` | `26px` |
| `web/index.html:112` | `M12 16.4V9.2` / `M8.9 12.3 12 9.2l3.1 3.1`（旧大箭头） | `M12 15.7V9.9` / `M9.5 12.4 12 9.9l2.5 2.5` |

也就是说**六边形和箭头一起被放大了**，而你要的是六边形放大、箭头收敛。

两版并排真 Chromium 对照图（available / busy 58% / ready 三态 × 浅色深色）：
`output/icon-compare-light.png`、`output/icon-compare-dark.png`。

> ⚠ 这类纯 CSS 像素值 + SVG path 坐标 **jsdom 测不出来，没有任何护栏**。
> 就算改回去，以后也不会有测试拦住再次改错，只能靠人工看截图。

**已确认**：用户看过对比图后表示现状就是对的，保持不变，不用改回 26px + 小箭头。

### P2：`busyLocked()` 并发闸门零测试覆盖

`update.go:304` 定义，被 `update.go:292`、`update.go:317`、`update_download.go:93` 三处引用。

我把函数体整个改成 `return false`（等价于闸门彻底失效），**全量 `go test ./...` 依然 ok（66.9s）**。

工单要求的"连续调 10 次 check 只外发一次"这条**表面行为是真的**，但保护它的是
`Check()` 里的 `lastManual` 5 秒防抖，不是这个并发闸门——现有测试全部走 `force=true`，
单靠防抖就能通过。真正靠 `busyLocked()` 兜底的路径是
**后台 6 小时定时检查与一个正在进行的下载/校验重叠**，这条路径完全没测。

风险不高（不影响当前功能），但这个函数以后被误删/改错不会有任何测试报红。

---

## 三、已确认真实落地的关键项（抽样）

- **B 节没踩 `webview2.New*` 的坑**：`grep "webview2\.New"` 全仓零命中，直接用
  `pkg/edge`（`window_windows.go:8`），窗口 style 只有 `WS_POPUP|WS_SYSMENU|WS_MINIMIZEBOX|WS_CLIPCHILDREN`，
  无 `WS_CAPTION/WS_OVERLAPPEDWINDOW/WS_THICKFRAME` ⇒ 不会闪系统标题栏。
- **`/D=` 命令行手工拼装**：`install_windows.go:136` 用 `SysProcAttr.CmdLine`，
  `exec.Command` 只传 exe 路径。变异（把 `/D=` 挪到非末位、给路径加引号）双双被杀。
- **进度模型常量齐全**：`progress.go:8-27` 的 `progressPrepare=4` /
  `progressBeforeExit=97` / `progressTimeFloorCap=80`，去掉封顶后进度会飙到 934 被测试抓住。
- **★卸载数据缺口已补**：`installer/uninstall/finish.go:12-18` 在 Core 成功后额外删
  `%LOCALAPPDATA%\LOLLootAssistant`，`uninstall_test.go:75-129` 用四个子用例
  （勾选+成功 / 不勾选 / Core 失败 / 缓存被占用）钉死，双向变异都被杀。
  `storage.go:21`+`:99-103` 口径未变，我诊断的缺口确实成立且确实修了。
- **不碰注册表、不改 NSIS 核心**：`desktop/nsis/installer.nsh` 只追加了
  `DL_UNINSTALL_SHELL` + `customInstall` 两处，vendored 的 `uninstaller.nsh:183-185`
  `RMDir /r $INSTDIR` 原样未动。
- **没用 `api.github.com`**：全仓 grep 零命中，走的是
  `releases/latest/download/latest.json`（`update.go:20-22`），镜像顺序回退 + 单源 8s 超时。
- **SHA-256 边下边算 + 续传补哈希**：`update_download.go:247-256`（TeeReader）、
  `:244-252`（续传前把已有字节读进 hash），绕过校验/不补哈希/重试次数放大三种变异全被杀。
- **换镜像重新探测 Range**：每个镜像独立发 HEAD，不沿用上一个镜像结论（`:197-217`），有专项测试。
- **速度是 5 秒滑动窗口**：`update_download.go:22-48`，改成"总字节/总耗时"立刻被抓。
- **E 节护栏都是真渲染断言不是正则匹配**：16 个 DOM id 逐个删除 → jsdom 真报
  `el.xxx is undefined`；去掉 `escapeHTML` → `<img onerror>` 用例抓住；
  放行 `javascript:` → `querySelectorAll("a")` 断言抓住。
- **D 节升级参数正确**：`installer/update.go:54` 固定 `/S --updated /D=<dest>`，
  **传了 `--updated`、没传 `--no-desktop-shortcut`**，两个反向变异都被杀。

---

## 四、未完成 / 无法在沙箱验证

1. **F3 实际上传 GitHub 未做**（GPT 已自曝）：没建 tag/Release，`latest.json` 没上传
   ⇒ **更新链路端到端目前不通**，前面所有代码都还没法真正生效。
2. **发布拦路虎（建议优先处理）**：GPT 指出当前构建**内嵌个人 Riot API Key**，
   与公开分发冲突。这和既有的"EXE 内嵌 personal key 全员共享配额"问题是同一条，
   公开发 Release 前必须先解决，否则 key 会被滥用/封。
3. **真机项**：R80 的 16 项 + R81 的升级闭环，都需要 Windows + WebView2 实机。
   沙箱只能验证代码逻辑、交叉编译和浏览器渲染，验不了无边框窗口观感、DPI 缩放、
   UAC 行为、NSIS 真实执行、注册表读写。
4. **镜像可用性**：`ghfast.top` / `gh-proxy.com` / `ghproxy.net` 是否还在线、
   是否真能代理大文件 `.exe`，沙箱无法实测（测试用的是 fake RoundTripper）。

---

## 五、建议的下一步

1. 先定夺 P1 图标方向（看 `output/icon-compare-*.png` 二选一）。
2. 处理 Riot Key 的分发边界——这是发布的前置条件。
3. 再做 F3 上传，跑通一次真机升级闭环。
4. P2 可以顺手补一条 `busyLocked()` 的测试（后台检查与下载重叠），优先级最低。
