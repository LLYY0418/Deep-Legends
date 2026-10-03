# R199 执行账本

日期：2026-10-03。只更正文档，不单独递增版本。R198 引用本工单，要求 0.12.60 保持未发布、R196 与 R197/R198 合并为 0.12.61。

- 只读查询 `repos/LLYY0418/Deep-Legends/releases`：0.12.60 草稿 id `402378684`，draft=true、prerelease=false、published_at=null；目标提交 `e3ba8b6fe58cfb1c188bedaa75ca82c307fe70b4`，三个 public 附件。证据 [draft-readonly.json](../reports/r199/draft-readonly.json)。
- 只读查询 `git/ref/tags/v0.12.60`：404，标签不存在。没有创建、移动或删除此标签。
- R196 账本已改为“0.12.60 未发布（用户决定与 R197、R198 合并后发布）”，保留所有构建及测试结果；旧草稿、附件、其他发布保持原样。
- WORKLIST-INDEX 顶部已增加发布证据规则：release id、isLatest=true 的查询结果、匿名 latest.json 版本缺一不可。索引登记 R199，R196 注明并入 R197/R198。
- 本工单无代码改动、无 Release 写操作。0.12.61 的发布属于 R198 收尾，记录在 R197/R198 账本。

R198 发布 0.12.61 后再次核验，0.12.60 草稿及三附件与发布前元数据逐项一致；证据见 [正式发布核验](../reports/r198/published-release.json)。按目录约定，已关闭工单与账本归档至 history。
