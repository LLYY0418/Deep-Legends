# R162 执行账本

## 图二与诊断日志

诊断文件：`/Users/ly/Downloads/lol-loot-diagnostics-0926-0046.jsonl`。文件同时包含 0.12.24 与 0.12.26 的运行段（第 3、9277 行的 `app_start`）；下表时间为日志 UTC。

| 结算时间 | 文件行 | 捕获结果 |
|---|---:|---|
| 09-25 13:36:51 | 1295 | `lp_capture_skipped`, `games_jumped` |
| 09-25 14:26:36 | 4246 | `lp_capture_recorded` |
| 09-25 14:56:19 | 6383 | `lp_capture_skipped`, `games_jumped` |
| 09-25 15:38:19 | 10342 | `lp_capture_recorded` |
| 09-25 16:08:26 | 13035 | `lp_capture_skipped`, `games_jumped` |
| 09-25 16:45:59 | 15776 | `lp_capture_recorded` |

三次跳过前均有 `has_baseline:true`，第一次轮询也均达到 `capability_state:available`；13:36 和 16:08 的相邻数据源决策选择了 LCU（第 1292–1295、13033–13035 行），所以这些局不是排位接口 HTTP 失败。16:46 的 SGP 战绩请求成功返回 20 局（第 15803 行），参与者字段有 `win`，没有单局 LP 增量（第 15802 行）。

前端 `renderMatch` 对排位战绩的 `lpDelta` 正负值都显示。后端只在结算后快照相对基线恰好增加 1 场时记录单局 LP；增加超过 1 场即主动跳过，避免把多场的 LP 差误标到一场。截图的胜负与日志的交替结果高度吻合，但日志不含玩家身份、单局胜负和跳跃的具体场次值，不能仅凭它逐局证明账号与胜负，也不能计算缺失局的 LP。无法从这份日志区分基线过旧和上游场次计数异常；新增 `games_gap` 供后续诊断，不回填未知数值。

## 改动

1. 详情中双方队伍按已知位置稳定排序。位置重复时保留先后，未知排末尾；移除默认关闭且会因位置重复而完全放弃排序的旧开关。当前玩家高亮仍随玩家数据走。
2. 保留双方共享行高和当前玩家整行背景，战绩小卡的 flex 行内容向底部对齐，不再把单行卡片拉高。
3. `lp_capture_skipped` 的 `games_jumped` 诊断增加仅包含场次数差的 `games_gap`；隐私测试继续禁止账号、段位、LP 和胜负场详情入日志。
4. 桌面版本提升到 0.12.27。

## 验证

- `go test ./...`：通过（后端 191.699 秒）。首次尝试因沙箱无法访问系统默认 Go 缓存而未启动测试；经缓存权限批准后完整运行通过。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：982 通过、1 跳过、0 失败。最后一次设置文案调整后重跑 `backend/web/gameplay.test.cjs`：37 通过。
- `node --check backend/web/gameplay.js` 与 `git diff --check`：通过。
- 使用系统默认 Go 缓存完成本机及 Windows amd64 后端验证构建，版本均为 0.12.27，源码指纹 `35d53c2edd5c` 均经 `verify-build-fingerprint.cjs` 校验。两个产物分别为 `/private/tmp/Deep-Legends-0.12.27-public` 与 `/private/tmp/Deep-Legends-0.12.27-public.exe`。**Key mode：public**；两者均经嵌入密钥策略检查确认无 Riot Key。未打 Electron 安装包。
- 本机未运行用户的 Windows League 客户端。卡片的最终视觉效果及新 `games_gap` 数值需 Windows 真机新日志复核；当前日志来自旧版本，不含该字段。
