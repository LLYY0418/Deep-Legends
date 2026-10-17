'use strict';
const fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'..'),url='https://raw.communitydragon.org/latest/cdragon/arena/zh_cn.json';
function snapshot(data,overrides) {
 if(!Array.isArray(data.augments) || data.augments.length<200)throw Error('Invalid Chinese augment catalog');
 const byID=new Map(data.augments.map(row=>[row.id,row]));
 for(const row of overrides.augments)if(!byID.get(row.id)?.desc && !byID.get(row.id)?.tooltip)byID.set(row.id,row);
 const augments=[...byID.values()].sort((a,b)=>a.id-b.id);
 if(augments.some(row=>!(row.id>0) || !row.name || !(row.desc || row.tooltip)))throw Error('Incomplete Chinese snapshot');
 return {...data,augments};
}
async function main() {
 const data=process.argv[2]?JSON.parse(fs.readFileSync(process.argv[2],'utf8')):await (async()=>{const response=await fetch(url,{signal:AbortSignal.timeout(30000)});if(!response.ok)throw Error('CommunityDragon HTTP '+response.status);return response.json()})();
 const result=snapshot(data,JSON.parse(fs.readFileSync(path.join(root,'backend/data/augment_description_overrides_zh_cn.json'),'utf8')));
 const target=path.join(root,'backend/data/augment_descriptions_20261017.json');fs.writeFileSync(target,JSON.stringify(result)+'\n');
 console.log(JSON.stringify({source:url,rows:result.augments.length,target:path.relative(root,target)}));
}
module.exports={snapshot};if(require.main===module)main().catch(error=>{console.error(error.message);process.exitCode=1});
