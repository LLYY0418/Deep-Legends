"use strict";
// Development acceptance evidence requested by R280 P13. Existing CI gates stay
// unchanged: record the original exit/log and measured values for review.
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
const out=path.resolve(process.argv[2]);fs.mkdirSync(out,{recursive:true});
const result={record_only:true,platform:process.platform,node:process.version};
try {
 const env={...process.env,CHROME_BIN:require('./r277-ci-orchestrate.cjs').chrome(process.env),R280_BROWSER_OUT:path.join(out,'browser')};
 const child=spawnSync(process.execPath,[path.join(__dirname,'r280-browser.cjs')],{env,encoding:'utf8',timeout:180000,maxBuffer:8*1024*1024});
 fs.writeFileSync(path.join(out,'measurement.log'),(child.stdout||'')+(child.stderr||''));
 Object.assign(result,{exit:child.status,error:child.error?.message});
} catch(error) {result.error=error.stack;}
fs.writeFileSync(path.join(out,'measurement.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));
