"use strict";
const path = require("node:path");

function createDiagnosticsDirectoryController({app,dialog,fileSystem:fs,isTrustedRenderer,getMainWindow,onChanged}) {
  const settings=path.join(app.getPath("userData"),"export-directory.json");
  let saved=null;const staged=new Map();
  let canonicalExists=false;
  try {fs.statSync(settings);canonicalExists=true;saved=JSON.parse(fs.readFileSync(settings,"utf8")).saveDirectory || null;}catch(_){}
  function inspect(directory) {
    if(typeof directory!=="string"||!path.isAbsolute(directory))throw Error("invalid directory");
    const canonical=fs.realpathSync(directory), stat=fs.lstatSync(directory);
    if(canonical!==path.resolve(directory)||!stat.isDirectory()||stat.isSymbolicLink())throw Error("untrusted directory");
    return {path:canonical,dev:stat.dev,ino:stat.ino};
  }
  function valid(record) {try {const now=inspect(record?.path);return now.dev===record.dev&&now.ino===record.ino;}catch(_){return false;}}
  function persist(record) {
    saved=record;
    fs.mkdirSync(path.dirname(settings),{recursive:true});
    const temp=settings+".tmp";
    fs.writeFileSync(temp,JSON.stringify({saveDirectory:saved}),{mode:0o600});fs.renameSync(temp,settings);
    onChanged?.({directory:saved?.path || ""});
  }
  // Upgrade either old export preference once, taking the most recent valid choice.
  // A cleared/invalid canonical setting must not resurrect a stale legacy path.
  if(!canonicalExists) {
    const legacy=[];
    for(const [name,key] of [["diagnostics-export.json","diagnosticsSaveDirectory"],["share-export.json","saveDirectory"]]) {
      try {
        const file=path.join(app.getPath("userData"),name),value=JSON.parse(fs.readFileSync(file,"utf8"))[key];
        const record=typeof value==="string"?inspect(value):value;
        if(valid(record))legacy.push({record,time:fs.statSync(file).mtimeMs});
      }catch(_){}
    }
    legacy.sort((a,b)=>b.time-a.time);
    if(legacy.length)persist(legacy[0].record);
  }
  function getDirectory() {if(!valid(saved)){if(saved)persist(null);return "";}return saved.path;}
  function trusted(event) {return event?.sender===getMainWindow()?.webContents&&isTrustedRenderer(event.sender);}
  async function choose() {
    const result=await dialog.showOpenDialog(getMainWindow(),{title:"选择导出保存位置",defaultPath:getDirectory()||app.getPath("downloads"),properties:["openDirectory","createDirectory"]});
    if(result.canceled||!result.filePaths?.length)return {canceled:true,directory:getDirectory()};
    const directory=fs.realpathSync(result.filePaths[0]);persist(inspect(directory));return {canceled:false,directory};
  }
  return {
    getDirectory,
    getSaveDirectory(event){if(!trusted(event))throw Error("untrusted renderer");return {directory:getDirectory()};},
    chooseSaveDirectory(event){if(!trusted(event))throw Error("untrusted renderer");return choose();},
    rememberFile(file){persist(inspect(path.dirname(file)));},
    prepareFile(destination) {
      if(!valid(saved)||path.dirname(destination)!==saved.path)throw Error("directory changed");
      const folder=fs.mkdtempSync(path.join(app.getPath("userData"),"diagnostics-stage-"));
      const file=path.join(folder,"download.jsonl");staged.set(file,{record:{...saved},name:path.basename(destination),folder});return file;
    },
    discardFile(file) { const entry=staged.get(file); if(entry){staged.delete(file);fs.rmSync(entry.folder,{recursive:true,force:true});} },
    async finalizeFile(file) {
      const entry=staged.get(file);if(!entry)return file;
      try {
        let record=entry.record;
        if(!valid(record)){persist(null);const chosen=await choose();if(chosen.canceled)throw Error("save canceled");record={...saved};}
        // Recheck at the actual write, not just at directory selection/download start.
        if(!valid(record))throw Error("directory changed before write");
        const stat=fs.lstatSync(file);if(!stat.isFile()||stat.isSymbolicLink()||stat.size>(path.extname(entry.name)===".png"?64:16)*1024*1024)throw Error("untrusted staged export");
        const fd=fs.openSync(file,fs.constants.O_RDONLY|(fs.constants.O_NOFOLLOW||0));
        let data;try {const opened=fs.fstatSync(fd);if(opened.dev!==stat.dev||opened.ino!==stat.ino)throw Error("staged export changed");data=fs.readFileSync(fd);}finally{fs.closeSync(fd);}
        const extension=path.extname(entry.name)===".png"?".png":".jsonl",stem=path.basename(entry.name,extension);
        for(let n=1;n<=10000;n++){
          if(!valid(record))throw Error("directory changed during write");
          const target=path.join(record.path,stem+(n===1?"":"-"+n)+extension);let out;
          try {out=fs.openSync(target,fs.constants.O_WRONLY|fs.constants.O_CREAT|fs.constants.O_EXCL|(fs.constants.O_NOFOLLOW||0),0o600);}catch(e){if(e.code==="EEXIST")continue;throw e;}
          try {if(!valid(record))throw Error("directory replaced");fs.writeFileSync(out,data);}finally{fs.closeSync(out);}
          return target;
        }
        throw Error("too many filename collisions");
      } finally {staged.delete(file);fs.rmSync(entry.folder,{recursive:true,force:true});}
    },
  };
}
module.exports={createDiagnosticsDirectoryController};
