'use strict';
const policy=require('../release-validation-policy.json').local;
const defaultCosts=require('../renderer-costs.json');
const browserCosts=require('../preflight-browser-costs.json');
function makespan(rows,workers){
 const lanes=Array.from({length:Math.max(1,workers)},()=>0);
 for(const row of [...rows].sort((a,b)=>b.ms-a.ms)){const i=lanes.indexOf(Math.min(...lanes));lanes[i]+=row.ms;}
 return Math.max(0,...lanes);
}
function planExecution(selection,{costs=defaultCosts,workers=6,limits=policy,goNamesByFile=new Map(),full=false,browsers=false}={}){
 const changed=new Set(selection.changed.filter(c=>c.status!=='D').map(c=>c.file));
 const nodeRows=selection.nodeTests.map(row=>({file:row.file,ms:costs[row.file]??limits.unknownNodeFileMs,known:Object.hasOwn(costs,row.file)}));
 const totalCost=Object.values(costs).reduce((a,b)=>a+b,0),selectedCost=nodeRows.reduce((a,b)=>a+b.ms,0);
 const share=totalCost?selectedCost/totalCost:0;
 const directNode=selection.nodeTests.filter(row=>changed.has(row.file));
 const directGoFiles=selection.goTestFiles.filter(row=>changed.has(row.file));
 const requiredGoNames=[...new Set(directGoFiles.flatMap(row=>goNamesByFile.get(row.file)||[]))];
 const reasons=[];
 if(nodeRows.some(row=>!row.known&&!changed.has(row.file)))reasons.push("associated Node cost is unknown; defer rather than promise a short run");
 if(selection.broad)reasons.push('selected Node file ratio exceeds the existing broad scope threshold');
 if(share>limits.maxAssociatedNodeCostShare)reasons.push('associated Node tests exceed the measured cost share');
 const nodeMs=makespan(nodeRows,workers);
 if(nodeMs>limits.targetMs)reasons.push('estimated Node worker time exceeds the local feedback target');
 if(selection.goTestNames.length>limits.maxUnprofiledGoCases)reasons.push('Go race scope has no measured cost profile and exceeds the local case allowance');
 const deferAssociated=!full&&reasons.length>0;
 const selectedBrowser=selection.browserGuards.map(row=>({name:row.name,ms:browserCosts.guards[row.name]??null}));
 const unknownBrowser=selectedBrowser.filter(row=>row.ms===null).map(row=>row.name);
 const browserMs=unknownBrowser.length?null:selectedBrowser.reduce((a,b)=>a+b.ms,0);
 const deferBrowser=browsers&&!full&&(deferAssociated||browserMs===null||browserMs>limits.targetMs);
 return {fullOverride:full,estimate:{nodeWorkerMs:nodeMs,nodeFileCostShare:share,unknownNodeFiles:nodeRows.filter(row=>!row.known).map(row=>row.file),goCases:selection.goTestNames.length,goDurationMs:null,browserSerialMs:browserMs,unknownBrowser,profile:browserCosts.source,warning:'prediction only; worker contention, cold builds and runner speed affect actual duration'},
  reasons,deferAssociated,deferBrowser,
  nodeTests:deferAssociated?directNode:selection.nodeTests,
  goTestNames:deferAssociated?requiredGoNames:selection.goTestNames,
  deferred:{nodeTests:deferAssociated?selection.nodeTests.filter(row=>!changed.has(row.file)):[],goTestNames:deferAssociated?selection.goTestNames.filter(name=>!requiredGoNames.includes(name)):[],browserGuards:deferBrowser?selection.browserGuards:[]},
  requiredDirect:{nodeFiles:directNode.map(row=>row.file),goFiles:directGoFiles.map(row=>row.file)}};
}
module.exports={makespan,planExecution};
