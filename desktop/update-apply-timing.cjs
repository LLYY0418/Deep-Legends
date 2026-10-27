'use strict';
const fs=require('node:fs'),path=require('node:path');
function markWindowsHidden(root,origin,now=Date.now()) {
 if(!Number.isSafeInteger(origin)||origin<=0||now<origin||now-origin>3600000)return;
 const file=path.join(root,'update-apply-timing.json');
 try {const info=fs.lstatSync(file);if(!info.isFile()||info.isSymbolicLink()||info.size>4096)return;const stages=JSON.parse(fs.readFileSync(file,'utf8'));stages.windows_hidden=now-origin;fs.writeFileSync(file,JSON.stringify(stages),{mode:0o600});}catch{}
}
module.exports={markWindowsHidden};
