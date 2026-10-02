# R185 执行账本

日期：2026-10-02。版本：0.12.50。源码基线：0.12.49。发布前 API 复核发现 v0.12.49 当前仍为草稿，GitHub 最新正式版为 v0.12.19；本次将正式发布 v0.12.50 并设为 Latest。用户明确要求执行工单后发布。

## 范围与证据边界

工单引用的 1002-1606 日志没有前端长任务、CPU、内存指标。本轮只修复已确认的生涯页事件整包重载，补齐下一次定位所需的性能诊断；不能据此宣称所有页面卡顿已经解决。未增加界面文字，未更改游戏设置。

## 逐项执行

| 工单项 | 实现 |
|---|---|
| P1-1 | 后端按 URI 保存内存指纹：chat 仅投影 icon/availability/statusMessage/lol 段位字段；挑战仅称号与展示 ID；regalia/backdrop 整体哈希。相同事件直接忽略，解析失败、部分 chat 事件保守刷新；连接重置清理指纹。 |
| P1-2 | `skins=0` 省略 skins 字段且不读取皮肤目录；其余字段读取路径保持一致。SSE 使用轻量请求，前端只有字段省略才保留旧目录，显式空列表替换；轻量刷新不延长完整目录 TTL。手动、poll 与应用后确认仍完整读取。 |
| P1-3 | 关键字段的数量与双轻量哈希按列表对象缓存于 state WeakMap；SSE 省略皮肤时完全跳过皮肤比较。 |
| P1-4 | 后端最小广播间隔 5 秒，保留窗口结束补发；手动刷新不走此节流。 |
| P1-5 | 有事件时每分钟汇总 `facade_event_source`，固定 chat/challenges/regalia/backdrop 分类，received/ignored/broadcast 与整体实际广播数；不记录原始 URI、聊天或身份。 |
| P2-1 | `renderer_perf` 记录 ≥50ms 长任务并按 section/tab 分组、可用的 heap、DOM/img 数、1 秒计时器超过 1.2 秒的延迟。每分钟有长任务才上报，否则每 5 分钟心跳；保留未上报窗口的延迟。计时器总数无法可靠统计，按工单允许省略。页面事件/点击更新归属，dispose 清理观察器和计时器。 |
| P2-2 | Electron 每分钟用 getAppMetrics 按四类进程聚合 CPU/RSS；后端 RSS 由 Windows Get-Process 或 macOS ps 读取，无法取得省略。主日志及诊断导出均保留结构化白名单字段；没有 PID、路径、命令行或身份。退出清理采样。 |
| P2-3 | Go 每分钟采集 goroutine、HeapAlloc/HeapInuse、NumGC、GC 暂停增量、SSE 连接及本地 HTTP 在途数。时钟/内存读取可注入，采集跟随应用 context，关闭日志前等待结束。HTTP 包装保留 SSE Flusher。 |
| P2-4 | 统一 fetch 计量覆盖总览、对局、收藏、英雄、职业、社交与套件各页签；仅固定 endpoint 分类，包含耗时及可取得的响应字节。JSON/text 消费和 NDJSON reader 无复制读流，诊断请求自身排除；原有状态诊断在未安装统一计量时保留后备。 |

## 验证

- Go R185 专项与 race、既有生涯回归通过；Node R185 8 项通过。
- 全量回归初轮发现请求计量对测试响应的 json/text 兼容、抽取夹具依赖、demo 请求及销毁后的微任务问题，已修复并复跑对应 84 项通过。
- Go 启动端到端夹具因新增采样计时器仍引用事件循环而超时；计时器 unref 后该用例通过（1.290 秒）。生产采样仍随 app 生命周期清理。
- 最终 `go test ./backend` 通过（216.738 秒），`go vet ./backend` 与 `git diff --check` 通过；工单要求的 `go test ./backend -count=1` 无缓存全量也通过（228.566 秒）。
- 最终 Web/Desktop 全量 Node：1058 项，1057 通过、1 项平台条件跳过、0 失败（256.534 秒）。补强显式空 skins 替换旧目录的断言后，R185 8 项再通过（234.843 毫秒）。
- 三项最终 overlay 均实际断言 FAIL：不比较指纹时 ignored=0 而非 30；SSE 整包请求缺少 `&skins=0`；移除签名缓存时遍历 11 次而非 1 次。临时 overlay 位于 /private/tmp/r185-mutants，没有修改生产源码。
- scripts 发布/构建工具回归 39 项：37 通过、2 项平台跳过、0 失败（11.565 秒）。
- 完整 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh` 通过：1645 个 Go 测试五分片、主模块与 installer 的 vet/test、Windows 后端交叉构建、NSIS 与安装器 shell、包内 runtime（包含 process-metrics）、源码/包内指纹、public 模式收据及 SHA-256。没有嵌入 Riot Key，最终只保留 `-public` 安装包。日志 /private/tmp/release-0.12.50-build.log。
- `node scripts/make-release.cjs` 经 public 收据生成发布三件套。指纹 **ea5645b17558**；安装包 110935552 字节；SHA-256 `d24ab6470bb2cba9fa477e2a28291276a36df8c9ea2d0c66aa9d2fccc515ce8e`。latest.json 的名称、URL、大小、SHA-256 与包一致，SHA256SUMS-public.txt 校验两文件。
- GitHub 上传、正式发布与匿名更新入口结果将在完成后补入本节。

## 发布与真机验收

- 发布使用 `DEEP_LEGENDS_KEY_MODE=public`，不嵌入个人 Riot API Key；最终包必须有 `-public` 后缀并经过构建收据、指纹及更新清单校验。
- 尚未执行 Windows 一小时真机验收。用户需连续使用至少一小时，包含大厅停留、多局游戏、页面切换；卡顿时记时间和页面，保持软件运行并导出日志。新日志用 renderer_perf / desktop_process_metrics / backend_runtime_metrics 同一时段核对。
