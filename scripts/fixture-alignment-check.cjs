#!/usr/bin/env node
"use strict";
const fs=require("node:fs"),path=require("node:path");
const {inventory,lineAt}=require("./lib/source-inventory.cjs");
const {guards}=require("./r276-browser-catalog.cjs");
function analyze(source,entries=guards) {
  const {files,read}=source,set=new Set(files),startupEndpoints=[],automaticUI=[];
  // A conservative inventory of initial script assets, including conditional calls.
  // Static analysis cannot prove runtime reachability or replace the API census.
  const initial=new Set();
  for(const file of ["backend/web/index.html","backend/web/default/index.html"].filter(f=>set.has(f)))for(const m of read(file).matchAll(/<script\b[^>]*src=["']\/?([^"'?]+)["']/g))if(set.has("backend/web/"+m[1]))initial.add("backend/web/"+m[1]);
  for(const file of initial){const text=read(file);for(const m of text.matchAll(/["'`](\/api\/[\w/.-]+)/g))startupEndpoints.push({path:m[1],file,line:lineAt(text,m.index),reachability:"initial-script; conditional calls included"});for(const m of text.matchAll(/initAnnouncements\(|\.showModal\(|\bpopup\b/g))automaticUI.push({file,line:lineAt(text,m.index),evidence:text.split("\n")[lineAt(text,m.index)-1].trim()});}
  const closure=entry=>{
    const seen=new Set(),visit=file=>{if(seen.has(file)||!set.has(file))return;seen.add(file);const text=read(file);for(const m of text.matchAll(/["']([^"'\n]+\.cjs)["']/g)){
      const request=m[1],candidate=request.startsWith(".")?path.posix.normalize(path.posix.join(path.posix.dirname(file),request)):request;
      if(set.has(candidate))visit(candidate);else if(!request.includes("/")){const matches=files.filter(f=>path.posix.basename(f)===request);if(matches.length===1)visit(matches[0]);}
    }};visit(entry);return [...seen];
  };
  const rows=entries.map(([name,entry])=>{
    const dependencies=closure(entry),texts=dependencies.map(file=>({file,text:read(file)}));
    const page=texts.filter(row=>/Fetch\.requestPaused|Page\.navigate|createServer\(/.test(row.text));
    const routing=page.filter(row=>/Fetch\.continueRequest|createServer\(/.test(row.text));
    const issues=[];
    for(const {file,text} of routing){
      const shared=/\b(?:browserAPI|r264API|r265API|serveStaticAPI)\s*\(/.test(text);
      const staticShared=/createServer\(/.test(text)&&/index\.html/.test(text)&&/if\s*\(\s*serveStaticAPI\(req\s*,\s*res\)\s*\)\s*return/.test(text);
      const paused=text.indexOf('Fetch.requestPaused'),continued=text.indexOf('Fetch.continueRequest',Math.max(0,paused)),prefix=paused>=0?text.slice(paused,continued):'';
      const bound=/(?:const|let)\s+(\w+)\s*=\s*(?:browserAPI|r264API|r265API)\s*\(/.exec(prefix);
      const fulfilled=bound&&new RegExp('if\\s*\\(\\s*'+bound[1]+'\\s*\\)').test(prefix)&&prefix.includes('Fetch.fulfillRequest')&&prefix.includes('JSON.stringify('+bound[1]+')');
      const exactAnnouncement=/pathname\s*===\s*['"]\/api\/announcements['"][\s\S]*Fetch\.fulfillRequest[\s\S]*JSON\.stringify\(browserAPI\(/.test(prefix);
      if(continued>=0&&!staticShared&&!fulfilled&&!exactAnnouncement)issues.push({file,line:lineAt(text,continued),reason:"同源请求可直接放行，未证明共享夹具结果在此路由中被返回；无关调用不算覆盖"});
      if(!shared&&/createServer\(/.test(text)&&/index\.html/.test(text)&&/readFileSync\(/.test(text))issues.push({file,line:lineAt(text,text.indexOf("createServer(")),reason:"完整静态页面server未使用共享启动API夹具"});
    }
    return {name,file:entry,dependencies,sharedFixture:texts.some(row=>/\b(?:browserAPI|r264API|r265API|serveStaticAPI)\s*\(/.test(row.text)),routing:page.length?"page-or-delegate":"measurement-only",issues};
  });
  return {startupEndpoints,automaticUI,guards:rows,issues:rows.flatMap(row=>row.issues.map(issue=>({guard:row.name,...issue}))),limitations:"静态初始脚本端点是保守候选集合，包含条件调用；动态启动请求仍由真实浏览器API普查验证。只读报告不自动改夹具。"};
}
function main(args){let ref="",out="";for(let i=0;i<args.length;i++){if(args[i]==="--ref")ref=args[++i];else if(args[i]==="--json")out=args[++i];else throw Error("unknown argument "+args[i]);}const result=analyze(inventory(path.resolve(__dirname,".."),ref));console.log(JSON.stringify(result,null,2));if(out){fs.mkdirSync(path.dirname(path.resolve(out)),{recursive:true});fs.writeFileSync(out,JSON.stringify(result,null,2)+"\n");}return result;}
if(require.main===module){const result=main(process.argv.slice(2));process.exitCode=result.issues.length?1:0;}
module.exports={analyze};
