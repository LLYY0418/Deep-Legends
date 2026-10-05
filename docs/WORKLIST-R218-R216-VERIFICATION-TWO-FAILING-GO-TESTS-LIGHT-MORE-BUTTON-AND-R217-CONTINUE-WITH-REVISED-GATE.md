# WORKLIST-R218：R216 复核——两个 Go 测试失败（账本却写全量通过）、浅色主题“显示更多对位”按钮看不清；R217 改门槛后继续执行 P2 / P3

诊断人：Claude（复核 R216 收尾与 R217 P1：读源码；在独立环境跑 Go 全量 / vet；在用户机器跑 Node 全量；独立重算 R217 研究数据；查看 R216 截图）。执行人：GPT。日期：2026-10-05。基线：0.12.71 + R211～R217 未提交工作区。构建 / 发布按当前暂停指示。

---

## 复核结果（已确认没问题，不用再动）

- **v2.1 已上线**：`match_score.go` 里 `currentScoreParams = v21ScoreParams`（.4，[.36,.26,.16,.02,.18,.02]），`match_score_computed` 记 `v2.1`。前端 `computeMatchScores` 已删除。
- **缓存版本**：
  - 对局详情 `riot-match-v4`；
  - 赛季 schema 12；
  - 标签持久化 schema 2。
- **关键词关闭**：
  - 后端 `disabledMatchKeywords` 在计算和读盘两处都过滤。
  - 前端只渲染 unstoppable / resilience / unlucky / struggling。
- **R216 冻结文件**：评估结果、候选、关键词阈值的 hash 均未变。
- **Node**：`backend/web` 全量 920 / 920 通过（用户机器）。
- **Go**：`go vet ./backend` 通过，`gofmt` 无输出。
- **R217 P1**：Claude 独立重算，数字与账本完全一致。
  - 560 局、5,600 人；
  - 查表 96.38%；
  - left 98.66%、right 99.27%、last 87.52%。
- **标签截图**：MVP / SVP、2～5 杀徽章、4 个关键词、零阵亡在 780px 下排成一行，不挤压装备格。
- **英雄表截图**：深色 1280 列齐全；所有英雄行、默认展开、对位缩进、sticky 英雄列正常。

---

## P1　`TestR168LiveHistoryWindowFindsOlderSameQueueGames` 每次必失败

### 现象

```
gameplay_test.go:4896: unexpected live history window: /riotclient/command-line-args
state="ok" ... recent=6 requests=2
```

两个子用例都失败，每次运行都失败，不是偶发。

### 根因

R216 在 `loadLiveLCUMatches`（`gameplay.go` 约 8525 行）开头加了：

```go
if reference.ServerID == "" && reference.Region == "" {
    reference.ServerID = clientTencentServerID(client)
}
```

`clientTencentServerID` → `client.platformInfo()`。客户端还没探测过大区时，会请求一次 `/riotclient/command-line-args`。测试桩把所有请求都当成历史请求计数，并断言请求数为 1，所以失败。

生产行为本身没问题：每个 `LCUClient` 只探测一次，之后走缓存。

### 修改

1. 不改生产逻辑，改测试：
   - 桩按路径区分。`/riotclient/command-line-args` 返回 `[]`，不计入历史请求。
   - 历史请求单独计数，仍断言为 1，`begIndex=0`、`endIndex=29` 的检查不变。
2. 加一个断言：同一个 client 连续调用两次，`command-line-args` 只被请求 1 次，证明探测有缓存。

---

## P2　`TestR211SGPForceSummaryFreshAndAllExpectedReusesFirstPage` 偶发失败（30 次失败 22 次）

### 现象

```
r211_test.go:40: all verification fetched an extra page: initial=3 new=4
```

`go test -count=30 -run '^TestR211SGPForceSummaryFreshAndAllExpectedReusesFirstPage$' ./backend`：22 次失败，8 次通过。

### 根因

R214 P3 让强制刷新同时触发一次不走缓存的赛季头部后台扫描，这是有意的行为。这个后台 goroutine 的 SGP 请求和前台请求共用同一个计数器 `calls`。它什么时候落在测试的“前后差值”窗口里，取决于调度，所以前台请求数时而 3、时而 4。

### 修改

1. 测试只统计**前台总览**的请求。可以按请求路径或标记区分，或者在测量前等后台赛季扫描完成（用已有的完成信号，不用 sleep）。
2. 不得放宽 `sgp_history_cache_hits == 0` 断言，也不得删掉“expectGameId 不多拉一页”的检查。
3. 另加一条断言，证明强制刷新确实触发了一次后台赛季扫描，避免为了让测试变稳把这个行为测丢。
4. 验收：`-count=100` 全部通过，`-race -count=20` 全部通过。

---

## P3　账本的“Go 全量通过”在当前代码上复现不了

R216 账本收尾写的是 `go test ./backend：通过`，但当前工作区 P1 必失败、P2 大概率失败。可能是跑全量之后又改了代码，或者用了 go test 的结果缓存。

规则（写进 `docs/WORKLIST-INDEX.md` 顶部的执行约定）：
- 账本里写“Go 全量通过”，必须是**最后一次改动 Go 代码之后**运行的 `go test -count=1 ./backend`。
- 同时记录运行时间、耗时和输出最后 5 行。
- 改完代码后没有重跑，只能写“未重跑”。

---

## P4　浅色主题下英雄表的“显示更多对位”按钮几乎看不清

GPT 自己的截图 `table-light-780.png` 里，这个按钮是灰底灰字，基本读不出来。深色主题正常。

修改：
- 按钮在对位行的 `surface-strong` 底色上，浅色和深色两种主题都要满足文字对比度 ≥ 4.5:1。
- 修好后重新截 `table-light-780.png` / `table-light-1280.png`，并测出对比度写进 `layout.json`。

---

## P5　R217：用户同意改门槛后，继续 P2 / P3

### 为什么 “last ≥ 95%” 这个门槛不合适

R217 P1 的三项各 ≥ 95% 是中间门槛，用来判断规则能不能复现。Claude 补算了工单没要求的**端到端**结果：用 GPT 选出的 left / right / last 规则和查表，从 OP.GG **自己的**曲线推关键词，整体能对 **85.98%**。各关键词的 precision 如下：

| 关键词 | precision | 关键词 | precision |
|---|---:|---|---:|
| 势不可挡 | 99.1% | 大器晚成 | 88.5% |
| 不走运 | 98.7% | 平凡 | 87.6% |
| 坚韧 | 96.7% | 过山车 | 87.3% |
| 胜者 | 93.6% | 奉献 | 81.8% |
| 不屈之志 | 92.8% | 挣扎 | 77.9% |
| 下坡路 | 83.6% | 领袖 | 54.7% |
| 慢热 | 45.5% | 竭尽全力 | 30.0% |

- last 的误差 94% 是“一般 / 好”互判，主要影响“挣扎 / 不屈之志”这一对。
- GPT 搜了 1,673 种 last 定义，Claude 另试 15 种，最高都停在 86%～87.5%。OP.GG 很可能用了更细的内部数据，继续搜收益很小。
- 结论：last 不是瓶颈。真正决定效果的是我们的曲线和 OP.GG 的曲线有多像，也就是 R217 的 P2。

### 执行

1. **冻结 R217 P1 的研究结果**，不再搜索：
   - 查表；
   - left / right 的分段中位数规则；
   - 四档 last 规则；
   - 势不可挡 / 领袖的消歧规则。

   文件：`docs/history/reports/r217/rule-freeze.json`，记 SHA-256。
2. **按 R217 原文执行 P2 和 P3**，以下几处修改：
   - **阈值要换到我们自己的分数尺度**（重要）：
     - OP.GG 的 left / right 本质是“这半段的中位数是否 ≥ 5”，5 是 OP.GG 的持平分。
     - v2.1 的持平分是 **6.0**（每项 `x = 基准` 时贡献一半，`2 + 8 × 0.5`），不是 5。
     - 所以 left / right 的阈值、last 中 `<5` / `≥5` 的判断，都要在训练数据上用按账号分组的交叉验证重新选，或者用持平点 6.0 作为起点。不得直接照抄 5.0。
     - 结尾窗口的 -0.28 同理。
   - P2 第 3 条报告里加一行：用我们的曲线算出的**端到端**关键词准确率，与上表“OP.GG 自己曲线”的上限并排对比。
   - 领袖（54.7%）、慢热（45.5%）、竭尽全力（30.0%）在 OP.GG 自己的曲线上都过不了 P3 的 50% 线，可以预期不会打开，照常进 P3 评估，不提前删除。
3. P3 的上线规则不变：留出集 precision ≥ 50% 才打开。P3 采集后停下，由 Claude 一次性评估。

---

## 收尾

1. 最后一次改代码之后跑：
   - `go test -count=1 ./backend` 全量；
   - `go vet`；
   - P1 / P2 的 `-count=100` 和 `-race`；
   - `backend/web` Node 全量；
   - `desktop/overview-render.test.cjs`。

   按 P3 的规则记录。
2. 账本：
   - P1～P4 写进 `docs/r218-execution-ledger.md`；
   - P5 写进 `docs/r217-execution-ledger.md` 的后续小节。
3. `docs/WORKLIST-INDEX.md`：
   - 加 R218 行；
   - R217 行状态改为“P1 完成，门槛按 R218 P5 修订，继续 P2”。

## 用户真机验收

浅色主题打开英雄数据表，展开一个英雄：“显示更多对位”按钮清楚可读。
