'use strict';
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),cp=require('node:child_process');
const out=fs.mkdtempSync(path.join(os.tmpdir(),'r287-preflight-build-'));
try{const result=cp.spawnSync('go',['build','-tags=','-buildvcs=false','-o',path.join(out,'backend.exe'),'./backend'],{stdio:'inherit',env:{...process.env,GOOS:'windows',GOARCH:'amd64',CGO_ENABLED:'0'}});if(result.error)throw result.error;process.exitCode=result.status??1;}finally{fs.rmSync(out,{recursive:true,force:true});}
