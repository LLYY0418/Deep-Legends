# R200 执行账本

日期：2026-10-03。基线为已发布 0.12.61 加上用户此前授权的维护/总览修复（本地 0.12.62）；本次版本 0.12.63，发布范围包含这些尚未发布的改动。

## 实现

- P1：读取 `allowSubsetChampionPicks`。每个选人会话第一次需要卡片时读取一次 `GET /help`，递归提取明确的 champ-select / team-builder GET 子集路径；先尝试发现路径，再尝试 `/lol-lobby-team-builder/champ-select/v1/subset-champion-list`、`/lol-champ-select/v1/subset-champion-list`。只接受 HTTP 200 且合法的正整数数组或含 `championId` 的对象数组，合法空数组表示没有卡片。记住成功路径，会话刷新和写前核验重新读取卡片；全部失败不读全英雄列表、不选人。
- 卡片按配置序列和既有队友避让规则选择，保留三种策略与锁定等待；未锁定时新出现的更高优先级英雄可替换当前亮出。现有英雄名称格式化复用于两条卡片记录。
- P2：选用英雄两次未生效后标为该行动不可用，顺延下一个；最多三个不同英雄。失败计数跨规划/选人阶段稳定，禁用规则与手动接管规则保留。六次都未生效时保留既有手动操作提示。
- P3：备战席请求成功后只记录已发送，读取自己的英雄确认后才记录成功。两秒未生效时最多重试一次；目标消失或两次失败后看下一个目标。自己的英雄为零或本地选人尚未完成时不请求换人；写前也核对。
- P4：补齐 source 的 subset_mode/ids/source、bench-gate 的 bench_ids/current_champion_id、候选数量及 subset 来源；`subset_help_matches` 仅记路径；`subset_probe` 记路径、HTTP 状态、解析结果和 ID 数量；bench-postflight 记 applied/not-applied/target-gone、目标、实际英雄和次数。
- 用户追加 R198 修正：镜头 PATCH 核验成功后先读 `/help`，不依赖 swagger。没有列出 save 时直接探测一次 POST save；404/405 作为 unsupported 记入镜头诊断，不报错，其余失败保持原处理。

## 接口真实性边界

本环境没有运行的 LCU，用户确认没有离线 `/help` 或 OpenAPI。**子集接口未在真机核实，以用户下一局日志为准。** 本机实际使用的真实子集接口尚未知；模拟客户端覆盖了发现路径、两个候选路径、整数/对象返回与各种失败。不能把模拟接口记作用户客户端已核实接口。下一局从 `subset_probe` 的 valid=true 与 source 的 subset_source 确认实际路径。

## 验证

- 相关 Go 测试通过；完整 Go、Node、vet、竞态与 public 构建结果待本轮完成后补记。
- 四项 Go overlay 变异均触发断言 FAIL，没有以编译失败充数：卡片用全英雄、读取失败回退全英雄、失败不顺延、备战席不确认/恢复。见 [变异结果](history/reports/r200/mutations.json)。
- 真机卡片选择、备战席实际交换及镜头保存仍由用户下一局日志验收，工单保留进行中。

## 发布

发布未完成；完成后补充 release id、Latest 查询及匿名 latest.json 版本证据。
