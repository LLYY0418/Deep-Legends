'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path'),{spawnSync}=require('node:child_process');
test('R276 checkout proof catches changes to tracked and new source, with exact file evidence',()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r276-source-proof-'));
 try{
  for(const args of [['init','-q'],['add','tracked.go']]){
   if(args[0]==='add')fs.writeFileSync(path.join(root,'tracked.go'),'package fixture\n');
   const r=spawnSync('git',args,{cwd:root,encoding:'utf8'});assert.ifError(r.error);assert.equal(r.status,0,r.stderr);
  }
  fs.writeFileSync(path.join(root,'new.cjs'),'module.exports=1;\n');
  const helper=path.join(__dirname,'mutation-source-proof.py');
  const code=`import runpy,sys\nfrom pathlib import Path\nh=runpy.run_path(sys.argv[1]);r=Path(sys.argv[2]);before=h['snapshot'](r);h['unchanged'](r,before)\nfor name in ['tracked.go','new.cjs']:\n original=(r/name).read_bytes();(r/name).write_bytes(original+b'changed')\n try:h['unchanged'](r,before)\n except AssertionError as e:\n  assert name in str(e);print('FAIL expected: '+str(e))\n else:raise AssertionError('source mutation survived: '+name)\n (r/name).write_bytes(original)\nh['unchanged'](r,before)\n`;
  const r=spawnSync(process.platform==='win32'?'python':'python3',['-c',code,helper,root],{encoding:'utf8'});assert.ifError(r.error);assert.equal(r.status,0,r.stdout+r.stderr);assert.match(r.stdout,/FAIL expected: .*tracked\.go/);assert.match(r.stdout,/FAIL expected: .*new\.cjs/);
 }finally{fs.rmSync(root,{recursive:true,force:true});}
});
