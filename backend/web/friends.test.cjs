"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("../../desktop/node_modules/jsdom");
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
  const snapshots = [];
  window.addEventListener("deep-legends:friends-presence", event => snapshots.push(event.detail.friends));
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
    assert.equal(snapshots.at(-1)[0].playerRef, "anon", "loaded presence is also available before opening a player");
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
    window.dispatchEvent(new window.CustomEvent("deep-legends:status", {detail:{connected:false}}));
    assert.equal(snapshots.at(-1).length,0,"disconnect clears banner presence");
  } finally {
    window.dispatchEvent(new window.CustomEvent("deep-legends:dispose"));
    window.close();
  }
});

test("friend banner matches opaque aliases, escapes labels and rejects stale or foreign presence", () => {
  const gameplay = fs.readFileSync(path.join(__dirname,"gameplay.js"),"utf8");
  const state={friendPresence:new Map()};
  const now=Date.now();
  const source=gameplay.slice(gameplay.indexOf("  function friendPresenceForTab("),gameplay.indexOf('  window.addEventListener("deep-legends:friends-presence"'));
  const render=Function("state","riotTab","escapeHTML","Date",`${source}\nreturn friendPresenceMarkup;`)(state,tab=>tab.region==='kr',escapeHTML,{now:()=>now});
  const friend={playerRef:"friend_cn_ref",availability:"dnd",product:"league_of_legends",gameStatus:"inGame",queueLabel:"灵活排位",championName:"虚空掠夺者",gameStartedAt:now-852000};
  state.friendPresence.set(friend.playerRef,friend);
  const tab={playerRef:"canonical_cn_ref",playerRefs:new Set([friend.playerRef]),region:"cn"};
  assert.match(render(tab),/灵活排位 · 虚空掠夺者.*已进行 14:12/);
  assert.equal(render({...tab,region:"kr"}),"");
  assert.equal(render({playerRef:"other_cn_server_ref",gameName:"same name",region:"cn"}),"");
  for(const changes of [{availability:"offline"},{availability:"away"},{gameStatus:"outOfGame"},{product:"valorant"}]) {
    state.friendPresence.set(friend.playerRef,{...friend,...changes});assert.equal(render(tab),"");
  }
  state.friendPresence.set(friend.playerRef,{...friend,queueLabel:'<img src=x onerror="alert(1)">',gameStartedAt:NaN});
  assert.match(render(tab),/&lt;img/);assert.doesNotMatch(render(tab),/<img|<time/);
});

test('R317 group toggles, identical refresh, status changes, moves and search preserve decoded images',async()=>{
 const dom=new JSDOM('<button id="friends-toggle"><span id="friends-count"></span></button><aside id="friends-dock"><button id="friends-close"></button><input id="friends-search-input"><span id="friends-summary"></span><div id="friends-list"></div></aside>',{url:'http://localhost',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window,d=w.document;
 const timers=new Map();let seq=0;w.setTimeout=(fn,ms)=>{timers.set(++seq,{fn,ms});return seq};w.clearTimeout=id=>timers.delete(id);
 let friends=[{playerRef:'a',gameName:'甲',groupId:1,icon:1,availability:'dnd',gameStatus:'inGame',championId:64},{playerRef:'b',gameName:'乙',groupId:2,icon:2,availability:'chat',gameStatus:'outOfGame'},{playerRef:'c',gameName:'丙',groupId:1,icon:3,availability:'offline'}];
 w.deepLegendsRuntime={escapeHTML};w.fetch=async()=>({ok:true,json:async()=>({groups:[{id:1,name:'默认'},{id:2,name:'自定义'}],friends:structuredClone(friends)})});
 const flush=async()=>{await new Promise(setImmediate);await new Promise(setImmediate)};
 const refresh=async()=>{w.dispatchEvent(new w.Event('deep-legends:friends-updated'));for(const [id,job]of [...timers])if(job.ms===300){timers.delete(id);job.fn()}await flush()};
 try{
  w.eval(source);w.dispatchEvent(new w.CustomEvent('deep-legends:status',{detail:{connected:true}}));await flush();d.querySelector('#friends-toggle').click();await flush();
  const rows=new Map([...d.querySelectorAll('.friend-row')].map(n=>[n.dataset.playerRef,n])),images=[...d.querySelectorAll('.friend-row img')];
  let mutations=[];const observer=new w.MutationObserver(records=>mutations.push(...records));observer.observe(d.querySelector('#friends-list'),{subtree:true,attributes:true,attributeFilter:['src','data-queued-src'],characterData:true,childList:true});
  for(const head of d.querySelectorAll('.friends-group-head')){head.focus();for(let i=0;i<6;i++)head.click();assert.equal(d.activeElement,head)}
  await flush();assert.equal(mutations.filter(m=>m.target.parentElement?.closest('.friend-row')||m.target.matches?.('.friend-row')).length,0,'toggle changes no image attribute or friend text node');
  await refresh();for(const [key,row]of rows)assert.equal(d.querySelector(`[data-player-ref="${key}"]`),row);for(const img of images)assert.equal(img.isConnected,true);
  assert.equal(mutations.filter(m=>m.type==='attributes').length,0);mutations=[];
  friends[1].availability='away';await refresh();assert.match(rows.get('b').querySelector('.friend-status').textContent,/离开/);assert.equal(mutations.some(m=>rows.get('a').contains(m.target)),false,'unchanged friend text is untouched');assert.equal(mutations.filter(m=>m.type==='attributes').length,0);mutations=[];
  friends[0].groupId=2;await refresh();assert.equal(rows.get('a').closest('[data-friends-group]').dataset.friendsGroup,'g2');for(const img of images)assert(img.isConnected);
  const input=d.querySelector('#friends-search-input');input.value='乙';input.dispatchEvent(new w.Event('input'));assert.equal(rows.get('a').hidden,true);assert.equal(rows.get('b').hidden,false);input.value='';input.dispatchEvent(new w.Event('input'));for(const img of images)assert(img.isConnected);
  const pending=[];w.Image=function(){const img=d.createElement('img');pending.push(img);return img};
  const avatar=rows.get('a').querySelector('.friend-avatar img');friends[0].icon=99;await refresh();assert.equal(rows.get('a').querySelector('.friend-avatar img'),avatar,'old decoded image remains while replacement loads');assert.equal(pending.length,1);pending[0].onload();assert.equal(rows.get('a').querySelector('.friend-avatar img'),pending[0]);assert.equal(rows.get('b').querySelector('img'),images.find(n=>rows.get('b').contains(n)));
  observer.disconnect();
 }finally{w.dispatchEvent(new w.Event('deep-legends:dispose'));w.close()}
});
