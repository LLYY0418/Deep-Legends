# R192 执行账本

R191 基线已提交：`0ee9780c`。本次修正外部战绩列表摘要未补全与重试失效，版本目标 0.12.56。

## 实现

外部 view.render(id, options) 在纯展开/收起/详情页切换时保留摘要；加载状态变化、成功替换对局数据、失败状态变化显式传 full:true，仅替换目标卡片并完整绑定全部控件。两条分支都调用同一重试按钮绑定函数，通过节点标记确保一次绑定。主总览及浮层的 rerenderMatch / replaceMatchEntry 保持 R191 行为。

## 验证

| 验收项 | 验证入口与结果 |
|---|---|
| 1 补全名单、主体高亮、其余两卡 DOM 保持 | `backend/web/r192.test.cjs` 第 1 项，通过 |
| 2 首次 reject、重试第 2 次成功展开 | 第 2 项，通过；补全后收起/再展开无需再次请求 |
| 3 loading 中连续两次点重试只加一次请求 | 第 3 项，通过，累计请求数为 2 |
| 4 无 loader 展开/收起保留摘要 | 第 4 项，通过；整卡、摘要、其余卡片引用均保持 |
| 5 斗魂伤害/承伤从占位更新为补全值 | 第 5 项，通过；夹具 98209 / 47140 |

5 项使用真实 index.html 和全套生产前端脚本，实际 mount、hydratedExternalMatch、renderMatch 和事件流程，不以手写摘要模拟生产渲染。初版夹具错误地请求了 demo 未覆盖的技能目录、数字断言误用了千位分隔格式，均已修正，仅调整测试环境与断言。

两个变异全部被断言杀死（无语法错误/运行时缺依赖充数）：移除 full:true 分别令第 1、5 项失败，移除重试绑定令第 2 项失败。结果见 `history/reports/r192/mutations.json`，执行脚本 `desktop/r192-mutants.py`。

R190/R191/R192 专项合计 19/19 通过。`desktop/overview-render.test.cjs` 55/55 通过（244.2 秒），其他 backend/web、desktop、scripts 全量 1079 项：1076 通过、3 项平台条件跳过、0 失败（37.2 秒）。两个互补批次覆盖全部 Node 测试文件，合计 **1134 项，1131 通过、3 平台跳过、0 失败**。日志分别为 `/private/tmp/r192-target-node-final.log`、`/private/tmp/r192-overview-render.log`、`/private/tmp/r192-node-other-suites.log`。

R86 外部卡片护栏的变异注入点同步到新的 render(id, options) 签名，保持其禁止重建无关卡片的断言。主总览/浮层 `rerenderMatch` 与 `replaceMatchEntry` 两个函数和 R191 基线逐字比较一致；R191 的 9/10 项继续通过。独立只读复核未发现实现缺陷。

## Chromium 验证

`node desktop/r192-browser.cjs` 通过。加载真实生产 index.html、CSS、懒加载英雄模块与 demo API，点击英雄页→斗魂竞技场→安蓓萨→高手第一名对局。只通过夹具拦截详情接口，使第一次返回 503、第二次返回完整参与者；生产页面及渲染器没有替换。

- 失败态：请求次数 1，显示“完整详情未加载”和已绑定的重试按钮，其他两张卡片引用保持。
- 重试补全：请求次数 2，名单改为补全名字、主体高亮、伤害 98209 / 承伤 47140，失败块消失并展开详情，其他两张卡片引用保持，页面异常集合为空。
- 截图：`history/reports/r192/arena-details-failed.png`、`arena-details-hydrated.png`；测量/行为结果 `browser.json`。
- 截图、名字及数值均为手写/演示夹具，不代表 Windows/真实 Riot 数据实测。

## 构建

完整 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh` 成功，版本 **0.12.56**，key mode **public**。Go 全量 **1686 项**五分片全部通过（78.1 秒，包含 R190/R191 后端专项），backend vet、installer 全量及 vet 通过。NSIS、安装器封装、打包运行时校验、指纹及校验表生成全部通过。

- 源码、后端与打包运行时指纹：`45812c8aa84f`；最终重新计算一致。
- 产物：`dist/desktop/Deep Legends Setup 0.12.56-public.exe`，已重新计算 SHA256 与收据逐字核对。
- 安装包 SHA256：`c3a5b909a4aa977f80c671a56c0d8ed76d3675e0cca8781e86b748aa07f3bc7c`。
- 后端 SHA256：`1532c0b58e9ae2e4a49fbcec9bfe8a4c659c19da47710c023fa5beb2201e109e`。
- 收据与校验表：`history/reports/r192/release-build.json`、`SHA256SUMS-public.txt`。
- 构建日志：`/private/tmp/r192-public-build.log`。
- dist 仅保留明确的 `-public` 安装包，无与 private 同名的包。

本地全部实现及验收完成；以下 Windows 真机项目仍待用户验证。

## Windows 待验

沿用 R191：国服构建页在变量缺失时显示固定效果，不是一列 0；带致命节奏的对局导出日志取得 perk_effect_sample；海斗构建页其他卡片不闪图标。

新增：英雄页斗魂竞技场高手第一名对局点开后，摘要名单、主体高亮和伤害/承伤行正确；断网点开失败，联网后点重试可加载完整详情。
