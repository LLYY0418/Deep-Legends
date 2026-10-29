'use strict';
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
function verifyRuntimeDependencies(file,{run=spawnSync,executable}={}){
 if(!executable){
  const vswhere=path.join(process.env['ProgramFiles(x86)']||'C:\\Program Files (x86)','Microsoft Visual Studio/Installer/vswhere.exe');
  const found=run(vswhere,['-latest','-products','*','-requires','Microsoft.VisualStudio.Component.VC.Tools.x86.x64','-property','installationPath'],{encoding:'utf8'});
  if(found.error||found.status!==0)throw Error('Cannot locate MSVC dependency inspector');
  const tools=path.join(found.stdout.trim(),'VC/Tools/MSVC');
  executable=fs.readdirSync(tools).sort().reverse().map(version=>path.join(tools,version,'bin/Hostx64/x64/dumpbin.exe')).find(file=>fs.existsSync(file));
  if(!executable)throw Error('MSVC dumpbin unavailable');
 }
 const result=run(executable,['/nologo','/dependents',file],{encoding:'utf8'});
 if(result.error||result.status!==0)throw Error('Native resize dependency inspection failed');
 const dependencies=[...String(result.stdout).matchAll(/^\s+([\w.-]+\.dll)\s*$/gim)].map(row=>row[1]);
 if(!dependencies.length)throw Error('Native resize dependency inspection returned no DLLs');
 if(dependencies.some(name=>/^(?:vcruntime|msvcp)140/i.test(name)))throw Error('Native resize still depends on the VC++ runtime');
 console.log('resize_border_static_runtime '+JSON.stringify(dependencies));return dependencies;
}
function build({platform=process.platform,run=spawnSync}={}){
 if(platform!=='win32')throw Error('Windows native resize border requires a Windows build runner; use build-desktop-windows.ps1');
 const directory=path.join(__dirname,'native/resize-border'),target=require('electron/package.json').version;
 const result=run(process.execPath,[require.resolve('node-gyp/bin/node-gyp.js'),'rebuild','--directory',directory,'--arch=x64','--target='+target,'--dist-url=https://electronjs.org/headers'],{stdio:'inherit'});
 if(result.error)throw result.error;if(result.status!==0)throw Error('Native resize-border build failed: '+result.status);
 fs.copyFileSync(path.join(directory,'build/Release/resize_border.node'),path.join(__dirname,'native/resize-border.node'));
 verifyRuntimeDependencies(path.join(__dirname,'native/resize-border.node'),{run});
}
if(require.main===module)build();
module.exports={build,verifyRuntimeDependencies};
