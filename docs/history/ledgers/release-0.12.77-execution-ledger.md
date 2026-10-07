# 0.12.77 发布执行账本（R238，进行中）

日期：2026-10-07，Asia/Shanghai。已亲自通读R238全文、自己的账本、项目记忆两份文档、AGENTS与发布模板。发布候选版本 **0.12.77**，key mode **public**，默认无注册码；分支 `codex/release-0.12.77`，最新候选SHA **40363c2e2676d76ae0dccb0a6d6b2bce895f661b**，生产指纹 **129e36bf324c**。P0/P6/P7既有结果保留，P1完成，最终本地P2/变异/截图/两份public候选审计完成，P3真机清单已给。前两轮CI失败原始记录保留；第三轮完整质量工作流37631380966进行中。尚未tag、创建草稿或发布Latest。Windows CI已实跑的项目与仍待游戏真机的项目分开记录；所有待真机项**未在 Windows 实跑**。下文旧SHA/旧指纹/旧“未推送”是当时阶段的历史状态，不代表当前状态。

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

Chromium R235首轮失败：[原日志](../reports/r238/p2/chromium-r235-20261007T204053.493976.log)。旧对比度采样只把rgb数字除255，R241的color-mix计算样式为color(srgb ...)时误把0–1值当0–255；改用R241现有canvas像素采样，4.5断言阈值与所有预算未改。初轮截图保留于chromium-r235-01，下一轮用-02。

R235截图第二轮颜色断言通过后，在旧+N定位超时：[原日志](../reports/r238/p2/chromium-r235-measured-20261007T204205.090662.log)。R241已将+N改为独立match-tags-popover，R235驱动仍等global-tooltip；改等待实际展开的popover含同一单杀文本，20秒预算不变、不改产品。第二轮截图仍保留，第三轮用-03。

R235截图第三轮仍超时（-03保留）：正常宽度下单杀是高优先级可见标签，R241的+N只列隐藏项；驱动使用R241已验证的窄容器夹具让标签全部进入+N，仍要求单杀文本与原等待预算。R235变异首轮5项断言杀死，第6项因R241新增data-match-tag属性导致旧精确定位失效（未注入、非有效变异）；只更新定位前缀，完整6项在新-02目录再跑，首轮结果不替换。

第四轮+N超时定位为驱动等待旧状态：隐藏数>5在缩窄前已经成立，ResizeObserver重算后关闭旧popup，而鼠标没有第二次pointerover。等待条件改为该单杀标签已隐藏（布局状态提交），原20秒不增、不加重试。第五轮加DOM/截图失败诊断，不以偶然通过代替这一等待修复。

macOS未安装pwsh，附加PowerShell解析检查没有执行（启动报ENOENT，非脚本通过）；不安装工具、不写成Windows结果。4个renderer条件跳过是R86/R222 Windows两个与R82 PowerShell两个，Windows CI明确选择精确计数实跑；run-check已补启动失败持久化，现有失败原始记录保留。

第六轮R235已通过+N和两主题主要截图，最后Page.navigate 30秒超时：前面各场景的SSE页面从未关闭，同源HTTP连接累计耗尽。驱动现在每个独立场景完成截图/断言后关闭target，原CDP预算不改，不重试。-06证据保留；R241/R244两主题与R100/R117在同一最终源码已通过。Windows升级脚本另补已装076与候选077的真实ProductVersion核对，清晰分离旧阶段文件/诊断offset，防止拿旧成功事件当本轮通过，原30秒预算不变。

## P2 本地预检完成 / P3 清单完成

2026-10-07 20:55（Asia/Shanghai）：最终Go全量180.904秒、全量race216.844秒、最终vet1.491秒全部PASS；原首轮Go/race失败均保留。默认Go集合238个test文件，license_*为0（inventory记录具体数以JSON为准）。Installer test3.300秒/vet0.227秒PASS。Renderer1332/1328 PASS/4原条件skip、147.024秒；desktop独立296/294/2条件skip、76.714秒；Worker16/16、0skip。setup-only/release-quality-gates/verify-ci-test-filters单独再次PASS，未改断言/预算。默认Electron首帧2.108秒：主界面可见、无激活表单/授权小窗、设置无到期、授权API/域名0、license事件0；startup-loading的inert属于R235等待本人标签页，不是授权层。

全部21变异：R235 6、R241 4、R244 7、R248 2、P6 2均断言FAIL，驱动PASS，无语法/依赖失败。R235/R241变异使用与最终生产文件SHA逐一一致的临时输入副本（它们原驱动会修改工作区），其余overlay；真实发布源码没有被变异。R235深浅Chromium第七轮46.058秒PASS，anchor/箭头误差0、对比度最低5.39>4.5、切回全部15/16.7ms且原节点保持；R241 11.718秒24结果/0异常，R244 1.964秒真实Chromium+合成10行/0异常；R100/R117 PASS。旧失败、原截图保留，不替换。

license标签再次编译并运行，仅余TestR233FailedCounterSaveDoesNotRefreshCachedLifetime、TestR242LicenseExpiryStrictSignedField两个已定位的R246半成品合同冲突（license.go:28是2小时，夹具分别硬编码10+5分钟/900秒），按授权记录，不修/不放宽安全规则。R237移动后的精确隐私断言已经PASS。macOS缺pwsh启动记录为UNAVAILABLE，不标Windows执行；真正PowerShell/升级将由CI实跑。

[Windows真机清单](../reports/r238/windows-device-checklist.md)已完成，含通过标准与日志路径，所有真实游戏/账号/晚高峰项目均待真机；匿名077 Latest在线路径待用户确认发布后真机。本地Go默认更新兼容已用已发布076清单与111854592字节公开资产核验，通过076 unsigned/固定URL/大小/SHA策略。接下来才更新077元数据，产公共候选和同SHA CI；包、WindowsCI及草稿附件尚待完成。

安装包ASAR核验纠正日志位置：package.name是deep-legends-desktop，顶层productName未设置；main未setName，因此实际ElectronuserData默认为APPDATA/deep-legends-desktop。升级脚本直接从已安装076的ASAR解析productName||name并要求该目录已存在，避免在错误目录种哨兵而虚报保留；真机清单已更正。此项是验证脚本修正，未改产品存储逻辑。

## P2 本地public包审计 / P4进入Windows CI

正式build-desktop.sh在逐文件复制的临时构建输入运行（无license标签、public mode、无私钥/真实码），源码仍是主发布分支；190.216秒完成唯一新本地Setup。输出全新dist/R238-0.12.77-public，macOS后端为dist/R238-0.12.77-macos-public。原有160个STAGING/R248审计文件大小与SHA全相同。没有创建便携包。

| 最终本地检查 | 结果 / 证据 |
|---|---|
| Go全量 / 全量race / vet | PASS；180.904s / 216.844s / 1.491s；p2/go-full-final、race-final、final-vet |
| Installer test/vet | PASS；3.300s / 0.227s |
| Renderer all / desktop node --test / Worker | PASS；1332/1328/4 skip、296/294/2 skip、16/16/0 skip；未新增skip |
| build/quality/filter护栏 / gofmt / JS / diff | PASS；PowerShell仅本机UNAVAILABLE、待实际Windows CI |
| license编译专项 | 编译通过；仅2个R246半成品时长夹具失败已记录，R237精确隐私测试通过 |
| 21变异 / R235-R241-R244深浅Chromium / R100-R117 | PASS；全部原断言与预算保留 |
| 默认首帧 / API与域名0请求 / license事件0 | 真实Electron/macOS PASS；Windows源码首帧待CI，真实安装后界面待用户 |
| 新public包授权域名/kid/公钥/隐私句缺失、Riot Key缺失 | PASS；[完整包审计](../reports/r238/p2/package-audit.json) |
| 后端digest/fingerprint、ASAR与fuses | PASS；固定digest/ASAR header实际匹配；RunAsNode/NodeOptions/Inspect关闭、ASARIntegrity/OnlyASAR开启 |
| 默认在线更新策略 | 与076一致，unsigned manifest/schema/固定URL/size/SHA；已用真实076公开清单和资产验证 |
| macOS backend / 隔离数据自检 | PASS；0.12.77/7e1229c2a79c，奖池554，哈希dee5e21f5234；未在Windows实跑 |

本地Setup SHA-256 **1a9a7ad155460849c075338d3857f703a2b4137191a52bc6aefad8f4bf2b6ce4**；Windows后端SHA-256 **a67dc62bd94bcecf2a7a832084f6db7778ef887dd8e6f08e559d74faa8095702**；macOS后端SHA-256 **3bf5b1fed5ff28eb8625449f51222ebfb2dc081d72a293585ad1fa5984c3cd9e**。这份本地包用于审计，正式附件以Windows工作流实际生成的草稿为准，不能混用两个摘要。

玩家说明已按工单写入077 CHANGELOG，无R编号、注册码或隐藏识别。按用户本轮指定顺序推发布分支、同SHA完整质量通过后再tag；快照分支不推。与日常R222只推tag的去重顺序不同，本轮遵循用户明确的分支质量闸门，记录可能的同SHA tag复检。到Latest明确停下等待确认；真实账号/晚高峰/匿名077在线仍待真机或发布后，不标通过。

发布分支于2026-10-07 21:04:56（Asia/Shanghai）推送成功，冻结源码SHA **078cc6de7f072927fa9f21ef1b55992f517961a9**，指纹7e1229c2a79c，key mode public。完整质量工作流[37625739383](https://github.com/LLYY0418/Deep-Legends/actions/runs/37625739383)在2026-10-07T13:05:01Z启动Linux quality/Windows build两作业，当前尚未结束，不标PASS。两个本地快照均未推送；tag/草稿/Latest仍未创建。

## P4 首轮同SHA CI失败（未tag，原日志不覆盖）

[37625739383](https://github.com/LLYY0418/Deep-Legends/actions/runs/37625739383) conclusion=failure。Windows完整backend/installer与全部专属/PowerShell护栏已经实跑PASS，但正式构建在gofmt启动失败：585个绝对路径估算命令35352字符>Win32 32767上限，PowerShell报StandardOutputEncoding只在redirect时支持。修为每100文件一批，保留全部585文件、每批检查退出码与全部未格式化输出；现有Windows R86夹具补600个合法长文件名，原失败Go测试和early-build变异断言/60秒预算不改。原Windows完整日志保存为p4/windows-job-112806901067-raw01.log与ANSI净化副本。

Linux race等已PASS；renderer1332项/1330PASS/2 Windows条件skip、断言0失败，单文件最大75588ms<90秒，但总245554ms>240秒，故正确FAIL。并发4在CI互相争用；现在统一采用既有Windows的上限3（本地R235验收也使用3），不改文件集合/断言/90或240秒预算，重跑后才判PASS。原Linux完整日志p4/linux-job-112806901255-raw01.log、净化副本与watch失败结果全部保留，不引用重跑覆盖此失败。

Windows包/首帧/实际升级尚未执行，不能算Windows整体通过。0.12.77没有tag/Release，Latest仍076。构建脚本输入变化导致新指纹，前一个本地候选与macOS后端保留审计目录；接下来使用全新-02目录重新构建和审计，不覆盖任何既有包。

CI范围内修复后的本地结果：受影响build/quality/filter护栏11.108s PASS，默认renderer1332项/1328PASS/4原skip、141.749s PASS，90/240秒未改；格式/JS/filter/diff再次PASS。正式Bash输入的完整Go分片/installer/vet与新public包178.746秒通过，源码Go业务和夹具没有变化，前述Go全量/race仍对应这些文件；下一轮CI按新SHA再跑全部。新指纹 **129e36bf324c**，新本地Setup SHA **34e811cc30e2b3575186e8485b8da3c0ef1528d21d57d6ea10e335ea70174482**，Windowsbackend SHA **8cbbaea70908d1aa3603ca35dc3ae9231636db13ce3c4ff37d30f0759d09d965**，macOSbackend SHA **0a44dbe86d353a04a973ddac00dba3d1cbfaf09e88b80c1b00246f69b5037ad5**；新-02包再次授权材料/Key/digest/fuses/ASAR严格审计PASS。旧包、旧160个STAGING/R248文件与全部失败不覆盖。真正Windows升级与匿名077 Latest仍未通过，不标完成。

## P4 第二轮同SHA CI失败（原记录保留）

候选a83abff77f5cf10fe8ddf5580c2bd7e2ea9849c9于21:30:39推送；[37629060990](https://github.com/LLYY0418/Deep-Legends/actions/runs/37629060990)两作业最终failure，未tag。Linux原renderer总预算失败已解除，全部renderer通过；真实Chromium r100在20秒启动预算内未取得DevTools地址而失败，r117未执行。既有驱动超时不保存stderr，无法从这轮证据认定sandbox或具体冷启动根因；没有添加no-sandbox、不增加20秒、不重试。两个驱动补启动PID/退出状态/耗时与stderr，CI无论通过失败均上传原始诊断；原第二轮完整日志p4/linux-job-112818255359-raw02.log和净化副本保留。

Windows正式构建、专属条件四项、真实Electron源代码首帧已实跑PASS，完整Windows仍因升级脚本失败。首帧实际Windows CI显示正常主框、无遮罩/激活表单/授权到期/小窗，实际显示bounds(40,40,1024,720)、总启动1176ms；CI虚拟显示器的工作区钳位不能称1050×750真机尺寸一致，仍待用户清单。完整升级到安装076后版本断言失败：从固定SHA的公开076 Setup只读提取PE/ASAR核验，ProductVersion实际0.12.76.0、FileVersion和ASAR为0.12.76（p4/public076-version-resource.json）；原脚本把四段字段与三段版本比较。改为三个字段同时精确等于Version.0/Version/Version并保存安装路径/SHA/时间，严格验证增强、无业务代码变化。原Windows日志p4/windows-job-112818255780-raw02.log及失败升级artifact保留；不把已过的首帧当升级通过。

本次只改验证诊断/版本字段核对，指纹仍129e36bf324c，第二份本地包对应的生产输入未变化，原审计结果仍有效。受影响R100/R117结构/构建/质量/filter检查9.458s PASS，2个原Windows条件skip；本机真实R100 1.333s PASS，旧Linux启动失败不被此结果覆盖。下一轮同SHA完整CI结束前Windows整体保持未通过，Latest仍076。

## P4 第三轮候选与等待状态

2026-10-07 21:47:53推送候选 **40363c2e2676d76ae0dccb0a6d6b2bce895f661b**。完整工作流[37631380966](https://github.com/LLYY0418/Deep-Legends/actions/runs/37631380966)按同SHA执行Linux quality/Windows build；未结束前不标成功、不tag。本机R117真实Chromium3.398s PASS，原断言/20秒启动/45秒CDP预算保留。两次失败原记录不替换，全部生产构建输入仍与public-02/macos-public-02相同，指纹129e36bf324c。

原始CI日志本身含行末空格与空行，保留本机原字节，不为格式门禁改写；源码/文档diff --check保持严格，归档日志检查单独排除原字节log。Git对Windows CRLF的规范化不改变本机保存原始日志。没有提交禁止的大日志/原始jsonl或dist包。

## P4 第三轮CI：Windows完整成功 / Linux仍超总预算

[37631380966](https://github.com/LLYY0418/Deep-Legends/actions/runs/37631380966)最终failure。Windows job112826235595于13:57:21Z完整success：backend/installer、原四个Windows/PowerShell条件项全部实际PASS，public构建/校验和、源代码真实Electron首帧、065/068/076真实安装与候选077升级全部PASS。PE076.0/077.0、FileVersion与ASAR076/077均精确匹配；在线1次检测、1次下载111880704字节、SHA/Apply交接成功，8阶段完整、total8572ms，缓存/收藏/设置/窗口/缩放5个哨兵SHA相同、快捷方式创建时间/IconLocation不变。在线候选使用真实loopback HTTP与精确GitHub URL映射；匿名077 Latest待发布后真机。完整原日志windows-job-112826235595-raw03.log，摘要证据windows-ci03-*.json。不是玩家真实缓存与收藏的证据，不修改P3待真机状态。

Linux renderer 1332/1330PASS/2原条件skip、断言0失败，最大文件60.14秒<90，但全量258446.702ms>240秒，正确FAIL；Worker/Chromium之后步骤未执行。原日志linux-job-112826235265-raw03.log、净化副本、renderer-timings-ci03.json与watch失败保持。不能以Windowssuccess代替整轮success，未tag。

只改调度而不改业务/断言/集合/预算：19个反复创建完整jsdom/200局DOM的文件与145个其他文件分队列同时启动。全局墙钟从两个run启动前到两个run结束，统一240秒；每个文件90秒；计时数量和唯一文件覆盖检查，count逐组相加，任何组失败或缺summary都整体fail，无重试。两个临时合成探针分别让大DOM组/其他组断言FAIL，均整体exit1且两文件完整运行（renderer-group-failure-probes.json）；生产测试不加skip。首个2+1配置本机179.937秒PASS、164文件/1332项/4原skip；最终配置最多3个大DOM+1个其他worker利用等待空档，防止4个大DOM同时争用，完整实跑后再新SHACI。指纹不受测试调度影响，仍129e36bf324c。保留初次配置耗时，不替换为最终配置结果。

最终分组配置本机127.447秒PASS；真实全量164文件、1332项/1328PASS/4原skip，19/145分组完整，统一墙钟127409ms<240000，两个组失败探针仍断言exit1。发布/质量/filter定向13.448s PASS；原2个Windows条件skip没有增加。源码/文档diff --check、JS语法和指纹再次通过。接下来第四轮整轮CI；之前第三轮Windows实际成功证据只归属于40363c2e，不标为新SHA结果。
