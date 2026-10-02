# R151 执行账本

日期：2026-09-24。基线：0.12.19 + R149 + R150；本单不重做 R150。按用户此前明确要求，**暂不改版本号或打包**。

## 探测与分支

真实单资源响应与来源表见 [r151-probe-findings.md](r151-probe-findings.md)。英雄 887、3、157 的 YOUR.GG 聚合 `augments` 均含银、金、棱彩三类，每行有 `augmentId/tier/score`，返回 ID 全部对应 CommunityDragon 目录。英雄 3 的站点页面展示总行数与接口吻合；没有证据表明接口只返每品质前几项。`response` 不含搭档段。选择 **分支 A**：海克斯统计整段用 YOUR.GG，搭档继续 OP.GG；没有混合两站海克斯行。网页 Network 的附加路径未能完整抓取，边界已在探测文件写明，代码不依赖任何未核实的新路径。

## 代码

- `yourGGArenaAggregateResponse` 接收真实 `augments` 数组；行映射保留 YOUR.GG 字母与评分，百分比字段按既有 YOUR.GG 装备规则换算。响应没有 `pickRate` 和品质，前者留空，后者用 CommunityDragon ID 目录对照。目录缺失的 ID 保留为“海克斯 ID”且不伪造图标或品质；若目录整体不可用、数组缺失、行结构无档位/评分，或三个品质缺一，整段退回 OP.GG。
- 斗魂正常路径不再调用 `applyLocalArenaAugmentGrades`。YOUR.GG 聚合失败或海克斯段无效时，OP.GG `augment_group` 整段回退，沿用 R150 本地 S/A/B 字母且 `Score=0`；核心与棱彩装备各自保持 R150 的 YOUR.GG 优先与 OP.GG 回退。新增 `arena_augment_source` 诊断事件，只记来源、行数或降级原因，不记响应内容、Cookie 或令牌。
- 前端现有 `score > 0` 卡片规则直接显示 YOUR.GG 海克斯综合评分和字母六边形；无分行依然隐藏评分。卡片渲染代码、排序规则未改，只补了官方海克斯行的回归测试。
- OP.GG 详情请求仍提供搭档和技能等缺失段以及既有完整性校验；YOUR.GG 聚合没有这些字段，当前不能省掉该请求。当前斗魂 UI 不展示技能，也未启用反制段，来源表按代码真实状态记录。

## 验证

- `go build -o /tmp/deep-legends-r151 ./backend` 通过，最终产物 SHA-256：`d4499fa49dafc1add6f4be496c008022e69dc75c9929fece08f39306575f5a7f`。
- `go vet ./...` 通过；最终代码的 `go test ./... -count=1` 通过，backend 202.361 秒。对照 R150 的 `go test ./...` 通过、backend 194.756 秒，既有失败没有增加；本次时长不构成性能基准。
- `go test ./backend -run 'TestR151|TestStructuredArenaDetailPreservesAllAugmentRows' -count=1` 通过。真实字段形状的脱敏 Go 夹具覆盖三品质、官方字母/评分/图标、缺目录 ID、聚合失败与缺少海克斯段时 OP.GG 回退、回退 `Score=0`。
- `node --test backend/web/r150.test.cjs` 5/5 通过，新增对斗魂海克斯官方分显示、无分隐藏和六边形字母的断言。
- 四项临时 Go 对抗变异均触发测试断言失败：关闭 YOUR.GG 正常路径、丢弃官方评分、聚合失败时不回退、把本地分写入回退行。每项后在 `finally` 原样恢复源码。`git diff --check`、`node --check backend/web/r150.test.cjs` 通过。
- 全量 `node --test backend/web/*.test.cjs desktop/*.test.cjs`：967 项，966 通过、1 跳过、0 失败，与 R150 基线一致。

## 真机边界

本机为 macOS，没有 Windows 英雄联盟客户端或 LCU，无法完成 R150 遗留的四英雄真实榜单与详情截图、跨页面徽章抽查、双主题 Beaufort 标题 ±1px 实测、长账号溢出、导出日志里的 `arena_header_source hasListRow=true`。本单没有把网页数据或自动化断言冒充这些验收。R150 的本地页面浏览器安全策略阻止仍有效，未绕过。以上待 Windows 客户端使用含 R151 代码的构建验收。
