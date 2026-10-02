# 0.12.49 GitHub 发布账本

日期：2026-10-02。用户明确要求发布新版本，以试用应用内更新功能。当前 GitHub 最新版为 v0.12.19；本轮发布版本为 **0.12.49**，包含当前工作区累计工单改动，源码指纹 **e55eeb092260**。

## 发布准备

- CHANGELOG 的未发布内容转为 `0.12.49 — 2026-10-02`，使用更新弹窗支持的标题；版本文档同步为 0.12.49。
- 源码快照使用 `codex/release-0.12.49` 分支，保留 main；排除 docs/r116-validation、r121-validation、r166-validation、r174-validation 本机原始验收日志和截图。密钥文件、构建产物沿用 gitignore 不进入源码提交。原始验证材料留在本地，没有删除。
- 暂存时发现 2026-09-24 遗留的空 .git/index.lock，确认没有进程占用后清理。
- 本地 macOS 使用 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh` 完整交叉打包 Windows amd64 安装器；没有使用 private 包或绕过构建收据。

## 构建与回归

- 打包门禁发现并运行 **1640 个 Go 测试**，五分片全部通过；主模块和 installer 的 test/vet 均通过。日志：/private/tmp/release-0.12.49-build.log。
- Web、Desktop、scripts 联合 Node 回归共 1089 项：首轮 1085 通过、3 项因 Windows/PowerShell 平台条件跳过、1 项失败。唯一失败为 refresh-orchestration 抽取函数的测试夹具没有加载真实 clearRuneStarterRetries；补齐该函数依赖后，该文件 **14/14 通过**（5.936 秒）。生产代码未改变，安装包与指纹不变。日志：/private/tmp/release-0.12.49-node.log、release-0.12.49-refresh-tests.log。最终已覆盖的联合用例为 1086 通过、3 项平台跳过。
- 更新专项 `go test ./backend -run TestUpdate -count=1` 通过，1.337 秒；/private/tmp/release-0.12.49-update.log。
- 清单/收据专项 4/4 通过；/private/tmp/release-0.12.49-manifest-tests.log。
- 包内 runtime、包内后端指纹、源码后端指纹、public 空密钥策略、构建收据与最终安装包 SHA-256 全部通过；git diff --check 通过。

## 发布三件套

`node scripts/make-release.cjs` 依据最终 public 构建收据生成以下且仅以下文件：

| 文件 | 校验 |
|---|---|
| Deep-Legends-Setup-0.12.49-public.exe | 110922240 字节；SHA-256 c40007deffc11549347a9d062ac2893864a9dc6b64562b5cd7ad610b45c897dd |
| latest.json | schema=1、version=0.12.49、fingerprint=e55eeb092260、minSupported=0.9.0；asset 名称、URL、大小及 SHA-256 与安装包一致 |
| SHA256SUMS-public.txt | 两行，分别校验安装包与 latest.json |

发布目录：dist/release。客户端入口为 `https://github.com/LLYY0418/Deep-Legends/releases/latest/download/latest.json`；安装包 URL 为 `https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.49/Deep-Legends-Setup-0.12.49-public.exe`。

## 真机边界

用户本次明确授权正式发布，发布后由用户试用更新。没有声称 Windows 真机检查、下载、重启安装或失败回退已经验证。public 包不包含个人 Riot API Key。R184 仅增加设置监测诊断，镜头重置问题尚未修复。
