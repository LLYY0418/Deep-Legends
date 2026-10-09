'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path'),{EventEmitter}=require('node:events');
const {attachDiagnosticsExport}=require('./diagnostics-export.cjs');
test('R264 P4 Electron rejects truncated bytes and a partial last line, preserving only complete files',async()=>{
 const directory=fs.mkdtempSync(path.join(os.tmpdir(),'r264-export-'));
 try{for(const kind of ['short','partial','complete']){
  const session=new EventEmitter(),sender={},completed=[],errors=[];
  const detach=attachDiagnosticsExport({session,sender,getBaseURL:()=> 'http://127.0.0.1:8795',getDirectory:()=>directory,fileSystem:fs,onCompleted:file=>completed.push(file),onError:error=>errors.push(error.message)});
  const item=new EventEmitter();item.getURL=()=> 'http://127.0.0.1:8795/api/diagnostics/log';item.setSavePath=file=>item.file=file;item.getSavePath=()=>item.file;
  item.getReceivedBytes=()=>fs.existsSync(item.file)?fs.statSync(item.file).size:0;item.getTotalBytes=()=>kind==='short'?100:Buffer.byteLength(kind==='partial'?'{}':'{}\n');
  session.emit('will-download',{},item,sender);fs.writeFileSync(item.file,kind==='partial'?'{}':'{}\n');item.emit('done',{},'completed');
  if(kind==='complete'){assert.deepEqual(completed,[item.file]);assert.deepEqual(errors,[]);assert(fs.existsSync(item.file));}
  else{assert.deepEqual(completed,[]);assert.deepEqual(errors,['导出不完整，请重试']);assert(!fs.existsSync(item.file));}
  detach();
 }}finally{fs.rmSync(directory,{recursive:true,force:true});}
});
