'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const {createDiagnosticsDirectoryController:create}=require('./diagnostics-directory.cjs');
const {createShareExportController}=require('./share-export.cjs');
function fixture(t){
 const root=fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(),'r110-export-')));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const dirs=['userData','downloads','pictures','chosen'].map(n=>path.join(root,n));dirs.forEach(d=>fs.mkdirSync(d));
 const [userData,downloads,pictures,chosen]=dirs,sender={id:1},event={sender},changes=[];let canceled=false,prompts=0;
 const args={app:{getPath:key=>({userData,downloads,pictures})[key]},dialog:{showOpenDialog:async()=>{prompts++;return {canceled,filePaths:[chosen]}}},fileSystem:fs,isTrustedRenderer:x=>x===sender,getMainWindow:()=>({webContents:sender}),onChanged:x=>changes.push(x)};
 const share=directoryController=>createShareExportController({...args,BrowserWindow:{},directoryController,randomToken:()=> 'a'.repeat(24)});
 return {...args,userData,downloads,pictures,chosen,event,changes,share,create:()=>create(args),get prompts(){return prompts},set canceled(v){canceled=v}};
}
test('R110 either export chooses the same remembered folder and updates settings immediately',async t=>{
 const f=fixture(t),directory=f.create(),share=f.share(directory);
 directory.rememberFile(path.join(f.downloads,'first.jsonl'));
 assert.equal(share.getSaveDirectory(f.event).directory,f.downloads);
 assert.equal((await share.prepareSave(f.event,'image')).directory,f.downloads);assert.equal(f.prompts,0);
 await share.chooseSaveDirectory(f.event);assert.equal(directory.getDirectory(),f.chosen);assert.equal(f.create().getDirectory(),f.chosen);assert.equal(f.changes.at(-1).directory,f.chosen);
 f.canceled=true;await directory.chooseSaveDirectory(f.event);assert.equal(directory.getDirectory(),f.chosen);
 assert.equal(fs.existsSync(path.join(f.userData,'share-export.json')),false);
});
test('R110 first share export updates diagnostic destination without another picker',async t=>{
 const f=fixture(t),directory=f.create(),share=f.share(directory);
 const result=await share.prepareSave(f.event,'image');assert.equal(result.prompted,true);assert.equal(f.prompts,1);
 assert.equal(directory.getDirectory(),f.chosen);assert.equal(f.changes.at(-1).directory,f.chosen);
});
test('R110 migrates most recent valid old choice once and does not resurrect cleared settings',t=>{
 const f=fixture(t),stat=fs.statSync(f.downloads);
 const old=path.join(f.userData,'diagnostics-export.json');fs.writeFileSync(old,JSON.stringify({diagnosticsSaveDirectory:{path:f.downloads,dev:stat.dev,ino:stat.ino}}));fs.utimesSync(old,1,1);
 fs.writeFileSync(path.join(f.userData,'share-export.json'),JSON.stringify({saveDirectory:f.pictures}));
 assert.equal(f.create().getDirectory(),f.pictures);
 fs.rmSync(f.pictures,{recursive:true});assert.equal(f.create().getDirectory(),'');assert.equal(f.create().getDirectory(),'');
});
test('R110 settings exposes a single export directory',()=>{
 const html=fs.readFileSync(path.join(__dirname,'../web/index.html'),'utf8');
 assert.equal((html.match(/id="setting-share-directory"/g)||[]).length,1);
 assert.equal(html.includes('id="setting-diagnostics-directory"'),false);
});
test('R110 shared PNG destination revalidates replaced folders and preserves PNG names',async t=>{
 const f=fixture(t),directory=f.create();directory.rememberFile(path.join(f.downloads,'original.jsonl'));
 const stage=directory.prepareFile(path.join(f.downloads,'share.png'));fs.writeFileSync(stage,'png');
 fs.renameSync(f.downloads,f.downloads+'-old');fs.mkdirSync(f.downloads);
 const file=await directory.finalizeFile(stage);
 assert.equal(path.dirname(file),f.chosen);assert.equal(path.basename(file),'share.png');assert.equal(fs.readFileSync(file,'utf8'),'png');assert.equal(fs.readdirSync(f.downloads).length,0);
});
