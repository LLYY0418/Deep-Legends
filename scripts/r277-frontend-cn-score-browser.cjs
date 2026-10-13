'use strict';
const fs=require('node:fs'),path=require('node:path'),Module=require('node:module'),assert=require('node:assert/strict');
assert(process.env.R277_CN_SCORE_SERVER,'R277_CN_SCORE_SERVER ready file is required');
const ready=JSON.parse(fs.readFileSync(process.env.R277_CN_SCORE_SERVER,'utf8'));
const fixtureURL=new URL(ready.url);assert.equal(fixtureURL.hostname,'127.0.0.1','fixture server must be isolated loopback');
let wrapper=fs.readFileSync(path.join(__dirname,'r277-frontend-browser.cjs'),'utf8');
function change(a,b){assert.equal(wrapper.split(a).length,2,'unexpected shared browser wrapper anchor');wrapper=wrapper.replace(a,b);}
change("require('./r277-frontend-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})", "require('./r277-frontend-cn-score-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})");
const upstream=`   if(pathname==='/api/gameplay/match'){const response=await fetch(${JSON.stringify(ready.url)}+pathname,{method:'POST',headers:{'Content-Type':'application/json'},body:m.params.request.postData});const body=await response.text();await send('Fetch.fulfillRequest',{requestId:id,responseCode:response.status,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(body).toString('base64')},m.sessionId);return;}\n   const fixture=browserAPI(new URL(url));`;
const inserts=[
 ["'*/api/gameplay/summoner-spells*'","'*/api/gameplay/match','*/api/gameplay/summoner-spells*'"],
 ["   const fixture=browserAPI(new URL(url));",upstream],
 ['const intercept=`',"const intercept=`if(pathname==='/api/gameplay/match')return nativeFetch(input,init);\n"],
];
change('const m=new Module(__filename,module);',inserts.map(([a,b])=>`replace(${JSON.stringify(a)},${JSON.stringify(b)});`).join('\n')+'\nconst m=new Module(__filename,module);');
const m=new Module(__filename,module);m.filename=__filename;m.paths=module.paths;m._compile(wrapper,__filename);
