# WORKLIST-R199：R196 账本写了"已发布 0.12.60 Latest"，但 GitHub 上没有（更正账本，0.12.60 不发布）

诊断人：Claude（公网只读核对）。执行人：GPT。日期：2026-10-03。基线：R196 提交 `e3ba8b6fe58cfb1c188bedaa75ca82c307fe70b4`（0.12.60）。

**用户决定：0.12.60 先不发布。** 先执行 R197、R198，合并成一个版本后再发布（见 R198 收尾）。本工单只更正账本、补发布规则，**不创建、不发布、不修改任何 GitHub Release**。

## 现象

`docs/history/ledgers/r196-execution-ledger.md`「发布与真机边界」写的是"按工单发布 0.12.60 public Release Latest，附安装包、latest.json、SHA256SUMS"。

2026-10-03 15:2x（北京时间）匿名核对：

| 请求 | 结果 |
|---|---|
| `https://github.com/LLYY0418/Deep-Legends/releases/latest/download/latest.json`（两次，带 no-cache 和随机参数） | `"version": "0.12.59"` |
| `https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.60/latest.json` | **404** |

0.12.60 没有正式发布。账本也没有 R195 那样的证据：没有 release id、发布时间、`draft/prerelease/isLatest` 状态、附件 digest，也没有匿名下载 latest.json 的核对。

## 要做的

1. 只读查询 0.12.60 的 Release 实际状态（草稿 / 不存在 / 其他），在 R196 账本里如实写明，不推断原因。
2. 如果存在 0.12.60 的**草稿**：保持原样，不发布、不删除；账本写明草稿 id。如果标签 `v0.12.60` 已推送，保留，不移动、不删除。
3. 更正 R196 账本「发布与真机边界」：删掉"按工单发布 0.12.60 public Release Latest…"这句，改为"0.12.60 未发布（用户决定与 R197、R198 合并后发布）"，附上第 1 步查到的状态。构建和测试结果保留不变。
4. 在 `docs/WORKLIST-INDEX.md` 顶部的执行约定里加一条规则：

   > 任何账本写"已发布"，必须同时有：release id、`isLatest=true` 的查询结果、匿名下载 `releases/latest/download/latest.json` 得到的版本号。缺任何一项，只能写"发布未完成"。

## 收尾

- 不改代码、不递增版本。`docs/WORKLIST-INDEX.md` 加 R199，R196 那行备注"0.12.60 未发布，并入 R197/R198 的版本发布"。账本 `docs/r199-execution-ledger.md`。
