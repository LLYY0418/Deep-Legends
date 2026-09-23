# R126 执行记录：生涯背景原皮目录错配

日期：2026-09-22  
执行人：GPT  
工单：`docs/WORKLIST-R126-CAREER-BACKGROUND-BASE-SKIN-MISMATCH.md`  
最终版本：0.12.16（验证时 `desktop/package.json` 与 `package-lock.json` 已同步，本轮未重复覆盖并行版本改动）

## 1. 执行范围

完成工单 P1-a 至 P1-d；P2 明确为非阻塞建议，本轮不扩做 URI 聚合诊断或后端事件字段差异广播。

## 2. 实施结果

- `Snapshot` / `app` 新增含原皮的完整目录 `AllWithBase` / `allSkinsWithBase`。
- 完整目录在 `applySkinOwnership` 之后保存，原皮继续按“拥有英雄即拥有原皮”的既有规则计算。
- 收藏页仍使用剔除原皮的 `allSkins`，现有收藏展示、奖池和统计语义不变。
- 生涯页只复用 `allSkinsWithBase`；字段未就绪时回到 LCU 完整目录，不再回退到剔除原皮的收藏目录。
- `facadeProfile` 新增 `backgroundChampionId`。自动背景直接保留客户端 `championId`，匹配具体皮肤后以目录英雄 ID 覆盖。
- `facade_skin_state` 与 `facade_backdrop_read` 诊断增加 `background_champion_id`。
- 前端英雄兜底顺序改为：匹配皮肤英雄 -> `backgroundChampionId` -> 皮肤 ID 推导 -> 空。
- 英雄选择器加入“请选择英雄”；目录缺少已知英雄时显示 `英雄 <ID>`，不会视觉上落到首个英雄。
- 背景 dirty 判定和刷新签名纳入 `backgroundChampionId`，自动背景变化能刷新，用户已经修改的背景草稿不会被 SSE 覆盖。
- 原皮进入生涯目录后沿用现有 `draft.skinId` 高亮和 `backgroundApplied` 逻辑，当前原皮高亮且“应用背景”按钮隐藏。
- 断连、失败快照和内存快照复制均同步处理完整目录，避免跨账号残留。

## 3. 自动化验收

已通过：

- 生产后端构建：`go build -o "$PI_SCRATCH_DIR/r126-backend-final-live" ./backend`。
- 后端完整测试：`go test ./backend`，整包 PASS，耗时 199.661 秒。
- R126 后端测试 4/4：
  - 收藏已加载、展示目录无原皮时，自动李青背景仍解析为 `64000`，英雄为 `64`，名字为原皮名称，目录内可高亮。
  - 目录完全无法匹配时，皮肤 ID 保持 0，但英雄 ID 仍为 `64`。
  - 自动背景图片路径不可用时，仍保留客户端明确返回的英雄 ID。
  - 收藏刷新必须把 `Snapshot.AllWithBase` 安装到运行态，删除该赋值时测试失败。
- R126 前端测试 3/3：显式英雄兜底、无证据保持空、刷新签名包含英雄 ID。
- 生涯相关前端联合回归 49/49。
- 前端完整测试 628/628。
- `git diff --check` 通过。

## 4. 真机边界

当前环境没有可连接的 Windows 英雄联盟客户端，不能完成工单中的真机步骤：先打开收藏页加载目录，再进入生涯页确认右侧为李青、原皮高亮、左右一致。

内置 HTML 预览已打开；浏览器自动化快照权限被拒绝，未进行伪造点击验证。真机验收应同时确认诊断 `facade_skin_state.background_in_catalog == true`。
