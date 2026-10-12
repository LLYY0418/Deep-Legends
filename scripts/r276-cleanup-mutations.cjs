'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const out=path.resolve(process.argv[2]||path.join(os.tmpdir(),'r276-cleanup-mutations'));fs.mkdirSync(out,{recursive:true});
const original=fs.readFileSync(path.join(__dirname,'chrome-test-cleanup.cjs'),'utf8'),line='const stopped = await Promise.all(children.map(child => stopChild(child, options)));';assert(original.includes(line));
const variants=[
 ['no-exit-wait',original.replace(line,'const stopped = children.map(child => { child?.kill(); return true; });'),'profile deletion began before child exit'],
 ['immediate-removal',original.replace(line,'for (const child of children) child?.kill(); await removeDirectories(directories, options);\n  '+line),'profile deletion began before child exit'],
 ['masked-main-failure',original.replace('const warn = options.warn || warning;','process.exitCode = 0;\n const warn = options.warn || warning;'),'cleanup masked main guard failure']
];
const rows=[];
for(const [name,source,signature] of variants){
 const file=path.join(out,name+'.cjs');fs.writeFileSync(file,source);
 const r=spawnSync(process.execPath,['--test',path.join(__dirname,'chrome-test-cleanup.test.cjs')],{env:{...process.env,R276_CLEANUP_HELPER:file},encoding:'utf8'});
 const log=(r.stdout||'')+(r.stderr||'');fs.writeFileSync(path.join(out,name+'-FAIL.log'),log);assert.notEqual(r.status,0,name+' survived');assert(log.includes(signature),name+' failed without required signature');rows.push({name,exitCode:r.status,expected:'FAIL',signature});
}
const fixed=spawnSync(process.execPath,['--test',path.join(__dirname,'chrome-test-cleanup.test.cjs'),path.join(__dirname,'check-browser-cleanup.test.cjs')],{encoding:'utf8'});
fs.writeFileSync(path.join(out,'restored-PASS.log'),(fixed.stdout||'')+(fixed.stderr||''));assert.equal(fixed.status,0,'restored cleanup failed');rows.push({name:'restored',exitCode:fixed.status,expected:'PASS'});
fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2)+'\n');console.log(JSON.stringify(rows));
