# R213 执行账本

2026-10-04；基线 0.12.71。执行范围：[R213 工单](WORKLIST-R213-CHAMPSELECT-DETAIL-ROSTER-FLICKER-PARTIAL-PROGRESS-SNAPSHOT-OVERWRITE.md)。继续沿用用户“不构建、不发布”的要求；不打包、不增版本，保留同工作区 R211/R212 改动。本次完成源码、文档和自动验证；选人录像与新日志真机验收待恢复构建后进行。

## P1：后台刷新保留完整阵容

采用方案 a。同 gameId、同 queueId，已有 available 且非空玩家阵容、非 awaiting 时，进度快照直接忽略；最终响应沿原 `loadLive` 路径一次性替换。旧 requestId、旧局、阶段落后先拒绝，不因同局快照规则绕过隔离。没有实施可选后端快照改造。

首次进入、换局、换队列或 awaiting 仍应用快照。新增 `liveProgressSnapshot` 记录“当前 state.live 是否由本请求的初次进度产生”，避免第一份骨架变成 available 后就把后续逐人结果全部忽略。新请求与最终完整响应清除此标记，后台请求不能继承首次加载例外。忽略时不改快照对象或触发渲染，段位、组队、隐藏战绩、补位、职业标识和位置字段自然保持。

## P2：完整重建也复用图片

HTML 仅解析到 `fullHolder` 一次；先 `preserveLiveImages` 移入原有图片，再 `replaceChildren` 将真实节点移入新的 `data-live-body`，避免重新解析丢失图片对象。保留已加载/排队图片的对象及状态；相同数量、相同地址的图片重建计数为 0。

`live_render_rebuild.full_reasons` 记录 tab-row、banner、panel-count、lane-slot、other，可同时计入多个发生变化的部分。页签只比 `.recommendation-tabs`，提示条比实际 banner 节点；lane-slot 比空槽外壳，内容变化继续走原增量替换，不冒充 full。

## P3：进度来源与聚合

进度触发的状态条/正文重绘记 `sources.progress`。`live_progress_apply` 每 60 秒汇总 applied、ignored_same_game、ignored_stale；同一后台请求的骨架及多个逐人事件只增加一次 ignored_same_game，便于与完整重读次数核对。applied/ignored_stale 按快照计数。requestId 仅用于窗口内去重，不进入诊断输出。dispose flush 并清理定时器与快照状态；销毁后进度事件不重新建立计时器。

runtime 与 Go 同步登记白名单、固定来源和原因词表、计数限幅；记录只含计数和有限 phase，不含名字、PUUID 或原始路径。没有添加界面说明文字。

## 测试与复核

- R204 两条旧测试按方案 a 改写；旧 request/局/阶段拒绝与首次逐人显示保留。R129 完整重建探针同时统计节点移入，原“整块重建必然换图片”的断言改为图片对象复用；R91/R94/R210 等提取夹具注入真实 `preserveLiveImages`，保留原断言。
- 新增 R213：同局完整阵容含两人组队、三人段位、一人隐藏战绩 10 局，骨架/单人结果均不改对象和完整 markup；首次骨架及后续 ready 玩家；新请求隔离；换局/awaiting/换队列；40 张图片含已加载与排队状态的节点身份；full 原因；60 秒聚合去重、dispose 与白名单匿名性。
- 定向 Node 66/66 通过，见 [node-targeted.log](history/reports/r213/node-targeted.log)。独立代码复核两组均未发现确定缺陷；复核者分别运行 R213/R204 14/14、R213/R129 18/18 通过。
- Go R213 诊断白名单与全事件登记检查通过，1.167 秒，见 [go-targeted.log](history/reports/r213/go-targeted.log)。新增 Go 回归验证请求身份不进入日志、未知聚合键丢弃、负值/上限裁剪。
- 两个临时源文件变异分别放行后台进度、重新解析已挪入的图片，均触发行为断言 FAIL（非语法错误），见 [mutation-background.log](history/reports/r213/mutation-background.log)、[mutation-images.log](history/reports/r213/mutation-images.log)。正式源文件未写入变异。
- 首次前端全量 913 项、912 通过、1 失败，失败仅为 R91 addendum 提取夹具缺 `preserveLiveImages`；补齐真实依赖后 `node --test backend/web/*.test.cjs` 最终 **913/913 通过、0 跳过、0 失败**（14.466 秒），见 [node-final.log](history/reports/r213/node-final.log)。原始结果保留在 [node-full.log](history/reports/r213/node-full.log)。
- `go test ./backend -count=1` 全量通过（258.224 秒），见 [go-full.log](history/reports/r213/go-full.log)；`go vet ./backend` 通过，见 [go-vet.log](history/reports/r213/go-vet.log)。最终 `git diff --check` 通过；结果记录在 [verification.json](history/reports/r213/verification.json)。

## 验收边界

未构建、未发布，版本仍为 0.12.71。工单的验证性打包因用户禁令不执行，key mode/产物名称无新条目。恢复构建后再与 R211/R212 合并；用户在单双排或灵活组排选人详情页停留两分钟、换一次英雄，录屏并导出日志，核对标签/段位/图片无闪烁、后台 progress 重绘为 0、完整重建 images_recreated 接近 0。自动 DOM 测试不替代这项真机验收。

## R215 补充复核（2026-10-05）

完整重建挪入已加载图片后才 stamp，会使行签名包含图片队列属性，下一次单行变化误替换全部行。R215 在完整重建、非 insight 面板与对线槽三条路径移动图片前记录干净签名，保留图片对象复用。新增模拟加载属性的组合回归，见 [R215 账本](r215-execution-ledger.md)；此前 R213 的自动验证结果保留当时语境，真机验收与 R215 合并进行。
