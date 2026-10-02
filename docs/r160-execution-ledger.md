# R160 执行账本

## 诊断基线

- 日志版本 0.12.24，`build_fingerprint=01178a4727f7`。海斗详情两次 1112/545ms，Hexdata 图标域名 16 次请求零失败；图片内容取了上游 `*_small.png`。
- 斗魂 `arena_augments_built` 两次均 `rowsOut=45, missingMeta=45`；CommunityDragon 增益目录取消/超时，装备目录一次 12000ms，YOUR.GG 一次 12000ms。
- 总览首次非流式请求 4528ms，其中 `champion_names=2878ms`；流式重访 `matchIDs=748ms`。多标签共享 Riot 限流且无 429；未知 App Key 实际额度，未改配额。

## 改动

1. `backend/data/augment_catalog_20260924.json` 与 `augment_icons_20260924.bin` 按官方 ID 打包中文元数据和 543 张 PNG。代理只接受 `/augments/{已收录 ID}.png`，防止任意文件读取；前端把 `builtin:` 资源放本机图片通道。
2. 海斗推荐、斗魂 YOUR.GG/OP.GG 海克斯卡、战绩全局增益目录和海克斯图鉴统一使用已验证的本地完整图。没有图的 ID 沿用现有来源或占位。
3. 斗魂同步路径读取本地增益目录及本地装备元数据；仅在本地目录意外不可用时回退现有在线加载。日志里 YOUR.GG 聚合另有 12 秒超时，现将详情等待限制为 4 秒；超时仍走 OP.GG 并记录 `arena_aggregate_fallback`，不把回退行伪装成 YOUR.GG 统计。
4. 总览使用 Data Dragon 16.19.1 的 173 个英雄中文名和对应方形 PNG 本地快照，省去当前版本的首屏目录等待及离线韩服页签的头像远程请求；本地海克斯目录单独从 `/api/gameplay/augments` 返回，卡片图标不再受整份符文目录加载阻塞。版本已知不匹配时英雄名和头像保持原有联网路径。Match-V5 的 1 分钟缓存和 8 路详情并发不变。
5. 图片队列新增成功慢图采样 `image_queue_slow`，只含排队/加载耗时、当时慢图数和来源枚举，不含路径或玩家信息；与现有失败诊断独立，避免成功事件挤掉失败记录。

## 验证

- 本地增益图按偏移逐张复核 PNG 块 CRC 与解压：554 个目录 ID，543 张有效图（500 张 256×256、34 张 64×64、9 张 512×512）；截图中的 9 个斗魂 ID 与 2 个海斗 ID 均为 256×256。173 张英雄方形图均为 128×128 有效 PNG；未知 ID 不伪造名称和图标。
- `go test ./... -count=1`：2292 通过、22 跳过、0 失败，后端包耗时 192.724 秒。针对性 `TestR160` 与来源回退测试也通过。
- 本机及 Windows amd64 后端验证程序均按 0.12.25、源码指纹 `ff608c533a04` 构建。key mode 为 **public**，未注入 Riot API Key；Windows 产物名为 `Deep-Legends-0.12.25-public.exe`，内嵌密钥分发校验通过。本机程序 `-self-test` 输出 0.12.25 自检通过；未打 Electron 安装包。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：979 通过、1 跳过、0 失败，耗时 268.679 秒。`git diff --check` 与前端脚本语法检查通过。Windows 真机复核待补。

## 真机复核边界

本机没有用户当时的 Windows 联盟客户端与同一 Riot/SGP 会话。最终应在 0.12.25 包上查看海斗/斗魂截图与新诊断日志，并分开比较详情响应、图片可见时间、Match-V5 的 `matchIDs` 与 `limiter_queue_ms`。当前没有证据支持提高 Riot 限流值。
