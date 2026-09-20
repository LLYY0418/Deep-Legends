# WORKLIST-R97 — 职业选手账号清零真bug修复 + 范围收回原6队

## 背景

用户截图：选手管理页选中T1，队伍筛选栏出现了12个队（全部战队/BLG/IG/T1/HLE/GEN/DK/Winners/Young Miracles/
Machi Esports/Suning Gaming-S/Anyone's Legend.Young·二队/Suning/VSG），T1这5名选手（Doran/Oner/Faker/Peyz/Keria）
全部显示"暂无账号"、"部分记录待核验"（橙色），"韩服账号"列停在"正在读取选手账号…"，本质是**0个账号**。

## 根因（已诊断清楚，不是"队伍太多"导致的）

**这是一个跟队伍数量无关的真实退化bug**，证据是T1本来就在R95改动前的原6队白名单里（`pro_roster.go:27-73`
硬编码的BLG/IG/T1/HLE/GEN/DK），T1这种老队伍现在也中招，说明问题出在账号校验逻辑本身，不是新加的队伍缺数据。

`pro_players.go:449`（R95改动引入，未提交）：
```diff
-	if raw.Region != "" && !strings.EqualFold(raw.Region, "kr") {
+	if !strings.EqualFold(strings.TrimSpace(raw.Region), "kr") {
```
旧逻辑：**只有明确标了非韩服才拒绝**（region为空视为放行）。
新逻辑：**必须明确标了韩服才收**（region为空直接拒绝）。

而`pro_players.go:302-304`代码自己的注释写得很清楚：**OP.GG真实返回数据里账号级`region`字段经常是null**，
是不是韩服账号本来就该在目录/队伍这一层判断，账号级的region字段一直被当作可选字段处理。这次改动把它变成了
强制字段，导致几乎所有真实OP.GG账号（含T1全员）在`normalizeProAccount`里被判无效，`buildProPlayers`
（`pro_players.go:558-563`）里进而把这些账号从candidates里剔除，选手状态标成"partial"（对应界面上橙色
"部分记录待核验"），账号数清零。"正在读取选手账号…"本身是个正常的加载态文案（绑定后端`data.updating`），
不是真的卡死，只是加载结束后结果永远是0账号，看着像卡住。

**测试为什么没抓到**：`pro_players_test.go`里`proFixtureAccount`这个测试辅助函数所有fixture都硬编码
`Region:"kr"`，从来没测过生产环境最常见的"region为空"这种情况，是本轮验收的一个真实盲区。

## 用户决定

跟用户核实后，用户选择**修复根因，同时把职业选手范围缩回R95改动前的原6个队（BLG/IG/T1/HLE/GEN/DK）**，
不保留R95新扩的其余队伍（Winners/Young Miracles/Machi Esports/Suning Gaming-S/Anyone's Legend.Young二队/
Suning/VSG等）。

## 要做的事

### 1. 修region校验的根因bug

把`pro_players.go:449`那行改回**只在`raw.Region`非空且明确不是"kr"时才拒绝**（空region视为放行，交给
上层目录/队伍归属判断），恢复R95改动前的逻辑。

**★这里必须补一条真实的对抗测试**：`pro_players_test.go`里补一个"账号region字段为空但账号本身来自已知
韩服队伍目录"的fixture，断言这个账号能被正常收录、不会被清零。这条测试要能在"把region校验改回强制非空"
这个变异下真实失败，防止这个bug以后用同样的方式再退化回来。

### 2. 职业选手范围收回原6队

把R95引入的"放开到全部一队+二队降级展示"的扩容撤回，恢复到R95改动前的6队白名单范围（`pro_roster.go`
硬编码的BLG/IG/T1/HLE/GEN/DK）。**这个范围收回要覆盖两个界面**：
- 选手管理页（用户截图里那个，队伍筛选栏、账号列表）——回到只显示这6队。
- 战绩详情/当前对局/玩家总览里的职业选手徽章识别系统（`pro_identity.go`）——同样收回只识别这6队的选手，
  不要一边选手管理页只显示6队、一边战绩页却还在用全量目录识别100+队的选手，两处口径要一致。

明确要做的事：
- 前端`web/pro-players.js`的队伍筛选栏列表收回到6个。
- 后端凡是走`proDirectoryRoster`（全量目录）拿职业选手数据的路径，改回走原来的6队白名单（`pro_roster.go`
  那个函数，R95之前就有，不要删，直接复用）。
- `pro_players_ladder.go:34`目前调用`buildProPlayers(teams, proRoster)`用的还是原6队roster——范围收回后
  这行本来就该继续这样用，不用再额外改，但要确认收回后这里没有残留对全量目录的引用。
- R95新加的"二队/学院队视觉降级"（灰色徽章+后缀）相关UI/逻辑，如果范围收回后已经用不到了（因为不再展示
  二队），可以保留代码但确认没有死代码路径产生视觉异常；不强制要求删除这部分代码，只要求确认收回后不会
  被触发到、不会展示出二队相关UI。
- `docs/pro-players-sources.md`如果记录了扩容后的目录范围说明，同步更新成6队范围，避免文档和代码不一致。

### 3. 不要做的事

- 不要碰`pro_identity.go`里精确ID匹配（`matchProIdentity`）的匹配逻辑本身——那部分R95/R96两轮都验收过
  是对的，只是这次要把它的**输入范围**从全量目录收窄回6队白名单，匹配算法不用动。
- 不要因为要"缩回6队"就把R95新增的账号/目录相关基础设施（`pro_identity.go`、`proDirectoryRoster`等）
  整个删掉——`proDirectoryRoster`这类通用抓取/解析能力可以保留在代码里不调用，方便以后真要扩容时复用，
  但生产路径今天只能走6队白名单。
- 不要用mock/假数据掩盖回归测试——第1条要求的"region为空账号能正常收录"这条测试必须用真实结构的fixture，
  不能只测个空字符串走过场。

## 验收要求

- 一条真实的对抗测试证明"region为空不再导致账号被清零"，且这条测试在把校验改回强制非空的变异下会真实失败。
- 一条测试证明选手管理页与职业徽章识别两处的队伍范围都收回到6队，不再出现R95新增的那些队名。
- 常规回归：`go build .`、`go vet .`、`go test -count=1 -v .`、`go test -race ./...`、
  `node --test web/*.test.cjs desktop/*.test.cjs`全部真实跑一遍且全绿，附真实日志文件。
- 不要求真实客户端验收（fixture级别的对抗测试即可证明这个bug被修复）。
