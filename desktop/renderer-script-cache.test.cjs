"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { JSDOM } = require("jsdom");
const { runRendererScript } = require(process.env.R278_RENDERER_CACHE_MODULE || "./renderer-script-cache.cjs");
test("compiled renderer cache keeps window state and changed source isolated", () => {
  const a = new JSDOM("<p>one</p>", {runScripts:"outside-only",url:"http://localhost"});
  const b = new JSDOM("<p>two</p>", {runScripts:"outside-only",url:"http://localhost"});
  try {
    const source = "window.runs=(window.runs||0)+1;document.querySelector('p').textContent+='!';localStorage.setItem('runs',String(window.runs));";
    runRendererScript(a,source,"renderer.js");runRendererScript(a,source,"renderer.js");runRendererScript(b,source,"renderer.js");
    assert.equal(a.window.runs,2);assert.equal(b.window.runs,1);
    assert.equal(a.window.document.querySelector('p').textContent,"one!!");assert.equal(b.window.document.querySelector('p').textContent,"two!");
    assert.equal(a.window.localStorage.getItem('runs'),"2");assert.equal(b.window.localStorage.getItem('runs'),"1");
    runRendererScript(b,"window.runs=99;","renderer.js");assert.equal(b.window.runs,99);assert.equal(a.window.runs,2);
  } finally {a.window.close();b.window.close();}
});
