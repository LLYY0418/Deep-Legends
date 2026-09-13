"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),vm=require("node:vm"),path=require("node:path"),{EventEmitter}=require("node:events");
const source=fs.readFileSync(path.join(__dirname,"main.cjs"),"utf8");
function harness(){
 const app=new EventEmitter(),child=new EventEmitter(),messages=[];let quits=0,kills=0;
 Object.assign(app,{isPackaged:false,setAppUserModelId(){},requestSingleInstanceLock:()=>true,whenReady:()=>({then(){}}),getPath:()=>"/test",quit(){quits++;}});
 child.stdout=new EventEmitter();child.stderr=new EventEmitter();child.stdout.setEncoding=child.stderr.setEncoding=()=>{};child.kill=()=>kills++;
 const electron={app,BrowserWindow:class{},dialog:{showMessageBox(value){messages.push(value);return Promise.resolve();}},ipcMain:new EventEmitter(),nativeTheme:{},session:{},shell:{}};
 const context=vm.createContext({require(name){if(name==="electron")return electron;if(name==="node:child_process")return {spawn:()=>child};if(name==="node:fs")return {mkdirSync(){},appendFileSync(){}};return require(name);},__dirname,process:{platform:"win32",env:{}},URL,Buffer,console,setTimeout:()=>1,clearTimeout(){}});
 vm.runInContext(source+'\nglobalThis.probe={startBackend,onBackendStdout,ready(){backendReady={baseUrl:"http://127.0.0.1:8787",token:"x"};},state(){return {quitting,shutdownStarted}}};',context);
 context.probe.startBackend();context.probe.ready();
 return {child,app,probe:context.probe,messages,get quits(){return quits},get kills(){return kills}};
}
test("upgrade stdout may arrive after exit but before close without a false startup error",()=>{
 const h=harness();h.child.emit("exit",0,null);assert.equal(h.messages.length,0);
 h.child.stdout.emit("data","LOOT_QU");h.child.stdout.emit("data","IT update\n");h.child.emit("close",0,null);
 assert.equal(h.quits,1);assert.equal(h.kills,0);assert.equal(h.messages.length,0);
 let prevented=false;h.app.emit("before-quit",{preventDefault(){prevented=true}});assert.equal(prevented,false);
 assert.equal(h.probe.state().shutdownStarted,true);
});
test("user quit shares the graceful path; unexpected backend death still reports",()=>{
 const h=harness();h.child.stdout.emit("data","LOOT_QUIT user\n");h.child.emit("close",0,null);assert.equal(h.quits,1);assert.equal(h.kills,0);assert.equal(h.messages.length,0);
 const crashed=harness();crashed.child.emit("close",1,null);assert.equal(crashed.messages.length,1);assert.match(crashed.messages[0].message,/本地数据服务已退出/);
});
