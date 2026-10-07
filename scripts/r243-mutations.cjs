"use strict";
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

const fs=require("node:fs"),os=require("node:os"),path=require("node:path"),assert=require("node:assert/strict"),{spawnSync}=require("node:child_process");
const root=path.resolve(__dirname,".."),out=path.join(root,"docs/history/reports/r243/mutations"),temp=fs.mkdtempSync(path.join(os.tmpdir(),"r243-mutations-")),results=[];
fs.mkdirSync(out,{recursive:true});
function killed(name,command,args,env,expected){const started=new Date(),r=spawnSync(command,args,{cwd:root,env:{...process.env,...env},encoding:"utf8",timeout:90000}),log=(r.stdout||"")+(r.stderr||"");fs.writeFileSync(path.join(out,name+".log"),log);assert.equal(r.error,undefined);assert.equal(r.status,1);assert.match(log,expected);assert.doesNotMatch(log,/SyntaxError|build failed|undefined:|error: expected/);results.push({name,exit_code:r.status,assertion_killed:true,started:started.toISOString(),finished:new Date().toISOString(),last5:log.trimEnd().split("\n").slice(-5)})}
try {
 const desktop=path.join(temp,"desktop");fs.mkdirSync(desktop);
 for(const name of fs.readdirSync(path.join(root,"desktop")))if(name.endsWith(".cjs"))fs.copyFileSync(path.join(root,"desktop",name),path.join(desktop,name));
 const target=path.join(desktop,"license-window.cjs"),original=fs.readFileSync(target,"utf8");assert.ok(original.includes('await verifyActiveSize();'));fs.writeFileSync(target,original.replaceAll('await verifyActiveSize();','/* mutation: skip ACTIVE size guard */'));
 killed("a-active-size-guard-module",process.execPath,["--test","--test-name-pattern=R243 startup network recovery",path.join(desktop,"license-window.license.cjs")],{},/ACTIVE minimum guard must repair ignored native sizing/);
 killed("a-active-size-guard-electron",process.execPath,[path.join(root,"scripts/r243-electron.cjs"),"mutation"],{R243_SOURCE_ROOT:temp,R243_ELECTRON_OUTPUT:path.join(out,"electron")},/ACTIVE minimum guard must restore native window/);
 const originalGo=path.join(root,"backend/license.go"),mutant=path.join(temp,"license.go"),source=fs.readFileSync(originalGo,"utf8");assert.ok(source.includes('retryLicenseActivation(err)'));fs.writeFileSync(mutant,source.replace('retryLicenseActivation(err)','false'));
 const overlay=path.join(temp,"overlay.json");fs.writeFileSync(overlay,JSON.stringify({Replace:{[originalGo]:mutant}}));
 killed("b-no-activation-retry","go",["test","-tags=license","-count=1","-overlay="+overlay,"-run","^TestR243ActivationRetriesFreshSignedRequest$/^timeout$","./backend"],{},/FAIL: TestR243ActivationRetriesFreshSignedRequest\/timeout/);
 fs.writeFileSync(path.join(out,"results.json"),JSON.stringify(results,null,2)+"\n");console.log(JSON.stringify(results));
}finally{fs.rmSync(temp,{recursive:true,force:true,maxRetries:5,retryDelay:100})}
