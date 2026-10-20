'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const {extract,assertOriginalStrings,decodeHTML,compareVersions,main}=require('./extract.cjs');
const fixture=v=>fs.readFileSync(path.join(__dirname,'fixtures',v+'.html'));
const expected={
 '26.20':{ranked:{sections:4,entries:20,items:14},aramMayhem:{sections:3,entries:13,items:14},arena:{sections:0,entries:0,items:0}},
 '26.17':{ranked:{sections:3,entries:16,items:6},aramMayhem:{sections:2,entries:7,items:6},arena:{sections:3,entries:23,items:0}},
 '26.16':{ranked:{sections:6,entries:22,items:12},aramMayhem:{sections:3,entries:11,items:19},arena:{sections:4,entries:25,items:1}},
};
for(const version of Object.keys(expected))test(`R297 official ${version}: every content string is a verbatim whitespace-normalized substring`,()=>{
 const {document,report,sourceText}=extract(fixture(version));assert.equal(document.version,version);assert.deepEqual(report.modes,expected[version]);assert(report.contentStringsChecked>300);assert.equal(assertOriginalStrings(document,sourceText),report.contentStringsChecked);assert.deepEqual(report.unmappedHeadings,[]);
 assert(document.modes.ranked.sections[0].entries.every(e=>e.name.includes(' ')),'ranked champion full official headings retained');
 for(const mode of ['aramMayhem','arena'])assert(!JSON.stringify(document.modes[mode]).includes('记分板显示优化'));
 assert(report.discarded.some(d=>d.heading==='经典模式'));assert(report.discarded.some(d=>d.heading==='即将到来的皮肤和炫彩'));
 const changed=structuredClone(document);changed.modes.ranked.sections[0].entries[0].groups.push({lines:[{text:'这里是自行翻译的说明'}]});assert.throws(()=>assertOriginalStrings(changed,sourceText),/逐字子串/);
});
test('R297 latest boundaries, explanations, intro, embedded list hero and exact numeric units',()=>{
 const {document:d}=extract(fixture('26.20')),ranked=d.modes.ranked,mayhem=d.modes.aramMayhem;
 assert.equal(d.publishedAt,'2026-10-08');assert.deepEqual(d.modes.arena.sections,[]);
 assert.match(ranked.sections[0].entries[0].groups[0].lines[0].text,/对于高熟练度玩家来说/);
 assert.match(mayhem.sections[0].intro,/各位海斗狂热者/);assert.equal(mayhem.sections[0].entries.length,6);assert.equal(mayhem.sections[0].entries[4].name,'斯维因');
 const bard=mayhem.sections[0].entries[0];assert.equal(bard.championId,432);assert.deepEqual(bard.groups[0].lines[0],{k:'木灵 法术强度收益率',from:'50%',to:'67%'});
 assert(!JSON.stringify(ranked).includes('木灵 充能时间'));assert(!JSON.stringify(mayhem).includes('火箭腰带】依然更多被用作逃生工具'));
 assert(ranked.sections.some(s=>s.title==='5人排位赛'&&s.intro.includes('16:00-次日04:00')));
});
test('R297 historical arena retains all official subblocks and never merges their entries into ranked',()=>{
 const d=extract(fixture('26.16')).document;
 assert.deepEqual(d.modes.arena.sections.map(s=>s.title),['英雄','装备','强化符文','BUG修复']);
 const arena=JSON.stringify(d.modes.arena),ranked=JSON.stringify(d.modes.ranked);assert(arena.includes('连招破坏者会在被控制后3秒后而不是4秒后触发'));assert(!ranked.includes('连招破坏者会在被控制后3秒后而不是4秒后触发'));
 assert(arena.includes('8秒-6秒'));assert(arena.includes('7秒-5秒'));assert(d.modes.arena.sections[0].intro.includes('勇敢斗魂竞技场'));
});
test('R297 unknown h2/h3/h4 fail publication and traditional page cannot publish',()=>{
 const html=decodeHTML(fixture('26.20'));
 for(const mutant of [html.replace('<h3><strong>英雄','<h3><strong>未知分组'),html.replace('<em>强化符文</em>','<em>未知子块</em>'),html.replace('<h3><strong>英雄','<h2>未知二级</h2><h3><strong>英雄')])assert.throws(()=>extract(mutant),/未映射标题/);
 assert.throws(()=>extract(html.replaceAll('国服','國服').replaceAll('装备','裝備')),/拒绝发布繁体/);
 assert.throws(()=>extract(html,'https://lol.qq.com.evil.example/gicp/news/410/1.html'),/只允许/);
});
test('R297 one command writes document, report and most recent three numerically sorted feed',()=>{
 const temp=fs.mkdtempSync(path.join(os.tmpdir(),'r297-patch-extract-'));
 try{for(const v of ['26.20','26.16','26.17'])main([path.join(__dirname,'fixtures',v+'.html'),'--out',temp]);const feed=JSON.parse(fs.readFileSync(path.join(temp,'patch-notes.json')));assert.deepEqual(feed.versions.map(d=>d.version),['26.20','26.17','26.16']);assert(fs.existsSync(path.join(temp,'26.20-report.json')));assert(compareVersions('26.9','26.10')<0)}finally{fs.rmSync(temp,{recursive:true,force:true})}
});
test('R297 nested future lists keep text lines separate and new entry marker is structural',()=>{
 const html=decodeHTML(fixture('26.20'));
 const nested=extract(html.replace('<li>木灵 法术强度收益率：50% ⇒ 67%</li>','<li>木灵 法术强度收益率：50% ⇒ 67%<ul><li>木灵 充能时间：8秒 ⇒ 5秒</li></ul></li>'));
 const first=nested.document.modes.aramMayhem.sections[0].entries[0];assert.equal(first.groups[0].lines[0].k,'木灵 法术强度收益率');assert.equal(first.groups[0].lines[1].k,'木灵 充能时间');
 const added=extract(html.replace('<strong>巴德</strong>','<strong>新增 巴德</strong>')).document.modes.aramMayhem.sections[0].entries[0];assert.equal(added.name,'巴德');assert.equal(added.isNew,true);
 assert.throws(()=>extract(html.replace('<h3><strong>英雄','<div><h3>未知嵌套标题</h3></div><h3><strong>英雄')),/未映射标题/);
});
