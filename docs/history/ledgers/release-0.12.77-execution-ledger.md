# 0.12.77 发布执行账本（R238，进行中）

日期：2026-10-07，Asia/Shanghai。已亲自通读 R238 全文、项目记忆两份文档、AGENTS 与发布账本模板。目标 0.12.77，key mode **public**，默认不带注册码。本次继续执行前为 `codex/release-0.12.75` / `b37357b2a6cf29397ac0f16255d37fa1566e87ac`；现在已合并 v0.12.76，新建 `codex/release-0.12.77`，版本暂为 **0.12.76**。业务已按工单拆提交，尚未构建候选、推送、打 tag 或发布。最终 SHA、指纹、Release id 和新附件 SHA-256 尚未产生。**未在 Windows 实跑**。

## P0 发布闸门

| 闸门 | 已核对证据 | 结论 |
|---|---|---|
| R248 完成 | [R248 账本](../../r232-execution-ledger.md#r248-注册码搁置默认关闭与审计包p0p4-完成)，P0–P4 完成 | PASS |
| 快照可还原 | 分支 `codex/license-shelved-r232-r247`，提交 `475c20dc11cad9a0d808be02498c78c74e80d76e`；实际 tree 4413 文件；再次 `git bundle verify` 成功，完整历史 | PASS |
| 默认包无授权材料/网络/UI/事件 | [包审计](../reports/r248/package-audit.json)、[Electron](../reports/r248/electron/startup.json)、零网络测试与两项断言变异；实际审计包 SHA 再核仍为 `61bcbe529eaf9a309da0601e42954275dd0e6ab9f71540aae655ec24e12d59ea` | PASS；最终发布构建须再审 |
| 更新方式恢复 0.12.76 | 真实匿名 0.12.76 资产已在 R248 本地校验；默认 schema/版本/固定地址/大小/SHA 检查，签名信任仅 license 标签 | PASS；最终发布构建须再核 |
| 启动对比各 3 次 | before 1044/673/641ms，after 623/761/608ms；均值 786→664、中位 673→623；[对比记录](../reports/r248/startup-comparison.json) | PASS；macOS 小样本，不代替 Windows |
| R247 P1/P2、R246 停止 | R247 等页面状态提交的测试修复完成，R246 未继续 | PASS |

只读 GitHub API 已确认正式 Latest 仍是 [v0.12.76](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.76)，Release id `404668937`，SHA `8ff273d7bc3e5d6d9ade683dc66c62f40971f854`，非草稿/预发布。附件只有 Setup、latest.json、SHA256SUMS-public.txt；Setup 为 111854592 字节，API digest `01014312b60e591a05bfc87f86e098adf6c5fc5e59520db5aedac5b5dba02ff3`，与 R248 使用的旧资产一致。

## P1 分支整理（范围已由用户确定）

已完成备份：`/private/tmp/r238-pre-merge-nqzky9aj/`。`files/` 保存当前所有非忽略文件 4559 个，逐个复制后 SHA-256 核对；保留行尾、mode、符号链接。`manifest.json` 记录文件、原 HEAD/分支/status，`working-tree.patch` 为 binary diff。`repository.bundle` 保存 33 个本地分支/tag ref，验证完整历史成功。恢复可先从 bundle clone、checkout 原 HEAD，再依据 manifest 覆盖 files/；没有读私钥、Secret、.dev.vars 或 manage 仓库，没有签名/推送。

用户本轮明确决定：R249–R251 不纳入 0.12.77，不再询问范围、不评审其逻辑。先创建本地快照 `codex/r249-r251-hidden-identity-snapshot`，提交 **6fb02a6a414d614f4de366eeda30f2f5d25a131e**，tree **4335 文件**、本次1120文件改动，保存所有当前可提交源码与文档（含R249–R251、P6/P7）；用户明确排除的原始日志/旧validation/dist不提交，原始文件仍由既有备份及本机保留。快照不推送，分离过程中该分支未修改。既有 `repository.bundle` 再次 verify 完整历史成功，没有重做整份备份。

从原发布分支合并 v0.12.76（merge **0863a4fe54b1f1af04e22141d4ffad5eaa72e98c**），再建 `codex/release-0.12.77`。发布分支不继承完整快照提交，避免把未纳入内容及大资产带进可推送历史；按路径恢复规定工单源码，再逐块移出隐藏识别。独占live_identity/live_namesets、R249/R250/R251测试/向量/脚本/文档只保留于快照。共享块按账本定位：gameplay身份恢复/早期身份推送/nameset batch移出，保留R244的早期历史推送；nameset字段/隐私子句/独立发现context增量移出；固定掩码函数输入处理与注释恢复v0.12.76原文，掩码未改；前端隐藏行与敌方占位恢复v0.12.76条件，保留R241标签与R244标题/实际条目规则。未发现仍无法安全拆分的块。旧R129/R193测试中的displayStats和实际recentGames夹具属于R244标题同源适配，保留，不属于新增隐藏识别。

`backend/desktop/scripts` 产品源码检索 R249/R250/R251、nameset接口/字段、两个新增身份诊断事件均为0；快照中原文件仍在。`r122_autofill_diagnostic_test.go`、`social_test.go` 相对 v0.12.76 **零差异**，直接沿用正式基线修复，不再提交重复清理/锁修复。

两份基线夹具已核对，均无差异。未使用 `r235-only.diff` 回滚。原始大日志、四个旧 validation 目录、r235/attempts、dist 审计/STAGING 均不得进入发布提交。

## P6 一次性自动缩放迁移

采用工单默认第 1 种：旧固定档升级后重置一次。桌面文件无 `defaultAuto:1` 时原子写 `{mode:"auto",defaultAuto:1}`；随后用户选择固定档写 `{mode:"fixed",value,defaultAuto:1}`，重启保留。损坏、缺文件、超过 1024 字节也写标记自动档。浏览器直连按 `lol-loot-ui-scale-default-auto` 独立迁移，桌面回报仍覆盖 localStorage；未改 UI 文案、基准常量、bounds 策略。

Node 27 项 PASS/0 skip，覆盖迁移、用户新选择、无重复写、损坏/过大、浏览器独立标记及外壳 auto 覆盖固定 localStorage。临时副本的「每次重置」「不迁移」两项变异同时修改桌面与浏览器条件，分别 exit 1，捕获原断言，非语法/依赖失败。[变异结果](../reports/r238/mutations/results.json)。

真实 Electron **43.3.0/macOS** 使用生产 main/preload/前端，隔离 userData 和 localhost 合成后端，不启动游戏、不请求授权。第一次进程从无标记 fixed 1.25 得到 auto；选择 1.25 后落盘标记；第二次真实进程重启仍为 1.25。窗口仍为 `(40,40,1050,750)`，缩放文件迁移没有改窗口位置或大小。授权 API/域名请求数 0。[三轮结果与截图](../reports/r238/electron/results.json)。

模拟方式：物理屏幕保持 **1680×1050 DIP、scaleFactor=2**，不修改 macOS 分辨率。用生产 `windowBoundsForWorkArea({width:1920,height:1080})` 得到 **1766×1010**，对真实 Electron 原生窗口 `setBounds`，读取实际 content bounds 与前端 CSS zoom。实际 content 1766×1010、auto **1.00（100%）**；因 `min(1766/1920,1010/900)<1` 被下限钳为 1。工单提及“高 990 就是110%”还受宽度约束，不能仅从高度推出倍率。本次是原生窗口几何模拟，**未在物理 1920×1080 屏上实跑；未在 Windows 实跑**。基准900与文案1080不一致仍只报告，不改两者。

第一轮自编 Electron 夹具 token 只有25字符，被现有至少32字符校验拒绝，30秒超时；修夹具为重复3次后通过，未改业务校验或等待预算。原第一轮未生成结果 JSON；最终驱动已在超时清理前保存日志，避免再次丢证据。

## P7 逐项处理

1. **修：坏 SGP 局。** 整页先 `json.Unmarshal` 为 RawMessage，内层缺 `json`、fallback 缺 `participants` 等也可得到 `field=""/invalid-json`；该标签本身不能证明整局字节被截断。5–6MB 小于24MiB限制，`readLimited` 超限直接报错，不静默剪裁；读失败本来有有界重试。未取得工单所述06:28原始日志，不能认定那22条的真实根因。新增仅长度/首尾类别的坏局诊断白名单，不记内容/ID/账号；成功HTTP但JSON“unexpected end”只补一次重试，仍在既有最多3次请求与 context 预算内。坏局仍跳过、服务器 consumed 仍推进；赛季并行头扫和串行回补现在保留同页好局统计，不跨过坏页、不标完整，之后可重取。`season_stats_head_refresh` 增加真实 `decode_failed`，recent 分支为0。测试覆盖缺 JSON/非法JSON形状、其余局保留、准确失败数、截断只重试一次、完整性/游标以及实际异步头刷新事件。
2. **修：pro_identity_match 刷屏。** 同玩家/对局的内存摘要先去重，再进入已有日志去重器；有 gameId 的战绩、live、post-game 传真实 ID，无 ID 的 overview 按进程去重。8192个有界摘要，不落盘/不输出身份；日志轮转不重置这层。测试让10个玩家的实际总览战绩装饰路径执行10轮，身份结果正确，摘要10个、日志≤10条且不生成刷屏的 diagnostic_dedup；下一局可记、轮转不重记。
3. **记录：ARAMKit。** 工单记载3.2/10.7/25.5秒、ttfb1.5–3.7秒/143KB，均非本会话新采集。最近连续三个北京20–22点晚高峰样本缺失，不能计算该窗口分布/P90，不能宣称已改善真实延迟。保留待真机采集项；未虚构三晚数据或据此新开未经证据支持的性能方案。
4. **记录：facade。** 后台facade与首卡路径各有独立事件，但共享LCU连接是否形成竞争仍需同run时间线；没有证据证明9.1秒阻塞首卡，也没有证据证明完全不影响。未擅自改调度，保留真机核对。
5. **记录：绝活哥符文超时。** match_ids失败直接停止该玩家并返回upstream-timeout/空runes，绝活哥来源没有自动换源。前端保留OPGG/客户端备用/职业来源；失败只在绝活哥区域显示超时与重试，不清空整页。但其他源可用取决于其真实响应，不能称超时一定能拿到完整符文。既有timeout/negative-cache Go护栏已定向实跑；未改推荐业务。
6. **记录：隐藏玩家。** R249–R251已保存于完整本地快照，发布分支已移出；未纳入0.12.77，本单不评审其逻辑。
7. **记录：R246。** 保持停止，半成品留在 R248 默认关闭的 license 标签内；没有改签名/字段/错误码/向量/过期必须联网成功规则。
8. **记录：环境失败。** 本机有Go，当前定向race/vet通过；setup-only-build/release-quality-gates/verify-ci-test-filters及最终全量仍待P1合并后统一重跑，不能以R248旧结果替代。

## 当前定向检查（不是 P2 最终预检）

原始日志、命令、开始/结束与最后五行写入 [r238 reports](../reports/r238/)。未增加任何skip或放宽断言/预算。

| 检查 | 结论 | 证据 |
|---|---|---|
| 缩放 Node 两文件，27项 | PASS、0 skip，2.332s | [记录](../reports/r238/targeted-node.json) |
| R238/R223/R231/R241头/R235头/R98Chovy/符文超时，race | PASS，20.079s（包5.481s） | [记录](../reports/r238/targeted-race.json) |
| go vet ./... | PASS，1.440s | [记录](../reports/r238/go-vet.json) |
| 真实Electron迁移/重启/尺寸模拟 | PASS，6.151s | [记录](../reports/r238/electron.json) |
| 两项缩放变异 | 两者断言FAIL，驱动PASS，8.497s | [记录](../reports/r238/scale-mutations.json) |
| 受影响JS syntax / git diff --check | PASS | [diff记录](../reports/r238/diff-check.json) |

将日志上限测试从直接调用提升到实际装饰路径时，首轮合成ID `private-0` 太短，被 `normalizeGameplayReference` 的已有合法性校验清除，原身份断言失败。只将新测试夹具改为合法48字符合成ID，未改断言、业务校验或预算；最终上表race通过。首轮失败保留于 [non-final](../reports/r238/non-final/pro-identity-invalid-fixture.json) 与同名log，未用成功结果覆盖该失败。

## 后续发布门槛

P1整理后将在最终合并源码完整执行P2（默认集无授权、全部原变异/截图、公共包审计、macOS backend、同SHA Windows质量与实际升级）。用户取消便携包要求：**沿用0.12.76正式资产结构，只产Setup、latest.json、SHA256SUMS-public.txt**，不扩展Setup-only流程，旧便携兼容逻辑不删除。Windows实际升级补0.12.76，并验证在线检测/下载/校验/安装及数据保留。用户本轮允许本地预检全过后改0.12.77；P3游戏客户端真机项只给清单、保持待真机，到发布Latest前等待用户明确确认。

尚未发布，不修改R235/R241/R244/R247/R248为“已发布”；注册码工作单继续搁置。**未在 Windows 实跑**。无新安装包、无新安装包SHA-256。

## P1 完成记录

业务提交：R235 `f92632a5`、R241 `2a7800f3`、R244 `86944a8c`、R247 `5982b4e9`、R248 `d96c97ab`（默认关闭，需要license标签）、P6 `963cf814`、P7 `aa712cf9`；文档提交依次 `ddd2e662`、`afe17ebd`、`04db662e`、`09ce01ff`、`31627b35`、R238 `38fcac8c`。P1结束时git status无输出、diff --check通过；[基线差异统计](../reports/r238/p1-diff-stat.txt)。分支未推送，快照不在发布祖先历史中。

P2准备：验证脚本只增加独立输出路径，不改断言/预算；run-check每次生成不可覆盖的时间戳日志。默认更新兼容测试依赖076的公开111MB安装包，未将大资产提交；CI测试前下载固定公开版本并核验大小/SHA，保留原测试的严格文件校验。

P2升级验证脚本已补精确公开076 Setup（大小与SHA固定）、076→077候选清单、真实loopback HTTP检测/下载/SHA/Apply交接、实际Windows安装和持久目录哨兵校验。该HTTP夹具只在测试transport映射合法GitHub URL，不改产品信任策略；不能标成匿名Latest实测，匿名077 Latest直到用户确认发布后才能验证。缓存/收藏/偏好哨兵保留不代替真实账号真机验收。

## P2 首轮失败（原始证据保留）

默认Go全量首轮175.866秒失败：`TestSGPRequestObservationTracksRetryAndSafeParseShape`严格拒绝P7新增的HTTP事件字段truncated_json。修复删除该冗余未审查字段，保留已有parse_failed/payload_prefix_shape/payload_sample_bytes和截断最多一次重试；没有扩展白名单或放宽断言。[原始日志](../reports/r238/p2/go-full-20261007T203533.661534.log)。最终Go/race须重跑。

license首轮3项失败：R237指向默认license.html，改读取R248已移动的license-activation.html，隐私文字与断言不变；R233失败计数租约锚夹具硬编码10+5分钟、R242有效到期夹具900秒，二者均与R246已改2小时合同冲突，按用户要求记录半成品，不修/不放宽安全规则。[首轮日志](../reports/r238/p2/license-retained-20261007T203623.764155.log)。

Renderer最终检查1332项/1328通过/4原条件skip，147.024秒，预算不变；默认集没有license测试。installer test/vet、root vet、Worker16、Go格式585文件与JS语法313文件通过；完整证据在p2目录。Windows真机清单已形成，所有实际游戏项保持待真机。
