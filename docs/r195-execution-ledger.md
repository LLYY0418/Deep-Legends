# R195 执行账本

版本：0.12.59。工单：[R195](WORKLIST-R195-RUNE-EFFECTS-KEYSTONE-VALUES-NO-SHARDS-PREMADE-ICONS-UPDATE-DETECTION-AND-INDEX-CLEANUP.md)。

## 实现与覆盖

| 工单项 | 完成情况 |
| --- | --- |
| P1 碎片 | 删除效果区碎片 DOM 与 CSS；共享符文板保留。 |
| P1 致命节奏 | 8008 仅显示 var2 的已造成伤害，计入头部伤害汇总。样本 `[932,972,0]` 显示 972。 |
| P1 神奇之鞋 | 删除 8304 覆盖，客户端三变量时间模板显示 7:30；非法分钟或个位数字丢弃。 |
| P1 基石 | 44px 图标；有数值时副标题仅系名，18px 主题色数值；无数值保留描述。 |
| P2 组队 | 20×20、4px 圆角头像位于名字前，间距 8px；仅代理 URL、现有图片队列；未选英雄留占位，隐藏名字仍打码。 |
| P3 安装识别 | 去空白和引号、Clean、Windows 长路径、EvalSymlinks 后比较；失败保留原值。默认目录有卸载程序时兜底 installed；便携环境优先。 |
| P3 诊断 | 增加七项布尔值/计数，不记录路径；保留原始与规范化匹配结果供排障。 |
| P4 索引 | 按用户确认关闭旧工单并注明接手关系；仅 R153、R156、R195 进行中；已关闭工单/账本归档，修复相对链接。映射见 [归档清单](history/reports/r195/index-cleanup.json)。 |

## 自动验证

- Node 专项：32/32 通过；R195 九项断言包括数字、模板、布局、头像、占位和打码。
- 四项变异均由指定断言检出，见 [变异报告](history/reports/r195/mutations.json)。
- Go 专项：`go test ./backend -run '^(TestR195|TestR186InstallDetection|TestUpdate)' -count=1` 通过；覆盖引号、斜杠、注入短路径转换、默认目录兜底、不同目录和诊断隐私。
- Chromium 演示：改前与改后宽/窄符文布局及组队弹窗通过；真实图片队列加载两个本地英雄头像、一个占位。
- 完整 Node、Go、vet、public 构建与 GitHub 发布核验：执行中，结果在发布后补齐。

## 演示截图

截图使用演示玩家和符文数据，不代表 Windows 真机验收。英雄头像来自仓库资源；符文小图使用演示素材。

| 场景 | 改前 | 改后 |
| --- | --- | --- |
| 符文宽屏 | ![改前](history/reports/r195/before-runes-wide.png) | ![改后](history/reports/r195/after-runes-wide.png) |
| 符文窄屏 | ![改前](history/reports/r195/before-runes-narrow.png) | ![改后](history/reports/r195/after-runes-narrow.png) |
| 组队弹窗 | ![改前](history/reports/r195/before-premade.png) | ![改后](history/reports/r195/after-premade.png) |

## 发布与升级边界

0.12.59 将按工单发布正式 GitHub Release Latest，附 public 安装包、SHA256 和 latest.json。正式发布版本保留公开历史记录；新版本接替 Latest，不删除旧版本或改回草稿。

0.12.50：旧 R185 账本声称正式发布的 Release ID `401671502` 现返回 404；当前 v0.12.50 为另一个草稿记录 `401679319`，创建于 2026-10-02 09:04:27 UTC，未正式发布。现有证据无法确定旧记录删除时间、操作者，或旧账本 ID 是否有误。已在 [R185 账本](history/ledgers/r185-execution-ledger.md) 更正当前状态，不推断原因。

0.12.58 → 0.12.59 使用 0.12.58 内的旧安装位置判断，可能仍失败。失败时使用发布页手动安装一次 0.12.59；之后升级使用本工单修正后的判断。自动测试和构建不替代这一次实际升级。

## 用户真机验收（待完成）

1. 在 0.12.58 检查更新，验证 0.12.59 的立即升级、下载、重启升级；失败时手动安装。
2. 带致命节奏的真实对局详情显示伤害数值，效果区没有属性碎片。
3. 选人或对局的组队弹窗显示头像加名字。
4. 导出日志，核对 `update_install_detection result=installed`；仍 mismatch 时使用新增诊断字段继续定位。

R195 因以上真机验收保留进行中。
