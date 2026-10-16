"use strict";
const fs=require("node:fs"),path=require("node:path"),cp=require("node:child_process");
function inventory(root,ref="") {
  const git=args=>cp.execFileSync("git",args,{cwd:root,encoding:"utf8",maxBuffer:64*1024*1024});
  const files=git(ref?["ls-tree","-r","--name-only",ref]:["ls-files","-co","--exclude-standard"]).split(/\r?\n/).filter(Boolean);
  const cache=new Map();
  const read=file=>{if(!cache.has(file))cache.set(file,ref?git(["show",`${ref}:${file}`]):fs.readFileSync(path.join(root,file),"utf8"));return cache.get(file);};
  return {files:[...new Set(files)],read};
}
const lineAt=(source,offset)=>source.slice(0,offset).split("\n").length;
module.exports={inventory,lineAt};
