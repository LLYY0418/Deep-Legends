# R169 执行账本

日期：2026-09-26。基线源码版本 0.12.33；本轮源码版本 0.12.34。

## P1 更新清单与诊断

`validateUpdateManifest` 现在只接受三个精确文件名：`Deep-Legends-Setup-<版本>.exe`、`Deep-Legends-Setup-<版本>-<清单内12位指纹>.exe`、`Deep-Legends-Setup-<版本>-public.exe`。实际匹配的名字同时用于 `asset.name` 和 GitHub Release URL 路径校验；`-beta` 等未列入的后缀仍拒绝。发布流水线及 `scripts/make-release.cjs` 固定生成 `-public` 命名，本轮没有修改发布脚本。

更新模块接入应用统一 `recordDiagnostic`。各镜像的清单获取失败以 `update_check_failed` 记录 `stage`（`fetch`/`parse`/`validate`）、`mirror_prefix`、`http_status`、`error_kind`、安全的 `error` 类别、`current_version`、`build_fingerprint`；成功以 `update_check_succeeded` 记录 `latest_version`、`state`、缓存命中及版本/指纹。镜像仅记录直连标识或 HTTPS 主机，不记自定义路径、查询参数、完整资源 URL 或玩家身份。用户界面的原有错误提示语义保留。

本地构造与 R143 真实发布命名相同的清单，用 `Check(true)` 实际走镜像读取与校验：首次进入 `available`，把相同安装包字节放入更新目录后再次检查进入 `ready`，两次 `status.Error` 都为空。回归测试还覆盖纯版本号、旧指纹名、交叉错配 URL、未知 `-beta` 后缀，并把 HTTP、JSON 解析、清单校验三种失败写入真实诊断 JSONL 核对字段与脱敏。

## P2 新日志其他事件

工单所述 `lol-loot-diagnostics-0926-2236.jsonl` 原文件不在当前工作区；本轮不能独立复算其中 15606 行、142 种事件的计数。按工单诊断记录与源码逐项核对：`collection_data_retry_exhausted` 是三次重试后结束空白战利品等待的既定兜底，已有 R144 跟踪；单次 `image_queue_slow` 样本不足以判定持续性能问题；`sgp_ranked_stats_incomplete` 是排位负场缺失时不展示不可靠胜率的既有诊断；一次启动期 `desktop_error` 暂作观察项。未发现需要在 R169 中新增的独立代码改动，后续若同类启动或图片排队事件重复出现再单独排查。

## 验证与交付边界

- `go test ./backend -run '^TestUpdate' -count=1`：最终定向通过；`go test ./backend -count=1`：最终全套通过（192.977 秒）。`node --test scripts/make-release.test.cjs desktop/release-build.test.cjs backend/web/update.test.cjs`：17/17 通过。`git diff --check` 与 gofmt 检查通过。
- 公开模式（**key mode: public**，`main.riotAPIKey` / `main.riotAPIKeyCipher` 均为空）构建 macOS arm64 `/private/tmp/Deep-Legends-backend-0.12.34-public` 和 Windows amd64 `/private/tmp/Deep-Legends-backend-0.12.34-public.exe`。两者嵌入的源码指纹 `6cf571345981` 均经 `verify-build-fingerprint.cjs` 校验；macOS `-self-test` 输出 0.12.34 并通过。仅保留带 `-public` 后缀的验证产物，不制作或发布安装包；Windows 真实 Release 清单仍需真机复核。
