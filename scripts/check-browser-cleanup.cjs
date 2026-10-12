'use strict';
// Conservative test-entry audit: process/profile cleanup belongs to the shared
// awaited helper, rather than attempting to infer ordering through arbitrary code.
const fs=require('node:fs'),path=require('node:path');
function mask(source){return source.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*|'(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*"|`(?:\\.|[^`\\])*`/g,text=>text.replace(/[^\n]/g,' '));}
function inspect(source,file){
 const code=mask(source),issues=[];
 const add=(index,message)=>issues.push({file,line:source.slice(0,index).split('\n').length,message});
 for(const m of code.matchAll(/\bsetTimeout\s*\(/g)){
  let depth=1,end=m.index+m[0].length;
  for(;end<code.length&&depth;end++){if(code[end]==='(')depth++;if(code[end]===')')depth--;}
  if(/\brmSync\s*\(/.test(code.slice(m.index,end)))add(m.index,'directory removal in an unowned timer; use awaited cleanupChromeTest');
 }
 if(/\bawait\s+launchChrome\s*\(/.test(code)){
  if(!/chrome-test-cleanup\.cjs/.test(source)||!(/\bawait\s+cleanupChromeTest\s*\(/.test(code)||/\.finally\s*\(\s*\([^)]*\)\s*=>\s*cleanupChromeTest\s*\(/.test(code)))add(0,'browser owner must await or return shared cleanup');
  for(const m of code.matchAll(/\.kill\s*\(/g))add(m.index,'direct kill in browser owner bypasses exit barrier');
  for(const m of code.matchAll(/\brmSync\s*\(/g)){
   // Existing guards clear only their previous JSON output before launch.
   if(!/^\s*path\.join\s*\(\s*out\s*,\s*file\s*\)/.test(code.slice(m.index+m[0].length)))add(m.index,'direct profile removal bypasses exit barrier');
  }
 }
 // Also catches newly introduced guard scripts that do not use our launcher.
 if(/\.kill\s*\(/.test(code)&&/\brmSync\s*\(/.test(code))add(code.search(/\.kill\s*\(/),'kill and direct removal in one guard; use the shared exit barrier');
 for(const m of code.matchAll(/\.finally\s*\(\s*(?:async\s*)?\([^)]*\)\s*=>\s*\{[^}]*?cleanupChromeTest\s*\(/g)){
  if(!/\b(?:return|await)\s+cleanupChromeTest/.test(m[0]))add(m.index,'finally does not await or return cleanup promise');
 }
 return issues;
}
function audit(root){
 const files=[],issues=[];
 function visit(directory){for(const entry of fs.readdirSync(path.join(root,directory),{withFileTypes:true})){
  const name=entry.name,file=directory+'/'+name;
  if(entry.isDirectory()) {if(!['node_modules','dist','output','build'].includes(name)&&!name.startsWith('.'))visit(file);continue;}
  if(!entry.isFile()||!name.endsWith('.cjs')||name.endsWith('.test.cjs'))continue;
  const source=fs.readFileSync(path.join(root,file),'utf8');
  // Scan every entry, including future guards, while omitting the implementation.
  if(name==='chrome-test-cleanup.cjs'||name==='check-browser-cleanup.cjs')continue;
  files.push(file);issues.push(...inspect(source,file));
 }}
 for(const directory of ['desktop','scripts'])visit(directory);
 const helper=fs.readFileSync(path.join(root,'scripts/chrome-test-cleanup.cjs'),'utf8');
 if(/\bprocess\s*\.\s*(?:exitCode|exit)\b/.test(mask(helper)))issues.push({file:'scripts/chrome-test-cleanup.cjs',line:1,message:'cleanup must not change the main flow exit status'});
 return {files:files.length,issues};
}
if(require.main===module){
 const result=audit(path.resolve(__dirname,'..')),output=process.argv.find(x=>x.startsWith('--json='))?.slice(7);
 if(output){fs.mkdirSync(path.dirname(output),{recursive:true});fs.writeFileSync(output,JSON.stringify(result,null,2)+'\n');}
 for(const issue of result.issues)console.error(`${issue.file}:${issue.line}: ${issue.message}`);
 console.log(`browser cleanup audit: files=${result.files} unsafe=${result.issues.length}`);
 if(result.issues.length)process.exitCode=1;
}
module.exports={inspect,audit};
