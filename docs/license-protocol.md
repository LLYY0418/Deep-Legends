# Deep Legends 客户端授权协议 v1

日期：2026-10-07。协议版本：`1`（尚未发布，R242 新增到期字段及 EXPIRED，R246 在 v1 内将租约上限改为7200秒）。本合同由本仓库按用户本次指示先行提出，供独立项目 `deep-legends-manage` 对齐；替代 R232 中“服务端先产出协议”的实施顺序。本仓库不实现服务端、码表或管理页。

## 固定业务规则

- 首批共 100 码：99 个 `standard`、1 个 `admin`；客户端不下载或内置码表。
- `standard` 最后一次明确激活的设备立即生效；旧设备下一次续租收到 `REPLACED`。无交接等待、无 `ack-stop`。
- `admin` 不限制设备、并发和激活次数；其他设备激活不会替换本机；可设置业务到期日，仍使用短租约。
- 租约最多 7200 秒（2小时），正常心跳间隔 60 秒，请求超时 15 秒，激活只重试一次、总预算35秒；过期缓存启动失败按3/6/12/15/30/60秒重试，正常临时失败按 15/30/60 秒重试，不能延长已有租约。
- 未过期的本机有效租约允许离线启动；同时立即续租。首次激活联网；`REPLACED`/`REVOKED`/`EXPIRED` 不自动激活或恢复，只有用户重新提交码。
- 旧设备离线时可用到已签发租约到期；在线检测包含请求耗时，不承诺网络故障时仍在 60 秒内接收通知。

## 地址与传输

生产 origin 固定为 `https://license.yinxiaobia.net`，仅有两个客户端接口：`POST /v1/activate` 和 `POST /v1/renew`。客户端不访问管理主机或 `/admin`。HTTPS，禁止重定向；JSON UTF-8，响应 `Cache-Control: no-store`，响应体上限 32 KiB。注册码、设备字段和签名不出现在 URL。测试通过构造器注入内存/httptest transport 与测试公钥；独立联调构建见下文。发布程序无地址覆盖、免授权环境变量或测试密钥。

## 编码

- Ed25519 公钥 32 字节、签名 64 字节，均为无填充 Base64URL（RFC 4648 URL-safe alphabet，拒绝 padding/非规范编码）。
- `device_pub_hash` 为公钥原始 32 字节的 SHA-256，小写 64 位十六进制。
- 时间为 Unix 秒整数；`counter` 为正 uint64 的十进制字符串（无前导零），避免 JavaScript 的整数精度丢失；`revision` 为正安全整数，最大 `9007199254740991`。
- JSON 不依赖对象属性顺序。对原始载荷字节的签名覆盖其 Base64URL 编码，不做 JSON 重新序列化后验签。拒绝重复字段、任何未知字段/协议版本、尾随 JSON 与超限字段。
- 注册码显示为 `DL-XXXXX-XXXXX-XXXXX-XXXXX`。规范化：移除可选开头 `DL`、空格与 `-`，转大写，`O→0`、`I/L→1`，之后必须恰好 20 个 Crockford Base32 字符（不含 U）；向服务端发送规范化后的 20 字符。客户端不判定码类型。

## 请求 envelope

```json
{"version":1,"request_id":"随机128bit十六进制","device_pub":"Base64URL公钥","ts":1791244800,"counter":"42","payload":"Base64URL载荷JSON","signature":"Base64URL设备签名"}
```

`request_id` 每个逻辑请求随机生成，重试使用新计数器；客户端将计数器原子持久化后才发请求，跨重启递增。一次只发一个激活/续租请求，旧响应不能覆盖后发请求的状态。失败也不回滚计数器。

激活载荷：`{"code":"20字符规范化码","client_version":"0.12.75"}`。

续租载荷：`{"license_id":"服务端opaque ID","revision":1,"client_version":"0.12.75"}`。`license_id` 只来自已验签租约；续租绝不包含或重新使用注册码。服务端需允许合法设备在租约过期后续租，否则网络故障无法自动恢复。

设备签名的字节严格为以下 UTF-8 文本，字段以 LF 分隔，末尾没有 LF：

```text
DL-LICENSE-REQUEST-V1
POST
/v1/activate（或 /v1/renew，实际路径，无括号文字）
request_id
device_pub
ts 的十进制
counter
SHA256(payload 解码后的原始字节)的小写十六进制
```

服务端验证设备签名、时间窗 ±300 秒，并按 `(license_id, device_pub_hash)` 原子验证 `counter > last_counter`，在成功接纳请求时更新计数器。激活先通过码摘要定位 license，之后执行同一检查。相同设备重复激活不能自踢；网络重试不能造成同设备多次变更 revision。普通码换设备才递增占用 revision，旧设备不能以续租改变占用者。管理员按各设备独立维护计数器/会话；服务端管理事件或签名轮换不由客户端控制。

## 响应 envelope 与服务端签名

所有可被客户端当成权威结论的成功和业务错误必须签名：

```json
{"algorithm":"Ed25519","kid":"服务端公钥ID","payload":"Base64URL载荷JSON","signature":"Base64URL服务端签名"}
```

服务端签名覆盖 UTF-8 文本（末尾没有 LF）：

```text
DL-LICENSE-RESPONSE-V1
kid
payload 的原样Base64URL文本
```

`kid` 必须为 1–64 个可见 ASCII 字符（0x21–0x7e，不含空格或换行）；客户端只信任随代码内置的 `{kid: public_key}`，最多同时认当前/下一把公钥。算法固定 Ed25519，无 `none`，无从响应下载公钥或证书的机制。公钥不是秘密，生产私钥始终由用户保管；本仓库测试密钥只存在 `_test.go` / 测试夹具，不进入运行代码或安装包。生产公钥未配置时客户端保持锁定，不回退测试信任根。

成功载荷：

```json
{"version":1,"request_id":"回显请求号","device_pub_hash":"设备摘要","server_time":1791244800,"status":"ACTIVE","license_id":"opaque ID","kind":"standard","revision":1,"issued_at":1791244800,"expires_at":1791252000}
```

必需校验：version=1、请求号匹配、设备匹配、status=ACTIVE、kind 仅 standard/admin、license_id 非空、revision 正整数、`issued_at <= server_time < expires_at`、`expires_at-issued_at <= 7200`，及固定字段范围。有到期日的 ACTIVE 载荷增加可选 `license_expires_at`：Unix 秒正整数，不能为 null、字符串或小数，且不早于 `issued_at`；永久码省略该字段。字段包含在原始 payload 签名中，租约 `expires_at` 不得晚于 `license_expires_at`，可以小于 7200 秒。已验签续租被业务到期日截短时按新截止时间处理，不延长为 7200 秒。缓存保存原始 signed envelope，可离线复验；缓存同时保存最近服务器时间、观察到的墙钟高水位、终止状态及计数器，受本机 DPAPI 保护。具体时间与本地写入处理如下；不改变任何协议字段或签名域。

### 时间与本地持久化（R233）

- 回拨容差为 **300 秒**（命名常量 `licenseClockTolerance`）。启动时对最近服务器时间和本地墙钟高水位减去该容差；运行时对本地高水位采用相同容差。≤300 秒保留有效会话，>300 秒锁定并立即唤醒续租，成功后恢复。300 秒用于吸收时间同步、休眠恢复等小幅校正，远小于 7200 秒租约上限。
- 运行截止时间以本次请求发出时的单调时钟加 `expires_at-server_time` 保守计算；网络耗时消耗剩余时间。小幅墙钟回拨不改此截止时间，也不取消业务 context；休眠后按墙钟与单调截止时间较早者到期，运行期间单份租约最多使用 7200 秒。
- 缓存另存本地租约检查点 `lease_wall_time` / `lease_remaining`，仅属于 DPAPI 文件，**不是请求/响应字段**。启动剩余时间为 `min(7200, expires_at-server_time, lease_remaining) - max(0, 当前墙钟-lease_wall_time)`；旧缓存无检查点时按 `min(7200, expires_at-max(当前墙钟, server_time))` 保守回退。计数器保存或失败请求不能刷新租约检查点。本机比服务器慢 60 秒仍可离线重启。跨进程没有原进程单调时钟，容差内回拨可能使离线重启多得约 300 秒，但单次重启授予的剩余时间仍≤7200秒；本地快照回滚与反复时钟操纵不属于硬件级保证。
- 单文件原子写入先尝试一次，失败后再重试三次，间隔 50/200/800 毫秒；每次清理临时文件。请求前保存计数器失败则不发请求，按 15/30/60 秒退避，当前有效租约继续，到期进入 `NETWORK_LOCKED`。成功租约保存失败仍使用已验签内存租约，下一次心跳重新保存；磁盘旧租约不会获得额外时长。
- `REPLACED` / `REVOKED` 保存失败仍立即取消业务、锁定并清内存缓存，保持终态；仅按 15/30/60 秒重试写入，不自动续租。若进程在终态真正落盘前退出，磁盘可能仍有旧租约，重启需在线权威核验，无法承诺磁盘不可写时终态永久保存。`DEVICE_ERROR` 保留给不可用设备身份，普通运行中写入故障不进入该状态。
- 写入失败/恢复诊断只记录 `license_store_write` 的结果和阶段（counter/lease/terminal/terminal_retry）；大幅回拨记录 `license_clock`。不记录存储错误原文、注册码、设备私钥或租约原文。

错误载荷：

```json
{"version":1,"request_id":"回显请求号","device_pub_hash":"设备摘要","server_time":1791244800,"status":"ERROR","error":"REPLACED"}
```

客户端校验签名与请求/设备绑定后才采纳业务错误。未签名 4xx/5xx、HTML、超时、证书错误或无效签名都仅表示请求失败；已有有效租约可继续到期，绝不因失败延长期限。服务端错误原文不展示或记入诊断。

| 错误码 | 建议 HTTP | 客户端行为 |
|---|---:|---|
| INVALID_CODE | 400 | 输入框提示注册码无效；不保存明文码 |
| REPLACED | 409 | 立即门禁、持久化被替换状态并清缓存租约；显示「注册码已被其他地方使用」；不自动 renew/activate |
| REVOKED | 403 | 立即门禁并清缓存租约；显示「注册码已停用」；不自动激活 |
| EXPIRED | 403 | 沿用本地 REVOKED 终态，持久化 terminal_reason=EXPIRED；立即门禁并清签名租约，重启仍显示「注册码已过期」；续期或恢复网络均不自动解锁，需明确重输码 |
| REPLAY | 409 | 拒绝该响应放行；不降低本地计数器，需新的明确请求 |
| TIMESTAMP_INVALID | 400 | 签名错误响应中的 server_time 可修正下次请求时间，不延长租约；只允许一次立即重试 |
| RATE_LIMITED | 429 | 退避，租约内继续；不得解释为管理员次数限制 |
| SERVICE_UNAVAILABLE | 503 | 退避，租约内继续；到期 NETWORK_LOCKED |
| UNSUPPORTED_VERSION | 426 | 拒绝新租约，提示激活服务暂不可用；更新仍可访问 |

本地状态：`LOCKED`、`ACTIVE`、`NETWORK_LOCKED`、`REPLACED`、`REVOKED`、`DEVICE_ERROR`。本地接口 `GET /api/license/status` 仅给状态、必要中文文案、授权代际及已验签租约的可选 `license_expires_at`；`POST /api/license/activate` 接收 `{code}`，仍要求本地 session token。注册码不从这两个接口返回。授权失败时后台门禁先关闭，再通知界面。客户端内存中的授权代际用于丢弃在途业务响应，不作为服务端 revision。

## 第二阶段与独立更新签名

P4b relay 授权是第二阶段，本次不改 relay。未来 relay token 应使用独立 domain/audience（`deep-legends-relay-v1`），禁止把本协议 lease 直接当管理凭据；其字段与联调另行升级合同。

更新签名使用与授权不同的 Ed25519 key/kid 信任表。更新 `latest.json` 保留现有 schema=1 载荷，同时增加 `signed_manifest`：envelope 字段同上，signature domain 为 `DL-UPDATE-MANIFEST-V1\n<kid>\n<payload>`，payload 是完整原始清单 JSON 字节，**不包含 signed_manifest 本身**。客户端以验签后的 payload 为唯一下载/执行依据，外层字段必须与签名字段一致；拒绝无签名/未知公钥、混装版本、错误 SHA256/URL。无生产更新公钥时拒绝联网更新放行，不使用许可公钥或测试公钥兜底。生产清单签名及私钥保管由发布环境后续对齐，本轮不发版。

## 数据处理与验收边界

服务端收到用户提交的单个码、设备公钥与摘要、版本、时间/计数器及续租标识；不接收硬件序列号、电脑名、游戏账号、Riot Key、收藏。客户端保存 DPAPI 保护的设备私钥、计数器和签名租约，不保存明文注册码。服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。本期不提供删除渠道，不对外承诺删除流程。

假服务器测试用于验证协议和客户端状态机，不代表 Cloudflare 存储一致性、真实服务端或两台 Windows 真机通过。生产签名公钥、服务环境、码池、真实双机以及实际 Windows 产物篡改验收未就绪时，R232 保持未完成。协议字段或签名域变更必须同步两个项目；R242 按未发布 v1 的工单在 v1 内增加字段与错误码，其他升级依合同进行，不静默兼容另一种签名文本。

## R233 构建隔离与共享测试向量

默认构建使用 `backend/license_config_release.go`；`-tags=license_staging` 单独使用 `license_config_staging.go`，联调 origin 为 `https://license-staging.yinxiaobia.net`。R236 已安装用户提供的 staging 授权公钥（kid `staging-2026-10`），仅这一把；release 授权及独立更新信任表仍为空，等待用户提供生产公钥，**发布被阻塞：缺生产公钥**。staging 独立更新信任表也为空，且构建期关闭在线更新：不读取正式更新缓存/设置、不访问正式 Latest、不提供正式发布页链接，手动检查、下载、应用均拒绝；未来配置更新公钥也不能绕过该构建政策。

手动执行 `node scripts/build-license-staging.cjs`，或指定 `--output <末尾为-staging-public的空目录>`，在独立临时源码目录生成自研 Windows Setup 和目录联调包。窗口标题、快捷方式及卸载显示名为 `Deep Legends-STAGING`，安装包名为 `Deep-Legends-Setup-<版本>-staging-public.exe`，目录带 `-staging-public`，key mode 为 public；仅保存本地产物，electron-builder 显式 `--publish=never`，不创建 tag/Release、不更新 latest.json。安装壳继续沿用内置 `Deep Legends.exe`/`Deep Legends` 目录合同，联调安装时选择独立目录。正式脚本显式清空 Go 构建标签，beforePack、最终发布构建记录和 CI 检查正式后端中不含 staging 域名、公钥及测试 kid/公钥（同时检查文本与原始公钥字节）。staging 公钥的无填充 Base64URL 字面值自动纳入正式包的排除列表。

共享夹具路径为 `backend/testdata/license-protocol-vectors.json`，不进入 Go 内嵌资源或安装包。R233 执行时为 `ready:false` / `source:pending-deep-legends-manage`，当时共享签名测试跳过；R234 已安装下述最终向量，三个客户端 S15 入口现在都要求完整就绪，缺失必须失败。服务端用测试密钥生成后复制相同文件到两仓库，不能用客户端自签结果冒充跨仓库核验。

夹具容器版本 `fixture_version:1` 与协议 `protocol_version:1` 独立于网络 envelope；容器格式为：

| 字段 | 含义 |
|---|---|
| ready / source | 服务端向量就绪后为 true；source 写独立项目提交或生成来源 |
| public_keys | `{kid: 无填充Base64URL的32字节测试公钥}`；仅测试夹具 |
| device_seed_hex | 32 字节固定测试设备种子的 64 位 hex；正向请求共用此测试设备，不是配置公钥或生产私钥 |
| normalization | `{id,input,output,valid}` 注册码规范化正反例 |
| base64url | `{id,input,size,bytes_hex,valid}` 规范编码与 padding/非法位/字符反例 |
| strict_json | `{id,kind,json,valid}`；kind 为 request/payload/envelope，json 为原始 JSON 文本，覆盖重复字段、未知字段、尾随内容、错误类型 |
| requests | `{id,path,json,text,valid}`；json 为原始设备请求，text 为确切签名文本，无末尾 LF。正向必须包含两个端点，客户端用给定测试种子复算并验证签名结果 |
| responses | `{id,json,request_id,error,payload,category,valid}`；json 为原始响应 envelope，正向 payload 为完整预期载荷 JSON 文本、error 为预期错误码（ACTIVE 为空）；反向 category 表示拒绝类别 |

就绪门槛：两个请求端点、standard/admin 成功租约、全部九种签名错误（含 EXPIRED）；拒绝类别至少包含 `noncanonical_base64url`、`duplicate_json`、`invalid_kid`、`unknown_kid`、`bad_signature`、`binding`。测试使用生产验签/严格解析函数，逐字段比较成功与错误载荷；缺覆盖时失败。测试种子、公钥与签名只存在测试文件中；公开构建扫描测试公钥/kid，不能将其加入生产信任表。

R234 向量交接：原样复制自 `deep-legends-manage/test/license-protocol-vectors.json`，SHA-256 为 `053eaa3841c73b15cd4f46c1c08e6386896be935d3622a7473515c4473d45c0e`，`fixture_version=1`、共 59 条；测试 kid `test-license-r233` 及公钥/设备种子仅用于夹具，不能进入 staging/release 信任配置；协议字段与签名域不变。

R242 向量交接：仅读取用户指定的共享向量文件，原样复制后 SHA-256 为 `5f3fee3c89790cffdcb37f651c4333afb18e0f65f7247b1ac5fdda5dc458b89b`，共 68 条（9 规范化、5 Base64URL、11 严格 JSON、10 请求、33 响应）。增加 EXPIRED、注册码到期与截短租约正例及到期字段反例。R236、R239、R240 旧 STAGING 包不认识这些新字段/错误码，会报 invalid 或 unknown error，不再用于联调；包文件保留。

R246 向量交接：核对用户提供的会话B交接SHA后按字节保留，SHA-256 `39377b860440d8f609f9d28162afe086e4e01ac91d3d2ff39da2a45d5eebfcc7`，共69条（9/5/11/10/34），覆盖7200秒接受、7201秒拒绝、业务到期截短。字段、签名域、错误码不变。旧R243及更早STAGING客户端拒绝7200秒租约；服务端部署与安装R246新客户端需同步进行，本会话不部署。
