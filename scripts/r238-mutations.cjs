"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
const fs=require("node:fs"),path=require("node:path"),os=require("node:os"),assert=require("node:assert/strict"),{spawnSync}=require("node:child_process");
const root=path.resolve(__dirname,".."),out=process.env.R238_MUTATION_OUT||evidencePath('r238/mutations');
fs.mkdirSync(out,{recursive:true});
const temp=fs.mkdtempSync(path.join(os.tmpdir(),"r238-scale-mutants-")),results=[];
try {
  for(const name of ["always-reset","no-migration"]) {
    const copy=path.join(temp,name);
    fs.cpSync(path.join(root,"desktop"),path.join(copy,"desktop"),{recursive:true,filter:p=>!p.includes(path.sep+"node_modules")&&!p.includes(path.sep+"assets")&&!p.endsWith(".exe")});
    fs.symlinkSync(path.join(root,"desktop/node_modules"),path.join(copy,"desktop/node_modules"),"dir");
    fs.cpSync(path.join(root,"backend/web"),path.join(copy,"backend/web"),{recursive:true});
    for(const [file,from,to] of [
      ["desktop/main.cjs","if (stored.defaultAuto === 1)",name==="always-reset"?"if (false)":"if (true)"],
      ["backend/web/app.js",'if (!window.desktopScale && preference("ui-scale-default-auto", "") !== "1")',name==="always-reset"?'if (!window.desktopScale)':'if (false)'],
    ]) {
      const location=path.join(copy,file),source=fs.readFileSync(location,"utf8");
      assert.ok(source.includes(from),"mutation target missing: "+file);fs.writeFileSync(location,source.replace(from,to));
    }
    const start=Date.now(),p=spawnSync(process.execPath,["--test","desktop/ui-scale.test.cjs","desktop/ui-scale-render.test.cjs"],{cwd:copy,encoding:"utf8",timeout:30000}),log=p.stdout+p.stderr;
    fs.writeFileSync(path.join(out,name+".log"),log);
    assert.equal(p.status,1,name+" must fail assertions");
    assert.match(log,/ERR_ASSERTION|AssertionError/);
    assert.match(log,/R238 old fixed scale migrates once, then preserves the user's fixed choice/);
    assert.match(log,/R238 browser fixed preference resets once and subsequent fixed choices survive/);
    assert.doesNotMatch(log,/SyntaxError|Cannot find module/);
    results.push({name,exit_code:p.status,elapsed_ms:Date.now()-start,assertion_failed:true});
  }
  fs.writeFileSync(path.join(out,"results.json"),JSON.stringify(results,null,2));console.log(JSON.stringify(results));
} finally {fs.rmSync(temp,{recursive:true,force:true,maxRetries:5,retryDelay:100});}
