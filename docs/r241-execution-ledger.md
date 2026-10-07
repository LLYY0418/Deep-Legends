# R241 执行账本

日期：2026-10-07（北京时间）。工单：[R241](WORKLIST-R241-MATCH-DATA-TAGS-OWN-STYLE-NO-ICON-MODE-RULES-SINGLE-LINE-RIGHT-OF-ITEMS-AND-HEAD-REFRESH-REFETCHES-11MB-EVERY-OPEN.md)。本地 P1–P6 实现与自动验收；真实客户端及 Windows 用户验收待补，因此工单继续保留在进行中目录。

## 范围与用户追加要求

以进入工单时的脏工作区为基线，保留 R235 及授权工作。未提交、推送、占用发布版本号或发布；R238/R222 发布闸门不变。实际 package.json/lockfile 版本 **0.12.75**；不要使用工单文字中的历史版本替代实际构建版本。

本轮用户追加：「不要加什么第一……输出第一就改成输出……把涉及到名次的都去除」。最终数据标签只显示输出、伤转率等维度名，**不显示第一/队内**；全场/队内最高与实际数值只放在已有悬停说明里。战绩标签条的数字名次胶囊去除，MVP/SVP、多杀、关键词与零阵亡保留。全场/队内层级继续用填充强度、描边与字重区分。

基线目标文件副本：[baseline](history/reports/r241/baseline/README.md)。运行环境重载删除了 `/tmp/r241-baseline/`，随后从已保存的增量 diff 重建进入本单前的目标文件，仅操作报告目录，未回退生产源码。本单增量：[r241-only.diff](history/reports/r241/r241-only.diff)；[归属清单](history/reports/r241/scope.json)。本轮另一个授权会话改过 main/preload/license-window 等文件，清单逐项列明；没有将它们回退或并入 R241 增量。构建使用共享工作区最终源码，不能把整仓库 diff 当成本单变更。

## P1 / P2：标签与地图矩阵

| 色系 | 标签 | 既有变量 |
| --- | --- | --- |
| 输出 | 输出、伤转率、击杀 | `--tag-unstoppable` |
| 控制 | 控制 | `--tag-resilience` |
| 承受与支援 | 承伤、治疗、护盾 | `--tag-dedication` |
| 经营 | 补兵、经济、拆塔、参团 | `--tag-leader` |
| 技巧 | 单杀、越塔、塔之子、压刀、好钩 | `--tag-innocent` |

数据标签 20px、5px 圆角、2px 7px 内边距、10px 字号；无图标、星号、渐变或阴影。全场 26% 底/75% 描边/700 字重；队内 14%/55%/600。浅色的固定深色字与浅底直接共用既有关键词调色板，未复制新色相；全场仍用 26% 底。现有关键词、零阵亡与多杀的外观没有改动。

真实 Chromium canvas 将 CSS color-mix 颜色转成 RGB，再按 WCAG 相对亮度计算；覆盖页面所有数据标签（包括隐藏标签）的五个色系与全场/队内强度。深色最低 **5.3939**；浅色最低 **5.5863**，均 ≥4.5。[完整颜色与比值](history/reports/r241/chromium.json)。

| 标签 | map 11 | map 12 | map 30 | 其他 |
| --- | --- | --- | --- | --- |
| 输出/控制/治疗/护盾/承伤/击杀 | ✓ | ✓ | ✓ | ✓ |
| 伤转率/好钩 | ✓ | ✓ | — | — |
| 参团/补兵/经济/拆塔 | ✓ | — | — | — |
| 单杀/压刀/越塔/塔之子 | ✓ | — | — | — |
| 多杀/关键词/零阵亡/MVP/SVP | 沿用 | 沿用 | 沿用 | 沿用 |

按 mapId 在计算前过滤，不依赖 queueId 白名单。Node 同一份完整参赛者夹具覆盖四种地图；排除维度设置抛错 getter 也不被读取，证明过滤发生在计算之前；缺失字段仍不能推断最大值。没有标签时不渲染空容器。

## P3：单行与隐藏项悬浮窗

合并重复 match-badges 规则；match-build nowrap、装备固定宽度、badges flex:1 1 0/min-width:0、collection flex nowrap。顺序为 MVP/SVP > 多杀 > 关键词 > 数据 > 零阵亡；MVP/SVP 不能隐藏。按实际宽度隐藏末尾项，+N 为中性描边 button，无原生 title。

单个 body/fixed 面板同步响应 hover/focus，触屏点击切换；默认正上方、窗口边缘夹取、顶部空间不足向下翻转，带三角。面板只克隆隐藏项，保留每项的说明和键盘焦点；多杀 SVG 克隆使用独立渐变 ID。tooltip z-index 55，高于面板 54。Esc、失焦、移开、滚动、resize、窗口失焦及源卡片移除都会关闭。MutationObserver 仅在面板打开期间监听源卡片存活，关闭即断开。

Node 覆盖隐藏集合相等、不含已显示项、评分保留、第二面板替换、Esc/blur/scroll/touch/移开/源卡移除；保留 ResizeObserver 只在宽度变化时重算、离屏不读布局的护栏。

Chromium 测量采用同一张卡的 items 和 collection DOMRect：**所有 1280/900/780、深浅、1x 场景 top 差为 0px，标签高 30px = 装备高 30px**；1.5x 两者均 45px，不换行。受限空间无评分卡仅显示 +6，仍同高同顶。顶部翻转使用真实生产按钮/隐藏项克隆到 body 顶端 12px 的边界夹具，验证生产定位函数，不替代真实客户端截图。

截图均为生产前端 + 明确标记的合成 API 数据，**不是用户账号的真实对局**：

- 深浅 × 斗魂/大乱斗/峡谷 × 1280/900/780/1600/1920：`history/reports/r241/{dark,light}-after-{arena,aram,rift}-*.png`。
- [深色上方悬浮窗](history/reports/r241/dark-popover-top.png)、[浅色上方悬浮窗](history/reports/r241/light-popover-top.png)；`*-popover-bottom.png` / `*-popover-tag-tooltip.png` / `*-only-more-780.png`。
- `*-before-cards-*-zoom-*.png` 与 `*-after-cards-*-zoom-*.png` 为同一夹具的改前改后；`chromium-before.json` 由进入本单的 R235 工作区副本提供，未使用旧 HEAD 假装改前。

## P4：赛季头刷新鲜度与流量

后台头扫先无 tag 请求最新 1 场，关闭历史页缓存，比较独立 HeadGameID。老缓存没有该标记时，最新 ID 已在 GameIDs 也足以跳过；否则首次扫描建立无标签标记，支持最新场属于未统计模式的情况。

无新场只保存头部标记并记 skipped=true/skip_reason=no_new_game；同账号 **60 秒内（包括手动 fresh）完全不发头部请求**，记 recent 与 0 字节。新场才进双流并发扫描；已有累计时逐次 count=1，遇已有 ID 立即停，冷缓存沿用批量初扫与后台回补。标记时间独立于回补 UpdatedAt，回补不能续期或覆盖新标记；取消时不能推进标记，新增取消夹具覆盖该边界；标记写入与回补聚合用同一文件锁。

诊断增加 skipped、skip_reason、new_games；完整请求数和实际 HTTP 字节仍沿用 overviewLoadCost。

| 场景 | 字节数/请求 | 证据边界 |
| --- | --- | --- |
| 改前重复打开 | 11,632,735 / 2 条头部流 | 工单给出的真实旧日志，未重新联网采集 |
| 改后无新对局 | **1,261 / 1 个无标签 count=1** | 假 SGP 的真实 HTTP 响应字节；不是用户网络的测量值 |
| 改后新增 1 场 | **3,796 / 小检查 + 新增/旧 ID 停止** | 同一 SGP 夹具，旧聚合 2→3，未固定请求 50 场 |
| 改后 60 秒内 | **0 / 0** | 普通和 fresh 连续打开均验证 |
| 改后真实客户端 | 未采集 | 需用户日志 season_stats_head_refresh 验证 |

冷夹具初始化为 3,784 字节。[后端定向日志](history/reports/r241/go-focused-final.log)；R231 并发写入/回补抑制与 R235 双流首推/逐页聚合护栏保留，并纳入 race。

## P5：收藏诊断与一小时负缓存

重试三次耗尽增加 blank_kinds、blank_samples（最多3个类别+数值ID）、last_error_kind。来源无类别/数值 ID 时保持 `类型未知` / 0，不能猜皮肤、表情或网络原因。

合成夹具实际记录：

```json
{"event":"collection_data_retry_exhausted","attempts":3,"blank_entries":1,"blank_kinds":{"类型未知":1},"blank_samples":[{"kind":"类型未知","id":0}],"last_error_kind":"empty_identity"}
```

**真实日志新样本尚未取得**，不能把该 fixture 当成已查明用户的空条目。

负缓存按公开目录内容（皮肤 ID/名称、loot metadata）与原始条目结构/身份的摘要键持久化，1小时、最多512条；不保存账号、数量或所有权明细。目录内容改变或超时立即失效；未知身份只保留来源结构的摘要，不赋予物品身份。重新读取的真实库存数量和普通条目不受影响，只停止已耗尽空记录的 DataPending 重试。测试模拟3次 timer、耗尽记录、重启后0次、TTL、目录变化及 ownership 不进入目录版本。新增本地写入已补入 stores 声明与写入点护栏；未根据未知样本改变收藏页分类/过滤。

## P6：中间列宽度与性能

宽窗口改成 champion | minmax(96px,1fr) KDA | minmax(188px,1.4fr) 数据三轨，数据行标签靠左、数值靠右；去掉空尾列。保留约64px数据区右留白 + 8px列间距，名单沿用 clamp(150px,18cqi,190px)。900/780 窄窗口及容器断点继续原有布局，只把 build/tag 单行化。

| 1x 宽度 | 改前：斗魂/大乱斗/峡谷 | 改后（三者相同） |
| --- | --- | --- |
| 1280 | 120.08 / 120.08 / 128.08px | **72px** |
| 1600 | 110.72 / 110.72 / 118.72px | **72px** |
| 1920 | 234 / 234 / 242px | **72px** |

深浅结果相同。1.5x 的1600/1920物理间距为108px（=72×1.5）；1280时走窄容器布局，名单隐藏，距离不适用。完整数据：[布局对照](history/reports/r241/layout-comparison.json)。summary min-height 仍118px（边框计入时卡片120px），未改变 content-visibility 或估算机制；去掉标签独占的第二行，避免之前152px的额外增高。

生产 Chromium 200场全部→灵活→全部：深 **11.60ms**、浅 **16.70ms**；同一200节点复用，切回窗口长任务0。R235深17.6/浅17.7ms，均未超过2倍。初次合成页面未观测到>200ms任务；不将该小夹具当成真实14,479节点加载峰值已消除。

## 最终验证与构建

生产源码指纹 **df770a2f065d**。最终全量运行前后与构建保持一致；无安装包、tag、提交或发布，Windows可执行文件仅交叉构建，未执行/升级验收。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| Go 全量 -count=1 | PASS，169.551秒 | [go-full.log](history/reports/r241/go-full.log) |
| Go vet | PASS | [go-vet.log](history/reports/r241/go-vet.log) |
| P4/P5、R231/R235头扫相关 race -count=1 | PASS，5.336秒 | [go-race.log](history/reports/r241/go-race.log) |
| Node web/desktop/scripts 全量 | PASS，1337项：1333通过、4条件跳过、0失败；154.543秒 | [renderers-all.log](history/reports/r241/renderers-all.log) |
| JS syntax | 289文件 PASS；3个后续普通mock修改也逐项node --check通过 | [js-syntax.json](history/reports/r241/js-syntax.json) |
| 真实 Chromium 深浅/各宽度/1x1.5x | PASS，24组，异常0 | [chromium.json](history/reports/r241/chromium.json) |
| P2/P3/P4/P6真实源码变异 | 4项全部FAIL，且恢复原始字节 | [mutations.json](history/reports/r241/mutations.json) |
| git diff --check | PASS | 最终检查 |
| macOS arm64 / Windows amd64 public 后端 | PASS；版本/指纹/无Riot key校验及macOS临时目录自检通过 | [构建回执](history/reports/r241/public-build.json)、[日志](history/reports/r241/public-build.log) |

旧布局快照（R87/R88）按本单新布局更新，窄屏规则仍钉住。普通Go并行测试原先只给无tag请求加延迟，现精确匹配前台count=20战绩请求，排除后台流扫描与count=1小检查，连续10次通过，未放宽并行耗时/重叠断言。三份普通Electron测试mock缺现有ipcRenderer监听API，补齐mock方法后原全部断言保留；没有修改授权生产代码或授权测试来换通过。另一授权会话同步更新了原有窗口/启动夹具；旧失败尝试保留在 attempts，不作为通过证据。

Node全量使用3个真实文件worker，与R235相同，以限制同机CPU争用；辅助脚本采用报告目录的绝对路径，子进程切换目录仍可加载。无过滤或放宽断言；最大单文件 43.781秒，4个条件跳过不算Windows真机验证。构建产物：

- `dist/r241-validation/loot-service-0.12.75-r241-darwin-arm64-public`
- `dist/r241-validation/loot-service-0.12.75-r241-public.exe`

需要用户真机确认：短标签观感、地图过滤、装备右侧单行/悬浮窗、中间块留白；重复进入总览的小字节/recent诊断；真实收藏 blank_samples 与下一启动不再3次重试。R241不推断这些真机结果。
