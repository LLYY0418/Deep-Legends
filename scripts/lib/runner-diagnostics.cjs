'use strict';
const os=require('node:os');
function runnerMetadata(){return {cpu_model:os.cpus()[0]?.model||'unknown',logical_cpus:os.cpus().length,runner_image:process.env.ImageOS||process.env.RUNNER_OS||process.platform,image_version:process.env.ImageVersion||null,node:process.version};}
function timingComparison(files,reference){
 const ratios=files.filter(f=>f.duration_ms>0&&reference[f.file]>0).map(f=>({file:f.file,ratio:f.duration_ms/reference[f.file]}));
 if(!ratios.length)return {kind:'no-reference',ratios:[]};
 const sorted=ratios.map(r=>r.ratio).sort((a,b)=>a-b),middle=Math.floor(sorted.length/2),median=sorted.length%2?sorted[middle]:(sorted[middle-1]+sorted[middle])/2;
 const share=ratios.filter(r=>Math.abs(r.ratio/median-1)<=.2).length/ratios.length;
 const outliers=ratios.filter(r=>r.ratio>=median*2);
 return {kind:median>1.2&&share>=.8&&!outliers.length?'uniform-slowdown':outliers.length?'file-outliers':'mixed',medianRatio:median,uniformShare:share,ratios,outliers,note:'描述耗时分布，不推断根因；不同负载/参考集合不可直接归因'};
}
module.exports={runnerMetadata,timingComparison};
// Legacy jobs already print renderer JSON. Read those bytes without editing the pinned job.
function timingReportsFromLog(text){
 const reports=[];
 for(const match of text.matchAll(/"scope"\s*:\s*"(?:all|desktop)"/g)){
  const start=text.lastIndexOf('{',match.index);if(start<0)continue;
  // GitHub treats workflow commands inside test names as annotations and can
  // strip the JSON line prefix in `cases`. Runner/file metadata precedes that
  // untrusted section; decode its complete prefix without inventing case data.
  const casesAt=text.slice(start).search(/\n\s*"cases"\s*:/);
  if(casesAt>=0){try{const header=JSON.parse(text.slice(start,start+casesAt)+'\n"cases":[]}');if(header.runner&&header.timingComparison){reports.push({...header,logMetadataOnly:true});continue;}}catch{}}
  let depth=0,string=false,escaped=false;
  for(let i=start;i<text.length;i++){
   const c=text[i];if(string){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c==='"')string=false;continue;}
   if(c==='"')string=true;else if(c==='{')depth++;else if(c==='}'&&--depth===0){try{const data=JSON.parse(text.slice(start,i+1));if(data.runner&&data.timingComparison)reports.push(data);}catch{}break;}
  }
 }
 return reports;
}
module.exports.timingReportsFromLog=timingReportsFromLog;
