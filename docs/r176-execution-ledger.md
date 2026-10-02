# R176 执行账本

日期：2026-10-01。工单：WORKLIST-R176-REMOVE-R172-CORE-ROUTE-STARTER-PREFIX.md。

R175 已独立完成于 0.12.40，本轮递增至 0.12.41；desktop/package.json 与 package-lock.json 根版本同步。源码指纹：c06daae1113a。

## 实施范围

- backend/web/gameplay.js 的核心装 optionList 调用移除第 5 个参数；optionList 与 renderConfigOption 均移除 leadingIds，删除前缀图标/divider 的拼装。data-icon-count 恢复为 ids.length。
- 顶部 build-summary-starter、核心装备顺序、箭头和 config-item 类名保留；没有新增 UI 文字或 tooltip。
- gameplay.css 的 route-divider、renderRuneEquipment、patchRuneStarterEquipment、specialist-game-spells 与 R173/R174 全部实现保留。独立探子对本轮修改前源码快照核验，生产差异仅上述三处逻辑。
- 原 R172 两条测试改为 R176 反向验收，未删除：经典模式覆盖两条核心路线及完整顶部出门装，每行顺序/箭头/计数；竞技场与海克斯模式无 divider，海克斯核心区无出门装图标；直接 renderConfigOption 覆盖空、单件、三件数量。
- WORKLIST-INDEX 新增 R176，R172 状态标记「核心装路线前缀已由 R176 移除」；更新未发布变更记录。

## 自动验证

- R176 定向测试：2/2 通过。
- R173/R174 实际符文装备行 divider 定向测试：2/2 通过。
- node --test backend/web/*.test.cjs：753/753 通过，16.130 秒；日志 /private/tmp/r176-web-full.log。
- go vet ./backend：通过；日志 /private/tmp/r176-vet.log。
- go test ./backend -count=1：通过，194.776 秒；日志 /private/tmp/r176-go-full.log。
- git diff --check：代码修改后与最终收尾均通过。

## 变异验证

使用 /private/tmp/r176-before-gameplay.js 修改前源码，通过 /private/tmp/r176-mutant-preload.cjs 仅在测试读取 gameplay.js 时覆盖，恢复完整可执行的 R172 leadingIds 前缀（核心调用、参数传递和图标拼装）。仓库生产文件未回滚。

node --require /private/tmp/r176-mutant-preload.cjs --test --test-name-pattern='R176 classic' backend/web/champions.test.cjs 返回 1，经典测试在核心区发现 1055/2003 与 route-divider，doesNotMatch 触发 AssertionError。没有 ReferenceError 或 SyntaxError。日志 /private/tmp/r176-mutant.log。

## 构建

两平台后端构建通过：

- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.41-public。
- Windows amd64：/private/tmp/Deep-Legends-backend-0.12.41-public.exe。
- node desktop/verify-build-fingerprint.cjs 对两个产物均验证 c06daae1113a；该指纹由 desktop/source-fingerprint.cjs 在最终生产源码与版本修改后生成。
- macOS 在 /private/tmp/r176-selftest 隔离目录、空 RIOT_API_KEY 下 self-test 通过，日志确认 Deep Legends 0.12.41，奖池 554 条，哈希 dee5e21f5234；日志 /private/tmp/r176-selftest.log。

key mode=public；CGO_ENABLED=0，两次构建都显式设置 main.version=0.12.41、main.buildFingerprint=c06daae1113a、main.riotAPIKey=、main.riotAPIKeyCipher=。产物文件名带 -public。没有发布安装包。

## Windows 真机边界

本机 DOM 回归与交叉构建不能代替 Windows 真机验收。用户待确认：核心装每行从第一个核心图标开始，没有出门装/竖线；顶部出门装区块仍在；绝活哥/职业选手符文记录装备行仍有出门装及竖线。
