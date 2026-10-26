'use strict';
// Diagnostic installed copy only: the source package and original installation stay intact.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const asar=require('../desktop/node_modules/@electron/asar');
(async()=>{
 const root=path.resolve(process.argv[2]),archive=path.join(root,'resources','app.asar');
 const scratch=fs.mkdtempSync(path.join(os.tmpdir(),'r312-native-theme-'));
 try{
  asar.extractAll(archive,scratch);const file=path.join(scratch,'main.cjs');let source=fs.readFileSync(file,'utf8');
  assert.equal(source.split('let modalOpen = false;').length,2);assert.equal(source.split('      rendererTheme = theme;\n      syncTitleBar();').length,2);
  source=source.replace('let modalOpen = false;','if (process.platform === "win32") nativeTheme.themeSource = rendererTheme;\nlet modalOpen = false;').replace('      rendererTheme = theme;\n      syncTitleBar();','      rendererTheme = theme;\n      if (process.platform === "win32") nativeTheme.themeSource = rendererTheme;\n      syncTitleBar();');
  fs.writeFileSync(file,source);await asar.createPackageWithOptions(scratch,archive+'.candidate',{unpack:'backend/**'});fs.renameSync(archive+'.candidate',archive);
  // Keep the backend unpack path identical to the original installed copy.
  const unpacked=archive+'.candidate.unpacked';if(fs.existsSync(unpacked)){fs.cpSync(unpacked,archive+'.unpacked',{recursive:true});fs.rmSync(unpacked,{recursive:true});}
  console.log('R312 diagnostic copy: exact Windows bootstrap and theme IPC assignments installed');
 }finally{fs.rmSync(scratch,{recursive:true,force:true});}
})().catch(error=>{console.error(error);process.exitCode=1});
