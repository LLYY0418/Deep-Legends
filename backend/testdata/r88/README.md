# R88 回归样本

- `yourgg-arena-rankings-upstream.json`：2026-09-14 从工单指定公开接口 https://api.your.gg/kr/api/arena/champions 抓取，44,907 字节，173 行，首档 S；本次实时数据已不含 OP。SHA-256：`a518b736b745c08e6465a8841c7a0aeef05a6f9902f65f02ba0c33a25bfe6df7`。
- `yourgg-arena-rankings.json`：基于上述原始快照，仅将首行 `tier` 改为 `OP` 的派生回归样本，其他字段不变；不冒充工单当时的 44,951 字节响应。
- `roster-names.json`：用户补充的 23 个可见名称；明确标记五个猜补编号。宽度验收只使用去编号的名称；tooltip 测试验证身份完整传递，不验证猜补编号的真实性。
- `protected-layout-rules.json`：R88 开始前所有有效 `.match-main` / `.match-stats` 规则的逐字快照。按工单要求删除的三个死 `arena-first` 容器及其内部规则不计入有效规则快照，其余规则必须严格相同。
