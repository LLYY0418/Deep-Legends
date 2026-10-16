'use strict';
const cp=require('node:child_process'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'..');
function stagingIssues({changed,staged,unstaged}){
 const s=new Set(staged),u=new Set(unstaged);
 return changed.filter(file=>!file.endsWith('.md')&&(!s.has(file)||u.has(file))).map(file=>`${file}: 请显式暂存该任务文件的最终内容；预检不会自动git add`);
}
function check(){
 const list=args=>cp.execFileSync('git',args,{cwd:root,encoding:'utf8'}).split('\0').filter(Boolean);
 const staged=list(['diff','--cached','--name-only','-z']),unstaged=list(['diff','--name-only','-z']),fresh=list(['ls-files','--others','--exclude-standard','-z']);
 return stagingIssues({changed:[...new Set([...staged,...unstaged,...fresh])],staged,unstaged});
}
module.exports={stagingIssues,check};
if(require.main===module){const errors=check();for(const e of errors)console.error(e);process.exitCode=errors.length?1:0;}
