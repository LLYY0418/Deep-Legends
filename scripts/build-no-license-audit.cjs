"use strict";
// Exactly one default public package via the formal build script, in isolation.
const fs=require("node:fs"),path=require("node:path"),os=require("node:os"),{spawnSync}=require("node:child_process");
const root=path.resolve(__dirname,".."),output=process.env.R248_PACKAGE_DIR||path.join(root,"dist/R248-no-license-public");
if(fs.existsSync(output)&&fs.readdirSync(output).length)throw Error("R248 audit output must be empty; do not rebuild");
const snapshot=fs.mkdtempSync(path.join(os.tmpdir(),"r248-default-public-"));
try{
 for(const name of ["go.mod","go.sum","backend","installer","scripts","build-desktop.sh","build-desktop-windows.ps1","build-windows.ps1","desktop"])fs.cpSync(path.join(root,name),path.join(snapshot,name),{recursive:true,filter:file=>!file.includes(path.sep+"node_modules")&&!file.includes(path.sep+"payload"+path.sep+"files")&&!file.includes(path.sep+"_shots")&&!file.endsWith(path.sep+"loot-service.exe")&&!file.endsWith(path.sep+"backend-digest.cjs")&&!file.endsWith(path.sep+"uninstall-shell.exe")&&!/\.(?:pem|key)$/.test(file)&&!file.endsWith(path.sep+".dev.vars")});
 // Formal preflight reads the public README and CI source; local notes are optional.
 fs.copyFileSync(path.join(root,"README.md"),path.join(snapshot,"README.md"));
 if(fs.existsSync(path.join(root,"docs")))fs.cpSync(path.join(root,"docs"),path.join(snapshot,"docs"),{recursive:true,filter:file=>!file.split(path.sep).includes(".git")});
 fs.cpSync(path.join(root,".github"),path.join(snapshot,".github"),{recursive:true});
 fs.mkdirSync(path.join(snapshot,"installer/payload/files"),{recursive:true});
 fs.copyFileSync(path.join(root,"installer/payload/files/.gitkeep"),path.join(snapshot,"installer/payload/files/.gitkeep"));
 // All documentation fixtures, including the published public asset, are preflight inputs only; never packaged.
 fs.symlinkSync(path.join(root,"desktop/node_modules"),path.join(snapshot,"desktop/node_modules"),"dir");
 fs.mkdirSync(path.join(snapshot,"desktop/backend"),{recursive:true});
 fs.mkdirSync(path.join(snapshot,"dist/desktop"),{recursive:true});
 const version=require(path.join(root,"desktop/package.json")).version;
 const fixtureDirectory=process.env.DEEP_LEGENDS_UPDATE_FIXTURE_DIR||path.join(root,"output/update-fixtures/release-0.12.76");
 if(process.argv[2]==="--check-inputs") {
  for(const [cwd,args] of [[snapshot,["test","-count=1","-run","R248|R102SeedAccountsMatchVerificationDoc|SessionTokenPrivacyDocumentation","./backend"]],[path.join(snapshot,"installer"),["test","-count=1","-run","R248","./..."]]]) {
   const check=spawnSync("go",args,{cwd,env:{...process.env,GOFLAGS:"",DEEP_LEGENDS_UPDATE_FIXTURE_DIR:fixtureDirectory},stdio:"inherit"});if(check.error)throw check.error;if(check.status!==0)throw Error("Audit snapshot input check failed");
  }
  console.log("Audit snapshot preflight inputs verified; no package build");
 } else {
 const result=spawnSync("bash",[path.join(snapshot,"build-desktop.sh"),version],{cwd:snapshot,env:{...process.env,GOFLAGS:"",DEEP_LEGENDS_KEY_MODE:"public",DEEP_LEGENDS_LICENSE_BUILD:"0",DEEP_LEGENDS_UPDATE_FIXTURE_DIR:fixtureDirectory,SKIP_NPM_INSTALL:"1",KEEP_UNPACKED_FOR_AUDIT:"1",CSC_IDENTITY_AUTO_DISCOVERY:"false",CSC_LINK:"",CSC_KEY_PASSWORD:""},stdio:"inherit"});
 if(result.error)throw result.error;if(result.status!==0)throw Error("Formal public audit build failed: "+result.status);
 fs.mkdirSync(path.dirname(output),{recursive:true});if(fs.existsSync(output))fs.rmdirSync(output);
 fs.renameSync(path.join(snapshot,"dist/desktop"),output);
 console.log("R248 single default public audit package: "+output);
 }
}finally{fs.rmSync(snapshot,{recursive:true,force:true,maxRetries:5,retryDelay:100})}
