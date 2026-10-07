# WORKLIST-R233：R232 复核修复 — 客户端时钟回拨误锁、授权文件写入失败永久锁定，并实现 deep-legends-manage 授权服务端与管理页

日期：2026-10-06。评估与工单：Claude。后续执行：GPT。核对基线：HEAD `10933787`，版本 **0.12.75**，工作区含 R232 客户端改动。

**状态：问题已确认，未修复，服务端未开始。本单不改协议字段与签名域、不改版本、不发布。**

本单合并两部分，对应同一件事（让 R232 的授权方案真正可上线）：

| 部分 | 仓库 | 内容 |
|---|---|---|
| 客户端修复 | `deep-legends`（本仓库） | P1 时钟回拨容差、P2 授权文件写入容错、P6 联调构建与生产公钥写入 |
| 服务端 | 新仓库 `deep-legends-manage` | P3 工程与环境、P4 授权服务、P5 管理页、P6 协议一致性测试、P7 运维与隐私 |

执行方式：P1/P2 与 P3–P5 互不依赖，可由两个会话并行；P6 在 P4 完成后进行；P8 是最终验收。**服务端的所有改动只发生在 `deep-legends-manage`，在本仓库只做 P1、P2、P6 列明的文件。**

## 复核结论（R232 客户端）

通读 R232 客户端授权代码与账本，核对路由门禁、状态机、设备存储、桌面端后端摘要校验，并重跑 `desktop/license-gate`、`backend-integrity`、`share-export`、`portable-template` 与 `backend/web/license-ui` 测试（全部通过）。Go 测试与真实 Windows 行为本次未能独立重跑，Go 部分以 R232 账本记录为准，**需 GPT 在最终改动后重新实跑**。

协议、状态机、默认拒绝路由、业务上下文取消、DPAPI 设备密钥、后端 SHA-256 校验的设计和实现与 R232 方案一致，未发现绕过类缺陷。下面 P1、P2 是会误伤正常用户的容错问题。

---

## P1　系统时钟回拨 1 秒即锁定已激活会话（客户端）

**现象证据**

- `backend/license.go` 的 `expireLocked`：ACTIVE 状态下只要 `Now().Unix() < m.disk.LastWallTime` 就进入 `NETWORK_LOCKED`；`exchange` 每次请求前后都会把 `LastWallTime` 推进到当前时间，所以它始终约等于最近一次续租时刻。
- `newLicenseManager` 启动路径同样要求 `now >= LastServerTime && now >= LastWallTime`。
- `backend/license_test.go`（约第 305 行）明确断言「回拨 1 秒也拒绝缓存启动」，零容忍被测试固化。

**影响（推断，需实测确认）**：Windows 时间同步、虚拟机/休眠恢复、本机时钟偏快后被校正，都会产生数秒的向后跳变。发生在两次续租之间时，已激活用户会被 `NETWORK_LOCKED`：业务 context 被取消、LCU 连接断开、待执行的自动接受/选禁被清掉、界面回到授权遮罩，直到下一次心跳成功（最长约 60 秒，期间没有原因提示）。另外 `LastServerTime` 取自服务器时间，本机时钟比服务器慢几十秒的用户，重启后离线时无法使用有效租约。

**修复要求**

1. 向后跳变允许容差，建议 **300 秒**（远小于 900 秒租约上限）：运行中与启动时对 `LastWallTime`、`LastServerTime` 的比较都减去该容差；超过容差才视为回拨并锁定。容差写成命名常量，并在 `docs/license-protocol.md` 的时间一节记录取值与理由（回拨不超过容差时，离线重启最多多得约 300 秒，仍不超过租约上限）。
2. 运行中的到期仍以单调时钟 `deadline` 为准，不因墙钟小幅回拨而延长；不得因放宽检查而让任何已签发租约的实际可用时间超过协议上限。
3. 因回拨超过容差而锁定时，**立即触发一次续租**，不等下一个心跳；续租成功即恢复 ACTIVE。
4. 更新 `license_test.go` 中的回拨用例：≤300 秒允许、>300 秒拒绝（不是单纯放宽旧断言）；新增：本机时钟比服务器慢 60 秒时，续租后立即重启仍可离线使用有效租约；运行中向后跳变 5 秒不触发锁定、不取消业务 context；向后跳变 10 分钟触发锁定并立即续租。

## P2　授权文件偶发写入失败让 ACTIVE 用户永久进入 DEVICE_ERROR，需重新输码（客户端）

**现象证据**

- `exchange` 中三处 `m.options.Store.Save(...) != nil` 都直接 `setStateLocked("DEVICE_ERROR")`：请求前持久化计数器、收到 REPLACED/REVOKED 后保存、续租成功后保存租约。
- `writeLicenseFile` 只尝试一次「临时文件 + `os.Rename`」，没有重试。
- `Run` 的 `due` 条件排除 `DEVICE_ERROR`，`Renew` 在该状态直接返回 `errLicenseLocked`，**心跳此后不再运行**，唯一恢复方式是用户在遮罩上重新输入注册码（普通码会因此再次接管占用）。

**影响（推断，需实测确认）**：杀毒软件或索引服务短暂占用刚生成的文件、磁盘瞬时繁忙，都可能让 `Rename` 偶发失败。一次偶发失败就会在游戏中锁住软件、取消业务任务，且不会自动恢复。

**修复要求**

1. `writeLicenseFile` 对写入/`Rename` 错误做有限重试（建议 3 次，间隔约 50/200/800 毫秒），仍失败才返回错误。
2. 运行中的持久化失败不得直接锁定：
   - 续租成功后保存失败：内存中的租约和截止时间照常生效，记录诊断事件，下一次心跳重试保存；只有重启时才依赖磁盘。
   - 请求前保存计数器失败：本次不发请求（计数器未落盘不得使用），按退避重试，不改变当前状态，租约到期才进入 `NETWORK_LOCKED`。
   - 保存 REPLACED/REVOKED 终态失败：仍立即锁定（安全方向），保持内存终态，并持续重试写入，避免重启后复活。
3. `DEVICE_ERROR` 只用于启动时设备密钥不可读或无效这类无法继续的情况，该状态保持现有行为（遮罩 + 重新激活）。
4. 补测试：用可注入失败的假存储覆盖「偶发 1–2 次失败后成功」「持续失败直到租约到期」「终态保存失败」；断言各自的状态、业务 context 是否被取消、诊断事件，以及心跳是否继续。

---

## P3　`deep-legends-manage` 工程、环境与资源

### 范围与硬性约束

- 新建独立仓库 `deep-legends-manage`，不放进 `deep-legends`，也不放进 `relay/`。**协议以本仓库 `docs/license-protocol.md` v1 为准，不得改动字段、签名域、编码规则**；发现协议有缺口先记录到执行账本，不自行修改。
- 技术栈：Cloudflare Workers（TypeScript，ES modules）+ Durable Objects（SQLite 存储）+ D1；管理页为同 Worker 提供的静态页面（原生 HTML/CSS/JS，不引用第三方 CDN）；测试用 `vitest` + `@cloudflare/vitest-pool-workers`；部署用 `wrangler`。Ed25519 优先用 Workers 的 WebCrypto（`Ed25519`），若运行时不支持再用经审计的纯 JS 实现（如 `@noble/curves`），两者结果必须与协议文本逐字节一致。
- **GPT 不接触任何生产密钥、生产注册码或 Cloudflare 凭据，也不索要。**生产签名私钥、管理接口配置、生产 D1 的创建与 Secret 写入由用户本人完成；GPT 只提供脚本、配置模板和步骤文档。测试环境使用测试密钥对和测试码。
- 不使用现有 `relay/riot-worker`，也不与其共用 Worker、额度或绑定。

### 目录与环境

1. 仓库结构建议：`src/`（`public.ts` 客户端接口、`admin.ts` 管理接口、`license-do.ts` Durable Object、`protocol.ts` 编解码与签名、`access.ts` Access JWT 校验、`codes.ts` 注册码生成与规范化）、`admin-ui/`（静态页面）、`migrations/`（D1）、`scripts/`、`test/`、`docs/`（协议副本、隐私声明草稿、执行账本）。
2. `wrangler` 配置两个环境：`staging` 与 `production`，各自独立的 Worker 名称、D1 数据库、Durable Object 命名空间、Secret 和路由。主机名：
   - 生产：`license.yinxiaobia.net`（客户端接口）、`manage.yinxiaobia.net`（管理页）。
   - 测试：`license-staging.yinxiaobia.net`、`manage-staging.yinxiaobia.net`（名称可由用户调整，写成配置变量）。
3. 同一个 Worker 按 `Host` 严格分流：客户端主机只响应 `POST /v1/activate`、`POST /v1/renew`，其余一律 404；管理主机只响应 `/admin*` 与静态页面，**客户端主机上不得存在任何管理路径**，管理主机上不提供 `/v1/*`。`workers_dev` 关闭。
4. Secret 与变量（名称为建议）：`LICENSE_SIGNING_KEY`（Ed25519 私钥，PKCS8 Base64）、`LICENSE_SIGNING_KID`、`ACCESS_TEAM_DOMAIN`、`ACCESS_AUD`、`ADMIN_EMAILS`（允许登录管理页的邮箱，逗号分隔）。缺少签名密钥或 kid 时所有业务接口返回 503，**不得回退到任何内置测试密钥**。

### 用户需要手动完成的事（写入 `docs/SETUP.md`，GPT 不代做）

1. Cloudflare 账号升级到 Workers Paid（或使用独立账号），开启两步验证。
2. 为 `license` / `manage`（及 staging）主机名添加 DNS 与 Worker 路由。
3. 创建 Zero Trust 的 Access 应用，保护 `manage.yinxiaobia.net`（及 staging），策略只放行用户本人邮箱（一次性验证码登录），记录 AUD。
4. 创建 D1 数据库（staging/production），把数据库 ID 填入 `wrangler` 配置。
5. 在自己电脑上运行 `scripts/gen-signing-key.mjs` 生成签名密钥对，用 `wrangler secret put` 写入 Worker；私钥另存密码管理器与离线备份；公钥发给 GPT 写入客户端（P6）。

---

## P4　授权服务（客户端接口）

### 数据模型

- **D1（码表与记录）**：
  - `licenses`：`license_id`（随机 128 bit 十六进制，主键）、`code_digest`（规范化 20 位码的 SHA-256 十六进制，唯一）、`kind`（`standard`/`admin`）、`status`（`active`/`revoked`）、`note`、`batch`、`created_at`、`revoked_at`，以及展示用摘要：`current_device_short`、`last_seen_at`、`takeover_total`、`takeover_today`、`activated_at`（首次激活时间，用于判定"从未激活"）。
  - `events`：`id`、`ts`、`license_id`、`type`（`activate`/`takeover`/`renew_replaced`/`revoke`/`restore`/`unbind`/`reset`/`create`…）、`device_short`（设备摘要前 8 位）、`client_version`、`result`。**不记录注册码、完整设备摘要、IP、租约内容。**
  - `admin_audit`：`ts`、`actor`（Access 邮箱）、`action`、`target`（脱敏标识）、`detail`（不含完整码）。
- **Durable Object（每个 license 一个，对象名为 `license_id`，SQLite 存储）**，是设备占用与计数器的唯一权威：
  - 普通码：`current_device`（设备摘要）、`revision`、每设备 `last_counter`。
  - 管理员码：设备集合（`device_hash`、`last_counter`、`last_seen`），`revision` 固定为 1；不设设备数上限。
  - 方法：`activate(device, counter, ts, kind)`、`renew(device, counter, ts, revision)`、`unbind()`、`snapshot()`。每次变更后把摘要字段回写 D1；**回写失败不影响授权裁决，但要记录并可由「重建摘要」操作修复**。
- 注册码状态（`status`）的权威在 D1；`renew` 先用 `license_id` 查 D1 确认存在且未撤销，再进入 DO。DO 不会因任意 `license_id` 被创建持久状态：未激活过的 `license_id` 的 `renew` 不写任何存储。

### 激活 `POST /v1/activate`

1. 严格解析请求 envelope（协议「编码」「请求 envelope」两节）：拒绝重复字段、未知字段/版本、尾随内容、非规范 Base64URL、超限字段（请求体上限建议 4 KiB）；验证设备签名（签名文本逐字节按协议）；`ts` 与服务器时间相差不超过 300 秒，否则返回签名的 `TIMESTAMP_INVALID`（携带 `server_time`）。
2. 载荷 `{code, client_version}`：规范化码（与客户端一致的规则）→ SHA-256 → 查 D1。码不存在 → 签名的 `INVALID_CODE`；已撤销 → 签名的 `REVOKED`。统一无效结果，不区分「不存在」与其他原因。
3. 进入 DO，**在同一个事务内**完成：`counter > last_counter` 检查并更新（`REPLAY` 即拒绝）、占用裁决：
   - 普通码：设备 ≠ `current_device` → `current_device = 该设备`、`revision + 1`，写 `takeover` 事件；设备 = 当前设备 → 幂等，`revision` 不变，不记为接管。
   - 管理员码：把该设备加入集合，不影响其他设备，`revision` 不变。
4. 成功返回签名的 ACTIVE 租约（`issued_at = server_time`、`expires_at = issued_at + 900`），字段与校验规则逐条符合协议「成功载荷」。

### 续租 `POST /v1/renew`

- 载荷 `{license_id, revision, client_version}`；先验设备签名、时间窗和计数器（DO 内原子）。
- 普通码：设备 = `current_device` 才签发租约（返回当前 `revision`）；否则签名的 `REPLACED`。**续租绝不能改变占用者。**
- 管理员码：设备必须在设备集合内，否则 `REPLACED`。
- **租约已过期的合法设备仍可续租**（否则断网恢复后无法自动恢复）。
- 被管理员「解除绑定」或「重置」后，旧设备的续租返回 `REPLACED`（文案为「注册码已被其他地方使用」，需要重新输码；该语义写入协议备注，不改字段）。`license` 已撤销返回 `REVOKED`。

### 其他要求

1. 所有响应 `Cache-Control: no-store`，参数不出现在 URL；业务结论（成功与签名错误）按协议签名；格式错误、签名无效等请求级失败返回**未签名**的 4xx（`400`），限流返回未签名 `429`，服务异常返回未签名 `503`；`426 UNSUPPORTED_VERSION` 按协议。
2. 限流：按来源 IP 与按码摘要限制激活频率（建议每 IP 每分钟 10 次激活；续租单独较宽的额度）；限流不对管理员码设置使用次数上限。限流计数用 Workers 的 Rate Limiting 绑定或 DO，说明其「最终一致、非精确」的边界。
3. 密钥：从 Secret 读取私钥签名；响应的 `kid` 来自配置；支持轮换时新旧 `kid` 并存的验证由客户端内置公钥列表完成，服务端只需在配置里切换当前 `kid`。
4. 日志：使用结构化日志，仅记录 request ID、脱敏的 license/device 标识、结果类别、耗时；**任何日志、异常栈、事件表都不得包含完整注册码、私钥、租约原文或请求体**。
5. 备份恢复：启用 D1 的 Time Travel，并提供 `scripts/export-d1.sh`（导出码表/事件，不含明文码）；写明「恢复旧数据后必须轮换签名 `kid`，使旧租约在 15 分钟内自然失效」的操作步骤。
6. 数据保留（草案，需用户确认后才写入客户端隐私文本）：`events` 保留 180 天（定时清理的 `scheduled` 任务）；`admin_audit` 保留 365 天；`licenses` 保留到被删除。删除渠道的联系方式由用户提供，GPT 不得编造。

---

## P5　管理页与管理接口

位置：`manage.yinxiaobia.net`（staging 同理），与客户端接口完全分离。

### 访问保护

1. 由 Cloudflare Access 保护整个管理主机；**Worker 还必须自己校验 `Cf-Access-Jwt-Assertion`**：取 `https://<team>.cloudflareaccess.com/cdn-cgi/access/certs` 的公钥（带缓存与轮换处理），校验 RS256 签名、`iss`、`aud`、`exp`，且邮箱在 `ADMIN_EMAILS` 内；任一失败返回 403，不泄露原因。Access 配置错误时管理页也不可访问。
2. 所有写操作要求同源（`Origin` 校验）和自定义请求头（如 `X-DL-Admin: 1`）；不使用客户端接口的任何凭据；管理接口不接受来自 `license` 主机的请求。
3. 每个写操作写入 `admin_audit`（操作人邮箱、动作、脱敏目标）。管理页不展示完整注册码（仅生成当下显示一次）、私钥、租约内容。

### 功能

- **查**：注册码列表（掩码，如 `DL-ABCDE-…-WXYZ2` 只显示首尾若干位）、类型、状态、备注、批次、创建时间；使用状态——当前绑定设备的短 ID、最后在线时间、租约剩余估计、被替换次数（总计/今日）、是否曾激活；筛选（状态/类型/批次/是否已激活/异常）、按备注搜索、分页；单个码的事件时间线（脱敏）；接管次数异常（建议 >10 次/天）标记；管理员码展示已激活设备数与列表（短 ID、最后在线）。
- **增**：批量生成（数量、类型、批次名、备注；单次建议上限 500）。码用安全随机数生成：20 位 Crockford Base32（100 bit），显示为 `DL-XXXXX-XXXXX-XXXXX-XXXXX`；服务端只存摘要；结果页**只显示一次明文**并在浏览器内生成 CSV 下载（CSV 不经服务器保存）。生成**管理员类型**需要二次确认并在对话框中重新输入「ADMIN」。首批按 99 普通 + 1 管理员创建（用户在管理页操作，不由 GPT 生成生产码）。
- **改**：编辑备注/批次；撤销；恢复；解除绑定（清空 DO 占用，`revision + 1`，下次输码可被任意设备激活）；重置（换新码并让旧码失效，同时解除绑定，显示新码一次）；「重建摘要」（修复 D1 与 DO 摘要不一致）。
- **删**：仅允许删除「从未激活过」的码；已激活过的码只能撤销，保留事件记录。
- 页面使用中文界面，沿用简洁风格即可；不添加说明性免责文案；表格在窄屏下可横向滚动；所有危险操作有确认。

### 管理接口

`/admin/api/...`，仅管理主机；对应上述功能的增删改查与重建摘要；返回值不含完整码（生成与重置的一次性响应除外）；分页与筛选在服务端完成。

---

## P6　协议一致性与联调（跨两个仓库）

### 一致性测试向量

1. 在 `deep-legends-manage` 用**测试密钥对**生成一份 `license-protocol-vectors.json`：覆盖请求签名文本与签名结果、规范化码、成功响应、各类签名错误响应（`INVALID_CODE`、`REPLACED`、`REVOKED`、`TIMESTAMP_INVALID` 等）、非规范 Base64URL、重复 JSON 字段、`kid` 非法字符等正反例，同时包含对应的测试公钥。
2. 服务端测试加载向量，验证自己的实现；复制一份到本仓库 `backend/testdata/license-protocol-vectors.json`，新增 Go 测试用客户端的 `verifySignedEnvelope`/请求签名逻辑逐条验证。**两边任何一方改协议实现，该测试必须失败。**
3. 向量里的私钥只存在于测试夹具，不得进入生产代码、安装包或生产配置。

### 联调构建（客户端，仅测试用）

1. 在本仓库用 Go 构建标签（如 `license_staging`）提供测试配置：`backend/license_config_staging.go` 写入 staging 的 origin 与测试公钥；默认（发布）构建使用 `license_config_release.go`，不含任何 staging 域名或测试公钥。把现有 `licenseOrigin`/`licenseTrustKeys()` 拆到这两个文件。
2. 提供一个仅用于联调的打包脚本，产物名与窗口标题带 `-STAGING`，**禁止接入发布流程**；发布脚本/CI 增加检查：正式产物中不得出现 staging 域名、测试公钥标识。
3. 用户在生产环境生成签名密钥并给出**公钥**后，GPT 才把它写入 `license_config_release.go` 的 `licenseTrustKeys()`；在此之前保持为空，并在账本中标明「发布被该项阻塞」。更新清单的信任公钥（`updateTrustKeys()`）同理，由发布环境的独立密钥对提供，不得复用授权签名密钥。

### 更新清单签名工具（可选，不阻塞授权上线）

在 `deep-legends-manage` 提供 `scripts/gen-update-key.mjs` 与 `scripts/sign-update-manifest.mjs`（按协议「独立更新签名」一节的 domain 与格式），私钥由用户保管；何时在发布流程中启用，由后续单独决定。

---

## P7　运维与隐私

1. 监控：Worker 与 D1 的错误率、请求量、Workers 额度用量，写入 `docs/OPERATIONS.md`（查看位置与阈值）；容量按每台每分钟一次续租估算，100 台全天在线约 14.4 万次/天，需 Workers Paid。
2. 起草 `docs/privacy-disclosure.md`：列出服务端实际保存的字段、保存期限（P4 草案）、删除渠道（待用户提供）、基础设施日志的处理，供客户端 `installer/ui/license.html` 与 `/api/privacy` 引用。**期限与渠道在用户确认前保持「待确认」，不得当作已生效政策写入客户端。**
3. 故障预案写入 `docs/OPERATIONS.md`：授权服务长时间故障时，应急做法是在服务端临时延长租约上限的配置（不超过客户端接受的 900 秒协议上限，如需更长必须同步升级协议版本），**不得**提供「服务异常全部放行」或公开万能码。
4. 签名密钥轮换流程：生成新密钥对 → 发布同时信任新旧 `kid` 的客户端 → 服务端切换当前 `kid` → 确认旧版本占比足够低后再移除旧 `kid`。写成可执行的步骤文档。
5. 管理员码泄漏演练步骤：在管理页「重置」管理员码，旧管理员设备最迟 15 分钟内被锁定，新码只发给本人。

---

## P8　验收

以下均为**后续必须实际运行**的验收，目前未运行，不得标为通过。服务端测试在 `deep-legends-manage` 执行；真机项在联调构建上执行。

| 编号 | 场景 | 通过标准 |
|---|---|---|
| S01 | 请求解析：重复字段、未知字段、尾随内容、非规范编码、超限、错误签名、`ts` 超窗、计数器不递增 | 全部拒绝；`ts` 超窗返回签名的 `TIMESTAMP_INVALID`；其余为未签名 400 或签名的 `REPLAY` |
| S02 | 普通码激活：A 激活，B 激活，A 续租 | B 立即获得租约；A 续租得到签名的 `REPLACED`；`revision` 只在换设备时递增 |
| S03 | 并发：同一码被 5 个设备、共 50 个并发请求激活 | 最终只有一个 `current_device`；`revision` 单调；失败者续租均为 `REPLACED`；计数器无回退 |
| S04 | 同设备重复激活、网络重试 | 幂等，`revision` 不变，不记接管 |
| S05 | 管理员码：多设备反复激活和续租 | 互不影响，无设备数上限，`revision` 固定；普通码无法冒充管理员类型 |
| S06 | 撤销、恢复、解除绑定、重置 | 撤销后续租 `REVOKED`；恢复后可用；解除绑定与重置后旧设备 `REPLACED`；重置后旧码 `INVALID_CODE` |
| S07 | 租约已过期的合法设备续租 | 成功签发新租约 |
| S08 | 响应校验：用公钥验证所有签名响应，检查 `expires_at - issued_at ≤ 900`、请求号/设备摘要回显 | 全部通过；篡改任一字段验签失败 |
| S09 | 签名密钥或 kid 缺失 | 业务接口 503，无回退密钥 |
| S10 | 限流 | 超限返回 429；管理员码无使用次数上限 |
| S11 | 管理页访问：未登录、非本人邮箱、缺失/伪造/过期 Access JWT、错误 `aud`、从 `license` 主机访问 `/admin` | 全部拒绝；客户端主机上不存在任何管理路径 |
| S12 | 管理页功能：批量生成、搜索筛选、撤销/恢复/解除绑定/重置、删除未激活码、尝试删除已激活码 | 状态与 D1/DO 一致；明文码只显示一次；已激活码不可删；每次写操作有审计记录 |
| S13 | 日志与存储审计 | 日志、事件表、审计表、导出文件中无完整注册码、私钥、租约原文、请求体 |
| S14 | 保留期清理 | 超期事件被清理，未超期保留 |
| S15 | 协议一致性向量（两个仓库） | Go 与 TypeScript 双向验证全部通过；故意改一端实现时测试失败 |
| S16 | 联调构建 + 两台 Windows 真机：换机、旧机离线到期、管理员多机、服务不可达后恢复 | 符合 R232 的 T03–T10、T16；真机证据不能由虚拟测试代替 |
| S17 | 客户端 P1/P2 修复 | 见 P1、P2 的测试要求；全套 Go/JS 在最终改动后重新实跑 |
| S18 | 发布产物检查 | 正式产物不含 staging 域名与测试公钥；生产公钥未写入前，账本标明发布被阻塞 |

### 交付与记录

- 客户端：在 `docs/r232-execution-ledger.md` 末尾追加 R233 一节，记录容差、重试参数、联调构建、实际测试输出与时间；按 R218 保留 `go test -count=1 ./backend` 末尾输出、`-race`、`go vet ./...`，以及 `node scripts/test-renderers.cjs all` 与 `desktop` 的 `node --test`。
- 服务端：在 `deep-legends-manage/docs/execution-ledger.md` 记录依赖版本、配置、测试输出、与协议的偏差（应为零）和未完成项；未完成的验收如实标注，不写「已通过」。
- 不改版本号，不发布，不创建 tag；生产部署、生产密钥、生产注册码均不在本单范围内，由用户在验收后自行完成。

## 已知遗留与风险（不属于本单修复）

- 生产 `licenseTrustKeys()`/`updateTrustKeys()` 在用户提供公钥前保持为空，客户端会拒绝一切授权和联网更新，**未配置前不能发版**；首个带授权的版本需通过现有（旧）更新通道分发。
- `TestR122SGPSummaryHistoryEventCarriesAutofillCounts` 在 R232 账本中出现过一次偶发 TempDir 清理失败，属既有问题，未根治。
- R232 的 T03–T19 正式验收（真实服务端、两台 Windows 真机、产物篡改）仍需在服务端联调后完成。
- 复核时仓库 `dist/` 下留有临时文件 `.r232-review-src.tgz`（约 19 MB，已被 `.gitignore` 忽略，可直接删除）。
