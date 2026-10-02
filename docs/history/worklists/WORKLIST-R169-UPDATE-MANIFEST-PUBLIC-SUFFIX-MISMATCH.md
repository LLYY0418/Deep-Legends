# WORKLIST-R169：更新清单「格式不正确」——`-public` 后缀命名与本地校验器不匹配（+ 该日志的其余扫描结果）

诊断人：Claude（只读代码排查 + 新诊断日志 `lol-loot-diagnostics-0926-2236.jsonl`，无截图之外的实机操作）。
执行人：GPT。
日期：2026-09-26。
落地版本：源码 0.12.30（构建 `c4186a398af1`）。

用户反馈原话：设置页点"检查更新"报错"无法检查更新：更新清单格式不正确，请从发布页手动下载"，要求结合日志定位问题，并检查日志是否还暴露了其他问题。

---

## 结论先行

这是一个**百分之百必现**的 bug，不需要更多现场复现就能定位：R143 把 Windows 打包切到 GitHub Actions 后，发布产物的安装包文件名固定加了 `-public` 后缀（`Deep-Legends-Setup-<版本>-public.exe`），但 `backend/update.go` 里校验清单的 `validateUpdateManifest` 从来没有认识过这个后缀——它只接受"纯版本号"或"版本号+12 位十六进制指纹"两种命名，`-public` 两种都不匹配，所以只要真的从发布页拉到清单就必然被判定为"格式不正确"。

这也解释了为什么日志里**完全找不到**这次报错对应的诊断事件：`update.go` 全文件没有一处调用 `p.diag(...)`（诊断埋点），更新检查这条链路对诊断系统是不可见的。下面先给出代码证据，再给出两处需要修的地方，最后是对这份新日志其余部分的扫描结论。

---

## P1：`validateUpdateManifest` 的文件名校验漏掉了 `-public` 命名

### 现象

- 截图：设置页版本 `0.12.30`，构建 `c4186a398af1`；点击"检查更新"后提示"无法检查更新：更新清单格式不正确，请从发布页手动下载"。
- 这条报错文案是 `backend/update.go` 里**唯一**会产出"更新清单格式不正确"字样的地方（`update.go:266`），出现在 `validateUpdateManifest` 返回 non-nil 错误、经 `fetchManifestSource`（`update.go:~444`「更新清单无法读取」是 JSON 解析失败的另一条路径，不是这次的报错文案）透传、最终被 `update.go:377` 包成 `"无法检查更新：" + err.Error()` 显示出来的那一条。

### 根因（代码级、已核实，无需再等复现）

`backend/update.go:265`（`validateUpdateManifest`）：

```go
func validateUpdateManifest(m updateManifest) error {
    _, versionOK := parseUpdateVersion(m.Version)
    _, minimumOK := parseUpdateVersion(m.MinSupported)
    expected := "Deep-Legends-Setup-" + strings.TrimPrefix(m.Version, "v") + ".exe"
    // Accept already-published legacy manifests as well as version-only names.
    legacy := "Deep-Legends-Setup-" + strings.TrimPrefix(m.Version, "v") + "-" + m.Fingerprint + ".exe"
    if m.Asset.Name == legacy {
        expected = legacy
    }
    parsed, err := url.Parse(m.Asset.URL)
    if m.Schema != 1 || !versionOK || (m.MinSupported != "" && !minimumOK) ||
        !updateFingerprintPattern.MatchString(m.Fingerprint) || m.Asset.Name != expected ||
        ... ||
        parsed.Path != "/"+updateRepo+"/releases/download/v"+strings.TrimPrefix(m.Version, "v")+"/"+expected {
        return errors.New("更新清单格式不正确，请从发布页手动下载")
    }
    return nil
}
```

它只接受两种资产文件名：
1. `expected` = `Deep-Legends-Setup-<版本>.exe`（纯版本号）
2. `legacy` = `Deep-Legends-Setup-<版本>-<12位十六进制指纹>.exe`

而 `.github/workflows/release.yml`（R143 引入）和它调用的 `build-public-release-windows.ps1` 里，真实发布产物的命名是**第三种**：

`build-public-release-windows.ps1:44`：
```powershell
$setupName = "Deep-Legends-Setup-$Version-public.exe"
```

`.github/workflows/release.yml` 上传/发布的资产也固定是这个名字（`Deep-Legends-Setup-${{ env.RELEASE_VERSION }}-public.exe`），脚本自己还会在构建后校验 `manifest.asset.name -eq $setupName`（`build-public-release-windows.ps1:51`），说明**清单里 `asset.name` 字段本身就是 `...-public.exe`**——这是发布链路的既定设计，不是误传。

`-public` 后缀里的 `public` 不是 12 位十六进制，`updateFingerprintPattern` 校验的是 `Fingerprint` 字段本身（与文件名无关），所以：
- `m.Asset.Name` = `Deep-Legends-Setup-0.12.30-public.exe`
- 既不等于 `expected`（`Deep-Legends-Setup-0.12.30.exe`）
- 也不等于 `legacy`（`Deep-Legends-Setup-0.12.30-<真实指纹>.exe`）
- → `m.Asset.Name != expected` 恒为 `true` → 校验失败
- 同时 `parsed.Path` 校验用的也是同一个 `expected`，同样恒不匹配——**两处校验都会独立地把这次发布判为不合法**。

### 为什么 Go 单测没拦住

`backend/update_test.go:27`（`updateTestManifest`）构造的测试用清单用的是"legacy 指纹"命名：
```go
name := "Deep-Legends-Setup-0.12.0-a1b2c3d4e5f6.exe"
```
从未用 `-public` 后缀跑过 `validateUpdateManifest`，所以这个不匹配在单测里从未暴露。`docs/WORKLIST-INDEX.md` 里 R143 的状态写的是"已关闭（public 草稿未发布）"——也就是说这条发布流水线此前一直没有真正发布过 release，这次应该是第一次有真实用户对着已发布的 `-public` 资产走"检查更新"，所以此前也没有真机复现过。

### 修复方向（二选一，建议 A，理由见下）

- **方向 A（改 `validateUpdateManifest`，推荐）**：把 `-public` 也纳入合法命名集合，例如把 legacy 判断从"是否等于旧指纹命名"扩展成一个小白名单／正则，同时接受：
  - `Deep-Legends-Setup-<版本>.exe`
  - `Deep-Legends-Setup-<版本>-<12位十六进制>.exe`
  - `Deep-Legends-Setup-<版本>-public.exe`

  `expected` 与 `parsed.Path` 校验都要跟着改用同一个"匹配到的那个合法名字"，不能只改其中一处（否则文件名过了、URL 路径校验又会单独炸）。
  推荐这个方向，因为 `-public` 是发布流水线里明确、稳定的既定产物名（`build-public-release-windows.ps1` 自己也在断言这个名字），改校验器去适配一个已经稳定下来的真实产物，比反过来改发布流水线要小、要安全。

- **方向 B（改发布流水线，去掉 `-public` 后缀）**：把 `release.yml`/`build-public-release-windows.ps1` 里的产物名改回 `Deep-Legends-Setup-<版本>.exe`（落进 `expected` 分支）。风险：`-public` 后缀大概率是有意为之（例如用来和某种"非公开/内部构建"命名区分），改名前需要确认没有其他脚本、文档、已发布 Release 资产依赖这个后缀；且历史上可能已经有草稿 Release 用了 `-public` 命名，改名会造成新旧命名并存，校验器届时还是要同时兼容两种，等于绕了一圈还是要做方向 A 的事。

**结论**：除非有理由认为 `-public` 后缀本身是错误设计需要撤销，否则按方向 A 改校验器更直接。

### P1 附带问题：更新检查链路完全没有诊断埋点

`backend/update.go`（含 `update_download.go`）通篇没有任何 `p.diag(...)` 调用。`Check()`/`fetchManifest`/`fetchManifestSource` 无论成功、HTTP 出错、JSON 解析失败还是这次的"格式不正确"，全部只落进内存里的 `u.status.Error` 供前端展示一次，**不落诊断日志**。这就是这次"根据日志定位问题"实际上日志里一条相关记录都没有的原因——不是没抓到，是这条链路从设计上就没打日志。

**修复要求**：在 `Check()` 的 goroutine 里，`fetchManifest`/`fetchManifestSource` 返回错误、以及 `validateUpdateManifest` 返回错误这几个分支，补一条 `p.diag(...)` 事件（建议事件名 `update_check_failed`，字段至少包含：`stage`（`fetch`/`parse`/`validate`）、`mirror_prefix`、`http_status`（如有）、`error`、`current_version`、`build_fingerprint`），成功分支也建议补一条 `update_check_succeeded`（`latest_version`、`state`）。这样以后再出问题，不用等用户截图，看日志就能定位。

### 测试要求

- Go 单测：`backend/update_test.go` 里新增一个用 `Deep-Legends-Setup-<版本>-public.exe` 命名的清单 fixture（比照 `updateTestManifest` 的写法），断言 `validateUpdateManifest` 对它返回 `nil`；同时保留对纯版本号、legacy 指纹两种命名仍然通过的既有断言，防止修复时把旧的合法命名改坏。
- 新增一条针对"完全不认识的第三种命名"（比如 `Deep-Legends-Setup-<版本>-beta.exe`）的反例断言，确认它仍然被拒绝——避免把校验器改得过于宽松。
- 若按方向 A 落地，还需要补一条断言：`legacy`/`-public` 两个分支互斥时，`parsed.Path` 校验用的 `expected` 值确实同步切换到匹配的那个命名（避免像本次一样文件名和 URL 路径两处校验各写各的、只改一处漏了另一处）。
- 若补了诊断埋点，补一个断言：`fetchManifest`/`validateUpdateManifest` 失败路径确实调用了 `p.diag`/`recordDiagnostic`（比照仓库里其他诊断埋点的测试写法，例如 `collection_reads.go` 对应的测试）。

---

## P2：对这份新日志（`lol-loot-diagnostics-0926-2236.jsonl`，15606 行，142 种事件）的整体扫描结论

按本项目一贯做法，除了用户点名的问题，也过了一遍事件类型直方图和几个数量较高/较少见的事件的样本，结论如下——**没有发现新的、独立于已知工单的问题**：

- `collection_data_retry_exhausted`（3 次，每小时整点各 1 次，`attempts=3, blank_entries=1`）：核对了 `backend/collection_reads.go` 的 `scheduleCollectionDataRetry`/`settleBlankLoot`，这是该功能**按设计**的兜底路径（重试 3 次仍拿到同一条空白战利品记录后，主动结束"稍后重试"状态并打点），行为与命名都对得上、非异常；且该主题已有独立工单 `docs/history/worklists/WORKLIST-R144-BLANK-LOOT-RECORD-NOT-SYNCED.md` 在跟踪，不需要在本工单重复开项。
- `image_queue_slow`（仅 1 次，`load_ms=9, queue_wait_ms=10000`）：单次样本，量级不足以判断是否是真实性能问题，暂不单独立项；如果后续日志里这个事件频繁出现，值得回头看。
- `sgp_ranked_stats_incomplete`（584 次，`queue_count=2`）：抽样看是同一账号/同一时间段内的正常重复调用模式（该玩家在 2 个排位队列下数据不全时的预期提示），未发现异常聚集或报错特征，判定为已知的正常业务噪音，不追加问题。
- `desktop_error`（1 次，`"启动阶段发送失败"`，紧跟在一次异常短的 `desktop_startup`（`total: 1133ms`，明显短于其余 4000-5000ms 的正常启动）之后）：像是一次孤立的启动期 IPC 发送时序问题（可能是渲染进程还没准备好就发送了启动阶段耗时事件），只出现 1 次，不影响功能（后续同一 `run_id` 下应用照常工作），暂记录为观察项，不建议本工单立项，若后续日志重复出现同类现象再单独处理。
- 其余高频事件（`lcu_request` 4354 次、`gameflow_phase_client` 2395 次、`champselect_ban_probe`/`champselect_trace` 等）均为已知业务轮询/追踪事件，未见异常错误率或新模式。

**这份日志里唯一需要真正处理的问题就是 P1（更新清单校验）**，其余均为已知项或噪音级观察，不新增独立 P 项。

---

## 验收标准

1. 真实按 R143 的发布流水线产出一份 `-public` 命名的清单+安装包（或本地构造同样命名的清单文件指给 `updateManager` 走一遍），确认 `Check(true)` 后 `u.status.State` 能正确进入 `available`/`ready`，`u.status.Error` 为空，不再出现"更新清单格式不正确"。
2. 上面列出的 Go 单测新增/补充用例全部通过。
3. 若按建议补了诊断埋点：人为构造一次校验失败（例如喂一个字段缺失的清单），确认日志里出现新增的 `update_check_failed` 事件且字段完整。
4. 确认改动没有影响"纯版本号"和"legacy 指纹后缀"两种旧命名的兼容性（回归断言，见测试要求）。

---

*本工单由 Claude 基于代码只读排查 + 日志 `lol-loot-diagnostics-0926-2236.jsonl` 完成，未做任何实机操作，未修改任何源码。*
