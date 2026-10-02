# WORKLIST-R121：位置契约探测真机首次运行——`/help` 兜底扫描实际不可用，结论仍未定（不要删探测代码）

**撰写日期：** 2026-09-22　**基线版本：** 0.12.15（工作区未提交状态）
**触发：** 用户在真实 WeGame 客户端上运行了交叉编译的独立 Windows 探测二进制（`TestR121PositionContractProbeLiveClient`），
提交了完整的 `r121-position-probe-output.jsonl`（5 条事件，`trace_id=e361d1b420373b6c57aaa06c`）。
**本轮性质：** 对真机产出的诊断日志做逐行解读 + 对 `backend/position_contract_probe.go` 的兜底扫描逻辑做代码审查，未改动仓库任何文件。

## 0. 结论

真机运行**本身完全成功、安全边界完整**：客户端自动发现生效、三个来源全部只发了 GET、`write_executed` 全程为 `false`、日志里没有任何响应体取值——这些都符合设计。

但**核心问题没有被回答**：`docs/r121-position-probe-findings.md` 要的是「本机 LCU 契约里到底有没有位置偏好/补位字段」，这次真机运行给出的是**不可判定（inconclusive）**，而且探测代码自己在事件里明确标注了这一点（`negative_conclusive_all: false`）——**不是「查了、确实没有」，是「压根没查到」**。原因是两个契约来源（`/swagger/v3/openapi.json`、`/swagger/v2/swagger.json`）双双 404，兜底用的 `/help?format=Full`（200，3,022,367 字节）交给纯文本路径扫描后，**在 3MB 的响应体里只找到 1 个疑似路径 token，且这 1 个还不匹配我们关心的五个命名空间**（`matched_path_count: 0`）。

这不合理：这份 Deep Legends 代码库自己在别处（`gameplay.go`、`champselect.go` 等）大量调用 `/lol-champ-select/v1/session`、`/lol-lobby/v2/lobby` 这类端点，说明当前客户端上这些端点必然存在；`/help?format=Full` 的 3MB 响应体里却几乎扫不出任何 `/xxx/yyy` 形状的字符串，唯一合理的解释是：**`positionProbeTextPaths` 依赖的假设——"`/help?format=Full` 的响应体里会以字面 `/段/段` 形式包含路径字符串"——对当前客户端版本不成立**（详见 §1 根因分析）。也就是说这个兜底扫描从一开始就没有真正扫到过东西，`/help` 这条来源等于白跑。

**按标准约定处理：不删除 `backend/position_contract_probe.go` 与 `position_contract_probe_test.go`**——`docs/r121-position-probe-findings.md` §4 的删除前提是"结论已写入"，但这次结论是"没查到"，不是"查到了、确实没有"，删了代码就再也补不了这次真机验证的窟窿。

## 执行纪律

- 本工单不涉及生产代码，只涉及一次性侦察代码（`position_contract_probe.go`），修复方向仍要遵守其文件头写明的安全边界：只读 GET、`write_executed` 恒 `false`、只记录契约形状不记录响应值。
- 修复后必须能在**不依赖真实 LCU 客户端**的前提下用合成 fixture 证明"扫描逻辑本身有效"（构造一个已知包含 `/lol-champ-select/v1/session` 之类路径的假响应体，断言能扫出来），再排真机复测去验证"这次真的连上了、真的读到了目标端点"。
- 变异测试可选（这是诊断代码而非生产逻辑），但新增的合成 fixture 测试必须能在改回旧实现时 FAIL，证明测试真的在测东西。

---

## 真机运行原始记录（逐行解读，`trace_id=e361d1b420373b6c57aaa06c`）

| # | 来源 | 状态 | 结果 | 关键字段 |
|---|---|---|---|---|
| 1 | `/swagger/v3/openapi.json` | 404 | `unavailable` | `contract_read=false`，`negative_conclusive=false` |
| 2 | `/swagger/v2/swagger.json` | 404 | `unavailable` | 同上 |
| 3 | `/help?format=Full` | 200 | `ok` | `body_bytes=3022367`，`contract_read=true`（能拿到响应），但走的是**文本兜底分支**：`paths_scanned`(=扫描到的 token 总数)=**1**，`unique_paths=1`，`matched_path_count=0`，`matched_paths=[]` |
| 汇总 | — | — | — | `contract_read_any=true`，`contract_read_structured=false`，`negative_conclusive_all=false` |

安全边界确认（全部符合预期，无需跟进）：三条事件 `write_executed` 全为 `false`；没有任何字段携带响应体取值、召唤师名或对局数据；`temporary_reconnaissance_code=true` 全程带着。前两个来源 404 与 R82 此前的真机观测一致，不是新问题。

## 1. 根因分析：`/help?format=Full` 的文本兜底扫描对当前客户端版本形同虚设

`backend/position_contract_probe.go:373-398`（`positionProbeTextPaths`）的假设写在注释里（40-47 行）：

> "只要形如「/段/段…」的绝对路径 token 都收集" ——这个假设来自 R82 观测"swagger 两个来源双双 404、`/help?format=Full` 返回 200"，但 **R82 只验证了"返回 200"，没有验证"响应体里真的含有字面路径字符串"**。

真机这次的结果是反证：3,022,367 字节的响应体里，`positionProbeTextPath` 这个正则（`/[A-Za-z0-9._{}:-]+(?:/[A-Za-z0-9._{}:-]+)+`）总共只匹配到 **1 次**。已知这台客户端一定支持 `/lol-champ-select/v1/session` 等端点（本仓库其他生产代码天天调它们），如果 `/help?format=Full` 的响应体真的以字面 `"/lol-champ-select/v1/session"` 这种斜杠分隔字符串描述端点，3MB 的文档里不可能只匹配到 1 次。

最可能的解释（LCU `/help` 端点是老牌 RPC 反射接口，公开的第三方 LCU 逆向资料里早有记录）：现在这台客户端的 `/help?format=Full` 输出的是 **驼峰式 RPC 函数/类型名**（例如形如 `OnJsonApiEvent_lol_champ_select_v1_session` 或纯类型名 `LolChampSelectV1Session` 这类，不含裸的 `/` 分隔路径字符串），而不是字面的 REST 路径字符串。`positionProbeTextPaths` 用"扫字面斜杠路径"的策略去解析这种格式，天然扫不出东西——这不是"客户端没有这些端点"，是"探测代码找错了模式"。

（受探测代码自身安全边界所限，这次真机日志里不允许、也没有记录 `/help` 响应体的实际内容取值，所以以上是基于代码审查 + 已知 LCU 生态的推断，不是直接证据；真正确认需要下面的修复方案。）

---

# P1

## P1-1　放弃对 `/help` 做格式不可知的文本扫描，改成对已知端点命名空间直接发请求 + 扫响应 JSON 的键名

**修复方向：**

不再依赖任何"契约清单"文档（openapi 或 `/help`）——两个 openapi 来源已知 404，`/help` 格式对不上假设——而是直接对工单点名的五个命名空间下、代码库里已经在用或已知存在的具体端点发 GET，然后**只扫响应 JSON 顶层与一层子对象的键名**（不进值），用 `positionProbeProperty` 谓词去匹配。候选端点建议至少覆盖：

- `/lol-champ-select/v1/session`（进 champ select 时才有效，可能 404，属预期）
- `/lol-lobby/v2/lobby`
- `/lol-gameflow/v1/gameflow-phase`
- `/lol-gameflow/v1/session`
- `/lol-match-history/v1/products/lol/current-summoner/matches`
- `/lol-end-of-game/v1/eog-stats-block`（局外才有效，可能 404，属预期）

每个端点各自独立记一条 `result`（`ok` / `unavailable`，404 也算探测完成，不算失败），键名扫描命中即记 `matched_properties`；**只要至少一个端点返回 200 且完成了键名扫描，就能重新定义 `negative_conclusive`**（不再依赖"有没有解析出契约文档"，改成"这个端点的响应我们真的看到了、键名列表里没有目标字段"）。

**验收判据：**
1. 新增一个不依赖真实客户端的合成测试：构造一个假的 `/lol-lobby/v2/lobby` JSON 响应（顶层带一个已知会命中谓词的键，比如 `"positionPreferences"`），断言新的键名扫描逻辑能扫出这个键、记录类型、且不记录其值；同一 fixture 里再放一个不该命中的键，断言不会被记录。
2. 把 `positionProbeProperty` 谓词改回一个必然扫不到东西的假谓词（变异），上面这条合成测试必须 FAIL——证明测试确实在测扫描逻辑本身，不是空转。
3. 台账写明：为什么放弃「解析契约清单」转向「直接测已知端点」（即本工单 §1 的根因），新方案覆盖的端点清单来自哪里（代码库里已经在调用这些端点的位置，逐个给出文件:行号出处）。

## 已复核无需返工（不要重复排查）

- **真机安全边界**：三条事件 `write_executed` 全 `false`，无响应值/召唤师名/对局数据落诊断日志，`temporary_reconnaissance_code` 标记完整——探测代码本身的只读约束在真机上验证有效，这部分不需要重做。
- **openapi v2/v3 两个来源 404**：与 R82 此前真机观测一致，不是新问题，不用再追。
