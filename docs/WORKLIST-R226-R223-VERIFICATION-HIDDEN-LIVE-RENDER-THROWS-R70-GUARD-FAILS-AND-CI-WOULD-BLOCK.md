# WORKLIST-R226：R223 复核——隐藏页调用 `renderLive` 会抛 ReferenceError，R70 护栏测试失败，CI 会被它挡住

诊断：Claude（复核 R223 执行结果，重跑测试）。执行：GPT。日期：2026-10-05。基线：0.12.73，工作区含 R222～R224 未提交改动。

## 复核结论

R223 的 P1～P7 我逐项核对过，实现与工单一致（宽容解析、`sgp_game_decode_failed`、1223 立即取消、删除 LeagueClient.exe 候选、外服菜单 8 项、分组改名外服、统计口径文案删除）。前端 `node --test backend/web/*.test.cjs` 我这边重跑 **950/950 通过**。**只有下面这一个问题。**

## 问题：`node --test desktop/refresh-orchestration.test.cjs` 失败 1 项

```
not ok 11 - R70 hidden gameplay pages do not construct DOM, and destroyed pages remain inert
ReferenceError: liveRenderTriggerLabel is not defined
    at renderLive (gameplay.js)
```

- R223 账本已写明「renderer all 1274 项 / 1 失败」，原因是「共享实时对局 `renderLive` 在隐藏页提前调用新增依赖的 R70 护栏」，但没有修，也没有登记到任何工单。CI（`.github/workflows/ci.yml` 的 `node scripts/test-renderers.cjs all`）会跑这一项，**不修的话下一次发布流水线必红**。
- 根因：`backend/web/gameplay.js` 的 `renderLive`（约 5848 行）把
  ```js
  state.liveRenderSource = state.liveRenderTrigger || liveRenderTriggerLabel(source);
  state.liveRenderTrigger = "";
  ```
  放到了函数**第一行**，在 `if (state.destroyed || (state.section && state.section !== "live")) return;` 之前。这是 R224（实时重绘来源诊断）改的，不是 R223。
- R70 约定：页面隐藏（`state.section !== "live"`）或已销毁时，`renderLive` 必须直接返回，不碰任何状态和依赖。这个测试只用 `{ state }` 编译 `renderLive`，所以一调到 `liveRenderTriggerLabel` 就抛。
- 真实页面里不会抛（函数在同一个闭包里），但隐藏页每次重绘都会白白改写 `state.liveRenderSource`，并清空 `state.liveRenderTrigger`，会让下一次真正的实时页重绘丢失触发来源，**R224 想要的诊断就不准了**。

## 修复要求

1. 把这两行移到隐藏/销毁的 early return **之后**。保持 R224 的语义：真正渲染实时页时才记录来源并清空 trigger。
2. 不要为了让测试通过去改 R70 测试注入 `liveRenderTriggerLabel`；测试表达的是「隐藏页不碰任何依赖」，应保持原样。
3. 补一个断言：隐藏页调用 `renderLive` 后，`state.liveRenderTrigger` 保持原值、`state.liveRenderSource` 不变。
4. 重跑：`node --test desktop/refresh-orchestration.test.cjs`，再跑 `node scripts/test-renderers.cjs all`，记录结果。renderer all 里 R223 账本提到的「多个文件超过 90 秒」另行列出是哪些，如果是 CI 的超时限制问题，写进 R222 的耗时记录，不要靠放宽时限处理。

## 另外两点（不单独开工单，随本单顺手确认）

- R223 账本里的前端全量失败（R192 两项挂载超时、R206 100ms 图片队列）我这边重跑都通过，属于并发调度时的耗时抖动。请在账本里确认它们是否在 CI 里也会偶发；如果会，说明哪个文件的并发度要降，**不要放宽断言**。
- 账本里「先前全量 race 另有 R201 探测时限超时」：保留独立复测通过的记录即可。

## 验收

- `node --test desktop/refresh-orchestration.test.cjs` 全通过。
- `node scripts/test-renderers.cjs all` 无失败项（跳过项要写明原因）。
- 新建 `docs/r226-execution-ledger.md`，在 `docs/WORKLIST-INDEX.md` 登记。
