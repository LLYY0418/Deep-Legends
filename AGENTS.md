# Deep Legends 项目约定（Codex 侧）

与 CLAUDE.md 共用同一套项目记忆，先读以下两份：

1. `docs/assistant-conversation-digest.md` — 2026-08-04 至 09-18 用户与 Codex/Claude 全部开发对话的汇总（编年时间线、用户原则与偏好、问题→工单因果链、原始记录定位方法）。执行工单前建议先查对应日期的上下文。
2. `docs/pro-accounts-verification-2026-09-17.md` — 职业页 6 队、33 人、53 个账号的唯一归属来源；修改身份前必须核对该文件，保留大小写、空格和原始顺序。动态目录只补充账号数据，不覆盖人工归属。

当前版本：0.12.18（以 `desktop/package.json` 为准）。R107 的视频/日志分析、真实数据验证与 Windows 真机验证边界见 `docs/r107-execution-ledger.md`。

目录布局：Go 源码、内嵌资源（`web/`、`data/`、`prestige_chromas.json`）与测试夹具（`testdata/`）统一位于 `backend/`；构建 `go build ./backend`，测试 `go test ./backend`，前端测试在 `backend/web/`。`installer/` 与 `tools/` 仍是独立 Go 模块。Go 构建缓存使用系统默认目录，不放仓库内。`docs/` 根目录只放进行中的 `WORKLIST-*` 与对应账本、长期参考文档；关闭后工单归档到 `docs/history/worklists/`，账本归档到 `docs/history/ledgers/`，核查报告、提案和探测结论归档到 `docs/history/reports/`。查工单先看 `docs/WORKLIST-INDEX.md`。

## 执行纪律（来自用户反复强调的原则，详见 digest）

- 工单驱动：严格按 WORKLIST 执行，不遗漏任何一条；超范围改动（「瞎改」）零容忍。
- 数据准确性红线：证据不足明确降级，不用推断值代替；隐私声明必须与实际写操作一致。
- 界面文案红线（R128 §2.5，2026-09-22 用户指示）：界面上不添加「统计口径」、方法论、免责声明类的说明文字（如“仅描述关联，不构成因果结论”“不代表……”“口径为……”）。数据来源与算法说明写在代码注释或 docs 里，不进 UI。确需提示用户的只限：错误/失败、数据回退到其他模式或旧版本。R116 系列工单里“统计口径必须常驻可见”的要求已撤销，不要按旧工单加回来。
- 日志完备：排障场景把能想到的地方都补上日志，避免二次返工。
- 构建打包：改动后必须重新构建并确认版本号最新；用户通常自己打包。
- 沟通：中文、简短直接、给出确定答案。
