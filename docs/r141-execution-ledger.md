# R141 执行台账：优先旗帜可见尺寸

工单：`docs/WORKLIST-R141-BANNER-GRID-VISIBLE-SIZE-OVER-COLUMN-COUNT.md`。基线 `9f1f60fd`，产品版本 `0.12.19`，本轮不递增版本或打包安装包。

## 尺寸取舍

R140 根据用户当时选定的「1200/960px 都保留 4–5 列」硬指标，只能把 150px 增到 152px；实际图案几乎没有变大。用户现改为优先看清旗帜，允许列数变成 3–4 列，因此本轮重新测量并选用 200px 列宽。

使用生产页面、生产 CSS/JS 和 [真实客户端旗帜资源镜像](https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/regalia/bannerskins/lny2023.png)在 Chromium 中测量。样本原图为 `580 × 1480`，图像比约 `0.3919`；演示数据中的拥有标记仅为夹具，不代表用户真实库存。

| 窗口宽 | 网格可用宽 | R138 150px/旧间距 | R140 152px/20×16 间距 | R141 200px/20×16 间距 |
|---|---:|---:|---:|---:|
| 1200px | 882.22px | 5 列，卡片高 403.64px | 5 列，卡片高 408.75px | **4 列，卡片高 531.22px** |
| 960px | 657.56px | 4 列，卡片高 403.64px | 4 列，卡片高 408.75px | **3 列，卡片高 531.22px** |

候选列宽 190–208px 在两个窗口下都得到 4/3 列；200px 与工单的可见尺寸目标吻合，比 R140 宽 48px（约 31.6%），图片区域高从 382.75px 增到 505.22px。212px 在 960px 下只剩 2 列，不满足本轮列数范围。卡片 `contain-intrinsic-size` 按实测高度从 409px 更新为 531px。旗帜间距仍为 `20px 16px`，头像网格与详情弹窗代码未变。

## 验证

- [三阶段 Chromium 截图](r141-validation/)：1200px、960px 各有 R138 原始列宽/间距、R140 后、R141 后一张。肉眼检查，R141 卡片和旗帜图案比 R140 明显变大。
- `desktop/r140-banner-layout.cjs` 的现行布局断言已更新为两个窗口均 3–4 列、列宽 ≥190px（实际 200px），并保留真实图片比例、`object-fit: contain`、未拥有态灰度/透明度和 30px 锁图标的检查。用 `R141_LAYOUT=1` 只运行本轮列表三阶段截图；旧的 `R140_LAYOUT=1` 路径继续验证详情 640px、无滚动条和界面缩放。本轮详情回归通过。
- `backend/web/r130.test.cjs` 已去掉 152px/4–5 列旧断言，改为新宽度和 531px 虚拟占位高度。独立临时 CSS 变异把列宽改回 152px：单测 FAIL，Chromium 在 1200px 得到 5 列而非 3–4 列，也按预期 FAIL；未修改仓库源码。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：通过，954 项中 953 通过、1 项原有跳过。
- `go vet ./...`、`go test ./...`、`go build -o /private/tmp/r141-backend ./backend`：全部通过；Go 测试约 205 秒，本机构建产物为 arm64 Mach-O。

验证边界：本机 Chromium 演示页面与真实游戏旗帜图片已核验；本轮未在 Windows 客户端连接真实用户库存做手工检查。R140 台账和截图保留为当时 152px 决策的历史记录；当前产品行为由本台账和 R141 截图记录。
