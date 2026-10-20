'use strict';
const {browser}=require('./r297-browser-harness.cjs'),path=require('node:path'),fs=require('node:fs'),assert=require('node:assert/strict');
const out=path.join(process.env.R297_REPORT_DIR||path.join(require('node:os').tmpdir(),'r297-browser-proof'),'p7-rank-note');
(async()=>{const b=await browser({out});const results=[];try{
 await b.call('Page.navigate',{url:b.origin+'/?demo&section=overview'});await b.until('window.__r297?.activeTab()?.data&&!__r297.activeTab().loading');
 for(const theme of ['dark','light'])for(const capped of [false,true]){
 await b.evaluate(`(()=>{document.documentElement.dataset.theme='${theme}';const g=__r297,t=g.activeTab();t.data.seasonStatsProgress={...t.data.seasonStatsProgress,upstreamCapped:${capped},rankedOldestAt:Date.parse('2026-04-07T12:00:00')};g.renderOverview()})()`);
 const r=await b.evaluate(`(()=>{const h=document.querySelector('.rank-career-section h3'),n=h.querySelector('.season-data-note'),a=h.getBoundingClientRect(),c=n.getBoundingClientRect();return {title:h.textContent,copy:n.dataset.tooltip,center:Math.abs((a.top+a.bottom-c.top-c.bottom)/2),color:getComputedStyle(n).color}})()`);
 assert.equal(r.title,'排位');assert(r.center<1);assert.equal(r.copy,'统计的是当前整个赛季的排位数据'+(capped?'\n官方接口仅提供 4月7日 之后的对局，更早的场次暂时无法计入':''));
 await b.evaluate("document.querySelector('.rank-career-section .season-data-note').focus()");
 await b.until("document.querySelector('.global-tooltip')?.dataset.shown==='true'");
 const tooltip=await b.evaluate("({text:document.querySelector('.global-tooltip').textContent,space:getComputedStyle(document.querySelector('.global-tooltip .tooltip-body')||document.querySelector('.global-tooltip')).whiteSpace})");
 assert(tooltip.text.includes('统计的是当前整个赛季'));if(capped){assert(tooltip.text.includes('更早的场次暂时无法计入'));assert.equal(tooltip.space,'pre-line')}
 results.push({theme,capped,...r,tooltip});await b.shot(theme+'-'+capped);await b.evaluate('document.activeElement.blur()');
 }
 assert.deepEqual(b.errors,[]);fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({results,errors:b.errors},null,2));
}finally{await b.stop()}})().catch(e=>{console.error(e);process.exitCode=1});
