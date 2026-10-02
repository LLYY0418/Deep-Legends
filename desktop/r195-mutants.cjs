'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const {spawnSync}=require('node:child_process');
const assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r195-mutants-'));
const gameplay=fs.readFileSync(path.join(root,'backend/web/gameplay.js'),'utf8'),app=fs.readFileSync(path.join(root,'backend/web/app.js'),'utf8');
const cases=[
 {name:'suppress-lethal-tempo',test:'R195 2 ',gameplay:gameplay.replace('8008: ["已造成的伤害：@eogvar2@"]','8008: []')},
 {name:'remove-digit-time',test:'R195 3 ',gameplay:gameplay.replace(/^\s*else if \(digitTime[^\n]+\n/m,'\n')},
 {name:'restore-effect-shards',test:'R195 1 ',gameplay:gameplay.replace('${content}</div>`;', '${content}<div class="shards-line"><small>属性碎片</small></div></div>`;')},
 {name:'restore-hero-name-text',test:'R195 7 ',gameplay:gameplay.replace('championIconURL: Number(member.championId) > 0 && !member.championPickPending ? proxyAsset(assetPath("champion", liveDisplayedChampionId(member, currentChampionId))) : "",','champion: member.championName || "",'),app:app.replace('playerNode.append(icon, name);','const champion = document.createElement("span"); champion.className = "tooltip-roster-champion"; champion.textContent = String(item?.champion || ""); playerNode.append(name, champion);')},
];
const results=[];
for(const entry of cases){
 assert.notEqual(entry.gameplay,gameplay,`${entry.name} did not alter source`);
 const gameFile=path.join(temp,entry.name+'-gameplay.js'),appFile=path.join(temp,entry.name+'-app.js');
 fs.writeFileSync(gameFile,entry.gameplay);fs.writeFileSync(appFile,entry.app||app);
 const run=spawnSync(process.execPath,['--test','--test-name-pattern='+entry.test,'backend/web/r195.test.cjs'],{cwd:root,encoding:'utf8',env:{...process.env,R195_GAMEPLAY_SOURCE:gameFile,R195_APP_SOURCE:appFile}});
 const log=(run.stdout||'')+(run.stderr||'');fs.writeFileSync(path.join(temp,entry.name+'.log'),log);
 assert.notEqual(run.status,0,entry.name+' survived');assert.match(log,/AssertionError/,entry.name+' must fail an assertion');assert.doesNotMatch(log,/SyntaxError|ReferenceError/,entry.name+' failed to execute');
 results.push({name:entry.name,test:entry.test.trim(),killed:true});
}
const output=path.join(root,'docs/history/reports/r195');fs.mkdirSync(output,{recursive:true});fs.writeFileSync(path.join(output,'mutations.json'),JSON.stringify({results,logs:temp},null,2)+'\n');
console.log(JSON.stringify(results,null,2));
