"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path"),os=require("node:os"),vm=require("node:vm");
const {EventEmitter}=require("node:events");
const {backendExitEvidence,normalizeBackendEvidence,stderrTail,writeRelaunchMarker,consumeRelaunchMarker,STDERR_LIMIT}=require("./backend-evidence.cjs");
const {appendDesktopLogFile,desktopLogForExport}=require("./desktop-log.cjs");
const source=fs.readFileSync(path.join(__dirname,"main.cjs"),"utf8");
const panic=`panic: runtime error: index out of range [9] with length 9

goroutine 89 [running]:
main.(*app).loadGameplayLive(0x123, {fakePlayerName fake-puuid-private-value}, 0x456)
\tC:/Users/fakePlayerName/private/backend/gameplay.go:7719 +0x20
main.(*app).cachedGameplayLive(0x123, fake-puuid-private-value)
\tC:/private/backend/gameplay_refresh.go:196 +0x12
main.(*app).observeGameplayPhase.func2()
\tC:/private/backend/gameplay_refresh.go:89 +0x10
created by main.(*app).observeGameplayPhase in goroutine 2
`;
function rootFor(t){const root=fs.mkdtempSync(path.join(os.tmpdir(),"r175-shell-"));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));return root;}
function harness(root){
 const app=new EventEmitter(),child=new EventEmitter(),ipcMain=new EventEmitter();const handlers=new Map(),order=[];
 Object.assign(app,{isPackaged:false,setAppUserModelId(){},requestSingleInstanceLock:()=>true,whenReady:()=>({then(){}}),getPath:()=>root,relaunch(){order.push("relaunch")},quit(){order.push("quit")}});
 ipcMain.handle=(name,fn)=>handlers.set(name,fn);ipcMain.removeHandler=name=>handlers.delete(name);
 child.stdout=new EventEmitter();child.stderr=new EventEmitter();child.stdout.setEncoding=child.stderr.setEncoding=()=>{};
 const contents={getURL:()=>"http://127.0.0.1:8787/",send(){},isDestroyed:()=>false};
 const electron={app,ipcMain,BrowserWindow:class{},nativeTheme:{},session:{},shell:{},dialog:{showMessageBox:()=>Promise.resolve()}};
 const context=vm.createContext({require(name){if(name==="electron")return electron;if(name==="node:child_process")return{spawn:()=>child};return require(name)},__dirname,process:{on(){},platform:"win32",env:{}},URL,Buffer,console,setTimeout:()=>1,clearTimeout(){}});
 vm.runInContext(source+'\nglobalThis.probe={startBackend,recordRelaunchCompletion,attach(){backendReady={baseUrl:"http://127.0.0.1:8787",token:"test"};mainWindow={isDestroyed:()=>false,webContents:globalThis.contents};setupBackendIPC();}};',context);
 context.contents=contents;context.probe.startBackend();context.probe.attach();return{child,context,contents,handlers,order};
}
test("R175 actual backend close and restart IPC export only safe panic/relaunch evidence",t=>{
 const root=rootFor(t),h=harness(root),logs=path.join(root,"logs");
 h.child.stderr.emit("data",panic.slice(0,90));h.child.stderr.emit("data",panic.slice(90));h.child.emit("close",2,null);
 let rows=desktopLogForExport(logs).trim().split("\n").map(JSON.parse),event=rows.find(row=>row.event==="desktop_backend_exit");
 assert.deepEqual(Object.keys(event).sort(),["event","code","signal","uptime_ms","panic_kind","frames","time"].sort());
 assert.equal(event.code,2);assert.equal(event.signal,"none");assert.ok(event.uptime_ms>=0);assert.equal(event.panic_kind,"index-out-of-range");
 assert.deepEqual(event.frames,["main.(*app).loadGameplayLive","main.(*app).cachedGameplayLive","main.(*app).observeGameplayPhase.func2"]);
 assert.doesNotMatch(JSON.stringify(rows),/fakePlayerName|fake-puuid-private-value|C:\/|7719/);
 assert.match(fs.readFileSync(path.join(logs,"desktop.log"),"utf8"),/fakePlayerName/,"raw stderr remains local only");
 assert.equal(h.handlers.get("desktop-backend-restart")({sender:h.contents}),true);assert.deepEqual(h.order,["relaunch","quit"]);
 const marker=path.join(root,"desktop-relaunch.json");assert.deepEqual(Object.keys(JSON.parse(fs.readFileSync(marker))),["requested_at"]);
 // A newly launched shell consumes the marker through the real startup helper.
 const next=harness(root);next.context.probe.recordRelaunchCompletion();assert.equal(fs.existsSync(marker),false);
 rows=desktopLogForExport(logs).trim().split("\n").map(JSON.parse);
 assert.equal(rows.filter(row=>row.event==="desktop_relaunch_requested").length,1);
 const completed=rows.find(row=>row.event==="desktop_relaunch_completed");assert.ok(completed.interval_ms>=0);
 assert.deepEqual(Object.keys(completed).sort(),["event","interval_ms","time"].sort());
});
test("R175 relaunch marker computes elapsed time, deletes once, tolerates missing/corrupt files",t=>{
 const root=rootFor(t),marker=path.join(root,"desktop-relaunch.json");
 assert.equal(consumeRelaunchMarker(root,1000),null);assert.equal(writeRelaunchMarker(root,1000),true);
 assert.deepEqual(consumeRelaunchMarker(root,1456),{event:"desktop_relaunch_completed",interval_ms:456});assert.equal(fs.existsSync(marker),false);
 for(const data of ["bad JSON",'{}','{"requested_at":"secret-puuid"}','{"requested_at":99999}',"x".repeat(1000)]){
  fs.writeFileSync(marker,data);assert.equal(consumeRelaunchMarker(root,2000),null);assert.equal(fs.existsSync(marker),false);
 }
});
test("R175 stderr is limited to 64 KiB; frame arguments and unreviewed fields cannot pass export",t=>{
 let tail=stderrTail(Buffer.alloc(0),panic);tail=stderrTail(tail,"x".repeat(STDERR_LIMIT+1));assert.equal(tail.length,STDERR_LIMIT);assert.equal(backendExitEvidence(null,"secret-signal",12,tail).panic_kind,"none");
 const root=rootFor(t);appendDesktopLogFile(root,"后端证据 "+JSON.stringify({event:"desktop_backend_exit",code:2,signal:"fake-puuid",uptime_ms:10,panic_kind:"fake-puuid",frames:["main.good", "main.foo(fake-puuid)", "C:/fakePlayerName"],puuid:"fake-puuid",stderr:panic}));
 const rows=desktopLogForExport(root).trim().split("\n").map(JSON.parse);assert.deepEqual(rows[0].frames,["main.good"]);assert.equal(rows[0].signal,"none");assert.equal(rows[0].panic_kind,"other");assert.doesNotMatch(JSON.stringify(rows),/fake-puuid|fakePlayerName|stderr|puuid/);
 assert.deepEqual(normalizeBackendEvidence({event:"desktop_relaunch_requested",puuid:"secret"}),{event:"desktop_relaunch_requested"});
 for(const [text,kind] of [["panic: runtime error: invalid memory address or nil pointer dereference","nil-pointer"],["panic: runtime error: slice bounds out of range [2:1]","slice-bounds"],["panic: interface conversion: secret is int, not string","type-assertion"],["panic: send on closed channel","closed-channel"],["panic: fake-puuid","other"],["ordinary stderr","none"]])assert.equal(backendExitEvidence(1,null,0,Buffer.from(text)).panic_kind,kind);
});
