# WORKLIST-R144：账户页「客户端数据暂未同步，可稍后重试」——LCU 返回的空白战利品记录被误标成“暂未同步”

诊断人：Claude（读了 `lol-loot-diagnostics-0924-1019.jsonl`、用户截图、`backend/lcu_api.go` / `collection_reads.go` / `web/app.js`）。
这一轮用户要求「结合日志仔细分析修改」，所以 Claude **已经把代码改完**（清单见 §3）；执行人 GPT 负责真实依赖下的复核、真机验证和发版（§5）。
日期：2026-09-24。
基线：0.12.19，HEAD `e10ad3a3`。工作区里另有 R140 的未提交改动（`web/app.css`、`web/favorites-facade.js`、`web/r130.test.cjs`、`desktop/r140-banner-layout.cjs`），本单一个字都没碰。
状态：代码已改，待 GPT 复核与真机验证。

用户的问题：账户页为什么出现「客户端数据暂未同步，可稍后重试」，是不是读不到？

结论先说：**读得到，是客户端自己返回了一条什么信息都没有的记录，应用没办法知道它是什么。**它不是“同步没完成”：三个不同日期的日志里都有，同一会话里重试三次也原样不变。「可稍后重试」这句话没有依据。

---

## 1. 证据（`lol-loot-diagnostics-0924-1019.jsonl`，版本 0.12.19，单次运行）

- 截图：材料区第一张卡，标题就是这句话，数量 ×74，卡内小字又是同一句，页面顶部提示条还是同一句，一共三处重复。
- `loot_name_fallback`（log_seq 206 / 243 / 279 / 320）：每次刷新恰好一条，`raw_key_empty=true`、`type_empty=true`、`has_loot_name=false`、`has_localized_name=false`、`loot_id_prefix="OTHER"`、`display_categories=""`。这条记录的键、ID、名称、类型全是空的，只剩数量。
- `loot_map_shape`（log_seq 212 / 249 / 285 / 326）：四次完全一致，`raw_entries=42`、`kept=42`、`unnamed_type_counts={"":1}`，其余 41 条都有类型和 ID。
- 刷新时间线（`collection_refresh_request`）：02:15:27.5 → 02:15:34.1 → 02:15:50.6 → 02:16:22.1，间隔 6.6 / 16.5 / 31.4 秒，正是 R87 的 5/15/30 秒三次自动重试。**初次加三次重试读到的是同一条空白记录**，重试预算用完也没变。
- 接口没有失败：`/lol-loot/v1/player-loot-map` 三次都是 200，耗时 6–20ms。名称目录也都加载成功（`loot` 3703、`wards` 1064、`icons` 10214 来自本机客户端，`translations` 342 来自 communitydragon）。`/fe/lol-loot/trans.json` 是 404，这是已知的本机无此文件、走 communitydragon 兜底，与本问题无关。
- 历史上出现过同类记录：`docs/loot-names-0912.md` 记录 09-12 日志里「唯一的 `loot_name_fallback` 是没有类型、没有键和名称的空壳」（log_seq=370）；R87 工单记录 09-13 一次会话里同类无法命名的条目（`raw_key_empty=true`、`OTHER`）。三个不同日期都在，所以不是刚登录时的同步竞态。

## 2. 根因

1. LCU 的战利品表里有一条记录，键、`lootId`、`lootName`、`localizedName`、`type` 全空，只有数量。名称目录都加载了，但这条记录没有任何 ID 可查，应用无从命名，这不是应用漏读。
2. R87 把它归为“暂时未就绪”：后端给它设置 `DataPending=true`，还把 `DisplayName` 写成「客户端数据暂未同步，可稍后重试」，并做 5/15/30 秒三次自动重试。日志证明重试对它无效，用户会一直看到这句话，而且标题、小字、顶部提示条重复三次。
3. 前端 R61 本来就有更准确的兜底文案「客户端返回的空白条目」（`lootName()` 的最后一个分支），但被后端写死的 `DisplayName` 盖住，从来没显示过。
4. 这条记录按空名称排在最前面，还被默认归入“材料”并标上“材料”，等于把不知道的类型断言成材料。

**这条记录到底是什么（数量 74）：本日志回答不了。**诊断按隐私规则只记“字段有没有”，不记取值。本单加了字段名诊断（§3），下一份日志就能回答。

## 3. 已做的修改（Claude，未提交、未打包）

`backend/lcu_api.go`
- `LootItem` 新增 `Blank`（JSON `blank`）。无键、无 ID、无名称、无可用本地化名的记录：`Blank=true`、`Kind="类型未知"`，`DisplayName` 留空（前端自然显示「客户端返回的空白条目」），`DataPending=true` 只在重试期内保留。
- `PlayerLoot()` 改为 `GetBytes` 后逐条解码，行为等价；对空白记录额外取原始 JSON 中**有值的字段名**（只记键名，不记取值）。
- 排序：空白记录排在同区块最后，不再因为空名称排到最前面。
- `loot_name_fallback` 事件对空白记录新增 `field_keys`、`item_status`、`rarity`、`redeemable_status`、`asset`（美术资源路径，用第一个非空的 asset/tile/splash）、`store_item_id_set`。不记数量、不记其他字段取值。

`backend/collection_reads.go`
- 新增 `settleBlankLoot()`。`scheduleCollectionDataRetry` 发现「仍有 pending、没有待触发的重试计时器、三次重试已用完」时，把 pending 记录的 `DataPending` 清掉（返回副本，不原地改共享切片），并写诊断 `collection_data_retry_exhausted{attempts, blank_entries}`（在解锁之后写）。第三次重试还在等计时器时不会提前清除。

`backend/web/app.js`：**没有改**。`DataPending` 清掉后顶部提示条自动消失（`lootPending` 本来就只看它）；卡片标题走 R61 的兜底文案，类型标签用后端的 `kind`。重试期内仍显示提示条和卡内小字，标题不再重复这句话。

界面文案红线：没有新增说明性文字。「客户端返回的空白条目」「类型未知」属于回退提示；提示条只在重试期内出现。

测试：`lcu_api_test.go` 更新 2 处（空白记录的字段与排序断言，并断言 `field_keys` 只含有值的键名、日志里不出现字段取值）；新增 `backend/loot_blank_test.go` 4 条（settle 不改共享切片、重试耗尽后清 pending、计时器未走完不清、`PlayerLoot` 空白记录排最后且只记有值字段名）；`web/champions.test.cjs` 新增 1 条（重试期有提示且标题不重复，settle 后无提示、类型未知、数量保留）。

## 4. Claude 这边的验证边界（GPT 别当作最终验证）

- Go：云端沙箱访问不了 `proxy.golang.org`，第三方依赖（gorilla/websocket、go-pinyin、x/net、x/sys）是用**空实现桩**替换后编译的。`go vet ./backend` 通过；全量 `go test ./backend` 的失败集合与改动前完全一致（12 项，都由桩引起：HTML 解析、websocket、拼音、缺 `desktop/` 目录）；本单新增和改动的 Go 测试全部通过。**这不是真实依赖下的验证。**
- 对抗变异 4 项，均让对应测试 FAIL，并已整文件还原（`diff` 确认）：去掉耗尽后清 pending；去掉空白记录排序；字段名不再过滤空值；把提示句塞回 `DisplayName`。
- Node：在用户电脑上 `node --test backend/web/*.test.cjs` 693/693 通过（含新增 1 条）。

## 5. GPT 要做

1. **真实依赖下跑全量**：`go build -o <tmp> ./backend`、`go vet ./...`、`go test ./...`，与 R143 基线比较，R135 记录的 5 项既有失败不得增加；确认 `loot_blank_test.go` 的 4 条和 `lcu_api_test.go` 更新的 2 处通过。
2. **复核变更范围**：本单只动了 `backend/lcu_api.go`、`backend/collection_reads.go`、`backend/lcu_api_test.go`、`backend/loot_blank_test.go`（新）、`backend/web/champions.test.cjs`。确认与 R140 的未提交改动没有交叉。
3. **发版**：版本号递增，`private` 模式打包（R136 规则：public 产物文件名必须带 `-public`），账本记 key mode、指纹、SHA256。
4. **写账本**：`docs/r144-execution-ledger.md`，并更新 `docs/WORKLIST-INDEX.md`。

用户在真机上做：

- 装新包，打开「账户与物品」。前约 1 分钟：顶部有提示条，那张卡的小字有提示、标题是「客户端返回的空白条目」；约 1 分钟后：提示条消失，那张卡仍在材料区**最后**，类型标签是「类型未知」，数量不变。
- 导出诊断日志发回来，Claude 读 `collection_data_retry_exhausted` 和 `loot_name_fallback` 里的 `field_keys` / `asset` / `item_status`，回填这条记录是什么。
- 不用工具也能先帮忙：在英雄联盟客户端「战利品」里找有没有数量为 74 的物品，把名称告诉 Claude。

## 6. 边界与未解决

- 这条记录是什么：见 §7，已由新日志回填——它是客户端的占位记录，战利品接口给不出名称，没有映射的可能，不另开映射工单。
- R87 的三次重试保留，客户端刚启动时确有可能补全，本次日志没有反证。
- 整张战利品表为空（`player-loot` capability 为 pending）的场景不在本单，本次日志没有出现，行为不变，仍显示「可稍后重试」。

## 7. 新日志回填（`lol-loot-diagnostics-0924-1054.jsonl`，构建指纹 `ed80d9bdd807`，2026-09-24）

这份日志来自已带 R144 诊断的新包（`loot_name_fallback` 里出现了 `field_keys`）。

- **空白记录有值的字段**（`field_keys`，两次刷新完全一致）：`count`、`expiryTime`、`itemStatus`（值 `NONE`）、`parentItemStatus`、`parentStoreItemId`、`redeemableStatus`（值 `NOT_REDEEMABLE`）、`rentalGames`、`rentalSeconds`、`splashPath`、`storeItemId`、`tilePath`、`value`。**没有** `lootId`、`lootName`、`localizedName`、`type`、`rarity`、`displayCategories`、`asset`。
- **图标路径是空名称拼出来的**：日志里的 `asset`（`asset`/`tilePath`/`splashPath` 中第一个非空的）是 `/fe/lol-loot/assets/loot_item_icons/.png`，文件名为空。同一份日志里这个路径被请求并 404（`lcu_request`，02:54:14.498），上一份日志（0924-1019，02:16:48.271）同样如此。
- **`store_item_id_set=false` 但 `storeItemId` 键有值**：说明值非零且不是正数（多半是 -1 之类的“未设置”哨兵值；这是推断，日志按隐私规则不记取值）。`expiryTime`、`parentStoreItemId`、`rentalGames`、`rentalSeconds` 这类字段“有值”很可能也是同类默认值，不代表它是租借物品。
- **结论**：这是客户端的占位记录——没有战利品定义，图标路径由空名称拼出，数值字段像默认值。`/lol-loot/v1/player-loot-map` 给不出它的名称，也没有可用的 ID 去别的目录里查，不需要另开映射工单。显示成「客户端返回的空白条目」是准确的，也符合 `docs/loot-names-0912.md` 的原则：不因名称缺失删除真实库存条目，也不推断它过期、隐藏或错误。数量 74 是不是某个已下线物品的残留计数，只能靠用户在客户端「战利品」里核对。
- **这份日志验证不到 R144 的“重试用完后提示条消失”**：日志只覆盖启动后约 26 秒（02:53:54–02:54:20），只有两次刷新（02:54:01、02:54:07），第 2、3 次重试（+15 秒、+30 秒）和 `collection_data_retry_exhausted` 都还没发生。下次请启动后放置 60 秒以上再导出。

### 顺带小修（GPT 做，一并放进 R144 的发版）

- **空白记录不带任何图片路径**：`enrichLootItemsWithMetadata` 里 `blank` 分支清空 `TilePath`、`SplashPath`、`Asset`，前端不再为它请求 `.../loot_item_icons/.png`（必 404）。空白记录本来就显示占位图标，视觉不变。
- 验收：`loot_blank_test.go` 加一条断言，空白记录三个路径都为空；**对抗变异**：去掉清空，该断言必须 FAIL。
- 这一项不改变任何文案。`local_request_client image` 里出现的 `queue_wait_ms≈10005` 是图片队列的另一个问题（日志里它和 `.png`、`chest_128.png` 这几个 404 的时间几乎重合，无法判定对应哪一个），不归本单；这次小修之后再看它还在不在，再决定是否另开工单。

### 7.1 第三份日志（`lol-loot-diagnostics-0924-1102.jsonl`，同一次运行，延长到 03:02:45）

- **重试用完后清除已在真机生效**：02:54:01.4 / 02:54:07.9 / 02:54:24.4 / 02:54:55.8 四次刷新（初次 + 三次重试，间隔 6.5 / 16.6 / 31.4 秒），每次都是同一条空白记录；最后一次刷新后 02:54:57.3 写出 `collection_data_retry_exhausted{attempts:3, blank_entries:1}`。日志只能证明后端状态已清除，提示条是否消失、卡片是否只剩「客户端返回的空白条目」+「类型未知」，要看界面（真机步骤 §5 第一条）。
- **`.png` 仍在请求**：03:01:28.5 和 03:01:38.5 两次打开账户页，`/fe/lol-loot/assets/loot_item_icons/.png`、`.../chest_128.png`、以及另一个 `.../loot_item_icons/{id}` 同一毫秒内全部 404。上面的“空白记录不带图片路径”小修还没做，所以这是预期内的。
- **观察，不归本单**：每一轮这三个 404 之后约 10 秒，都有一条 `local_request_client image failed`，`queue_wait_ms` 恰好约 10 秒（02:16:48、02:54:16、03:01:38 三次，跨两份日志）。三个 404 里只有 `.png` 属于本单；做完小修后再看这个 10 秒等待还在不在，如果还在，说明它来自 `chest_128.png` 那类客户端没有的图标，另开工单。

## 8. 决定：账户页不再展示这条空白记录（2026-09-24，用户指示）

用户看完 §7 的结论后决定：这条命名不了的空白记录“不行就不展示了”。

- **`backend/web/app.js`**（`loadAccount`）：渲染前过滤掉 `blank` 记录；顶部“暂未同步”提示条也不再因它出现（`lootPending` 基于过滤后的列表）。一旦这条记录拿到身份（不再带 `blank`），会作为普通物品正常显示。
- **后端不变**：`Blank` 标记、`DataPending` 的三次自动重试、`collection_data_retry_exhausted` 诊断、`loot_name_fallback` 的字段名诊断都保留（重试是为了客户端刚启动时记录可能补全；诊断用来在它再次出现变化时看得出来）。记录仍在 `/api/account` 响应里，只是不渲染。
- **与旧原则的关系**：`docs/loot-names-0912.md` 的“不因名称缺失删除真实库存条目”仍然成立——有 ID 但缺名称的条目照旧显示（原始 ID 兜底）。只有“无键、无 ID、无名称、无可用本地化名”这一类无从命名的占位记录不显示。
- **测试**：`web/champions.test.cjs` 新增 1 条（页面不渲染空白记录和提示条；记录拿到身份后正常显示；去掉过滤则该断言 FAIL，已在测试内做了变异）。全量 `node --test backend/web/*.test.cjs` 通过。
- **§3 里空白卡片的文案**（「客户端返回的空白条目」「类型未知」）现在只是 `lootCard` 的兜底，页面不再走到它。
- **§5 真机步骤 1 的验收改为**：账户页材料区里没有那张 ×74 的卡，顶部也没有“暂未同步”提示条。
