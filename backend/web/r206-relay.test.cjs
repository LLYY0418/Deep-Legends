'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile:compileSource,escapeHTML}=require('./r188-harness.cjs');
const compile=(source,names,deps)=>compileSource(source.replace(/^[ \t]*\/\/.*$/gm,''),names,deps);
const app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),gameplay=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8');
test('R206 relay settings and unavailable rune service show built-in state and retry',()=>{
 const el={settingRiotKeyState:{textContent:''}};
 compile(app,['renderRiotKeySettings'],{el}).renderRiotKeySettings({source:'relay',status:'configured'});
 assert.equal(el.settingRiotKeyState.textContent,'使用内置服务');
 const state={live:{},specialistRuneFailures:new Map([['fixture',{reason:'riot-relay-unavailable'}]])};
 const f=compile(gameplay,['renderRuneSourceSection'],{state,escapeHTML,specialistRequestTarget:()=>({key:'fixture'}),specialistRuneFailure:v=>v});
 const dom=new JSDOM(f.renderRuneSourceSection({key:'specialist',items:[],failed:true},true));
 assert.equal(dom.window.document.querySelector('strong').textContent,'战绩服务暂时不可用');
 assert.equal(dom.window.document.querySelector('p'),null);
 assert.equal(dom.window.document.querySelector('[data-retry-specialist-runes]').textContent,'重试');
 assert.equal(dom.window.document.querySelector('[data-open-riot-settings]'),null);dom.window.close();
});
