"use strict";
// Explicit manual suite, deliberately outside default Node test discovery/CI.
const fs=require("node:fs"),path=require("node:path"),{spawnSync}=require("node:child_process");
const root=path.resolve(__dirname,".."),files=["backend/web","desktop"].flatMap(dir=>fs.readdirSync(path.join(root,dir)).filter(n=>n.endsWith(".license.cjs")).map(n=>path.join(root,dir,n)));
const result=spawnSync(process.execPath,["--test",...files],{cwd:root,env:{...process.env,DEEP_LEGENDS_TEST_LICENSE:"1"},stdio:"inherit"});
if(result.error)throw result.error;process.exitCode=result.status??1;
