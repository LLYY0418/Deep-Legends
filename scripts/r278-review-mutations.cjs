'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict'),{spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]);fs.mkdirSync(out,{recursive:true});
const tests=['backend/web/r278-claude-fixes.test.cjs','desktop/renderer-script-cache.test.cjs','backend/web/r278-measured-topbar.test.cjs'];
const rows=[];
function run(name,env={},negative=false,pattern){
 const args=['--test','--test-reporter=tap',...(pattern?['--test-name-pattern='+pattern]:[]),...tests];
 const result=spawnSync(process.execPath,args,{cwd:root,env:{...process.env,...env},encoding:'utf8',timeout:30000});
 const log=result.stdout+result.stderr;fs.writeFileSync(path.join(out,name+'.log'),log);assert.ifError(result.error);
 assert.equal(result.status,negative?1:0,name);
 if(negative){assert(/AssertionError|ERR_ASSERTION/.test(log),name+' needs a real assertion');assert(!/SyntaxError:|ReferenceError:|Cannot find module/.test(log),name+' failed outside its target');}
 rows.push({name,status:negative?'FAIL (expected)':'PASS',exitCode:result.status});
 fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(rows,null,2));
}
run('baseline');
const cases=[
 ['measured-compact-disabled','backend/web/app.js','if (topbarOverflows(bar) || level < minimumLevel) next = Math.min(5, level + 1);','if (false) next = Math.min(5, level + 1);','R278_CLAUDE_APP_SOURCE','measured compact controller'],
 ['compact-width-floor-removed','backend/web/app.js','const minimumLevel = logicalWidth <= 600 ? 3 : 0;','const minimumLevel = 0;','R278_CLAUDE_APP_SOURCE','logical width floor'],
 ['restart-permanent-manual-block','backend/web/app.js','const follow=ready && platform && (',"const follow=ready && platform && el.playerSearchRegion.dataset.manual!=='true' && (",'R278_CLAUDE_APP_SOURCE','approved restart contract'],
 ['corrupt-search-follow-removed','backend/web/app.js','if(follow){const from=','if(false){const from=','R278_CLAUDE_APP_SOURCE','approved corrupt contract'],
 ['official-countdown-restored','backend/web/gameplay.js','const copy=official?"英雄联盟官方战绩服务器暂时无法返回对局数据，游戏客户端里的战绩也会受影响。恢复后会自动重新读取。"','const copy=official?"英雄联盟官方战绩服务器暂时无法返回对局数据，游戏客户端里的战绩也会受影响。恢复后会自动重新读取。2 秒后自动重试"','R278_CONTRACT_GAMEPLAY_SOURCE','approved official-auto contract'],
 ['query-pointer-pan-removed','backend/web/app.js',"      if (event.buttons || !max()) return;",'      return;','R278_CLAUDE_APP_SOURCE','query viewport pans'],
 ['follow-writes-saved-preferences','backend/web/app.js','    if (manual) {\n      el.playerSearchRegion.dataset.manual', '    savePreference("search-region", region);\n    if (manual) {\n      el.playerSearchRegion.dataset.manual','R278_CLAUDE_APP_SOURCE','startup and client following'],
 ['empty-name-message-restored','backend/web/history-filters.js',"if(error){error.textContent='';error.hidden=true;}return false;","if(error){error.textContent='请先输入名称';error.hidden=false;}return false;",'R278_CLAUDE_FILTER_MODULE','empty name only'],
 ['popup-steals-input-focus','backend/web/announcements.js'," || doc.activeElement?.matches('input,textarea,select,[contenteditable=\"true\"]')",'','R278_CLAUDE_ANNOUNCEMENTS_MODULE','unread popup waits'],
 ['popup-steals-modal-focus','backend/web/announcements.js',',\u005baria-modal="true"\u005d:not(dialog):not([hidden]),[role="dialog"]:not(dialog):not([hidden])','','R278_CLAUDE_ANNOUNCEMENTS_MODULE','unread popup waits'],
 ['eager-announcement-assets','backend/web/app.js',"      if (!manual && (phase === 'unknown' || active.has(phase) || !unread.some(n => n.popup))) return;",'','R278_CLAUDE_APP_SOURCE','announcement assets are requested'],
 ['announcement-onload-not-awaited','backend/web/app.js',"        await window.deepLegendsSections.load('announcements');","        void window.deepLegendsSections.load('announcements');",'R278_CLAUDE_APP_SOURCE','announcement assets are requested'],
 ['renderer-cache-stale-source','desktop/renderer-script-cache.cjs','const key = filename + "\\0" + source;','const key = filename;','R278_RENDERER_CACHE_MODULE','compiled renderer cache'],
];
const scratch=fs.mkdtempSync(path.join(process.env.PI_SCRATCH_DIR||os.tmpdir(),'r278-review-mutants-'));
try{
 for(const [name,file,before,after,key,pattern] of cases){
  const source=fs.readFileSync(path.join(root,file),'utf8');assert.equal(source.split(before).length,2,name+' anchor');
  const mutant=path.join(scratch,name+path.extname(file));fs.writeFileSync(mutant,source.replace(before,after));run(name,{[key]:mutant},true,pattern);
 }
}finally{fs.rmSync(scratch,{recursive:true,force:true});}
run('restored');console.log(JSON.stringify({checks:rows.length}));
