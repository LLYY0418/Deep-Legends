# WORKLIST-R139：「阵营位置播报」设置项太乱——去掉两个多余勾选框，剩下那个只在选队伍频道时才出现，样式换成全站统一的开关组件

诊断人：Claude（用户反馈截图 + 读 `backend/web/suite.js`、`backend/watch_broadcast.go`、`backend/watch_rules.go`，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：R138 之后（若 R138 尚未执行，以 R137 之后 HEAD 为准）。
状态：已完成（见 `r139-execution-ledger.md`）。只动「阵营位置播报」（`positionBroadcast`）这一个设置项，其它 `watchDefinitions` 条目不动。

## 0. 用户反馈与决定

截图：设置面板「阵营位置播报」下面堆了三个勾选框——「阵营位置（保留）」「附带己方英雄（仅队伍频道）」「附带我的分路（客户端提供时）」，用户觉得乱。查代码后确认：

- **「阵营位置（保留）」这一项本来就不是真的可选项**：`backend/watch_broadcast.go:33`（`positionBroadcastMessage`）里 `parts := []string{watchCampPrefix + camp}` 是无条件执行的，跟 `rule` 里任何字段都没关系。前端 `suite.js:328-329` 特意把它标成 `fixed`+`disabled`，渲染出来是一个"永远勾上、点不动"的勾选框——纯 UI 摆设，没有对应的真实开关。
- **「附带我的分路」现在默认是关的**（`watchBroadcastRule` 是 Go struct，`AssignedPosition bool` 零值 `false`；`defaultWatchSettings()` 里没给它显式赋值，见 `backend/watch_rules.go:147`），且它不区分"仅自己可见"还是"发到队伍频道"，两种模式下都会生效（`positionBroadcastMessage` 里没有对它做 `rule.Visibility` 判断，只有 `TeamComposition` 那段有）。
- **只有「附带己方英雄」是真正需要用户决定的开关**：它标了 `TeamOnly: true`（`watch_broadcast.go:26`），代码里也确实做了 `rule.Visibility != "team"` 的判断（`watch_broadcast.go:40`）——发给自己看时带上"己方英雄名单"没有意义，这是唯一一个跟"发给谁看"直接挂钩、需要用户勾选的东西。

用户的决定：**只保留「附带己方英雄」这一个勾选框，而且只在选了"发到队伍频道"时才显示**（不是像现在这样一直显示、只是置灰）；「阵营位置」和「附带我的分路」不再展示成勾选框，直接按"默认开启、用户不可关"处理。

## 1. 前端：只渲染一个勾选框，且视条件显示，样式换成全站统一的开关组件

### 1.1 现状：这里是全仓唯一一处用原生 `<input type="checkbox">` 裸勾选框的地方

`backend/web/suite.js:325-332`（`watchRuleControl` 里 `control === "visibility"` 分支）：

```js
if (definition.control === "visibility") {
  const options = (state.watch?.broadcastOptions || []).map(option => {
    const fixed = option.key === "camp";
    const disabled = fixed || (option.teamOnly && rule.visibility !== "team");
    return `<label class="watch-broadcast-option"><input type="checkbox" data-watch-broadcast="${escapeHTML(option.key)}"${fixed || rule[option.key] ? " checked" : ""}${disabled ? " disabled" : ""}> ${escapeHTML(option.label)}<small>发送格式：${escapeHTML(option.template)}</small></label>`;
  }).join("");
  return watchChoiceButtons("positionBroadcast.visibility", rule.visibility, [["self", "仅自己可见"], ["team", "发到队伍频道"]], "播报范围") + `<div class="watch-broadcast-options">${options}</div>`;
}
```

对应 CSS（`backend/web/suite.css:599-601`）只有三行，没有任何自定义外观，渲染出来就是浏览器原生方框勾选框——这是用户反馈"丑"的直接原因。全仓其它任何一个布尔开关，不管是每张规则卡右上角的总开关（`suite.js:447`）、征召托管的备战席开关（`suite.js:701-703`）、生涯页"只显示已拥有"（`suite.js:1462`）还是"登录时重设"（`suite.js:1464`），全部用的是同一个自定义组件 `.suite-switch`（`suite.css:115-121`，一个圆角药丸形滑块，选中态用主题金色），唯独这三个勾选框是原生样式，是当初（R115）写这块时漏掉了统一组件，不是设计上故意做成这样。

同时，`.cs-bench-row`（`suite.css:507-511`）已经是一个成熟的"左边标题+说明文字、右边一个 `.suite-switch`"两栏行组件，全站好几处都在用（备战席自动换英雄、优先靠前英雄等），正好是这里需要的形状。

### 1.2 改法：只在 `visibility === "team"` 时渲染一行，用 `.suite-switch` + 两栏行样式

```js
if (definition.control === "visibility") {
  const teamComposition = (state.watch?.broadcastOptions || []).find(option => option.key === "teamComposition");
  const optionsHTML = rule.visibility === "team" && teamComposition
    ? `<div class="watch-broadcast-options"><div class="watch-broadcast-row"><span><strong>${escapeHTML(teamComposition.label)}</strong><small>发送格式：${escapeHTML(teamComposition.template)}</small></span><label class="suite-switch"><input type="checkbox" aria-label="${escapeHTML(teamComposition.label)}" data-watch-broadcast="teamComposition"${rule.teamComposition ? " checked" : ""}><span class="sr-only">${escapeHTML(teamComposition.label)}</span></label></div></div>`
    : "";
  return watchChoiceButtons("positionBroadcast.visibility", rule.visibility, [["self", "仅自己可见"], ["team", "发到队伍频道"]], "播报范围") + optionsHTML;
}
```

`.watch-broadcast-row` 直接照抄 `.cs-bench-row`（`suite.css:507-511`）的属性写一份同名新规则（不要跨功能直接复用 `.cs-bench-row` 这个类名，语义上属于征召托管），标签文字（"附带己方英雄（仅队伍频道）"）和格式说明（"发送格式：当前己方英雄：..."）保持原样不改，只是从"复选框+纯文本一行"换成"标题+说明在左、金色药丸开关在右"这种全站统一的行布局：

```css
.watch-broadcast-options { display: grid; width: 100%; }
.watch-broadcast-row { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 10px 0; }
.watch-broadcast-row > span { display: grid; gap: 3px; }
.watch-broadcast-row strong { font-size: 12.5px; font-weight: 600; }
.watch-broadcast-row small { max-width: 58ch; color: var(--muted); font-size: 11.5px; line-height: 1.55; }
```

（不需要 `.cs-bench-row` 那条 `border-top`，因为这里只有一行，没有上一行可分隔；具体数值以真机截图微调为准，但必须用 `.suite-switch` 渲染开关本身，不能保留原生 `<input type="checkbox">` 裸样式。）

旧的 `.watch-broadcast-option`、`.watch-broadcast-option small` 两条规则（`suite.css:600-601`）删掉，不再有代码引用它们。

`backend/positionBroadcastOptions()`（`watch_broadcast.go:23`）**不用改**——继续声明全部三项，它同时也是 `TestR115_BroadcastDefaultsAndMissingData` 里"发送格式没有漂移"这条护栏的数据源（见下），只是前端不再把 `camp`、`assignedPosition` 两项渲染出来。

`bindWatchControls`（`suite.js:459-462`）里的事件监听不用改：它已经用 `["teamComposition", "assignedPosition"].includes(input.dataset.watchBroadcast)` 白名单过滤，`assignedPosition` 的勾选框以后不会再出现在 DOM 里，这段代码留着不影响正确性（如果顺手把白名单收窄成只剩 `"teamComposition"` 更干净，可以做，不强制）。

切换「仅自己可见」/「发到队伍频道」时，`watch-broadcast-options` 这块要跟着增删（不是显示/隐藏 CSS，是有没有这个 DOM 节点）——确认现有的重渲染机制（应该是整个设置面板按 `state` 重新 `render()`）会自然处理这个，不需要额外写监听。

## 2. 后端：「附带我的分路」改成默认开、且不可关

### 2.1 新装默认值

`backend/watch_rules.go:147`：

```go
PositionBroadcast: watchBroadcastRule{Visibility: "self"},
```

改成：

```go
PositionBroadcast: watchBroadcastRule{Visibility: "self", AssignedPosition: true},
```

### 2.2 已有存档、任何来源的 settings 一律强制打开

`backend/watch_rules.go:243-247`（`normalizeWatchSettings` 里已有的 `Visibility` 兜底逻辑旁边）加一行，不管加载出来的值是什么，一律强制为 `true`（跟 `camp` 一样，变成没有对应用户开关的固定行为）：

```go
switch settings.Rules.PositionBroadcast.Visibility {
case "self", "team":
default:
    settings.Rules.PositionBroadcast.Visibility = "self"
}
settings.Rules.PositionBroadcast.AssignedPosition = true // R139：不再是用户可关的选项，跟 camp 一样固定开启
```

`normalizeWatchSettings` 是 `loadWatchSettings`（读盘时）和 `saveWatchSettings`（写盘时）都会经过的函数，能覆盖到"用户之前手动关掉过"的旧存档；但 `loadWatchSettings` 里还有两处提前 `return defaultWatchSettings()` 的分支（读盘失败、JSON 解析失败）和 `currentWatch()` 里 `r == nil` 时的分支不经过 `normalizeWatchSettings`，所以 §2.1 那处默认值也必须改，两处都要改才能保证所有路径下都是 `true`。

`TeamComposition` 不受影响，继续保持默认 `false`、用户可开关。

## 3. 已有测试要更新（不是新写，是修正过时断言）

### 3.1 `backend/web/r115.test.cjs`

现有测试「R115 broadcast controls use server declarations, disable team-only options for self, and retain default-off choices」（第 38-43 行）断言 `visibility:'self'` 时渲染出 3 个 input、`inputs[1].disabled===true`。这条断言测的正是这次要改掉的旧行为，改成：

- `visibility: 'self'` 时：`inputs.length === 0`（一个都不渲染）。
- `visibility: 'team'` 时：`inputs.length === 1`，对应 `teamComposition`，不带 `disabled`，`checked` 跟 `rule.teamComposition` 走；这个 input 的父元素链上要能找到 `class="suite-switch"`（断言渲染结果里存在 `.suite-switch input[data-watch-broadcast="teamComposition"]`，而不是裸 `<input>`），确认真的换成了全站统一的开关组件，不是随手换了个 class 名字。
- 测试名字改成反映新行为（例如「R139 broadcast controls hide team-only option for self, show single suite-switch toggle for team」）。
- `assert.match(source,/state.watch.rules.positionBroadcast\[input.dataset.watchBroadcast\] = input.checked/)` 这条保留，仍然成立。

### 3.2 `backend/r115_test.go` 的 `TestR115_BroadcastDefaultsAndMissingData`

第 144-146 行：

```go
if s.Rules.PositionBroadcast.TeamComposition || s.Rules.PositionBroadcast.AssignedPosition {
    t.Fatal("upgrade enabled new messages")
}
```

这条断言当初是为了防止"升级后静默给旧用户多发消息"，但现在 `AssignedPosition` 按产品决定变成固定开启（跟 `camp` 一样没有开关），不再适用；`TeamComposition` 那部分防护逻辑仍然有效，要保留。改成：

```go
if s.Rules.PositionBroadcast.TeamComposition {
    t.Fatal("upgrade enabled new messages")
}
if !s.Rules.PositionBroadcast.AssignedPosition {
    t.Fatal("R139: assigned position must default on, it is no longer user-configurable")
}
```

`options := positionBroadcastOptions()` 那几行（第 160-163 行）不用改，`positionBroadcastOptions()` 本身没变。

## 4. 验收

- Go：`TestR115_BroadcastContentsAndGuards`（不受影响，继续通过）、更新后的 `TestR115_BroadcastDefaultsAndMissingData` 通过；新增一条：用一份显式写了 `"assignedPosition":false` 的旧存档 JSON 走 `loadWatchSettings`，断言读出来之后是 `true`（证明旧存档也被强制覆盖，不是只对新装生效）。
- Node：更新后的 `r115.test.cjs` 用例通过；`backend/web/r130.test.cjs`、`r123.test.cjs` 等其它涉及 suite 面板的测试不受影响（本单没碰它们涉及的选择器）。
- 变异：
  - 把 §1 的条件改回"始终渲染三项"，新的 `visibility:'self' → 0 inputs` 断言必须 FAIL。
  - 把 §2.2 加的强制赋值删掉，新增的"旧存档 assignedPosition:false 读出来变 true"断言必须 FAIL。
- 真机/手动：打开设置面板，「仅自己可见」时「阵营位置播报」下面不再有任何勾选框；切到「发到队伍频道」，只出现「附带己方英雄」一行，样式是标题+说明在左、金色药丸开关在右（和同一页面里其它开关长得一样），不再是原生方框勾选框；勾选状态和之前一致（默认未勾）。
- 真 Chromium 截图（复用现有的 suite 面板截图脚本即可）：这一行和页面里其它 `.suite-switch` 行放在一起对比，视觉上是同一套组件，没有突兀感。

## 5. 不做的事

- 不改 `positionBroadcastOptions()` 的声明内容（三项都保留，仍是消息格式的唯一数据源）。
- 不改 `TeamComposition` 的默认值和可关性。
- 不改「仅自己可见 / 发到队伍频道」这两个大按钮本身的文案和逻辑。
- 不改 `positionBroadcastMessage` 里 `camp` 和 `assignedPosition` 的拼接逻辑（本来就是无条件/不区分可见范围，行为不变，只是 UI 不再暴露成勾选框）。

## 验收总表

| 项 | 判据 |
|---|---|
| 前端渲染 | `visibility=self` 时 0 个勾选框；`visibility=team` 时只有 1 个（`teamComposition`），用 `.suite-switch` 渲染，不是原生裸勾选框；旧的 `.watch-broadcast-option` CSS 规则已删除；`r115.test.cjs` 更新用例通过；变异 FAIL |
| 后端默认值 | 新装和旧存档的 `AssignedPosition` 都被强制为 `true`；`TeamComposition` 默认值不变；`r115_test.go` 更新用例通过；变异 FAIL |
| 全量 | `go vet ./...`、`go test ./...`、`node --test backend/web/*.test.cjs desktop/*.test.cjs` 全绿；r100/r117 Chromium 护栏通过 |

本单是纯设置面板简化，不涉及版本号递增或重新打包（可以和 R138 一起打包发一版，也可以单独发，不强制）。
