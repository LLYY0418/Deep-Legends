# R154 执行账本

日期：2026-09-25。基线为含 R144–R153 未提交改动的工作区；这些既有改动未回退。产品版本由 0.12.19 递增至 0.12.20。未打安装包。

## 逐项结果

| 项 | 实施与证据 | 本机状态 |
|---|---|---|
| P1 | `communitydragon` 请求在主机退避时跳过远端；英雄数字图标、头像和物品数字图标只在能够精确换算时回落 Data Dragon。补全了 `/api/champion-asset` 和离线总览实际使用的 `/api/image` 两条路径。无现成版本号时只查询一次 Data Dragon 版本。诊断事件记录最终主机与回落路径。 | 定向 HTTP 测试覆盖超时、主机退避和整组负缓存；移除回落后的隔离副本对抗变异失败。强化符文等无核验等价资源的图片仍不会伪装为另一张图。 |
| P2 | `hero-json` 熔断保持按 `kind` 跨英雄 ID，增加专门错误标记；HTTP 503 和前端文案使用“上游暂时不可用，请稍后再试”，普通本地请求超时显示“网络超时，请重试”。 | 三次 `context.DeadlineExceeded` 后，89/105 两个 ID 均立即熔断且网络请求数为 0；前端文案分支与去分支变异通过。 |
| P3 | 头像未拥有图像与旗帜一致，`opacity:.68; filter:none`。 | CSS 定点核对；待真机截图。 |
| P4 | 首次切到未加载的视图立即清空旧卡片，设置加载态。 | 延迟旗帜响应的 jsdom 测试通过；去掉清空逻辑的变异失败。 |
| P5 | 按视图和 ID 复用仍可见卡片与图片节点，只增删变化的卡片，排序时移动节点；保留 7ms 分帧。 | jsdom 验证筛选与排序后节点引用及 `is-loaded` 不变；恢复全量重建的变异失败。 |

## 与工单诊断的核对

- 总览离线头像通过 `/api/image` 进入 `serveCommunityDragonImage`，工单 P1 只指出 `/api/champion-asset`。本轮同时修复这条实际路径。
- `hero-json` 熔断确实按 `kind` 跨英雄 ID。当前海斗详情使用它；斗魂详情主请求在 `loadStructuredDetail` 中读取 OP.GG，海克斯可选数据由 YOUR.GG 补充。现有代码无法证明斗魂详情由 `hero-json` 熔断直接连坐，因此没有为了工单推断去修改熔断粒度或 3 次阈值。两页均使用新的错误文案映射，但只有收到熔断错误的请求会显示上游暂不可用。
- 工单引用的 `lol-loot-diagnostics-0925-1427.jsonl` 原件不在本工作区，也未随本任务附上；本轮不能归档或拿它作新增真机日志。工单内摘录仅用于定位。

## 验证记录

- `go test ./backend -run 'TestR154|TestHandleChampionAssetCommunityDragon' -count=1`：通过。
- `node --test backend/web/r154.test.cjs`：3 项通过；P2/P4/P5 变异均触发断言失败。
- P1 对抗变异在 `/private/tmp` 的隔离副本将回落函数改为恒定无匹配：英雄图标与离线头像测试均由 200 变成 404，预期失败；主工作区未改坏。
- `go build -o /private/tmp/deep-legends-r154 -ldflags '-X main.version=0.12.20' ./backend`：通过；最终构建的 `-self-test` 输出“Deep Legends 0.12.20 自检通过”。
- `go vet ./...`：通过。
- `go test ./... -count=1`：通过，backend 196.126 秒。末尾新增诊断路径断言和物品 ID 映射后，再跑 R154 定向测试通过。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：970 项，969 通过、1 跳过、0 失败（R153 基线 967 项、966 通过、1 跳过、0 失败）。
- [Riot Data Dragon 官方文档](https://developer.riotgames.com/docs/lol)列出版本化的英雄方形图、物品和头像图片路径，本轮的三种回落映射据此限定。

## 待 Windows 真机

1. 客户端不启动，刷新韩服总览；核对头像、英雄和物品图标，并导出新诊断日志确认 `champion_asset_fallback` 与实际图标呈现一致。强化符文资源若只在 CommunityDragon 有而该主机不可达，仍应据实记录缺图。
2. 在受控网络中让 Hexdata `hero-json` 连续超时三次，观察海斗错误文案；另测斗魂实际失败来源，不能凭同一时段截图认定为共用熔断。恢复网络后复核请求与文案。
3. 对照头像、旗帜未拥有配色截图；限速切视图，检查旧卡片不残留；来回切换“显示未拥有”，检查已加载头像不再闪烁。
