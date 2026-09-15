# WORKLIST-R91-ADDENDUM — 加载页面分组 + 额度提示必须完全静默

诊断人：Claude（只读诊断，未改仓库任何代码文件）。执行人：GPT。
基于用户追问补充两点，**不新增其余诊断**，其余 P1~P9 仍以 `WORKLIST-R91-KR-QUOTA-ARENA-BAN-FLICKER-GROUPING.md` 为准。

---

## 口径决定

1. **加载页面（GameStart）这轮就要开始取小队数据，不是等下一轮再定。**
   `applyArenaLiveGrouping` 的调用条件本轮必须扩大到 `GameStart || InProgress || Reconnect`
   （见 P6-ADDENDUM 第 1 条），这是要落地的代码改动，不是等日志才决定要不要做的选项。
   本次日志样本不够，只影响"扩大后效果好不好"这个问题的答案，不影响"要不要扩大"这个决定——
   后者已经定了。配套的埋点补全（第 2~3 条）是为了让下一份日志能直接看到效果。
2. **额度限流必须做到用户完全看不见**：不是"降低出现频率"，是**这一类错误不得进入任何
   `notice is-warning` / `showToast` / 红黄横幅**，一律转成静默的后台自动重试。

---

## P6-ADDENDUM · 为什么加载页面（GameStart）没有分组

### 代码事实（不是本次日志能证明的，是读代码看到的硬事实）

`gameplay.go` 里三处条件把分组尝试**完全限定在 `phase == "InProgress" || phase == "Reconnect"`**：

```go
if arenaMode && (phase == "InProgress" || phase == "Reconnect") {
    ... a.applyArenaLiveGrouping(...)
}
```

**`GameStart` 阶段被彻底排除在外**——不只是"probe 失败了才不分组"，是**代码根本没有走到分组这一步**。
`GameStart` 在你截图里对应的就是"进入游戏加载页面"那个阶段（客户端显示英雄头像+进度条的那几秒到几十秒）。

### 这次日志能看到什么、看不到什么

本次唯一一局斗魂里，`GameStart` 只持续了 **约 1 秒**（12:36:35 → 12:36:36 立刻变成 `InProgress`），
然后你就掉线进了 `Reconnect`。这段时间太短，日志里连一条 `lcu_gameflow_session_shape` 都没能
在 `GameStart` 阶段捕获到（现有埋点是按"阶段+模式+队列"去重的，短短 1 秒可能被别的阶段占了名额）。

**所以我现在没法预判"加载页面阶段能拿到几成完整名单"——但这不影响下面的改动要不要做，
只影响改完之后实际命中率有多高。这轮直接落地，用你真实打的这一局验证效果。**
你在客户端上看到的"加载页面"，从我们后端的角度看很可能持续了不止 1 秒（真实对局里，
从选人结束到 `InProgress` 之间的 `GameStart` 通常有 10~30 秒），只是这次因为很快掉线，
样本不够。

### 要做的（本轮落地，不是留到下一轮）

1. **把分组尝试的阶段从 `InProgress || Reconnect` 扩大到 `GameStart || InProgress || Reconnect`。**
   `GameStart` 阶段大概率还连不上 2999 端口（游戏进程没起来），所以这个阶段应该走
   **纯 `session-order` 兜底**（`arenaSessionOrderGroups`，只需要 LCU gameflow 的名单顺序，
   不依赖 2999 端口）——这条路径本来就不需要游戏进程，没道理只在 `InProgress` 才用。
2. **新增一条 `lcu_gameflow_session_shape` 强制采样**：在 `GameStart` 阶段，只要收到一次
   session 数据就立刻记一条（不要被现有的按阶段去重键挡掉，`GameStart` 单独给一次机会），
   这样下一份日志能直接看到"加载页面这一刻，18 人的 puuid/teamParticipantId 到底齐不齐"。
3. **`arena_group_order_rejected` 的 reason 要加一条 `phase-not-attempted`**，
   用于统计"多少次是根本没试就结束了这个阶段"——这次的情况就该记成这个，而不是空白。
4. **如果 `GameStart` 阶段名单不完整（比如又是 17/18）**，允许在进入 `InProgress` 后
   用同一套 session-order 结果**继续沿用**，不需要从头重算，减少"进游戏那一下"的空窗。

### 下一步

等你打完真实一局、导出完整日志后，重点看这三样：
`GameStart` 阶段的 `lcu_gameflow_session_shape`（人数、字段是否齐）、
是否出现了 `arena_group_order_rejected`（reason 是什么）、
最终 `live_roster_shape` 的 `arena_grouped` 是不是在 `GameStart` 或稍后的 `InProgress` 就已经为 `true`。

---

## P1-ADDENDUM · 额度限流不得再有任何用户可见提示（完全静默）

### 现状：提示是怎么冒出来的（两条独立路径，都要堵）

`web/gameplay.js` 里有自己的 `api()` 封装（约 185~245 行），HTTP 非 2xx 时
`error.status = response.status`，但**没有读取后端已经下发的 `Retry-After` 响应头**
（后端 `riot_api.go:1465 writeRiotHTTPError` 确实写了这个头，只是前端没用）。

这个 `error` 之后在两处被直接渲染成横幅：

1. **首屏加载**（`loadOverview`，约 816 行）：
   ```js
   else if (quiet && tab.initialPagePending && tab.data) tab.initialPageError = error.message || "请求失败";
   ```
   → 渲染成 `<div class="notice is-warning" role="alert">已保留前 N 场，完整战绩暂未补齐：…</div>`
   （截图一那条）。
2. **翻页加载更多**（同函数 `append` 分支，约 800~810 行）：
   ```js
   tab.data.pagination = { ...tab.data.pagination, hasMore: true, autoPaused: true, pauseReason: "", moreError: retryCopy };
   showToast(`更多战绩加载失败：${error.message}`);
   ```
   → 渲染成 `pagination.moreError`（截图四"自动加载已暂停"）**外加**一个 toast，双重可见。

两处都是"只要 `error.message` 非空就展示"，没有对"这是本地限流,不是真的错误"做任何区分。

### 要做的

1. **后端**：`writeRiotHTTPError` 在设置 `Retry-After` 头的同时，
   **给 JSON 错误体也带上机器可读字段**（例如 `{"error":"...", "retryAfter":40, "kind":"rate-limited"}`，
   不要只塞纯文本 `http.Error`），前端才能可靠区分"这是限流"还是"这是真失败"。
2. **前端 `web/gameplay.js` 的 `api()`**：把 `Retry-After` 头（或上面新加的 JSON 字段）
   解析进 `error.retryAfter`，并给 `error.errorKind` 打上 `"rate-limited"` 标记。
3. **`loadOverview` 的两处 catch 分支都要新增短路**：
   ```js
   if (error.errorKind === "rate-limited") {
     // 不写 tab.initialPageError / pagination.moreError，不 showToast
     const delay = Math.max(1000, Number(error.retryAfter || 5) * 1000);
     setTimeout(() => void loadOverview(tab, false, quiet, append), delay);
     return false;
   }
   ```
   即：**保留已加载的数据原样展示，不出现任何横幅、不出现任何 toast**，
   到点后自己悄悄重试；重试成功就无缝把新数据接上，用户全程无感。
4. **`append`（翻页）路径同理**：限流时不要把 `pagination.autoPaused` 置 true 也不要写
   `moreError`，用同样的静默定时重试；只有重试还是失败**且原因不是限流**时，才走回现有的
   "自动加载已暂停"提示（这部分保留给真实错误用）。
5. **别忘了 P1 主工单里的根因项**（合并 5+20 两轮请求、缓存三态埋点）依然要做——
   这条 ADDENDUM 只解决"用户看不看得到提示"，请求量本身还是要降，
   否则静默重试会更频繁地在后台发生（用户虽然看不见，但仍然是浪费）。

### 验收判据（变异测试）

1. 构造后端返回 429 + `Retry-After: 5`，断言 `loadOverview` 之后 `tab.initialPageError` 为空、
   没有调用 `showToast`，且 5 秒后（fake timer）自动发起了第二次请求。
   把"短路 return”删掉必须变红（重新出现 `initialPageError`）。
2. 构造后端返回一个**非限流**的真实 500 错误，断言**依然**走原有的
   `tab.initialPageError` / `pagination.moreError` 路径（防止把这条也一起静音）。
3. 翻页场景同样两条：429 静默重试 / 真实错误仍提示。

---

## 本 ADDENDUM 明确不做的事

- 不在这次改动里把 `GameStart` 阶段的分组算法写死结论——等真实日志。
- 不改 P1 主工单里"合并请求/缓存三态"之外的限流数值（15/秒、90/2分钟不动）。
- 不改 503（本地过载）的现有提示逻辑，只隔离 429（本地配额限流）这一类。
