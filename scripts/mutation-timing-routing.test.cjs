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

function rendererSpeedupRouting(source){
 const definition=source.split('\n').filter(line=>line.trimStart().startsWith('const result=cp.spawnSync('));assert.equal(definition.length,1);
 const rows=[];
 for(const inherited of ['0','1',undefined])vm.runInNewContext(definition[0],{
  cp:{spawnSync(command,args,options){rows.push(options.env.R287_MUTATION_PATH);assert.equal(options.cwd,'/fixture');assert.equal(options.env.NODE_OPTIONS,'');return {status:1};}},
  process:{execPath:process.execPath,env:{R287_MUTATION_PATH:inherited,NODE_OPTIONS:'parent-options'}},testFile:'desktop/overview-render.test.cjs',sandbox:'/fixture'
 });
 assert.deepEqual(rows,['1','1','1'],'this runner has only mutant subprocesses, regardless of the parent mode');
}
test('R289 renderer-speedup mutant subprocess explicitly disables P5 retry',()=>{
 rendererSpeedupRouting(fs.readFileSync(path.join(__dirname,'renderer-speedup-mutations.cjs'),'utf8'));
});
test('R289 removing the renderer-speedup child switch fails its routing oracle',()=>{
 const source=fs.readFileSync(path.join(__dirname,'renderer-speedup-mutations.cjs'),'utf8'),needle=",R287_MUTATION_PATH:'1'";assert.equal(source.split(needle).length,2);
 assert.throws(()=>rendererSpeedupRouting(source.replace(needle,'')),{name:'AssertionError'});
});

function sharedProofRouting(runner,proof){
 const program=`import ast,json,os,re,tempfile
from pathlib import Path
from types import SimpleNamespace
runner=${JSON.stringify(runner)}
proof=${JSON.stringify(proof)}
tree=ast.parse(runner)
definition=next(n for n in ast.parse(proof).body if isinstance(n,ast.ClassDef) and n.name=='Proof')
method=next(n for n in definition.body if isinstance(n,ast.FunctionDef) and n.name=='run')
scope={}
assignments=[n for n in tree.body if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id in ('CASES','PATTERN') for t in n.targets)]
exec(compile(ast.Module(body=assignments,type_ignores=[]),'runner-cases','exec'),scope)
tests=sorted(set(case[-1] for case in scope['CASES']))
seen=[]
def fake(command,**options):
 mode=options['env'].get('R287_MUTATION_PATH');seen.append(mode)
 selected=[test for test in tests if re.search(command[command.index('-run')+1],test)]
 for test in selected:options['stdout'].write(json.dumps(dict(Test=test,Action='fail' if mode=='1' else 'pass',Elapsed=0))+'\\n')
 return SimpleNamespace(returncode=1 if mode=='1' else 0)
namespace=dict(json=json,re=re,subprocess=SimpleNamespace(run=fake))
exec(compile(ast.Module(body=[method],type_ignores=[]),'actual-Proof.run','exec'),namespace)
actual_run=namespace['run']
with tempfile.TemporaryDirectory(prefix='r289-proof-routing-') as directory:
 class CapturedProof:
  def __init__(self,out,cache):self.out=Path(directory);self.root=Path(directory);self.env=dict(os.environ);self.tests=tests;self.timeout=180;self.result={'runs':[]}
  def __enter__(self):return self
  def __exit__(self,*args):return False
  def persist(self):pass
  def run(self,*args,**kwargs):return actual_run(self,*args,**kwargs)
  def mutate(self,label,relative,changes,pattern,expected_fail):return actual_run(self,label,pattern,expected_fail=expected_fail)
 scope['helper']=SimpleNamespace(arguments=lambda description:Path(directory),Proof=CapturedProof)
 scope['__doc__']='routing probe'
 main=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='main')
 exec(compile(ast.Module(body=[main],type_ignores=[]),'actual-runner.main','exec'),scope)
 scope['main']()
 assert seen==['0']+['1']*len(scope['CASES'])+['0'],seen
`;
 const result=cp.spawnSync(process.platform==='win32'?'python':'python3',['-c',program],{cwd:root,env:{...process.env,R287_MUTATION_PATH:'1'},encoding:'utf8'});
 assert.equal(result.status,0,result.stdout+result.stderr);
}
test('R289 actual client and wire runners retain shared proof baseline/mutant/restored routing',()=>{
 const proof=fs.readFileSync(path.join(__dirname,'r277-ci-proof.py'),'utf8');
 for(const file of ['r277-client-mutations.py','r277-wire-diagnostic-mutations.py'])sharedProofRouting(fs.readFileSync(path.join(__dirname,file),'utf8'),proof);
});
test('R289 removing the shared proof switch fails both actual runner routing oracles',()=>{
 const proof=fs.readFileSync(path.join(__dirname,'r277-ci-proof.py'),'utf8'),needle='"1" if expected_fail else "0"';assert.equal(proof.split(needle).length,2);
 for(const file of ['r277-client-mutations.py','r277-wire-diagnostic-mutations.py'])assert.throws(()=>sharedProofRouting(fs.readFileSync(path.join(__dirname,file),'utf8'),proof.replace(needle,'"0"')),{name:'AssertionError'});
});
