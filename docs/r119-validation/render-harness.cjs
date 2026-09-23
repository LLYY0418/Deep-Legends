const fs=require('fs'),vm=require('vm'),path=require('path');
const web='/home/claude/v/base/backend/web';
const src=fs.readFileSync(path.join(web,'gameplay.js'),'utf8');
function extract(name){const s=src.indexOf(`function ${name}(`);return src.slice(s,src.indexOf('\n  }',s)+4);}
const positionLabel=(v)=>({top:'上路',jungle:'打野',middle:'中路',bottom:'下路',utility:'辅助'})[v]||'位置未知';
const escapeHTML=(v)=>String(v).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/"/g,'&quot;');
const ctx={positionLabel,escapeHTML,playerParticipantName:(p)=>p.gameName,proBadgeAttributes:()=>'',
 iconFigure:()=>'<span class="game-icon is-small" style="background:#557;display:inline-block"></span>',
 renderMatchScoreCell:()=>'<span class="match-score-cell"><b class="match-score">7.4</b></span>',
 renderItemIcons:()=>'<span class="game-icon is-small" style="background:#665;display:inline-block"></span>',
 number:(v)=>String(v??0),kda:()=>'2.1'};
vm.runInNewContext(['matchTableRows','autofillChip','matchTableShell'].map(extract).join('\n'),ctx);
const players=[
 {participantId:1,gameName:'正常选位',position:'top',kills:5,deaths:2,assists:8},
 {participantId:2,gameName:'补位辅助',position:'utility',autofill:true,kills:1,deaths:4,assists:15},
 {participantId:3,gameName:'一个非常非常长的召唤师名字用来测试截断#KR1',position:'jungle',autofill:true,kills:9,deaths:1,assists:3},
 {participantId:4,gameName:'ShortNm',position:'middle',autofill:true,kills:3,deaths:3,assists:3},
 {participantId:5,gameName:'换位中单',position:'bottom',kills:7,deaths:5,assists:2},
];
const rows=ctx.matchTableRows(players,new Map());
const theme=process.argv[2]||'light', width=process.argv[3]||'1200';
const css=['app.css','build-item-row.css','gameplay.css','friends.css','metrics.css'].map(f=>`<style>${fs.readFileSync(path.join(web,f),'utf8')}</style>`).join('\n');
fs.writeFileSync(`page-${theme}-${width}.html`,`<!doctype html><html data-theme="${theme}" lang="zh-CN"><head><meta charset="utf-8">${css}</head><body style="padding:16px;background:var(--bg,var(--surface))"><section class="team-overview is-blue" style="width:${width-32}px"><header><strong>蓝方 · 胜利</strong><span>26 击杀</span></header>${ctx.matchTableShell(rows)}</section></body></html>`);
