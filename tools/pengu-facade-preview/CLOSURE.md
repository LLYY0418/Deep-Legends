# 未拥有头像/旗帜「本地可见」实验 —— 结论存档（已放弃）

状态：**已放弃**（2026-09-18，第十一轮后使用者决定终止）。
载体：`tools/pengu-facade-preview/`（Pengu Loader 插件脚本 + 桩件测试）。
本文只记结论与证据，供日后同类需求直接引用，避免重复踩坑。

## 一句话结论

在国服 LoL 客户端上，「未拥有的头像/旗帜仅本地可见」**不存在既不破坏原生功能、又能覆盖生涯页/旗帜的解法**。
服务器所有权校验是硬墙；客户端内部组件会在任何「装备」意图时主动向服务端保存并被拒、随即回滚。

## 十一轮实测得到的硬事实

1. **服务端所有权墙**：未拥有头像 `PUT /lol-summoner/v1/current-summoner/icon` → 401 RPC_ERROR；
   未拥有旗帜经客户端保存路径 → `POST /lol-challenges/v1/update-player-preferences`
   → `400 RPC_ERROR: Player does not own REGALIA_BANNER <id>`（第十一轮日志实证）。
   任何让客户端「装备」未拥有物品的路径都会撞这堵墙并被回滚。
2. **数据层改写（响应副本）可行但覆盖有限**：顶部栏、好友栏、聊天等「推送订阅型」表面
   可通过合成 `OnJsonApiEvent_*` 推送立即生效；生涯页/旗帜组件是「显示时才发请求」或
   「按属性渲染 + 保存」，数据层追不上或触发保存撞墙。
3. **属性补丁（DOM）不可用**：改 `lol-regalia-identity-customizer-element` 的 `banner-id`/
   `profile-icon` 会触发客户端自己的保存路径 → 撞墙 → 回滚；且选择器列表项
   （`lol-regalia-banner-v2-element`，无身份属性）会被误补丁导致「所有旗帜一样」+ 误让位。
4. **让位机制对多主体源不成立**：hovercard 等源服务多个主体（好友/自己），其「真值」随悬停
   对象变化，按「源:字段」存键仍会在启动时误让位（第十一轮日志：`hovercard:icon 6574 -> 5911`）。
5. **像素层（shadow DOM img src 替换）理论可行但未完成**：不碰输入、不触发保存、不撞墙；
   需要 banner 资源 URL 映射（未拿到）。即便如此，也只解决「看得见」，不解决客户端内部
   状态与保存路径的一致性，投入产出比低，使用者决定终止。
6. **国服库存端点**：`/lol-inventory/v2/inventory/SUMMONER_ICON` 返回已拥有子集（508 条全 owned）；
   旗帜拥有态唯一权威是 `/lol-regalia/v3/inventory/REGALIA_BANNER`（`/lol-inventory/v2` 的同名类型全 false，禁用）。
7. **生涯页旗帜读取源**：regalia v2（`bannerType` 字符串口径）与 `summoner-profile.regalia`
   （JSON 字符串、数值口径）并存；`challenge-summary.bannerId` 是挑战旗帜口径。三套口径互不相通。

## 可保留的成果

- **已拥有物品的切换**：客户端原生功能完全可用，无需任何插件。
- **顶部栏/好友栏头像本地预览**：数据层 + 合成推送可行（若日后只需要这两个表面，可复活 v0.5.0 的数据层部分）。
- **桩件测试**：曾达 39 个测试块 / 149 项断言（覆盖响应改写、合成推送、让位语义、DOM/像素补丁边界），
  已按使用者要求于 2026-09-18 删除；如需回归基线可按 CLOSURE 结论重建。
- **诊断方法论**：`[diag]` 结构摘要 + 真值/改写分离 + 单变量假设更替，十一轮里每次定位都靠它。

## 清理步骤（使用者机器）

1. 删除插件目录：`<Pengu Loader 安装目录>\plugins\facade-preview\`（整个文件夹）。
2. 重启 LoL 客户端一次（清除已注入的 hook 与持久化选择）。
3. 无需其它操作；本实验未写服务端、未改游戏文件。

## 仓库内位置

- 参考脚本：`tools/pengu-facade-preview/facade-local-preview.js`（桩件测试已按使用者要求删除）
- 说明与逐轮结论：`tools/pengu-facade-preview/README.md`（第 1–21 条实测结论）
- 本存档：`tools/pengu-facade-preview/CLOSURE.md`
