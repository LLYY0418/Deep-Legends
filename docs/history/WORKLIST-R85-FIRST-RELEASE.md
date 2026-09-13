# WORKLIST-R85 · 首次正式发布 v0.13.0（版本号提升 + 发布文件生成）

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。
用户已确认：这次发布用 **0.13.0**（不是继续用 0.12.1），把 `CHANGELOG.md`
里"未发布"一节的内容正式归到这个版本号下。

---

## 背景

`README.md` 发布流程（第 236-238 行）本身已经写得很清楚，本单只是把
"确认版本号该跳到多少"这一步定下来，并要求 GPT 严格按 README 已有步骤执行，
不要自己另想一套。

`build-desktop-windows.ps1:19` 会强制校验 `desktop/package.json` 的版本号
必须与 `-Version` 参数一致，所以**版本号必须先改好，再跑构建**，顺序不能反。

## 任务

### 一、版本号同步（对应 README 第 236 行第 1 步）

1. `desktop/package.json` 的 `"version"` 从 `0.12.1` 改成 `0.13.0`。
2. `desktop/package-lock.json` 顶层的 `"version"`（第 3、9 行那两处，是本包自身的
   版本号，不是任何依赖包的版本号）同步改成 `0.13.0`。**只改这两处**，
   不要动其它依赖项各自的 version 字段。
3. `CHANGELOG.md`：把当前"## 未发布"标题下的全部内容（安装页面继续显示启动进度、
   启动加载窗口按内容就绪显示这两条）移到一节新的 **`## 0.13.0 — 2026-09-12`**
   标题下面（今天的日期），标题格式必须与文件里已有的其它版本号小节完全一致
   （`scripts/make-release.cjs` 的 `latestNotes()` 靠正则匹配
   `^##\s+(\S+)\s+—\s+\d{4}-\d{2}-\d{2}`，格式不对会直接报错）。
   "## 未发布"这个标题本身保留在最上面，下面留空，供以后继续积累。
4. **顺带检查**这次发布实际包含的改动范围是否比"未发布"里那两条更广
   （R80 安装/卸载外壳、R81 应用内更新、R82/R83 启动优化、R84 外壳阶段埋点等
   一整批工作，很多可能是在更早的版本号下就已经写进 CHANGELOG 了，不需要重复）。
   如果你判断"未发布"里这两条确实就是自上次版本号以来全部新增的用户可见变化，
   直接照抄即可；如果发现还有别的已完成但没写进 CHANGELOG 的用户可见变化，
   请一并补充成 `### 优化` / `### 修复` 列表项，不要遗漏，但也不要把纯内部
   工程性改动（比如测试基础设施、诊断埋点这种用户感知不到的）塞进用户可见的
   更新日志里。
5. `README.md:118` 和 `:237` 里举例用的 `0.12.1` 顺手改成 `0.13.0`
   （只是示例，改不改不影响构建，但改了更准确）。

### 二、执行 public 构建 + 生成发布文件（对应 README 第 237-238 行第 2 步）

严格按 README 已写的命令：

```powershell
./build-desktop-windows.ps1 -Version 0.13.0 -KeyMode public
node scripts/make-release.cjs
```

- 这一步**必须在真实 Windows 环境下跑完整的 NSIS 打包**（不是 Linux 交叉编译
  产出一个测试用二进制就算数——之前给用户做真机 A/B 测试用的那些安装包就是这样
  构建出来的，复用同一条真实构建路径）。
- `-KeyMode public` 会显式清空内嵌的个人 Riot API Key。构建脚本自带的
  `verifyRiotKeyPolicy` 和 `make-release.cjs` 里的 `verifyPublicReleaseBuild`
  会在打包前后各查一次，**不要绕过或注释掉这些检查**。
- 产出应该在 `dist/release/` 下正好三个文件：
  `Deep-Legends-Setup-0.13.0.exe`、`latest.json`、`SHA256SUMS.txt`。

### 三、验收判据

1. `node scripts/make-release.cjs` 命令本身成功退出（不抛异常），控制台会打印
   版本号、指纹和 Setup 的 SHA-256。
2. 打开生成的 `latest.json`，确认：
   - `"version": "0.13.0"`
   - `"asset"."name"` 是 `Deep-Legends-Setup-0.13.0.exe`（没有空格、没有指纹后缀）
   - `"asset"."url"` 是
     `https://github.com/LLYY0418/Deep-Legends/releases/download/v0.13.0/Deep-Legends-Setup-0.13.0.exe`
   - `"notes"` 字段内容与 CHANGELOG 里新增的 `## 0.13.0` 一节一致
3. 用 `Get-FileHash -Algorithm SHA256` 核对 `dist/release/` 里的 EXE 和
   `latest.json` 的哈希，与 `SHA256SUMS.txt` 里记录的完全一致。
4. 用 `desktop/verify-embedded-riot-key.cjs` 或直接用文本工具确认
   `dist/release/Deep-Legends-Setup-0.13.0.exe` 里**不包含**任何看起来像
   Riot API Key 的字符串（`verifyPublicReleaseBuild` 已经在构建脚本里做过
   这个检查，这里是双重确认，不要跳过）。
5. 运行现有回归，确认没有被这次改动带崩：
   `node --test scripts/make-release.test.cjs desktop/release-build.test.cjs`，
   以及涉及版本号比较的 `go test -run TestUpdate ./...`（这些测试用的是各自内部
   构造的 fixture 版本号，不依赖真实 `package.json`，理论上不受这次版本号提升影响，
   但仍然要跑一遍确认）。

### 四、交付物

把 `dist/release/` 下的三个文件（`Deep-Legends-Setup-0.13.0.exe`、
`latest.json`、`SHA256SUMS.txt`）连同验收报告一起交回，**这三个文件需要用户
后续手动上传到 GitHub Release，本单不包含"实际发布到 GitHub"这一步**
（那一步需要用户自己的 GitHub 账号权限，由用户在拿到这三个文件之后自己操作）。

## 不要做的事

- 不要自己去创建/发布 GitHub Release、不要尝试用任何自动化方式推送到
  `LLYY0418/Deep-Legends` 仓库——没有这个权限，也不该有。
- 不要在这一单里顺带改动任何 R80~R84 已经验收过的安装器/启动逻辑代码。
- 不要为了"赶紧发布"而跳过 `verifyPublicReleaseBuild` 或注释掉 Key 检查。
- 不要把 CHANGELOG 写成营销文案，保持现有条目"做了什么"的客观描述风格。
