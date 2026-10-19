'use strict';
// Conservative source checks: findings request human review; they never authorize a release.
function mask(source){return source.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`/g,m=>m.replace(/[^\n]/g,' '));}
function tests(source){
 const code=mask(source),rows=[];
 for(const m of code.matchAll(/\bfunc\s+(Test\w+)\s*\(/g))rows.push({id:m[1],line:code.slice(0,m.index).split('\n').length});
 for(const m of code.matchAll(/\b(t\.Run|test|it)\s*\(/g)){const tail=source.slice(m.index+m[0].length),name=tail.match(/^\s*(["'`])((?:\\.|(?!\1)[^\\])*?)\1/);rows.push({id:m[1]+':'+(name?.[2]||'<dynamic>'),line:code.slice(0,m.index).split('\n').length});}
 return rows;
}
function skips(source){const code=mask(source);return [...code.matchAll(/\bt\.Skip(?:f|Now)?\s*\(|\.skip\s*\(|[{,]\s*skip\s*:/g)].map(m=>({id:m[0].replace(/\s/g,''),line:code.slice(0,m.index).split('\n').length}));}
function missing(before,after){const counts=new Map();for(const r of after)counts.set(r.id,(counts.get(r.id)||0)+1);return before.filter(r=>{const n=counts.get(r.id)||0;if(n){counts.set(r.id,n-1);return false;}return true;});}
function thresholds(source){const rows=[];const code=mask(source);for(const [i,line]of code.split('\n').entries())for(const m of line.matchAll(/(?:<=?|>=?)\s*(\d+(?:\.\d+)?)/g)){rows.push({key:line.slice(0,m.index).trim().replace(/\s+/g,' ')+m[0].replace(m[1],'#'),value:Number(m[1]),line:i+1});}
 for(const [i,line]of code.split('\n').entries())if(/r287(?:TooSlow|Measure|WaitSelect|NewDeadline|NewClockDeadline)\(/.test(line))for(const m of line.matchAll(/(\d+(?:\.\d+)?)\s*\*\s*time\.(?:Nanosecond|Microsecond|Millisecond|Second|Minute)/g))rows.push({key:line.replace(/\d+(?:\.\d+)?/g,'#').trim().replace(/\s+/g,' '),value:Number(m[1]),line:i+1});
 return rows;}
const budgetFile=file=>/^backend\/(?:season_stats_budget|riot_page_budget|binary_disk_budget)\.go$/.test(file)||/(?:^|\/)(?:renderer-budgets\.cjs|renderer-budget-gate\.cjs|renderer-budget[^/]*\.json|[^/]*budget[^/]*\.json)$/.test(file)||/^backend\/web\/r277-deferred-assets\.test\.cjs$/.test(file);
function scanWeakening(changes){
 const hits=[];
 for(const {file,before='',after=''}of changes){
  const add=(kind,line,detail)=>hits.push({kind,file,line,detail});
  if(before===after)continue;
  if(budgetFile(file))add('product-budget',1,'product budget source changed');
  if(file==='scripts/test-hygiene-baseline.json'){
   const old=JSON.parse(before||'{"files":{}}').files||{},next=JSON.parse(after||'{"files":{}}').files||{};
   for(const [name]of Object.entries(next))if(!Object.hasOwn(old,name))add('hygiene-increase',1,`${name}: new baseline entry`);
   for(const [name,row]of Object.entries(next))for(const [key,value]of Object.entries(row))if(typeof value==='number'&&value>(old[name]?.[key]||0))add('hygiene-increase',1,`${name} ${key}: ${old[name]?.[key]||0} -> ${value}`);
  }
  if(!/(?:_test\.go|\.test\.[cm]?js|-browser\.cjs|-checks\.cjs|-layout\.cjs)$/.test(file))continue;
  const assertionCount=source=>(mask(source).match(/\bassert[\w.]*\s*\(|\bt\.(?:Fatalf?|Errorf?|FailNow|Fail)\s*\(/g)||[]).length;
  if(assertionCount(after)<assertionCount(before))add('removed-assertion',1,`assertion count ${assertionCount(before)} -> ${assertionCount(after)}`);
  for(const row of missing(tests(before),tests(after)))add('deleted-test',row.line,row.id);
  for(const row of missing(skips(after),skips(before)))add('added-skip',row.line,row.id);
  const previous=new Map();for(const row of thresholds(before)){if(!previous.has(row.key))previous.set(row.key,[]);previous.get(row.key).push(row.value);}
  for(const row of thresholds(after)){const values=previous.get(row.key);if(values?.length){const old=values.shift();if(row.value>old)add('increased-threshold',row.line,`${row.key}: ${old} -> ${row.value}`);}}
 }
 return hits;
}
module.exports={scanWeakening,tests,skips,thresholds,budgetFile};
