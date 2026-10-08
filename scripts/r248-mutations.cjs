"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
const fs=require("node:fs"),path=require("node:path"),os=require("node:os"),assert=require("node:assert/strict"),{spawnSync}=require("node:child_process");
const root=path.resolve(__dirname,".."),out=process.env.R248_MUTATION_OUT||evidencePath('r248/mutations');fs.mkdirSync(out,{recursive:true});
const temp=fs.mkdtempSync(path.join(os.tmpdir(),"r248-mutations-")),results=[];
function killed(name,args,env,pattern){const begin=Date.now(),p=spawnSync(process.execPath,args,{cwd:root,env:{...process.env,...env},encoding:"utf8",timeout:45000});const log=p.stdout+p.stderr;fs.writeFileSync(path.join(out,name+".log"),log);assert.equal(p.status,1,name+" must fail assertions");assert.match(log,pattern);results.push({name,status:p.status,elapsed_ms:Date.now()-begin,assertion_failed:true});}
try{
 const mutated=path.join(temp,"frontend");fs.mkdirSync(mutated);
 fs.cpSync(path.join(root,"desktop"),path.join(mutated,"desktop"),{recursive:true,filter:f=>!f.includes(path.sep+"node_modules")&&!f.endsWith(".exe")});
 fs.cpSync(path.join(root,"backend/web"),path.join(mutated,"backend/web"),{recursive:true});
 const file=path.join(mutated,"backend/web/license-ui.js"),s=fs.readFileSync(file,"utf8"),from='disableLicense(); return;',to='disableLicense(); layer.hidden = false; form.hidden = false; frame.hidden = true; document.documentElement.dataset.license = "locked"; return;';assert.ok(s.includes(from));fs.writeFileSync(file,s.replace(from,to));
 const launcher=path.join(temp,"overlay.cjs");fs.writeFileSync(launcher,`require(${JSON.stringify(path.join(root,"scripts/r248-electron.cjs"))}).probe({phase:"after",state:"DISABLED",sourceRoot:${JSON.stringify(mutated)},index:1}).catch(e=>{console.error(e);process.exitCode=1});`);
 killed("default-overlay",[launcher],{R248_ELECTRON_OUTPUT:path.join(out,"electron-overlay")},/AssertionError.*|R248 first frame must show disabled main UI/);
 const go=path.join(root,"backend/license_disabled.go"),source=fs.readFileSync(go,"utf8"),mut=source.replace('func (a *app) startApplicationBusiness(ctx context.Context) {','func (a *app) startApplicationBusiness(ctx context.Context) {\n response, _ := http.Get("https://license.yinxiaobia.net/mutation"); if response != nil { response.Body.Close() }');assert.notEqual(mut,source);
 const overlaySource=path.join(temp,"disabled.go");fs.writeFileSync(overlaySource,mut);const overlay=path.join(temp,"go-overlay.json");fs.writeFileSync(overlay,JSON.stringify({Replace:{[go]:overlaySource}}));
 const begin=Date.now(),p=spawnSync("go",["test","-count=1","-overlay",overlay,"-run","^TestR248DisabledStartupNoAuthorizationNetwork$","./backend"],{cwd:root,env:{...process.env,GOFLAGS:""},encoding:"utf8",timeout:120000}),log=p.stdout+p.stderr;fs.writeFileSync(path.join(out,"default-network.log"),log);assert.equal(p.status,1);assert.match(log,/default startup must issue zero authorization requests 1/);assert.doesNotMatch(log,/build failed|undefined:|syntax error/);results.push({name:"default-network",status:p.status,elapsed_ms:Date.now()-begin,requests:1,assertion_failed:true});
 fs.writeFileSync(path.join(out,"results.json"),JSON.stringify(results,null,2));console.log(JSON.stringify(results));
}finally{fs.rmSync(temp,{recursive:true,force:true,maxRetries:5,retryDelay:100})}
