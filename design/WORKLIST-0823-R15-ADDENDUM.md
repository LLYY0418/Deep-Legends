# WORKLIST 0823-R15-ADDENDUM（补一条护栏：`build-item-row.css` 的 `border-top: 0` 漂移无人盯防）

诊断人：Claude（只读验收，未改仓库文件）。执行人：GPT。
背景：`design/WORKLIST-0823-R15.md` 已验收通过（11 杀 1 活），唯一存活的变异体就是这条。

---

## 问题

`web/build-item-row.css:10`：

```css
.build-core-column + .item-depth-columns > section, .item-depth-columns > section + section { border-left: 1px solid var(--line); border-top: 0; }
```

这一行是 R15 合并 `champions.css`/`gameplay.css` 两份重复 CSS 时，顺手修掉的一处历史漂移
（合并前 `gameplay.css` 那份少了 `border-top: 0`，会导致对局内出装区域比详情页多一条分隔线）。
现在两个文件已经合并成一份，理论上不会再裂开——但**没有测试盯着这行具体规则**，
把 `border-top: 0;` 删掉，`node --test web/champions.test.cjs` 92 项测试依然全绿（已用真副本变异实测确认）。

---

## 要做

在 `web/champions.test.cjs` 里，`sharedBuildStyles` 相关的既有断言块附近
（`:165-166` 两行 `assert.match(sharedBuildStyles, ...)` 紧挨着，加在这两行之后即可）补一行：

```js
assert.match(sharedBuildStyles, /border-left:\s*1px solid var\(--line\);\s*border-top:\s*0;\s*\}/);
```

---

## 验收

- [ ] 新断言加入后 `node --test web/champions.test.cjs` 依然全绿
- [ ] 变异体自验：把 `web/build-item-row.css:10` 的 `border-top: 0; ` 删掉，
      这条新断言必须让测试 FAIL；改回来，测试必须变绿
- [ ] `go test ./...` 不受影响（本单只碰一个 CSS 文件和一条 JS 测试断言）
