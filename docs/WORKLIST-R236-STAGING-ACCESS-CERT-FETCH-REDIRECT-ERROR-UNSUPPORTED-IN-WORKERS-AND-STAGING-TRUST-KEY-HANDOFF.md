# WORKLIST-R236：staging 联调中发现服务端取 Access 公钥用了 Workers 不支持的 `redirect:'error'`，并交接 staging 公钥、构建联调包

日期：2026-10-06。诊断：Claude（根据用户 staging 实测与本机脚本对照）。执行：GPT。版本保持 **0.12.75**，不发布，不改协议字段与签名域。

**与 R234 的关系**：R234（客户端大小写别名字段名、共享向量）仍然有效，**先执行 R234，再执行本单的 P2/P3/P4**；本单 P1 在 `deep-legends-manage`，与 R234 互不依赖，可并行。

## 现象与根因（已确认）

用户把 `deep-legends-manage` 部署到 staging（`manage-staging.yinxiaobia.net`），Access 登录成功后，管理页返回 `{"error":"FORBIDDEN"}`。逐项排查：

1. 用本机脚本按服务端同样的规则检查用户的 Access 令牌：iss、aud、exp、邮箱、公钥、签名**全部通过**。
2. 在本机 Node 里直接运行服务端真实的 `accessEmail`（`src/access.ts`）：**接受**令牌。
3. 线上仍拒绝。差别只剩运行环境：`src/access.ts` 第 9 行 `fetch(\`${issuer}/cdn-cgi/access/certs\`, {redirect:'error', ...})`。**Node 支持 `redirect:'error'`，Cloudflare Workers 只接受 `follow` 与 `manual`**，线上这行直接抛异常，被外层 `catch` 吞掉返回 `null`，于是一律 `FORBIDDEN`，日志里也没有原因。
4. 用户把这一行改成 `redirect:'manual'` 并重新部署 staging 后，管理页正常（已成功生成一个普通码和一个管理员码）。第 10 行 `if(!response.ok) throw` 保留：`manual` 下 3xx 的 `ok` 为 false，仍被拒绝。

**为什么测试没发现**：服务端自带测试把 `fetch` 整个模拟掉，没有经过 Workers 对请求参数的校验。

**注意**：`src/access.ts` 第 9 行的修改是用户在本机手工做的，以其当前内容为基线，**不要还原**。

---

## P1　`deep-legends-manage`：补回归测试、审计其他 Workers 不兼容用法、加失败原因日志

仅改 `deep-legends-manage`，不改协议。

1. **回归测试（必须用真实平台校验）**：在 Worker 测试池（workerd）里，把 `fetch` 模拟成 `(input, init) => { new Request(input, init); return new Response(certsJson) }`，即让 workerd 自己校验 `init`，再返回构造好的公钥 JSON。用真实签名的测试令牌调用 `accessEmail`，期望接受。变异验证：把 `redirect:'manual'` 改回 `'error'`，该测试必须失败。结果记入账本。
2. **补一条静态守卫**：测试里扫描 `src/**/*.ts` 的 `fetch(`、`new Request(`、`new Response(` 的第二个参数，出现 `redirect:` 时取值必须是 `follow` 或 `manual`；同时列出 `src` 里其他 `fetch` 参数（`cache`、`signal`、`cf` 等），逐个确认 workerd 支持。
3. **审计 `src` 里其他可能在 Node 通过、在 workerd 失败的用法**（例如依赖 Node 内置模块、`AbortSignal.timeout`、`crypto.subtle` 的参数组合、`structuredClone`、`TextDecoder` 选项），结论写进账本：检查范围、每一项的结论。若发现真实问题，在本单内修复并补测试。
4. **失败原因日志**：`accessEmail` 每个返回 `null` 的分支，通过 `console.warn` 输出一行结构化 JSON，只含固定原因码，例如 `config_invalid`、`header_missing`、`token_malformed`、`claims_invalid`（附 `iss`/`aud`/`exp`/`nbf`/`email_type` 之一）、`email_not_allowed`、`kid_not_found`、`signature_invalid`、`certs_unavailable`。**不得记录令牌、邮箱、请求头内容**。外层 `catch` 也要记录 `exception` 与错误类型名（不含消息体里可能带的敏感内容）。补测试：每个原因码对应一个用例，且断言日志里不含令牌与邮箱字符串。
5. 不要部署到 Cloudflare，不要读取任何 Secret；部署由用户自己执行。更新 `deep-legends-manage/docs/execution-ledger.md` 与 `docs/SETUP.md`：写明“staging 管理页返回 FORBIDDEN 时，先看 Worker 日志里的原因码”，并把 Zero Trust 里“登录方式需先添加一次性 PIN、策略邮箱必须与 `ADMIN_EMAILS` 一致”的注意事项补进去（界面用中文名称：集成 → 身份提供程序、访问控制 → 应用程序）。
6. 重跑 Worker 全套测试与 UI/工具测试，记录最终改动之后的输出。

## P2　客户端：填入 staging 签名公钥（用户已提供，GPT 不生成、不索取私钥）

用户在自己电脑上生成并保管私钥，只交付以下公钥元数据，来自 `license-staging.public.json`：

```json
{
  "purpose": "license",
  "kid": "staging-2026-10",
  "algorithm": "Ed25519",
  "public_key": "E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE"
}
```

1. 写入 `backend/license_config_staging.go` 的 `licenseTrustKeys()`，沿用现有结构。写入前校验 `public_key` 为 base64url（无填充）且解码后恰好 32 字节。
2. **release 配置保持为空**（`license_config_release.go` 的 `licenseTrustKeys()`、`updateTrustKeys()` 不动），账本继续标明“发布被阻塞：缺生产公钥”。
3. 新增测试：staging 配置只含这一个 kid 且公钥与上面一致；release 配置不含任何 key；staging 与 release 配置都不得包含共享向量里的测试公钥（kid `test-license-r233`）。
4. 变异验证：把 staging 公钥改一个字符，对应测试必须失败。

## P3　构建 STAGING 联调包（不发布）

1. 使用 `license_staging` 构建标签，授权服务地址为 `license-staging.yinxiaobia.net`。产物文件名必须带 `-staging`（例如 `Deep-Legends-Setup-0.12.75-staging.exe`），版本号保持 0.12.75。
2. **只作为本地或 CI 产物保存，不创建 tag，不创建 GitHub Release，不更新 `latest.json`，不得被设为 Latest**。
3. **核查在线更新**：STAGING 包安装后，更新检查不能把它升级成不带授权配置的正式包，也不能指向正式 Latest。如果现有更新机制会这样，STAGING 构建必须关闭在线更新或指向空清单，并在账本里写明做法与验证。
4. 核查：STAGING 包内置的授权地址、公钥 kid 与 P2 一致；包内不含个人 Riot Key、任何私钥、真实注册码；release 构建（无 `license_staging` 标签）里没有 staging 地址与 staging 公钥。写入账本，附构建输出和 SHA-256。
5. 如果本机无法构建 Windows 安装包，按项目现有流程在 CI 上构建，并标明不是正式发布流程。

## P4　写好 S16 真机清单（用户在两台 Windows 上执行）

GPT 把下面场景整理成可照做的清单，写进 `docs/r232-execution-ledger.md` 的 R236 一节，每项写“操作 → 预期现象 → 通过/失败填写处”。用户持有两个 staging 测试码（一个普通码、一个管理员码），**注册码内容不得写进任何文档、账本或聊天**。

1. 机器 A 输入普通码激活成功，重启后保持已授权。
2. 机器 B 输入同一个普通码：B 立即授权成功；A 在下一次续租（约 1 分钟内）出现遮罩“注册码已被其他地方使用”，且不会自动恢复。
3. 机器 A 再输入同一码重新接管：A 成功，B 被遮罩。
4. 管理员码在 A、B 同时激活，两台都保持可用，管理页显示设备数增加。
5. 管理页停用某个普通码：对应设备在下一次续租后进入“已停用”遮罩；启用/重置后行为符合协议（重置后旧设备为已被替换）。
6. 断网：租期（≤15 分钟）内仍可使用；超过租期出现“网络不可用”遮罩；恢复网络后在续租成功时自动恢复，且仅限网络类锁定。
7. 系统时间向前或向后调整超过 5 分钟：行为符合 R233 的时钟回拨规则，不误锁正在使用的会话。
8. 管理页在 staging 上生成码时只显示一次、列表只显示标识与摘要，不出现完整码。

## 验收

- 服务端：P1 全部测试通过，变异验证有记录；客户端：`go test -count=1 ./backend`、`-race`、`go vet ./...`、`node scripts/test-renderers.cjs all`、`desktop` 的 `node --test` 在最终改动后实跑，保留末尾输出。
- 账本：`docs/r232-execution-ledger.md` 追加 R236 一节；`deep-legends-manage/docs/execution-ledger.md` 追加对应一节。写清根因、修复、审计范围与结论、STAGING 包文件名与 SHA-256、未完成项。
- 不改版本、不发布、不改协议、不部署 Cloudflare、不接触任何私钥或真实注册码。

## 仍待用户完成（不属于本单）

1. 在 staging 上部署 P1 之后的版本，确认管理页与日志原因码正常。
2. 用 STAGING 包在两台 Windows 上做 S16，结果告知 Claude 复核。
3. 事件保存期限（草稿 180 天，操作审计 365 天）与删除渠道联系方式的确认。
4. S16 通过后，用户自行创建生产资源与生产密钥，在管理页生成首批 99 普通 + 1 管理员码，把**生产公钥（只要 kid 和公钥）**交给 GPT 写入 release 配置，再发布。
