# R80 / R81 验收整改记录

日期：2026-09-11。依据：`ACCEPTANCE-CLAUDE-R80-R81.md` 和用户“图标按照上面改完的为准”的明确要求。

本轮已完成并发测试缺口修补、公开构建的个人凭据隔离，以及新的本地发布候选包。GitHub 实际上传与 Windows 真机升级闭环仍未完成，不能宣称 R80 / R81 全部验收通过。

## 逐项结果

| 验收项 | 状态 | 本轮处理 |
|---|---|---|
| P1 升级图标 | 已确认并保留 | `web/app.css` 仍为 30×30px；SVG 箭头仍为 `M12 16.4V9.2` / `M8.9 12.3 12 9.2l3.1 3.1`。本轮未修改图标代码；已检查最终打包后端包含上述 CSS 与 SVG。 |
| P2 `busyLocked()` 测试缺失 | 已完成 | 新测试通过实际 Check、Download、Apply 流程进入 checking/downloading/verifying/applying 四态，用通道固定重叠时机；各态均拒绝 10 次 `Check(false)`、手动检查、重复下载和修改镜像。断言原任务完成、状态不被覆盖、仅一次外发请求。 |
| P2 漏网变异 | 已完成 | `scripts/r81-mutations.py` 增加将整个 busy gate 改成 `return false` 的变异，基线通过、该变异被测试捕获；共 16/16 变异被捕获。 |
| 个人 Riot Key 分发边界 | 已完成本地实现与构建验证 | 新增 public 构建方式，跳过本机 Key 文件和密文环境变量，显式清空内嵌明文/密文变量。保留默认 private 私用构建。打包前后检查个人 Key，公开发布检查拒绝 private 包及陈旧产物。 |
| F3 四文件准备 | 已完成 | 两个无空格 EXE、`latest.json`、`SHA256SUMS.txt` 已生成，最终大小、SHA-256、清单 URL 与指纹已核对。 |
| F3 GitHub 实际上传 | 未完成 | 当前无 `gh`/`hub` 或 GitHub 连接器；GitHub 直连 8 秒超时，浏览器打开发布页也超时。未创建 tag、未创建 Release、未上传任何文件。 |
| Windows 真机验收 | 未完成 | 当前为 macOS arm64，未发现 Windows VM/兼容层。交叉编译和测试通过不能替代 R80 的 16 项真机验收及 R81 更新闭环。 |
| 三个镜像连通与 Range | 已实测通过公开样本 | 三者均返回 HTTP 206，`bytes 0-65535/331012528`，64 KiB 内容均为相同 SHA-256，且具有 EXE 的 MZ 文件头。 |
| 大 EXE 完整传输 | 两个通过，一个未通过限时验证 | `ghfast.top` 用时 46.61 秒、`gh-proxy.com` 用时 18.67 秒，均完整收到 331,012,528 字节且哈希一致；`ghproxy.net` 在 120 秒限时内收到 5,139,926 字节，不能标为完整传输通过。 |
| 本项目更新发现端点 | 未打通 | 三个镜像请求本项目 `releases/latest/download/latest.json` 均返回 HTTP 404；GitHub 直连超时。不能据此断言仓库不存在，也不能称自动升级端到端已可用。 |

## 公开构建方式及功能边界

macOS：

```bash
DEEP_LEGENDS_KEY_MODE=public bash build-desktop.sh 0.12.0
node scripts/make-release.cjs
```

Windows：

```powershell
./build-desktop-windows.ps1 -Version 0.12.0 -KeyMode public
node scripts/make-release.cjs
```

public 模式不会使用开发者的个人 Key；本轮构建额外指定不存在的 `RIOT_KEY_FILE` 仍成功，验证了这条路径不依赖本机 Key 文件。默认 private 模式保留原来的本人私用构建流程，但不能通过公开发布检查。

公开包保留国服本机客户端功能。韩服账号、段位、战绩、竞技场详情和绝活哥符文等 Riot API 查询需要使用者在启动应用的环境中自行提供 `RIOT_API_KEY`；没有配置时沿用现有明确提示／空结果降级，不向 Riot API 发请求。README 与本版本 CHANGELOG 已说明该差异。

`desktop/release-build.cjs` 在完整构建结束前检查原始后端与打包后端的凭据和哈希，记录两个实际 EXE 的哈希到本地 `dist/desktop/release-build.json`。`make-release.cjs` 生成公开发布文件前再次核对 public 模式、指纹、后端及 EXE 哈希，缺失记录、private 包、后端或产物被替换均失败。构建记录是本机构建流程的追溯信息，不是代码签名；不上传该记录。

## 本轮验证

| 检查 | 结果 | 证据 |
|---|---|---|
| 根模块 `go test ./...` | 通过，74.729 秒 | `output/update-r81/acceptance-go-tests.log` |
| 更新模块 `go test -race -count=1 -run '^TestUpdate' .` | 通过，10.599 秒 | `output/update-r81/acceptance-update-tests.log` |
| 安装器与卸载器 `go test ./...` | 通过 | `output/update-r81/acceptance-installer-tests.log` |
| 根模块与 installer 的 Windows `go vet ./...` | 通过 | `output/update-r81/acceptance-windows-vet.log`、`acceptance-installer-windows-vet.log` |
| desktop / web / scripts Node 测试 | 526 通过、0 失败 | `output/update-r81/acceptance-node-tests.log` |
| Go / 安装器变异 | 16/16 捕获，含 busy gate disabled | `output/update-r81/acceptance-go-mutations.log` |
| 凭据与发布校验用例 | 通过，包含合成密钥、无 Key、明文 Key、加密 Key、后端替换、EXE 替换、private 拒绝等 | `desktop/release-build.test.cjs`、`scripts/make-release.test.cjs` |
| 完整 public 构建 | 通过，含 portable、NSIS、Go 安装/卸载外壳、打包运行时检查 | `output/update-r81/public-build.log` |
| 四个最终发布文件 | 大小、哈希及图标内容检查通过；安装器为 x64 GUI PE | `output/update-r81/public-release-verification.json` |

Windows PowerShell 构建脚本仅完成静态复核；本轮实际构建在 macOS 执行 Bash 脚本。未执行任何下载的 Windows EXE。

## 本次产物

版本 `0.12.0`，源码指纹 `dd5512e9514e`，模式 `public`。

- `dist/release/Deep-Legends-Setup-0.12.0-dd5512e9514e.exe`：103,212,544 字节。
- `dist/release/Deep-Legends-0.12.0-dd5512e9514e.exe`：141,194,709 字节。
- `dist/release/latest.json`。
- `dist/release/SHA256SUMS.txt`。

安装包 SHA-256：`10365c9d36d21fef68de4f1788cf3a90189d3d88eb686e97823c7ca7e9bb892c`。

发布目标保持 `LLYY0418/Deep-Legends` 的 `v0.12.0` Release；实际上传前仍需能访问 GitHub 且具备目标仓库发布权限的环境。不得把本地四文件已生成算作已上传。上传后必须验证 `latest.json` 的在线字节及资产 SHA-256，并在旧版 Windows 安装环境实际执行发现 → 下载 → 校验 → 退出 → 安装外壳升级 → 重启 → 清理旧包的完整流程。

## 联网验证边界

样本 EXE 来源为 [Obsidian 官方下载页](https://obsidian.md/download)，页面指向 `obsidianmd/obsidian-releases` 的 `v1.13.7/Obsidian-1.13.7.exe`。样本只用于传输检查；下载内容未执行，临时文件在测试后删除。

- 本项目端点结果：`output/update-r81/live-network.json`。
- 大文件 Range 结果：`output/update-r81/live-mirror-exe.json`。
- 完整大文件传输结果：`output/update-r81/live-mirror-full-exe.json`。`ghfast.top` 和 `gh-proxy.com` 的完整文件 SHA-256 均为 `f233dc24896b3f2d5f9e4b01111181a561d0760b2105f0a474024c5f3143a9bc`；`ghproxy.net` 超过本次 120 秒验证时限，仅完成约 5.1 MB，未证明完整传输可用。这里的 120 秒为验收脚本总时限，产品本身仍按原工单使用 8 秒无数据超时，不把大文件的总下载时间限制为 120 秒。

镜像实测仅代表当前网络和上述样本，不能代替本项目发布后的文件校验，也不能保证长期可用。仍需在实际用户网络和 Windows 环境完成最终验收。
