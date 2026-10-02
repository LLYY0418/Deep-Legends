"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),os=require("node:os"),path=require("node:path");
const {normalizeProcessMetrics,startProcessMetrics}=require("./process-metrics.cjs"),{appendDesktopLogFile,desktopLogForExport}=require("./desktop-log.cjs");
test("R185 desktop metrics sample once per minute by process type and export only safe fields",async t=>{
 const rows=[],timers=[],cleared=[],root=fs.mkdtempSync(path.join(os.tmpdir(),"r185-desktop-"));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const app={getAppMetrics:()=>[{type:"Browser",cpu:{percentCPUUsage:2},memory:{workingSetSize:2048},name:"private-name"},{type:"Renderer",cpu:{percentCPUUsage:4},memory:{workingSetSize:1024}},{type:"Renderer",cpu:{percentCPUUsage:6},memory:{workingSetSize:3072}},{type:"GPU",cpu:{percentCPUUsage:1},memory:{workingSetSize:1024}},{type:"Utility",cpu:{percentCPUUsage:0},memory:{workingSetSize:512}},{type:"private",cpu:{percentCPUUsage:1},memory:{workingSetSize:1}}]};
 const m=startProcessMetrics({app,getBackendPid:()=>123,readBackend:async pid=>{assert.equal(pid,123);return 8;},report:r=>{rows.push(r);appendDesktopLogFile(root,"进程指标 "+JSON.stringify({...r,command:"private-command"}));},interval:(fn,ms)=>{timers.push({fn,ms});return 1;},clear:id=>cleared.push(id)});
 assert.equal(timers[0].ms,60000);assert.equal(rows.length,0);await timers[0].fn();assert.equal(rows[0].processes.length,4);assert.deepEqual(rows[0].processes[1],{type:"Renderer",cpu_percent:10,working_set_mb:4,count:2});assert.equal(rows[0].backend_working_set_mb,8);
 const exported=desktopLogForExport(root);assert(!exported.includes("private"));const event=JSON.parse(exported.trim());assert.equal(event.event,"desktop_process_metrics");assert.equal(event.backend_working_set_mb,8);
 m.dispose();await m.sample();assert.equal(rows.length,1);assert.deepEqual(cleared,[1]);
});
test("R185 unknown backend memory is omitted and disposing a pending sample prevents late export",async()=>{
 let finish;const rows=[],m=startProcessMetrics({app:{getAppMetrics:()=>[]},getBackendPid:()=>3,readBackend:()=>new Promise(resolve=>finish=resolve),report:r=>rows.push(r),interval:()=>1,clear(){}});
 const work=m.sample();await m.sample();m.dispose();finish(2);await work;assert.equal(rows.length,0);
 assert(!Object.hasOwn(normalizeProcessMetrics({}),"backend_working_set_mb"));
 const main=fs.readFileSync(path.join(__dirname,"main.cjs"),"utf8");assert.match(main,/processMetrics\?\.dispose\(\)/);assert.match(main,/getBackendPid:\(\)=>backend\?\.pid/);
});
