# WORKLIST-R234：R233 复核 — Go 客户端接受大小写别名字段名，服务端共享协议向量未装入客户端（S15 未通过）

日期：2026-10-06。评估与工单：Claude。后续执行：GPT。核对基线：`deep-legends` HEAD `10933787`（含 R232/R233 未提交改动），`deep-legends-manage` 为未初始化 Git 的本地目录。版本保持 **0.12.75**。

**状态：问题已确认，未修复。本单只改客户端的严格 JSON 解码和共享向量安装，不改协议字段与签名域、不改版本、不发布。**

## 复核结论

**R233 的客户端修复（P1 时钟回拨容差、P2 授权文件写入容错）通过复核**：`licenseClockTolerance = 300s`，`retryLicenseFileWrite` 重试 4 次（0/50/200/800 毫秒），R233 的 9 个新用例和 R232 的授权管理器用例在独立环境加 `-race` 下全部通过。

**服务端 `deep-legends-manage` 的协议实现通过复核**：用独立 Node 脚本把共享向量 59 条逐条喂给服务端的 `normalizeCode`、`rawURLDecode`、`parseContractJSON`、`parseRequest`/`requestText`、`verifySignedEnvelope`，59/59 与向量期望一致（含大小写别名被拒绝）；服务端自己的 Worker 22 项 + UI/工具 4 项测试在其最终源码之后的运行记录为全部通过（我这边无法安装 workerd，未独立重跑，见下）。

**跨实现一致性测试发现缺陷（S15 不通过）**：把服务端最终向量原样放进客户端后，只有 3 条失败，其余 56 条（编码、请求签名文本、响应验签、各类签名错误）全部通过，说明两端的签名与编码在字节级一致。失败的 3 条正是服务端会话已发现的：

| 向量 | 内容 | 期望 | 客户端实际 |
|---|---|---|---|
| `json/case-alias-request` | 请求 envelope 字段名用大小写别名（如 `Version`） | 拒绝 | 接受 |
| `response/case-alias-envelope` | 响应 envelope 字段名大小写别名 | 拒绝 | 接受 |
| `response/case-alias-payload` | 响应载荷字段名大小写别名 | 拒绝 | 接受 |

另外发现：**客户端仓库里的 `backend/testdata/license-protocol-vectors.json` 仍是 1,273 字节的占位文件**（SHA-256 `ae032e72…3221d`），不是服务端交付的最终向量（`deep-legends-manage/test/license-protocol-vectors.json`，31,615 字节，SHA-256 `053eaa3841c73b15cd4f46c1c08e6386896be935d3622a7473515c4473d45c0e`）。这就是 R233 账本里 S15 被"SKIP"的原因，也是这个缺陷此前没被客户端测试发现的原因。

**复核的局限**：本次复核环境没有 Windows，没有 Cloudflare 访问，也装不了 workerd，因此没有独立重跑服务端 Worker 测试和客户端的 3 个依赖完整 `app` 的授权运行时用例（`TestR232LocalSideEffectsCheckAgain`、`TestR232HTTPAndLateResponseGate`、`TestR232LCUWriteAndInFlightCancellation`）。这些以各自账本中最终改动之后的运行记录为准。

---

## P1　`strictLicenseJSON` 必须按协议固定字段名精确匹配

**根因**：`backend/license.go` 的 `strictLicenseJSON` 先检查重复键，再用 `json.Decoder` + `DisallowUnknownFields()` 解码。Go 标准库解码对象字段时**大小写不敏感**，因此 `{"Version":1}` 会被当作 `version` 接受；`DisallowUnknownFields` 只拦截完全不匹配的名字，拦不住大小写别名。协议 v1 规定字段名固定，别名应拒绝。

**影响**：授权响应与载荷由服务端签名，攻击者无法借此伪造授权；但两端对「同一份字节」的解释出现分歧，违背协议的严格解析原则，也让共享向量测试无法通过，不能关闭 S15。

**修复要求**

1. 在 `strictLicenseJSON` 中对目标结构体做**精确字段名校验**：通过反射取目标类型的 `json` 标签名集合（含内嵌/嵌套结构体和切片元素类型），遍历原始 JSON 的每个对象键，键必须与标签名**逐字节相等**，否则返回错误。不要靠 `strings.EqualFold` 去重或放宽。
2. 该函数的所有调用点都要覆盖并保持行为一致：授权响应 envelope 与载荷、请求 envelope、本机授权状态文件、`/api/license/activate` 的输入，以及共享向量文件自身的容器解析。**不能因为容器类型不同而在个别调用点绕过精确匹配。**
3. 审计同样验证"服务端签名内容"的其他解码路径（如独立更新清单 `signed_manifest` 的解析、`updateTrustKeys` 相关代码），若存在同样的大小写别名接受行为，按同一方式修复并补用例；若无此问题，在账本中写明检查范围和结论。
4. 新增单元用例（不依赖向量文件）：`{"Version":1,...}`、`{"VERSION":1,...}`、同时出现 `status` 与 `Status`、嵌套对象里的别名、数组元素里的别名，全部拒绝；精确字段名的正常输入仍通过；本机授权状态文件仍能读回。

## P2　安装服务端最终共享向量并让 S15 真正运行

1. 把 `deep-legends-manage/test/license-protocol-vectors.json` **原样**复制到 `backend/testdata/license-protocol-vectors.json`（覆盖占位文件），复制后核对 SHA-256 为 `053eaa3841c73b15cd4f46c1c08e6386896be935d3622a7473515c4473d45c0e`；不得手改文件内容。
2. 取消 `license_protocol_vectors_test.go` 中因向量未就绪而 SKIP 的路径：向量文件 `ready` 必须为 `true`，条目为空或缺失类别时测试应**失败而不是跳过**。保留现有对"覆盖类别不得悄悄消失"的断言。
3. 验收：`TestR233ProtocolParserVectors`、`TestR233RequestSignatureTextContract`、`TestR233SharedSignedProtocolVectors` 全部通过，包含 `case-alias-request`、`case-alias-envelope`、`case-alias-payload` 三条；实跑输出中不得出现任何 `SKIP`。
4. 故意变异验证：在临时副本里把客户端响应验签的 domain 字符串改成 `DL-LICENSE-RESPONSE-MUTATED`，向量测试必须失败；把 `strictLicenseJSON` 的精确匹配改回旧行为，三条别名向量必须失败。结果记入账本，临时副本用后删除。
5. 在 `docs/license-protocol.md` 末尾补一句向量交接说明：向量来源、SHA-256、`fixture_version=1`；协议字段与签名域不得改动。
6. 向量里的测试私钥/种子只存在于测试夹具，不得进入运行代码、安装包或任何生产配置；`deep-legends-manage` 向量使用的测试公钥 kid `test-license-r233` **不得**写入 staging 或 release 的信任配置。

## P3　staging 公钥交接（等用户提供，GPT 不生成密钥）

- 用户在自己电脑上用 `deep-legends-manage` 的 `scripts/gen-signing-key.mjs` 生成 **staging** 签名密钥对后，只把 `kid` 和**公钥**（`.public.json` 内容）交给 GPT。GPT 写入 `backend/license_config_staging.go` 的 `licenseTrustKeys()`，并更新 staging 配置测试；**在拿到之前保持为空，不得用向量测试公钥代替**。
- release 配置的 `licenseTrustKeys()`/`updateTrustKeys()` 同样保持为空，直到用户提供生产公钥；账本继续标明「发布被阻塞」。
- 增加测试：staging 与 release 的信任配置都不得包含向量文件里的公钥。

## P4　`deep-legends-manage` 可选优化（不阻塞联调，可与 P1–P3 并行）

仅在 `deep-legends-manage` 内修改，不改协议：

1. **减少续租时的 D1 写入**：`license-do.ts` 的 `admit` 每次成功续租都会执行 `writeSummary`（一次 D1 `UPDATE`）。100 台设备每分钟一次约 14.4 万次写入/天。建议仅在占用/状态变化，或 `last_seen_at` 距上次写回超过 5 分钟时才写；这样管理页的"最后在线时间"精度降为 5 分钟，需在管理页和文档里说明。**占用裁决不得依赖该摘要。**
2. **限制未知设备的续租写入**：对没有绑定的设备，`renew` 会在 DO 里为其建一行 `bound=0` 的设备记录。给每个 license 的未绑定设备记录设置上限（例如 50 行，超出时覆盖最旧的未绑定记录），防止持有有效 `license_id` 的人撑大存储。补测试。
3. 以上改动后重跑 `deep-legends-manage` 的 Worker 全套测试，并更新其 `docs/execution-ledger.md`。

---

## 验收

- 客户端：`go test -count=1 ./backend`、`-race`、`go vet ./...`，以及 `node scripts/test-renderers.cjs all`、`desktop` 的 `node --test`，在最终改动之后实跑并保留末尾输出；S15 三个入口无 SKIP、无 FAIL。
- 在 `docs/r232-execution-ledger.md` 末尾追加 R234 一节：根因、修复、向量 SHA-256、变异验证结果、实际测试输出与时间；未完成项如实写。
- 不改版本号、不发布、不改协议字段与签名域。

## 仍待用户完成（不属于本单）

1. 按 `deep-legends-manage/docs/SETUP.md` 创建 staging 的 D1、Access 应用、DNS 与路由，生成 staging 签名密钥并写入 Secret，执行迁移后部署 staging。
2. 确认隐私声明草稿里的保存期限（事件 180 天、操作审计 365 天）和删除渠道联系方式（目前待用户提供），确认后才会写进客户端隐私文本。
3. 用 STAGING 联调构建在两台 Windows 真机上做 S16（换机、离线到期、管理员多机、服务不可达后恢复）。
4. 通过后，用户自行创建生产资源与生产密钥，在管理页生成首批 99 普通 + 1 管理员码，把生产公钥交给 GPT 写入 release 配置，再发布。
