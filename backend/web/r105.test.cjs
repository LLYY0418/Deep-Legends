"use strict";
const test = require("node:test"), assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path");
const { JSDOM } = require("../../desktop/node_modules/jsdom");
const settle=()=>new Promise(setImmediate);

test("R105 all 53 reviewed accounts remain available after expanding unranked groups",async()=>{
  const doc=fs.readFileSync(path.join(__dirname,"../../docs/pro-accounts-verification-2026-09-17.md"),"utf8");
  const teams=[];let team;
  for(const line of doc.split("\n")) {
    const heading=line.match(/^## (BLG|IG|T1|HLE|GEN|DK)（/);
    if(heading){team={code:heading[1],name:heading[1],players:[]};teams.push(team);continue}
    if(line.startsWith("## "))team=null;
    if(!team || !line.startsWith("|"))continue;
    const columns=line.split("|"), id=columns[3]?.match(/`([^`]+)`/);if(!id)continue;
    const name=columns[1].trim().replaceAll("*","");let player=team.players.find(x=>x.name===name);
    if(!player){player={key:team.code+"/"+name,name,position:"top",accounts:[]};team.players.push(player)}
    const [gameName,tagLine]=id[1].split("#");player.accounts.push({gameName,tagLine,reviewed:true,dormant:true,rankStatus:"unavailable"});
  }
  const dom=new JSDOM(fs.readFileSync(path.join(__dirname,"index.html"),"utf8"),{url:"http://localhost/",runScripts:"outside-only",pretendToBeVisual:true});
  try {
    const w=dom.window;w.fetch=async()=>({ok:true,json:async()=>({teams,playerCount:33,accountCount:53})});
    w.eval(fs.readFileSync(process.env.R105_PRO_SOURCE || path.join(__dirname,"pro-players.js"),"utf8"));
    w.dispatchEvent(new w.CustomEvent("deep-legends:section",{detail:{name:"pro-players"}}));await settle();
    assert.equal(w.document.querySelectorAll("[data-pro-account]").length,0,"unknown accounts should start folded");
    const keys=[...w.document.querySelectorAll("[data-pro-history]")].map(x=>x.dataset.proHistory);
    for(const key of keys) [...w.document.querySelectorAll("[data-pro-history]")].find(x=>x.dataset.proHistory===key).click();
    const buttons=w.document.querySelectorAll("[data-pro-account]");assert.equal(buttons.length,53);
    for(const team of teams) for(const player of team.players) for(const a of player.accounts) assert.ok([...buttons].some(b=>b.dataset.proAccount===`${player.key}:${a.gameName}#${a.tagLine}`),`${player.name} missing ${a.gameName}`);
    assert.equal(w.document.querySelectorAll(".pro-player-name").length,33);
  } finally {dom.window.close()}
});
