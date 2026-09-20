"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
function fixture(flush) {
 const source=fs.readFileSync(path.join(__dirname,"app.js"),"utf8");
 const start=source.indexOf('  el.exportDiagnostics.addEventListener("click", async (event) => {');
 const code=source.slice(start,source.indexOf('  el.copyDiagnostics.addEventListener(',start));
 let handler,downloads=0,folders=0;
 const el={exportDiagnostics:{addEventListener:(_type,fn)=>handler=fn}};
 const link={click:()=>downloads++,remove:()=>{}};
 const window={flushFlowDiagnostics:flush,desktopDiagnostics:{openFolder:async()=>folders++}};
 const document={createElement:()=>link,body:{appendChild:()=>{}}};
 const setReady=Function("el","window","document","diagnosticExportFilename","showToast",`let diagnosticsExportReady=false,diagnosticsExportPending=false;function resetDiagnosticsExport(){diagnosticsExportReady=false;} ${code}; return value=>diagnosticsExportReady=value;`)(el,window,document,()=>"fixture.jsonl",()=>{});
 return {click:()=>handler({preventDefault:()=>{}}),setReady,downloads:()=>downloads,folders:()=>folders,link};
}
test("diagnostic export waits for telemetry and prevents duplicate download clicks",async()=>{
 let resolve;const f=fixture(()=>new Promise(r=>resolve=r));
 const pending=f.click();await f.click();assert.equal(f.downloads(),0);
 resolve();await pending;assert.equal(f.downloads(),1);assert.equal(f.link.href,"/api/diagnostics/log");assert.equal(f.link.download,"fixture.jsonl");
});
test("telemetry failure never blocks export and open-folder action does not export again",async()=>{
 let calls=0;const f=fixture(async()=>{calls++;throw Error("offline");});
 await f.click();assert.equal(f.downloads(),1);
 f.setReady(true);await f.click();assert.equal(f.folders(),1);assert.equal(calls,1);assert.equal(f.downloads(),1);
});
