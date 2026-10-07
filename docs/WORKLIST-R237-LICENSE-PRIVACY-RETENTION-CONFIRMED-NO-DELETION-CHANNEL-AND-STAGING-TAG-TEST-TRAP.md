# WORKLIST-R237：授权隐私文本按已确认的保存期限定稿（无删除渠道），并修 staging 标签下的全量测试陷阱

日期：2026-10-06。评估与工单：Claude。执行：GPT。版本保持 **0.12.75**，不发布，不改协议字段与签名域。

**用户已确认（2026-10-06）**：事件保存期限按建议执行——`events` 保留 180 天，`admin_audit` 保留 365 天；**不提供删除渠道**，不要求联系方式，GPT 不得编造任何邮箱、网址或“删除流程”。

**重要：正在进行 S16 真机联调，联调用的 STAGING 包（SHA-256 `a33b60ba4434a25f1a77f5f08381d12ff0673e6f208659236677c9f98002b7f5`）保持不动。本单的改动不得重新构建、覆盖或删除 `dist/R236-staging-public/` 下的任何文件。** S16 通过后由用户另行通知再构建新的候选包。

## 背景核对（Claude 已读到的现状）

| 位置 | 现状 |
|---|---|
| `backend/features.go:1179` `licenseDisclosure` | 末尾写“服务端授权记录的保存期限和删除方式由授权发放方在正式启用前提供。”——这是待定占位 |
| `installer/ui/license.html` 第 8 行 | 同样的占位句 |
| `docs/license-protocol.md` 第 117 行（客户端）与 `deep-legends-manage/docs/license-protocol.md` | 写着“保存期限与删除渠道需……上线前定稿” |
| `deep-legends-manage/docs/privacy-disclosure.md` | 标题“隐私披露草案（待确认）”；events 180 天、admin_audit 365 天仍标“政策待确认”；删除渠道写“待用户提供” |
| `deep-legends-manage/src/index.ts` 第 17–18 行 | `cleanup` 已按 180 天 / 365 天删除，由 `scheduled` 每天执行 |
| `backend/license_test.go` `TestR232NormalizeAndFailClosed` | 末尾断言 `licenseTrustKeys()` 必须为空；在 `-tags license_staging` 下 staging 有一个 key，整套测试会失败 |

## 定稿措辞（两个仓库的所有文本以此为准，不得各自改写）

用下面这句**替换**客户端两处占位句“服务端授权记录的保存期限和删除方式由授权发放方在正式启用前提供。”：

> 服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。

要求：
- 不写“可通过……联系删除”“联系作者删除”等任何删除渠道表述，也不写“无法删除”。**不提及删除方式**。
- 不新增“统计口径/方法论/免责声明”类解释文字；只替换这一句，不扩写。
- `admin_audit` 的 365 天保存的是管理员操作审计（含管理员的 Access 邮箱），不属于用户数据，**不写进客户端文本**，只写进 `deep-legends-manage/docs/privacy-disclosure.md`。

---

## 会话 A（仓库 `deep-legends`）：P1、P2

### P1　客户端文本定稿

1. 在 `backend/features.go` 的 `licenseDisclosure` 与 `installer/ui/license.html` 第 8 行，把占位句换成上面的定稿措辞，其余文字不动。
2. `docs/license-protocol.md` 第 117 行改为与定稿一致的说法：事件保留 180 天；授权记录与防重放计数器持续保留；删除渠道本期不提供，**不写成承诺**。
3. 搜索是否还有测试或文案断言旧占位句“由授权发放方在正式启用前提供”；有则一并更新，没有就在账本写“已搜索，无引用”。补一个测试：`licenseDisclosure` 与 `installer/ui/license.html` 都包含定稿句，且都不包含“由授权发放方”“删除方式”“联系”字样。变异验证：把任一处改回旧占位句，该测试必须失败。
4. 授权遮罩页 `/api/privacy` 的展示（`backend/web/license-ui.js`）读取的就是 `licenseDisclosure`，确认无需再改；如有单独缓存或快照测试，一并更新。

### P2　`TestR232NormalizeAndFailClosed` 的构建陷阱

1. 该测试里“`len(licenseTrustKeys()) != 0 || len(updateTrustKeys()) != 0` 即失败”只对 release 构建成立。把这两条断言从通用测试里移出，只放在 `//go:build !license_staging` 的测试里（`license_config_release_test.go` 已有同类断言则复用，不重复）；通用测试保留其余的规范化与“空信任表不允许激活”逻辑（它用 `m.options.Keys = map...{}` 显式清空，与构建无关）。
2. 验收：分别在默认标签和 `-tags license_staging` 下运行 `go test -count=1 -race -run 'R232|R233|R234|R236' ./backend`，**两种标签都通过，无 SKIP**。变异验证：把 release 配置塞入一个 key，release 测试必须失败；staging 配置清空，staging 配置测试必须失败。
3. CI 的 staging 步骤保持现有三个点名用例，不要改成宽匹配，除非同时让 `scripts/verify-ci-test-filters.cjs` 通过并在账本说明理由。

### 会话 A 的边界与验收

- 不重新构建 STAGING 包，不动 `dist/R236-staging-public/`；不改版本、不发布、不改协议。
- 在最终改动后实跑：`go test -count=1 ./backend`、`-race`、`go vet ./...`、`node scripts/test-renderers.cjs all`、`desktop` 的 `node --test`，保留最后 5 行。若 `desktop/refresh-orchestration.test.cjs` 的 “dirty collection rescans …” 再次失败，先单独连续运行 3 次并记录结果，再报告，不要改该测试或业务代码。
- 账本：`docs/r232-execution-ledger.md` 末尾追加 R237 一节，没做的如实写。

---

## 会话 B（仓库 `deep-legends-manage`）：P3

仅改文档，并核对保留期限的测试覆盖，**不改协议、不改业务代码，不部署**。

1. `docs/privacy-disclosure.md`：标题去掉“（待确认）”，改为“隐私披露（保存期限已确认，无删除渠道）”，日期 2026-10-06。表格中 events 写“180 天后由 scheduled 删除（已确认）”，admin_audit 写“365 天后由 scheduled 删除（已确认）”；licenses 写“持续保留，用于授权校验”；LicenseDO 的 counter 写“持续保留，用于防重放”；RateLimitDO、日志两行保持原样但把“政策待确认”改成实际行为说明（约两分钟清桶；模板关闭持久日志，临时排障日志用后关闭）。删除渠道一栏改写为：“本期不提供删除渠道，不对外承诺删除流程”，并**保留**“当前 API 只删除从未激活的码，已激活的码只撤销”这一事实说明。
2. 文档里引用的客户端措辞必须与上面“定稿措辞”逐字一致。
3. `docs/license-protocol.md`、`docs/OPERATIONS.md`、`README.md`、`docs/SETUP.md` 中若还有“保存期限待定/删除渠道待用户提供”的句子，同步改成上面的结论；协议字段与签名域、协议副本的 SHA-256 **不得变化**（`docs/license-protocol.md` 若已含协议副本，只改末尾“隐私”类说明，核对改动前后协议部分的哈希一致并写进账本）。
4. 核对 `src/index.ts` 的 `cleanup`（180 / 365 天）已有测试覆盖边界：179 天与 181 天（以及 364/366 天）的记录，保留与删除结果正确。**已有则在账本里引用测试名与最后一次实跑输出，没有则补测试**，并做变异验证（把 180 改成 18，测试必须失败）。
5. 实跑 Worker 全套测试与 UI/工具测试，保留最后 5 行输出；更新 `docs/execution-ledger.md` 追加 R237 一节。

## 不在本单范围

- 管理端优化（续租时 D1 写入限频、未绑定设备记录封顶）：S16 通过之前不改动服务端，由用户之后决定。
- 构建新的 STAGING 或 release 候选包、生产公钥填写、生产资源与注册码生成、发布。
- 任何真实注册码、私钥或 Cloudflare 凭据，GPT 一律不接触。

## 仍待用户完成

1. 两台 Windows 上按 `docs/r232-execution-ledger.md` 的 R236 清单做 S16，并把结果告诉 Claude。
2. S16 通过后：创建生产资源与生产密钥，生成首批 99 个普通码 + 1 个管理员码，把生产公钥（kid + public_key）交给 GPT 写入 release 配置，再发布。
