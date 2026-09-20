'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const {EventEmitter}=require('node:events');
const {createDiagnosticsDirectoryController}=require(process.env.R100_DIRECTORY_SOURCE||'./diagnostics-directory.cjs');
const {attachDiagnosticsExport}=require('./diagnostics-export.cjs');
function fixture(t){
 const root=fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(),'r100-export-')));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const userData=path.join(root,'private'),downloads=path.join(root,'downloads'),second=path.join(root,'second');for(const p of [userData,downloads,second])fs.mkdirSync(p);
 const sender={},event={sender};let prompts=0,selected=second,canceled=false;
 const args={app:{getPath:key=>({userData,downloads})[key]},dialog:{async showOpenDialog(){prompts++;return {canceled,filePaths:[selected]};}},fileSystem:fs,isTrustedRenderer:x=>x===sender,getMainWindow:()=>({webContents:sender})};
 const create=()=>createDiagnosticsDirectoryController(args);
 return {root,userData,downloads,second,sender,event,create,get prompts(){return prompts},set selected(v){selected=v},set canceled(v){canceled=v}};
}
test('R100 P7 first native save persists independently; second and restart save without prompt; no overwrite',async t=>{
 const f=fixture(t),controller=f.create(),session=new EventEmitter(),completed=[];
 const detach=attachDiagnosticsExport({session,sender:f.sender,getBaseURL:()=> 'http://localhost:7777',getDefaultDirectory:()=>f.downloads,getDirectory:controller.getDirectory,prepareFile:controller.prepareFile,finalizeFile:controller.finalizeFile,discardFile:controller.discardFile,fileSystem:fs,onCompleted:file=>{controller.rememberFile(file);completed.push(file)}});t.after(detach);
 function download(){const item=new EventEmitter();item.getURLChain=()=>['http://localhost:7777/api/diagnostics/log'];item.setSaveDialogOptions=o=>item.options=o;item.setSavePath=p=>item.file=p;item.getSavePath=()=>item.file;session.emit('will-download',{},item,f.sender);return item;}
 const first=download();assert.equal(first.file,undefined);assert.equal(path.dirname(first.options.defaultPath),f.downloads);
 first.file=path.join(f.downloads,'first.jsonl');fs.writeFileSync(first.file,'first');first.emit('done',{},'completed');await new Promise(setImmediate);
 assert.equal(controller.getDirectory(),f.downloads);assert.equal(f.create().getDirectory(),f.downloads);assert.equal(completed.length,1);
 const next=download();assert.ok(next.file.includes('diagnostics-stage-'));fs.writeFileSync(next.file,'second');next.emit('done',{},'completed');await new Promise(setImmediate);
 assert.equal(f.prompts,0);assert.equal(completed.length,2);assert.equal(fs.readFileSync(first.file,'utf8'),'first');
 const staged=controller.prepareFile(completed[1]);fs.writeFileSync(staged,'third');const collision=await controller.finalizeFile(staged);assert.notEqual(collision,completed[1]);assert.equal(fs.readFileSync(completed[1],'utf8'),'second');
 assert.equal(fs.existsSync(path.join(f.userData,'share-export.json')),false);
});
test('R100 P7 deleted/replaced directory re-prompts at actual write; cancel cleans staging',async t=>{
 const f=fixture(t),c=f.create();c.rememberFile(path.join(f.downloads,'first.jsonl'));
 let staged=c.prepareFile(path.join(f.downloads,'next.jsonl'));fs.writeFileSync(staged,'evidence');
 fs.renameSync(f.downloads,f.downloads+'-old');fs.mkdirSync(f.downloads);
 const final=await c.finalizeFile(staged);assert.equal(path.dirname(final),f.second);assert.equal(f.prompts,1);assert.equal(fs.existsSync(staged),false);assert.equal(fs.readdirSync(f.downloads).length,0);
 staged=c.prepareFile(path.join(f.second,'cancel.jsonl'));fs.writeFileSync(staged,'secret');c.discardFile(staged);assert.equal(fs.existsSync(path.dirname(staged)),false);
 fs.rmSync(f.second,{recursive:true});assert.equal(c.getDirectory(),'');await c.chooseSaveDirectory(f.event).catch(()=>{});assert.equal(f.prompts,2);
});
test('R100 P7 rejects foreign renderer and symbolic link files; packaged controller',async t=>{
 const f=fixture(t),c=f.create();assert.throws(()=>c.getSaveDirectory({sender:{}}),/untrusted/);assert.throws(()=>c.chooseSaveDirectory({sender:{}}),/untrusted/);assert.equal(f.prompts,0);
 await c.chooseSaveDirectory(f.event);assert.equal(c.getDirectory(),f.second);
 const source=path.join(f.root,'private-secret');fs.writeFileSync(source,'secret');const staged=c.prepareFile(path.join(f.second,'safe.jsonl'));fs.symlinkSync(source,staged);await assert.rejects(c.finalizeFile(staged),/untrusted/);assert.equal(fs.readFileSync(source,'utf8'),'secret');
 assert.ok(require('./package.json').build.files.includes('diagnostics-directory.cjs'));
});

test('R100 P7 failed SavePath access and window disposal remove private staged downloads',async t=>{
 for(const mode of ['throw','relative','cancelled','dispose']){
  const f=fixture(t),c=f.create();c.rememberFile(path.join(f.downloads,'initial.jsonl'));
  const session=new EventEmitter();const detach=attachDiagnosticsExport({session,sender:f.sender,getBaseURL:()=> 'http://localhost:7777',getDirectory:c.getDirectory,prepareFile:c.prepareFile,finalizeFile:c.finalizeFile,discardFile:c.discardFile,fileSystem:fs});
  const item=new EventEmitter();item.getURLChain=()=>['http://localhost:7777/api/diagnostics/log'];item.setSavePath=p=>item.file=p;item.getSavePath=()=>item.file;
  session.emit('will-download',{},item,f.sender);const staged=item.file;fs.writeFileSync(staged,'evidence');
  if(mode==='throw')item.getSavePath=()=>{throw Error('gone')};if(mode==='relative')item.getSavePath=()=> 'relative';
  if(mode==='dispose')detach();else item.emit('done',{},mode==='cancelled'?'cancelled':'completed');
  await new Promise(setImmediate);assert.equal(fs.existsSync(path.dirname(staged)),false,mode);detach();
 }
});
