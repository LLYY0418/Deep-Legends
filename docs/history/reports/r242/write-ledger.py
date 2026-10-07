import hashlib
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[4]
OUT = ROOT / 'docs/history/reports/r242'
LEDGER = ROOT / 'docs/r232-execution-ledger.md'
receipt = json.loads((ROOT / 'dist/R242-staging-public/staging-build.json').read_text())
names = ['go-test-final', 'go-race-final', 'vet-final', 'license-default-final',
         'license-staging-final', 'renderers-final', 'desktop-final', 'browser-pass',
         'staging-build', 'package-audit', 'fingerprint']
rows = [json.loads((OUT / (name + '-summary.json')).read_text()) for name in names]
assert all(r['exit_code'] == 0 for r in rows)
assert all(r.get('skip', 0) == r.get('fail', 0) == 0 for r in rows[:7])
preservation = json.loads((OUT / 'artifact-preservation.json').read_text())
assert all(r['unchanged'] for r in preservation.values())
source = json.loads((OUT / 'final-source-files.json').read_text())
assert all(hashlib.sha256((ROOT / r['file']).read_bytes()).hexdigest() == r['sha256'] for r in source)
assert hashlib.sha256(pathlib.Path(receipt['setup']).read_bytes()).hexdigest() == receipt['setup_sha256']

text = LEDGER.read_text()
assert '\n## R242 ' not in text, 'R242 section already exists'
old = '**R240 更新：R239 与 R236 包均已作废，S16 统一改用 `dist/R240-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe`，SHA-256 `91abee9452d842260d0ee8ab7823d89963b798d6923b7dec2f53e744be37f89e`。旧包保留，不覆盖、不删除；历史构建 SHA 留在原节。**'
new = f"**R242 更新：R240、R239、R236 包均不再用于联调，S16 统一改用 `dist/R242-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe`，SHA-256 `{receipt['setup_sha256']}`。旧包文件保留，不覆盖、不删除；历史构建 SHA 留在原节。旧包遇到 EXPIRED 或 license_expires_at 会报 unknown error 或 invalid。**"
assert old in text
text = text.replace(old, new, 1)
text = text.replace('不要截取首次完整码弹窗。', '不要截取注册码输入、完整码列表/详情或 CSV 内容。', 1)
old_row = '| S16-08 管理页显示 | 用户在 staging 管理页生成测试码，看到一次完整值后保存到自己的安全位置；关闭弹窗，刷新列表并重新打开详情 | 完整码仅首次显示一次；列表/后续详情只有标识与摘要；不再返回或显示完整码；反馈不包含完整码 | □通过 □失败；时间/现象：____ |'
new_row = '| S16-08 管理页显示 | 用户在 staging 管理页生成测试码，刷新列表、打开详情并复制；检查旧版本生成的测试码 | 新版生成的码可随时显示/复制；没有保存密文的旧码显示“旧版本生成，无法显示”，需重置或重新生成；反馈不包含完整码 | 未在 Windows 实跑；□通过 □失败；时间/现象：____ |'
assert old_row in text
text = text.replace(old_row, new_row, 1)
marker = '| S16-09 更新隔离补查 | 两机尝试检查更新，重启程序观察更新入口；若有旧正式更新缓存也重复观察 | 不显示正式 Latest/发布页链接，不检查或下载正式包、不启动正式安装器；更新入口隐藏或返回当前构建不支持更新；标题始终 STAGING | □通过 □失败；时间/现象：____ |'
assert marker in text
extra = '''
| S16-10 普通码过期与续期 | 普通码设置“首次激活起算 1 天”并激活；在管理页把到期日改到当前时间之前；观察、重启、恢复网络；续期后明确重新输入同一码 | 客户端在 1 分钟内显示“注册码已过期”；立即锁定，重启仍是已过期，不显示已停用；仅续期/联网不自动恢复，续期并明确重输后恢复 | 未在 Windows 实跑；□通过 □失败；操作/锁定/重启/重输时间及文案：____ |
| S16-11 自定义管理员码修改 | 自定义管理员码激活两台电脑；在详情抽屉“修改注册码”；观察两台续租，再分别明确输入新码 | 两台均显示“注册码已在其他设备使用或已被重置”，不能自动恢复；两台都需输入新码，输入后可同时使用 | 未在 Windows 实跑；□通过 □失败；两机状态/时间/现象：____ |
| S16-12 设置页到期信息 | 有效期码激活后进入设置→隐私与能力；核对到期日与剩余天数；续期后再次查看；永久码重复查看 | 显示一行“授权到期：日期（剩 N 天）”，续期后更新；永久码显示“授权到期：永久”；不增加其他提示或弹窗 | 未在 Windows 实跑；□通过 □失败；日期/剩余天数/永久/续期更新：____ |'''
text = text.replace(marker, marker + extra, 1)

body = '''
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
'''
for r in rows:
    command = ' '.join(r['command']) + ('（cwd desktop/）' if r['cwd'] == 'desktop' else '')
    command = command.replace('|', r'\|')
    result = 'exit 0'
    if 'skip' in r:
        result += f"；{r['top_level_pass']} 顶层 PASS；{r['skip']} SKIP/{r['fail']} FAIL"
    body += f"| `{command}` | {r['started']} | {r['finished']} | {r['elapsed_seconds']} | {result} |\n"
body += '\n完整日志/时刻见 [最终七项检查](history/reports/r242/checks-final.json)。真实末五行（不足五行原样保留）：\n'
for r in rows:
    body += '\n`' + ' '.join(r['command']) + ('（cwd desktop/）' if r['cwd'] == 'desktop' else '') + '`\n\n```text\n'
    body += '\n'.join(r['last_5_lines']) or '（无输出）'
    body += '\n```\n'

body += f'''
### 唯一一次 public STAGING 构建与只读审计

七项最终检查通过并核对源码未变后，仅执行一次 `node scripts/build-license-staging.cjs --output dist/R242-staging-public/`。key mode **public**、构建标签 **license_staging**、版本 **0.12.75**、标题 **Deep Legends-STAGING**、压缩级别 **9**、`--publish=never`。结果和真实末五行已记录在上表及 [构建记录](history/reports/r242/staging-build-summary.json)。

- 新安装包 [Deep-Legends-Setup-0.12.75-staging-public.exe](../dist/R242-staging-public/Deep-Legends-Setup-0.12.75-staging-public.exe)，{pathlib.Path(receipt['setup']).stat().st_size} 字节；目录版 `dist/R242-staging-public/Deep-Legends-staging-public/`；附 [SHA256SUMS](../dist/R242-staging-public/SHA256SUMS-staging-public.txt)、[staging-build.json](../dist/R242-staging-public/staging-build.json)。
- Setup SHA-256：`{receipt['setup_sha256']}`。
- Backend SHA-256：`{receipt['backend_sha256']}`。
- ASAR SHA-256：`{receipt['archive_sha256']}`。
- 构建指纹：`{receipt['fingerprint']}`；按唯一 STAGING 标题覆盖重算与实际包一致，见 [fingerprint-inputs](history/reports/r242/fingerprint-inputs.json)。
- 对实际 Setup/目录版只读审计 PASS：origin/kid/用户 staging 公钥准确，R237 隐私、R240 pending/窗口握手、R242 EXPIRED/终止原因/到期字段/设置行/原生转发均嵌入；无私钥、共享测试向量、测试信任材料、个人 Riot Key，固定 backend digest、五项 fuse 和 PE ASAR 完整性一致。为信任隔离审计只在临时目录构建 public/default-release 后端，完成删除，未构建 release 分发包。见 [package-audit](history/reports/r242/package-audit.json)。
- R240、R239、R236 旧目录各 **26 文件**，文件集合、SHA-256、size、mtime_ns、mode 前后完全不变；没有删除、覆盖或重建。见 [before](history/reports/r242/artifacts-before.json)、[after](history/reports/r242/artifacts-after.json)、[preservation](history/reports/r242/artifact-preservation.json)。R240 与更旧包不能处理新协议字段/错误码，不再用于 S16。

### 非最终失败与未执行边界

- 首次 Go 专项因系统默认缓存写入受沙箱限制而 setup failed；获准使用系统缓存后双标签专项及 Go 全量/race/vet 均实际重跑通过。两次自动审批超时没有执行命令，之后重试成功，未写为通过。首次 Chrome 沙箱启动超时；后续合成 status 缺少 process-not-found 和过期文案断言未等异步渲染造成护栏失败，仅修 R242 浏览器夹具和等待断言，最终真实 Chromium 通过。失败日志保留在本报告目录，未修改业务加载遮罩以迎合测试。
- S16 已更新为新包，并增加 S16-10、S16-11、S16-12；S16-08 同步新版管理页“随时可见”预期。**未在 Windows 实跑**：Windows 安装/启动、DPAPI、真实注册码/真实服务、两机、自定义管理员码修改与管理页联调均由用户执行，未把本机合成测试/静态审计记为这些项目通过。S16-07 校时仍按 R240 留待以后。
- 会话 A 的 P8/P9 已完成；会话 B 的服务端、管理页、部署与验收不属于本次执行。未触发远端 CI、未发布、未打 tag，生产公钥仍未提供，不把 STAGING public 包当正式发布包。

新安装包 SHA-256：`{receipt['setup_sha256']}`。
'''
LEDGER.write_text(text.rstrip() + '\n' + body)
print(json.dumps(dict(ledger=str(LEDGER), setup_sha256=receipt['setup_sha256']), ensure_ascii=False))
