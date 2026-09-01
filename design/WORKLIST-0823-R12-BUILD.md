# WORKLIST 0823-R12（构建与分发链路整改：让"打完包立刻能用"成为默认）

诊断人：Claude（只读验收，未改仓库文件）。执行人：GPT。
优先级：**全部 P0**。这一单不改任何业务功能，只修构建/分发链路。

## 为什么要单独开这一单

R11 + R11B 的 22 项改动在仓库里**全部已实现且测试全绿**，我也已静态证明打出来的
`Deep Legends.exe` 里确实含这些改动、且 Riot key 密文能正确解密出
`riot_key.local.txt` 里那把真 key。但用户在真机上运行时，**一项都没生效，还报"尚未配置 Riot API Key"**。

也就是说：**代码对、包对、跑起来的却不是这个包**。这已经不是第一次了
（R11 的"打包后 key 消失"事故同理）。用户明确要求："每次构建打包都特别麻烦，
经常遇到这种问题……下次再打包出来的必须立刻就能用。"

本单的目标是让**构建产物的身份可验证、构建步骤不可漏、跑错版本能立刻看出来**。

---

## 一、事故复盘（证据链，GPT 必须先看懂再动手）

### 事实 1：包本身是对的

- 上传的 `Deep Legends.exe`（139,763,540 字节）解包后，
  `resources/app.asar.unpacked/backend/loot-service.exe` 的 SHA-256 =
  `4c4c4a71…a5faac`，与仓库 `desktop/backend/loot-service.exe` **完全一致**。
- 用 `desktop/verify-embedded-riot-key.cjs` 同款 AES-256-GCM 派生逻辑对该二进制扫描，
  解出明文 `RGAPI-67911df0-…`，与 `riot_key.local.txt` **逐字节相同**。
- R11/R11B 的改动字符串（`fourthItems`、`build-routes` 内的 `${depthGroups}`、
  `currentRankedSeason`、`shouldReloadOverview`、`byWinRate`、`item-build-layout` 等）
  在二进制里**全部命中**（前端 JS/CSS 经 `go:embed` 原样进二进制，`strings` 可直接读出）。

### 事实 2：用户机器上跑的不是这个包

用户按提示打开 `C:\Users\18056\AppData\Local\Deep Legends`，**Windows 提示该路径不存在**。

这一条把根因锁死了，推理如下。

### 事实 3：这次打包用的是 electron-builder **上游**的 portable 模板，不是本项目定制模板

对比两份模板：

| | `desktop/nsis/portable.nsi`（定制） | 上游 `app-builder-lib/templates/nsis/portable.nsi` |
|---|---|---|
| 解压目标 | `$LOCALAPPDATA\<APP_FILENAME>\app-${VERSION}-r${REVISION}` | `$PLUGINSDIR\app`（临时目录） |
| 启动方式 | `Exec`（启动器立即退出） | `ExecWait`（启动器驻留） |
| 退出后 | 保留缓存 | `RMDir /r $INSTDIR` 删除 |

定制模板会创建 `%LOCALAPPDATA%\Deep Legends\`；上游模板**不会**。
用户那里该目录不存在 → **这次打的包走的是上游模板**。

> ⚠️ 注意：不能靠"压缩包里是散文件而非内嵌 7z 包"来判断用了哪份模板——
> 上游模板在 `APP_DIR_64` 存在时同样走 `File /r "${APP_DIR_64}\*.*"`。
> 我一开始就是这么误判的，唯一可靠的判据是 `%LOCALAPPDATA%` 下有没有缓存目录。

### 事实 4：为什么会用成上游模板 —— macOS 构建脚本漏了覆盖步骤

`build-desktop-windows.ps1:100-114`（Windows 链路，**完整**）：

```powershell
if (Test-Path "package-lock.json") { npm ci } else { npm install }
# 覆盖为定制模板
Copy-Item -Force $portableTemplate $builderTemplate   # ← 关键步骤
npm run pack:win
npm run pack:win-setup                                 # ← 同时产出安装版
```

`build-desktop.sh:19-22`（macOS 链路，**残缺**，本次实际使用的就是它）：

```bash
if [[ "${SKIP_DESKTOP_TESTS:-0}" != "1" ]]; then
  (cd desktop && npm ci)      # ← npm ci 会整个重装 node_modules，
fi                            #    把之前手工拷进去的定制模板一并抹掉
(cd desktop && npm run pack:win)   # ← 既没有覆盖模板，也不产出安装版
```

`desktop/nsis/portable.nsi` 的注释写着"构建脚本会在打包前自动完成模板覆盖"，
但这句话只对 PowerShell 脚本成立。**定制模板的唯一生效位置在 `node_modules` 里，
而 `npm ci` 每次都会把它冲掉**——这是一个"靠运气生效"的设计。

### 事实 5：所以用户跑的到底是什么？

上游模板每次启动都全新解压、退出即删，**不会产生陈旧副本**。
因此用户运行的必然是**磁盘上另一个程序**，最可能是：

- 之前某一轮的 `Deep Legends Setup.exe` 装出来的版本
  （NSIS 用户级安装，落在 `%LOCALAPPDATA%\Programs\Deep Legends\`，
  卸载项在"设置 → 应用"里，未必出现在老版控制面板列表）；或
- 下载目录里残留的旧 `Deep Legends.exe`（同名、无版本号，肉眼无法区分）。

**这一条我没有直接证据，是排除法推出来的**，所以下面 D 项专门解决"如何一眼看出在跑哪个构建"。

---

## 二、根因归纳（改的时候对着这四条，不要只改表面）

| # | 根因 | 造成的后果 |
|---|---|---|
| R1 | 定制 portable 模板只存在于 `node_modules`，靠构建脚本手工 `Copy-Item` 生效，且 `npm ci` 会抹掉它；macOS 脚本干脆没有这一步 | 打出来的便携版行为不确定，取决于当时 node_modules 的状态 |
| R2 | 定制模板的缓存键 = `VERSION` + **手工维护**的 `DEEP_LEGENDS_CACHE_REVISION`，两者都不随构建内容变化 | 一旦定制模板生效，同版本重打的包会**静默复用旧缓存**，改动全不生效 |
| R3 | 构建产物没有任何可见的身份标识：`main.go:30 var version` 虽由 `-X` 注入、`/api/status` 也返回了（`main.go:433`），但**前端从不显示**；产物文件名 `Deep Legends.exe` 不含版本 | 跑错版本时用户和我都无法察觉，只能靠猜 |
| R4 | 两条构建链路能力不对等（macOS 少了模板覆盖与安装版产出），且都需要人记住若干手工步骤 | 每次打包都"特别麻烦"，且漏步骤不报错 |

---

## 三、任务

### A【P0】把定制模板的应用做成构建的必经步骤，且不可能被 npm ci 冲掉

新建 `desktop/apply-portable-template.cjs`，两条构建链路都调用它（避免 bash / PowerShell 各写一份逻辑）：

```js
"use strict";
// 把定制 portable 模板写入 electron-builder 的模板位置，并注入构建指纹。
// electron-builder 对 portable 目标硬编码读取内置模板路径，无法通过配置替换，
// 因此只能覆盖文件；必须在 npm ci 之后、打包之前执行。
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const source = path.join(__dirname, "nsis", "portable.nsi");
const target = path.join(__dirname, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
const backend = path.join(__dirname, "backend", "loot-service.exe");

function buildFingerprint() {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(backend));
  for (const name of ["main.cjs", "preload.cjs", "proxy-resolution.cjs"]) {
    hash.update(fs.readFileSync(path.join(__dirname, name)));
  }
  return hash.digest("hex").slice(0, 12);
}

function applyPortableTemplate() {
  if (!fs.existsSync(source)) throw new Error(`定制 portable 模板缺失：${source}`);
  if (!fs.existsSync(target)) throw new Error(`electron-builder 模板位置不存在（是否漏了 npm ci？）：${target}`);
  if (!fs.existsSync(backend)) throw new Error(`后端二进制缺失，请先构建 Go：${backend}`);
  const fingerprint = buildFingerprint();
  const script = fs.readFileSync(source, "utf8");
  if (!script.includes("@@BUILD_FINGERPRINT@@")) {
    throw new Error("定制模板缺少 @@BUILD_FINGERPRINT@@ 占位符");
  }
  fs.writeFileSync(target, script.replaceAll("@@BUILD_FINGERPRINT@@", fingerprint), "utf8");
  return fingerprint;
}

module.exports = { applyPortableTemplate, buildFingerprint };

if (require.main === module) {
  console.log(`portable template applied, fingerprint=${applyPortableTemplate()}`);
}
```

### B【P0】缓存键改为自动派生，删掉手工 REVISION

改 `desktop/nsis/portable.nsi`：

```nsis
# 缓存目录带构建指纹（后端二进制 + 桌面壳源码的 SHA-256 前 12 位），
# 由 desktop/apply-portable-template.cjs 在打包前注入。
# 任何一次内容变化都会自动换目录，不再需要人工递增版本号或修订号。
!define DEEP_LEGENDS_BUILD_FINGERPRINT "@@BUILD_FINGERPRINT@@"
...
StrCpy $INSTDIR "$LOCALAPPDATA\${APP_FILENAME}\app-${VERSION}-${DEEP_LEGENDS_BUILD_FINGERPRINT}"
```

同时把 `.deep-legends-ready` 的内容也改成写入 `${VERSION}-${DEEP_LEGENDS_BUILD_FINGERPRINT}`。

**删除 `!define DEEP_LEGENDS_CACHE_REVISION "3"` 及其所有引用。**
保留"缓存未命中时先 `RMDir /r "$LOCALAPPDATA\${APP_FILENAME}"` 清理旧版本"的行为——
它能顺带清掉历史遗留的 `app-0.11.2-r3` 等目录。

### C【P0】beforePack 增加"模板已正确应用"的硬校验

扩展 `desktop/verify-embedded-riot-key.cjs`（或新建 `desktop/verify-package-preconditions.cjs`
并在 `desktop/package.json:35` 的 `beforePack` 指向它），在原有 Riot key 校验之外增加：

1. `node_modules/app-builder-lib/templates/nsis/portable.nsi` 必须含
   `DEEP_LEGENDS_BUILD_FINGERPRINT`，且**不含** `@@BUILD_FINGERPRINT@@`（占位符必须已被替换）；
2. 该文件里的指纹值必须等于当前 `buildFingerprint()` 的计算结果
   （防止用上一次构建残留的模板打包）；
3. 任一条不满足就 `throw`，让打包失败。

这一条是本单的核心护栏：**R1 那类"漏了一步却照样打出包"从此不可能再发生**。

### D【P0】让"正在跑哪个构建"一眼可见

三处都要做，缺一不可：

1. **构建指纹注入 Go**：两条构建脚本的 `-ldflags` 追加
   `-X main.buildFingerprint=<指纹>`（`main.go` 加 `var buildFingerprint = "dev"`）。
   注意指纹要在 Go 构建**之后**才能算（依赖二进制本身），
   所以顺序改为：先普通构建 → 算 `loot-service.exe` 的哈希 → 用该哈希作为指纹**重新构建一次**，
   或更简单：指纹只取"源码指纹"（`*.go` + `web/*` + `desktop/*.cjs` 的哈希），
   构建前即可算出，一次构建搞定。**推荐后者**，实现更简单且同样满足"内容变则指纹变"。
2. **前端显示**：`/api/status` 已经返回 `version`（`main.go:433`），
   在设置页底部增加一行"版本 0.11.2 · 构建 a1b2c3d4e5f6"。
   `web/app.js` 目前对 `status.version` **零引用**，需要新增渲染。
3. **产物文件名带指纹**：`desktop/package.json` 的
   `portable.artifactName` / `nsis.artifactName` 改为
   `Deep Legends ${version}-${env.DEEP_LEGENDS_FINGERPRINT}.${ext}` 之类，
   让下载目录里两个不同构建**不可能同名**。
   （electron-builder 支持 `${env.XXX}` 插值，构建脚本导出该环境变量即可。）

### E【P1】两条构建链路对齐，各自都能一条命令打全

`build-desktop.sh` 补齐（顺序很重要）：

```bash
# 1) Go 构建（注入 version + fingerprint + riot cipher）
# 2) cd desktop && npm ci
# 3) node apply-portable-template.cjs      ← 必须在 npm ci 之后
# 4) npm run pack:win
# 5) npm run pack:win-setup                ← 便携版与安装版必须分两次调用，
#                                             原因见 build-desktop-windows.ps1:110-112 的注释
```

`build-desktop-windows.ps1` 把第 104-108 行的 `Copy-Item` 换成调用同一个
`node apply-portable-template.cjs`，两边共用一份逻辑。

另外把 `build-desktop.sh:19` 那个 `SKIP_DESKTOP_TESTS` 变量改名——
它控制的是 `npm ci` 而不是测试，名字有误导性（建议 `SKIP_NPM_INSTALL`）。

### F【P1】护栏补齐（现有的是假护栏）

`desktop/portable-template.test.cjs:14-15` 现在只断言 `DEEP_LEGENDS_CACHE_REVISION`
**存在**、路径模板**引用了**它，**从不校验它的值是否变化**——
"每次发布必须手工递增某常量"这类约束天生测不出来，这也正是 B 项要直接删掉它的原因。

改完后新增/改写测试：

1. `desktop/nsis/portable.nsi` 含 `@@BUILD_FINGERPRINT@@` 占位符，
   且**不再**含 `DEEP_LEGENDS_CACHE_REVISION`；
2. `applyPortableTemplate()` 行为测试：在临时目录里造一份假 `node_modules` 模板位置，
   跑一次，断言目标文件里占位符已被 12 位十六进制替换；
3. 后端二进制内容变化时指纹必须变（改一个字节 → `buildFingerprint()` 结果不同）；
4. beforePack 前置校验测试：占位符未替换 / 指纹不匹配 两种情况都必须 `throw`。

**每条新断言都要用变异体自验**（本项目反复踩过假护栏，见验收清单最后一节）。

---

## 四、给用户的即时操作（GPT 不用做，但改完要在 README 写清楚）

这次的包用户拿到后应当：

1. 先确认磁盘上**只有一个** `Deep Legends.exe`，把旧的下载全删掉；
2. 检查是否存在旧的安装版：`%LOCALAPPDATA%\Programs\Deep Legends`，
   有就从"设置 → 应用"里卸载；
3. 顺手删掉 `%LOCALAPPDATA%\Deep Legends`（本次不存在，属正常，删不掉忽略即可）；
4. 再运行新 exe，进设置页确认版本与构建指纹与本次构建一致（D2 做完之后才有这一步）。

---

## 五、验收清单

- [ ] A 新建 `desktop/apply-portable-template.cjs`，两条构建脚本都调用它，且都在 `npm ci` 之后
- [ ] B `portable.nsi` 缓存目录带自动指纹；`DEEP_LEGENDS_CACHE_REVISION` 已彻底删除
- [ ] C beforePack 会因"模板未应用/指纹过期/占位符未替换"而**打包失败**（三种情况各手工验一次）
- [ ] D1 二进制含 `buildFingerprint`；D2 设置页显示版本+指纹；D3 产物文件名含指纹
- [ ] E `./build-desktop.sh` 一条命令产出便携版 + 安装版，与 PowerShell 脚本能力对等
- [ ] F 四条新测试全绿，且每条都用变异体验证过是真护栏
- [ ] `go test ./...` + `node --test web/champions.test.cjs` + `node --test desktop/portable-template.test.cjs` 全绿
- [ ] `gofmt -l` 只剩既有的 `yourgg_arena.go`

**测试纪律**（本项目反复踩过，务必遵守）：

1. 跑测试用 `node --test web/champions.test.cjs`，**不能用 `node --test web/`**。
2. 前端/脚本类正则断言极易是假护栏，**每条新断言都要先造变异体确认能被杀掉**。
3. 变异测试拷副本时必须拷**真副本**（不能软链接），且要带 `desktop/` 整个目录、
   `prestige_chromas.json`（`prestige.go:20` 有 `go:embed`）；
   **跑变异体之前先跑一次未变异的对照组，确认 `# fail 0`**，否则所有结论都是假的。
4. Go 里把条件改成 `if false` 会触发 "declared and not used" 编译错误 → 假杀，
   要换成仍然引用原变量的变异形式。
