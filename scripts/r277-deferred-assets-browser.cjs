'use strict';
// Extend the existing isolated Chromium harness; do not bypass the production
// embedded module in the success case. Recovery injects exactly one explicit 503.
const fs=require('node:fs'),path=require('node:path'),Module=require('node:module'),assert=require('node:assert/strict');
let wrapper=fs.readFileSync(path.join(__dirname,'r277-frontend-browser.cjs'),'utf8');
function edit(a,b){assert.equal(wrapper.split(a).length,2,'deferred wrapper anchor changed');wrapper=wrapper.replace(a,b);}
edit("require('./r277-frontend-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})","require('./r277-deferred-assets-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})");
const edits=[];
if(process.env.R277_DEFERRED_FAULT==='503'){
 edits.push(["'*/api/gameplay/summoner-spells*'","'*/match-details.js','*/api/gameplay/summoner-spells*'"]);
 edits.push(['let currentCase,clickEvidence;','let currentCase,clickEvidence;let deferredFaultInjected=false;']);
 edits.push(['   const fixture=browserAPI(new URL(url));',"   if(pathname==='/match-details.js'&&!deferredFaultInjected){deferredFaultInjected=true;await send('Fetch.fulfillRequest',{requestId:id,responseCode:503,responseHeaders:[{name:'Content-Type',value:'text/javascript'}],body:Buffer.from('fixture deliberate 503').toString('base64')},m.sessionId);return;}\n   const fixture=browserAPI(new URL(url));"]);
 edits.push(['assert.equal(errors.length,0,JSON.stringify(errors));',"assert.equal(errors.length,1,JSON.stringify(errors));assert(errors[0].url?.endsWith('/match-details.js') && errors[0].text.includes('503'),JSON.stringify(errors));"]);
}
edit('const m=new Module(__filename,module);',edits.map(([a,b])=>`replace(${JSON.stringify(a)},${JSON.stringify(b)});`).join('\n')+'\nconst m=new Module(__filename,module);');
const m=new Module(__filename,module);m.filename=__filename;m.paths=module.paths;m._compile(wrapper,__filename);
