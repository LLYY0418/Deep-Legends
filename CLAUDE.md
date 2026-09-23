# 项目维护约定

`docs/pro-accounts-verification-2026-09-17.md` 已于 R105 完整嵌入代码。
职业页以该表的 6 队、33 人、53 个账号为唯一归属来源；修改身份前必须核对该文件，保留大小写、空格和原始顺序。动态目录只补充账号数据，不覆盖人工归属。

当前版本：0.12.7（以 `desktop/package.json` 为准）。R107 的视频/日志分析、真实数据验证与 Windows 真机验证边界见 `docs/r107-execution-ledger.md`。

目录布局：Go 源码与内嵌资源位于 `backend/`（构建 `go build ./backend`，测试 `go test ./backend`，前端在 `backend/web/`）；`installer/`、`tools/` 为独立模块。历史工单归档在 `docs/history/worklists/`，工单与验证证据继续写入 `docs/`。

## 界面文案红线（R128 §2.5，2026-09-22 用户指示）

界面上不添加「统计口径」、方法论、免责声明类的说明文字（如“仅描述关联，不构成因果结论”“不代表……”“口径为……”）。数据来源与算法说明写在代码注释或 docs 里，不进 UI。确需提示用户的只限：错误/失败、数据回退到其他模式或旧版本。R116 系列工单与 `docs/r116-proposal-feasibility-review.md` 里“统计口径必须常驻可见”的要求已撤销，不要按旧工单加回来；`backend/web/shared.js` 已随该决定删除。

## 助手对话历史（项目记忆）

2026-08-04 至 2026-09-18 与 Codex/Claude 的全部开发对话已汇总为 `docs/assistant-conversation-digest.md`（Codex 根会话 115 个 + Claude Cowork 会话目录 33 个，约 2,200 条用户消息）。查询历史决策、用户原始措辞、优先级、数据源拍板和问题→工单因果链时先读该文件；需要逐字原文时按其文末「原始记录查阅」定位本机会话文件。该文件同时是 AGENTS.md 的引用目标，两端共用。
