## R240 授权窗口切换、首帧、运行中锁定与启动续租

执行日期：2026-10-07；时间均为 Asia/Shanghai。先亲自通读 R240 工单全文及本账本 R239、S16 两节；范围为本仓库 P1–P5 和最后 P7。先完成 P3 诊断及真实 Electron 复现，再修改窗口逻辑。P6 属于另一仓库，本会话未访问或执行；R238 发布工单未执行。版本保持 **0.12.75**，未改授权协议字段、签名域、错误码或共享向量，未创建 tag、发布或读取任何私钥文件，未使用真实注册码。新增授权测试使用该测试新生成的内存密钥。

### P3 先诊断与复现证据

- 首先增加 `license_window_state` 客户端白名单、严格几何结构和实际诊断写入/导出处理，再给原 R239 窗口逻辑加诊断。`TestR240WindowDiagnosticReachesActualExportAndRejectsIdentity` 用实际 Go HTTP 处理器走 POST→磁盘→GET 导出，并拒绝嵌套 code/任意身份值；不是仅写 desktop.log。字段包括 from/to、切换前最大化/全屏、请求/实际内容区、显示缩放、耗时、retry、原生等待/渲染超时、状态/尺寸不一致，不含注册码。
- 修改尺寸/等待逻辑前，真实 macOS Electron 的生产 main.cjs + 合成本地状态接口实跑最大化→REVOKED→ACTIVE。实测最大化 1680×1020→锁定 700×470→恢复最大化；**本机未复现“最大化后被锁不缩小”**，没有把猜测写成复现结论。先诊断的原始记录见 [baseline/result](history/reports/r240/electron/baseline-ACTIVE-1/result.json)、[诊断](history/reports/r240/electron/baseline-ACTIVE-1/diagnostic-export.jsonl)、[首次 show 截图](history/reports/r240/electron/baseline-ACTIVE-1/show-1.png)。
- 同一实跑确实复现首帧错误：Go 为 ACTIVE，但第一次 show 即时 capturePage 的 DOM 为 locked、formVisible=true、frameHidden=true，激活图标区域有 474 个金色像素。

### P1–P4 最终实现与核验

- 授权内容区固定 **860×580 DIP**，居中、不可拖大/最大化，不按屏幕分辨率自适应。卡片 480、图标 64×64、标题 24px、输入 46、按钮 44、提示与链接 13px；保留布局顺序和原文，仅按 P4 改 REPLACED 错误提示。隐私弹窗仍为内部滚动、底部关闭按钮；R117 CSS 预算未放宽。
- 选择 **opacity=0**：保留原生显示/焦点及渲染帧推进，几何修改前先透明，避免 hide 后后台节流妨碍两次 RAF。原生退出全屏/最大化分别等待对应事件，上限 600ms；再设内容尺寸，误差>2 DIP 最多重试一次并记诊断，最后才禁用 resize/maximize。锁定尺寸及切换事件不写 window-bounds.json，恢复原正常位置/尺寸、最大化或全屏。
- HTML 默认 pending，遮罩和主框架均 hidden。主进程用本地 token 读 Go 状态后才推送应用消息；页面应用后两次 requestAnimationFrame 发送仅含 renderId 的时间信号。ACK 仍校验主窗口 sender/mainFrame/origin，并再次读 Go：状态不同重新应用，generation 不同重发。1500ms 上限超时显示并记录；不接受页面状态作为授权依据。
- 原生纯模块测试覆盖普通/max/full→锁定→恢复、unmaximize 不来超时、首次尺寸被系统改回的重试、opacity 在几何前为 0、渲染等待/超时与落盘保护。缩放入口复核：授权切换内最小尺寸变更处于透明态；锁定后的 display/resize 回调仅清除最小尺寸，不改变固定内容区。
- 真实 Electron 最终验收：最大化→锁定 **860×580**、resize/maximize=false→恢复最大化；第一 show 即时 capturePage 和 DOM 双重判断，ACTIVE formVisible=false/金色像素=0，LOCKED formVisible=true/金色像素=642。额外让 Go 在渲染 ACK 前 ACTIVE→LOCKED，首次 show 仍为锁定表单并记录 renderMismatch。见 [acceptance](history/reports/r240/electron/acceptance.json)。
- 已在 Windows CI 增加明确真实 Electron 用例和截图/JSON 上传；Node.exe 合成 backend 子进程无需 cmd/shell，生产 main 与 BrowserWindow API 真实。**已加入 CI，未在远端跑**；本机 macOS 合成状态不等于 Windows、DPAPI、真实租约或两机 S16。
- REPLACED 改为“注册码已在其他设备使用或已被重置”；REVOKED 保持“注册码已停用”，所有运行中的引用测试/快照同步。旧历史账本文字保留历史含义。
- 双续租实证：缓存 ACTIVE 的首个请求跨过 1s tick，旧 Run 在 requests token 被 worker 取走、nextRenew 仍为 0 时又入队，前 5s **2 次**；新 queued 占位一直保留到 maintain 完成，前 5s **1 次**，首请求仍在 500ms 内立即发出。60s heartbeat 未改。见 [before](history/reports/r240/renew-before.log)、[after](history/reports/r240/renew-after.log)，最终两标签也实际运行该 5s 测试。

### P2 启动交错采样

用真实 Electron、新隔离 profile、合成本地 HTTP 状态，前后交错 ACTIVE/LOCKED 各 3 次；故意将 renderer 首次状态响应延迟 500ms 暴露首帧竞态，native token 请求不延迟。第一次 show 的调用栈立即发起 capturePage 与 DOM 采样，不以等待 1s 的截图或仅测尺寸代替。原始 [startup.json](history/reports/r240/electron/startup.json)、[统计](history/reports/r240/startup-comparison.json)。

| 状态 | 前 total ms（三次） | 后 total ms（三次） | 平均前→后 ms | 中位前→后 ms | 首帧内容 |
|---|---|---|---|---|---|
| ACTIVE | 985 / 835 / 883 | 916 / 913 / 832 | 901→887 | 883→913 | 前 3/3 错误激活表单；后 3/3 正确主界面、图标像素 0 |
| LOCKED | 829 / 852 / 844 | 876 / 816 / 750 | 841.67→814 | 844→816 | 后 3/3 首帧激活表单，固定 860×580 |

本机暖启动均值未回退；ACTIVE 中位数增加 30ms，未声称每个样本更快。仅此小样本/合成状态的性能结论；Windows 冷启动和真实缓存仍待 S16。

### P5 四项变异与最终检查

- [真实 Chromium](history/reports/r240/browser/chromium.json) 为 860×580，覆盖 pending 双隐藏、全部 R239 场景、暗/亮主题和 1/2.5 主界面 zoom 下授权 zoom=1、关闭 dialog/input 命中/焦点/Tab、隐私真实点击/Close/Esc/长文内部滚动/失败消息、输入 Enter/disabled/清空、REPLACED 一行、无页面滚动与业务请求；没有用 JSDOM 替代新渲染验收。
- 临时副本/Go overlay 四变异：a 默认遮罩可见、b 不透明直接改尺寸、c 去掉 unmaximize 等待、d 恢复旧 REPLACED 文案，**全部 exit 1，日志命中对应断言，非编译/语法失败**，临时副本结束后删除。最终重跑见 [results](history/reports/r240/mutations/results.json)、[完整入口日志](history/reports/r240/mutations-final.log)。
- 以下均在最终对应源码上实跑。JS 全量涉及的 HTML/CSS/JS/desktop 源码在测试后不变；并行 R241 仅更新 Go/Go 测试及独立 Chromium 脚本，受影响 Go 检查补跑。774 文件 SHA 对照见 [final source](history/reports/r240/final-source-files.json)。

| 命令 | 开始（+08:00） | 结束（+08:00） | 耗时 s | 结果 |
|---|---|---|---|---|
| `go test -count=1 ./backend` | 02:52:22.781609 | 02:55:11.435228 | 168.654 | exit 0 |
| `go test -count=1 -race ./backend` | 02:42:22.984198 | 02:46:02.738533 | 219.756 | exit 0 |
| `go vet ./...` | 02:51:02.091545 | 02:51:03.518020 | 1.426 | exit 0 |
| `go test -count=1 -race -json -run R232|R233|R234|R236|R237|R240 ./backend` | 02:46:04.126984 | 02:46:13.237373 | 9.111 | exit 0；33 顶层 PASS，0 SKIP/FAIL |
| `go test -count=1 -race -json -tags=license_staging -run R232|R233|R234|R236|R237|R240 ./backend` | 02:46:13.242902 | 02:46:45.586360 | 32.344 | exit 0；34 顶层 PASS，0 SKIP/FAIL |
| `node scripts/test-renderers.cjs all` | 02:36:12.669192 | 02:37:43.343304 | 90.675 | exit 0 |
| `node --test` | 02:37:43.344961 | 02:39:03.515882 | 80.172 | exit 0 |

完整时刻与日志： [checks-go](history/reports/r240/checks-go.json)、[checks-node](history/reports/r240/checks-node.json)。实际末五行（不足五行原样保留，vet 无输出）：

`go test -count=1 ./backend`

```text
ok  	lol-loot-assistant/backend	166.986s
```

`go test -count=1 -race ./backend`

```text
ok  	lol-loot-assistant/backend	206.171s
```

`go vet ./...`

```text
（无输出）
```

`go test -count=1 -race -json -run R232|R233|R234|R236|R237|R240 ./backend`

```text
{"Time":"2026-10-07T02:46:11.954077+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Output":"--- PASS: TestR232UpdateSignatureTrust (0.00s)\n"}
{"Time":"2026-10-07T02:46:11.956283+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Elapsed":0}
{"Time":"2026-10-07T02:46:11.956299+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T02:46:12.968959+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t7.176s\n"}
{"Time":"2026-10-07T02:46:12.972311+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":7.18}
```

`go test -count=1 -race -json -tags=license_staging -run R232|R233|R234|R236|R237|R240 ./backend`

```text
{"Time":"2026-10-07T02:46:44.335852+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Output":"--- PASS: TestR236StagingUpdateHTTPGate (0.00s)\n"}
{"Time":"2026-10-07T02:46:44.335879+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Elapsed":0}
{"Time":"2026-10-07T02:46:44.338138+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T02:46:45.350916+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t7.535s\n"}
{"Time":"2026-10-07T02:46:45.352176+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":7.536}
```

`node scripts/test-renderers.cjs all`

```text
    },
    "duration_ms": 90629.620541
  },
  "success": true
}
```

`node --test`

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 80136.46325
```

- 渲染全量 166 文件、1337 项：1333 PASS、4 条平台条件跳过、0 FAIL；总 90.63s，最慢单文件 48.51s，未放宽 90s/240s。desktop 303 项：301 PASS、2 条既有平台条件跳过、0 FAIL。授权专项两标签为严格 0 SKIP/FAIL。
- `dirty collection rescans on entry and view changes without duplicate refresh requests` 在两次最终 JS 全量都 PASS，未触发单跑三次。其文件和 backend/web/app.js、package/lock SHA 与本轮开始相同，见 [保护文件](history/reports/r240/protected-files.json)。
- 中断/失败没有隐去：首轮长进程失去工具会话且进程清单已空，未拿到结束码，不记通过，保留 interrupted 日志；旧夹具 `TestGameplayOverviewOverlapsIndependentUpstreams` 首轮 FAIL，R241 会话在共享区修了其前台匹配（本会话未修改）；下一轮 `TestGameplayOverviewReturnsPartialBeforeTwentySeconds` 清理超时 FAIL，该用例原样单跑 PASS，并行会话结束后 Go 全量原样重跑 PASS。见 [首次汇总](history/reports/r240/checks-go-first.json)、[补跑失败](history/reports/r240/go-test-final-summary.json)、[单跑](history/reports/r240/unrelated-go-focused-summary.json)、[稳定全量](history/reports/r240/go-test-stable-summary.json)。
- vet 最初把 `docs/history/reports/r241/baseline/backend` 的源码摘录当作应用包，报 LCUClient 未定义；只新增 baseline/go.mod 隔离报告快照，不修改摘录、测试或业务，精确 `go vet ./...` 补跑 PASS。外部四文件变更时间/SHA 见 [shared-source-changes](history/reports/r240/shared-source-changes.json)。

