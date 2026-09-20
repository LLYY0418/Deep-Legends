# WORKLIST-R103：修复 proseed 锚点 LRU 保护的测试盲区

诊断人：Claude（只读诊断，未改仓库代码文件）。
执行人：GPT。
日期：2026-09-17。
背景：R102 独立验收阶段做对抗变异时，发现种子账号 PUUID 锚点（`proseed-` 前缀文件）不被磁盘 LRU 驱逐这条保护逻辑，如果被删掉，**现有测试套件完全检测不到**——包括 R99 专门为这条保护设计的压力测试。这不是产品代码的缺陷（保护逻辑本身完好，独立验证过确实在正常工作），纯粹是测试设计的盲区，本轮只修测试，不改任何生产逻辑。

---

## 口径决定

1. **真正需要补测试的函数是 `binary_disk_budget.go` 的 `accountBinaryDiskWriteLocked`（约 34-58 行），不是 `champion_cache.go` 的 `pruneDiskLocked`。** 这两个是完全独立的两套驱逐实现：`newRiotIdentityCache`（`riot_identity_cache.go:16-21`）经 `newPublicBinaryCache` 强制 `strictDisk=true`，所有种子锚点的写入都走 `accountBinaryDiskWriteLocked`（线性扫描找最旧，`proseed-` 判据在扫描循环内部用 `continue` 跳过，第 41-43 行附近）；`pruneDiskLocked`（`sort.Slice` 排序后逐个删除，`proseed-`/`hexdata-` 判据在装入候选切片前预过滤，444-447 行附近）服务的是 `strictDisk=false` 的实例，**目前没有任何代码路径会让一个 `strictDisk=false` 的缓存实例收到 `proseed-` 前缀的 key**（该前缀只在 `riotIdentityKey` 里对 `proseed:v1:` 开头的 identity 生成，而这个函数只被 strictDisk 缓存使用）。本轮先请 GPT 用 `grep`/调用链核实这一点是否属实——如果确认属实，`pruneDiskLocked` 里的 `proseed-` 判据目前是死代码防御，不需要为它补测试，但也不要删掉这行防御（万一将来某处新增了走非 strict 路径又用了这个 key 前缀，这行是最后一道保险）；如果核实发现确实存在某条被漏查的路径能让非 strict 缓存拿到 `proseed-` key，那要额外报告，不要自己动手改。
2. **不修改任何生产逻辑，只新增/改造测试。** `accountBinaryDiskWriteLocked`/`pruneDiskLocked` 函数体本轮零改动。
3. **新测试必须不依赖 `time.Now()` 的真实调用时机**，直接构造 `binaryDiskEntry{modified: ...}` 塞进 `strictEntries` map，用相隔很远、绝对不会打平的时间戳（比如按年份错开），排除"同一秒写入导致排序/比较结果不确定"这个旧测试的根因。

---

## P1 — 新增确定性单元测试：受保护条目永远不会被选为驱逐目标

**要做什么：**

构造一个 `championDataCache` 实例（`strictDisk=true`，`diskMaxEntries` 设一个小值比如 3），手工往 `c.strictEntries` 塞入至少 5 条记录：2 条 `proseed-` 前缀（`modified` 设成最早，比如 `time.Date(2000,...)` 和 `time.Date(2001,...)`）、3 条普通前缀（`modified` 设成比 proseed 条目晚得多，比如 2020~2022 年，且彼此互不相同）。调用一次 `accountBinaryDiskWriteLocked` 触发预算超限的驱逐循环（新写入的条目让 `len(strictEntries) > diskMaxEntries`），断言：

- 两条 `proseed-` 条目**都还在** `c.strictEntries` 里。
- 3 条普通条目里，时间最早的那些被删掉，直到条目数回到预算内。

**验收判据：**
- `TestR103ProseedEntriesNeverSelectedForEviction`：如上构造，断言驱逐后 `strictEntries` 里两条 `proseed-` key 都存在，且被删掉的 key 精确是普通条目里时间最旧的那些（不是随便断言"proseed还在"，要连"具体删的是谁"也断言，防止未来有人把判据改成"随机跳过某条"这种看似满足"proseed还在"但逻辑其实错了的写法）。

**变异判据（必须 FAIL）：**
1. 把 `accountBinaryDiskWriteLocked` 里 `strings.HasPrefix(filepath.Base(key), "proseed-")` 这行保护判据删掉 → 断言 FAIL（proseed 条目因为 `modified` 最早会被误删）。
2. 把保护判据从"永远不选中"改成"随机跳过一次"（比如加个计数器只保护一次）→ 断言 FAIL（第二条 proseed 记录在后续驱逐轮次里会被删）。

---

## P2 — 新增确定性单元测试：全部候选都受保护时必须报错而不是误删

**要做什么：**

构造 `c.strictEntries` 只包含 `proseed-` 前缀条目（比如 2 条），`diskMaxEntries` 设成 1（预算已经超限但唯二的候选全是受保护的）。调用 `accountBinaryDiskWriteLocked`，断言：

- 返回的 `error` 就是现有的 `errors.New("protected cache entries exceed disk budget")`（用 `errors.Is`/字符串精确匹配都行，但要断言具体是这个错误，不是随便一个非 nil 错误）。
- `strictEntries` 里两条 `proseed-` 条目**都没有**被删除（即便报错了，函数不能在报错前先删掉一部分再返回错误——检查现有实现是不是"扫描到找不到就直接 return"，不涉及部分删除，如果确认是就正常断言；如果发现有部分删除后才报错的情况，这是一个需要额外报告的真实产品缺陷，不在本轮修复范围内，先报告不要自己改）。

**验收判据：**
- `TestR103AllCandidatesProtectedReturnsErrorNotEviction`：如上，断言错误消息 + 两条记录都完整保留。

**变异判据（必须 FAIL）：**
1. 把"找不到 oldest 时返回错误"改成"找不到就跳过预算检查直接返回 nil"（相当于让缓存无限增长）→ 断言 FAIL（因为不会再有这个特定错误产生，测试断言错误类型这一步会失败）。

---

## P3 — 核实 `pruneDiskLocked` 的 `proseed-` 判据是否可达

**要做什么：** 按口径 1，用 `grep`/调用链核实是否存在任何 `strictDisk=false` 的缓存实例会被写入 `proseed-` 前缀的 key。在执行账本里写清楚结论：

- 如果确认不可达（目前的调研结论） → 不需要新增测试，保留该判据作为防御性代码，账本注明"已核实为当前不可达的防御性代码，不补测试"。
- 如果发现确实可达 → 在账本里报告具体路径，不要自己动手改代码或补测试，留给下一轮工单处理。

---

## 本轮明确不做的事

1. 不改动 `accountBinaryDiskWriteLocked`/`pruneDiskLocked` 的生产逻辑。
2. 不改 `TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField` 原有的 1030 次压力测试——它验证的是"正常使用场景下容量不失控"，仍然有价值，只是不能单独作为"保护判据没被删"的证据，本轮是新增互补测试，不是替换它。
3. 不处理 P3 万一发现的真实可达路径（如果有的话），只报告。
