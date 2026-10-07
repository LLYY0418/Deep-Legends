# R232 执行账本

2026-10-07 最终决定：注册码相关工作已随 R248 搁置，默认构建完全关闭；以下 R232–R246 为历史记录，最终状态见文末 R248。

日期：2026-10-06。基线 HEAD `10933787940f`，版本保持 **0.12.75**；未更新版本号、未发布、未创建 tag。状态：客户端实现与本地验证已完成；R232 整体未关闭，真实服务端与 Windows 验收未完成。

## 范围与执行顺序

- 动手前亲自通读 R232 全文及项目基础文档。按本次用户指令先起草、展示并打开 [license-protocol.md](license-protocol.md)，然后才写客户端；本次指令覆盖工单 P1 中“服务端先定协议、就绪后才做客户端”的旧顺序。
- 仅实施 P2–P6 客户端与 P7 客户端隐私修订。没有授权 Worker、DO、D1、码表或管理页，没有改 `relay/`；P4b 留待第二阶段。既有 Worker 测试只作回归。
- P0 保持用户确认的 100 码（99 普通 + 1 管理员）、最后明确激活者立即生效、15 分钟租约、60 秒心跳、老用户先发码再发版。请求超时 8 秒，失败重试 15/30/60 秒；未实施附录 A 严格交接。
- 未获取、生成或保存真实注册码、服务端签名私钥、管理页内容。假授权服务、测试码与随机测试密钥由测试注入；发布构建没有免授权开关、测试地址覆盖或测试信任根。
- 开始时已有的 `backend/queue_groups.go` / `queue_groups_test.go` 及 R222 历史报告等共享工作区改动予以保留，不归入本轮授权实现。

## 实际改动

| 项 | 客户端结果 | 未完成边界 |
|---|---|---|
| 协议 | v1 的请求/响应 envelope、字段、Ed25519 签名域、编码、错误码、重试及独立更新签名合同 | `deep-legends-manage` 尚未对齐和部署 |
| P2 | `backend/license.go` 默认拒绝；签名、请求号、设备、码型、revision 和 900 秒上限核验；持久化计数器后发请求；串行 activate/renew；有效本机缓存可离线启动；立即续租；运行中单调截止 + 墙钟检查；REPLACED/REVOKED 持久化终止，不自动抢回；手动提交可接管 | 真实服务端并发占用、轮换和时钟行为未验收 |
| P3 | `license_device*.go` 通过 x/sys/windows 调用 CurrentUser DPAPI，无 CGO；设备密钥、计数器、原始签名租约存 `%APPDATA%\Deep Legends\license`；凭据不可用时创建新身份、要求重新激活；安装与便携路径共用该目录；成功卸载且选择删除数据时清理 | DPAPI 跨机、跨 Windows 账户、升级/便携交替实测未完成；TPM 未实施 |
| P4 | HTTP 显式白名单、默认业务拒绝；业务 context + 授权代际；写出前复核；授权成功后才启动 LCU、职业缓存与业务生命周期；失效取消连接与待执行自动化；LCU/RiotClient 请求、设置写入、客户端启动执行前复核；旧 context 不被新激活复活；分享图原生 IPC 向 Go 核验，并在异步截图后复核代际 | 真实游戏自动接受、选禁、领奖、符文/装备与流式中途失效未验收；已交给操作系统/LCU 的副作用不能撤回 |
| P5 | 首帧独立遮罩与隐藏/inert 主体；粘贴、Enter、重复提交防护、Tab 焦点循环、Esc 不解锁；激活后清输入；失效清页面状态并重建空壳；原连接遮罩不能解除授权锁；英雄目录预加载推迟到授权后；隐私/诊断可在锁定时访问 | Windows DPI、原生窗口键盘与真实客户端共存体验未验收 |
| P6 | fuses：禁 RunAsNode/NODE_OPTIONS/inspect，启 EmbeddedAsarIntegrity/OnlyAsar；构建期后端 SHA-256 常量进入 ASAR，spawn 前流式核验；beforePack 覆盖各构建入口；独立 Ed25519 更新清单验签，缓存/下载/应用都复验，保留包大小/SHA256 校验 | Windows 实际拒启、`resources/app` 注入、自研安装器 payload 全流程和合法升级未验收；Authenticode 未实施；发布端清单签名尚未对齐 |
| P7 | 安装器许可文本与 `/api/privacy` 修订，移除“不限台数/所有数据仅本机”；说明授权交换、设备字段、DPAPI、本地保存与卸载删除；诊断不包含码、私钥、签名租约或令牌 | 服务端保存期限与删除渠道尚未定稿，当前文本明确在正式启用前提供，未编造期限；生产上线流程未执行 |

生产 `licenseTrustKeys()` 与 `updateTrustKeys()` 目前均为空，**拒绝新的授权/更新放行**。这是等待独立服务公开配置的状态，不能用于上线。后续只需对齐公开的 kid/公钥及签名合同，不需要把码表或私钥交给本仓库。

### 未授权路由白名单

其他路由默认受保护，不能凭 `/api/` 前缀整体放行；下列 API 仍经过既有本地 session 校验：

| 路由 | 依据 |
|---|---|
| GET 静态壳资源、首页 bootstrap | 展示激活页，无真实业务数据 |
| GET `/api/license/status`、POST `/api/license/activate` | 激活与仅含状态/文案/代际的状态查询 |
| GET `/api/privacy`、GET `/api/diagnostics/log` | 隐私文本与脱敏导出 |
| POST `/api/diagnostics/client`、`/startup`、`/startup-stage` | 脱敏启动与错误诊断 |
| POST `/api/quit` | 正常退出 |
| GET `/api/update/status`、`/settings`；POST `/api/update/check`、`/download`、`/cancel`、`/apply`、`/settings` | 独立可信更新；没有 `/api/update/unknown` 的通配例外 |

`/api/status`、`/api/events`、LCU/业务查询、`/api/system-proxy`、自动化与游戏写接口均不在白名单。系统代理由桌面主进程在 Go 已确认 ACTIVE 后下发；renderer 的通知本身不能授权。

## 测试与构建

最终结果的时间均为 Asia/Shanghai。运行记录和日志见 [verification.json](history/reports/r232/verification.json) 与同目录 `*.log.gz`；不是沿用改动前的结果。

| 检查 | 最终运行时间 | 实际结果 |
|---|---|---|
| `go test -count=1 ./backend` | 17:21:53–17:23:45，墙钟 112.283 秒 | 通过；最后一次 Go 文件（含测试）改动之后运行 |
| `go test -count=1 -race ./backend` | 17:17:18–17:20:14，175.635 秒 | 通过 |
| `go vet ./...` | 17:19:51–17:19:54，2.304 秒 | 通过 |
| `node scripts/test-renderers.cjs all` | 17:13:31–17:15:09，97.580 秒 | 1305 通过、4 既有跳过、0 失败；每文件与全套预算通过 |
| JS syntax | 17:21:58–17:22:02，4.615 秒 | 157 文件通过 |
| installer `go test ./...` / `go vet ./...` | 17:20:52 / 17:20:57 | 通过；最终 test 命中缓存，此前改后首次运行也通过 |
| Worker 既有测试 | 17:20:49，0.229 秒 | 通过，未修改 Worker |
| Windows `go test -c`，CGO=0 | 17:19:50–17:20:02，11.747 秒 | 交叉编译通过；未执行 Windows 测试程序 |
| R232 真实 Chromium | 17:21:56–17:22:14，18.016 秒 | [chromium.json](history/reports/r232/chromium.json)：等待 >15 秒保持锁定、业务请求零；深浅主题/1x/1.5x、Tab/Esc、隐私、移除遮罩后的 fetch 阻断、激活清输入、失效清页面通过；使用合成本地状态接口 |
| 既有 R100/R117 Chromium | 17:14:25 / 17:14:21 | 通过；R117 无无样式帧，授权状态由测试注入 |
| CI test filters / diff 检查 | 17:21:54 / 收尾 | 通过；R232 Chromium 护栏已接入 CI |
| public Windows dir 构建 | 17:14:18–17:14:44，26.104 秒 | Electron 43.3.0，版本 0.12.75，key mode public；未发布 |

R218 所需的 Go 全量输出最后 5 行（实际仅 1 行）：

```text
ok  	lol-loot-assistant/backend	111.159s
```

最终 race 输出实际仅 1 行：

```text
ok  	lol-loot-assistant/backend	161.591s
```

### 修复与失败记录

- 首轮 Go/JS 检查暴露旧启动/更新夹具和源码锚点不再适配授权生命周期、独立更新签名及生成摘要；夹具显式注入授权状态/测试签名，保留原业务断言，没有改发布路径来迁就测试。
- 英雄目录预加载曾早于授权，导致别名搜索缺失；已改为 ACTIVE 后预加载，R58 与全套前端测试通过。
- 加门禁后曾把已取消请求的错误改成“尚未激活”；修复为先返回原 context 错误，保持既有读取取消语义。
- race 首轮发现 `social_test.go` 请求列表/计数的既有测试竞态；仅测试夹具补锁和快照，未改社交功能。最终全量/race 都在该改动之后执行。
- 17:17 的普通 Go 全量遇到 `TestR122SGPSummaryHistoryEventCarriesAutofillCounts` TempDir 清理 `directory not empty` 的一次失败；未扩大修改该功能。后续单独全量重跑通过；不能说该偶发问题已根治。
- 一次 Go 全量启动的自动审批超时，未启动测试；按工具指示重试成功，没有遗留审批阻塞。早期 httptest loopback 沙盒限制通过内存 transport 与必要的测试端口许可解决。

### 产物证据与耗时

构建/审计目录原为 `/private/tmp/r232-pack-public/win-unpacked`，验证后保留为 `/private/tmp/r232-pack-public/win-unpacked-public`；暂存后端保留为 `/private/tmp/r232-backend-public.exe`，Windows 测试程序为 `/private/tmp/r232-backend-tests-public.exe`。均有 public 标识，不作为发布产物。仓库内生成的暂存 exe/摘要已清理，后续正常构建会重新生成；安装包包装/发布未执行。

- 源码指纹 `8b2449e279b8`；后端 32,476,672 字节，SHA-256 `176b955c71984064ca8217bbc653fba90e90ed1262849daccafd51ed18010c81`。
- [package-audit.json](history/reports/r232/package-audit.json) 读取实际 PE 的五个 fuse 值与 `INTEGRITY/ELECTRONASAR` 资源；ASAR header 的 SHA-256 与 PE 资源一致。ASAR 内运行模块齐全，不包含测试夹具，后端内未检出测试 issuer 标识。
- 对实际构建的后端副本追加字节，即使同时提供相邻 `.sha256`，运行校验函数仍拒绝；实际 ASAR 副本修改 header 后与 PE 完整性摘要不符。**这些是 macOS 上的静态/校验函数验证，不等于 Windows 真正拒启。**修改副本已删除。
- 本机 macOS 对该后端连续 10 次流式摘要校验：最小 16.89 ms、中位 19.75 ms、最大 20.71 ms。属于本机热缓存测量，未测 Windows 冷启动耗时，不把工单估算 50–100 ms 写成真机结果。

## T03–T19 与残余边界

**T03–T19 的正式验收全部未完成，未标为通过。**假服务/合成数据仅验证客户端分支：

| 验收 | 本轮替代证据 | 正式状态 |
|---|---|---|
| T03–T06：接管、断网、并发、重启不抢回 | 两个客户端管理器 + 假签发方；请求绑定、串行请求、持久化终止、手动接管 | 未完成：真实服务端原子占用与两台 Windows |
| T07–T10：共用身份、管理员、复制、时间 | 内存身份/缓存、普通与管理员签名测试、到期与回拨拒绝；Windows DPAPI 交叉编译 | 未完成：真实 DPAPI、安装/portable/升级、休眠冻结与多机 |
| T11–T12：副作用失效、篡改/重放 | Go HTTP/LCU 在途取消和代际、桌面 IPC、签名/字段/设备/请求绑定与计数器测试 | 未完成：真实 LCU 和服务端重放裁决 |
| T13–T15：篡改、攻击边界、升级 | 实际目录静态 fuse/ASAR/后端审计、摘要拒绝修改副本、更新签名和包校验测试 | 未完成：Windows 改 ASAR/放 resources/app/替换后端后的实际启动、自研 payload、合法升级 |
| T16–T19：故障、主题键盘、隐私审计、轮换 | 假服务不可达与到期恢复、Chromium 主题键盘、客户端脱敏测试、产物排除测试夹具 | 未完成：真实服务不可达/额度、Windows、Worker 日志与管理员泄漏/轮换演练 |

上线前还需独立项目完成服务、码池、公开公钥/合同对齐、服务端隐私期限/删除渠道和独立更新清单签名，并完成工单规定的真实服务与 Windows 验收；按用户决策先发码给老用户再发版。

本机软件密钥/DPAPI、文件完整性与签名可以提高普通篡改和凭据复制的成本，不能保证抵抗本机高级二进制补丁、完整用户配置/虚拟机克隆、回滚本地快照。跨进程离线启动仍依赖可用墙钟与本地记录，不能把它写成硬件级防回拨保证。旧机离线可用至已签发租约到期；心跳发现接管还包含实际请求耗时。P4b 服务端资源边界未实施；不宣称“无法破解”或“任何时刻零重叠”。

## R233 客户端复核修复与联调构建

日期：2026-10-06；时间均为 Asia/Shanghai。基线仍为 `10933787` / **0.12.75**，共享工作区已有 R232 客户端及其他工单改动；本轮只增量执行 R233 的 P1、P2、P6 客户端联调构建和向量测试框架。先亲自通读 R233 全文，未实现 P3/P4/P5/P7 服务端内容，未改 `relay/`、网络协议字段、签名域、协议版本或产品版本；未提交发布、创建 tag 或发布产物。

**状态：P1/P2 修复及本地验证完成；P6 构建拆分完成，跨仓库签名向量验收未完成。生产授权公钥和独立更新公钥均为空，发布被阻塞。** 没有生成、猜测或写入 production/staging 配置密钥；既有测试用测试 issuer 与测试设备密钥只在测试中运行，未编译进产物。

### 实际改动

| 范围 | 文件及结果 |
|---|---|
| P1 | `backend/license.go` 命名常量 300 秒；墙钟/单调计时分开，小回拨保持业务 context 和代际，运行截止时间不延长；超过容差立即锁定并唤醒续租。启动允许本地时间慢 60 秒，保存独立租约检查点，失败请求不能刷新缓存寿命。`license_test.go` 原 1 秒拒绝断言改为累计 1/300 秒允许、301 秒拒绝。 |
| P2 | `license_device.go` 单文件原子写先尝试一次，再按 50/200/800 ms 重试三次（最多四次），清理失败临时文件。请求前计数器保存失败不发请求，按 15/30/60 秒退避、有效会话继续；验签租约保存失败仍保留内存租约，下一次实际心跳保存；REPLACED/REVOKED 保存失败仍立即取消业务并锁定，仅重试落盘。新增脱敏失败/恢复诊断，运行中写入失败不进入 DEVICE_ERROR。 |
| 用例 | `license_r233_test.go` 覆盖慢 60 秒续租后离线重启、运行回拨 5 秒不取消、不延长单调期限、10 分钟回拨立即续租、失败请求不刷新缓存检查点、1–2 次计数器写入失败后的真实 Run 心跳、租约写失败后下一心跳恢复、持续失败至到期及无需输码自动恢复、两种终态连续写入失败与无续租恢复、原子 Rename/写入重试及临时文件清理；断言状态、context、诊断、计数器请求与心跳。 |
| P6 配置 | 新增 `license_config_release.go`（`!license_staging`）与 `license_config_staging.go`（`license_staging`）和各自标签测试，分别提供 origin、licenseTrustKeys/updateTrustKeys；信任表均为空等待公开配置。`build-desktop.sh` / Windows 脚本显式 `-tags=`；不添加运行时切换地址或免授权环境变量。 |
| P6 联调 | 新增 `scripts/build-license-staging.cjs`，手动、独立临时源码目录、Windows dir、公钥未配置保持锁定；产物目录/exe 标注 STAGING-public、窗口标题模块 `desktop/app-title.cjs` 为 STAGING。`desktop/main.cjs` 阻止网页标题覆盖配置标题。`verify-license-staging-pack.cjs` 核对 staging 后端、public key mode 和摘要，打包后复核 ASAR 标题与 exe 名字。 |
| P6 正式护栏 | 新增 `desktop/verify-license-release.cjs` / `license-release.test.cjs`，beforePack、release-build receipt 与 CI 扫描 staging 域名、测试 kid/公钥（文本及原始字节）。source-fingerprint 纳入新构建输入，package.json 仅新增运行标题模块，版本未变。正式流程不调用 STAGING 打包脚本。 |
| P6 向量 | 新增 `backend/testdata/license-protocol-vectors.json` 与 `license_protocol_vectors_test.go`；当前仅无签名解析案例与签名文本契约，`ready:false` 明确跳过独立服务签名向量。就绪后复算两端点设备签名、验证响应并逐字段核对预期载荷，要求 standard/admin、八错误码及至少六类拒绝覆盖。夹具明确不进入 go:embed；没有生成假共享向量或配置测试公钥。 |
| 文档/回归 | `docs/license-protocol.md` 补时间、存储、标签、向量容器说明（网络 v1 字段/签名域不变）；构建/启动夹具补标题和检查模块；R225 CI 筛选断言纳入新的 Go 标签测试；CI 把 R232 Chromium 激活护栏与既有 R117 护栏分成独立且不允许跳过的步骤。索引追加 R233。 |
| 清理 | 已删除 `dist/.r232-review-src.tgz`（19,767,727 字节），未删除其他 dist 产物。构建后仓库暂存 exe/摘要已清理，保留明确 public 名称的临时验证产物。 |

### 最终实跑记录

原始日志（gzip）与每项开始/结束、耗时、exit code、最后五行保存在 [verification.json](history/reports/r233/verification.json)；前端计数见 [renderer-summary.json](history/reports/r233/renderer-summary.json)。下面五项均在最后一次 Go 文件（含测试）修改 **18:32:15** 之后执行。

| 检查 | 运行时间 | 墙钟耗时 | 结果 |
|---|---|---:|---|
| `go test -count=1 ./backend` | 18:33:13–18:35:30 | 136.909 秒 | 通过；Go 最后修改 18:32:15 之后运行，未使用结果缓存 |
| `go test -count=1 -race ./backend` | 18:37:02–18:39:30 | 148.238 秒 | 最终通过；首轮仅 R206 耗时门槛失败，见下文 |
| `go vet ./...` | 18:33:12–18:33:15 | 2.366 秒 | 通过，无输出，exit 0 |
| `node scripts/test-renderers.cjs all` | 18:33:07–18:34:34 | 87.557 秒 | 1307 通过、4 跳过、0 失败；每文件及全套预算通过 |
| `在 desktop/ 执行 node --test` | 18:34:47–18:36:01 | 74.069 秒 | 292 项：290 通过、2 平台跳过、0 失败 |

各项最后 5 行（不足 5 行按实际保留）：

`go test -count=1 ./backend`：

```text
ok  	lol-loot-assistant/backend	122.080s
```

`go test -count=1 -race ./backend`：

```text
ok  	lol-loot-assistant/backend	147.181s
```

`go vet ./...`：

实际输出为空，进程 exit code 为 0。

`node scripts/test-renderers.cjs all`：

```text
    },
    "duration_ms": 87514.025416
  },
  "success": true
}
```

`在 desktop/ 执行 node --test`：

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 74021.565666
```

其他实跑：

| 检查 | 时间/耗时 | 结果 |
|---|---|---|
| `focused` | 18:28:56–18:29:04 / 7.469 秒 | R232/R233 定向通过；共享签名向量显式 SKIP |
| `focused-race` | 18:32:15–18:32:30 / 15.212 秒 | 最终 R233 心跳集成用例及缓存容差定向 race 通过；共享向量 SKIP |
| `build-tests` | 18:28:50–18:29:00 / 9.395 秒 | 构建/启动/指纹/质量护栏通过，2 项平台跳过 |
| `staging-config` | 18:30:02–18:30:10 / 7.815 秒 | staging origin/STAGING 标签/空公钥测试通过；共享签名向量 SKIP |
| `js-syntax` | 18:35:32–18:35:46 / 13.921 秒 | 277 JS/CJS/MJS 文件语法通过 |
| `windows-tests` | 18:35:31–18:35:48 / 17.181 秒 | CGO=0 Windows/amd64 测试 PE 交叉编译通过，未运行 Windows 测试程序 |
| `public-build` | 18:30:23–18:30:43 / 20.372 秒 | 正式标签 public Windows dir 构建通过，不是安装器包装/发布 |
| `staging-build` | 18:29:45–18:29:55 / 10.47 秒 | 独立 license_staging Windows dir 构建通过 |
| `package-audit` | 18:33:58–18:33:59 / 1.304 秒 | 实际两个目录包静态审计通过；未启动 Windows 真机 |
| `r206-race-recheck` | 18:36:41–18:36:52 / 11.578 秒 | 既有 R206 超时门槛用例独立 race 连续三次通过 |

### 构建证据与失败记录

实际构建及静态审计见 [package-audit.json](history/reports/r233/package-audit.json)。key mode 均为 **public**，Electron 43.3.0，产品版本 0.12.75。正式源码指纹 `96147ca0ede7`；STAGING 独立快照指纹 `e9e17e317514`（标题配置不同）。

- 正式目录：`/private/tmp/r233-pack-public/win-unpacked-public`；后端 32,482,816 字节，SHA-256 `b919e496ad9c830f1a22345df1f8f886d44a33e86635d4d5089536474855216b`。检查器通过，未检出 staging 域名/已知测试信任标记；打包标题为 Deep Legends。
- STAGING 目录：`/private/tmp/r233-pack-STAGING-public/Deep-Legends-STAGING-public`；exe 为 `Deep Legends-STAGING-public.exe`、ASAR 标题模块为 Deep Legends-STAGING；后端 SHA-256 `035fe339b84a5003d0d8bc31d7a681717134ffd5195ff2e81b0decdc4c949e54`。实际调用正式检查器确认它因 staging 域名被拒绝。
- 两包均验证五项 Electron fuse、PE 的 ELECTRONASAR 完整性资源与 ASAR header 摘要一致、后端与 ASAR 内固定摘要一致，运行模块齐全且无测试/向量/联调构建工具。只做 macOS 静态验证，不声称 Windows 真正拒启或实际窗口显示通过。
- 独立后端 `/private/tmp/r233-backend-public.exe`、Windows 测试程序 `/private/tmp/r233-backend-tests-public.exe` 亦保留 public 名称。一次未设置 GOOS 的附加测试编译仅为 macOS 编译，已另名为 `r233-backend-tests-macos-public`，未作为 Windows 证据；最终 Windows 程序经 file 确认 PE32+。
- STAGING 首轮复制规则误将 `generate-backend-digest.cjs` 当生成文件排除，构建在源码指纹阶段失败；改为精确文件名排除后，完整重跑通过。
- Node 全套首轮两项失败：R117 护栏精确步骤断言与合并的 R232 CI 步骤不匹配、R225 筛选数量由 3 变为 4。修正 CI 步骤拆分及筛选断言后，定向 12/12、最终全套通过；未削弱浏览器护栏。
- 首次最终完整 race 未报告数据竞态，但 `TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds` 实测 3.066 秒，超过已有 3 秒门槛。未修改该业务/测试门槛；独立 race 连续三次通过，再单独完整 race 通过。将首轮失败日志保留，不能说此既有耗时敏感问题已根治。
- 曾有一轮 Go 全量通过但早于随后心跳测试完成同步的最后改动，记录为 earlier_attempts；未用于最终结论，最终普通全量已重新实跑。

### 未完成项与残余边界

| 项 | 真实状态 |
|---|---|
| P3/P4/P5/P7 | 独立项目 deep-legends-manage 的工程、授权服务、码表、管理页、运维与隐私未做；本仓库无新增服务端代码，relay 未改。 |
| P6 / S15 共享签名向量 | 未完成。服务端未提供测试公钥/固定测试设备种子/签名向量；客户端框架和无签名解析契约通过不能替代 Go/TypeScript 双向验签。当前测试明确 SKIP。 |
| P6 公钥配置 / S18 完整上线验收 | 生产与 staging 的 licenseTrustKeys/updateTrustKeys 均为空。**发布被阻塞**，须由用户/服务端提供对应公开公钥后配置；未索要注册码、私钥或管理页。正式包隔离检查已经实跑，但尚不能上线。 |
| S16、R232 T03–T19 | 真实授权服务、两台 Windows 真机、DPAPI、游戏副作用、接管/离线/管理员、篡改实际启动、升级及轮换验收仍未完成，没有标为通过。 |
| 签名更新与安装器发布 | 本轮只构建 Windows dir，未生成发布安装包、未发布、未更改版本。签名清单发布端仍未对齐。 |

运行中每份租约仍受单调时钟 900 秒上限约束；离线跨进程重启只能依赖受保护的本地墙钟检查点，≤300 秒回拨可能多得约 300 秒，单次恢复的剩余时间仍≤900秒，不承诺防本地快照/反复时钟操纵。终态持久化失败时当前进程立即锁定并重试，但在真正落盘前退出可能留下旧缓存；重启后的权威裁决依赖在线续租。没有改成磁盘故障时全部放行，也没有新增跳过授权的发布开关。

## R234 精确 JSON 字段与共享向量验收

日期：2026-10-06，时间均为 Asia/Shanghai，版本保持 **0.12.75**。动手前亲自通读 R234 全文，仅执行本仓库 P1、P2、P3 的可执行部分；没有修改 deep-legends-manage，P4 未执行。网络协议字段、签名域、版本未改，未发布。

**P1/P2 实现、59 条向量及两项变异验证完成；P3 公钥隔离测试完成，实际 staging/production 公钥仍未提供，发布被阻塞。全仓 Go/race 验收未通过，不能把本工单整体写成全量验收通过。**

### 根因、修复与审计范围

- 根因：encoding/json 的结构体匹配会接受大小写别名，DisallowUnknownFields 无法拦截；旧客户端还保留 ready:false 占位向量，S15 被跳过。
- `backend/license.go` 的 strictLicenseJSON 在标准解码前按目标类型的 json 标签精确匹配对象键；支持匿名结构体/指针字段提升、深度与标签优先级、嵌套指针、切片/数组元素；歧义字段拒绝。动态 map 的键是数据，值仍递归按声明类型校验。继续拒绝重复/转义重复键、未知字段与尾随 JSON，没有 EqualFold 放宽字段名。
- 调用点覆盖：授权响应 envelope、授权签名 payload、请求 envelope（假服务/向量）、DPAPI 状态文件及其嵌套 lease、`/api/license/activate` 输入、向量容器/各类行/预期载荷。新增 `backend/license_json_test.go` 独立用例覆盖 Version/VERSION、status+Status、嵌套、数组、map 值、匿名指针、忽略标签和正常字段/本机状态读回。
- 其他签名内容审计：`update.go` 网络 fetchManifestSource、`update_signature.go` 签名载荷都已调用 strictLicenseJSON，因而继承同一缺陷并随本轮修复；新增真正签名的别名载荷、网络 envelope/嵌套 asset、正常清单用例。`update.go` 的 update-manifest.json 缓存曾使用普通 json.Unmarshal，本轮改为严格解析，覆盖缓存 manifest/envelope/asset 别名与正常恢复。`update_download.go` 下载及安装前再次调用 verifyUpdateManifestTrust，没有独立 JSON 解码入口。未发现另一个独立服务端签名域的普通解码入口；更新设置、安装耗时和普通 Riot/LCU 数据等非签名 JSON 不在本轮修改范围。

### 原样向量与变异验证

- 来源：`deep-legends-manage/test/license-protocol-vectors.json`，31,615 字节，fixture_version=1 / protocol_version=1，ready=true。原样复制到 `backend/testdata/license-protocol-vectors.json`；两份文件字节相同，SHA-256 均为 `053eaa3841c73b15cd4f46c1c08e6386896be935d3622a7473515c4473d45c0e`。未手改向量，未生成签名配置密钥。
- 59 条组成：normalization 9、base64url 5、strict_json 11、requests 10、responses 24。加载器固定交接 SHA、ready、版本、条目计数和非空公钥；缺失/未就绪/类别消失全部失败。三个 S15 入口均要求最终夹具，已删除 Skip 路径；保留两端点、standard/admin、八错误码和拒绝类别断言，并补齐 lease/trailing_json/unknown_field 类别。
- `TestR233ProtocolParserVectors`、`TestR233RequestSignatureTextContract`、`TestR233SharedSignedProtocolVectors` 实跑全部通过，含 case-alias-request、case-alias-envelope、case-alias-payload；最终普通定向、staging 标签及授权专项 race 输出均 **0 SKIP、0 FAIL**。这是客户端对独立服务交付文件的验证，本会话没有重跑服务端 Worker 或做实际部署。
- 两项变异只在临时完整 backend 副本实施，结果与日志见 [mutations.json](history/reports/r234/mutations.json)：响应域改为 DL-LICENSE-RESPONSE-MUTATED，成功租约和八种签名错误响应均失败；精确解码改回旧行为，恰好三条别名向量失败。两次 exit code 都为 1，属于预期失败；无 SKIP、无编译失败，临时副本随后删除，正式签名域与代码未变异。
- `license_config_release.go` / `license_config_staging.go` 的 licenseTrustKeys/updateTrustKeys 仍为空。新测试在默认和 staging 标签下比较向量 kid/公钥与两套信任表，并检查两配置源文件；test-license-r233 没有写入任一配置。真实 staging 公钥写入 **未完成，等用户提供**；不能用向量公钥代替。
- 正式构建检查增加向量设备种子的文本/原始字节排除，并以真实向量的 kid、公钥文本/原始字节、种子验证拒绝；go:embed 与包文件清单继续排除 testdata。协议文档追加来源、SHA-256 与 fixture_version 交接说明。

### 指定检查的实际结果

最后一次 **R234** Go 文件修改为 21:17:21；[source-files.json](history/reports/r234/source-files.json) 保留本轮文件的 SHA 与时间。验证期间其他会话持续修改 gameplay、season、riot_relay/riot_api 和前端等共享源码，本轮授权文件 SHA 未再变动。因此这里区分本轮专项通过与全仓失败，不沿用早于共享 Go 改动的全量通过。原始日志、最后五行及时间见 [verification.json](history/reports/r234/verification.json)。

| 检查 | 时间 | 墙钟耗时 | 实际结果 |
|---|---|---:|---|
| `go test -count=1 ./backend` | 21:33:09–21:34:14 | 64.279 秒 | 失败：共享中转改动后的 R206/R208 用例失败，riotHTTPErrorBody 空指针；未修复超范围代码 |
| `go test -count=1 -race ./backend` | 21:29:19–21:30:49 | 90.139 秒 | 失败：R206/R208 中转测试竞态，随后同一空指针退出 |
| `go vet ./...` | 21:33:07–21:33:09 | 2.084 秒 | 通过，实际输出为空 |
| `node scripts/test-renderers.cjs all` | 21:23:28–21:25:31 | 122.246 秒 | 最终通过：1308 通过、4 平台跳过、0 失败，原预算未改 |
| `desktop/ 下 node --test` | 21:26:55–21:28:34 | 98.831 秒 | 通过：291 通过、2 平台跳过、0 失败 |

各项最后 5 行（不足 5 行按实际保留）：

`go test -count=1 ./backend`：

```text
	/opt/homebrew/Cellar/go/1.24.5/libexec/src/testing/testing.go:1792 +0xe4
created by testing.(*T).Run in goroutine 1
	/opt/homebrew/Cellar/go/1.24.5/libexec/src/testing/testing.go:1851 +0x374
FAIL	lol-loot-assistant/backend	51.906s
FAIL
```

`go test -count=1 -race ./backend`：

```text
	/opt/homebrew/Cellar/go/1.24.5/libexec/src/testing/testing.go:1792 +0x184
created by testing.(*T).Run in goroutine 1
	/opt/homebrew/Cellar/go/1.24.5/libexec/src/testing/testing.go:1851 +0x688
FAIL	lol-loot-assistant/backend	74.723s
FAIL
```

`go vet ./...`：

实际输出为空，exit code=0。

`node scripts/test-renderers.cjs all`：

```text
    },
    "duration_ms": 122202.121708
  },
  "success": true
}
```

`desktop/ 下 node --test`：

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 98737.442084
```

补充检查：

| 检查 | 时间 / 耗时 | 结果 |
|---|---|---|
| `focused-final` | 21:17:24–21:17:54 / 29.971 秒 | 精确字段、更新签名/缓存、本机状态及 59 条向量通过；S15 无 SKIP |
| `staging-guards-final` | 21:20:09–21:20:33 / 24.268 秒 | staging 空信任表、向量公钥隔离及 S15 通过；无 SKIP |
| `license-race` | 21:36:40–21:36:58 / 17.435 秒 | R232/R233/R234 授权相关全部通过；S15 无 SKIP |
| `public-build` | 21:15:00–21:15:11 / 11.341 秒 | public Windows dir 构建通过；版本0.12.75，不发布 |
| `package-audit` | 21:16:16–21:16:18 / 1.712 秒 | 实际包静态隔离、fuse、ASAR 与后端摘要通过 |
| `js-syntax` | 21:16:29–21:16:49 / 19.633 秒 | 276 文件通过；此后共享前端改动不属于该结果 |
| `windows-compile` | 21:16:33–21:17:32 / 58.707 秒 | Windows/amd64 CGO=0 测试程序交叉编译通过，未真机运行 |

### 失败、共享改动与构建边界

- 新增歧义结构负例最初故意静态声明重复 json 标签，被 vet 正确拒绝；改为运行时 reflect.StructOf 构造同一歧义，保持拒绝断言，不关闭 vet。最终专项与 vet 重跑通过。
- 前端首轮 R192 挂载超时、第二轮 R206 的原有 100ms 断言失败；没有改业务或门槛。在构建与 Go 检查结束后，原命令独立全套重跑通过。三轮日志都保留，不断言这些耗时敏感问题已根治。
- Go 普通全量 21:18:42–21:20:39 曾通过（115.515s），但之后出现共享 Go 改动，当前全量重跑失败：TestR206RelayEmptyAndFailureCooldown、TestR206Relay429PreservesBackoff、TestR208RelayQuotaDaily 等，最终 TestR208RelayApplicationSharedAcrossProviders 调用 riotHTTPErrorBody 空指针。完整 race 同路径还有竞态警告。涉及正在修改的 riot_relay.go/riot_api.go，本会话未修改这些文件，没有超范围替其修复，也没有把失败记为通过。
- 授权专项 race 的首次编译遇到共享 gameplay.go 临时引用尚未定义的 loadCurrentHistoryFast，构建中断；再次运行 R232–R234 授权专项完整通过。中断日志保留。
- public 构建于 21:15:00–21:15:11 完成，指纹 `276fdbb2073d`，目录 `/private/tmp/r234-pack-public/win-unpacked-public`，key mode public、版本0.12.75；后端 SHA-256 `d0c8e82794143023da1fe06b7df548bb9c6599a63d139092552561e8e6f4583a`，32,490,496 字节。实际扫描未检出向量 kid/公钥/种子，ASAR 无测试夹具，五个 fuse、完整性资源与后端摘要匹配，详见 [package-audit.json](history/reports/r234/package-audit.json)。
- 构建后仅本轮测试夹具有调整，R234 运行代码未再改变；后续其他会话使全仓源码指纹变化，**此包不是后续共享源码的构建**。仓库暂存 exe/摘要已清理，独立后端 `/private/tmp/r234-backend-public.exe` 与 Windows 测试程序 `/private/tmp/r234-backend-tests-public.exe` 保留 public 名称。未生成发布安装包，未运行 Windows。

### 未完成项

- 全仓 Go/race 验收 **未通过**；需要其他共享业务改动完成后，对最终稳定源码重新全量验证。本轮专项、前端及 desktop 的通过不替代它。
- staging 与生产签名公钥写入、服务部署、两台 Windows S16 / R232 T03–T19、真实密钥轮换和隐私政策定稿仍未完成；**发布被阻塞**。
- P4 独立项目 D1 写回/未知设备优化未做，等用户另行决定；没有写任何服务端、码表或管理页代码。

## R236 staging 公钥交接、更新隔离与 S16 清单

日期：2026-10-06；时间均为 Asia/Shanghai。本会话仅执行本仓库 P2/P3/P4，已亲自通读 R236 工单、项目基础文档及 R234 交接。P1 由另一个会话在独立仓库执行；本会话未读取或修改 deep-legends-manage。版本保持 **0.12.75**；不创建 tag/Release，不发布、不更新 latest.json、不部署 Cloudflare。

### P2 公钥与 P3 更新隔离

- 写入前用 base64url 字符集、解码长度与重新编码一致性核验：用户公钥 `E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE` 为规范无填充 base64url，解码 **32 字节**。`backend/license_config_staging.go` 仅信任 kid `staging-2026-10`；不是共享向量公钥。release 的 licenseTrustKeys/updateTrustKeys 仍为空，**发布被阻塞：缺生产公钥**。R234 P3 的“staging 保持为空”要求至此由本次用户交接覆盖，历史验收记录保留。
- 默认与 staging 标签分别断言生产空信任表、staging 单一 kid/准确公钥、两配置不得含 test-license-r233 或向量公钥；沿用 R234 两配置源文件及原始公钥比较。没有获取或索要私钥，没有读取真实注册码。
- 更新审计范围：update.go 构造器/缓存/设置/Start/Check/fetchManifest，update_download.go Download/Apply/ApplyAsync，update_http.go 路由与设置，以及启动/SSE 状态入口。旧实现虽因空更新公钥拒绝正式签名清单，仍会访问正式 Latest，未形成独立的 staging 禁用政策。
- 现以互斥构建配置 `licenseOnlineUpdates` 关闭 staging 更新：构造器在读取本地缓存/设置及安装检测前返回，supported=false、releaseUrl 为空；Start/Check/Download/Apply/ApplyAsync/设置与 HTTP 操作再次拦截。不改正式更新来源、不增加运行时开关。旧有效签名缓存、有效注入更新信任表乃至模拟 ready 状态都不能启动网络或安装器；两个专项测试接入既有 CI staging 筛选步骤。
- 独立 staging 脚本在临时快照中构建 `-tags=license_staging`，public key mode 显式清空内置 Riot Key；沿用 NSIS + 自研安装/卸载壳，压缩级别 9，`--publish=never`。安装包/目录带 -staging-public，标题/快捷方式/卸载名 STAGING、独立 appId。安装壳内主程序名沿用 Deep Legends.exe，Windows 联调选择独立安装目录。构建钩子与最终包校验准确 origin/kid/公钥，排除已知测试 kid、公钥/种子、私钥 PEM 标记和个人 Riot Key；ASAR 排除测试/向量/构建工具并核对固定后端摘要。真实注册码未进入任何构建输入。

### S16 两台 Windows 真机清单（待用户执行）

**R243 更新：S16 统一使用 `dist/R243-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe`，SHA-256 `603572946e55911a544284843182dc783e42bca27a96a8166e07285f0838719d`。R242 及更早包不再用于本轮联调，文件保留、不覆盖、不删除，历史构建 SHA 留在原节。**

本轮用户不做 S16-07 校时；07a–07c 留为以后清单，当前不要求执行，不勾选通过。

前置：A、B 使用本节指定的同一 STAGING 安装包，核对 SHA-256，窗口标题为 Deep Legends-STAGING；安装到独立目录。两机联网、先恢复正确系统时间。用户自行保管普通码与管理员码，仅在激活输入框输入；清单、截图、录屏、诊断导出与反馈均不得含完整码。反馈只用“普通码/管理员码”、管理页标识或摘要；不要截取注册码输入、完整码列表/详情或 CSV 内容。每项记录时间、机器、实际文案和脱敏证据，全部仍为 **未执行**。

| 项 | 操作 | 预期现象 | 用户填写 |
|---|---|---|---|
| S16-00 授权窗口、切换与隐私 | 两机装新包；开关隐私，含 Esc；输入普通码激活；已授权退出后重启；普通大小及最大化两种使用中停用/重置后锁定，再明确重输码 | 未授权内容区固定 860×580 DIP、居中、不可拖大/最大化，输入可用；隐私有正文且可关闭；激活时看不到变形/卡片飘移；已授权重启没有激活卡片；普通/最大化锁定均缩为 860×580，再激活恢复原正常大小/位置或最大化 | □通过 □失败；显示缩放：____；各尺寸/状态：____；重启及切换录屏/时间：____ |
| S16-01 普通码激活/重启 | A 输入普通码激活；退出程序后立即重启，保持联网 | 激活成功、遮罩消失；重启保留已授权且可操作，不再次索码；续租正常 | □通过 □失败；时间：____；现象/证据：____ |
| S16-02 B 接管 | A 保持运行且联网；B 输入同一普通码；观察两台并记录间隔 | B 立即成功；A 下一次续租（正常约 1 分钟，另计实际请求耗时）显示“注册码已在其他设备使用或已被重置”；持续等待、重启或恢复网络均不自动抢回 | □通过 □失败；B 成功时间：____；A 遮罩时间：____；现象：____ |
| S16-03 A 再接管 | 在 A 明确重新输入同一普通码；B 保持联网运行 | A 成功；B 下一次续租显示“注册码已在其他设备使用或已被重置”；B 不自动恢复 | □通过 □失败；时间/现象：____ |
| S16-04 管理员多机 | A、B 分别输入管理员码，持续运行至少两次心跳；查看 staging 管理页设备数 | 两台都保持可用、不互相替换；对应管理员码设备数增加；重启仍可正常续租 | □通过 □失败；时间：____；设备数前/后：____；现象：____ |
| S16-05 停用/启用/重置 | 普通码在 A 激活；管理页停用，等 A 续租；再启用，观察后明确重输普通码；随后管理页重置，再观察旧设备并明确重输 | 停用后“注册码已停用”；启用不自动解除 REVOKED，明确重输后成功；重置后旧设备续租显示“注册码已在其他设备使用或已被重置”，不能自动恢复，明确重输才重新绑定；锁定须缩为 860×580 | □通过 □失败；各操作时间：____；尺寸/现象：____ |
| S16-06 断网与恢复 | 普通码重新在 A 激活；断网并保持程序运行；在租约内观察，随后持续断网超过 15 分钟；恢复网络。另复查 REPLACED/REVOKED 后仅恢复网络 | 有效租约剩余时间内可用（不保证从断网起整整 15 分钟）；到期显示“网络不可用”遮罩；恢复后按 15/30/60 秒退避及成功续租自动恢复；被替换/停用终态不会仅因网络恢复而解锁 | □通过 □失败；断网/锁定/恢复时间：____；现象：____ |
| S16-07a 300 秒内校时 | 有效会话下关闭自动校时，回拨 60 秒及至 300 秒边界；各次观察后恢复正确时间 | 不误锁正在使用的有效会话，不取消业务；单调租期不延长、仍至多 900 秒 | □通过 □失败；幅度/时间/现象：____ |
| S16-07b 大幅回拨 | 有效会话下向后调整 10 分钟；观察后恢复正确系统时间并保持联网 | 超过 300 秒回拨进入网络类锁定、立即唤醒续租；成功签名续租可自动恢复，无需重输码；不能把旧租约无限延长，也不变成被替换/已停用终态 | □通过 □失败；调整/锁定/恢复时间：____；现象：____ |
| S16-07c 大幅前调 | 有效会话下向前调整 10 分钟，观察；再向前调整到超过原租约墙钟截止；恢复正确时间并保持联网 | 前调本身不按“回拨”误判；未越过有效截止时仍可用；一旦越过截止保守进入网络类锁定，成功续租恢复；恢复正确时间后不得把网络类锁定变为终态。记录实际续租与到期顺序 | □通过 □失败；幅度/时间/现象：____ |
| S16-08 管理页显示 | 用户在 staging 管理页生成测试码，刷新列表、打开详情并复制；检查旧版本生成的测试码 | 新版生成的码可随时显示/复制；没有保存密文的旧码显示“旧版本生成，无法显示”，需重置或重新生成；反馈不包含完整码 | 未在 Windows 实跑；□通过 □失败；时间/现象：____ |
| S16-09 更新隔离补查 | 两机尝试检查更新，重启程序观察更新入口；若有旧正式更新缓存也重复观察 | 不显示正式 Latest/发布页链接，不检查或下载正式包、不启动正式安装器；更新入口隐藏或返回当前构建不支持更新；标题始终 STAGING | □通过 □失败；时间/现象：____ |
| S16-10 普通码过期与续期 | 普通码设置“首次激活起算 1 天”并激活；在管理页把到期日改到当前时间之前；观察、重启、恢复网络；续期后明确重新输入同一码 | 客户端在 1 分钟内显示“注册码已过期”；立即锁定，重启仍是已过期，不显示已停用；仅续期/联网不自动恢复，续期并明确重输后恢复 | 未在 Windows 实跑；□通过 □失败；操作/锁定/重启/重输时间及文案：____ |
| S16-11 自定义管理员码修改 | 自定义管理员码激活两台电脑；在详情抽屉“修改注册码”；观察两台续租，再分别明确输入新码 | 两台均显示“注册码已在其他设备使用或已被重置”，不能自动恢复；两台都需输入新码，输入后可同时使用 | 未在 Windows 实跑；□通过 □失败；两机状态/时间/现象：____ |
| S16-12 设置页到期信息 | 有效期码激活后进入设置→隐私与能力；核对到期日与剩余天数；续期后再次查看；永久码重复查看 | 显示一行“授权到期：日期（剩 N 天）”，续期后更新；永久码显示“授权到期：永久”；不增加其他提示或弹窗 | 未在 Windows 实跑；□通过 □失败；日期/剩余天数/永久/续期更新：____ |
| S16-13 管理页开着激活管理员码 | staging 管理页保持前台自动刷新，在另一台电脑输入管理员码激活，记录开始/成功时刻并导出诊断 | 一次点击成功，总耗时在几秒内；诊断可区分每次请求的网络/服务端失败原因，不需要人工重复点击 | 未在 Windows 实跑；□通过 □失败；时间/总耗时/请求次数：____ |
| S16-14 覆盖安装首次网络恢复 | 保留原 window-bounds.json 与授权缓存，覆盖安装 R243 包；分别验证续租立即成功、首次失败后自动成功；包含普通与最大化保存状态 | 先显示“无法连接”时内容区为 860×580；自动恢复 ACTIVE 后为保存的大小/位置/最大化；无记录用默认尺寸；导出含实际内容区、fallback_used、分段耗时 | 未在 Windows 实跑；□通过 □失败；保存/锁定/恢复尺寸与状态/各时刻：____ |

这份清单不等于真机通过。S16、DPAPI 跨机/账户、真实续租/管理页和 Windows 安装运行均待用户实测；P1 服务端部署由用户及另一会话负责。生产公钥、独立更新签名上线和隐私期限/删除联系方式仍待确认。

### 实际构建、包审计与变异验证

- 工单已确认 P1 根因是服务端取 Access certs 的 fetch 使用 Workers 不支持的 redirect:error；Node 测试模拟绕过平台参数校验，异常被吞后统一 FORBIDDEN。服务端改 manual、workerd 回归/原因码日志属于另一会话，本会话没有复跑或声称 P1 通过。
- 本机 macOS 成功交叉构建 **自研 Windows 安装包**，不是仅交付 dir，也无需启用 CI 构建/发布。构建时间 23:15:05–23:25:50，644.066 秒，key mode **public**、license tag **license_staging**、版本 **0.12.75**、快照指纹 **008a3d957b9a**。窗口标题/快捷方式/卸载显示名 STAGING，主程序内部文件名仍为 Deep Legends.exe。临时源码快照构建后已删除；仓库普通 desktop/backend 和安装器 payload 未写入此次暂存二进制。
- 安装包：`dist/R236-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe`；目录：`dist/R236-staging-public/Deep-Legends-staging-public/`。仅为本地联调产物，未创建 tag、GitHub Release 或 Latest，未更新 latest.json。完整构建输出（含最后五行）见 [staging-build.json](history/reports/r236/staging-build.json) / [原始日志](history/reports/r236/staging-build.log.gz)，最终收据见 [staging-build-receipt.json](history/reports/r236/staging-build-receipt.json)。关键实际输出：`Installer shell built: Deep Legends Setup 0.12.75-public.exe (349226205 installed bytes)`，内部临时 NSIS/public 名称仅在快照中使用，最终保留文件已加 -staging-public。
- 安装包 SHA-256：`a33b60ba4434a25f1a77f5f08381d12ff0673e6f208659236677c9f98002b7f5`；包内后端 SHA-256：`a895505ee6660158a4ca96e1bb14ef0f3222a1c2fc91d49b07934f0dc7e80a8d`；ASAR SHA-256：`7713f3407129b449828ca701c78f2f359e41001a192fb4c97426bb6222a2243f`。校验文件 [SHA256SUMS-staging-public.txt](history/reports/r236/SHA256SUMS-staging-public.txt) 同时保存在产物目录。
- 23:26:40–23:26:42 对实际产物静态审计通过：[package-audit-details.json](history/reports/r236/package-audit-details.json)。PE32+ GUI/amd64 安装壳、ASAR 版本、真实后端 origin/kid/公钥、public Riot Key 扫描、测试/向量材料排除、固定后端摘要、五项 Electron fuse、PE 内 ELECTRONASAR 与 ASAR header 摘要一致均通过；正式检查器正确拒绝 STAGING 后端。没有运行 Windows 安装器，不把静态审计写成真机通过。
- 无 license_staging 标签的实际 release 后端 `/private/tmp/r236-backend-public.exe` 已重建：23:20:28–23:20:55，26.706 秒，32,587,264 字节，SHA-256 `5ed7ccdb9692503c6a3bf5335cd05d23185f18d52c81203c11bce0352ad3bc1a`。扫描无 staging 地址/kid、公钥文本/原始字节、已知测试信任材料或个人 Riot Key，详见 [release-audit.json](history/reports/r236/release-audit.json)。生产信任函数未填 key，仍不可发布。
- 构建快照与工作区逐文件比较 310 个运行文件，无差异；仅标题模块按设计改为 STAGING，见 [build-source-comparison.json](history/reports/r236/build-source-comparison.json)。工作区运行源码指纹为 b351048322b4；本轮文件 SHA/时间见 [source-files.json](history/reports/r236/source-files.json)。审计时 HEAD 为 b37357b2a6cf29397ac0f16255d37fa1566e87ac，package.json 与 lockfile 均为 0.12.75；本会话没有提交或改版本。
- 四项变异只在临时完整 backend 副本中执行，随后删除：[mutations.json](history/reports/r236/mutations.json)。staging 公钥首字符 E→F、staging 增加第二 kid、release 混入 test-license-r233 的空 key、staging 重新启用在线更新，全部由预期测试断言杀死（exit 1），没有编译失败/测试跳过。更新变异仅运行实际缓存/HTTP 门禁测试，证明失败不只来自配置常量断言。未在工作区或产物中保留变异。

### 五项指定检查的最终实际结果

所有下列结果均在最后一次本轮源码修改后实跑；[verification.json](history/reports/r236/verification.json) 与各 `*.log.gz` 保存实际命令、时刻、耗时、exit code 和末尾五行。本轮 13 个代码/CI 文件 SHA 未在验证后改变。**指定全套检查仍未全部通过：两项 Node 全套共同失败于既有藏品重扫断言，不把专项通过写成全仓验收通过。**

| 检查 | 最终时间 | 墙钟耗时 | 实际结果 |
|---|---|---:|---|
| `go test -count=1 ./backend` | 23:18:00–23:22:10 | 250.048 秒 | 通过 |
| `go test -count=1 -race ./backend` | 23:22:48–23:29:08 | 380.846 秒 | 通过 |
| `go vet ./...` | 23:18:05–23:18:09 | 4.750 秒 | 通过（无输出，exit 0） |
| `node scripts/test-renderers.cjs all` | 23:32:17–23:34:44 | 147.463 秒 | 失败：1319 通过、4 跳过、1 失败；90 秒/240 秒预算均通过 |
| `desktop/ 下 node --test` | 23:35:55–23:37:44 | 109.591 秒 | 失败：292 通过、2 跳过、1 失败 |

各项最后五行（不足五行按实际保留）：

`go test -count=1 ./backend`：

```text
ok  	lol-loot-assistant/backend	209.440s
```

`go test -count=1 -race ./backend`：

```text
ok  	lol-loot-assistant/backend	204.064s
```

`go vet ./...`：

实际输出为空，exit code=0。

`node scripts/test-renderers.cjs all`：

```text
    },
    "duration_ms": 147381.292583
  },
  "success": false
}
```

`desktop/ 下 node --test`：

```text
    actual: undefined,
    expected: undefined,
    operator: 'fail',
    diff: 'simple'
  }
```

### 失败、补充验证与未完成项

- 首次定向 Go 未启动测试：沙盒拒绝系统默认 Go cache 的写访问（operation not permitted）。按权限机制允许使用系统缓存后重跑通过，没有改缓存位置或使用仓库缓存。
- STAGING 最终专项 23:14:33–23:14:46（13.752 秒）通过，含真实交接配置、两配置向量公钥隔离、S15 三个入口及缓存/HTTP 更新禁用；无 SKIP/FAIL。STAGING 专项 race 23:22:55–23:25:48（172.683 秒，含编译）通过；desktop 新增打包护栏定向、CI 筛选也通过。
- renderer 首轮 23:18:27–23:25:39 与构建并发：1313 通过、4 跳过、7 失败，并有单文件/全套预算超限（431.788 秒）；原始记录保留。构建完成后原命令独立重跑 147.381 秒、所有预算通过，但仍有下述同一条失败，没有调整并发、超时或断言门槛。
- desktop 两轮原命令都失败同一条：`refresh-orchestration.test.cjs:224` 的 `dirty collection rescans on entry and view changes without duplicate refresh requests`，:271 报 `view change after 60 seconds did not start the deferred rescan`。最后五行是 Node 报告末尾的断言属性（不是成功摘要），已按实际保留；完整最终日志含 292 pass / 1 fail / 2 skip。
- 只读独立核验并点验 app.js:452–459、:2736–2744 与测试 :249–271：该路径在 JS/JSDOM 与 mock API 中运行，没有调用 Go 更新器或构建脚本。测试等请求计数不等状态提交，可能使 clean/dirty 状态被请求淘汰、重扫 in-flight 门禁未释放；**仅为推断，根因未定案、未修复**。本会话没有修改 app.js 或 refresh-orchestration.test.cjs，也没有顺手改藏品业务或测试来取得绿灯。应另立工单核验这一重复失败；本轮完整 Node 验收仍未通过。
- **P2/P3/P4 客户端任务已交付**：公钥准确、更新隔离、四项变异和实际本地 STAGING 自研安装包/静态审计完成，S16 可照做清单已写好。**S16 实测未完成**，Windows 安装/启动与双机、真实服务/管理页和 P1 服务端验证未执行；不声称真实联调成功。
- 生产授权/更新公钥未提供、真实 S16 与隐私期限/删除渠道未定稿，**发布仍被阻塞：缺生产公钥**；没有用测试公钥兜底。未改版本、tag、Release、Latest、latest.json，未接触真实注册码或用户私钥。

## R237 授权隐私定稿与 staging 标签测试修复

工单日期：2026-10-06；本会话验证跨至 2026-10-07，时间均为 Asia/Shanghai。已亲自通读指定 R237 工单，仅实施本仓库 P1/P2；P3 在独立会话，本会话未访问 deep-legends-manage。版本保持 **0.12.75**，不发布、不改协议字段或签名域。

### P1 定稿文本与引用核查

用户确认 events 保留 180 天、管理员 admin_audit 保留 365 天，本期不提供删除渠道。365 天管理员审计不属于客户端用户数据，本次没有写入客户端文本，也没有编造联系方式或删除流程。两处运行文本严格采用同一句：

> 服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。

- `backend/features.go` 的 handlePrivacy / licenseDisclosure 与 `installer/ui/license.html` 仅替换原占位句，其余文字不动。安装器原句实际用“保存期限与删除方式”，与工单引用的“和”有一字不同；已按其实际原文替换，最终句逐字一致。修改前/后 SHA 与只替换一句的逆向一致性核查见 [privacy-replacements.json](history/reports/r237/privacy-replacements.json)。
- `docs/license-protocol.md` 仅更新“数据处理与验收边界”的隐私说明：事件 180 天后清理，授权记录/防重放计数器持续保留，文档写明本期不提供删除渠道、不承诺删除流程；这条文档说明没有追加到 UI。协议字段、签名域、v1、origin 及公钥未变。
- 已搜索 backend/installer/desktop/scripts 的活跃源码、测试及快照：原占位句只有上述两处，没有旧文案测试/快照断言需要更新；替换后活跃路径无旧句引用。工单和历史账本中的旧说明保留为历史，本节覆盖此前“隐私保存期限/渠道待确认”的当前状态。
- `backend/web/license-ui.js` :77–84 点击时从 `/api/privacy` 读取 licenseDisclosure 并直接写入文本，无硬编码副本、独立缓存或快照；设置页 app.js 的 loadPrivacy 同样读取 API，无需改前端。没有修改 license-ui.js、app.js 或 refresh-orchestration.test.cjs。
- 新增 `TestR237LicensePrivacyDisclosureFinal`，实际调用 handlePrivacy 并读取安装器 HTML，分别要求定稿句恰好出现一次，拒绝旧占位词、删除方式、联系及 365 管理员审计字样。定向实跑通过。

### P2 两种构建标签与变异

- 从通用 `TestR232NormalizeAndFailClosed` 移除构建配置“信任表必须为空”的断言；已有 !license_staging 的 `TestR233ReleaseLicenseConfiguration` 继续验证 release 两信任表为空，不重复新增。通用规范化和显式 `m.options.Keys = map[...]{} ` 后拒绝激活/放行的 fail-closed 检查完整保留。
- 定向核查还发现第二个 staging 陷阱：R234 缓存正常样本要求构造器读取正式缓存，但 R236 的 STAGING 构造器按设计在读取缓存前返回。原六个缓存子用例（五种大小写别名拒绝 + 一个正常读回）原样移到新 `license_update_cache_release_test.go` 的 !license_staging 入口 `TestR234SignedUpdateCacheJSONNames`；通用签名载荷/网络 envelope 测试仍保留。STAGING 对有效签名缓存也必须忽略的行为继续由 R236 专项验证，没有加入运行时绕过或使用 Skip。
- 两种标签均实跑用户要求的 `go test -count=1 -race -run 'R232|R233|R234|R236' ./backend`（附加 -v 留存逐项证据）：默认 23:59:27–23:59:49 / 21.672 秒，29 个顶层测试通过；license_staging 23:59:27–23:59:49 / 21.546 秒，30 个顶层测试通过。两者都是 **0 SKIP、0 FAIL**，S15 三入口均实际运行，见 [license-matrix-summary.json](history/reports/r237/license-matrix-summary.json) 与两份 race 原始日志。
- CI staging 步骤保持原来的三个点名用例，未改成宽匹配，未修改 ci.yml；`node scripts/verify-ci-test-filters.cjs` 实跑通过，筛选仍为 TestR233StagingLicenseConfiguration、TestR236StagingUpdatesDisabledWithTrustedReleaseCache、TestR236StagingUpdateHTTPGate。
- 四项变异仅在临时 backend + installer/ui 副本执行，随后删除：API 文案恢复旧句、安装器文案恢复旧句、release 加入一个 key、staging 信任表清空。四项都被对应断言杀死（exit 1），没有编译失败或未跑到测试的伪失败；工作区信任表与原包未变。详见 [mutations.json](history/reports/r237/mutations.json) 和同目录四份变异日志。

### 保护正在联调的 R236 包

- 不运行 STAGING 或 release 打包脚本，不重建/覆盖/删除 `dist/R236-staging-public/` 任何文件。本次文案变更只在源码，正在做 S16 的旧联调包保持不动。
- 开始时只读计算该目录全部 26 个文件的 SHA-256、大小、mtime_ns 和 mode，见 [r236-artifacts-before.json](history/reports/r237/r236-artifacts-before.json)。安装包 SHA-256 为用户指定的 `a33b60ba4434a25f1a77f5f08381d12ff0673e6f208659236677c9f98002b7f5`；完成后再次只读对比。
- 本轮最终源码与保护文件 SHA/时间见 [source-files.json](history/reports/r237/source-files.json)，包含未改的授权运行代码、两配置、CI、app.js 与 dirty collection 测试，便于核查没有扩展本单范围。

### 最终指定检查与末尾输出

下列均在最终源码修改后实跑，未沿用 R236 结果；[verification.json](history/reports/r237/verification.json) 和各原始 `*.log.gz` 保存实际命令、时间、耗时、exit code。**五项指定检查最终全部通过**，两种标签授权专项 0 SKIP/FAIL；验证后本轮代码及其它保护文件 SHA 未变；共享 CI 后续外部追加的护栏已补跑受影响测试。

| 检查 | 日期 / 最终时间 | 墙钟耗时 | 实际结果 |
|---|---|---:|---|
| `go test -count=1 ./backend` | 2026-10-07 00:02:12–00:04:57 | 165.579 秒 | 通过 |
| `go test -count=1 -race ./backend` | 2026-10-07 00:19:46–00:23:15 | 208.887 秒 | 最终通过；首轮耗时断言失败保留 |
| `go vet ./...` | 2026-10-07 00:02:07–00:02:08 | 1.370 秒 | 通过（无输出，exit 0） |
| `node scripts/test-renderers.cjs all` | 2026-10-07 00:14:03–00:15:29 | 86.150 秒 | 1320 通过、4 平台跳过、0 失败；原时间预算通过 |
| `desktop/ 下 node --test` | 2026-10-07 00:16:16–00:17:32 | 76.427 秒 | 293 通过、2 平台跳过、0 失败 |

各项最后五行（不足五行按实际保留）：

`go test -count=1 ./backend`：

```text
ok  	lol-loot-assistant/backend	164.393s
```

`go test -count=1 -race ./backend`：

```text
ok  	lol-loot-assistant/backend	206.980s
```

`go vet ./...`：

实际输出为空，exit code=0。

`node scripts/test-renderers.cjs all`：

```text
    },
    "duration_ms": 86105.842458
  },
  "success": true
}
```

`desktop/ 下 node --test`：

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 76370.698209
```

### 首轮失败、条件检查与未完成项

- 完整 race 首轮 00:06:39–00:11:04（265.707 秒）没有 DATA RACE 告警，但 `TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds` 测得 3.144834334 秒，超过既有 3 秒门槛。未改该业务或测试门槛。随后单独 `go test -count=3 -race -run '^TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds$' ./backend -v` 连续三次通过，每次约 2.88 秒；最终完整 race 在 Node 检查结束后独立复跑通过。首轮失败、三连测及最终日志均保留，不能据此声称此耗时问题已根治。
- 两项完整 Node 检查中 “dirty collection rescans …” 都通过，本次 **未再次失败**；用户指定的“失败后先单独连续运行三次”条件未触发，因而没有运行这三次，不伪造三连测记录。未修改 refresh-orchestration.test.cjs 或 app.js。
- R236 目录全部 **26 个文件**的 SHA-256、大小、mtime_ns、mode 前后完全一致，目录文件集合也一致；安装包仍为 `a33b60ba4434a25f1a77f5f08381d12ff0673e6f208659236677c9f98002b7f5`，见 [r236-artifact-preservation.json](history/reports/r237/r236-artifact-preservation.json) 和 before/after 清单。没有重新构建 STAGING 或 release 候选包，没有写、删或覆盖现有包；新的定稿文案尚未进入用户正在测试的旧包，待用户 S16 通过后另行指示构建。
- **本仓库 P1/P2 已完成**：逐字定稿、测试标签边界、双标签 race/无 SKIP、四项变异与五项最终检查都完成。P3 独立仓库工作不属于本会话，未读取、修改或验证；Windows 真机 S16 由用户继续，本会话未执行真实激活/管理页操作。
- 完整测试结束后，共享 `.github/workflows/ci.yml` 于 00:25:18 被外部追加 R239 Chromium/变异护栏；本会话未改该文件，STAGING 原三项点名筛选逐字未变。本轮运行代码/其它保护文件 SHA 一致；已单独重跑 scripts/verify-ci-test-filters.test.cjs、desktop/release-quality-gates.test.cjs 与 backend/web/r117.test.cjs，均通过。此前 CI SHA 与当前 SHA 分别留存于 source-files.json、shared-ci-current.json；不把外部新增 R239 浏览器/变异任务写成本会话已实跑。
- 版本仍为 0.12.75（package.json 与 lockfile 一致）；未改协议、未创建 tag/Release、未发布。服务端保存期限与无删除渠道政策已由用户确认，不再要求提供联系方式；此前账本的隐私待确认状态由本节覆盖。生产公钥及用户 S16 验收仍待完成，发布仍被阻塞：缺生产公钥。

## R239 授权隐私弹窗显隐与固定 700×470 窗口

执行日期：2026-10-07，时间均为 Asia/Shanghai。已先亲自通读指定 R239 工单与两份项目记忆，按 P1→P2→P3→P4→P5 执行；本会话没有执行 R238 发布工单。版本保持 **0.12.75**；不改授权协议字段/签名域，不改版本、tag、Release、Latest、latest.json，未接触用户私钥或真实注册码。

### P1 隐私弹窗与同类显隐审计

- 移除 `.license-panel` 的通用 `display:grid`，仅 `#license-form, #license-privacy-dialog[open]` 使用 grid。理由：关闭态仍由 Chromium UA 的 dialog 规则隐藏，避免增加全局 dialog 覆盖规则；打开态保留 grid、backdrop、max-height 与内部滚动。`/api/privacy` 仍在点击后请求，再 showModal；失败原文“隐私说明读取失败，请重试”不变。
- 全部静态 dialog、55 个静态 hidden 元素、178 处动态 hidden 写入逐项位置/结论见 [visibility-inventory.json](history/reports/r239/visibility-inventory.json)。静态 hidden 均有 `app.css` 的 `[hidden] { display:none !important; }` 保护；其余作者 `display` 不会覆盖关闭态 dialog。两处更具体的 grid!important（分享图 career-column、mayhem 多杀 dd）没有 hidden 属性控制，不属于同类问题。

| dialog 来源 | 结论 |
|---|---|
| `index.html` license-privacy-dialog / `.license-panel` | 本次修复；关闭时 computed display 为 none，输入框中心 hit-test 命中 input |
| `index.html` update-dialog / `.update-dialog` | 基础样式无 display 覆盖，关闭正常 |
| `index.html` career-dialog / `.career-dialog` | 基础样式无 display 覆盖，[open] 仅动画，关闭正常 |
| `index.html` skin-dialog / `.skin-dialog` | 基础样式无 display 覆盖，关闭正常 |
| `index.html` facade-detail-dialog | flex 仅在 [open] 上，关闭正常 |
| `champions.js` 动态 mayhem/arena tier dialog | `.mayhem-tier-dialog` 基础样式无 display 覆盖，关闭正常 |
| `gameplay.js` / `suite.js` 动态 cs-dialog | 基础样式无 display 覆盖，关闭正常 |

### P2 固定授权窗口与启动时序

- 新 `desktop/license-window.cjs` 不依赖 Electron：所有非 ACTIVE 状态内容区固定 **700×470 DIP**，按所在屏幕 workArea 居中，取消原最小尺寸，禁止最大化/调整大小；没有按分辨率缩放尺寸。
- ACTIVE 恢复有效 `window-bounds.json` 的正常 bounds/位置/最大化标志，无存档用 `initialWindowBounds()`。使用中回锁先保存正常 bounds，重新激活按记忆恢复；锁定/状态待定/切换中的 resize、move、close 都经过同一持久化拒绝入口，小尺寸不写文件。纯模块测试使用实际临时 bounds 文件核对内容不变，覆盖锁定→激活→再锁定及最大化恢复。
- 主窗口先以正常大小 **隐藏创建**，本地授权状态请求与页面加载并发；ready-to-show 等待最新状态后再显示。second-instance / activate 同样不能提前显示主窗口。renderer 初始占位 LOCKED 不发送本机锁定通知；确认状态后发通知，主进程只接受本应用 mainFrame 的 IPC，并再次读带本地 token 的 Go 状态，不信任页面传来的状态值。
- 锁定时原生标题栏和最小尺寸按缩放 1；页面 `.license-panel` zoom 固定 1。原缩放偏好文件不改写，激活后恢复偏好。既有 `ui-scale.test.cjs` 夹具明确以 ACTIVE 开始，原断言及全部变异保留；startup-stage 夹具补齐窗口 API 与等待状态请求，不更改阶段名称/汇总字段。

### P3 700×470 排版

- 420 宽卡片在标题栏下方可用区域居中，56×56 图标、原标题/标签/输入、固定一行提示、全宽激活按钮、同一行左右分布的原“隐私说明”“导出诊断”链接。普通提示用次要色，错误用 danger 色；未新增说明性界面文字。
- 启动输入焦点、真实 Enter 提交、激活中禁用输入与按钮保持；隐私正文内部滚动、底部关闭按钮固定，最大高度 80dvh，窗口控制按钮的顶部 56 DIP 区域不被 backdrop 压暗。
- 实际截图见 [dark-1-locked.png](history/reports/r239/browser/dark-1-locked.png)。复用现有 button/button-primary 样式，避免原 primary-button 空类名显示为浏览器默认灰色。CSS padding 复用已有数值，R117 预算通过，没有放宽断言。

### P4 真实 Chromium、变异与最终验收

- 新 `scripts/r239-browser.cjs` 使用真实 Chrome/CDP，视口 700×470；真实鼠标点击、Input.insertText 和 Enter，不使用 JSDOM。覆盖首屏关闭弹窗、input elementFromPoint、初始焦点、可输入/提交、busy 禁用、深浅主题/人为 2.5 zoom 仍固定 1、页面/遮罩无滚动、长正文与边界、点击关闭/Esc、隐私读取失败原文与不跳动、回锁错误色。输出 [browser/chromium.json](history/reports/r239/browser/chromium.json)，0 exception。
- Linux quality 的既有真实 Chromium 步骤已接入 R239 及其三项变异，找不到 Chrome 直接失败；未仅留本机入口。新真实渲染约 2.5 秒，变异集合约 1.8 秒，未放宽 90 秒/240 秒预算。本地接入文件已核验，GitHub 远端 CI 本会话未触发。
- 三项变异只在临时副本执行并已删除：[mutations/results.json](history/reports/r239/mutations/results.json)。a 恢复通用 display:grid，由首屏 display none 断言失败；b 仅让已确认锁定状态继续写 bounds（待定态仍拒绝），由锁定文件写入断言失败；c 将固定 700×470 改回普通尺寸，由几何断言失败。三项均 exit 1、断言杀死，不是编译/语法/启动失败。
- 既有 R232 真 Chromium 护栏也通过，包含 >15 秒零业务请求、主题/缩放/Tab/Esc、移除遮罩仍不能放行业务、激活及 REPLACED 回锁：[r232-chromium.json](history/reports/r239/r232-chromium.json)。
- 指定五项 **最终源码** 检查全部 exit 0；完整原始日志与开始/结束/最后五行见 [checks-go.json](history/reports/r239/checks-go.json)、[checks-node.json](history/reports/r239/checks-node.json)。Go 成功输出不足五行时保留全部，vet 无输出如实为空。

| 命令 | 开始→结束 | 结果 |
|---|---|---|
| `go test -count=1 ./backend` | 00:37:14→00:40:13 | 通过，backend 166.796 秒 |
| `go test -count=1 -race ./backend` | 00:40:13→00:43:48 | 通过，backend 200.168 秒 |
| `go vet ./...` | 00:43:48→00:43:50 | 通过，无输出 |
| `node scripts/test-renderers.cjs all` | 00:36:09→00:37:42 | 通过；1328 tests / 0 fail / 4 既有 skip，集合 93.065 秒，最慢文件 45.339 秒 |
| `node --test`（desktop 目录） | 00:37:42→00:38:55 | 通过；299 tests / 297 pass / 0 fail / 2 既有 skip，72.860 秒 |

最终日志尾部原样：

`go-test`

```text
ok  	lol-loot-assistant/backend	166.796s
```

`go-race`

```text
ok  	lol-loot-assistant/backend	200.168s
```

`go-vet`

```text
（无输出，exit 0）
```

`node-renderers`

```text
    },
    "duration_ms": 93065.272375
  },
  "success": true
}
```

`desktop-node`

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 72860.085125
```

- “dirty collection rescans …” 在最终 renderer 集合和 desktop 全量均通过，**未触发**单独连续三次复跑的条件。没有修改 `desktop/refresh-orchestration.test.cjs` 或 `backend/web/app.js`；两者 SHA 与本会话开始快照一致，见 [boundary-checks.json](history/reports/r239/boundary-checks.json)。
- 初次验证遇到沙箱 Go 本地监听/缓存访问失败，改用允许本地测试监听的执行环境后实跑成功；初次 CSS budget 和旧缩放夹具失败已通过合并排版样式/明确 ACTIVE 夹具修复，随后完整重跑至全部绿。没有用旧结果代替最终验证，也没有改 dirty collection 业务/测试。

### 真实 Electron 启动对比与边界

- `scripts/r239-startup-probe.cjs` 启动真实 Electron/Chromium 和原生窗口，使用隔离 userData、合成本地状态后端并阻断外部联网。每个状态交错执行前/后主进程各 3 次；采样可见性与内容 bounds。改动前主进程快照配同一最终 renderer/backend fixture，比较窗口策略增加的成本；原始全改动前基线另存 [startup-before.json](history/reports/r239/startup-before.json)。
- 最终交错原始数据 [startup-paired.json](history/reports/r239/startup-paired.json)，摘要 [startup-comparison.json](history/reports/r239/startup-comparison.json)。此前一轮与 race 末段重叠的较慢结果保留为 [startup-paired-with-race-tail.json](history/reports/r239/startup-paired-with-race-tail.json)，未用它宣称空闲性能。

| 状态/中位数 ms | splash 首显 前→后 | backend→主窗可见 前→后 | 总启动 前→后 |
|---|---|---|---|
| ACTIVE | 385→377 | 229→234 | 741→727 |
| LOCKED | 409→445 | 217→217 | 746→745 |

- 首次状态确认耗时为 63/14/16/18/16/16 ms；均发生在主窗首次可见前。总启动中位数未增加；ACTIVE 的 backend→可见 +5 ms、LOCKED 的 splash 首显 +36 ms 等差异原样列出，不承诺所有分段每次绝对相等。首屏采取并发确认而非显示后再缩窗，对已授权用户保持正常尺寸。
- 三次 ACTIVE 最终采样均从正常尺寸隐藏→正常尺寸可见，未出现可见的 700×470；三次 LOCKED 均先在隐藏态改为 700×470，再首次显示，且原生 resizable/maximizable=false。独立纯模块覆盖保存/恢复/max；本机实测是 **macOS 暖启动与合成 ACTIVE/LOCKED 状态**，不是 Windows 冷安装、DPAPI 真缓存或真实租约认证的证明。Windows 真机是否闪烁仍需 S16-00 录屏，未做的不写通过。

### P5 新 public STAGING 包与静态审计

- P1–P4 全部通过后于 **00:47:31** 开始构建，**00:49:36** 完成，约 **125.386 秒**。R237 会话 A 的后端/安装器隐私定稿已在工作区，实际新后端也检出该定稿。仅执行一次 STAGING 安装包构建，key mode **public**、标签 **license_staging**、版本 **0.12.75**、快照指纹 **213c1bf832b3**；更新关闭。
- 新安装包：`dist/R239-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe`（111,429,120 字节）；目录版 `dist/R239-staging-public/Deep-Legends-staging-public/`。附 `SHA256SUMS-staging-public.txt` 和 staging-build.json；[构建最后五行/时刻](history/reports/r239/staging-build-summary.json)、[完整日志](history/reports/r239/staging-build.log)。
- 安装包 SHA-256：`374b496b058effafb29a03c7d8baaaeb9f83d3893efd58d41d58f54c1f9ec40d`。
- 包内后端 SHA-256：`9fcc4b141ab7f8c5e5129e167b59fa70fdff2cc025191009b067141193af5163`。
- ASAR SHA-256：`a274fa8a588b41c4d636bfe062b4999b3209bf3409df0ceac30f6a0a5bece838`。
- 对实际新包只读静态审计通过：[package-audit.json](history/reports/r239/package-audit.json)。授权地址 `https://license-staging.yinxiaobia.net`、kid `staging-2026-10` 与交接公钥准确；已知测试向量/测试信任/私钥 PEM、个人 Riot Key 不在包中；ASAR 无测试/构建夹具、含新窗口模块，固定后端摘要匹配，PE 内 ELECTRONASAR header 摘要匹配。五 fuse：RunAsNode=false、NODE_OPTIONS=false、CLI inspect=false、EmbeddedAsarIntegrity=true、OnlyLoadAppFromAsar=true；Setup 为 PE amd64 GUI。
- 无 license_staging 的实际 public 后端另在临时目录构建，正式检查器接受、STAGING 后端被正式检查器按 staging 原因拒绝；release 后端无 staging 地址/kid/公钥文本及 raw bytes，见审计记录。审计脚本曾有 fuse 常量导出路径错误，修正审计器后对同一产物重跑通过，没有为审计重建 STAGING 包。
- 构建快照 301 个运行源码/配置项与工作区比对，除有意设置 STAGING 的 app-title 外一致：[build-source-comparison.json](history/reports/r239/build-source-comparison.json)。快照与三个变异目录都已由各脚本删除；运行源码在构建后仍匹配快照源 SHA。
- 旧 `dist/R236-staging-public/` 的 **26 文件** SHA-256、大小、mtime_ns、mode 与开始时完全相同：[before](history/reports/r239/r236-artifacts-before.json) / [after](history/reports/r239/r236-artifacts-after.json)。旧包已作废但未删除或覆盖。R236 的历史构建 SHA 保留原样，S16 前置单独更新为新包，并在 S16-01 前加入 S16-00。

### 用户 Windows 真机验收（全部待执行）

1. 两台 Windows 下载/安装本节新包，核对上面的 SHA-256，窗口标题为 STAGING。
2. 打开未授权软件：内容区 700×470 DIP，居中、不可拖大/最大化，注册码输入框可用；隐私说明有正文，点击关闭与 Esc 均可关闭。
3. 输入普通码：成功后恢复正常大小/原位置与主界面；退出后重开，已授权用户直接进入正常大小主界面，无小窗口闪烁。记录显示缩放、尺寸与脱敏录屏；不提供真实完整注册码。
4. 继续上方 S16-01～S16-09 两机/真实服务联调，逐项填写。**本机静态包审计、合成授权渲染与窗口测量不等于 Windows/真实服务 S16 通过。**本会话未运行 Windows 安装器、未测试真实注册码/管理页、未触发远端 CI，也未发布。

## R240 授权窗口切换、首帧、运行中锁定与启动续租

执行日期：2026-10-07；时间均为 Asia/Shanghai。先亲自通读 R240 工单全文及本账本 R239、S16 两节；范围为本仓库 P1–P5 和最后 P7。先完成 P3 诊断及真实 Electron 复现，再修改窗口逻辑。P6 属于另一仓库，本会话未访问或执行；R238 发布工单未执行。版本保持 **0.12.75**，未改授权协议字段、签名域、错误码或共享向量，未创建 tag、发布或读取任何私钥文件，未使用真实注册码。新增授权测试使用该测试新生成的内存密钥。

### P3 先诊断与复现证据

- 首先增加 `license_window_state` 客户端白名单、严格几何结构和实际诊断写入/导出处理，再给原 R239 窗口逻辑加诊断。`TestR240WindowDiagnosticReachesActualExportAndRejectsIdentity` 用实际 Go HTTP 处理器走 POST→磁盘→GET 导出，并拒绝嵌套 code/任意身份值；不是仅写 desktop.log。字段包括 from/to、切换前最大化/全屏、请求/实际内容区、显示缩放、耗时、retry、原生等待/渲染超时、状态/尺寸不一致，不含注册码。
- 修改尺寸/等待逻辑前，真实 macOS Electron 的生产 main.cjs + 合成本地状态接口实跑最大化→REVOKED→ACTIVE。实测最大化 1680×1020→锁定 700×470→恢复最大化；**本机未复现“最大化后被锁不缩小”**，没有把猜测写成复现结论。先诊断的原始记录见 [baseline/result](history/reports/r240/electron/baseline-ACTIVE-1/result.json)、[诊断](history/reports/r240/electron/baseline-ACTIVE-1/diagnostic-export.jsonl)、[首次 show 截图](history/reports/r240/electron/baseline-ACTIVE-1/show-1.png)。
- 同一实跑确实复现首帧错误：Go 为 ACTIVE，但第一次 show 即时 capturePage 的 DOM 为 locked、formVisible=true、frameHidden=true，激活图标区域有 474 个金色像素。

### P1–P4 最终实现与核验

- 授权内容区固定 **860×580 DIP**，居中、不可拖大/最大化，不按屏幕分辨率自适应。卡片 480、图标 64×64、标题 24px、输入 46、按钮 44、提示与链接 13px；保留布局顺序和原文，仅按 P4 改 REPLACED 错误提示。隐私弹窗仍为内部滚动、底部关闭按钮；R117 CSS 预算未放宽。
- 选择 **opacity=0**：保留原生显示/焦点及渲染帧推进，几何修改前先透明，避免 hide 后后台节流妨碍两次 RAF。原生退出全屏/最大化分别等待对应事件，上限 600ms；再设内容尺寸，误差>2 DIP 最多重试一次并记诊断，最后才禁用 resize/maximize。锁定尺寸及切换事件不写 window-bounds.json，恢复原正常位置/尺寸、最大化或全屏。
- HTML 默认 pending，遮罩和主框架均 hidden。主进程用本地 token 读 Go 状态后才推送应用消息；页面应用后两次 requestAnimationFrame 发送仅含 renderId 的时间信号。ACK 仍校验主窗口 sender/mainFrame/origin，并再次读 Go：状态不同重新应用，generation 不同重发。1500ms 上限超时显示并记录；不接受页面状态作为授权依据。
- 原生纯模块测试覆盖普通/max/full→锁定→恢复、unmaximize 不来超时、首次尺寸被系统改回的重试、opacity 在几何前为 0、渲染等待/超时与落盘保护。缩放入口复核：授权切换内最小尺寸变更处于透明态；锁定后的 display/resize 回调仅清除最小尺寸，不改变固定内容区。
- 真实 Electron 最终验收：最大化→锁定 **860×580**、resize/maximize=false→恢复最大化；第一 show 即时 capturePage 和 DOM 双重判断，ACTIVE formVisible=false/金色像素=0，LOCKED formVisible=true/金色像素=642。额外让 Go 在渲染 ACK 前 ACTIVE→LOCKED，首次 show 仍为锁定表单并记录 renderMismatch。见 [acceptance](history/reports/r240/electron/acceptance.json)。
- 已在 Windows CI 增加明确真实 Electron 用例和截图/JSON 上传；Node.exe 合成 backend 子进程无需 cmd/shell，生产 main 与 BrowserWindow API 真实。**已加入 CI，未在远端跑**；本机 macOS 合成状态不等于 Windows、DPAPI、真实租约或两机 S16。
- REPLACED 改为“注册码已在其他设备使用或已被重置”；REVOKED 保持“注册码已停用”，所有运行中的引用测试/快照同步。旧历史账本文字保留历史含义。
- 双续租实证：缓存 ACTIVE 的首个请求跨过 1s tick，旧 Run 在 requests token 被 worker 取走、nextRenew 仍为 0 时又入队，前 5s **2 次**；新 queued 占位一直保留到 maintain 完成，前 5s **1 次**，首请求仍在 500ms 内立即发出。60s heartbeat 未改。见 [before](history/reports/r240/renew-before.log)、[after](history/reports/r240/renew-after.log)，最终两标签也实际运行该 5s 测试。

### P2 启动交错采样

用真实 Electron、新隔离 profile、合成本地 HTTP 状态，前后交错 ACTIVE/LOCKED 各 3 次；故意将 renderer 首次状态响应延迟 500ms 暴露首帧竞态，native token 请求不延迟。第一次 show 的调用栈立即发起 capturePage 与 DOM 采样，不以等待 1s 的截图或仅测尺寸代替。原始 [startup.json](history/reports/r240/electron/startup.json)、[统计](history/reports/r240/startup-comparison.json)。

| 状态 | 前 total ms（三次） | 后 total ms（三次） | 平均前→后 ms | 中位前→后 ms | 首帧内容 |
|---|---|---|---|---|---|
| ACTIVE | 985 / 835 / 883 | 916 / 913 / 832 | 901→887 | 883→913 | 前 3/3 错误激活表单；后 3/3 正确主界面、图标像素 0 |
| LOCKED | 829 / 852 / 844 | 876 / 816 / 750 | 841.67→814 | 844→816 | 后 3/3 首帧激活表单，固定 860×580 |

本机暖启动均值未回退；ACTIVE 中位数增加 30ms，未声称每个样本更快。仅此小样本/合成状态的性能结论；Windows 冷启动和真实缓存仍待 S16。

### P5 四项变异与最终检查

- [真实 Chromium](history/reports/r240/browser/chromium.json) 为 860×580，覆盖 pending 双隐藏、全部 R239 场景、暗/亮主题和 1/2.5 主界面 zoom 下授权 zoom=1、关闭 dialog/input 命中/焦点/Tab、隐私真实点击/Close/Esc/长文内部滚动/失败消息、输入 Enter/disabled/清空、REPLACED 一行、无页面滚动与业务请求；没有用 JSDOM 替代新渲染验收。
- 临时副本/Go overlay 四变异：a 默认遮罩可见、b 不透明直接改尺寸、c 去掉 unmaximize 等待、d 恢复旧 REPLACED 文案，**全部 exit 1，日志命中对应断言，非编译/语法失败**，临时副本结束后删除。最终重跑见 [results](history/reports/r240/mutations/results.json)、[完整入口日志](history/reports/r240/mutations-final.log)。
- 以下均在最终对应源码上实跑。JS 全量涉及的 HTML/CSS/JS/desktop 源码在测试后不变；并行 R241 仅更新 Go/Go 测试及独立 Chromium 脚本，受影响 Go 检查补跑。774 文件 SHA 对照见 [final source](history/reports/r240/final-source-files.json)。

| 命令 | 开始（+08:00） | 结束（+08:00） | 耗时 s | 结果 |
|---|---|---|---|---|
| `go test -count=1 ./backend` | 02:52:22.781609 | 02:55:11.435228 | 168.654 | exit 0 |
| `go test -count=1 -race ./backend` | 02:42:22.984198 | 02:46:02.738533 | 219.756 | exit 0 |
| `go vet ./...` | 02:51:02.091545 | 02:51:03.518020 | 1.426 | exit 0 |
| `go test -count=1 -race -json -run R232|R233|R234|R236|R237|R240 ./backend` | 02:46:04.126984 | 02:46:13.237373 | 9.111 | exit 0；33 顶层 PASS，0 SKIP/FAIL |
| `go test -count=1 -race -json -tags=license_staging -run R232|R233|R234|R236|R237|R240 ./backend` | 02:46:13.242902 | 02:46:45.586360 | 32.344 | exit 0；34 顶层 PASS，0 SKIP/FAIL |
| `node scripts/test-renderers.cjs all` | 02:36:12.669192 | 02:37:43.343304 | 90.675 | exit 0 |
| `node --test` | 02:37:43.344961 | 02:39:03.515882 | 80.172 | exit 0 |

完整时刻与日志： [checks-go](history/reports/r240/checks-go.json)、[checks-node](history/reports/r240/checks-node.json)。实际末五行（不足五行原样保留，vet 无输出）：

`go test -count=1 ./backend`

```text
ok  	lol-loot-assistant/backend	166.986s
```

`go test -count=1 -race ./backend`

```text
ok  	lol-loot-assistant/backend	206.171s
```

`go vet ./...`

```text
（无输出）
```

`go test -count=1 -race -json -run R232|R233|R234|R236|R237|R240 ./backend`

```text
{"Time":"2026-10-07T02:46:11.954077+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Output":"--- PASS: TestR232UpdateSignatureTrust (0.00s)\n"}
{"Time":"2026-10-07T02:46:11.956283+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Elapsed":0}
{"Time":"2026-10-07T02:46:11.956299+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T02:46:12.968959+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t7.176s\n"}
{"Time":"2026-10-07T02:46:12.972311+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":7.18}
```

`go test -count=1 -race -json -tags=license_staging -run R232|R233|R234|R236|R237|R240 ./backend`

```text
{"Time":"2026-10-07T02:46:44.335852+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Output":"--- PASS: TestR236StagingUpdateHTTPGate (0.00s)\n"}
{"Time":"2026-10-07T02:46:44.335879+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Elapsed":0}
{"Time":"2026-10-07T02:46:44.338138+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T02:46:45.350916+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t7.535s\n"}
{"Time":"2026-10-07T02:46:45.352176+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":7.536}
```

`node scripts/test-renderers.cjs all`

```text
    },
    "duration_ms": 90629.620541
  },
  "success": true
}
```

`node --test`

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 80136.46325
```

- 渲染全量 166 文件、1337 项：1333 PASS、4 条平台条件跳过、0 FAIL；总 90.63s，最慢单文件 48.51s，未放宽 90s/240s。desktop 303 项：301 PASS、2 条既有平台条件跳过、0 FAIL。授权专项两标签为严格 0 SKIP/FAIL。
- `dirty collection rescans on entry and view changes without duplicate refresh requests` 在两次最终 JS 全量都 PASS，未触发单跑三次。其文件和 backend/web/app.js、package/lock SHA 与本轮开始相同，见 [保护文件](history/reports/r240/protected-files.json)。
- 中断/失败没有隐去：首轮长进程失去工具会话且进程清单已空，未拿到结束码，不记通过，保留 interrupted 日志；旧夹具 `TestGameplayOverviewOverlapsIndependentUpstreams` 首轮 FAIL，R241 会话在共享区修了其前台匹配（本会话未修改）；下一轮 `TestGameplayOverviewReturnsPartialBeforeTwentySeconds` 清理超时 FAIL，该用例原样单跑 PASS，并行会话结束后 Go 全量原样重跑 PASS。见 [首次汇总](history/reports/r240/checks-go-first.json)、[补跑失败](history/reports/r240/go-test-final-summary.json)、[单跑](history/reports/r240/unrelated-go-focused-summary.json)、[稳定全量](history/reports/r240/go-test-stable-summary.json)。
- vet 最初把 `docs/history/reports/r241/baseline/backend` 的源码摘录当作应用包，报 LCUClient 未定义；只新增 baseline/go.mod 隔离报告快照，不修改摘录、测试或业务，精确 `go vet ./...` 补跑 PASS。外部四文件变更时间/SHA 见 [shared-source-changes](history/reports/r240/shared-source-changes.json)。

### P7 唯一一次 public STAGING 构建与静态审计

P1–P5 全部通过后才执行 `node scripts/build-license-staging.cjs --output dist/R240-staging-public/`；本轮仅一次 STAGING 包构建，key mode **public**、标签 **license_staging**、版本 **0.12.75**，没有发布或 tag。

- 构建 2026-10-07T02:55:57.768772+08:00→2026-10-07T02:57:33.398574+08:00，95.63s，exit 0；[完整日志](history/reports/r240/staging-build.log)、[时刻/末五行](history/reports/r240/staging-build-summary.json)。
- 新包 [`Deep-Legends-Setup-0.12.75-staging-public.exe`](../dist/R240-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe)，111,436,800 字节；目录版 `dist/R240-staging-public/Deep-Legends-staging-public/`。附 [SHA256SUMS](../dist/R240-staging-public/SHA256SUMS-staging-public.txt) 与 [staging-build.json](../dist/R240-staging-public/staging-build.json)。
- Setup SHA-256：`91abee9452d842260d0ee8ab7823d89963b798d6923b7dec2f53e744be37f89e`。
- Backend SHA-256：`a76fbdc25fb2e7e2604ef77961eb513bd719095cc6b44bbef16b5b756576a330`。
- ASAR SHA-256：`965a891646e58d5d51d6e0e70440062ff04418c209e5ddc881635e785ecb3383`。
- 构建指纹：`cb0f1e0c1100`；按构建脚本唯一 STAGING 标题覆盖后重算指纹一致，397 条输入读记录见 [fingerprint-inputs](history/reports/r240/fingerprint-inputs.json)。774 受检源码 SHA 在检查→构建→收尾不变，见 [build-source-comparison](history/reports/r240/build-source-comparison.json)。
- 对实际 Setup/目录版只读审计 PASS：staging 地址/kid/公钥准确、R237 定稿隐私和 R240 pending/CSS/窗口握手模块嵌入、无私钥/共享测试向量/个人 Riot Key、后端摘要固定校验、五项 fuse 和 PE ASAR 完整性一致。另新建临时 **public/default-release** 后端用于验证 staging 信任材料不进入 release，结束删除该临时后端；未构建 release 分发包或读取私钥。见 [package-audit](history/reports/r240/package-audit.json)、[审计时刻](history/reports/r240/package-audit-summary.json)。
- fuse：RunAsNode=false、EnableNodeOptionsEnvironmentVariable=false、EnableNodeCliInspectArguments=false、EnableEmbeddedAsarIntegrityValidation=true、OnlyLoadAppFromAsar=true；PE 中 ASAR header SHA-256 为 `d344150be333f2ced71c2ffe57af3300c91eddfbb1680325e5e2f8366a175acc`。
- 旧 R239、R236 两个目录各 **26 个文件**，文件集合、SHA、size、mtime_ns、mode 前后完全不变，未删除/覆盖。见 [preservation](history/reports/r240/artifact-preservation.json)、[R239 after](history/reports/r240/r239-artifacts-after.json)、[R236 after](history/reports/r240/r236-artifacts-after.json)。R239 包已被本新包取代，不再用于 S16。

构建真实最后五行：

```text
  • file signing skipped via signExecutable configuration  file=/var/folders/0n/pkflntvd76d53gtp1vj_n9gr0000gn/T/deep-legends-STAGING-GkltgI/dist/desktop/Deep Legends Setup 0.12.75-public.__uninstaller.exe
  • file signing skipped via signExecutable configuration  file=/var/folders/0n/pkflntvd76d53gtp1vj_n9gr0000gn/T/deep-legends-STAGING-GkltgI/dist/desktop/Deep Legends Setup 0.12.75-public.exe
packaged desktop runtime verified: app-title.cjs, backend-digest.cjs, backend-evidence.cjs, backend-integrity.cjs, desktop-log.cjs, diagnostics-directory.cjs, diagnostics-export.cjs, license-gate.cjs, license-window.cjs, main.cjs, preload.cjs, process-metrics.cjs, proxy-resolution.cjs, share-export.cjs, ui-scale.cjs, window-bounds-store.cjs, window-bounds.cjs
Installer shell built: Deep Legends Setup 0.12.75-public.exe (349280126 installed bytes)
{"version":"0.12.75","fingerprint":"cb0f1e0c1100","license_build":"STAGING","key_mode":"public","window_title":"Deep Legends-STAGING","license_origin":"https://license-staging.yinxiaobia.net","license_kid":"staging-2026-10","online_updates":"disabled at build time; no production Latest or cache","directory":"/Users/ly/personal/personal-work/deep-legends/dist/R240-staging-public/Deep-Legends-staging-public","setup":"/Users/ly/personal/personal-work/deep-legends/dist/R240-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe","setup_sha256":"91abee9452d842260d0ee8ab7823d89963b798d6923b7dec2f53e744be37f89e","backend_sha256":"a76fbdc25fb2e7e2604ef77961eb513bd719095cc6b44bbef16b5b756576a330","archive_sha256":"965a891646e58d5d51d6e0e70440062ff04418c209e5ddc881635e785ecb3383"}
```

### 交付与未执行

- 本仓库 P1–P5、P7 已完成；S16 前置、S16-00、S16-02/03/05 预期文案、S16-07 本轮不测说明已更新。请使用 R240 新包执行 S16-00：小→大不可见变形、已授权重启无激活卡片、普通/最大化使用中锁定→860×580→重新激活恢复原样；并继续停用/启用/重置。
- **未执行**：P6 管理仓库工作及部署、Windows 远端 CI、Windows 安装/DPAPI/两台真机与真实服务/真实注册码 S16、S16-07 校时。本机未复现 Windows“最大化被锁仍满屏”原现象；已如实保留 macOS 复现结果并加入 Windows 真实窗口护栏。没有把这些写成通过。

## R242 客户端注册码到期与共享向量适配（会话 A）

执行日期：2026-10-07；检查/构建时刻均为 Asia/Shanghai。亲自通读 R242 全文、R240/S16 账本及项目基础文档，只执行会话 A 的 P8、P9。按用户后续提供的绝对路径仅读取管理仓库的共享向量文件，没有访问其他管理仓库内容。版本始终 **0.12.75**；未读取任何私钥文件、`.dev.vars` 或 Secret，未使用真实注册码，未部署、发布或打 tag。新增状态测试仅使用测试生成的内存密钥。

### 共享向量与客户端实现

- 新向量复制前、复制后和构建前 SHA-256 均为 `5f3fee3c89790cffdcb37f651c4333afb18e0f65f7247b1ac5fdda5dc458b89b`，与用户提供的会话 B 值一致。原样替换 `backend/testdata/license-protocol-vectors.json`，共 **68 条**：9 规范化、5 Base64URL、11 严格 JSON、10 请求、33 响应；包括 EXPIRED、业务到期与租约截短，以及到期字段反例。全部向量测试通过，固定 SHA、条数与错误码/拒绝类别覆盖同步更新。
- EXPIRED 沿用 **REVOKED** 本地终态，另存 `terminal_reason=EXPIRED`；立即取消业务、清签名租约。持久化恢复后仍显示“注册码已过期”，不会显示“注册码已停用”；网络恢复/后台维护不发续租请求，续期后需明确重输。成功重输清除终止原因，之后收到普通 REVOKED 仍显示已停用；老缓存没有原因时保持原语义。
- `license_expires_at` 属于原始已签名载荷，只接受正整数且不早于 issued_at；字段出现时拒绝 null、零、负数、字符串、小数、指数写法、溢出及超过业务到期的租约。永久码省略字段。状态接口与原生渲染消息转发该字段，原生同态轮询也更新授权行和终止原因。
- 设置→隐私与能力只新增一行“授权到期：日期（剩 N 天）”，永久为“授权到期：永久”。日期按本机日历、剩余天数向上取整；算法写在代码注释/协议文档，没有新增其他 UI 提示或弹窗。
- 业务到期截短的签名续租可缩短原有 900 秒截止；其他续租仍拒绝截止回退。短租约激活、续租、缓存重启、截止取消业务和网络耗时消耗租期均有测试（40→15 秒缓存恢复、12 秒激活、10 秒租期减 4 秒请求耗时）。没有改 R240 几何、首帧和窗口尺寸逻辑。

### 最终检查与实跑证据

最终源码上 R240 五项完整检查全部通过；双标签 race 专项默认 **38**、staging **39** 个顶层 PASS，均为 **0 SKIP、0 FAIL**。渲染全量 **1339 项：1335 PASS、4 条既有平台条件跳过、0 FAIL**；desktop 全量 **303 项：301 PASS、2 条既有平台条件跳过、0 FAIL**，未放宽测试预算。965 个受检源码文件在最终检查→构建→审计后 SHA 不变。

真实 Chromium 实跑实际 HTML/JS 与 localhost 合成状态，覆盖到期日/剩余天、永久、ACTIVE 续租更新、已过期门禁/联网不放行及原有 R240 隐私/输入/布局护栏；截图和结果在 [browser](history/reports/r242/browser/chromium.json)、[设置页截图](history/reports/r242/browser/settings-expiry.png)。这些不代表 Windows 或真实授权服务联调。

| 命令 | 开始（+08:00） | 结束（+08:00） | 耗时 s | 结果 |
|---|---|---|---|---|
| `go test -count=1 ./backend` | 2026-10-07T12:54:30.077798+08:00 | 2026-10-07T12:57:37.909814+08:00 | 187.832 | exit 0 |
| `go test -count=1 -race ./backend` | 2026-10-07T12:57:52.189573+08:00 | 2026-10-07T13:01:14.362002+08:00 | 202.174 | exit 0 |
| `go vet ./...` | 2026-10-07T12:54:46.250966+08:00 | 2026-10-07T12:54:49.167940+08:00 | 2.917 | exit 0 |
| `go test -count=1 -race -json -run R232\|R233\|R234\|R236\|R237\|R240\|R242 ./backend` | 2026-10-07T12:53:52.444198+08:00 | 2026-10-07T12:54:16.199650+08:00 | 23.755 | exit 0；38 顶层 PASS；0 SKIP/0 FAIL |
| `go test -count=1 -race -json -tags=license_staging -run R232\|R233\|R234\|R236\|R237\|R240\|R242 ./backend` | 2026-10-07T12:54:03.452233+08:00 | 2026-10-07T12:54:25.524523+08:00 | 22.072 | exit 0；39 顶层 PASS；0 SKIP/0 FAIL |
| `node scripts/test-renderers.cjs all` | 2026-10-07T12:54:33.871956+08:00 | 2026-10-07T12:56:09.327185+08:00 | 95.454 | exit 0 |
| `node --test（cwd desktop/）` | 2026-10-07T12:56:24.615185+08:00 | 2026-10-07T12:57:39.477303+08:00 | 74.863 | exit 0 |
| `node scripts/r242-browser.cjs` | 2026-10-07T12:41:40.406155+08:00 | 2026-10-07T12:41:47.438927+08:00 | 7.033 | exit 0 |
| `node scripts/build-license-staging.cjs --output dist/R242-staging-public/` | 2026-10-07T13:01:38.585250+08:00 | 2026-10-07T13:03:20.743281+08:00 | 102.159 | exit 0 |
| `python3 docs/history/reports/r242/audit-package.py` | 2026-10-07T13:04:13.787324+08:00 | 2026-10-07T13:04:18.143745+08:00 | 4.356 | exit 0 |
| `node docs/history/reports/r242/verify-build-inputs.cjs` | 2026-10-07T13:04:14.937839+08:00 | 2026-10-07T13:04:15.050543+08:00 | 0.113 | exit 0 |

完整日志/时刻见 [最终七项检查](history/reports/r242/checks-final.json)。真实末五行（不足五行原样保留）：

`go test -count=1 ./backend`

```text
ok  	lol-loot-assistant/backend	173.913s
```

`go test -count=1 -race ./backend`

```text
ok  	lol-loot-assistant/backend	200.880s
```

`go vet ./...`

```text
（无输出）
```

`go test -count=1 -race -json -run R232|R233|R234|R236|R237|R240|R242 ./backend`

```text
{"Time":"2026-10-07T12:54:15.163952+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Output":"--- PASS: TestR232UpdateSignatureTrust (0.00s)\n"}
{"Time":"2026-10-07T12:54:15.163983+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Elapsed":0}
{"Time":"2026-10-07T12:54:15.16555+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T12:54:16.172155+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t8.201s\n"}
{"Time":"2026-10-07T12:54:16.172203+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":8.202}
```

`go test -count=1 -race -json -tags=license_staging -run R232|R233|R234|R236|R237|R240|R242 ./backend`

```text
{"Time":"2026-10-07T12:54:24.455246+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Output":"--- PASS: TestR236StagingUpdateHTTPGate (0.00s)\n"}
{"Time":"2026-10-07T12:54:24.455261+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Elapsed":0}
{"Time":"2026-10-07T12:54:24.457486+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T12:54:25.471243+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t7.374s\n"}
{"Time":"2026-10-07T12:54:25.471467+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":7.375}
```

`node scripts/test-renderers.cjs all`

```text
    },
    "duration_ms": 95397.089416
  },
  "success": true
}
```

`node --test（cwd desktop/）`

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 74825.230042
```

`node scripts/r242-browser.cjs`

```text
{"started":"2026-10-07T04:41:40.437Z","finished":"2026-10-07T04:41:47.380Z","scope":"Real Chromium client with synthetic local-state API; Windows/deployed-server acceptance pending","checks":["real rendered pending: overlay and frame both invisible","860x580 first frame: closed privacy, input hit/focus, no business requests","real privacy link/Close/Esc, nonempty scrollable body, bounds and caption area","privacy read failure keeps original message and stable one-line layout","real pointer/text/Enter activation, busy disabled, input cleared","R242 settings: dated expiry/remaining days, ACTIVE renewal updates, permanent code","R242 expiry message survives online event without granting admission","replacement warning remains one line without scrolling"],"errors":[]}
```

`node scripts/build-license-staging.cjs --output dist/R242-staging-public/`

```text
  • file signing skipped via signExecutable configuration  file=/var/folders/0n/pkflntvd76d53gtp1vj_n9gr0000gn/T/deep-legends-STAGING-Da4fR2/dist/desktop/Deep Legends Setup 0.12.75-public.__uninstaller.exe
  • file signing skipped via signExecutable configuration  file=/var/folders/0n/pkflntvd76d53gtp1vj_n9gr0000gn/T/deep-legends-STAGING-Da4fR2/dist/desktop/Deep Legends Setup 0.12.75-public.exe
packaged desktop runtime verified: app-title.cjs, backend-digest.cjs, backend-evidence.cjs, backend-integrity.cjs, desktop-log.cjs, diagnostics-directory.cjs, diagnostics-export.cjs, license-gate.cjs, license-window.cjs, main.cjs, preload.cjs, process-metrics.cjs, proxy-resolution.cjs, share-export.cjs, ui-scale.cjs, window-bounds-store.cjs, window-bounds.cjs
Installer shell built: Deep Legends Setup 0.12.75-public.exe (349282230 installed bytes)
{"version":"0.12.75","fingerprint":"64cfb53a8b5a","license_build":"STAGING","key_mode":"public","window_title":"Deep Legends-STAGING","license_origin":"https://license-staging.yinxiaobia.net","license_kid":"staging-2026-10","online_updates":"disabled at build time; no production Latest or cache","directory":"/Users/ly/personal/personal-work/deep-legends/dist/R242-staging-public/Deep-Legends-staging-public","setup":"/Users/ly/personal/personal-work/deep-legends/dist/R242-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe","setup_sha256":"0d78d3318012b8b632ce15350f36418bc91c4f70e8366e1a13973301b472c3f8","backend_sha256":"923ec82b6fcceea7c780f486d440b701653ff81ff18aee68cf57b4584a430910","archive_sha256":"269c989d28014093feaeb14e47f63105c251ec2fcba428ee9e0f09420764080e"}
```

`python3 docs/history/reports/r242/audit-package.py`

```text
{"version":"0.12.75","fingerprint":"64cfb53a8b5a","license_build":"STAGING","key_mode":"public","window_title":"Deep Legends-STAGING","license_origin":"https://license-staging.yinxiaobia.net","license_kid":"staging-2026-10","online_updates":"disabled at build time; no production Latest or cache","directory":"/Users/ly/personal/personal-work/deep-legends/dist/R242-staging-public/Deep-Legends-staging-public","setup":"/Users/ly/personal/personal-work/deep-legends/dist/R242-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe","setup_sha256":"0d78d3318012b8b632ce15350f36418bc91c4f70e8366e1a13973301b472c3f8","backend_sha256":"923ec82b6fcceea7c780f486d440b701653ff81ff18aee68cf57b4584a430910","archive_sha256":"269c989d28014093feaeb14e47f63105c251ec2fcba428ee9e0f09420764080e","scope":"Read-only macOS static audit of actual Windows public artifacts; Windows execution pending","packaged_version":"0.12.75","setup_bytes":111440384,"backend_bytes":32523264,"license_origin_and_kid_and_key_match":true,"finalized_privacy_embedded":true,"r240_css_and_window_module_embedded":true,"r242_protocol_and_settings_embedded":true,"test_fixtures_and_known_private_material_absent":true,"personal_riot_key_absent":true,"fixed_backend_digest_matches":true,"fuses":{"RunAsNode":false,"EnableNodeOptionsEnvironmentVariable":false,"EnableNodeCliInspectArguments":false,"EnableEmbeddedAsarIntegrityValidation":true,"OnlyLoadAppFromAsar":true},"asar_integrity":[{"file":"resources\\app.asar","alg":"SHA256","value":"b3afc0a24f19aa67f6afdb0055832508d41e5cec164684edca5f28d21648bd42"}],"staging_rejected_by_release_checker":true,"release_staging_material_absent":true,"release_backend_sha256":"d2fe8b649c6be0ab4ef5381727e216561afd2d1940b0fc53342f983f183649c7"}
```

`node docs/history/reports/r242/verify-build-inputs.cjs`

```text
{"fingerprint":"64cfb53a8b5a","inputs":397,"matched":true}
```

### 唯一一次 public STAGING 构建与只读审计

七项最终检查通过并核对源码未变后，仅执行一次 `node scripts/build-license-staging.cjs --output dist/R242-staging-public/`。key mode **public**、构建标签 **license_staging**、版本 **0.12.75**、标题 **Deep Legends-STAGING**、压缩级别 **9**、`--publish=never`。结果和真实末五行已记录在上表及 [构建记录](history/reports/r242/staging-build-summary.json)。

- 新安装包 [Deep-Legends-Setup-0.12.75-staging-public.exe](../dist/R242-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe)，111440384 字节；目录版 `dist/R242-staging-public/Deep-Legends-staging-public/`；附 [SHA256SUMS](../dist/R242-staging-public/SHA256SUMS-staging-public.txt)、[staging-build.json](../dist/R242-staging-public/staging-build.json)。
- Setup SHA-256：`0d78d3318012b8b632ce15350f36418bc91c4f70e8366e1a13973301b472c3f8`。
- Backend SHA-256：`923ec82b6fcceea7c780f486d440b701653ff81ff18aee68cf57b4584a430910`。
- ASAR SHA-256：`269c989d28014093feaeb14e47f63105c251ec2fcba428ee9e0f09420764080e`。
- 构建指纹：`64cfb53a8b5a`；按唯一 STAGING 标题覆盖重算与实际包一致，见 [fingerprint-inputs](history/reports/r242/fingerprint-inputs.json)。
- 对实际 Setup/目录版只读审计 PASS：origin/kid/用户 staging 公钥准确，R237 隐私、R240 pending/窗口握手、R242 EXPIRED/终止原因/到期字段/设置行/原生转发均嵌入；无私钥、共享测试向量、测试信任材料、个人 Riot Key，固定 backend digest、五项 fuse 和 PE ASAR 完整性一致。为信任隔离审计只在临时目录构建 public/default-release 后端，完成删除，未构建 release 分发包。见 [package-audit](history/reports/r242/package-audit.json)。
- R240、R239、R236 旧目录各 **26 文件**，文件集合、SHA-256、size、mtime_ns、mode 前后完全不变；没有删除、覆盖或重建。见 [before](history/reports/r242/artifacts-before.json)、[after](history/reports/r242/artifacts-after.json)、[preservation](history/reports/r242/artifact-preservation.json)。R240 与更旧包不能处理新协议字段/错误码，不再用于 S16。

### 非最终失败与未执行边界

- 首次 Go 专项因系统默认缓存写入受沙箱限制而 setup failed；获准使用系统缓存后双标签专项及 Go 全量/race/vet 均实际重跑通过。两次自动审批超时没有执行命令，之后重试成功，未写为通过。首次 Chrome 沙箱启动超时；后续合成 status 缺少 process-not-found 和过期文案断言未等异步渲染造成护栏失败，仅修 R242 浏览器夹具和等待断言，最终真实 Chromium 通过。失败日志保留在本报告目录，未修改业务加载遮罩以迎合测试。
- S16 已更新为新包，并增加 S16-10、S16-11、S16-12；S16-08 同步新版管理页“随时可见”预期。**未在 Windows 实跑**：Windows 安装/启动、DPAPI、真实注册码/真实服务、两机、自定义管理员码修改与管理页联调均由用户执行，未把本机合成测试/静态审计记为这些项目通过。S16-07 校时仍按 R240 留待以后。
- 会话 A 的 P8/P9 已完成；会话 B 的服务端、管理页、部署与验收不属于本次执行。未触发远端 CI、未发布、未打 tag，生产公钥仍未提供，不把 STAGING public 包当正式发布包。

新安装包 SHA-256：`0d78d3318012b8b632ce15350f36418bc91c4f70e8366e1a13973301b472c3f8`。

## R243 客户端失败诊断、激活一次重试与启动窗口恢复（P4–P6）
执行日期：2026-10-07；检查/构建时刻均为 Asia/Shanghai。已亲自通读 R243 全文及项目基础文档。仅执行 deep-legends 的 P4–P6，**未访问 deep-legends-manage 仓库**。版本始终 **0.12.75**；未读私钥文件、.dev.vars 或 Secret，未用真实注册码，未部署、发布、推送或打 tag。测试密钥只在本次测试内生成，授权缓存为合成签名租约、内存测试存储，Electron 的 userData/window-bounds 为临时目录。

### 窗口复现、确定事实与未查清边界

- 先用真实 macOS Electron + 生产 main/BrowserWindow 复现；最初三个健康基础场景（无保存/普通保存/最大化保存）都能恢复。之后只读提取 **R242 实际包 ASAR** 中的运行模块，接入真实 Go licenseManager：合成有效签名缓存过期 901 秒，首次续租裸 503，后台按原 15 秒退避第二次签名成功；没有把状态接口直接改成 ACTIVE 来冒充续租。
- R242 旧源码的自然恢复：保存 1200×780 为 47 ms，无保存记录为 39 ms，均正常。**故障注入**一次原生 setBounds 被忽略后，真实 Electron 出现主界面已 ACTIVE、实际内容区仍 **860×580**，请求目标仍为 **1200×780**，耗时 **33 ms**；截图和原始/实际 Go 导出见 [baseline](history/reports/r243/electron/baseline.json)、[小窗截图](history/reports/r243/electron/baseline-NETWORK_LOCKED-3/restored.png)。这是确定的软件缺口复现，不能代表 Windows 原事故的触发条件。
- 确定根因缺口：旧 ACTIVE 分支只发一次 setBounds，随后只复核 LOCKED 尺寸，原生操作被忽略也继续渲染/显示 ACTIVE；sizeMismatch 甚至只对 LOCKED 计算。原 restore 缺坐标时也没有先补齐真实原生矩形。现补齐 x/y，提交纯原生 Rectangle；ACTIVE 前后检查实际内容区 ≥780×600，先重设保存记录，再失败用 initialWindowBounds()，记录 fallback_used；锁定与切换过程仍不持久化小窗。
- 旧 requestedContent 用锁定窗口的 outer/content 差值反推 ACTIVE 目标，可能生成 NaN/非正值，但 **ACTIVE 真正设置的是 restore，不是 requestedContent**，不能把诊断被拒当成“目标尺寸无效导致窗口没恢复”的证明。现 ACTIVE requestedContent 取恢复/尺寸复核后的实际内容区，去掉与原生设置脱节的锁定态 inset 算术；尺寸保护是实际几何修复，不是放宽后端宽高校验。
- **未查清原 Windows 05:42 事故的确切无效字段和约 3.5 秒来源**：仓库只有工单转述，没有 1402/1403 原始日志/desktop.log。原后端还校验状态、坐标、displayScale、elapsedMs，并非只校验宽高；事件到达相差 3.5 秒也不等于控制器 elapsedMs（还有 2 秒前端轮询、状态 HTTP、队列/两帧握手/诊断发送）。本机自然路径与注入路径都没有重现 3.5 秒，不能宣称“背景节流”就是原事故根因。新增 status_read_ms/native_ms/render_ms/elapsed_ms，主窗 backgroundThrottling=false 保证透明/后台两帧握手继续；该设置是保护，非原事故已证实原因。**P5 原事故精确归因仍待原始日志或 Windows 复测，未关闭这一验收点。**

### P4/P5 客户端行为

- 激活/续租失败事件仍使用 license_renew，新增 kind、failure（timeout/dns/connect/tls/http_status/unsigned_error/bad_signature/parse）、http_status、server_error、elapsed_ms、cf_ray。server_error 只接受既有协议九个错误码，其余为 other；cf-ray 只接受 hex ID/三字母 colo。没有记录错误原文、响应体、注册码、完整设备摘要、私钥或签名。真实 diagnostics 导出测试覆盖。
- 激活对超时、DNS/连接、5xx、未签名错误及已验签 SERVICE_UNAVAILABLE 只自动重试一次。每次重新保存递增 counter、生成 request_id 和签名；20 秒总 context 覆盖等待操作锁与两次请求，单次仍 8 秒。明确业务错误不重试；保留原协议已规定的 TIMESTAMP_INVALID 一次校时重试。续租 15/30/60 秒退避未变。
- 前端激活等待为 21 秒（覆盖后端 20 秒预算及本地返回），输入/按钮持续禁用，按钮与提示“正在激活…”不中途被原生锁定通知覆盖，最终中文错误消息不变。
- 非法窗口诊断仍返回 400，额外写 license_window_state_invalid，只有固定 schema 字段名和原始 JSON 数值类型（number/string/null/missing 等），不写原始值或未知字段内容。保留原 schema 可省略坐标/elapsedMs 的兼容规则，宽高仍必须有效正整数。fallback_used 和分段耗时进入有效诊断导出。
- 协议字段、签名域、错误码和共享向量均未改；向量 SHA-256 仍为 `5f3fee3c89790cffdcb37f651c4333afb18e0f65f7247b1ac5fdda5dc458b89b`。

### 最终实跑检查
| 命令 | 开始（+08:00） | 结束（+08:00） | 耗时 s | 结果 |
|---|---|---|---|---|
| `go test -count=1 ./backend`（cwd .） | 2026-10-07T14:49:14.868974+08:00 | 2026-10-07T14:52:13.566250+08:00 | 178.699 | exit 0 |
| `go test -count=1 -race ./backend`（cwd .） | 2026-10-07T14:49:15.999023+08:00 | 2026-10-07T14:53:05.268910+08:00 | 229.272 | exit 0 |
| `go vet ./...`（cwd .） | 2026-10-07T14:51:19.981040+08:00 | 2026-10-07T14:51:21.211914+08:00 | 1.231 | exit 0 |
| `go test -count=1 -race -json -run R232\|R233\|R234\|R236\|R237\|R240\|R242\|R243 ./backend`（cwd .） | 2026-10-07T14:52:52.424553+08:00 | 2026-10-07T14:53:09.821789+08:00 | 17.397 | exit 0；45 顶层 PASS；0 SKIP/0 FAIL |
| `go test -count=1 -race -json -tags=license_staging -run R232\|R233\|R234\|R236\|R237\|R240\|R242\|R243 ./backend`（cwd .） | 2026-10-07T14:52:55.494111+08:00 | 2026-10-07T14:53:25.191346+08:00 | 29.697 | exit 0；46 顶层 PASS；0 SKIP/0 FAIL |
| `node scripts/test-renderers.cjs all`（cwd .） | 2026-10-07T14:45:21.360939+08:00 | 2026-10-07T14:46:43.194467+08:00 | 81.834 | exit 0 |
| `node --test`（cwd desktop） | 2026-10-07T14:46:43.237385+08:00 | 2026-10-07T14:47:55.090908+08:00 | 71.854 | exit 0 |
| `env R240_BROWSER_OUTPUT=docs/history/reports/r243/chromium node scripts/r240-browser.cjs`（cwd .） | 2026-10-07T14:47:55.134049+08:00 | 2026-10-07T14:47:57.860282+08:00 | 2.726 | exit 0 |
| `env R240_ELECTRON_OUTPUT=docs/history/reports/r243/r240-electron node scripts/r240-electron.cjs acceptance`（cwd .） | 2026-10-07T14:47:57.892723+08:00 | 2026-10-07T14:48:08.948949+08:00 | 11.056 | exit 0 |
| `node scripts/r243-electron.cjs after`（cwd .） | 2026-10-07T14:48:08.990546+08:00 | 2026-10-07T14:50:12.068918+08:00 | 123.079 | exit 0 |
| `node docs/history/reports/r243/run-activation-electron.cjs`（cwd .） | 2026-10-07T14:53:28.663082+08:00 | 2026-10-07T14:53:41.462098+08:00 | 12.799 | exit 0 |
| `node scripts/r243-mutations.cjs`（cwd .） | 2026-10-07T14:50:12.121143+08:00 | 2026-10-07T14:50:48.697275+08:00 | 36.576 | exit 0 |
| `node scripts/build-license-staging.cjs --output dist/R243-staging-public/`（cwd .） | 2026-10-07T14:55:21.824792+08:00 | 2026-10-07T14:56:55.110699+08:00 | 93.287 | exit 0 |
| `python3 docs/history/reports/r243/audit-package.py`（cwd .） | 2026-10-07T14:58:11.715390+08:00 | 2026-10-07T14:58:15.900580+08:00 | 4.185 | exit 0 |
| `node docs/history/reports/r243/verify-build-inputs.cjs`（cwd .） | 2026-10-07T14:58:08.835738+08:00 | 2026-10-07T14:58:08.945012+08:00 | 0.109 | exit 0 |

开始/结束、完整日志与真实末五行见 [checks-final](history/reports/r243/checks-final.json)。Go 五项最终通过后仅对 desktop/license-window.cjs 去掉一次 ACTIVE 不必要的工作区读取（修复关窗后的异步 null 引用），Go backend 编译输入未变；受影响 JS 全量/Chromium/Electron/变异均重跑，见 [affected-check-reconciliation](history/reports/r243/affected-check-reconciliation.json)。补充真实 Electron 激活用例时仅更新临时 issuer 测试夹具/代理脚本，随后 Go 全量/race/vet/双标签专项全部重跑；激活按钮/提示/输入 1 秒时仍保持 busy，真实首次 timeout 后 activate_requests=2、activation_fresh_signed_request=true，最终恢复 1200×780。第一次补充夹具漏转发 /api/license/activate 的断言失败已保留并修正代理，非运行代码问题。各项末五行（不足五行原样保留）：

`go-full`

```text
ok  	lol-loot-assistant/backend	177.494s
```

`go-race`

```text
ok  	lol-loot-assistant/backend	215.530s
```

`go-vet`

```text
（无输出）
```

`license-default`

```text
{"Time":"2026-10-07T14:53:08.776651+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Output":"--- PASS: TestR232UpdateSignatureTrust (0.00s)\n"}
{"Time":"2026-10-07T14:53:08.776692+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Elapsed":0}
{"Time":"2026-10-07T14:53:08.778511+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T14:53:09.794774+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t15.487s\n"}
{"Time":"2026-10-07T14:53:09.794888+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":15.487}
```

`license-staging`

```text
{"Time":"2026-10-07T14:53:24.026449+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Output":"--- PASS: TestR236StagingUpdateHTTPGate (0.00s)\n"}
{"Time":"2026-10-07T14:53:24.026461+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Elapsed":0}
{"Time":"2026-10-07T14:53:24.028998+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T14:53:25.047066+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t15.633s\n"}
{"Time":"2026-10-07T14:53:25.047198+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":15.634}
```

`renderers`

```text
    },
    "duration_ms": 81794.16925
  },
  "success": true
}
```

`desktop`

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 71820.950708
```

`chromium`

```text
{"started":"2026-10-07T06:47:55.165Z","finished":"2026-10-07T06:47:57.816Z","scope":"Real Chromium client with synthetic local-state API; Windows/deployed-server acceptance pending","checks":["real rendered pending: overlay and frame both invisible","860x580 first frame: closed privacy, input hit/focus, no business requests","real privacy link/Close/Esc, nonempty scrollable body, bounds and caption area","privacy read failure keeps original message and stable one-line layout","real pointer/text/Enter activation, busy disabled, input cleared","replacement warning remains one line without scrolling"],"errors":[]}
```

`electron-r240`

```text
[{"phase":"after","state":"ACTIVE","shows":[{"number":1,"bounds":{"x":67,"y":46,"width":1546,"height":959},"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":741,"y":360.5,"width":64,"height":64}},"activationIconGoldPixels":0,"captureInvokedAtShow":true}],"maximizedBefore":true,"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"maximized":false,"resizable":false,"maximizable":false,"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":0,"y":30,"width":1680,"height":1020},"maximized":true,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":808,"y":391,"width":64,"height":64}}},"phases":{"processToJs":1900,"jsToReady":60,"readyToSplash":87,"splashPaint":126,"splashWindowShown":2171,"spawnToReady":30,"readyToWindow":268,"total":2471}},{"phase":"after","state":"LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"phases":{"processToJs":115,"jsToReady":48,"readyToSplash":79,"splashPaint":94,"splashWindowShown":335,"spawnToReady":31,"readyToWindow":255,"total":622}},{"phase":"after","state":"ACTIVE","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"phases":{"processToJs":115,"jsToReady":48,"readyToSplash":78,"splashPaint":95,"splashWindowShown":335,"spawnToReady":31,"readyToWindow":269,"total":636}}]
```

`electron-final`

```text
[{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":90,"y":60,"width":1200,"height":780},"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}},"activationIconGoldPixels":0,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":90,"y":60,"width":1200,"height":780},"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}}},"restored":{"bounds":{"x":90,"y":60,"width":1200,"height":780},"normal":{"x":90,"y":60,"width":1200,"height":780},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}}},"phases":{"processToJs":126,"jsToReady":49,"readyToSplash":79,"splashPaint":94,"splashWindowShown":347,"spawnToReady":30,"readyToWindow":258,"total":636}},{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":90,"y":60,"width":1200,"height":780},"normal":{"x":90,"y":60,"width":1200,"height":780},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}}},"phases":{"processToJs":118,"jsToReady":49,"readyToSplash":79,"splashPaint":95,"splashWindowShown":339,"spawnToReady":31,"readyToWindow":257,"total":629}},{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":0,"y":30,"width":1680,"height":1020},"normal":{"x":90,"y":60,"width":1200,"height":780},"maximized":true,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":808,"y":391,"width":64,"height":64}}},"phases":{"processToJs":118,"jsToReady":47,"readyToSplash":79,"splashPaint":94,"splashWindowShown":337,"spawnToReady":31,"readyToWindow":143,"total":512}},{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":67,"y":46,"width":1546,"height":959},"normal":{"x":67,"y":46,"width":1546,"height":959},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":741,"y":360.5,"width":64,"height":64}}},"phases":{"processToJs":118,"jsToReady":47,"readyToSplash":78,"splashPaint":92,"splashWindowShown":334,"spawnToReady":30,"readyToWindow":140,"total":506}},{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":90,"y":60,"width":1200,"height":780},"normal":{"x":90,"y":60,"width":1200,"height":780},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}}},"phases":{"processToJs":151,"jsToReady":51,"readyToSplash":90,"splashPaint":104,"splashWindowShown":394,"spawnToReady":33,"readyToWindow":292,"total":721}},{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":410,"y":250,"width":1546,"height":959},"normal":{"x":410,"y":250,"width":1546,"height":959},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":741,"y":360.5,"width":64,"height":64}}},"phases":{"processToJs":117,"jsToReady":53,"readyToSplash":81,"splashPaint":98,"splashWindowShown":348,"spawnToReady":32,"readyToWindow":273,"total":654}},{"phase":"after","state":"NETWORK_LOCKED","shows":[{"number":1,"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}},"activationIconGoldPixels":642,"captureInvokedAtShow":true}],"locked":{"bounds":{"x":410,"y":250,"width":860,"height":580},"content":{"state":"locked","overlayHidden":false,"frameHidden":true,"formVisible":true,"icon":{"x":398,"y":171,"width":64,"height":64}}},"restored":{"bounds":{"x":90,"y":60,"width":1200,"height":780},"normal":{"x":90,"y":60,"width":1200,"height":780},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}}},"phases":{"processToJs":122,"jsToReady":53,"readyToSplash":83,"splashPaint":97,"splashWindowShown":354,"spawnToReady":31,"readyToWindow":259,"total":645}}]
```

`electron-activation`

```text
{"activation":{"button":"正在激活…","disabled":true,"message":"正在激活…","inputDisabled":true},"evidence":{"activate_requests":2,"activation_fresh_signed_request":true,"expired_cache":true,"renew_requests":1,"state":"ACTIVE"},"restored":{"bounds":{"x":90,"y":60,"width":1200,"height":780},"normal":{"x":90,"y":60,"width":1200,"height":780},"maximized":false,"content":{"state":"active","overlayHidden":true,"frameHidden":false,"formVisible":false,"icon":{"x":568,"y":271,"width":64,"height":64}}}}
```

`mutations`

```text
[{"name":"a-active-size-guard-module","exit_code":1,"assertion_killed":true,"started":"2026-10-07T06:50:12.182Z","finished":"2026-10-07T06:50:12.275Z","last5":["    actual: false,","    expected: true,","    operator: '==',","    diff: 'simple'","  }"]},{"name":"a-active-size-guard-electron","exit_code":1,"assertion_killed":true,"started":"2026-10-07T06:50:12.275Z","finished":"2026-10-07T06:50:32.835Z","last5":["  actual: false,","  expected: true,","  operator: '==',","  diff: 'simple'","}"]},{"name":"b-no-activation-retry","exit_code":1,"assertion_killed":true,"started":"2026-10-07T06:50:32.836Z","finished":"2026-10-07T06:50:48.686Z","last5":["    --- FAIL: TestR243ActivationRetriesFreshSignedRequest/timeout (8.00s)","        license_r243_test.go:72: 软件尚未激活","FAIL","FAIL\tlol-loot-assistant/backend\t8.650s","FAIL"]}]
```

`build`

```text
  • file signing skipped via signExecutable configuration  file=/var/folders/0n/pkflntvd76d53gtp1vj_n9gr0000gn/T/deep-legends-STAGING-jKsNf2/dist/desktop/Deep Legends Setup 0.12.75-public.__uninstaller.exe
  • file signing skipped via signExecutable configuration  file=/var/folders/0n/pkflntvd76d53gtp1vj_n9gr0000gn/T/deep-legends-STAGING-jKsNf2/dist/desktop/Deep Legends Setup 0.12.75-public.exe
packaged desktop runtime verified: app-title.cjs, backend-digest.cjs, backend-evidence.cjs, backend-integrity.cjs, desktop-log.cjs, diagnostics-directory.cjs, diagnostics-export.cjs, license-gate.cjs, license-window.cjs, main.cjs, preload.cjs, process-metrics.cjs, proxy-resolution.cjs, share-export.cjs, ui-scale.cjs, window-bounds-store.cjs, window-bounds.cjs
Installer shell built: Deep Legends Setup 0.12.75-public.exe (349304598 installed bytes)
{"version":"0.12.75","fingerprint":"d674d2584eeb","license_build":"STAGING","key_mode":"public","window_title":"Deep Legends-STAGING","license_origin":"https://license-staging.yinxiaobia.net","license_kid":"staging-2026-10","online_updates":"disabled at build time; no production Latest or cache","directory":"/Users/ly/personal/personal-work/deep-legends/dist/R243-staging-public/Deep-Legends-staging-public","setup":"/Users/ly/personal/personal-work/deep-legends/dist/R243-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe","setup_sha256":"603572946e55911a544284843182dc783e42bca27a96a8166e07285f0838719d","backend_sha256":"f9cda352673974a3367967c5e2b5c1d743f4a6b4fa01e09114368e952b50c090","archive_sha256":"4ca7fe51e96854df071682eb2745fe197688f13c9c5a3579edddd925bb244679"}
```

`audit`

```text
{"version":"0.12.75","fingerprint":"d674d2584eeb","license_build":"STAGING","key_mode":"public","window_title":"Deep Legends-STAGING","license_origin":"https://license-staging.yinxiaobia.net","license_kid":"staging-2026-10","online_updates":"disabled at build time; no production Latest or cache","directory":"/Users/ly/personal/personal-work/deep-legends/dist/R243-staging-public/Deep-Legends-staging-public","setup":"/Users/ly/personal/personal-work/deep-legends/dist/R243-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe","setup_sha256":"603572946e55911a544284843182dc783e42bca27a96a8166e07285f0838719d","backend_sha256":"f9cda352673974a3367967c5e2b5c1d743f4a6b4fa01e09114368e952b50c090","archive_sha256":"4ca7fe51e96854df071682eb2745fe197688f13c9c5a3579edddd925bb244679","scope":"Read-only macOS static audit of actual Windows public artifacts; Windows execution pending","packaged_version":"0.12.75","setup_bytes":111446016,"backend_bytes":32543744,"license_origin_and_kid_and_key_match":true,"finalized_privacy_embedded":true,"r240_css_and_window_module_embedded":true,"r242_protocol_and_settings_embedded":true,"r243_failure_retry_and_window_guard_embedded":true,"test_fixtures_and_known_private_material_absent":true,"personal_riot_key_absent":true,"fixed_backend_digest_matches":true,"fuses":{"RunAsNode":false,"EnableNodeOptionsEnvironmentVariable":false,"EnableNodeCliInspectArguments":false,"EnableEmbeddedAsarIntegrityValidation":true,"OnlyLoadAppFromAsar":true},"asar_integrity":[{"file":"resources\\app.asar","alg":"SHA256","value":"828c36c0e59ba231b15ec5a009e5e8256853fa25ebf483a1b0f21b38bb11a19d"}],"staging_rejected_by_release_checker":true,"release_staging_material_absent":true,"release_backend_sha256":"126e46f8e24dc986b46b55effcb280dda7cd1e70be3dad6dfb3a65444488700a"}
```

`fingerprint`

```text
{"fingerprint":"d674d2584eeb","inputs":398,"matched":true}
```

真实 Electron 最终八场景：立即续租成功、首次失败自动恢复、最大化保存、无保存记录、一次/两次原生恢复被忽略、最小化后台恢复，以及实际输入临时码的 8 秒超时→新签名第二次成功。全部 PASS，首帧立即 capturePage/DOM，故障回退有 fallback_used；实际 Go 诊断导出严格校验通过。普通保存恢复 1200×780 与位置 90/60，最大化恢复系统工作区，无保存与最终兜底用 initialWindowBounds。

```json
[{"index": 2, "elapsed_ms": 46, "native_ms": 4, "render_ms": 42, "status_read_ms": 6, "fallback_used": false}, {"index": 3, "elapsed_ms": 395, "native_ms": 362, "render_ms": 33, "status_read_ms": 4, "fallback_used": false}, {"index": 4, "elapsed_ms": 49, "native_ms": 5, "render_ms": 44, "status_read_ms": 4, "fallback_used": false}, {"index": 5, "elapsed_ms": 64, "native_ms": 23, "render_ms": 41, "status_read_ms": 4, "fallback_used": true}, {"index": 6, "elapsed_ms": 49, "native_ms": 7, "render_ms": 42, "status_read_ms": 4, "fallback_used": true}, {"index": 7, "elapsed_ms": 34, "native_ms": 2, "render_ms": 32, "status_read_ms": 2, "fallback_used": false}, {"index": 8, "elapsed_ms": 42, "native_ms": 4, "render_ms": 38, "status_read_ms": 6, "fallback_used": false}]
```
覆盖升级为合成数据兼容模拟：临时保存同一 window-bounds 与同一签名租约缓存经两个 manager 加载，R242 旧包运行模块/最终模块分别启动；**未在 Windows 实跑** R240→R242/R243 安装器覆盖、物理 DPAPI 文件。

非最终并行批次中 dirty collection rescans 失败后，按 R240 要求单独连续三次实跑均 PASS（2.070/2.105/2.082 秒），没有修改该测试或 app.js 非授权部分。

变异 a（移除 ACTIVE 最小尺寸保护）同时由纯模块及真实 Electron 断言杀死；变异 b（移除激活重试）首次真实 8 秒超时后由 Go 断言杀死；三条结果均 exit 1，非编译/语法错误，临时副本/overlay 已删除。见 [mutations](history/reports/r243/mutations/results.json)。

### 唯一一次 STAGING public 构建与只读审计

全部最终检查完成后仅执行一次 `node scripts/build-license-staging.cjs --output dist/R243-staging-public/`。版本 **0.12.75**，key mode **public**，标签 **license_staging**，标题 **Deep Legends-STAGING**，压缩级别 **9**，--publish=never；没有读私钥或正式授权数据。

- 新安装包 [Deep-Legends-Setup-0.12.75-staging-public.exe](../dist/R243-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe)，111446016 字节；附 [SHA256SUMS](../dist/R243-staging-public/SHA256SUMS-staging-public.txt)、[staging-build.json](../dist/R243-staging-public/staging-build.json)。
- Setup SHA-256：`603572946e55911a544284843182dc783e42bca27a96a8166e07285f0838719d`。Backend：`f9cda352673974a3367967c5e2b5c1d743f4a6b4fa01e09114368e952b50c090`。ASAR：`4ca7fe51e96854df071682eb2745fe197688f13c9c5a3579edddd925bb244679`。构建指纹：`d674d2584eeb`。
- 实际 Windows Setup/目录版在 macOS 只读静态审计 PASS：staging origin/kid/公钥、版本/标题、R243 失败/重试/UI/窗口保护标记、固定 backend digest、五项 fuse、PE ASAR 完整性；无私钥、共享向量/测试信任材料、个人 Riot Key。临时 public/default-release 后端只为信任隔离审计构建，结束删除，没有再构建分发包。
- R242/R240/R239/R236 四套旧目录 **104 文件**，文件集合/SHA-256 前后不变。最终受检源文件 918 个在最终检查→构建→审计后 SHA 不变。详见 [artifact-preservation](history/reports/r243/artifact-preservation.json)、[source-preservation](history/reports/r243/source-preservation.json)、[package-audit](history/reports/r243/package-audit.json)、[fingerprint-inputs](history/reports/r243/fingerprint-inputs.json)。

### 非最终失败与验收边界

- 第一次 localhost/Electron 启动被沙箱拒绝（EPERM），获准后已实跑。首轮 Go 全量发现新增诊断对旧可省略坐标/耗时不兼容，已修复并重跑受影响与全部最终检查。曾把 test-renderers all 与 desktop 全量并发导致重复 worker/原生浏览器竞争，出现超时和 90 秒预算失败；这些进程已停止，日志保留在 [non-final](history/reports/r243/non-final)，改串行最终实跑，不放宽预算。串行渲染器又发现新 ACTIVE 复核在关窗异步收尾重复读 getWorkArea() 导致 null.getBounds；已移除这次不必要读取，缩放/窗口专项和受影响全部 JS 检查重跑。
- 本机只有 macOS，**未在 Windows 实跑**：安装覆盖、DPAPI、用户真实授权缓存、真实注册码、真实服务、两机与管理页开着的 S16-13/14、Windows 原事故的无效字段与 3.5 秒精确归因。未触发远端 CI、未把注入复现/静态包审计当 Windows 实跑。S16-13/14 已加入清单，P1–P3 完全属于另一会话。
- P4、P5 激活重试/尺寸保护/诊断和本机合成验证、P6 检查/新包已交付；**P5 原 Windows 事故的精确触发根因未证实**，本单不能仅凭本机保护修复就宣称该条已彻底验收。

新安装包 SHA-256：`603572946e55911a544284843182dc783e42bca27a96a8166e07285f0838719d`。

## R245 停用后窗口恢复、非整数缩放与慢网络续租（P1–P3；已随 R248 搁置）

执行日期：2026-10-07；时间均为 Asia/Shanghai。先亲自通读 R245 全文及项目基础文档，仅修改本仓库 R245 客户端范围，未访问 deep-legends-manage 仓库。版本保持 **0.12.75**；协议字段、签名域、错误码、共享向量和“租约过期必须联网续租成功”的规则不变。未读取私钥文件、.dev.vars 或 Secret，未使用真实注册码，未发布、部署或创建 tag。测试只使用测试新生成的密钥与合成码。

历史推进状态（最终已被 R248 搁置决定覆盖，未构建 R245/R246 包）：按用户后续指示，先完成 R245 P1–P3，再执行 R246 P4–P6；**取消 R245 P4 的单独构建**，最后仅构建一次到 `dist/R246-staging-public/`。P1/P2 实现和专项验证已通过；P3 的 Go 全量/race/vet 与两标签专项通过，全量 renderer 仍有藏品重扫测试竞态失败，尚未通过全部前置检查；未构建安装包。

### P1 修复前复现与模拟方式

- 修复任何运行代码前，使用真实 macOS Electron 43.3.0、生产 main.cjs/窗口控制器、隔离 userData、仅 localhost 合成状态接口。通过 `--force-device-scale-factor=1.25/1.5/1.75` 模拟；每档实际核验 `screen.getDisplayMatching(...).scaleFactor` 与页面 `devicePixelRatio` 均等于目标倍率。没有把 CSS zoom 或 CDP 页面缩放当作原生显示缩放。
- 三档各测普通、最大化、最小化、被另一个 alwaysOnTop 原生窗口遮挡，共 12 场。流程为 ACTIVE→REVOKED→在真实页面输入合成码并提交→ACTIVE。自然路径均恢复；**未自然复现 Windows 原事故**，本机原生读取仍返回整数。证据：[自然基线](history/reports/r245/electron/baseline.json)。
- 为实证非整数读数缺口，仍用真实 Electron，额外把原生读数按“物理像素取整后除以倍率”转换，并在恢复参数含小数时注入原生忽略。没有改生产逻辑来产生故障。修复前 125%/175% 停在 860×580，150% 改成默认尺寸而未回到停用前 1201×781；这属于**故障注入复现**，不是 Windows 自然复现。先注入无忽略时 macOS 原生接受小数，已保留该对照；不能由本机结果断定 Windows 就是同一触发条件。证据：[注入基线](history/reports/r245/electron-injected-final/baseline-fractional.json)、[汇总](history/reports/r245/electron-summary.json)。

### P1/P2 实现与专项验证

- 窗口提交的 Rectangle、最小尺寸与保存快照统一取整；窗口存档既有写入入口也会取整。停用前保存正常矩形/最大化/全屏，最小化时优先正常矩形，零尺寸不覆盖有效记录。ACTIVE 复核保存目标大小和位置；原生失败捕获异常类型，按保存记录重设，再失败使用 initialWindowBounds；诊断记录 fallback_used。透明切换与锁定态不保存小窗规则保留。
- 非法窗口诊断只回显固定几何字段的原始数值，不回显字符串或其他内容；增加 non_integer/non_positive/missing/out_of_range。有效导出新增 scale_factor 与 native_error（无异常为 null），保留严格几何校验。原生异常只允许标准 Error 类型名，无错误原文。
- 请求超时 15 秒，激活仍只重试一次且每次重新签名，总后端预算 35 秒；前端 watchdog 为 36 秒，沿用原来额外 1 秒本地返回余量。整个请求期间按钮保持“正在激活…”。过期缓存首次续租结果前只显示普通颜色“正在连接激活服务…”，第一次真正失败后才显示原错误。启动退避 3/6/12/15/30/60 秒，成功后恢复正常 60 秒心跳及 15/30/60 秒失败退避；未延长租约或提前放行。
- httptrace 输出 dns_ms/connect_ms/tls_ms/ttfb_ms/body_ms；TTFB 从请求开始累计，其他是阶段耗时，未发生的阶段为 null，失败中的阶段记录已耗时间。并发拨号按外层跨度计时，锁保护回调与诊断快照；不保留地址、IP、主机名、请求内容或错误原文。
- 真正等待 10 秒的响应与 12 秒的响应体，经本机 HTTPS 服务签发测试租约，激活/续租四种组合均成功；TLS、连接、TTFB/body 测量与实际诊断导出通过。过期缓存在首次请求进行中、失败及六档退避期间始终拒绝业务，只有有效签名续租成功才能恢复。
- 最终真实 Electron 12 个自然场景及 3 个非整数读数/忽略注入场景全部通过，恢复停用前的普通矩形或最大化。实际 Go 诊断入口校验两份 Electron 报告全部通过。纯模块另测原生 setBounds/setContentBounds/setMinimumSize 异常、最小化零读数及渲染回调失败；临时副本两项变异均被断言杀死、exit 1、无编译/语法失败，完成删除。
- 证据：[Electron 最终](history/reports/r245/electron-final/after.json)、[非整数最终](history/reports/r245/electron-final/after-fractional.json)、[变异](history/reports/r245/mutations/results.json)、[Chromium](history/reports/r245/chromium/chromium.json)。**未在 Windows 实跑**：Windows 安装、DPI、DPAPI、真实码/真实服务和双机 S16；未触发 Windows CI。

### 非最终失败记录与共享工作区协调

- 首轮 localhost 启动受沙箱限制（listen EPERM），获准后真实 Electron 实跑；首次注入夹具缺 window-bounds.json 导致夹具 ENOENT，补齐隔离初始存档后在任何运行代码修改前重跑成功。自然和注入记录均保留，没有把夹具错误当作业务复现。
- 首轮纯模块暴露兜底后的第二次复核又恢复保存目标，导致 R243“两次原生忽略”预期不符；修正为记录本次已使用 initial fallback，相关 R240/R243/R245 窗口测试全部重跑通过。
- 非最终 Go 全量：R244LightDiscoveryAndOneSweepPerLaunch、R244SelfCatchUpPublishesOtherPlayersBeforeLCU 失败；当前源码单独重跑两项 PASS。同期 R244 变异入口直接改共享运行文件，存在测试编译输入被临时变异干扰的可能，未把该轮结果当最终验收。重新检查采用当前源码，并记录输入 SHA。
- 首轮 renderer 全量：R129 stats undefined、R193 三项旧卡片断言、R203 window.dispatchEvent 不存在；另一会话“执行指定工单”正在修复 R244 的兼容夹具。本会话未改相关运行代码或夹具。
- 首轮 desktop 全量：ui-scale.test.cjs 模拟窗口缺 getNormalBounds，新几何复核在异步收尾触发 TypeError；只给该模拟窗口补足 API，原缩放/信任/变异断言不动，单独受影响测试 PASS。另 dirty collection rescans 按 R240 要求单独连续三次仍 FAIL（9.265/8.419/9.940 秒），完整记录见 [三连测](history/reports/r245/dirty-collection-three.json)，未修改 app.js 非授权部分或 refresh-orchestration.test.cjs。
- 用户明确授权同步失败记录后，已发给“执行指定工单”会话，协调由负责 R244 改动者核查；没有把发送消息授权当成修改其他模块的授权。上述非最终日志保留于 [non-final](history/reports/r245/non-final/)。
- 最新只读状态确认：“执行指定工单”会话已完成 R244 本地实现，仍报告 Node 全量有同一个修改前已存在的收藏重扫失败；没有通过该会话消除本单全量检查阻塞。

### P3 当前完整检查与剩余阻塞（2026-10-07）

- 最终稳定源码 Go 全量 16:32:16–16:35:33（196.711 秒）、race 16:35:33–16:39:22（229.614 秒）、vet 16:39:22–16:39:24（1.587 秒）均 exit 0。default 授权专项 16:24:00–16:24:39（39.465 秒），50 PASS/0 SKIP/0 FAIL；staging 16:24:39–16:25:50（70.879 秒），51 PASS/0 SKIP/0 FAIL。完整命令、开始/结束与最后五行列于下方。前轮发现 R244 会话在检查期间更新 `backend/gameplay_test.go`，因此重新跑完上述三项，并核验 466 个 backend Go 文件 SHA 前后相同。
- 最终 desktop 全量 16:30:34–16:31:54（80.481 秒）exit 1，312 PASS/1 FAIL/2 SKIP；唯一失败仍为受限的 dirty collection rescans，ui-scale 模拟窗口补齐 API 后通过。详见 [desktop](history/reports/r245/desktop.json)。
- 当前 renderer 全量 16:23:32–16:25:54（141.735 秒）exit 1，只有 dirty collection rescans 失败；R129/R193/R203 兼容夹具已由 R244 会话修复。原受限测试的后一组三连测虽 PASS（2.217/2.487/2.433 秒），全量仍失败，未用三连成功掩盖全量失败。
- 独立核验指出测试等待的是 transport 完成计数，而非 `refreshStatus` 经过请求 token 检查后实际提交的 clean/dirty 状态。已在隔离临时副本准备仅测试的状态观察/等待补丁，原去重与 60 秒断言保留；副本连续三次 PASS（2.335/2.233/2.240 秒）。R240 P5.4 明确禁止修改该文件，此前请求用户授权，**尚未应用**；用户本轮改为按三连测试及相关性规则处理，不再等待该补丁授权，也未修改 app.js 非授权部分。
- 检查报告：[Go 全量](history/reports/r245/go-full.json)、[race](history/reports/r245/go-race.json)、[vet](history/reports/r245/go-vet.json)、[default](history/reports/r245/license-default.json)、[staging](history/reports/r245/license-staging.json)、[renderer](history/reports/r245/renderers.json)、[当前三连](history/reports/r245/dirty-current-three.json)、[补丁副本三连](history/reports/r245/dirty-proposed-three.json)。

### P3 检查明细（未全部通过，不构建）

所有时间为 Asia/Shanghai；Go 三项已在稳定源码上重新实跑，466 个 backend Go 文件 SHA 前后相同。授权两标签检查的运行代码未再改动。Node 两项仍保留真实失败。

`go-full`：`go test -count=1 ./backend`；2026-10-07T16:32:16.480300+08:00 → 2026-10-07T16:35:33.190905+08:00；196.711 秒，exit 0。

```text
ok  	lol-loot-assistant/backend	195.181s
```

`go-race`：`go test -count=1 -race ./backend`；2026-10-07T16:35:33.191466+08:00 → 2026-10-07T16:39:22.807495+08:00；229.614 秒，exit 0。

```text
ok  	lol-loot-assistant/backend	228.117s
```

`go-vet`：`go vet ./...`；2026-10-07T16:39:22.808468+08:00 → 2026-10-07T16:39:24.396118+08:00；1.587 秒，exit 0。

```text

```

`license-default`：`go test -count=1 -race -json -run R232|R233|R234|R236|R237|R240|R242|R243|R245 ./backend`；2026-10-07T16:24:00.270528+08:00 → 2026-10-07T16:24:39.732625+08:00；39.465 秒，exit 0。

```text
{"Time":"2026-10-07T16:24:38.629354+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Output":"--- PASS: TestR232UpdateSignatureTrust (0.01s)\n"}
{"Time":"2026-10-07T16:24:38.629599+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR232UpdateSignatureTrust","Elapsed":0.01}
{"Time":"2026-10-07T16:24:38.642798+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T16:24:39.656613+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t35.741s\n"}
{"Time":"2026-10-07T16:24:39.656727+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":35.745}
```

`license-staging`：`go test -count=1 -race -json -tags=license_staging -run R232|R233|R234|R236|R237|R240|R242|R243|R245 ./backend`；2026-10-07T16:24:39.738246+08:00 → 2026-10-07T16:25:50.616501+08:00；70.879 秒，exit 0。

```text
{"Time":"2026-10-07T16:25:49.476056+08:00","Action":"output","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Output":"--- PASS: TestR236StagingUpdateHTTPGate (0.00s)\n"}
{"Time":"2026-10-07T16:25:49.476068+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Test":"TestR236StagingUpdateHTTPGate","Elapsed":0}
{"Time":"2026-10-07T16:25:49.478893+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"PASS\n"}
{"Time":"2026-10-07T16:25:50.485777+08:00","Action":"output","Package":"lol-loot-assistant/backend","Output":"ok  \tlol-loot-assistant/backend\t35.360s\n"}
{"Time":"2026-10-07T16:25:50.485828+08:00","Action":"pass","Package":"lol-loot-assistant/backend","Elapsed":35.361}
```

`renderers`：`node scripts/test-renderers.cjs all`；2026-10-07T16:23:32.345229+08:00 → 2026-10-07T16:25:54.082658+08:00；141.735 秒，exit 1。

```text
    },
    "duration_ms": 141640.70575
  },
  "success": false
}
```

`desktop`：`node --test`；2026-10-07T16:30:34.280190+08:00 → 2026-10-07T16:31:54.761043+08:00；80.481 秒，exit 1。

```text
    actual: undefined,
    expected: undefined,
    operator: 'fail',
    diff: 'simple'
  }
```

五套旧 STAGING 目录共 130 文件，集合与 SHA 前后不变；当前未构建新安装包。共享向量、两套信任配置、协议文档、版本/lockfile、app.js 与受限测试 SHA 均未改动。临时 public/default-release 后端审计二进制已删除；它不是分发安装包。详见 [Go 输入核验](history/reports/r245/go-source-preservation.json)、[受保护文件](history/reports/r245/protected-current.json)、[旧包保留核验](history/reports/r245/artifact-preservation-pending.json)。

## R247 收藏重扫测试等待状态提交（P1–P2 完成，P3 已取消）

2026-10-07，Asia/Shanghai。已亲自通读 R247 全文。用户明确授权修正受 R240 限制的这一个测试；仅改变等待条件及必要的测试观测，不修改业务、assert、定时等待、waitFor 5000ms 预算或 60 秒限制。

### 修改与根因

原测试在 status Response 返回前递增 completedStatusRequests，页面还要解析 body、检查取消/请求 token、提交 status 并解除 gate。三处等待改为读取真正提交的 `collectionDirty` 和 `collectionRescanInFlight`：首次 dirty=true/gate=false、clean=false/gate=false、第二次 dirty=true/gate=false。

真实 JSDOM 页面没有公开 app 闭包 state；r71RefreshHarness 的 state 是另一个编译函数夹具，不能证明这个页面已提交。因此只在 `bootDemoApp({observeCollectionState:true})` 对测试页面 eval 的源码注入返回两个只读值的 getter，只有目标测试开启。生产 app.js 文件没有改动，安装包没有该测试文件或新接口。新增注释说明传输完成先于页面提交。

约束核验全部通过：全文件 assert 原文、所有 setTimeout 行、5000ms waitFor、60001ms 时钟推进、app.js SHA、版本/lockfile、新共享向量 SHA 均保留。见 [约束核验](history/reports/r247/constraints.json)。

### P2 实跑

- 单独连续20次：2026-10-07T17:06:55.099696+08:00 → 2026-10-07T17:07:44.660280+08:00，20/20 PASS；逐次耗时（秒）：2.236, 2.352, 2.189, 2.245, 3.64, 4.248, 2.474, 2.288, 3.378, 2.198, 2.205, 2.189, 2.346, 2.24, 2.224, 2.238, 2.184, 2.224, 2.199, 2.257。
- 持续独立CPU进程负载连续10次：2026-10-07T17:08:37.559836+08:00 → 2026-10-07T17:08:59.857654+08:00，10/10 PASS；逐次耗时（秒）：2.275, 2.251, 2.213, 2.185, 2.227, 2.22, 2.261, 2.207, 2.273, 2.183。
- 负载方式：测试全过程同时运行独立 `node -e 'for(;;){}'` 进程，结束在 finally 终止并等待回收；没有重试或忽略失败。
- 临时变异把三处等待恢复为计数，在假 status fetch 完成计数后暂停 50ms；暂停期间触发一次现有 visibilitychange 前台刷新，确定性模拟重叠状态请求。生产业务不改，原断言/预算不改。5/5 在“60 seconds … deferred rescan”的原断言失败、exit 1，无语法/模块加载失败；相同延迟及重叠请求夹具的新等待 5/5 PASS。完成删除临时副本。
- 单纯 50ms 延迟未击中，已如实保留在 non-final；没有把未击中当变异成功。加入一次合法前台刷新后的同夹具对照证明计数可包含旧请求，状态等待能挡住该缺口。
- 证据：[20次](history/reports/r247/normal.json)、[负载10次](history/reports/r247/load.json)、[变异及对照](history/reports/r247/mutations.json)、[执行前](history/reports/r247/before.json)。

P1/P2 通过。按用户最终指示，不执行 P3、不继续 R246，转入 R248 搁置。版本保持 0.12.75，未读取私钥、未使用真实注册码、未发布；**未在 Windows 实跑**。

## R246 两小时租约（已停止、已搁置 R248）

已亲自通读 R246 全文。执行顺序和唯一最终构建按用户指示：R245 P1–P3 → R246 P4–P6；版本继续保持 **0.12.75**。除用户明确授权的共享向量文件外，不访问 deep-legends-manage 仓库；未读取会话 B 账本或其他文件。

### 共享向量交接

2026-10-07 16:44:34（Asia/Shanghai），用户提供会话 B 的预期 SHA-256 后，重新读取获准文件并核对：`39377b860440d8f609f9d28162afe086e4e01ac91d3d2ff39da2a45d5eebfcc7`，41256 字节，共 69 条（normalization 9、base64url 5、strict_json 11、requests 10、responses 34）。一致后按字节替换 `backend/testdata/license-protocol-vectors.json`，目标 SHA 再核对相同。见 [交接记录](history/reports/r246/vector-candidate.json)。没有自行生成/编辑向量。

### 按最新指示处理 R245 P3 的收藏重扫失败

用户本轮要求：单独连续三次全通过才完整重跑 renderer；仍失败则检查是否相关，相关才修，无关停止并报告，不能放宽断言/预算。按此规则，没有应用此前待授权的测试补丁。

第 1 次：`node --test --test-name-pattern=dirty collection rescans desktop/refresh-orchestration.test.cjs`；2026-10-07T16:44:38.709963+08:00 → 2026-10-07T16:44:46.286530+08:00；7.576 秒，exit 1。

```text
    actual: undefined,
    expected: undefined,
    operator: 'fail',
    diff: 'simple'
  }
```

第 2 次：`node --test --test-name-pattern=dirty collection rescans desktop/refresh-orchestration.test.cjs`；2026-10-07T16:44:46.286638+08:00 → 2026-10-07T16:44:53.980022+08:00；7.693 秒，exit 1。

```text
    actual: undefined,
    expected: undefined,
    operator: 'fail',
    diff: 'simple'
  }
```

第 3 次：`node --test --test-name-pattern=dirty collection rescans desktop/refresh-orchestration.test.cjs`；2026-10-07T16:44:53.980177+08:00 → 2026-10-07T16:44:56.737499+08:00；2.757 秒，exit 0。

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 0
ℹ todo 0
ℹ duration_ms 2710.328791
```

三次结果 **FAIL/FAIL/PASS**。前两次均在原 `desktop/refresh-orchestration.test.cjs:271` 报 `view change after 60 seconds did not start the deferred rescan`。未满足三次全通过，**没有完整重跑 renderer，没有构建安装包**。

执行路径核验：此 JSDOM/demo 测试的授权 fixture 固定 ACTIVE，没有激活提交、Go 服务、原生窗口或共享向量读取。R245 的激活 watchdog/非 ACTIVE 文案变化不执行，ACTIVE 初始化路径没有改动；两单后端/窗口变化也不执行。测试在 status Response 返回前就递增完成计数，却未等响应体、控制器和 request token 校验后的状态提交。将失败归为本单范围外的既有测试时序问题，依用户指示停止；精确失败瞬间的 token/状态尚未捕获，未宣称精确根因已证实。

隔离撤回 R245 UI 的三次单测、当前/撤回 UI 的完整测试文件对照均 PASS；120ms clean 响应体延迟注入也未复现失败。上述成功不能替代本轮三连失败或完整 renderer 验收，临时副本均已删除。完整分析、原失败日志、诊断对照及出处见 [失败分析](history/reports/r246/dirty-analysis.md)、[三连汇总](history/reports/r246/dirty-three.json)、[受保护文件 SHA](history/reports/r246/protected-stop.json)。原测试、app.js、断言、等待预算、60 秒限制均未修改。

### 第一次暂停时的历史状态（被下方最终停止决定覆盖）

第一次暂停时，R246 仅完成 P4 的向量交接。`licenseLifetime` 当时仍为 15 分钟；两小时实现、旧测试与向量 SHA/count 校验更新、P5、P6 尚未执行。没有把当前源码当成可验收的 R246 客户端。后续全部规定检查通过后才允许唯一一次构建到 `dist/R246-staging-public/`，未执行 R245 单独构建。

没有读取私钥、使用真实注册码或发布。版本/lockfile、信任配置与协议文档未改动。**未在 Windows 实跑**。R243 及更早 STAGING 目录保留，未写入新安装包。

新安装包 SHA-256：**未构建，暂无**。

### 最终停止决定（R248）

用户在 R247 P1/P2 完成后决定搁置注册码，明确不执行 R247 P3、不继续 R246。此前“继续执行/最后构建 R246”的文字是历史指示，已被本决定覆盖。停止前已将 licenseLifetime 与测试 issuer 改为 7200 秒、新增两小时边界/断网测试、更新旧测试与向量 SHA/count、隐私和协议说明；定向 race 与两项 15 分钟旧上限变异完成。未完成 R246 全套最终检查或包验收，没有构建 `dist/R246-staging-public/`。

当前两小时半成品与 69 条共享向量原样保存到 R248 快照，随 license 标签关闭，不把 R246 标为验收完成。R248 指定的 license 回归中，R242 的旧“到期 issued+900 合法”夹具与两小时 issuer 冲突；不放宽租约截止/授权到期安全检查，不按本单继续修 R246。


## R248 注册码搁置、默认关闭与审计包（P0–P4 完成）

2026-10-07，Asia/Shanghai。亲自通读 R248 全文；按用户最终决定只完成 R247 P1/P2，取消 P3 与 R246 后续。版本保持 **0.12.75**，不推送、不打 tag、不发布、不读取私钥/`.dev.vars`/Secret、不使用真实注册码、不访问 manage 仓库。**未在 Windows 实跑**。

### P0 完整快照

先使用 `git stash --include-untracked` 保存所有非 dist 工作区内容，导出 binary patch、验证 workspace/snapshot 两个 Git bundle，再提交完整快照。备份目录 `/private/tmp/r248-pre-shelve-backup-b39401e2`；stash `b385e011904f4e33a584e2d9a4d64a5084f8055e`。raw CRLF、文件 mode 和并发 R238 更新另存补充副本，恢复后的 SHA/mode 全部核对一致。

快照分支 **codex/license-shelved-r232-r247**，提交 **475c20dc11cad9a0d808be02498c78c74e80d76e**；tree **4413 文件**，本次 commit 改动 1189 文件，不含 dist。提交禁用 Git 签名，未使用任何签名私钥。已回到原分支 `codex/release-0.12.75`（HEAD b37357b2a6cf29397ac0f16255d37fa1566e87ac），未推送。保留 stash 和两个验证过的 bundle，可用 `git bundle clone` 或 stash+patch+原始行尾/mode补充恢复。没有覆盖另一会话的 R238 工单更新。见 [快照记录](history/reports/r248/snapshot.json)。

### P1 默认关闭实现

- Go 所有授权运行代码/专项测试和 R232 更新验签进入 `license` 标签；OS 和 staging 约束与 license 取交集。单独 license_staging 不会打开授权。默认仅编译 disabled adapter，状态 `{"state":"DISABLED"}`，业务直接启动/放行，无设备身份、DPAPI 授权缓存、信任根或续租循环。
- 默认 HTML 直接展示 app-frame；disabled UI 同步放行、无状态轮询，不进入 pending 双隐藏/原生握手。授权到期行和表单隐藏。普通 startup-loading 仍按原业务连接规则运行，不承担授权等待。带 license 后端在静态资产缓存/hash/压缩前注入 pending/hidden，保留原授权安全逻辑。
- 桌面不可变 `license-build.cjs` 默认 false；普通 bounds 恢复/持久化/缩放/最大化/单实例显示正常运行；不创建授权窗口控制器、不读状态/透明切换/等待握手，原生门禁直接放行。preload 仅在显式打包参数下启用授权 IPC。恢复 0.12.76 的异步系统代理下发。
- 默认隐私仅删除授权字段与 stores 条目，保留其他业务新增内容；安装器默认许可文本恢复 v0.12.76，授权版文本另存且仅 license 嵌入。卸载授权凭据清理也进入 license 标签。默认不产生 license_* 事件，客户端白名单仍保留事件名。
- 正式脚本清空 GOFLAGS、固定默认标签和安装器标记；staging 脚本在临时副本中同时写 true 与 `license,license_staging`。release checker 默认拒绝授权域名、kid、公钥/测试材料和隐私定稿句，并拒绝 true 桌面标记。所有源码、测试、向量、账本保留。

### 在线更新与 0.12.76

R232 曾要求 Ed25519 signed_manifest 且在缓存/下载/应用前核对信任根；默认现在恢复 0.12.76 的 schema/版本/资产名/大小/SHA-256/固定 GitHub 发布地址检查，JSON 用原 Unmarshal 方式，不要求签名 envelope；实际安装包大小与 SHA-256 校验仍保留。带 license 的签名信任和 STAGING 禁更新保持原样。

使用本仓库保存的匿名正式资产作本地验证：0.12.76 Latest 为 7e2e42599170，111854592 字节，安装包 SHA-256 `01014312b60e591a05bfc87f86e098adf6c5fc5e59520db5aedac5b5dba02ff3`。真实 updater 从 0.12.75 经本地传输夹具读取该 unsigned Latest，得到 available/0.12.76；原发布包文件大小/hash通过，错误 hash/清单断言拒绝。不是访问授权服务器、不是实际安装或发布。

### P2 测试分组与变异

Go 专项编译隔离；Node 授权测试改名 `.license.cjs`，默认 discovery 不运行、没有新增 SKIP。CI 删除授权专项运行命令，保留默认包授权材料缺失检查。手动浏览器/Electron脚本须显式 DEEP_LEGENDS_TEST_LICENSE=1，夹具仅在隔离副本中注入 pending/true。以后恢复步骤见 [重新启用说明](license-shelved-reenable.md)。

新增默认状态/HTTP门禁/隐私、启动授权网络测试服务器计数、首帧/到期行/原生门禁、安装器文本与旧 Latest 资产检查。默认零授权网络变异实际发一次授权请求，经测试 transport 转向 localhost 服务，计数 1，在原零计数断言 FAIL；默认误开遮罩的临时源码用真实 Electron 首次 show 同栈截图，首帧断言 FAIL。两者 exit 1，均非语法/编译失败。临时副本和 overlay 删除，见 [两项变异](history/reports/r248/mutations/results.json)。

### 真实 Electron 首帧与启动耗时

基线为本仓库 v0.12.76（8ff273d7bc3e5d6d9ade683dc66c62f40971f854），从 tag 导出桌面和 web 源码；双方使用同一真实 Electron 43.3.0（已核对实际依赖和 lockfile）、本机 macOS、隔离 userData、同样的 localhost 合成业务后端。真实 native show 同一调用栈启动 capturePage 和 DOM读取，不是浏览器代替 Electron，不使用注册码或联网授权。拦截外网并记录请求，当前端/桌面没有 license API 请求，诊断 license_* 数为0。

最终冻结源码交错 before→after 各3次 total（ms）：0.12.76 **1044/673/641**，默认关闭版 **623/761/608**；均值 **786→664**，中位 **673→623**。backend-ready→show：基线259/156/186、当前148/226/149，均值200.33→174.33，中位186→149。单次波动与冷启动影响全部保留；仅3次 macOS 样本，不能宣称每次或 Windows 都更快。最终比较没有其他测试同时运行；开始时有一次旧 STAGING 文件 SHA 保留核验，之后才启动本轮 Go 重跑。较早的并发测试样本保留在 non-final，不用于最终包归因。


首次 show 均为正常可调整/最大化窗口、opacity=1、主框架可见、表单/授权到期行不可见；保留普通 LCU 连接提示。额外真实 saved bounds 样本恢复1050×750，无860×580授权窗。截图与逐帧/网络证据见 [Electron startup](history/reports/r248/electron/startup.json)、[均值与中位](history/reports/r248/startup-comparison.json)。**未在 Windows 实跑**。

### P3 最终检查

最终源码已冻结在 `/private/tmp/r248-checked-source-r147th5m`，运行指纹最初622e118df684；仅补齐lcu.go新字段对齐空格后，最终构建指纹f9c647287d80；与另会话结束后的当前工作区源码逐文件一致，无漏掉的新源码文件。此前检查过程中 R251 业务源码并发更新，所以保留原结果到 non-final，再在同一冻结源码重跑，构建也将用该源码，避免检查/打包错配。

冻结源码第一次默认 Go 全量失败：仅 `TestGameplayOverviewAppliesSeasonFallbackToIncompleteSGPRanks` 在 `testing.go:1267` 的 TempDir RemoveAll 报 `season-stats/sgp: directory not empty`；业务断言没有失败。该测试直接调 overview 后未等 seasonBackfills；trackTestStore 只关诊断文件，不等后台季赛写入。无法证明是哪一次save与删除重叠，但失败与后台写/目录清理竞态一致，与R248授权适配器无关。失败日志/最后五行保留在 non-final/before-authorized-cleanup-fix。

向用户展示一行候选补丁，仅注册已有 `waitGameplaySeasonJobsBeforeCleanup(t, a)`；临时 Go overlay 连续20次 race 通过（3.123秒）。用户明确授权这个单测试清理修复后，才同时应用到原工作区与冻结源码；断言、业务、现有5秒预算不变。Go全量/race/vet已在最终授权修复后全部通过，原失败仍保留，不用成功结果覆盖原失败。见 [一行补丁](history/reports/r248/proposed-season-cleanup.patch)、[候选验证](history/reports/r248/proposed-season-cleanup-test.log)。

### P4 默认 public 包审计

默认检查通过后，经正式 build-desktop.sh 完成一个 public/default 审计包，已移入 dist/R248-no-license-public/。不执行 R245/R246 STAGING 构建。只用于审计，不用于发布。


R248 正式脚本的前三次预检退出均发生在后端/安装器构建前：第一轮发现 lcu.go 新字段缺一个 gofmt 对齐空格；仅规范该空格，gofmt 前后 canonical bytes 相同（无语义变化），定向默认/身份/清理测试及 vet 通过。第二轮因审计副本漏复制 README 与职业归属说明导致正式 Go 分片预检失败；修复自己的隔离驱动，补齐全部文档/历史测试输入和 workflow，没有 skip、删测试或放宽断言。第三轮 Go 分片全过后，安装器预检发现隔离副本缺 payload/files/.gitkeep；保留该占位并先用独立 --check-inputs 验证副本后才继续。三轮完整日志在 non-final/package-format-preflight、package-documentation-preflight、package-payload-placeholder-preflight。前三次调用均在预检退出，没有后端-build/nsis/安装包产出；真正 NSIS 及安装器生成只有最终这一次。


### 最终实跑结果与最后五行

以下均为2026-10-07、Asia/Shanghai。默认检查对应同一冻结源码；随后仅补齐 lcu.go 的一个对齐空格，gofmt canonical bytes 一致，无语义变化，定向编译/测试和 vet 通过，正式脚本又在实际最终格式上完整运行所有Go分片、vet及安装器test/vet。审计驱动后续只修正副本输入，不改变业务源码或运行指纹。完整原始日志、最后五行和 cwd 见 [verification.json](history/reports/r248/verification.json)。

| 检查 | 开始–结束 | 耗时秒 | 结果 |
|---|---|---:|---|
| go-full | 18:29:56–18:32:56 | 180.301 | PASS |
| go-race | 18:29:55–18:33:34 | 219.152 | PASS |
| go-vet | 18:29:53–18:29:54 | 1.375 | PASS |
| renderers | 18:20:06–18:22:30 | 143.297 | PASS |
| desktop | 18:23:12–18:24:52 | 100.587 | PASS |
| license-go | 18:21:44–18:22:40 | 56.189 | R246旧夹具冲突，按单记录、不修 |
| installer-test | 18:21:45–18:21:49 | 4.622 | PASS |
| installer-vet | 18:21:44–18:21:45 | 0.647 | PASS |
| worker | 18:21:39–18:21:40 | 1.208 | PASS |
| electron | 18:29:06–18:29:15 | 9.03 | PASS |
| package | 18:47:32–18:50:50 | 197.867 | PASS |
| package-audit | 18:52:53–18:52:54 | 0.937 | PASS |
| chromium-image | 18:08:29–18:08:32 | 2.58 | PASS |
| chromium-css | 18:08:28–18:08:33 | 4.973 | PASS |

renderer：1330项，1326 PASS、0 FAIL、4项既有平台skip，166文件（默认未发现任何 .license.cjs），最慢文件84.450秒；未放宽90秒/文件、240秒/全套预算。desktop：292项，290 PASS、0 FAIL、2项既有平台skip；注册码专项没有新增skip。手动保留的 Node license专项33/33 PASS、0skip。license Go指定命令只有 R242 旧900秒到期夹具与R246两小时issuer冲突（license_r242_test.go:150），保留签名及到期拒绝规则，不修R246半成品。

go-full 最后五行（不足五行时全部列出）：

```text
ok  	lol-loot-assistant/backend	173.039s
```

go-race 最后五行（不足五行时全部列出）：

```text
ok  	lol-loot-assistant/backend	217.879s
```

go-vet 最后五行（不足五行时全部列出）：

```text
（无输出，exit 0）
```

renderers 最后五行（不足五行时全部列出）：

```text
    },
    "duration_ms": 143239.855208
  },
  "success": true
}
```

desktop 最后五行（不足五行时全部列出）：

```text
ℹ fail 0
ℹ cancelled 0
ℹ skipped 2
ℹ todo 0
ℹ duration_ms 100528.827917
```

license-go 最后五行（不足五行时全部列出）：

```text
--- FAIL: TestR242LicenseExpiryStrictSignedField (0.00s)
    license_r242_test.go:150: valid signed expiry rejected invalid license expiry
FAIL
FAIL	lol-loot-assistant/backend	33.028s
FAIL
```

### 唯一实际审计包与只读结论

正式脚本成功这轮：18:47:32–18:50:50，197.867秒；key mode **public**，无license标签、license-build=false、版本 **0.12.75**，指纹 **f9c647287d80**。NSIS实际生成一次，Go安装器壳包裹该NSIS一次，不是两个分发候选。前三轮只在格式/副本输入预检退出，无安装包；没有通过跳过正式测试达成构建。仅修自己的审计副本驱动，所有预检失败原样保留。

最终目录 `dist/R248-no-license-public/`，安装包 `Deep Legends Setup 0.12.75-public.exe`；Go后端和ASAR固定摘要一致，两个授权地址、所有已知kid/publickey（文字与原始字节）和R237隐私定稿句均缺失，测试夹具不在ASAR。正式更新方式/实际0.12.76发布资产本地校验通过。五项fuse与之前一致：RunAsNode=false、NODE_OPTIONS=false、CLI inspect=false、EmbeddedAsarIntegrity=true、OnlyAsar=true；PE的ELECTRONASAR资源与真实app.asar header SHA-256匹配。见 [只读审计](history/reports/r248/package-audit.json)。

未推送、未打tag、未发布；未读取私钥或真实注册码。**未在 Windows 实跑**。旧五套STAGING共130文件SHA前后相同，R245/R246包未生成；P0快照仍可恢复。当前工作区与检查快照业务源码一致，保留另一会话R251等更新。

新安装包 SHA-256：**61bcbe529eaf9a309da0601e42954275dd0e6ab9f71540aae655ec24e12d59ea**。
