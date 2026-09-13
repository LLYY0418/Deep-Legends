# WORKLIST-R82-ADDENDUM-3 · A/B 采集脚本在旧版 Node 上报错 `.at is not a function`

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。

## 问题

用户在真机上完成安装、脚本已经能正确读出指纹（`Run control / b07acedb0203`），
但在采集阶段报错：

```
[(...lines.matchAll(...))].at is not a function
Collection failed; see the error above.
```

**根因**：`scripts/r82-startup-ab-report.cjs` 第 78-79 行用了
`Array.prototype.at()`：

```javascript
const prewarm = [...lines.matchAll(/startup prewarm .../g)].at(-1);
const handoff = [...lines.matchAll(/application handoff .../g)].at(-1);
```

`Array.prototype.at()` 是 Node.js **v16.6.0** 才引入的方法（对应 V8 8.9 / ES2022提案）。
这个脚本通过 `& $node ...` 调用用户系统上安装的、任意版本的 Node（不是仓库固定的
开发环境版本），用户这台机器上的 Node 版本比 v16.6.0 更旧，`.at` 在其
`Array.prototype` 上不存在，于是抛出 `TypeError: ... .at is not a function`，
这条错误被 PowerShell 包成了 `Collection failed; see the error above.`。

这不是用户操作问题，也不是上一轮文件名指纹修复引入的新问题——是这个脚本本身
从写出来那天起就带着的一个 Node 版本兼容性 bug，只是这台机器的 Node 版本第一次
把它暴露出来。

## 要做什么

把这两处 `.at(-1)` 换成不依赖 `Array.prototype.at` 的写法（等价于取数组最后一个
元素），例如：

```javascript
const prewarmMatches = [...lines.matchAll(/startup prewarm .../g)];
const prewarm = prewarmMatches[prewarmMatches.length - 1];
const handoffMatches = [...lines.matchAll(/application handoff .../g)];
const handoff = handoffMatches[handoffMatches.length - 1];
```

或者写一个小 helper（`const last = (arr) => arr[arr.length - 1];`）复用两处，
用哪种写法不重要，只要不再调用 `.at(`。

**顺带检查**：`scripts/r82-startup-ab-report.cjs` 和它调用链上会被同一个
`& $node` 执行的其它文件（不包括测试文件 `*.test.cjs`，那些只在开发机上跑，
开发机 Node 版本不是问题）有没有用到其它现代语法/API，是这个用户真机 Node
版本大概率不支持的。具体检查方式：

1. 找出 `scripts/r82-startup-ab-report.cjs` 里所有会在**用户真机**（而不是
   开发机）上执行到的代码路径用到的语言特性/内置方法。
2. 对照该脚本目前隐含要求的最低 Node 版本（如果这次修完后不再用任何
   16.6+ 专属特性，就没必要再设最低版本要求；如果发现还有其它较新特性
   摘不掉，就用 `package.json` 的 `engines` 字段或脚本开头做一次版本检查，
   在真机 Node 版本不够时给出人能看懂的中文报错，而不是原生 TypeError 堆栈）。
3. 不要把这次检查范围扩大到整个仓库的其它脚本——只看这个真机采集脚本
   实际会执行到的代码。

## 追加发现：同一构建产物一旦启动过，报错文案要说清楚补救办法

用户在这次 `.at()` 崩溃之后，用**同一个安装包**（同一个指纹）重跑了一次，
这次卡在了另一处报错（这条不是 bug，是脚本设计上正确的保护）：

```
This fingerprint was already launched before the test; reject warm sample
```

**原因**：上一次虽然 `r82-startup-ab-report.cjs` 因为 `.at()` 崩溃、没能把结果
写进 `results.json`，但在崩溃之前，安装器早已跑完并真正启动过一次应用
（诊断日志 `diagnostics.jsonl` 里已经留下了这个指纹的 `app_start` 记录）。
这次重跑时，`collectRecord` 发现这个指纹的 `app_start` 时间戳早于本次
`$since`，判定"这不是一次真正的冷启动"而拒绝——这是有意为之的保护：
Windows 已经缓存过对这份 exe 的扫描结果，第二次启动的数据不能反映真实的
首次安装体验，采纳它会污染 A/B 对比。

**问题在于**：这条报错文案对用户来说看着和前两次的报错一样吓人，用户没法
从文案本身判断"这次不是 bug，需要换一份全新构建，不能重跑同一个安装包"。
请顺手把这条报错文案改清楚，明确告诉用户：

1. 这个指纹已经启动过，不能再用于本次 A/B 测量（不管上次采集是成功还是
   失败，只要应用被启动过一次就作废）。
2. 需要请求一份新的构建（哪怕代码没有实质变化也需要能产生新指纹的构建）
   再重跑，不要重复使用同一个安装包 `.exe` 文件重试。

同时检查一下操作文档（`ACCEPTANCE-REPORT-R82-AB-SCRIPT-FILENAME.md` 或
其它面向用户的操作说明，如果存在的话）有没有讲清楚这条规则——如果没有，
补一句类似"采集脚本从安装开始运行到结束之间，不管中途是否报错，只要软件
被启动过就不能重跑，必须换一份新构建"。

### 追加验收判据

1. 用一次"先正常跑完安装+启动，采集阶段人为制造失败（比如临时改坏
   report 脚本触发异常），再用同一个安装包重跑"的场景验证：新的报错文案
   必须清楚说明"需要换新构建"，而不是只有一句英文断言文本。
2. 这条报错文案的内容变化不能影响判定逻辑本身（依然是拒绝，不能因为
   "文案更友好"而放宽成允许重跑）。

### 追加变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 把"已启动过拒绝"这条判定逻辑本身弱化或删除（比如放宽时间窗口、忽略已有 app_start 记录） | ✅（必须仍有测试断言：同一指纹重复启动后必须拒绝，不能因为这次顺手改文案而破坏原有保护） |

## 追加发现：某一组已测满 3 次时，应在启动安装器前拦截，而不是白跑一次浪费构建

用户实测走到第 4 次测量时踩到了这个：前 3 次都用 `-Group control` 成功采集，
第 4 次仍然传了 `-Group control`（本该切到 `-Group prewarm`），命令是：

```
.\r82-startup-ab.ps1 -Installer '...\Deep Legends Setup 0.12.1.exe' -Group control
Run control / 4dda8ef4b66e. Complete installation normally; do not reopen the app during collection.
Use exactly 3 fresh samples per group
Collection failed; see the error above.
```

**这条拒绝逻辑本身是对的**（`r82-startup-ab-report.cjs` 的 `summarize()` 里
`Object.values(groups).some(rows => rows.length > 3)` 那条检查，防止某一组
测多于 3 次），报错发生在 `fs.writeFileSync` 之前，已有的 3 条记录不会被
污染或覆盖，这点没问题。

**但时机不对**：现在是先跑完一整次真实安装、真实启动应用之后，`--collect`
阶段才发现"这组已经测满了"再报错。而根据前面"追加发现"那条规则，
只要应用被启动过一次，这份构建就不能再用于任何一次有效测量（包括切换到
另一组重跑）——也就是说，用户这次白跑一次安装，还搭进去一份构建，只是
为了在最后一步才被告知"其实这个组已经够了"。

**要做什么**：在 `scripts/r82-startup-ab.ps1` 里，紧挨着现有的"这个指纹是否
已经测过"去重检查（`$previous | Where-Object { $_.fingerprint -eq $fingerprint }`
那一段），加一条同样在 **`Start-Process` 之前**执行的预检查：读取
`$Results`（如果存在）里属于 `$Group` 这个组的记录数，如果已经达到 3 条，
直接报错终止，不启动安装器。报错文案要清楚说明"这个组已经有 3 个有效样本，
不需要再测；如果是想测另一组，请改用 `-Group prewarm`（或 `control`）"。

这条预检查只看"当前这个组的样本数"，不影响另一组还没测满时的正常流程；
两组各自的判定完全独立。

### 追加验收判据

1. 构造一个已有 3 条 `group=control` 记录的 `results.json`，再次以
   `-Group control` 调用脚本：必须在 `Start-Process` 之前就报错终止，
   **不能**弹出/启动安装器。报错文案要能让人看懂"这组够了，换另一组"。
2. 同样的 `results.json`，改用 `-Group prewarm` 调用：必须正常继续走到
   启动安装器这一步（因为 prewarm 组还没有样本），不受 control 组已测满
   的影响。
3. 只有 2 条 `group=control` 记录时，以 `-Group control` 调用：必须正常
   继续（还没测满），不能被误判为已满。

### 追加变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 预检查判断两组样本数之和（而不是只看当前 `$Group` 那一组）| ✅（构造"control 3 条 + prewarm 0 条"，用 `-Group prewarm` 调用必须仍能正常继续，不能被 control 已满误伤） |
| 预检查放在 `Start-Process` 之后而不是之前 | ✅（用一个会在测试里被 mock 检测调用次数的 `Start-Process`，断言已测满时调用次数为 0） |
| 预检查阈值写成 `>= 4` 或 `> 3` 导致差一错误 | ✅（3 条时必须放行，达到 3 条后的第 4 次必须拦截，边界值要覆盖） |

## 验收判据

1. 在一个**不支持** `Array.prototype.at`（例如 Node 14.x，或任何刻意去掉
   `Array.prototype.at` 的模拟环境）上跑现有的
   `scripts/r82-startup-ab-report.test.cjs` 相关测试和一次端到端调用，
   修复前必须能复现 `.at is not a function`；修复后必须成功跑完，行为
   （匹配"最后一条日志记录"）与修复前在新版 Node 上的行为完全一致。
2. 在正常新版 Node（比如仓库开发机当前用的版本）上，原有全部测试
   （`r82-startup-ab-report.test.cjs`、`r82-startup-ab-script.test.cjs`、
   `desktop/release-build.test.cjs`）必须继续全绿，不能因为这次改动引入
   新的回归。
3. 用真实的多行 `DeepLegendsSetup-startup.log`（同一个 pid 出现多条
   `startup prewarm ...` / `application handoff ...` 记录，模拟用户可能
   重试过、日志里有历史行的情况）验证：取到的必须是**最后一条**匹配，
   不能因为改写方式而意外变成取第一条。

## 变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 改写后取到数组第一个元素而不是最后一个 | ✅（构造多条匹配的日志，断言取到的是最后一条） |
| 改写引入了另一个 Node 16+/18+ 专属方法（比如换成用了 `Object.hasOwn` 之类） | ✅（如果第 3 步的兼容性检查发现了其它现代特性，需要有测试覆盖修完后的最低版本要求；如果没发现别的现代特性，这条按不适用处理） |
| 只改了这两行但没跑一遍旧版 Node 复现+验证，就直接报告修完 | ✅（验收报告必须写清楚：用什么方式在什么 Node 版本下复现了原始报错、又在同一环境下验证修复后不再报错——不能只在新版 Node 上跑测试就算数） |

## 不要做的事

- 不要要求用户升级 Node.js 来绕过这个问题——用户的真机 Node 版本是既有环境，
  脚本应该适配普遍存在的 Node 版本，而不是把兼容性负担转嫁给用户。
- 不要把这次改动范围扩大到 R82 主工单/补充单其它逻辑；只修这一处 Node
  版本兼容性问题（以及第二步里如果发现的同类问题）。
- 不要改 PowerShell 脚本本身（`r82-startup-ab.ps1`），这次报错完全在
  `r82-startup-ab-report.cjs` 内部。
