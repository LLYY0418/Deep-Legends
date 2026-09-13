# R82 构建耗时排查与新样本 1/5

后续带子阶段计时的日志已定位 7z 压缩耗时，并调整为仅构建 Setup，见 [最新诊断](DIAGNOSIS-R82-SETUP-ONLY-BUILD.md)。以下保留当时的证据与样本记录。

日期：2026-09-12。只读检查已有产物和已安装 electron-builder 26.15.7 源码；本轮没有打包或安装。

## 当前证据

用户提供的日志及当前 `dist/desktop/release-build.json` 对应指纹 `b07acedb0203`，版本 `0.12.1`。

| 已有产物 | 修改时间（本地时间） |
|---|---|
| portable exe | 15:57:00 |
| 第二次组装的 `win-unpacked/resources/app.asar` | 15:57:06 |
| Go 外壳封装后的最终 setup exe | 16:01:46 |
| 构建记录 | 16:01:47 |
| ZIP | 16:02:22 |
| SHA256SUMS | 16:02:25 |

第二次应用组装至最终安装包约 **4 分 40 秒**；该区间包含 NSIS 和 Go 外壳封装，不能全部当成 7z 耗时。构建记录至 ZIP 完成约 **35 秒**。原日志没有逐阶段时间戳，无法从它进一步定量拆分。产物和校验文件已完整写出，这一轮最终完成，并非一直没有结果。

## 静默区间实际做什么

依据仓库内已安装依赖，而非猜测：

- `desktop/node_modules/app-builder-lib/out/targets/nsis/NsisTarget.js` 的 `buildAppPackage`：本项目 `differentialPackage:false`，NSIS 没有 `useZip:true`，因此从 `win-unpacked` 生成 `.nsis.7z`。portable 的 ZIP 配置不适用于这个安装包。
- `out/targets/archive.js` 的 `compute7zCompressArgs`：当前 7z 默认为 `-mx=9`，安装包过滤器为兼容提取器的 `-mf=BCJ`。`debug7zArgs` 添加 `-bd`；`builder-util.exec` 缓冲输出，因此压缩时终端可以长时间没有新行。这是当前最明确的高开销候选，尚无子阶段计时证明它独占上述全部时间。
- `NsisTarget.computeScriptAndSignUninstaller`：项目配置的是 **include**，不是替代完整脚本的 **script**，因此仍生成中间卸载器、再编译最终安装器。不能把 `include: nsis/installer.nsh` 误认为跳过该流程。
- 当前 macOS 对应 `isMacOsCatalina()` 的“大于等于”分支，优先由 `UninstallerReader` 直接解析中间文件，失败才退到 VM。此次日志没有回退警告，不能把卡顿直接归因于 Wine。
- 最后 `installer/build-shell.cjs` 将 NSIS payload 嵌入 Go 外壳，然后还会生成独立 ZIP 并计算最终哈希。

其余开销：`npm ci` 明确为 **4 秒**；Electron 使用本地缓存。两次独立 builder 调用都会重组装应用目录，确有重复工作，但产物时间显示这不是上述四分多钟的主要区间。项目先前保留两个独立目标是为了避免共享归档被提前清理，未贸然合并调用；`--prepackaged` 还会跳过 `beforePack`，不能直接替换而丢失校验。

## 本轮改动与验证

- `scripts/build-stage.cjs` 为 Bash 构建入口的 portable、卸载外壳、NSIS、安装外壳、ZIP 五个阶段增加开始/完成时间和每 15 秒的存活提示。提示表示命令未退出，不伪造压缩百分比。
- 保留命令、参数、环境、输出及退出码，不打印命令参数。失败继续让原来的 `set -e` 停止构建。
- 压缩级别、包格式、安装行为、启动逻辑和预热开关均未改变，以保持这些冷启动样本的条件一致。本轮是定位与可观测性补充，不声称已实测提速。
- `desktop_startup.go` 新增 `R82 cold-install sample 1/5` 注释，不改变运行逻辑。构建计时工具也纳入源码指纹输入。
- `node --test scripts/build-stage.test.cjs desktop/build-fingerprint.test.cjs desktop/portable-template.test.cjs desktop/installer-shell.test.cjs`：**26 PASS / 0 FAIL / 0 SKIP**。测试包括真实小型子进程的存活提示、退出码保留、启动失败、参数不写入日志，以及既有构建/模板检查；未调用真实打包器。
- `bash -n build-desktop.sh` 和 `gofmt` 检查通过。

## 本次仅准备第一份

| 样本 | 版本号 | 修改前指纹 | 本次待构建指纹 |
|---|---|---|---|
| 1/5 | 0.12.1 | b07acedb0203 | **974ec9e7284f** |

可以正常运行 `./build-desktop.sh`。为进一步区分内部 7z 与 NSIS 编译耗时，本次建议用上游调试日志：

```bash
DEBUG=electron-builder ./build-desktop.sh
```

这是用户随后执行的命令，本轮未运行。终端与最终 `release-build.json` 应显示新指纹；如期间还有其他源码改动，以构建实际输出为准。每份构建会使用相同版本号文件名，下一份构建前应将本份整个 `dist/desktop` 另存到独立目录，保留其 `release-build.json`，避免被下一轮清理或覆盖。其余四个新指纹等待用户逐个要求后再准备。
