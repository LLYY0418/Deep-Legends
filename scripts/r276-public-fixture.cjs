'use strict';
// Synthetic public executable for browser guards; never consumes real credentials.
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..');
function invoke(command,args,extra={}){const r=spawnSync(command,args,{cwd:root,env:{...process.env,RIOT_API_KEY:'',...extra},encoding:'utf8',maxBuffer:16*1024*1024});if(r.error||r.status!==0)throw Error(r.error?.message||r.stderr||'fixture build failed');return r.stdout.trim();}
function build(output){
 fs.mkdirSync(path.dirname(output),{recursive:true});
 const version=require('../desktop/package.json').version;
 const fingerprint=invoke(process.execPath,['desktop/source-fingerprint.cjs']);
 const cipher=invoke('go',['run','./backend','-encrypt-riot-key','RGAPI-00000000-0000-0000-0000-000000000000']);
 invoke('go',['build','-tags=','-buildvcs=false','-ldflags',`-X main.version=${version} -X main.buildFingerprint=${fingerprint} -X main.riotAPIKey= -X main.riotAPIKeyCipher=${cipher}`,'-o',output,'./backend']);
 invoke(process.execPath,['desktop/verify-embedded-riot-key.cjs','--expect-fake','--file='+output],{DEEP_LEGENDS_KEY_MODE:'public'});
 console.log('Synthetic public browser fixture built; fingerprint '+fingerprint);
 return output;
}
module.exports={build};
if(require.main===module)build(path.resolve(process.argv[2]));
