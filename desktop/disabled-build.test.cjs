"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path"),{JSDOM}=require("jsdom");
const root=path.resolve(__dirname,".."),web=path.join(root,"backend/web");
test("R248 disabled first frame never waits for authorization and hides expiry",()=>{
 const dom=new JSDOM(fs.readFileSync(path.join(web,"default/index.html"),"utf8"),{url:"http://127.0.0.1",runScripts:"outside-only"}),w=dom.window;let calls=0,started=0;
 w.fetch=()=>{calls++;throw Error("unexpected authorization request")};
 const frame=w.document.getElementById("app-frame"),overlay=w.document.getElementById("license-overlay");
 assert.equal(frame.hidden,false,"R248 first frame must show application");assert.equal(frame.hasAttribute("inert"),false);assert.equal(overlay,null,"default build must not embed activation overlay");
 w.eval(fs.readFileSync(path.join(web,"default/license-ui.js"),"utf8"));w.addEventListener("deep-legends:license",()=>started++);w.deepLegendsLicense.poll();w.deepLegendsLicense.poll();
 assert.equal(w.deepLegendsLicense.isActive(),true);assert.equal(started,1);assert.equal(calls,0,"R248 disabled UI must not poll authorization");
 assert.equal(w.document.getElementById("license-form"),null);assert.equal(w.document.getElementById("setting-license-expiry"),null);dom.window.close();
});
test("R248 default native gate allows IPC without status requests",async()=>{
 assert.equal(require("./license-build.cjs").enabled,false);
 const http={request(){assert.fail("disabled native gate requested authorization")}},gate=require("./license-gate.cjs");
 await (await gate.requireActiveLicense(http,()=>null))();assert.deepEqual(await gate.readLicenseSnapshot(http,()=>null),{state:"DISABLED"});
});
test("R248 default release rejects both authorization origins",t=>{
 const os=require("node:os"),dir=fs.mkdtempSync(path.join(os.tmpdir(),"r248-release-"));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));const file=path.join(dir,"backend.exe"),verify=require("./verify-license-release.cjs").verifyLicenseRelease;
 fs.writeFileSync(file,"MZ plain backend");assert.equal(verify(file,root),true);
 for(const marker of ["https://license.yinxiaobia.net","https://license-staging.yinxiaobia.net"]){fs.writeFileSync(file,"MZ "+marker);assert.throws(()=>verify(file,root),/license origin|staging/)}
});
test("R252 disabled release rejects embedded activation text and default assets contain no activation UI",t=>{
 const os=require('node:os'),dir=fs.mkdtempSync(path.join(os.tmpdir(),'r252-assets-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));const file=path.join(dir,'backend.exe'),verify=require(process.env.R252_LICENSE_VERIFIER || './verify-license-release.cjs').verifyLicenseRelease;
 for(const text of ['注册码','授权到期','DL-XXXXX'])for(const encoding of ['utf8','utf16le']){fs.writeFileSync(file,Buffer.concat([Buffer.from('MZ'),Buffer.from(text,encoding)]));assert.throws(()=>verify(file,root),/activation UI content/)}
 const html=fs.readFileSync(path.join(web,'default/index.html'),'utf8'),js=fs.readFileSync(path.join(web,'default/license-ui.js'),'utf8');assert.doesNotMatch(html+js,/注册码|授权到期|license-(?:overlay|form)|license\.yinxiaobia\.net/);
 const dom=new JSDOM(html,{url:'http://127.0.0.1',runScripts:'outside-only'}),w=dom.window;w.fetch=()=>assert.fail('default frontend requested authorization');w.eval(js);w.deepLegendsLicense.poll();assert(w.deepLegendsLicense.isActive());assert(!w.document.getElementById('app-frame').hidden);dom.window.close();
});
