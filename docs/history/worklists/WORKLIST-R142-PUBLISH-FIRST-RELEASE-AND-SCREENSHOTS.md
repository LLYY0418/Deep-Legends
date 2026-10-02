# WORKLIST-R142：首次发布到 GitHub Release（0.12.19）+ 演示截图

诊断人：Claude（只读诊断，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：HEAD `21f76280`（"Enlarge banner tiles for R141"），版本 0.12.19。
状态：未执行。

## 背景

README「发布流程」和 R85（`docs/history/WORKLIST-R85-FIRST-RELEASE.md`）已经把公开发布这条路径走通过一次（当时目标 0.13.0），但止步于"生成三个发布文件"，从未真正创建过 GitHub Release；之后版本号退回 0.12.x 继续迭代，`CHANGELOG.md`「未发布」一节从 0.12.6 一路堆到现在的 0.12.19，从来没有关闭过。本单目的：把「未发布」内容归档成正式版本小节，跑通同一条发布流程，并按要求补一组界面截图供写 Release 说明用。创建/上传 GitHub Release 本身仍不在本单范围内——原因见 R85，边界不变。

## P1　CHANGELOG 归档（对应发布流程第 1 步）

1. 把「## 未发布」下的全部内容——`R136·0.12.19`、`R132·0.12.18`、`R115 评分复核·0.12.7`、`R115·0.12.6` 四组，以及再往下没有 R 编号的 `### 优化` 列表——合并整理到新的一节 `## 0.12.19 — 2026-09-23` 下面。这是项目第一次真正发布，说明第一行直接写清楚"首次公开发布，一并包含 0.12.0 起累计的全部改动"，避免使用者以为只更新了一点点。
2. 按 `### 新增 / ### 优化 / ### 修复` 分类改写成用户能看懂的语言，去掉「R136」「P1」这类内部编号和实现细节；纯内部工程性改动（测试基础设施、诊断埋点等）不要塞进来。
3. 标题格式必须跟 `## 0.12.3 — 2026-09-17` 完全一致（`scripts/make-release.cjs` 的 `latestNotes()` 靠正则 `^##\s+(\S+)\s+—\s+\d{4}-\d{2}-\d{2}` 匹配，格式不对会直接报错）。
4. 「## 未发布」标题本身保留在最上面，下面留空。
5. 版本号不用改：`desktop/package.json`、`desktop/package-lock.json` 已经都是 `0.12.19`，核对一致即可。

## P2　执行 public 构建 + 生成发布文件（对应发布流程第 2 步）

```powershell
./build-desktop-windows.ps1 -Version 0.12.19 -KeyMode public
node scripts/make-release.cjs
```

- 必须在真实 Windows 环境跑完整 NSIS 打包，不是交叉编译占位。
- 不要绕过或注释 `verifyRiotKeyPolicy` / `verifyPublicReleaseBuild`。
- `dist/release/` 下应该正好三个文件（Setup、`latest.json`、`SHA256SUMS-public.txt`；`-public` 后缀按项目约定保留，验证完不能留下与 private 同名的包）。

### 验收

1. `node scripts/make-release.cjs` 成功退出，打印版本号、指纹、Setup 的 SHA-256。
2. `latest.json`：`version` 是 `0.12.19`；`asset.name` 无空格、无指纹后缀；`asset.url` 是 `https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.19/<asset.name>`；`notes` 与 CHANGELOG 新增的 `## 0.12.19` 一节内容一致。
3. `Get-FileHash -Algorithm SHA256` 核对 EXE 和 `latest.json`，与 `SHA256SUMS-public.txt` 完全一致。
4. 用 `desktop/verify-embedded-riot-key.cjs` 或文本搜索双重确认 EXE 里不包含任何看起来像 Riot API Key 的字符串。
5. 跑 `node --test scripts/make-release.test.cjs desktop/release-build.test.cjs`，以及 `go test -run TestUpdate ./...`。

## P3　演示截图（供写 Release 说明用，不进 App 界面）

1. 用项目自带的演示数据层跑前端，不需要真实客户端、不需要 Riot Key：启动 `go run ./backend`（或用任意静态服务器托管 `backend/web/`），浏览器打开首页并在地址栏加 `?demo=1`（`backend/web/runtime.js` 靠这个参数触发 `demo-data.js` 接管所有 API 请求）。
2. 页面右下角会常驻一条黄色「演示数据 · 仅样式预览」提示条（`backend/web/app.css` 的 `.demo-flag`，由 `backend/web/demo-data.js` 生成）——这是刻意留的防混淆设计，**不要改代码删掉它**。截图时用浏览器 DevTools 临时执行 `document.querySelector('.demo-flag').style.display='none'`（或者截图后裁掉底部约 34px），只影响本机截图，这行改动不要提交进仓库。
3. 依次点开左侧导航（`#sidebar` 的 `section-tabs`）六个菜单——总览、英雄、对局、收藏、工具、设置——每个存一张全窗口截图到 `docs/r142-validation/`，命名 `01-overview.png` … `06-settings.png`（数字前缀保证顺序）。
4. 用这六张图起草一份 Release 说明文案，存成 `docs/r142-release-notes-draft.md`：开头一两句项目介绍，中间按导航顺序放六张图各配一行说明，底部附上 CHANGELOG 新增的 `## 0.12.19` 一节内容。这份文件是给用户去 GitHub 网页创建 Release 时复制粘贴用的草稿，不是发布本身——图片本地相对路径在 GitHub Release 说明里显示不出来，草稿里明确写一句「以下 6 张截图上传 Release 后替换成真实链接」，不要放一个会 404 的相对路径当正式链接。

### 验收

- 六张截图都在 `docs/r142-validation/`，肉眼确认没有「演示数据」黄条、没有明显的加载中占位符。
- `docs/r142-release-notes-draft.md` 存在且内容完整。
- 截图相关的调试改动没有被提交（`git status`/`git diff` 干净，只有 CHANGELOG 和新增的截图文件）。

## 交付物

- `dist/release/` 三个发布文件。
- `docs/r142-validation/` 六张导航截图。
- `docs/r142-release-notes-draft.md`。
- 更新后的 `CHANGELOG.md`。

交回后由用户自己在 GitHub 仓库 `LLYY0418/Deep-Legends` 创建 `v0.12.19` Release，上传三个发布文件，把说明草稿贴进去并替换真实图片链接。

## 不要做的事

- 不要自己创建/发布 GitHub Release，不要以任何自动化方式推送到 `LLYY0418/Deep-Legends`——沿用 R85 的边界，没有这个权限。
- 不要为了截图修改 `.demo-flag` 相关的生产代码，截图后也不要遗留任何隐藏它的调试代码。
- 不要把发布说明草稿里的截图或说明文字带回 App 界面本身（红线见 `CLAUDE.md`「界面文案红线」）。
- 不要跳过 `verifyPublicReleaseBuild` / `verifyRiotKeyPolicy`，不要把 CHANGELOG 写成营销文案。

## 验收总表

| 项 | 判据 |
|---|---|
| CHANGELOG | 「未发布」清空，新增 `## 0.12.19 — 2026-09-23`，格式匹配 `latestNotes()` 正则 |
| 构建 | `dist/release/` 恰好三个 public 文件，`make-release.cjs` 成功 |
| 校验 | SHA256、`latest.json` 字段、Key 泄漏检查、回归测试全部通过 |
| 截图 | 6 张导航截图存在、无演示黄条、无调试改动遗留 |
| 说明草稿 | `docs/r142-release-notes-draft.md` 内容完整 |
| 边界 | 未创建/上传 GitHub Release |
