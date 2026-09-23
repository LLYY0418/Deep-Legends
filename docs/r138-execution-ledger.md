# R138 执行台账：头像间距、详情放大与战绩详情当前玩家

工单：`docs/WORKLIST-R138-FACADE-ICON-SPACING-AND-DETAIL-DIALOG-SIZE.md`。基线 HEAD `cd44bcd9`，版本 0.12.19。用户同轮追加：总览战绩详情「玩家」列突出当前玩家名称。本轮不递增版本，不单独打包。

## 头像列表与详情

- `backend/web/app.css` 只把 `.facade-grid.is-icons` 的 `gap` 从 `12px 10px` 调至 `20px 16px`；列宽保持 92px，`.skin-art` 保持 92px 正方形。旗帜 `.is-banners` 仍为 `12px 10px`，皮肤/炫彩规则未动。
- `backend/web/favorites-facade.js` 头像详情按较长原图边长 ×2 显示，上限 512px；128px → 256px、300px → 512px。用户接受放大后的模糊；图片源和 `object-fit: contain` 未变。旗帜详情分支、320px 高度上限及加载前 128px 占位未变。
- `backend/web/r123.test.cjs` 保留 92px 与正方形断言，新增行列间距下限、旧值禁用和旗帜间距保护。`backend/web/r130.test.cjs` 更新 R130 的头像 1:1 旧断言，注释记录 R138 用户决定；尺寸测试从生产源码读取常量，防止测试桩与实现漂移。
- R117 的 CSS `gap` 不同取值预算原为 54；R138 按工单新增唯一取值 `20px 16px`，`backend/web/r117-style.test.cjs` 将预算精确调整为 55，其余 `border-radius`、`padding` 预算不变。

真实 Chromium 对比截图位于 `docs/r138-validation/`，1200px 与 960px 各有修改前/后的网格和详情图。浏览器夹具使用 128px 测试头像，加载生产 CSS 与尺寸计算函数；两种宽度测得相邻头像列间隙 10 → 16px、行间隙 12 → 20px，格子 92px 未变；详情显示宽度 128 → 256px。

## 用户追加：总览战绩详情的当前玩家

- `backend/web/gameplay.js` 仅在展开后的普通对局详情表格和斗魂详情玩家列，根据 `match.subjectParticipantId` 与 `participantId` 的匹配给名称添加 `is-current-player`；缺少有效 ID 时不猜测。折叠战绩列表和其它玩家名称不变。
- `backend/web/gameplay.css` 仅为详情里的该名称使用主题令牌 `var(--primary-strong)`、字重 750。真实 Chromium 计算样式：普通名称 `rgb(232, 234, 240)`，当前玩家 `rgb(240, 190, 92)`，字重 750。`backend/web/r119.test.cjs` 运行真实 `matchTableRows`，断言只有目标行获得样式类。

## 对抗变异与验证

变异均放在 `/private/tmp/r138-mutations/` 的独立源码副本，仓库文件未修改：头像 gap 改回 `12px 10px`、详情倍数改回 1、当前玩家判定恒 false，对应的三个定向测试各自 FAIL；正常源码的定向测试 PASS。

| 检查 | 结果 |
|---|---|
| R138 Chromium 1200/960 网格、弹窗及当前玩家计算样式 | PASS；前后共 8 张截图 |
| R100 / R117 Chromium 护栏 | PASS；R117 `unstyledFrames=0` |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs` | PASS；954 项：953 通过、1 项原有跳过。首次运行仅 R117 旧 gap 预算失败，修正后全量复跑通过 |
| `go vet ./...`、`go test ./...` | PASS；Go 全量约 200 秒 |
| `go build -o /private/tmp/r138-backend ./backend` | PASS |

Windows 0.12.19 安装包仍对应 R136 源码；R138 前端更改按工单留待下一次合并发版。
