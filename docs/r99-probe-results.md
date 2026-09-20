# R99 / R101 / R105 客户端探测记录

## R105 最新证据（2026-09-17 20:38）

来自用户工单引用的 `lol-loot-diagnostics-0917-2038.jsonl`：W5 未拥有头像写入 **201 / ok=true**，W5-restore **201 / ok=true**；W6 切换旗帜 **200 / ok=true**，W6-restore **200 / ok=true**。此处归档的是工单提供的真机结果，本次 macOS 环境未取得该原始日志、未重新执行 Windows LCU 请求。

据此 R105 正式开放全部目录头像、旗帜的点击写入；拥有态不再限制选择。头像 PUT 必须返回201并回读确认，旗帜 PATCH 必须返回200并回读确认。旗帜从 GET 原槽位复制所有字段（含不透明 data），仅替换 contentId/inventoryType/itemId。非预期状态显示错误并保留选择器。R99/R101 探测代码保留。

以下为 R101 当时的历史证据和判断；“待测”“只读”及当时的401结论已被本节更新，不作为当前版本门禁。

2026-09-17 更新：R101 工单提供了 Windows 国服真机日志 `lol-loot-diagnostics-0916-2155.jsonl`（1824 行，build `8944d5e33f9c`，run `364cbd108cf990db4c7b93a1`）的审阅结果。下表按工单归档；本次 macOS 实现环境没有重新执行这些真机请求。此前“全部待测”的记录已被这些证据替代。

| 项目 | 真机结果（工单提供） | 当前结论 |
| --- | --- | --- |
| R1 头像目录 | 200；5099 条，title 非空 5099；yearReleased/isLegacy/imagePath/rarities/disabledRegions | 可读取目录；有效图片子集与总数分开计数 |
| R2 头像系列 | 200；81 条；id/displayName/description/icons/hidden | 使用 displayName + icons |
| R3 regalia | 200；139 条，可选 47；类型 kBanner/kNone；ID 字符串；TencentOnly 3 | 排除 id 1 和 id 2 的 11 个段位变体，旗帜 35 面 |
| R4 头像库存 | 200；508 条，owned=true 478；ownershipType=OWNED | 拥有态门禁解除；只投影 owned |
| R5 regalia/v3 旗帜库存 | 200；37 个数字键，isOwned=true 4；值有 isOwned/items/purchaseDate | 唯一旗帜拥有态来源；键对应 regalia ID |
| R6 regalia/v3 crest | 200；空对象 | 本轮不实现 crest |
| R7 regalia/v3 border | 400 unsupported | 永久排除，停止探测 |
| R8 inventory/v2 旗帜库存 | 200；数组 4 条，owned=true 0 | 禁止用于拥有态，停止探测 |
| R9 account loadouts | 200；1 条 ACCOUNT；存在 REGALIA_BANNER_SLOT；contentId/data/inventoryType/itemId | 试写复制完整槽位并保留 data |
| R10 regalia/v2 | 200；bannerType/preferredBannerType=blank，crest/preferredCrest=prestige | 段位旗沿用已验证枚举 |
| R11 flags / flags equipped / frames equipped | 200 空数组 / 404 / 404 | 关闭整个 /lol-banners 备选链路，不再探测 |
| W1 当前头像原值回写 | 201 ok | 只证明原值被接受，不能证明新头像可切换 |
| W2 当前完整旗帜槽位 | 200 ok | 只证明原样回写被接受 |
| W3 pair / content / all | 各 200；三次 restore 各 200 | 同样没有新值证据；不可据此开放旗帜写入 |
| W4 相邻挑战配色 | 400，changed=false；restore 204 | 删除配色入口及 previous-banner；不再执行 W4 |
| W5 已拥有且非当前头像 | **待真机运行新探测** | 201 才支持拥有校验解释；401 按 W5-b → W5-c 顺序继续判别 |
| W6 已拥有且非当前旗帜 | **待真机运行新探测** | 必须 PATCH 后 GET 回读 changed=true；200 本身不够 |

## 401 与恢复语义

七次实际头像切换均为 401/RPC_ERROR。同会话段位旗、生涯背景、loadout 请求成功。W1 的 201 与聚合日志不矛盾：LCU 请求桶在下一次同 key 请求到新窗口时惰性 flush；W1 触发上一桶输出，自身桶没有后续请求触发输出。没有证据否定 W1。

W5/W6 由「展示清理 → 检测客户端写入支持」手动触发，按钮明确说明会临时切换已拥有项目再恢复。W5 每次尝试用独立超时恢复；W5-b 的恢复若被拒，回退到已验证原值 PUT 恢复。恢复失败停止后续试写。W6 回读相同 loadout ID 的槽位，再完整恢复原 slot，所有试写后的失败/取消分支均会尝试恢复。`restored` 只表示恢复请求成功，不伪称恢复已回读确认。

W5-c 先读取 v2/v1 signedInventory 的状态和顶层字段；目前没有真实 token 响应 schema，**不猜 token 格式、不发送带猜测 token 的请求**。它会记录 skipped，待真实结构证据后再补该分支，不能提前判“三种途径都失败”。头像原正式 PUT 路径未调整；旗帜无正式写入接口，仅开放目录与拥有态浏览。

## 诊断与数据边界

诊断只记录结构、数量、有限枚举、HTTP 状态、ok/changed/restored，不记录头像目标 ID、槽位内容、玩家标识或 token 值。标识字段名使用 R99 的 stable-ref/player-ref/account-ref/opaque-ref/content-ref/item-ref/purchase-time/display-name 别名。历史只读编号保持 1…13 的含义，但 R7/R8/R11 不再请求。

公开头像目录的 5099 是总条数；86 条空图片及 ID 0 被排除后，可展示图片的子集是 5012。前端计数使用原始 catalog total，不能把有效图片子集冒充总目录。拥有库存失败时保留完整可展示目录、标记 unavailable，禁止误把未知当已拥有。
