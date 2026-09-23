# WORKLIST-R124：R123 头像/旗帜收藏子页——拥有状态不可用时「显示未拥有」关闭会把网格清空

诊断人：Claude（只读复核：构建 + 测试 + 对抗复现，未改仓库任何代码文件）。
执行人：GPT。
日期：2026-09-22。
基线：R123 相关文件（`backend/web/favorites-facade.js` 等，git status 显示尚未提交）。
状态：复核完成，1 个需要修的真实缺口（P1），2 项非阻塞观察（P2），其余全部核对通过。

---

## 0. 结论摘要

1. R123 的实现和确认过的设计方案高度吻合：写入代码（`writeFacadeIcon`/`writeFacadeIconResult`/`writeFacadeBanner`/`writeFacadeBannerPreferences`）全部清除，UI 触发点（选择头像/选择旗帜按钮、旗帜已应用文案等）grep 不到任何残留；收藏页新子页的标题、副标题、快捷分类（头像：全部头像/近三年新增；旗帜：全部/国服专属，均不含已拥有/未拥有）都和最终确认的设计一致；隐私声明 `explicitWrites` 两处改动文案和对话里确认的逐字一致；`go build`/`go vet` 全绿，`r123.test.cjs` 6 个测试全绿。
2. 独立复核发现一个真实缺口：**拥有状态读取失败（`iconOwnershipUnavailable`/`bannerOwnershipUnavailable` 为 true）时，如果用户此前把"显示未拥有"关掉过，整个网格会被过滤到 0 条，且此时开关本身被隐藏，没有任何入口能自己恢复**。已经用贴近后端真实行为的数据直接跑 `favorites-facade.js` 里的真实函数复现（3/3、2/2 全部被滤空），现有 `r123.test.cjs` 没有一条测试覆盖这个组合，6 个测试全绿掩盖了这个问题。
3. 两项非阻塞观察：`desktop/package.json` 版本号还停在 0.12.15，没有随本次改动递增；从生涯页跳到收藏页旗帜视图时会多打一次不必要的头像目录请求。

## 复核方法

- 全量阅读改动：`index.html` 新增的 `favorites-facade-panel`、`favorites-facade.js`（401 行全文）、`suite.js` 里生涯页头像/旗帜卡的改动、`backend/features.go` 的隐私声明改动、`backend/facade_icons.go`/`facade_banners.go`/`profile_facade.go` 的写入代码删除范围。
- 构建/测试：这台设备没有装 Go 工具链，把 `backend/` + `go.mod`/`go.sum` + 仓库里现成的 `.gomodcache`（已解压的依赖源码树）搬到一个有 Go、但没有公网出口的环境，用 `replace` 指令指向已解压的模块目录做离线构建：
  - `go build ./backend/...`、`go vet ./backend/...` 全绿；
  - `go test -run 'Facade|R99|R101|R105|R107|R108|Quality|Feature|R123' ./backend/...`：除一条外全绿——`TestR105_EmbeddedAccountCounts` 因为本轮复核环境裁剪时没带 `docs/pro-accounts-verification-2026-09-17.md`（该测试用相对路径 `../docs/...` 读这个文件），是复核环境本身的裁剪问题，不是代码缺陷，仓库里这个文件确实存在；
  - 跑整个 `./backend/...` 会在个别真实调用 Riot 官方接口/ddragon 的测试上卡死到超时（这台环境的出网策略不放行这些域名，跟 R123 无关），本轮全量测试没有覆盖到，如实说明，不代表这部分测试在正常环境里也会红。
- JS 侧：`node --test backend/web/r123.test.cjs`（6/6 绿）；另外把 `favorites-facade.js` 的纯函数直接抽出来，用贴近后端真实行为的数据（`owned` 全部为 `false`，对应"拥有状态不可用时 owned 一律 false"的真实实现）做了一次对抗复现，见下面 P1 的证据。

---

## P1　拥有状态不可用 + 显示未拥有=false 时网格被清空，且开关不可见、无法自行恢复

**证据（真实复现，直接跑仓库里的函数）：**
```
$ node -e '...(抽取 favorites-facade.js 里真正的 facadeCollectionIconRows / facadeCollectionBannerRows)...'
icons total=3 rows after filter=0
banners total=2 rows after filter=0
```
用的数据贴合后端真实行为：`backend/facade_icons.go` 第 116-118 行，`unknown`（即 `iconOwnershipUnavailable`）为真时 `owned` map 是空的，所以每一个 icon 的 `Owned` 字段都会是 `false`；`backend/facade_banners.go` 里 `loadFacadeBanners` 同理（`Owned: !unknown && inventory[item.ID].Owned`）。

`backend/web/favorites-facade.js` 里：
```js
function facadeCollectionIconRows(icons, filters, currentYear) {
  ...
  if (!filters.showUnowned && !icon.owned) return false;   // 没有把「拥有状态是否可用」传进来
  ...
}
```
`facadeCollectionBannerRows` 是同样的写法。这两个函数都不接收"拥有状态是否可用"这个参数，所以当它为真（不可用）时，`!icon.owned` 对每一条都成立；只要 `filters.showUnowned` 之前被用户关掉过，`!filters.showUnowned && !icon.owned` 就对全部条目成立，网格被滤成 0 条。

同时 `syncControls()` 里：
```js
el.unownedControl.hidden = unavailable;
```
只是把控件隐藏，没有把 `filters[state.view].showUnowned` 重置为 `true`。于是一旦触发（例如：用户在拥有状态正常时关掉过"显示未拥有"，之后客户端一次瞬断重连或 inventory 接口一次读取失败，导致这次目录变成"拥有状态不可用"），用户看到的是"没有符合条件的条目"的空态，而本该能救场的"显示未拥有"开关这时候恰好被隐藏，没有任何 UI 入口能回到能看见目录的状态，除非等到下一次拥有状态能成功读取。

**影响：** 需要"先关过开关"+"之后一次拥有状态读取失败"两个条件叠加才会触发，不是每次都中；但一旦触发，用户看到的是一个看起来完全空白、且找不到开关来源的收藏页，跟现有实现里字段映射那层已经做对的降级逻辑不一致——`facadeCollectionIconFields`/`facadeCollectionBannerFields` 已经正确地在 `ownershipUnavailable` 时不套用锁定态（`locked: !ownershipUnavailable && !icon.owned`），筛选那层（`facadeCollectionIconRows`/`facadeCollectionBannerRows`）漏了同样的判断。

**修复方向：**
1. `facadeCollectionIconRows`/`facadeCollectionBannerRows` 增加一个 `ownershipUnavailable` 参数，过滤条件改成 `if (!ownershipUnavailable && !filters.showUnowned && !icon.owned) return false;`（`currentRows()` 调用处把 `ownershipUnavailable()` 的结果传进去）。
2. 作为双重保险，`syncControls()` 判定 `unavailable` 为真时，同时把 `state.filters[state.view].showUnowned` 强制设回 `true`（不只是隐藏控件），避免拥有状态恢复正常之后开关的记忆值还是错的。

**验收判据：**
- 用本工单证据里那组数据（3 个 icon / 2 个 banner，`owned` 全 false，`showUnowned: false`）直接跑 `facadeCollectionIconRows`/`facadeCollectionBannerRows`，改完后必须返回全部条目（3、2），不是 0；
- 在 `r123.test.cjs` 补一条测试钉死这个组合场景（拥有状态不可用 + `showUnowned:false` → 不能过滤掉任何条目），对抗变异：把修复后加的 `!ownershipUnavailable &&` 条件去掉，新测试必须 FAIL；
- 人工核对：拥有状态不可用时，即使 `filters.showUnowned` 之前是 `false`，`syncControls()` 跑完后 `el.showUnowned.checked` 应该是 `true`（即便控件本身隐藏，状态也要是对的，为下一次拥有状态恢复正常做准备）。

---

## 已复核无需返工

- 写入代码与 UI 入口清理彻底：`grep -rn "writeFacadeIcon\b|writeFacadeIconResult|writeFacadeBanner\b|writeFacadeBannerPreferences" backend/*.go`（排除 `_test.go`）与 `grep -rn "openFacadeIconPicker|openFacadeBannerPicker|设为头像|旗帜已应用|立即更改" backend/web/*.js` 均无匹配。
- 段位旗（`writeFacadeRankBanner`）与挑战偏好写入未受影响，仍然只在 `rank-banner` action 里、每次都先读原值再回写，没有被顺手改动。
- 隐私声明 `backend/features.go:749` 的两处改动和对话里确认下来的文案逐字一致。
- `go build ./backend/...`、`go vet ./backend/...` 全绿；`r123.test.cjs` 6/6；目标子集 Go 测试除环境裁剪导致的一条无关失败外全绿。
- 收藏页新子页的标题/副标题/快捷分类/CSS（`.facade-quick-groups`、`.facade-metrics`、`.facade-grid.is-banners` 等）都在，旗帜卡网格做了非正方形比例适配（`object-fit: contain`），细节到位。

---

## P2　非阻塞观察（不要求本轮处理）

1. `desktop/package.json` 版本号仍是 0.12.15，没有随本次改动递增；按项目惯例（每个功能性工单都配一次版本号更新）建议下次打包前补一次。
2. `app.js:2263` 的 `deepLegendsOpenFacadeCollection`：从生涯页点"在收藏页浏览"跳旗帜视图时，会先调用 `activate()`（默认视图 `icons`，发一次 `/api/facade/icons`），再调用 `setView("banners")` 发第二次请求，多打了一次不必要的头像目录请求。不影响正确性（`load()` 里有 `state.view === view` 的守卫，不会渲染错视图），只是浪费一次网络请求，可以让 `activate()` 接受一个可选的目标 view 参数来合并成一次。
