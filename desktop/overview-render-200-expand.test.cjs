"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { bootDemoApp, settled, installWindowCleanup } = require("./overview-render-helpers.cjs");
installWindowCleanup(test);

test("总览战绩卡：点击展开按钮只替换目标卡片并可收起", async () => {
  const { window: w, errors } = bootDemoApp();
  await settled();
  const d = w.document;
  const list = d.querySelector(".match-list");
  const entries = [...d.querySelectorAll(".match-list .match-entry")];
  const button = entries[0]?.querySelector("[data-toggle-match]:not([disabled])");
  const untouched = entries.find((entry) => entry !== button?.closest(".match-entry"));
  assert.ok(list, "找不到战绩列表");
  assert.ok(button, "找不到可用的展开按钮");
  assert.ok(untouched, "找不到用于验证局部替换的其它战绩卡");
  const gameID = button.dataset.toggleMatch;
  const originalUntouched = untouched;
  button.click();
  await settled();
  assert.equal(d.querySelector(".match-list"), list, "展开不应重建整个战绩列表");
  const expanded = d.querySelector(`[data-match-id="${gameID}"]`);
  assert.ok(expanded?.querySelector(".match-detail"), "点击展开后目标卡片没有详情区");
  const expandedButton = expanded.querySelector(`[data-toggle-match="${gameID}"]`);
  assert.equal(expandedButton?.getAttribute("aria-expanded"), "true");
  assert.equal(expandedButton?.getAttribute("aria-controls"), `match-detail-${gameID}`);
  assert.equal(d.querySelector(`#match-detail-${gameID}`)?.closest(".match-entry"), expanded, "详情必须归属于目标战绩卡");
  assert.equal(d.querySelector(`[data-match-id]:not([data-match-id="${gameID}"])`), originalUntouched, "非目标战绩卡不应被重建");

  expandedButton.click();
  await settled();
  assert.equal(d.querySelector(".match-list"), list, "收起不应重建整个战绩列表");
  const collapsed = d.querySelector(`[data-match-id="${gameID}"]`);
  assert.equal(collapsed.querySelector(".match-detail"), null, "再次点击没有收起");
  assert.equal(collapsed.querySelector(`[data-toggle-match="${gameID}"]`)?.getAttribute("aria-expanded"), "false");
  assert.deepEqual(errors, []);
  w.close();

  const large = bootDemoApp({ matchCount: 200 });
  await settled();
  const largeDocument = large.window.document;
  const largeList = largeDocument.querySelector(".match-list");
  const largeEntries = [...largeDocument.querySelectorAll(".match-list .match-entry")];
  const largeButton = largeEntries[0]?.querySelector("[data-toggle-match]:not([disabled])");
  const largeUntouched = largeEntries.find((entry) => entry !== largeButton?.closest(".match-entry"));
  assert.ok(largeList && largeButton && largeUntouched, "200 场性能护栏找不到战绩目标");
  const createElement = largeDocument.createElement.bind(largeDocument);
  let createElementCalls = 0;
  largeDocument.createElement = (...args) => {
    createElementCalls += 1;
    return createElement(...args);
  };
  largeButton.click();
  await settled();
  largeDocument.createElement = createElement;
  assert.ok(createElementCalls < 500, `单次展开不应创建 ${createElementCalls} 个 DOM 元素`);
  assert.equal(largeDocument.querySelector(".match-list"), largeList, "200 场展开不应重建列表");
  assert.equal(largeDocument.querySelector(`[data-match-id]:not([data-match-id="${largeButton.dataset.toggleMatch}"])`), largeUntouched, "200 场展开不应重建非目标卡片");
  assert.deepEqual(large.errors, []);
  large.window.close();

  const mutated = bootDemoApp({
    gameplaySourceTransform: (source) => source.replace(
      "entry.replaceWith(replacement);",
      "rerender();",
    ),
  });
  await settled();
  const mutatedList = mutated.window.document.querySelector(".match-list");
  const mutatedButton = mutated.window.document.querySelector(".match-list .match-entry [data-toggle-match]:not([disabled])");
  assert.ok(mutatedList && mutatedButton, "变异副本找不到局部展开测试目标");
  mutatedButton.click();
  await settled();
  assert.equal(mutated.window.document.querySelector(".match-list"), mutatedList, "变异必须被局部替换回归测试捕获");
  mutated.window.close();
});
