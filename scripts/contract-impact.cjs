#!/usr/bin/env node
"use strict";
const path=require("node:path"),fs=require("node:fs");
const {inventory}=require("./lib/source-inventory.cjs");
const testFile=file=>/\.test\.[cm]js$|_test\.go$|(?:browser|layout|persisted-ui)[^/]*\.cjs$/.test(file);
const assertion=text=>/\bassert(?:\.[\w]+)*\s*\(|\bt\.(?:Fatalf?|Errorf?)\s*\(/.test(text);
function identifiersIn(source) {
  const tokens=new Set();
  for(const m of source.matchAll(/(?:lol-loot-|deep-legends-)[\w-]+|\/api\/[\w/.-]+|#[a-z][\w-]+|\b(?:search-server-id|search-region-manual)\b/g))tokens.add(m[0]);
  for(const m of source.matchAll(/getElementById\(\s*['"]([^'"]+)['"]/g))tokens.add(`#${m[1]}`);
  for(const m of source.matchAll(/querySelector(?:All)?\(\s*['"]([^'"]+)['"]/g))tokens.add(m[1]);
  for(const m of source.matchAll(/['"]([^'"\n]*[\u3400-\u9fff][^'"\n]*)['"]/g))if(m[1].length>=2&&m[1].length<=160&&!/[<>]|\$\{/.test(m[1]))tokens.add(m[1]);
  return [...tokens].sort();
}
function query({files,read},identifiers) {
  identifiers=[...new Set(identifiers.filter(Boolean))];
  const results=[];
  for(const file of files.filter(testFile).sort()) {
    const lines=read(file).split(/\r?\n/),references=[],assertions=[];
    lines.forEach((text,index)=>{
      const hits=identifiers.filter(token=>text.includes(token)||token.startsWith("#")&&text.includes(token.slice(1)));
      const row={line:index+1,text,assertion:assertion(text)};
      if(hits.length)references.push({...row,identifiers:hits});
      if(row.assertion)assertions.push(row);
    });
    if(references.length)results.push({file,references,assertions});
  }
  return {identifiers,files:results,note:"references为直接引用；assertions为该引用文件内的断言，未声称每条断言直接引用标识。"};
}
function main(args) {
  const root=path.resolve(__dirname,".."),tokens=[];let ref="",out="";
  for(let i=0;i<args.length;i++){if(args[i]==="--ref")ref=args[++i];else if(args[i]==="--json")out=args[++i];else if(args[i]==="--token")tokens.push(args[++i]);else tokens.push(args[i]);}
  if(!tokens.length)throw Error("用法：contract-impact.cjs <标识...> [--ref SHA] [--json file]");
  const result=query(inventory(root,ref),tokens);
  for(const row of result.files){for(const hit of row.references)console.log(`${row.file}:${hit.line}${hit.assertion?" [assert]":""} ${hit.text.trim()}`);for(const hit of row.assertions.filter(a=>!row.references.some(r=>r.line===a.line)))console.log(`${row.file}:${hit.line} [file assertion] ${hit.text.trim()}`);}
  if(out){fs.mkdirSync(path.dirname(path.resolve(out)),{recursive:true});fs.writeFileSync(out,JSON.stringify(result,null,2)+"\n");}
  return result;
}
if(require.main===module)main(process.argv.slice(2));
module.exports={query,identifiersIn,testFile};
