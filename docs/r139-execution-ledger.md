# R139 执行台账：阵营位置播报设置精简

工单：`docs/WORKLIST-R139-BROADCAST-SETTINGS-DECLUTTER.md`。接续 R138，产品版本 0.12.19；本轮不单独打包。

## 改动

- `backend/web/suite.js` 只在 `positionBroadcast.visibility === "team"` 时渲染「附带己方英雄」；`self` 时选项节点不存在。`camp` 与 `assignedPosition` 不再作为输入控件展示，后端提供的三项格式声明保持原样。切换播报范围经过现有 `saveWatch()` 重新渲染设置面板。
- `backend/web/suite.css` 使用与现有 `.suite-switch` 相同的药丸开关，新增左标题/说明、右开关的 `.watch-broadcast-row` 布局；删除原生勾选框的旧样式。
- `backend/watch_rules.go` 将 `AssignedPosition` 的新装默认值设为 `true`，读写存档时通过 `normalizeWatchSettings` 强制为 `true`；`TeamComposition` 默认仍为 `false`。播报消息拼接逻辑和 `positionBroadcastOptions()` 均未改。

## 验证

- `backend/web/r115.test.cjs` 验证 `self` 时 0 个输入、`team` 时唯一的 `.suite-switch input[data-watch-broadcast="teamComposition"]`，且默认/选中状态跟随设置。
- `backend/r115_test.go` 更新旧默认值断言，新增真实旧存档 JSON `"assignedPosition":false` 读盘转为 `true`，并验证写盘不能保存 `false`。
- 独立变异：用 R139 前的三项勾选框代码替换前端分支，新 Node 测试 FAIL（3 ≠ 0）；Go `-overlay` 单独移除归一化赋值，新旧存档测试 FAIL。原仓库源码未被变异修改。
- 真 Chromium 使用演示设置面板和生产 CSS/JS，`self` 无选项节点；`team` 只显示一行，`appearance: none`、默认关闭，切换状态可保存，再切回 `self` 时节点消失。已人工检查截图，开关与同卡片右上方的 `.suite-switch` 外观一致。截图：`r139-validation/broadcast-self.png` 与 `broadcast-team.png`。

| 检查 | 结果 |
|---|---|
| R139 定向 Go/Node 测试与双变异 | PASS / 变异按预期 FAIL |
| R139 Chromium 真实设置面板 | PASS |
| R100 / R117 Chromium 护栏 | PASS；R117 `unstyledFrames=0` |
| `go vet ./...`、`go test ./...`、`go build -o /private/tmp/r138-r139-backend ./backend` | PASS；Go 全量约 205 秒，产物为 arm64 Mach-O |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs` | PASS；954 项：953 通过、1 项原有跳过 |

Windows 0.12.19 安装包仍对应 R136 源码；R138/R139 更改留待下一次合并发版。
