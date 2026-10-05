# R211 验证记录

本轮无构建/打包/发布，版本仍 0.12.71。运行时 P1/P2/P6 未接入。

| 检查 | 结果 / 证据 |
|---|---|
| go test ./backend | 全量通过，265.080s；go-all.log |
| go vet ./backend | 通过，无输出；go-vet.log |
| go test -race ./backend -run 'TestR211\|TestSeasonStats\|Test.*Timeline\|TestMatchHistoryCache' | 通过，28.625s；go-race.log |
| node --test backend/web/*.test.cjs | 最终全量906/906通过，18.090s；node-all.log |
| Python 五个采集/校准脚本 py_compile | 通过；不会重新训练或重评新集 |
| P5 变异 | 将 finalize 改回写响应均值到累计对象，TestR211SeasonFinalizeNeverMutatesAccumulator 失败，成功检出；p5-mutation.json |
| Chromium | 单双/海斗/斗魂/熟练度/横幅×1280/780px，10个页面截图通过；额外生产熟练度弹窗边界与图标检查、浅色横幅截图；layout.json |
| P3 增强交互检查 | 实际绑定选择按钮，方向键移动焦点、点击后记忆并重绘装备/技能/海克斯，9/9 定向通过 |
| 报告一致性 | 冻结候选 hash、一次评估标记、新集数量/不重叠、原门槛与未完成状态均一致；final-artifact-check.json |
| git diff --check | 通过 |

生产 CSS 与渲染函数生成截图；头像和玩家名称是布局夹具，熟练度数值来自真实 TOPKING 响应，徽章是实际客户端108px crest。加载使用生产 image-queue.js；选择条5v5一行、斗魂宽一行窄两行，熟练度窄5列，不产生页面横向滚动。弹窗使用生产 masteryTooltipContent 和绑定，40px圆形头像、徽章存在，左右/上下均在视口内，杰斯3246/11000点验证通过。截图见 ranked-/mayhem-/arena-/masteries-/banner-wide.png 与 -narrow.png、mastery-popover-narrow.png、banner-light.png。

设计差异：生产使用客户端圆形金边头像和完整 crest，非设计稿占位徽章；56px头像和54px真实徽章保持比例。未接入的英雄表、评分悬停、25种标签色板没有截图，不能用横幅浅色截图代替。P1评分变异、P2关键词测试、P6聚合/表格测试未执行，依赖评分放行。

真实验收边界：P5整季重扫耗时/下载量、国服LCU熟练度原始字段、OP.GG点亮状态一致、Windows客户端交互及赛后真实resolved事件未核验。已通过的10:35事件顺序来自工单描述夹具，不是读取原始日志回放。详细状态见 [执行账本](../../../r211-execution-ledger.md)。
