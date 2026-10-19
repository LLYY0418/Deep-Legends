#!/usr/bin/env node
'use strict';
const fs=require('node:fs'),path=require('node:path');
function bumpVersion(version,date=new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai'}).format(new Date()),root=path.resolve(__dirname,'..')){
 if(!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version||''))throw Error('Expected version x.y.z');
 if(!/^\d{4}-\d{2}-\d{2}$/.test(date)||!Number.isFinite(Date.parse(date))||new Date(date).toISOString().slice(0,10)!==date)throw Error('Expected valid date YYYY-MM-DD');
 const files=['desktop/package.json','desktop/package-lock.json'];
 const values=files.map(file=>JSON.parse(fs.readFileSync(path.join(root,file),'utf8')));
 if(!values[1].packages?.[''])throw Error('package-lock root package is missing');
 let log=fs.readFileSync(path.join(root,'CHANGELOG.md'),'utf8');
 const existing=log.split('\n').filter(line=>new RegExp('^##\\s+'+version.replaceAll('.','\\.')+'(?:\\s|$)').test(line));
 if(existing.length){if(existing.length!==1||!new RegExp('^##\\s+'+version.replaceAll('.','\\.')+'\\s+[—·]\\s+\\d{4}-\\d{2}-\\d{2}\\s*$').test(existing[0]))throw Error('Existing version section format is invalid');}
 else {const at=log.search(/^##\s/m);if(at<0)throw Error('CHANGELOG has no version heading');log=log.slice(0,at)+`## ${version} — ${date}\n\n`+log.slice(at);}
 values[0].version=version;values[1].version=version;values[1].packages[''].version=version;
 for(let i=0;i<files.length;i++)fs.writeFileSync(path.join(root,files[i]),JSON.stringify(values[i],null,2)+'\n');
 fs.writeFileSync(path.join(root,'CHANGELOG.md'),log);return {version,date};
}
module.exports={bumpVersion};
if(require.main===module)try{if(process.argv.length<3||process.argv.length>4)throw Error('Usage: node scripts/bump-version.cjs <version> [date]');console.log(JSON.stringify(bumpVersion(...process.argv.slice(2))));}catch(e){console.error(e.message);process.exitCode=1;}
