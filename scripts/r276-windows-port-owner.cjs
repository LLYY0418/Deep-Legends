'use strict';
// Native TCP table preserves the original exact-port/all-PIDs ownership proof
// without paying for a new PowerShell/CIM session at each installed UI phase.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{execFileSync}=require('node:child_process');
function listeningOwners(text,port){
 const owners=[];
 for(const line of text.split(/\r?\n/)){
  const fields=line.trim().split(/\s+/);
  if(fields[0]!=='TCP'||fields[3]!=='LISTENING'||!fields[1]?.endsWith(':'+port))continue;
  assert.equal(fields.length,5,'malformed native TCP owner row');
  assert(/^\d+$/.test(fields[4]),'native TCP owner PID must be numeric');
  const pid=Number(fields[4]);assert(Number.isSafeInteger(pid)&&pid>0,'native TCP owner PID must be positive');owners.push(pid);
 }
 return [...new Set(owners)];
}
function verifyInstalledPortOwner({port,expectedPID,evidence,execute=execFileSync,now=performance.now.bind(performance)}){
 assert(Number.isInteger(port)&&port>0&&port<65536,'invalid CDP owner port');assert(Number.isInteger(expectedPID)&&expectedPID>0,'installed Electron PID proof missing');
 const file=path.win32.join(process.env.SystemRoot||'C:\\Windows','System32','netstat.exe'),args=['-ano','-p','tcp'];
 const started=now(),record={port,expectedPID,owners:[],command:file,args,timeout_ms:5000};
 try{
  record.raw=execute(file,args,{encoding:'utf8',timeout:5000,windowsHide:true});record.owners=listeningOwners(record.raw,port);
  assert(record.owners.length>0&&record.owners.every(pid=>pid===expectedPID),'CDP port belongs to another process');
  return record;
 }catch(error){record.error={message:error.message,code:error.code};throw error;}
 finally{record.elapsed_ms=now()-started;if(evidence)fs.writeFileSync(evidence,JSON.stringify(record,null,2));}
}
module.exports={listeningOwners,verifyInstalledPortOwner};
