'use strict';
// Re-run all fifteen guards and their metadata census as one complete job.
const {spawnSync}=require('node:child_process'),path=require('node:path'),assert=require('node:assert/strict');
assert(process.argv[2],'usage: node scripts/r276-api-census-run.cjs <out>');
const result=spawnSync(process.execPath,[path.join(__dirname,'r276-browser-job.cjs'),'all',path.resolve(process.argv[2])],{stdio:'inherit',env:{...process.env,CI:'true',GITHUB_ACTIONS:'true'}});
if(result.error)throw result.error;process.exitCode=result.status===null?1:result.status;
