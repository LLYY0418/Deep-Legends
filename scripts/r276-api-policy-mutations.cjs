'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{requireClean,failures}=require('./r276-api-census.cjs'),policy=require('./r276-api-fault-policy.cjs');
const out=path.resolve(process.argv[2]||''),root=path.resolve(__dirname,'..');assert(process.argv[2],'usage: node scripts/r276-api-policy-mutations.cjs <out>');fs.mkdirSync(out,{recursive:true});
const baseline={guard:'r100',observations:[...Array.from({length:3},()=>({source:'network',method:'GET',path:'/api/status',status:503}))]},rows=[];
function run(name,work,expected){let error;try{work()}catch(e){error=e;}const passed=expected?!!error:!error;rows.push({name,result:expected?'FAIL (expected)':'PASS',assertionVerified:passed,message:error?.message});fs.writeFileSync(path.join(out,name+'.log'),(error?.stack||'PASS')+'\n');fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({fixture:'Synthetic census metadata; full real browser census is archived separately',rows},null,2)+'\n');console.log(JSON.stringify(rows.at(-1)));assert(passed,name);}
run('baseline',()=>requireClean(baseline,'r100'));
for(const name of ['wrong-path','wrong-status','one-extra-injection','wrong-method','wrong-guard','unknown-source']){
 const data=structuredClone(baseline);for(const row of data.observations)if(row.status===503){if(name==='wrong-path')row.path='/api/other-status';if(name==='wrong-status')row.status=502;if(name==='wrong-method')row.method='POST';if(name==='unknown-source')row.source='invented-source';}
 if(name==='one-extra-injection')data.observations.push({...data.observations[0]});run(name,()=>requireClean(data,name==='wrong-guard'?'r264':'r100'),true);
}
run('business-assertion-deleted',()=>{const selected=policy.entries[0].assertions[0];policy.classify('r100',baseline,failures(baseline),file=>{const source=fs.readFileSync(path.join(root,file),'utf8');return file===selected.file?source.replace(selected.snippet,'/* removed business assertion */'):source;});},true);
for(const [name,row,expected] of [
 ['non-canceled-network-error',{status:null,failed:true,canceled:false,transportReason:'OTHER'},true],
 ['http-404',{status:404},true],['http-502',{status:502},true],
 ['normal-cancel',{status:null,failed:true,canceled:true,transportReason:'net::ERR_ABORTED'},false],
 ['cancel-after-http-status',{status:502,failed:true,canceled:true,transportReason:'net::ERR_ABORTED'},false]
])run(name,()=>requireClean({guard:'r100',observations:[{source:'network',method:'GET',path:'/api/champion-asset',...row}]},'r100'),expected);
run('restored',()=>requireClean(baseline,'r100'));
