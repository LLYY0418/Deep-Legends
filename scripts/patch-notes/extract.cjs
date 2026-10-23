'use strict';
// Release snapshot extraction; runtime official discovery has a separate bounded parser.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const {execFileSync} = require('node:child_process');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const root = path.resolve(__dirname, '../..');
const championCatalog = require('./data/champions.json').champions;
const items = require('../../backend/data/arena_items_20260924.json').items;
const augments = require('../../backend/data/augment_catalog_20260924.json').augments;
const clean = text => String(text || '').replace(/\s+/gu, ' ').trim();
const compact = text => String(text).replace(/\s/gu, '');
const canonical = {
 '英雄':'champions','Champions':'champions','裝備':'items','装备':'items','Items':'items',
 '符文':'runes','Runes':'runes','系统':'systems','系統':'systems','Systems':'systems','账号系统后台升级':'account',
 '海克斯大乱斗':'aramMayhem','海克斯大亂鬥':'aramMayhem','ARAM: Mayhem':'aramMayhem',
 '斗魂竞技场':'arena','鬥魂競技場':'arena','Arena':'arena','5人排位赛':'flex',
 'BUG修复及品质级改动':'bugfixes','版本要闻':'discard','经典模式':'discard',
 '极地大乱斗':'discard','即将到来的皮肤和炫彩':'discard','私人重播':'discard','队伍语音':'discard',
};
const allowedSubheads = {
 aramMayhem: new Set(['英雄','强化符文','BUG修复','赛季之星','新增的强化符文','已移除的强化符文']),
 arena: new Set(['英雄','装备','强化符文','BUG修复']),
 bugfixes: new Set(['品质级改动','BUG修复']),
 systems: new Set(['射手（ADC）魔法抗性','打野 灵兽同伴','辅助 分路任务']),
};
const discardedSubheads = new Set(['英雄','装备','符文','符文与进度','系统','美术','BUG修复','BUG修复及品质级改动','经典模式 皮肤和个性化内容']);
const sectionIDs = {'英雄':'champions','装备':'items','强化符文':'augments','新增的强化符文':'new-augments','已移除的强化符文':'removed-augments','赛季之星':'headliners','BUG修复':'bugfixes','品质级改动':'quality'};
function officialURL(value) {
 const url = new URL(value);
 if(url.protocol !== 'https:' || url.hostname !== 'lol.qq.com' || url.username || url.password || url.port || !/^\/gicp\/news\/410\/\d+\.html$/.test(url.pathname) || url.search || url.hash) throw Error('只允许国服官方公告 HTTPS URL');
 return url.href;
}
function compareVersions(a,b) {
 const x=a.split('.').map(Number), y=b.split('.').map(Number);
 for(let i=0;i<Math.max(x.length,y.length);i++) if((x[i]||0)!==(y[i]||0)) return (x[i]||0)-(y[i]||0);
 return 0;
}
function decodeHTML(raw) {
 if(typeof raw==='string') return raw;
 const head=raw.subarray(0,2000).toString('ascii');
 return new TextDecoder(/charset\s*=\s*["']?\s*(?:gbk|gb2312|gb18030)/i.test(head)?'gb18030':'utf-8',{fatal:true}).decode(raw);
}
function lines(element) {
 const clone=element.cloneNode(true);
 clone.querySelectorAll('br').forEach(n=>n.replaceWith('\n'));
 clone.querySelectorAll('ul,ol').forEach(n=>n.prepend('\n'));
 clone.querySelectorAll('p,li').forEach(n=>n.append('\n'));
 return clone.textContent.split('\n').map(clean).filter(Boolean);
}
function resolveID(name,kind) {
 const plain=name.replace(/^(?:\[新(?:增)?\]|新增)\s*/, '');
 if(kind==='champions') {
  const champion=championCatalog.find(c=>[c.name,c.title,`${c.name} ${c.title}`].some(n=>compact(n)===compact(plain)));
  return champion?{championId:champion.id}:{};
 }
 if(kind==='items') {const item=items.filter(i=>i.name===plain).sort((a,b)=>a.id-b.id)[0];return item?{itemId:item.id}:{}}
 if(kind==='augments') {const augment=augments.find(a=>a.name===plain);return augment?{augmentId:augment.id}:{}}
 return {};
}
const contentKeys=new Set(['title','intro','name','label','k','from','to','text','items']);
function assertOriginalStrings(document,sourceText) {
 const original=compact(sourceText);let count=0;
 function visit(value,key='') {
  if(typeof value==='string' && contentKeys.has(key)) {count++;if(!original.includes(compact(value))) throw Error(`文字不是官方原文逐字子串 (${key}): ${value}`)}
  else if(Array.isArray(value))value.forEach(v=>visit(v,key));
  else if(value && typeof value==='object')for(const [k,v]of Object.entries(value))visit(v,k);
 }
 visit(document);return count;
}
function extract(raw,sourceURL) {
 const html=decodeHTML(raw),dom=new JSDOM(html),d=dom.window.document;
 try {
  const article=d.querySelector('#article'),title=clean(d.querySelector('h1.art-tit')?.textContent);
  if(!article || !/^\d+\.\d+版本更新公告$/.test(title))throw Error('不是可识别的国服版本更新公告');
  const nested=[...article.querySelectorAll('h2,h3,h4')].filter(n=>n.parentElement!==article);if(nested.length)throw Error('未映射标题（嵌套结构）：'+nested.map(n=>clean(n.textContent)).join('；'));
  const sourceText=article.textContent;
  if(!sourceText.includes('国服') || /國服|裝備|強化符文|鬥魂競技場|錯誤修正/.test(sourceText))throw Error('拒绝发布繁体或非国服简体页面，必须核对 lol.qq.com 原文');
  const version=title.match(/^[\d.]+/)[0],articleID=d.querySelector('#readCount')?.getAttribute('data');
  const sourceUrl=officialURL(sourceURL || `https://lol.qq.com/gicp/news/410/${articleID}.html`);
  const publishedAt=d.querySelector('.art-time')?.getAttribute('data');
  if(!/^\d{4}-\d{2}-\d{2}$/.test(publishedAt||''))throw Error('公告发布日期缺失');
  const modes={ranked:{sections:[]},aramMayhem:{sections:[]},arena:{sections:[]}};
  const report={version,sourceUrl,publishedAt,sourceSHA256:crypto.createHash('sha256').update(Buffer.isBuffer(raw)?raw:Buffer.from(html)).digest('hex'),discarded:[],unmappedHeadings:[],unresolvedNames:[],modes:{}};
  let major='',majorTitle='',minor='',section=null,entry=null,group=null,intro=[];
  const getSection=(mode,id,title)=>{let result=modes[mode].sections.find(s=>s.id===id&&s.title===title);if(!result){result={id,title,entries:[]};modes[mode].sections.push(result)}return result};
  const newSection=(mode,id,title)=>{section=getSection(mode,id,title);entry=null;group=null;if(intro.length){section.intro=[section.intro,...intro].filter(Boolean).join('\n');intro=[]}return section};
  const startEntry=name=>{const kind=sectionIDs[minor]||major,id=resolveID(name,kind);entry={name:name.replace(/^(?:\[新(?:增)?\]|新增)\s*/,''),...id,groups:[]};if(/^(?:\[新(?:增)?\]|新增)/.test(name))entry.isNew=true;section.entries.push(entry);group=null;if(!Object.keys(id).length&&['champions','items','augments'].includes(kind))report.unresolvedNames.push({mode:['arena','aramMayhem'].includes(major)?major:'ranked',section:section.title,name})};
  const append=text=>{
   if(!section){intro.push(text);return}
   if(!entry){section.intro=[section.intro,text].filter(Boolean).join('\n');return}
   if(/^(?:被动\s*[-－]|[QWER]\s*[-－]|基础属性|基础能力值)/.test(text) && !/[⇒→]/.test(text)) {group={label:text,lines:[]};entry.groups.push(group);return}
   if(!group){group={lines:[]};entry.groups.push(group)}
   const match=text.match(/^(.+?)[：:]\s*(.+?)\s*[⇒→]\s*(.+)$/);
   group.lines.push(match?{k:match[1],from:match[2],to:match[3]}:{text});
  };
  const routeBug=text=>{
   let mode='ranked';
   if(/私人重播|私人回放|队伍语音|团队语音/.test(text)){report.discarded.push({heading:majorTitle,text});return}
   if(/斗魂|竞技场/.test(text))mode='arena';else if(/海克斯大乱斗/.test(text))mode='aramMayhem';else if(/经典模式|极地大乱斗/.test(text)){report.discarded.push({heading:majorTitle,text});return}
   const s=getSection(mode,'bugfixes-'+(sectionIDs[minor]||'general'),minor||majorTitle);delete s.entries;(s.items ||= []).push(text);
  };
  for(const el of article.children) {
   if(el.tagName==='H2') {report.unmappedHeadings.push('h2: '+clean(el.textContent));continue}
   if(el.tagName==='H3') {
    const name=clean(el.textContent);majorTitle=name;major=canonical[name];minor='';section=null;entry=null;group=null;intro=[];
    if(!major)report.unmappedHeadings.push('h3: '+name);
    else if(major==='discard')report.discarded.push({heading:name});
    else if(['champions','items','runes','systems','flex','account'].includes(major))newSection('ranked',major,name);
    continue;
   }
   if(el.tagName==='H4') {
    const name=clean(el.textContent);minor=name;entry=null;group=null;
    if(major==='discard') {if(!discardedSubheads.has(name))report.unmappedHeadings.push(`${majorTitle} / h4: ${name}`);continue}
    if(allowedSubheads[major]&&!allowedSubheads[major].has(name)){report.unmappedHeadings.push(`${majorTitle} / h4: ${name}`);continue}
    if(['aramMayhem','arena'].includes(major))newSection(major,sectionIDs[name],name);
    else if(['champions','items','runes','systems'].includes(major))startEntry(name);
    else if(major!=='bugfixes')report.unmappedHeadings.push(`${majorTitle} / h4: ${name}`);
    continue;
   }
   if(!major || major==='discard')continue;
   const blocks=['UL','OL'].includes(el.tagName)?[...el.children].filter(n=>n.tagName==='LI'):[el];
   for(const block of blocks) {
    const textLines=lines(block);
    for(let index=0;index<textLines.length;index++) {
     const text=textLines[index];
     if(major==='bugfixes'){routeBug(text);continue}
     if(['aramMayhem','arena'].includes(major)&&minor==='BUG修复'){delete section.entries;(section.items ||= []).push(text);continue}
     const kind=sectionIDs[minor];
     if(['headliners','removed-augments'].includes(kind)) {delete section.entries;(section.items ||= []).push(text);continue}
     if(kind==='new-augments') {
      if(['白银阶','黄金阶','棱彩阶'].includes(text))continue;
      if(block.tagName==='P') {startEntry(text);entry.isNew=true;continue}
     }
     if(['aramMayhem','arena'].includes(major)&&section&&kind) {
      const strong=clean(block.querySelector('strong,b')?.textContent),known=Object.keys(resolveID(text,kind)).length>0;
      if(known || index===0&&strong===text&&text.length<100) {startEntry(text);continue}
     }
     if(/私人重播|私人回放|队伍语音|团队语音/.test(text)){report.discarded.push({heading:majorTitle,text});continue}
     append(text);
    }
   }
  }
  if(report.unmappedHeadings.length){const error=Error('未映射标题：'+report.unmappedHeadings.join('；'));error.report=report;throw error}
  const document={schema:1,version,publishedAt,sourceUrl,modes};
  report.contentStringsChecked=assertOriginalStrings(document,sourceText);
  for(const [mode,value]of Object.entries(modes))report.modes[mode]={sections:value.sections.length,entries:value.sections.reduce((n,s)=>n+(s.entries?.length||0),0),items:value.sections.reduce((n,s)=>n+(s.items?.length||0),0)};
  return {document,report,sourceText};
 } finally {dom.window.close()}
}
function readSource(input) {
 if(!/^https?:\/\//i.test(input))return {raw:fs.readFileSync(input)};
 const url=officialURL(input);
 const result=execFileSync('curl',['--fail','--silent','--show-error','--location','--proto','=https','--proto-redir','=https','--max-time','30','--max-filesize','2097152','--write-out','\nR297_FINAL_URL:%{url_effective}',url],{maxBuffer:3<<20});
 const marker=Buffer.from('\nR297_FINAL_URL:'),index=result.lastIndexOf(marker);if(index<0)throw Error('下载缺少来源信息');officialURL(result.subarray(index+marker.length).toString());return {raw:result.subarray(0,index),sourceURL:url};
}
function main(args) {
 const input=args[0];if(!input)throw Error('用法：node scripts/patch-notes/extract.cjs <官方URL|HTML路径> [--out 输出目录]');
 const index=args.indexOf('--out'),base=index>=0?path.resolve(args[index+1]):path.join(root,'docs/history/reports/patch-notes');
 fs.mkdirSync(base,{recursive:true});let parsed,source;
 try{source=readSource(input);parsed=extract(source.raw,source.sourceURL)}catch(error){fs.writeFileSync(path.join(base,'failed-report.json'),JSON.stringify(error.report||{error:error.message},null,2)+'\n');throw error}
 const {document,report}=parsed,existingFile=path.join(base,'patch-notes.json');
 fs.writeFileSync(path.join(base,document.version+'-source.html'),source.raw);
 let versions=[];if(fs.existsSync(existingFile))versions=JSON.parse(fs.readFileSync(existingFile)).versions||[];
 versions=[document,...versions.filter(v=>v.version!==document.version)].sort((a,b)=>compareVersions(b.version,a.version)).slice(0,3);
 fs.writeFileSync(path.join(base,document.version+'.json'),JSON.stringify(document,null,2)+'\n');
 fs.writeFileSync(path.join(base,document.version+'-report.json'),JSON.stringify(report,null,2)+'\n');
 fs.writeFileSync(existingFile,JSON.stringify({schema:1,versions},null,2)+'\n');
 console.log(JSON.stringify({version:document.version,modes:report.modes,unresolvedNames:report.unresolvedNames.length,stringsChecked:report.contentStringsChecked,json:existingFile,report:path.join(base,document.version+'-report.json')},null,2));
}
if(require.main===module){try{main(process.argv.slice(2))}catch(e){console.error(e.message);process.exitCode=1}}
module.exports={extract,assertOriginalStrings,decodeHTML,compareVersions,officialURL,main};
