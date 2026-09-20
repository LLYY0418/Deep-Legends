# WORKLIST-R105：职业选手账号内置与头像旗帜解锁

**目标：** (1) 将用户核对完成的 33 个选手账号清单（`docs/pro-accounts-verification-2026-09-17.md`）完整嵌入代码，替换现有的旧账号列表；(2) 根据 2026-09-17 20:38 诊断日志的 W5/W6 探测结果（201/200 全部成功），解锁头像和旗帜功能的门禁与 UI，使其对所有用户可用。

**背景：** R101 之后用户上传了核对完成的账号清单，明确要求"以这个文档为准进行修改或者内置"，但 R102/R103/R104 都没有执行账号更新，导致产品界面上仍显示旧账号数据（Wei 还显示 `dyjkbysb#KR1`，Rookie 缺少新账号，Xun/TheShy 等人的更新也未生效）。本轮必须完整嵌入用户核对后的所有账号数据。头像/旗帜功能在 R101 设计的探测已在真机上全部成功（W5: 201, W6: 200），现在需要打开正式功能。

---

## P0：嵌入用户核对完成的职业选手账号清单

### 数据来源

**唯一事实来源：** `docs/pro-accounts-verification-2026-09-17.md`（用户核对定稿版本）。

该文档包含：
- **6 支队伍，33 名选手，共计 53 个账号**
- 每个账号的 `GameName#TagLine`、所属选手、位置
- 用户逐条对照 OP.GG 小程序核对的最终结果

**关键修正（必须严格执行）：**
1. **`dyjkbysb#KR1` 归属 Wei（打野）**，不是 Rookie——这是用户 2026-09-17 明确核对确认的裁决
2. **Rookie 新增 2 个账号：** `벼락식혜#0070` 和 `EmberKnight#KR0`
3. **Xun 主号变更为：** `我累铜泥丸#小重o`（新增）
4. **TheShy 账号列表更新：** `은여하#1103` 现在是最活跃账号（用户清单中标注为主号）
5. **knight 的 `BLG 온#KR1` 已确认归 knight**，不是 ON
6. **Smash 的账号顺序：** 用户清单中 `Smash#KR2` 标注为主号（段位较低但更活跃），`DK Smash#KR7` 是小号（段位更高但更久没打）——这是用户新排序规则（活跃度优先于段位）的直接例证

### 实现要求

1. **读取 `docs/pro-accounts-verification-2026-09-17.md`**，解析出所有 6 支队伍、33 名选手、53 个账号的完整列表

2. **更新 `pro_roster.go` 或相关数据源文件**：
   - 如果账号列表是硬编码在 Go 代码里，完整替换为用户清单中的账号
   - 如果账号列表是从外部数据文件读取（如 JSON/YAML），更新该文件
   - 保持现有的 `proRoster` 结构（Name/Position/Names 等字段）不变，只更新账号数据

3. **账号数据结构要求：**
   - 每个选手名下的账号列表必须包含用户清单中列出的所有账号（包括标注为"小号"的账号）
   - 账号顺序：暂时保持用户清单中的顺序（第一个账号通常是用户标注的主号或最活跃账号）
   - GameName 和 TagLine 必须精确匹配用户清单（包括大小写、空格、特殊字符）

4. **特殊字符处理：**
   - Canyon 的账号：`JUGKlNG#kr`（K 后为小写 L，不是大写 i）
   - Ruler 的账号：`강 철#샤 넬`（包含空格）
   - 确保这些特殊情况不被自动"纠正"

5. **文档同步更新：**
   - `docs/pro-players-sources.md` 中三处"Wenbo 目前没有可交叉确认归属的韩服账号"的表述已过期，Wenbo 账号 `14小孩幻想赢对线#4453` 已确认
   - 更新这些过期表述

### 验证判据

**必须 PASS：**

1. **账号数量验证：**
   ```go
   // 在 pro_players_test.go 或新增测试中
   func TestR105_EmbeddedAccountCounts(t *testing.T) {
       // 读取实际内置的账号数据
       totalAccounts := 0
       for _, team := range proRoster {
           for _, player := range team.Players {
               // 从账号数据源统计每个选手的账号数
               accounts := getPlayerAccounts(player.Name) // 需要实现
               totalAccounts += len(accounts)
           }
       }
       
       // 用户清单中共 53 个账号
       if totalAccounts != 53 {
           t.Errorf("total accounts = %d, want 53", totalAccounts)
       }
   }
   ```

2. **关键修正验证：**
   ```go
   func TestR105_CriticalAccountFixes(t *testing.T) {
       // Wei 的账号中必须包含 dyjkbysb#KR1
       weiAccounts := getPlayerAccounts("Wei")
       if !containsAccount(weiAccounts, "dyjkbysb", "KR1") {
           t.Error("Wei should have dyjkbysb#KR1")
       }
       
       // Rookie 的账号中必须包含新增的两个账号
       rookieAccounts := getPlayerAccounts("Rookie")
       if !containsAccount(rookieAccounts, "벼락식혜", "0070") {
           t.Error("Rookie missing 벼락식혜#0070")
       }
       if !containsAccount(rookieAccounts, "EmberKnight", "KR0") {
           t.Error("Rookie missing EmberKnight#KR0")
       }
       
       // Xun 的账号中必须包含新主号
       xunAccounts := getPlayerAccounts("Xun")
       if !containsAccount(xunAccounts, "我累铜泥丸", "小重o") {
           t.Error("Xun missing 我累铜泥丸#小重o")
       }
       
       // knight 的账号中必须包含 BLG 온#KR1
       knightAccounts := getPlayerAccounts("knight")
       if !containsAccount(knightAccounts, "BLG 온", "KR1") {
           t.Error("knight missing BLG 온#KR1")
       }
   }
   ```

3. **选手账号数验证（按用户清单逐个核对）：**
   ```go
   func TestR105_PlayerAccountCounts(t *testing.T) {
       expected := map[string]int{
           // BLG (7 人，9 个账号)
           "Bin": 1, "Wenbo": 1, "Flandre": 1,
           "Xun": 2, "knight": 2, "Viper": 1, "ON": 1,
           // IG (6 人，13 个账号)
           "TheShy": 5, "Wei": 2, "Rookie": 4,
           "Assum": 1, "JiaQi": 2, "Meiko": 1,
           // T1 (5 人，5 个账号)
           "Doran": 1, "Oner": 1, "Faker": 1, "Peyz": 1, "Keria": 1,
           // HLE (5 人，8 个账号)
           "Zeus": 1, "Kanavi": 2, "Zeka": 2, "Gumayusi": 2, "Delight": 1,
           // GEN (5 人，5 个账号)
           "Kiin": 1, "Canyon": 1, "Chovy": 1, "Ruler": 1, "Duro": 1,
           // DK (5 人，10 个账号)
           "Siwoo": 2, "Lucid": 2, "ShowMaker": 2, "Smash": 3, "Career": 2,
       }
       
       for playerName, wantCount := range expected {
           accounts := getPlayerAccounts(playerName)
           if len(accounts) != wantCount {
               t.Errorf("%s: got %d accounts, want %d", 
                   playerName, len(accounts), wantCount)
           }
       }
   }
   ```

4. **特殊字符验证：**
   ```go
   func TestR105_SpecialCharacters(t *testing.T) {
       // Canyon: JUGKlNG#kr（小写 L）
       canyonAccounts := getPlayerAccounts("Canyon")
       found := false
       for _, acc := range canyonAccounts {
           if acc.GameName == "JUGKlNG" && acc.TagLine == "kr" {
               // 验证是小写 L 不是大写 i
               if strings.Contains(acc.GameName, "I") {
                   t.Error("Canyon account should be JUGKlNG (lowercase L), not JUGKING (uppercase I)")
               }
               found = true
           }
       }
       if !found {
           t.Error("Canyon missing JUGKlNG#kr")
       }
       
       // Ruler: 강 철#샤 넬（包含空格）
       rulerAccounts := getPlayerAccounts("Ruler")
       if !containsAccount(rulerAccounts, "강 철", "샤 넬") {
           t.Error("Ruler missing 강 철#샤 넬 (with spaces)")
       }
   }
   ```

5. **UI 数据验证（通过 `pro_players` 事件检查）：**
   - 启动软件，打开职业选手页
   - 导出诊断日志，检查 `pro_players` 事件中：
     - Wei 的账号列表包含 `dyjkbysb#KR1`（不再显示给 Rookie）
     - Rookie 的账号列表包含 `벼락식혜#0070` 和 `EmberKnight#KR0`
     - Xun 的账号列表包含 `我累铜泥丸#小重o`
     - 所有选手的账号数与上述 `expected` map 一致

### 变异判据

**必须 FAIL（回归验证）：**

1. **变异 #1：删除 Wei 的 `dyjkbysb#KR1`**
   - 方法：从 Wei 的账号列表中移除 `dyjkbysb#KR1`
   - 预期：`TestR105_CriticalAccountFixes` 失败，报错 "Wei should have dyjkbysb#KR1"

2. **变异 #2：将 Rookie 的新账号从列表中移除**
   - 方法：从 Rookie 的账号列表中移除 `벼락식혜#0070` 和 `EmberKnight#KR0`
   - 预期：`TestR105_CriticalAccountFixes` 失败，报错 Rookie 缺少新账号

3. **变异 #3：账号总数改为 52**
   - 方法：从任意选手的账号列表中删除 1 个账号
   - 预期：`TestR105_EmbeddedAccountCounts` 失败，报错 "total accounts = 52, want 53"

4. **变异 #4：Canyon 账号改为大写 i（JUGKING）**
   - 方法：将 `JUGKlNG` 改为 `JUGKING`
   - 预期：`TestR105_SpecialCharacters` 失败，报错大小写错误

---

## P1：解锁头像选择器的门禁与 UI

### 探测结果回顾

**来自 `lol-loot-diagnostics-0917-2038.jsonl`：**
- **W5 (写入未拥有的头像): status 201, ok: true**
- **W5-restore (恢复原头像): status 201, ok: true**

**结论：** 头像写入功能在真机上已验证可用，无论是否拥有该头像（未拥有的头像返回 201 而非 401），现在可以打开正式功能。

### 实现要求

1. **移除头像选择器的拥有态门禁：**
   - 当前实现（R101）：头像选择器中，未拥有的头像置灰+不可选，默认勾选"只显示已拥有"
   - 新要求：**所有头像都可选**，移除"只显示已拥有"的筛选开关
   - 理由：W5 探测已证明未拥有的头像也可以成功写入（LCU 返回 201）

2. **保留现有 UI 结构：**
   - 全屏模态对话框
   - 名称/拼音搜索框
   - 图标网格（圆形缩略图）
   - 点击选择 → 立即发送 PUT 请求 → 成功后关闭弹窗

3. **错误处理：**
   - 如果 PUT 请求返回非 201 状态码，显示 toast 提示错误
   - 保持 R101 现有的错误处理逻辑（401 RPC_ERROR 等）

### 验证判据

**必须 PASS：**

1. **UI 渲染验证：**
   ```javascript
   // 在 web/gameplay.test.cjs 或新增测试中
   test('icon picker shows all icons without ownership filter', async () => {
       const { container } = renderIconPicker();
       
       // 应该没有"只显示已拥有"的开关
       const ownershipToggle = container.querySelector('[data-testid="ownership-toggle"]');
       expect(ownershipToggle).toBeNull();
       
       // 所有图标都应该可点击（不置灰）
       const iconCells = container.querySelectorAll('.icon-cell');
       iconCells.forEach(cell => {
           expect(cell.classList.contains('disabled')).toBe(false);
       });
   });
   ```

2. **写入请求验证：**
   ```javascript
   test('clicking an icon sends PUT request', async () => {
       const mockFetch = jest.fn().mockResolvedValue({ status: 201 });
       global.fetch = mockFetch;
       
       const { container } = renderIconPicker();
       const firstIcon = container.querySelector('.icon-cell');
       firstIcon.click();
       
       await waitFor(() => {
           expect(mockFetch).toHaveBeenCalledWith(
               expect.stringContaining('/lol-summoner/v1/current-summoner/icon'),
               expect.objectContaining({ method: 'PUT' })
           );
       });
   });
   ```

3. **真机验证（需要在 Windows 上测试）：**
   - 打开软件 → 工具 → 背景、签名与展示 → 点击头像卡片的"更改"按钮
   - 验证：
     - 弹出全屏图标选择器
     - 所有图标都可点击（无置灰）
     - 点击任意图标 → 立即切换 → 弹窗关闭
     - 打开游戏客户端，确认头像已成功切换

### 变异判据

**必须 FAIL：**

1. **变异 #1：恢复拥有态门禁**
   - 方法：将未拥有的图标重新设置为置灰+不可选
   - 预期：UI 测试失败，报错 "some icons are disabled"

2. **变异 #2：删除 PUT 请求**
   - 方法：注释掉点击图标后发送 PUT 请求的代码
   - 预期：写入请求验证失败，`mockFetch` 未被调用

---

## P2：解锁旗帜选择器的门禁与 UI

### 探测结果回顾

**来自 `lol-loot-diagnostics-0917-2038.jsonl`：**
- **W6 (切换旗帜): status 200, ok: true**
- **W6-restore (恢复原旗帜): status 200, ok: true**

**结论：** 旗帜切换功能在真机上已验证可用，现在可以打开正式功能。

### 实现要求

1. **移除旗帜选择器的拥有态门禁：**
   - 当前实现（R101）：旗帜选择器中，未拥有的旗帜置灰+不可选，默认勾选"只显示已拥有"
   - 新要求：**所有旗帜都可选**，移除"只显示已拥有"的筛选开关
   - 理由：W6 探测已证明旗帜切换功能可用（LCU 返回 200）

2. **保留现有 UI 结构：**
   - 全屏模态对话框
   - 旗帜缩略图网格（横向矩形）
   - 点击选择 → 立即发送 PATCH 请求到 `/lol-loadouts/v4/loadouts/{id}` → 成功后关闭弹窗

3. **数据字段处理：**
   - R101 探测发现 `regalia` 对象中有一个未记录在社区文档的 `data` 字段
   - 写入时必须保留现有的 `data` 字段值（不能丢失或覆盖）
   - 只修改 `contentId`/`inventoryType`/`itemId` 字段

4. **错误处理：**
   - 如果 PATCH 请求返回非 200 状态码，显示 toast 提示错误

### 验证判据

**必须 PASS：**

1. **UI 渲染验证：**
   ```javascript
   test('banner picker shows all banners without ownership filter', async () => {
       const { container } = renderBannerPicker();
       
       // 应该没有"只显示已拥有"的开关
       const ownershipToggle = container.querySelector('[data-testid="ownership-toggle"]');
       expect(ownershipToggle).toBeNull();
       
       // 所有旗帜都应该可点击
       const bannerCells = container.querySelectorAll('.banner-cell');
       bannerCells.forEach(cell => {
           expect(cell.classList.contains('disabled')).toBe(false);
       });
   });
   ```

2. **data 字段保留验证：**
   ```javascript
   test('PATCH request preserves data field', async () => {
       const originalLoadout = {
           REGALIA_BANNER_SLOT: {
               contentId: 'old-banner-id',
               data: { someUndocumentedField: 'preserve-this' },
               inventoryType: 'REGALIA_BANNER',
               itemId: 123
           }
       };
       
       const mockFetch = jest.fn()
           .mockResolvedValueOnce({ ok: true, json: async () => originalLoadout }) // GET
           .mockResolvedValueOnce({ status: 200 }); // PATCH
       global.fetch = mockFetch;
       
       const { container } = renderBannerPicker();
       const secondBanner = container.querySelectorAll('.banner-cell')[1];
       secondBanner.click();
       
       await waitFor(() => {
           const patchCall = mockFetch.mock.calls.find(call => call[1]?.method === 'PATCH');
           const patchBody = JSON.parse(patchCall[1].body);
           
           // data 字段必须被保留
           expect(patchBody.REGALIA_BANNER_SLOT.data).toEqual(
               originalLoadout.REGALIA_BANNER_SLOT.data
           );
       });
   });
   ```

3. **真机验证（需要在 Windows 上测试）：**
   - 打开软件 → 工具 → 背景、签名与展示 → 点击旗帜卡片的"更改"按钮
   - 验证：
     - 弹出全屏旗帜选择器
     - 所有旗帜都可点击（无置灰）
     - 点击任意旗帜 → 立即切换 → 弹窗关闭
     - 打开游戏客户端，确认旗帜已成功切换

### 变异判据

**必须 FAIL：**

1. **变异 #1：恢复拥有态门禁**
   - 方法：将未拥有的旗帜重新设置为置灰+不可选
   - 预期：UI 测试失败，报错 "some banners are disabled"

2. **变异 #2：删除 data 字段保留逻辑**
   - 方法：PATCH 请求中不包含原有的 `data` 字段
   - 预期：data 字段保留验证失败

---

## P3：更新 facade 页文案与文档

### 实现要求

1. **更新头像/旗帜卡片的说明文案：**
   - 当前文案（R101）：可能包含"只能选择已拥有的头像/旗帜"之类的限制说明
   - 新文案：移除拥有态相关的说明，改为简洁的"点击更改"或"点击选择"

2. **更新 `docs/r99-probe-results.md`：**
   - 追加 W5/W6 的真机探测结果（来自 2026-09-17 20:38 日志）
   - 记录 W5: 201（未拥有的头像也可以写入）
   - 记录 W6: 200（旗帜切换成功）

3. **更新 `docs/pro-accounts-verification-2026-09-17.md` 的引用：**
   - 在文件开头或 CLAUDE.md 中明确标注"此文件已于 R105 完整嵌入代码"
   - 避免未来重复嵌入

### 验证判据

**必须 PASS：**

1. **文案验证：**
   - 在 facade 页源码中搜索"拥有""已拥有""own"等关键词
   - 确认头像/旗帜相关说明文案中不再包含拥有态限制

2. **文档验证：**
   - `docs/r99-probe-results.md` 中包含 W5/W6 的最新探测结果
   - `docs/pro-accounts-verification-2026-09-17.md` 或 CLAUDE.md 中有明确标注"已于 R105 嵌入"

---

## 通用验证要求

### 编译与测试

1. **Go 编译：** `go build -o deep-legends.exe .`，exit code 0
2. **Go vet：** `go vet ./...`，无警告
3. **Go 测试：** `go test -count=1 -v .`，所有测试通过（包括新增的 R105 测试）
4. **Go race：** `go test -race -run 'TestR105' -count=1`，无数据竞争
5. **前端测试：** `node --test web/*.test.cjs`，所有测试通过（包括新增的 UI 测试）

### 对抗变异

执行上述所有"变异判据"（共 9 个变异），确认每个变异都能被相应的测试用例抓住，且 baseline（未变异）版本全部通过。

### 真机验证清单

1. **职业选手页：**
   - [ ] Wei 的账号列表显示 `dyjkbysb#KR1`
   - [ ] Rookie 的账号列表显示 `벼락식혜#0070` 和 `EmberKnight#KR0`
   - [ ] Xun 的账号列表显示 `我累铜泥丸#小重o`
   - [ ] 所有 33 个选手的账号数与 P0 的 `expected` map 一致

2. **头像选择器：**
   - [ ] 点击头像卡片 → 弹出全屏图标选择器
   - [ ] 所有图标都可点击（无置灰、无"只显示已拥有"开关）
   - [ ] 点击任意图标 → 立即切换 → 弹窗关闭
   - [ ] 游戏客户端中头像已更新

3. **旗帜选择器：**
   - [ ] 点击旗帜卡片 → 弹出全屏旗帜选择器
   - [ ] 所有旗帜都可点击（无置灰、无"只显示已拥有"开关）
   - [ ] 点击任意旗帜 → 立即切换 → 弹窗关闭
   - [ ] 游戏客户端中旗帜已更新

---

## 注意事项

1. **账号数据精确性：** GameName 和 TagLine 的大小写、空格、特殊字符必须与用户清单完全一致，不得自动"纠正"或标准化

2. **向后兼容：** 如果现有代码依赖旧的账号数量或特定账号名，必须同步更新相关断言和测试

3. **数据字段保留：** 旗帜 PATCH 请求中，除了 `contentId`/`inventoryType`/`itemId` 之外的字段（特别是 `data`）必须从 GET 响应中读取并原样回写

4. **不删除探测代码：** R99/R101 的探测代码（W5/W6 等）保留，供未来回归验证使用

5. **文档同步：** 所有与账号数据相关的文档（`docs/pro-players-sources.md` 等）必须同步更新，避免过期信息

---

## 交付物

1. 更新后的账号数据源文件（`pro_roster.go` 或等效）
2. 新增的 R105 测试文件（`TestR105_*` 系列测试）
3. 更新后的前端代码（移除头像/旗帜的拥有态门禁）
4. 更新后的文档（`docs/r99-probe-results.md`、`docs/pro-players-sources.md`、CLAUDE.md）
5. 执行账本（`docs/r105-execution-ledger.md`）
6. 验证artifacts（`docs/r105-validation/`）
7. 对抗变异结果（`docs/r105-validation/mutations/matrix.json`）

---

## 工单结束标志

- [ ] 所有 P0-P3 的实现要求完成
- [ ] 所有验证判据通过（编译/测试/race/变异）
- [ ] 真机验证清单全部勾选
- [ ] 执行账本和验证artifacts已提交到 `docs/`
