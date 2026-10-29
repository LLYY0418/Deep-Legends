'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const [candidate,output]=process.argv.slice(2),out=path.resolve(output);fs.mkdirSync(out,{recursive:true});
if(!process.versions.electron){
 const {spawnSync}=require('node:child_process'),env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
 const executable=require(path.join(candidate,'desktop/node_modules/electron'));
 const result=spawnSync(executable,[__filename,candidate,out],{env,stdio:'inherit',timeout:60000});
 if(result.error)fs.writeFileSync(path.join(out,'launch-error.json'),JSON.stringify({name:result.error.name,code:result.error.code}));
 process.exit(result.status??1);
}else{
 const {app,BrowserWindow}=require('electron'),{installResizeBorder}=require(path.join(candidate,'desktop/resize-border.cjs'));
 app.whenReady().then(async()=>{
  const events=[],window=new BrowserWindow({show:false,titleBarStyle:'hidden',webPreferences:{contextIsolation:true,nodeIntegration:false}});
  assert.equal(installResizeBorder(window,{report:event=>events.push(event)}),true);
  await window.loadURL('data:text/html,<main>Native module probe</main>');
  assert.equal(installResizeBorder(window,{report:event=>events.push(event)}),true);
  assert.deepEqual(events,[]);
  const fallback=new BrowserWindow({show:false,titleBarStyle:'hidden',webPreferences:{contextIsolation:true,nodeIntegration:false}}),load=()=>{throw Error('simulated unavailable module')};
  for(let i=0;i<2;i++)assert.equal(installResizeBorder(fallback,{load,report:event=>events.push(event)}),false);
  await fallback.loadURL('data:text/html,<main>Fallback window is usable</main>');
  assert.equal(fallback.isDestroyed(),false);assert.equal(await fallback.webContents.executeJavaScript('document.querySelector("main").textContent'),'Fallback window is usable');
  assert.deepEqual(events,[{event:'resize_border_unavailable',error_type:'Error'}]);
  const result={version:require(path.join(candidate,'desktop/package.json')).version,fingerprint:require(path.join(candidate,'desktop/source-fingerprint.cjs')).sourceFingerprint(candidate),installed:true,idempotent:true,fallbackReturnedFalse:true,fallbackRendererUsable:true,events};
  fs.writeFileSync(path.join(out,'native-probe.json'),JSON.stringify(result,null,2));console.log(JSON.stringify(result));window.destroy();fallback.destroy();app.exit(0);
 }).catch(error=>{fs.writeFileSync(path.join(out,'failure.json'),JSON.stringify({name:error.name,message:error.message},null,2));console.error(error);app.exit(1)});
}
