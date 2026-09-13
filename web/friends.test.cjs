"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("../desktop/node_modules/jsdom");
const source = fs.readFileSync(path.join(__dirname, "friends.js"), "utf8");
const escapeHTML = (value) => String(value ?? "").replace(/[&<>\"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[char]));
const helpers = Function("escapeHTML", source.slice(source.indexOf("  function formatDuration("), source.indexOf("  /* ---------- 数据")) + "\nreturn {presenceKind, statusHTML};")(escapeHTML);
const lobby = { availability: "chat", gameStatus: "lobby", partySize: 2, partyCapacity: 5, queueLabel: "海克斯大乱斗" };
function status(friend) { return helpers.statusHTML(friend, helpers.presenceKind(friend)); }

test("online friend party shows real occupancy and mode, including TFT", () => {
  assert.equal(status(lobby), "2/5 海克斯大乱斗");
  assert.equal(status({...lobby, partySize: 1}), "1/5 海克斯大乱斗");
  assert.equal(status({...lobby, partySize: 1, queueLabel: "排位赛（云顶之弈）"}), "1/5 排位赛（云顶之弈）");
  assert.equal(helpers.presenceKind({...lobby, availability: "dnd"}), "online");
  assert.equal(status({...lobby, partySize: 8, partyCapacity: 8}), "8/8 海克斯大乱斗");
});

test("partial or invalid occupancy is not invented as 1/5", () => {
  for (const size of [undefined, 0, -1, 1.5, Infinity, "<script>"]) {
    assert.equal(status({...lobby, partySize: size}), "组队大厅 · 海克斯大乱斗");
  }
  for (const capacity of [undefined, 0, -1, 1, 1.5, Infinity]) {
    assert.equal(status({...lobby, partyCapacity: capacity}), "2 人组队 · 海克斯大乱斗");
  }
  assert.equal(status({...lobby, queueLabel: ""}), "2/5 组队大厅");
  assert.equal(status({...lobby, partySize: 0, queueLabel: ""}), "组队大厅");
});

test("game phase wins over chat availability and stale lobby data", () => {
  const queue = {...lobby, gameStatus: "inQueue"};
  assert.equal(status(queue), "匹配中 · 海克斯大乱斗");
  assert.equal(helpers.presenceKind(queue), "ingame");
  assert.equal(status({...lobby, gameStatus: "championSelect"}), "英雄选择中");
  const game = {...lobby, gameStatus: "inGame", championName: "盲僧", gameStartedAt: Date.now() - 80000};
  assert.match(status(game), /^海克斯大乱斗 · 盲僧 · <b data-started="\d+">1:20<\/b>$/);
  assert.equal(helpers.presenceKind(game), "ingame");
  assert.equal(status({...lobby, gameStatus: "spectating"}), "观战中");
  assert.equal(status({...lobby, availability: "spectating"}), "观战中");
  assert.equal(status({...lobby, gameStatus: "outOfGame"}), "在线");
  assert.equal(status({...lobby, gameStatus: "outOfGame", statusMessage: "周末开黑"}), "在线 · 周末开黑");
});

test("offline, mobile, away and other product never reuse the LoL lobby text", () => {
  assert.equal(status({...lobby, availability: "offline"}), "离线");
  assert.equal(status({...lobby, availability: "none"}), "离线");
  assert.equal(status({...lobby, availability: "mobile"}), "手机在线");
  assert.equal(status({...lobby, availability: "away"}), "离开");
  assert.equal(status({...lobby, product: "valorant", productName: "无畏契约"}), "无畏契约");
  assert.equal(status({...lobby, queueLabel: '<img onerror="alert(1)">'}), "2/5 &lt;img onerror=&quot;alert(1)&quot;&gt;");
  assert.equal(status({...lobby, gameStatus: "inGame", gameStartedAt: Infinity}), "海克斯大乱斗");
});

test("real friend dock rerenders occupancy and removes old room details on every phase change", async () => {
  const dom = new JSDOM(`<button id="friends-toggle"><span id="friends-count"></span></button>
    <aside id="friends-dock"><button id="friends-close"></button><input id="friends-search-input">
    <span id="friends-summary"></span><div id="friends-list"></div></aside>`, {url: "http://localhost", runScripts: "outside-only"});
  const {window} = dom;
  window.deepLegendsRuntime = { escapeHTML };
  let payload = {...lobby, gameName: "房间好友", tagLine: "1234", playerRef: "anon", partySize: 1, groupId: 1};
  const requests = [];
  window.fetch = async (url) => {
    requests.push(url);
    return {ok: true, json: async () => ({groups: [{id: 1, name: "默认分组"}], friends: [{...payload}]})};
  };
  const flush = () => new Promise(setImmediate);
  try {
    window.eval(source);
    window.dispatchEvent(new window.CustomEvent("deep-legends:status", {detail: {connected: true}}));
    await flush();
    window.document.getElementById("friends-toggle").click();
    await flush();
    assert.equal(window.document.querySelector(".friend-status").textContent, "1/5 海克斯大乱斗");
    for (const [changes, expected] of [
      [{partySize: 2}, "2/5 海克斯大乱斗"],
      [{gameStatus: "inQueue"}, "匹配中 · 海克斯大乱斗"],
      [{gameStatus: "championSelect"}, "英雄选择中"],
      [{gameStatus: "inGame", championId: 64}, "海克斯大乱斗"],
      [{gameStatus: "outOfGame"}, "在线"],
    ]) {
      payload = {...payload, ...changes};
      window.document.getElementById("friends-close").click();
      window.document.getElementById("friends-toggle").click();
      await flush();
      assert.equal(window.document.querySelector(".friend-status").textContent, expected);
    }
    assert.equal(window.document.querySelector(".friend-champion"), null, "stale champion removed after returning to lobby");
    payload.availability = "offline";
    window.document.getElementById("friends-close").click();
    window.document.getElementById("friends-toggle").click();
    await flush();
    assert.equal(window.document.getElementById("friends-count").textContent, "0");
    assert.equal(window.document.querySelector(".friend-status.online"), null);
    assert.ok(requests.length >= 8);
    assert.ok(requests.every(url => url === "/api/social/friends"), "no roster or probe endpoints");
  } finally {
    window.dispatchEvent(new window.CustomEvent("deep-legends:dispose"));
    window.close();
  }
});
