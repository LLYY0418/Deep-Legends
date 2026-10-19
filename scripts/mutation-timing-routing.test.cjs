'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),cp=require('node:child_process');
const root=path.resolve(__dirname,'..');
function deferredRouting(source){
 const definition=source.split('\n').find(line=>line.startsWith('const run='));assert(definition);
 const rows=[],run=vm.runInNewContext(definition+'run',{spawnSync:(command,args,options)=>{rows.push(options.env.R287_MUTATION_PATH);return {stdout:'',stderr:'',status:0};},root,report:'/fixture',path,fs:{writeFileSync(){}},process:{execPath:process.execPath,env:{R287_MUTATION_PATH:'1'}}});
 run([],{},'baseline');run([],{},'mutant',true);run([],{},'restored');assert.deepEqual(rows,['0','1','0']);
}
function pythonRouting(source){
 const code=`import ast,json,os,tempfile,time,subprocess\nfrom pathlib import Path\nfrom types import SimpleNamespace\nsource=${JSON.stringify(source)}\nnode=next(n for n in ast.parse(source).body if isinstance(n,ast.FunctionDef) and n.name=='run')\nseen=[]\ndef fake(command,**kwargs):\n mode=kwargs['env'].get('R287_MUTATION_PATH');seen.append(mode);return SimpleNamespace(returncode=1 if mode=='1' else 0,stdout='AssertionError target',stderr='')\nwith tempfile.TemporaryDirectory(prefix='r287-routing-') as directory:\n ns=dict(os=os,time=time,json=json,root=Path(directory),out=Path(directory),rows=[],subprocess=SimpleNamespace(run=fake))\n exec(compile(ast.Module(body=[node],type_ignores=[]),'r270-run','exec'),ns)\n ns['run']('baseline',[],{},False)\n ns['run']('mutant',[],{},True,'target')\n ns['run']('restored',[],{},False)\n assert seen==['0','1','0'],seen\n`;
 const result=cp.spawnSync(process.platform==='win32'?'python':'python3',['-c',code],{cwd:root,env:{...process.env,R287_MUTATION_PATH:'1'},encoding:'utf8'});assert.equal(result.status,0,result.stdout+result.stderr);
}
test('actual R270 and deferred runner calls isolate baseline, mutant and restored retry modes',()=>{pythonRouting(fs.readFileSync(path.join(__dirname,'r270-mutations.py'),'utf8'));deferredRouting(fs.readFileSync(path.join(__dirname,'r277-deferred-assets-proof.cjs'),'utf8'));});
test('mutations: losing either runner negative-mode switch fails the actual routing oracle',()=>{const py=fs.readFileSync(path.join(__dirname,'r270-mutations.py'),'utf8'),js=fs.readFileSync(path.join(__dirname,'r277-deferred-assets-proof.cjs'),'utf8');assert.throws(()=>pythonRouting(py.replace("'1' if negative else '0'","'0'")),{name:'AssertionError'});assert.throws(()=>deferredRouting(js.replace("mutation?'1':'0'","'0'")),{name:'AssertionError'});});
