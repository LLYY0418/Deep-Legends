# R191 执行账本

基线 R190 已提交：`c2582075`。本次按 P1 → P5 执行，版本目标 0.12.55。

## 实现与证据

- P1：原始 var 使用指针，缺失/null 与明确 0 区分；一个已选符文三变量全部缺失时整组 perkStats 省略。v2 缓存保留 null。
- P2：沿用现有诊断通道，presence 每来源、sample 每符文在应用进程会话内最多 3 条；日志轮转不会重新放开上限。样本只选当前主体，保留原始三个 var（缺失为 null），无姓名/gameId。8008/8304 修正表保持空数组。
- P3：只更新有 ID 交集的已展开构建详情，覆盖普通页签、缓存页签、浮层和外部视图，保留卡片摘要和 revision。
- P4：目录错误、校验失败、缺少 ID 均记录 augment-directory 后走 CommunityDragon；构建接口仍只接受真实详情正文。
- P5：38/28px 图标、12px 数值间隔、宽布局树居中、碎片 22px 图标/左 padding 4px，窄布局横排。R117 仅额外豁免 `3px 9px 3px 4px` 这一工单要求的 padding，预算不扩大。

## 验证

工单 17 项逐一验证通过：

| 项目 | 验证入口 | 结果 |
|---|---|---|
| 1–4 Riot/SGP/LCU 缺失与零值、单条缺失、v2 往返 | `backend/r191_test.go` 的 `TestR191_01`–`04` | 通过 |
| 5 presence 数量与每来源上限 | `TestR191_05` | 通过，3 来源各 3 条；日志轮转不重置 |
| 6–8 主体样本、每符文上限、实际诊断导出 | `TestR191_06`–`08` | 通过；Riot/SGP/LCU 覆盖；每个 perk 在三来源之间共享 3 条预算，符合 P2 原文 |
| 9–10 二十张卡片保留、两个构建页一起更新 | `backend/web/r191.test.cjs` | 通过；实际生产局部替换函数更新 active/cached detail，摘要/卡片引用与 revision 保持；浮层及外部入口覆盖 |
| 11–13 图鉴 fallback 200、构建 unavailable、正常真实正文 | `TestR191_11`–`13` | 通过；目录请求失败、校验失败、无 ID 均覆盖 |
| 14–16 尺寸、碎片图标、仅宽容器竖排 | `backend/web/r191.test.cjs` | 通过；图标与共享板使用同一 Data Dragon 路径 |
| 17 Chromium 宽/窄布局与设计图对照 | `node desktop/r191-layout.cjs` | 通过；截图及测量值见 `history/reports/r191/` |

布局实测：1280px 的树栏 892.5px、效果栏 297.5px（3:1），树内容中心与树栏中心相同；820px 碎片 flex 同行。两种宽度无横向溢出，碎片图标均为 22×22。Chromium 发现继承的 flex-basis 会把碎片撑到 38px，已用局部 22px 规则修复。截图使用明确的手写布局夹具及替代图标，不代表真实 SGP/LCU 样本。

四个变异都被断言杀死，未以编译失败充数，见 `history/reports/r191/mutations.json`：缺失当零、全局重绘、目录直接返回错误、取消记录上限。工单只明确前三个，第四个补为取消样本次数上限，要求第 7 项测试失败。

独立只读复核已完成。R190 的 9 项 Node 回归、R117 样式预算及 R86 两项实际页面护栏通过。Node 首轮全量有 2 项失败：R86 的函数签名变异入口过期、外部卡片测试仍要求整体替换；已更新到新签名以及保留卡片/摘要但展开详情的预期，原过滤隐藏状态和禁止全局刷新的变异仍必须失败，定向复验 2/2 通过。未发现生产行为失败。

Go 全量 1686 项，五分片全部通过（71.9 秒）；backend vet、installer 全量测试及 vet 通过。Node 全量最终 1129 项：1126 通过、0 失败、3 项平台条件跳过（240.8 秒），覆盖 backend/web、desktop 和 scripts，日志 `/private/tmp/r191-full-node-final.log`。最后将 P2 主体样本断言补强为 `[3,1028,7]` 原值及顺序（不猜含义），全部 R191 Go 专项再次通过；仅测试夹具变化，生产源码及构建指纹保持一致。

## 构建收据

完整 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh` 成功，版本 **0.12.55**，key mode **public**，无内置私有 Riot Key。源码、后端和打包运行时指纹均为 `e664dffd29b6`，最终重新计算一致。NSIS、安装器封装、运行时校验及 SHA256 校验均通过。

- 产物：`dist/desktop/Deep Legends Setup 0.12.55-public.exe`（106 MiB）。
- 安装包 SHA256：`f224c5a5144f17a1cfb9af64b4546bc5e3dc89d101ce7379900c75aa33b456ea`。
- 后端 SHA256：`4d70c8fc113efa4b2d90433a7c1c91da6c7a31981b7bb8c5a17c0e92a282cfb6`。
- 收据/校验表：`history/reports/r191/release-build.json`、`SHA256SUMS-public.txt`；构建日志 `/private/tmp/r191-public-build.log`。
- dist 只保留可识别的 `-public` 安装包，未留下与 private 同名的包。

## Windows 待验

安装 0.12.55 后打开国服构建页，确认缺变量时显示固定效果而不是一列 0；需要时导出日志查看 perk_stats_presence。打开一场自己带致命节奏的对局构建页后导出日志，确认 perk_effect_sample 并发回；拿到真实样本后另开修正表工单。打开海斗构建页时确认其他卡片图标不闪。

如需显示致命节奏的数值，请打开一场带致命节奏的对局构建页后导出日志。
