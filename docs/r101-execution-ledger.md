# R101 执行账本（2026-09-17）

范围：按 R101 工单执行，保留共享工作区已有 R95–R100 改动。未提交、未推送、未发布。工单随附的真机结论作为既有证据归档；本机 macOS 自动化 fixture 不冒充 Windows 国服实测。

## 已实施与尚未解锁的边界

| 工单 | 实施情况 | 验证 / 边界 |
| --- | --- | --- |
| P0 | R1–R11、W1–W4 真机状态归档到 r99-probe-results.md；停止 R7/R8/R11 请求 | 保留历史探测编号，现仅执行八个仍有效的只读探测 |
| P1 | W5 手动选 owned 非当前头像，失败/取消也用独立上下文恢复；401 才试 W5-b，恢复失败停止后续探测；W5-c 读取两版 signedInventory 的状态与顶层字段 | **未取得新真机 W5 结论**。不调整正式头像 PUT 实现；不猜 inventoryToken 格式，带 token 的写入分支暂未实现，也不宣称三条路径均失败 |
| P2 | W6 选另一面已拥有旗帜，优先库存 contentId，缺失时回退目录；复制完整 slot，仅替换 itemId/contentId；保留 data 与 itemId 标量类型；PATCH 后按原 loadout ID GET 回读；最后完整恢复 | **未取得真机 W6 changed=true**，不开放正式旗帜写入。200/no-op 记录 ok=false；restore 成功仅指请求成功，不冒充回读确认 |
| P3 | 头像拥有态使用 SUMMONER_ICON 的 itemId/owned，缓存按 client 隔离；失败/空库存保留目录并标记 unavailable；计数区分原始 total 与可显示图像子集 | 已知拥有态默认只看已拥有；未拥有置灰、锁标、disabled；未知拥有态可浏览、不可应用；不下发库存元数据 |
| P4 | 新 facade_banners.go 和只读 GET /api/facade/banners；字符串 id/idSecondary；35 条筛选规则；只使用 regalia/v3 isOwned；保留国服专属标签 | 弹窗全部/已拥有/国服专属，按 localizedName 排序；未拥有 disabled；没有正式写入按钮/handler，明确显示等待客户端验证 |
| P5 | 删除配色行、previous-banner 按钮、后端 action case 与诊断枚举；停止 W4 | writeChallengePreferences **本体未改**；保留 clear-challenges/clear-title 的 R66 保全回归；旧 action 400 且零 LCU 请求 |
| P6 | 两侧三种策略均显示锁定等待；非等待策略显示 0 且输入/步进禁用 | 不改保存值，切回恢复原 10 秒；保留 disabled change 守卫并为步进事件添加同等保护 |
| P7 | 新默认 champSelect.enabled=true；已有显式 false 不覆盖；旧 schema 无对象采用新默认 | 默认序列仍为空，其他自动规则未打开。连接初始化 + planning/ban-pick/finalization 的四模式两侧流程测试零 PATCH/POST，另以 side=true 验证空池门禁 |
| P8 | 头像、旗帜卡位于左列预览之后/说明卡之前；当前头像缩略图；删预览长说明与死 CSS；旗帜上下排布；短文案“上赛季段位”、nowrap、局部行高 18px | 保留 facade-commit 原确认文案；背景/聊天卡内容保留；真实 Chromium 三视口两缩放检查列位置、无横向溢出、34px 控件 |
| P9 | Rookie 现有种子补官方 league-v4 单双排段位，精准 queueType，罗马小段映射，大师以上 1；无 solo 为 unavailable | 独立 proseed-rank:v1 / 6h 缓存；总览 ranks / 3m 不变；R100 单次准入 100ms 继续使用；额度/网络失败保留上次完整 seed 快照；不添加账号或天梯名次 |
| P10 | 更新 Wenbo 三处旧表述，关联 33 人核对材料；记录 icon 401、配色 400 与槽位存在 | 2026-09-08 的 90 账号/1 缺号保留为历史快照并标注不再代表当前候选证据；不编造新入库数量，不替用户裁决九项冲突 |

## 旧断言如何处理

- diagnostics_1110_test.go：原 master-off 断言确为旧新安装默认值，改为新默认 on；ARAM bench 默认、已保存 bench 偏好断言全部保留。
- r78_test.go：schema2 无 champSelect 对象应使用新默认 on，改测试名与该断言；既有自动规则的 enabled/delay 保全、各 side 默认关闭/空池及 ARAM bench 限制保留。新增显式 champSelect.enabled=false 存档测试，防止错误全量迁移。
- r64_test.go 原 previous-banner 成功用例改用 clear-challenges 验证同一头衔恢复字符串、清空勋章且保持原旗帜；r66_test.go 仅移除被删除 action 的参数化项，保留 clear-challenges 的多候选恢复/拒绝/诊断完整矩阵。生产保全函数没有修改。
- r99_test.go：旧 W1/W2/W3 原值回写与 W4 临时配色测试由 W5/W6 的新值/回读/取消恢复测试替代；只读矩阵由 13 请求更新为 8（关闭的路径仍有历史归档）；不安全 loadout ID、错误码脱敏与隐私断言保留。seed fixture 增加 league 返回值，不以移除 seed 隐私测试换绿。
- web/suite.test.cjs：原 fixture “owned:true 但 ownership unavailable”改为已证实拥有态，继续验证所有关闭路径、focus、首帧上限、CSP、lazy cache、disabled 条目和 apply。
- desktop/overview-render.test.cjs 的 R56：旧列表要求存在“切换上赛季旗帜”与 R101 冲突；改为确认旧配色/按钮消失及左列只读旗帜入口存在，头衔保全文案、背景拥有开关、领奖/下拉契约保留。
- web/r95.test.cjs 的 P3：旧条件隐藏行为由 R101 明确覆盖；仍测试 ban/pick 三策略以及非默认 2.5 秒值，改为始终存在、仅等待策略可编辑，其他显示 0。
- privacy explicitWrites 仍 11 条、automaticWrites 仍 10 条；只更新失效旗帜描述和征召默认状态，其他九条默认关闭语义保持。

## 独立核验与实测限制

只读子代理分别核验后端恢复/缓存/默认和前端交互；后端没有发现行动项。前端提出 SSE disconnected 与 LCU disconnected 事件不同：本轮沿用 setConnected(false) 关闭两个弹窗及清理头像缓存的既有连接语义，单纯事件流断开不等同客户端断连，不额外中断用户浏览。拥有态未知的旗帜分组可过滤为空，但全部条目 disabled，页脚明确标记 unavailable，不伪报未拥有。

浏览器使用真实 Chromium + 生产 demo 和 CSS/JS；5099 头像/35 旗帜均为合成 fixture。图像 API 无 LCU 上游，截图可能有缺图，占位图不作为真机图像质量验收。控件实际几何、禁用状态、焦点与滚动检查可复现。移除 nowrap 的变异使用额外 110px 压力夹具，让换行高度成为可观察断言；正常三视口也分别检查 <=34px。只读探子一次 Chrome 启动超时不算验收；主线程真实浏览器已完整通过。

## 必须后续由真机补齐

1. 在 Windows 国服连接后，打开生涯页，点击“检测客户端写入支持”，导出 W5/W5-restore、必要时 W5-b/W5-c token shape，以及 W6/restore。根据实际结论再决定正式头像路径和旗帜写入；不要把 mock PASS 当作端点可用证据。
2. 若出现恢复失败提示，立即在客户端核对原头像/旗帜。网络断开或客户端拒绝无法由恢复请求保证成功。
3. 真机确认拥有态目录滚动性能、默认空序列整局零写入、Rookie 最新实际段位与主号展示。
4. OP.GG 小程序完成候选账号清单核对后，才可在后续工单增加账号。

## 自动化验收结果

结果与完整日志位于 `docs/r101-validation/`，命令均实际运行：

| 检查 | 最终结果 | 日志 |
| --- | --- | --- |
| `GOCACHE=/tmp/deep-legends-go-cache go build .` | exit 0 | go-build.txt |
| 同环境 `go vet .` | exit 0 | go-vet.txt |
| `go test -count=1 -v .` | PASS，108.585s；1206 个顶层用例通过，17 个原有可选联网/capture 用例跳过 | go-full.txt |
| `go test -race ./...` | PASS，157.308s，无 race 报告 | go-race.txt |
| 补充三项 fixture / 加强 ARAM 备战席条件后 `go test -race -count=1 -run TestR101 .` | 26 个 R101 顶层用例通过，3.496s | go-final-focused-race.txt |
| `node --test web/*.test.cjs desktop/*.test.cjs` | 713 项：712 通过、0 失败、1 原有 Windows 发布门禁跳过；302.948s | node-full.txt |
| 真实 Chromium，1500/1180/980 × 缩放 1/1.25 | PASS；未拥有 disabled、左右列几何、控件高度/溢出、滚动、焦点；35 条旗帜 fixture | browser.txt、browser/result.json、截图；布局变异的 baseline 再次完整通过 |
| 25 项真实变异 | **25/25 killed**，每项 baseline exit 0、mutant exit 1 | mutations/matrix.json、逐项日志；mutations.txt + mutations-final.txt |
| JS 语法、`git diff --check` | PASS | diff-check.txt / syntax.txt |

Node 唯一 skip 为既有 Windows/PowerShell 门禁，未增加跳过用例。首轮两个旧契约失败保存在 node-first-regression.txt，最终全量输出是 node-full.txt。首次浏览器测到 35px 后修正局部行高；最终六组几何全部 34px。变异 #19 初次误先碰到焦点时序断言，已调整检查次序并要求精确高度断言，重新从 #19 运行至 #25：最终移除 nowrap 得到 pressureHeight=50，#20 精确失败于卡片左列归属。该初次焦点失败不作为有效变异证据。

变异覆盖：当前头像误选、取消恢复、ID 泄漏、当前旗帜误选、data 丢失、仅 200 判成功、旗帜不恢复、owned 恒真、仅置灰、默认开关关闭、R8 错误库存、头衔丢失、旧 action 未删、保存 0、条件隐藏、disabled 守卫删除、覆盖老 false、空池写入、nowrap、错误列位置、commit 文案误删、取首条 flex、分段恒 1、无 solo 伪造段位、3 分钟 TTL。

代码与自动化验证已完成；本账本前述 W5/W6 与 Windows 客户端验收仍是显式待办，未据测试结果解除真机门禁。
