'use strict';
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
function build({platform=process.platform,run=spawnSync}={}){
 if(platform!=='win32')throw Error('Windows native resize border requires a Windows build runner; use build-desktop-windows.ps1');
 const directory=path.join(__dirname,'native/resize-border'),target=require('electron/package.json').version;
 const result=run(process.execPath,[require.resolve('node-gyp/bin/node-gyp.js'),'rebuild','--directory',directory,'--arch=x64','--target='+target,'--dist-url=https://electronjs.org/headers'],{stdio:'inherit'});
 if(result.error)throw result.error;if(result.status!==0)throw Error('Native resize-border build failed: '+result.status);
 fs.copyFileSync(path.join(directory,'build/Release/resize_border.node'),path.join(__dirname,'native/resize-border.node'));
}
if(require.main===module)build();
module.exports={build};
