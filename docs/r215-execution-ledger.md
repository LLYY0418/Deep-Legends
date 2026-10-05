# R215 执行账本

日期：2026-10-05。基线 0.12.71；范围见 [R215 工单](WORKLIST-R215-R213-VERIFICATION-FULL-REBUILD-ROW-MARKUP-STAMPED-AFTER-IMAGE-MOVE.md)。本次不构建、不发布、不增版本，保留工作区 R211～R214 改动。

## 复现与修复

先加回归测试、再改代码。真实修前源码运行四条 R215 测试：无完整重建的对照通过；完整重建后段位只改一人却 `rowsReplaced=4`（期望 1）；符文行被替换；lane 槽签名含 `src` 和 `data-image-ready`。三条行为断言失败、没有语法/夹具错误，见 [node-before-fix.log](history/reports/r215/node-before-fix.log)。

R213 的原测试未覆盖“所有图片加载完成 → 完整重建 → 下一次单行变化”的组合。这次用例手动写入 `src`、`data-image-ready` 并删除 `data-queued-src`，不依赖空的 `prepareImages` 桩模拟加载。

只在 `backend/web/gameplay.js` 增加三处前置标记：

- `fullHolder.innerHTML = markup` 后先 `stampLiveRows(fullHolder)`，再移动旧图片。
- 对线卡槽 `nextSlot` 先 stamp，再 `preserveLiveImages`。
- 非 insight 面板 `next` 先 stamp，再比较/复用符文行及移动图片。

`stampLiveRows` 仍只在 `_liveMarkup` 未记录时写入。移动后原有 stamp 调用保留，不会覆盖干净签名。没有改比较规则、请求快照规则、诊断事件、界面文案或图片复用方式。

## 自动验证

- `backend/web/r213.test.cjs` 增加四条 R215 用例：完整重建后只替换一名玩家；无重建对照；符文行在前一轮更新后下一轮保持同一对象；lane 槽干净签名及二次 stamp 不覆盖。加载后的图片仍是原对象，`imagesRecreated=0`，其余三名玩家行节点保持不变。
- R213/R204/R129/R179 定向 **39/39 通过**，见 [node-targeted.log](history/reports/r215/node-targeted.log)。独立只读复核三条路径及回归覆盖未发现问题，单独 R213 文件 **11/11 通过**。
- 前端全量 `node --test backend/web/*.test.cjs` **919/919 通过、0 失败、0 跳过**（16.485 秒），见 [node-full.log](history/reports/r215/node-full.log)；后端全量 `go test ./backend -count=1` **通过**（261.440 秒），见 [go-full.log](history/reports/r215/go-full.log)。最终 `git diff --check` 通过；汇总见 [verification.json](history/reports/r215/verification.json)，本次生产代码仅三处前置 stamp 加一行注释，见 [gameplay-change.patch](history/reports/r215/gameplay-change.patch)。

## 真机边界

未构建、未发布，版本仍为 0.12.71。与 R213 合并验收：允许构建后进入选人详情页，本人换英雄触发完整重建，再让队友战绩/英雄变化，检查本窗口 `rows_replaced` 只累加真正变化的行，组队悬停和图片不因未变行替换中断。自动测试不替代真机录屏验收。
