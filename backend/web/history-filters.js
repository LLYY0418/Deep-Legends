/* R258: mode selection belongs to gameplay; these conditions operate on its loaded rows. */
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.deepLegendsHistoryFilters = api;
})(typeof window === 'undefined' ? globalThis : window, () => {
  'use strict';
  const categories = {result:'结果',hero:'英雄',position:'位置',time:'时间',coplayer:'同局玩家',multikill:'多杀',performance:'表现',duration:'对局时长'};
  const labels = {win:'胜利',loss:'失败',top:'上路',jungle:'打野',middle:'中路',bottom:'下路',utility:'辅助',today:'今天',d3:'近 3 天',d7:'近 7 天',d30:'近 30 天',season:'本赛季',3:'三杀',4:'四杀',5:'五杀',mvp:'MVP',svp:'SVP',top3:'前三',zero:'零阵亡',short:'< 20 分钟',medium:'20–30 分钟',long:'30–40 分钟',verylong:'> 40 分钟'};
  const key = 'deep-legends-history-presets-v1';
  const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const numeric = value => value != null && value !== '' && Number.isFinite(Number(value));
  const clone = value => JSON.parse(JSON.stringify(value));
  const conditions = tab => tab.advancedConditions || {};
  const active = tab => Object.keys(conditions(tab)).length > 0;
  function normalize(input) {
    const output = {};
    for (const category of Object.keys(categories)) {
      const c = input?.[category]; if (!c || typeof c !== 'object') continue;
      const values = [...new Set((Array.isArray(c.values) ? c.values : []).filter(v => typeof v === 'string').slice(0,512))];
      const thresholds = {};
      if (category === 'performance') for (const name of ['kda','score','kp']) if (numeric(c.thresholds?.[name]) && Number(c.thresholds[name]) >= 0) thresholds[name] = Math.min(name === 'kp' ? 100 : 1000, Number(c.thresholds[name]));
      if (values.length || Object.keys(thresholds).length) output[category] = {values, not:c.not === true, ...(category === 'performance' ? {scope:c.scope === 'all' ? 'all' : 'team', thresholds} : {}), ...(category === 'multikill' ? {atLeast:c.atLeast === true} : {}), ...(category === 'coplayer' ? {side:['team','opponent'].includes(c.side) ? c.side : 'either'} : {})};
    }
    return output;
  }
  function sameTeam(a,b) { return Number(a?.subteamId)>0 ? Number(a.subteamId)===Number(b?.subteamId) : a?.teamId != null && a.teamId===b?.teamId; }
  function multikill(subject, level, atLeast = false) {
    const fields = ['tripleKills','quadraKills','pentaKills'];
    const absent = fields.every(field => subject?.[field] == null);
    if (absent) return atLeast ? Number(subject?.multiKill) >= level : Number(subject?.multiKill) === level;
    return fields.some((field,index) => (atLeast ? index+3 >= level : index+3 === level) && Number(subject?.[field]) > 0);
  }
  function participation(match, subject) {
    if (!subject || !rift(match)) return null;
    const team = (match.participants || []).filter(p=>sameTeam(subject,p));
    if (!team.length || !team.every(p=>numeric(p.kills)) || !numeric(subject.assists)) return null;
    const total = team.reduce((n,p)=>n+Number(p.kills),0);
    return total>0 ? 100*(Number(subject.kills)+Number(subject.assists))/total : null;
  }
  function rift(match) { return Number(match.mapId)===11 || !Number(match.mapId) && ['solo','flex','match'].includes(match.modeGroup); }
  function timeValue(value) { const date = new Date(numeric(value) ? Number(value) : value); return Number.isFinite(date.getTime()) ? date.getTime() : NaN; }
  function matchesValue(category,value,c,match,ctx) {
    const s = ctx.subject(match); if (!s) return false;
    switch(category) {
      case 'result': return ['win','loss'].includes(match.result) && match.result===value;
      case 'hero': return String(s.championId)===value;
      case 'position': return rift(match) && s.position===value;
      case 'time': {
        const now=ctx.now?.() ?? Date.now(), day=new Date(now); day.setHours(0,0,0,0);
        const start=value==='season' ? timeValue(ctx.seasonStart) : value==='today' ? day.getTime() : now-Number(value.slice(1))*86400000;
        const at=timeValue(match.createdAt);return Number.isFinite(start) && at>=start && at<=now;
      }
      case 'coplayer': return (match.participants || []).some(p=>p!==s && p.playerRef===value && (c.side==='either' || !c.side || (c.side==='team' ? sameTeam(s,p) : !sameTeam(s,p))));
      case 'multikill': return multikill(s,Number(value),c.atLeast);
      case 'duration': {
        const minutes=Number(match.durationSeconds || match.duration)/60;if(!(minutes>0))return false;
        return value==='short' ? minutes<20 : value==='medium' ? minutes>=20 && minutes<30 : value==='long' ? minutes>=30 && minutes<=40 : minutes>40;
      }
      case 'performance': {
        const score=s.score;
        if(value==='mvp' || value==='svp')return String(score?.badge || '').toLowerCase()===value;
        if(value==='top3')return numeric(score?.rank) && Number(score.rank)>0 && Number(score.rank)<=3;
        if(value==='zero')return s.deaths===0;
        return value.startsWith('tag:') && (ctx.tags(match,s) || []).some(tag=>tag.kind===value.slice(4) && (c.scope!=='all' || tag.star));
      }
      default:return false;
    }
  }
  function matchesCategory(category,c,match,ctx) {
    const s=ctx.subject(match);
    const selected = !c.values.length || c.values.some(value=>matchesValue(category,value,c,match,ctx));
    const gates = category!=='performance' || Object.entries(c.thresholds || {}).every(([name,minimum])=> {
      const value=name==='kp' ? participation(match,s) : name==='score' ? s?.score?.value : s?.kda;
      return numeric(value) && Number(value)>=minimum;
    });
    const matched=selected && gates;
    if(category==='performance' && Object.keys(c.thresholds || {}).some(name=>!numeric(name==='kp' ? participation(match,s) : name==='score' ? s?.score?.value : s?.kda)))return false;
    // Missing result/position evidence must not become a positive "not" match.
    if(category==='result' && !['win','loss'].includes(match.result))return false;
    if(category==='position' && (!rift(match) || !['top','jungle','middle','bottom','utility'].includes(s?.position)))return false;
    return c.not ? !matched : matched;
  }
  function filter(rows,tab,ctx) { const cs=conditions(tab);return rows.filter(match=>Object.entries(cs).every(([category,c])=>matchesCategory(category,c,match,ctx))); }
  function options(rows,category,ctx,c={values:[]}) {
    const map=new Map();
    const add=(value,label,match,subject,extra={})=>{
      if(!value)return;const id=String(value),entry=map.get(id) || {value:id,label:label || id,count:0,wins:0,recent:0,...extra};
      entry.count++;entry.wins+=match.result==='win'?1:0;entry.recent=Math.max(entry.recent,timeValue(match.createdAt)||0);map.set(id,entry);
    };
    for(const match of rows) {
      const s=ctx.subject(match);if(!s)continue;
      if(category==='hero')add(s.championId,s.championName,match,s);
      else if(category==='coplayer') { for(const p of match.participants || [])if(p!==s && p.playerRef && (c.side==='either' || !c.side || (c.side==='team' ? sameTeam(s,p) : !sameTeam(s,p))))add(p.playerRef,p.gameName ? `${p.gameName}${p.tagLine?'#'+p.tagLine:''}` : p.displayName || p.summonerName || '隐藏玩家',match,s); }
      else if(category==='performance') {
        for(const value of ['mvp','svp','top3','zero'])if(matchesValue(category,value,c,match,ctx))add(value,labels[value],match,s);
        for(const tag of ctx.tags(match,s) || [])if(c.scope!=='all' || tag.star)add('tag:'+tag.kind,tag.label.replace(/×.*/,''),match,s,{kind:tag.kind});
      } else {
        const values=category==='result'?['win','loss']:category==='position'?['top','jungle','middle','bottom','utility']:category==='time'?['today','d3','d7','d30','season']:category==='multikill'?['3','4','5']:['short','medium','long','verylong'];
        for(const value of values)if(matchesValue(category,value,c,match,ctx))add(value,labels[value],match,s);
      }
    }
    return [...map.values()];
  }
  function readPresets(storage) { try {const saved=JSON.parse(storage.getItem(key) || '[]');return Array.isArray(saved) ? saved.filter(p=>p && typeof p.name==='string').slice(0,12).map(p=>({name:p.name.slice(0,60),conditions:normalize(p.conditions)})) : [];} catch(_){return [];} }
  function savePreset(storage,name,cs) { const saved=readPresets(storage),n=String(name || '').trim().slice(0,60);if(!n || !Object.keys(normalize(cs)).length)return false;const index=saved.findIndex(p=>p.name===n);if(index<0 && saved.length>=12)return false;const item={name:n,conditions:normalize(cs)};if(index<0)saved.push(item);else saved[index]=item;try{storage.setItem(key,JSON.stringify(saved));return true;}catch(_){return false;} }
  function writePresets(storage,saved) {try{storage.setItem(key,JSON.stringify(saved.slice(0,12)));return true;}catch(_){return false;} }
  function cancel(tab, abort) { const search=tab.advancedSearch;if(search?.running){search.running=false;search.cancelled=true;search.token++;abort?.();} }
  async function find(tab,ctx) {
    if(!active(tab) || tab.advancedSearch?.running || !ctx.isActive())return false;
    const token=(tab.advancedSearch?.token || 0)+1;
    const search={token,running:true,cancelled:false,scanned:0};tab.advancedSearch=search;ctx.render();
    try {
      while(search.running && search.token===token && ctx.isActive() && !tab.closed) {
        if(filter(ctx.rows(),tab,ctx).length>=10 || !tab.data?.pagination?.hasMore || search.scanned>=300)break;
        const before=ctx.cursor(),count=Math.min(20,300-search.scanned);
        const loaded=await ctx.load(count);
        if(!search.running || search.token!==token || !ctx.isActive() || tab.closed)break;
        const consumed=Math.max(0,ctx.cursor()-before);
        search.scanned+=Math.min(count,consumed);ctx.render();
        if(!loaded || consumed<=0 || tab.data?.pagination?.moreError)break;
      }
    } finally {if(tab.advancedSearch===search && search.token===token){search.running=false;ctx.render();}}
    return true;
  }
  function ui(tab) {return tab.advancedMenu ||= {open:false,page:'conditions',category:'',query:'',sort:'count',renaming:-1};}
  function draft(tab,category) { return conditions(tab)[category] || ui(tab).drafts?.[category] || {values:[],not:false,scope:'team',side:'either',thresholds:{}}; }
  function summary(category,c,ctx) {
    const names=options(ctx.rows(),category,ctx,c);const text=c.values.map(v=>names.find(o=>o.value===v)?.label || labels[v] || v.replace('tag:','')).join(' / ');
    const gates=Object.entries(c.thresholds || {}).map(([name,value])=>`${{kda:'KDA',score:'评分',kp:'参团'}[name]} ≥ ${value}${name==='kp'?'%':''}`).join(' · ');
    return [text,gates,c.atLeast?'及以上':''].filter(Boolean).join(' · ');
  }
  const button=(text,attribute,selected=false)=>`<button type="button" ${attribute}${selected?' class="is-active" aria-pressed="true"':' aria-pressed="false"'}>${text}</button>`;
  function menuBody(tab,ctx) {
    const m=ui(tab),saved=readPresets(ctx.storage),cs=conditions(tab);
    if(m.page==='saved')return `<div class="af-saved-list">${saved.map((p,i)=>m.renaming===i ? `<div class="af-saved-row is-renaming"><input data-af-rename-input="${i}" value="${escape(p.name)}" maxlength="60" aria-label="新名称"><button type="button" data-af-rename-save="${i}" aria-label="保存名称">✓</button><button type="button" data-af-rename-cancel aria-label="取消重命名">×</button></div>` : `<div class="af-saved-row"><button type="button" data-af-apply="${i}"><strong>${escape(p.name)}</strong><small>${Object.keys(p.conditions).map(k=>categories[k]).join(' · ')}</small></button><button type="button" data-af-rename="${i}" aria-label="重命名 ${escape(p.name)}">✎</button><button type="button" data-af-delete="${i}" aria-label="删除 ${escape(p.name)}">×</button></div>`).join('') || '<p class="af-empty">暂无常用筛选</p>'}</div><footer><span>${saved.length} / 12</span><input data-af-name placeholder="名称" aria-label="常用筛选名称" maxlength="60"><button type="button" data-af-save${!active(tab)?' disabled':''}>保存当前条件</button></footer>`;
    const query=m.query.toLowerCase().trim();
    if(!m.category)return `<input class="af-search" data-af-query placeholder="搜索英雄、玩家或条件" aria-label="搜索条件" value="${escape(m.query)}"><div class="af-categories">${Object.entries(categories).filter(([k,label])=>!query || label.includes(query) || options(ctx.rows(),k,ctx,cs[k]).some(o=>searchOption(o,k,query,ctx))).map(([k,label])=>`<button type="button" data-af-category="${k}"><span>${label}</span><small>${options(ctx.rows(),k,ctx,cs[k]).length}</small><span>›</span></button>`).join('')}</div>`;
    const category=m.category,c=draft(tab,category);
    let opts=options(ctx.rows(),category,ctx,c).filter(o=>!query || searchOption(o,category,query,ctx));
    if(category==='hero')opts.sort((a,b)=>m.sort==='winrate' ? b.wins/b.count-a.wins/a.count : m.sort==='recent' ? b.recent-a.recent : b.count-a.count);
    const segment=category==='coplayer' ? [['team','队友'],['opponent','对手'],['either','都行']] : category==='performance' ? [['team','队内最高'],['all','全场最高']] : [];
    const controls=segment.length ? `<div class="af-segment">${segment.map(([value,label])=>button(label,`data-af-setting="${category==='coplayer'?'side':'scope'}" data-af-value="${value}"`,c[category==='coplayer'?'side':'scope']===value)).join('')}</div>` : '';
    const sort=category==='hero' ? `<div class="af-segment">${[['count','场次'],['winrate','胜率'],['recent','最近']].map(([value,label])=>button(label,`data-af-sort="${value}"`,m.sort===value)).join('')}</div>` : '';
    const cards=opts.map(o=>button(`${category==='hero'?ctx.icon?.(o.value,o.label) || '':category==='position'?ctx.positionIcon?.(o.value) || '':''}<strong>${escape(o.label)}</strong><small>${o.count} 场${category==='hero'?' · '+Math.round(100*o.wins/o.count)+'%':''}</small>`,`data-af-option="${escape(o.value)}"`,c.values.includes(o.value))).join('');
    const thresholds=category==='performance' ? `<div class="af-thresholds">${[['kda','KDA',0.5],['score','评分',0.5],['kp','参团率',5]].map(([name,label,step])=>`<label>${label} ≥<input type="number" min="0" ${name==='kp'?'max="100"':''} step="${step}" placeholder="—" value="${c.thresholds?.[name] ?? ''}" data-af-threshold="${name}" aria-label="${label}门槛"><span>${name==='kp'?'%':''}</span></label>`).join('')}</div>` : '';
    return `<header>${button('‹ 条件','data-af-back')}<strong>${categories[category]}</strong><div class="af-segment">${button('是','data-af-not="false"',!c.not)}${button('不是','data-af-not="true"',c.not)}</div></header><input class="af-search" data-af-query placeholder="搜索${categories[category]}" aria-label="搜索${categories[category]}" value="${escape(m.query)}">${sort}${controls}<div class="af-options is-${category}">${cards || '<p class="af-empty">暂无可选项</p>'}</div>${category==='multikill'?`<label class="af-atleast"><input type="checkbox" data-af-atleast${c.atLeast?' checked':''}>及以上</label>`:''}${thresholds}`;
  }
  function searchOption(o,category,query,ctx) {return o.label.toLowerCase().includes(query) || category==='hero' && ctx.heroSearch?.(query,o.value,o.label)>0;}
  function render(tab,context) {
    const ctx=context(),m=ui(tab),cs=conditions(tab),n=Object.keys(cs).length,saved=readPresets(ctx.storage);
    return `<div class="af-select app-select${n?' is-active':''}" data-af-root><button class="app-select-trigger" type="button" data-af-open aria-haspopup="dialog" aria-expanded="${m.open}"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M3 4h14l-5.5 6v5l-3 1v-6z" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>筛选${n?`<b>${n}</b>`:''}</button><div class="app-select-menu af-menu is-${escape(m.category || 'categories')}" data-af-menu role="dialog" aria-label="战绩筛选"${m.open?'':' hidden'}><nav role="tablist">${button('条件',`data-af-page="conditions" role="tab" aria-selected="${m.page==='conditions'}"`,m.page==='conditions')}${button(`常用 ${saved.length}`,`data-af-page="saved" role="tab" aria-selected="${m.page==='saved'}"`,m.page==='saved')}</nav><div class="af-menu-body">${m.open?menuBody(tab,ctx):''}</div></div></div>`;
  }
  function counts(tab,ctx) {
    const search=tab.advancedSearch,count=filter(ctx.rows(),tab,ctx).length,scanned=search?.running ? search.scanned : Number(ctx.cursor() || ctx.rows().length);
    return `<span class="af-count"><b>${count}</b> 场</span><span>已查 ${scanned}${search?.running?' / 300':''} 场</span>${search?.running?`<progress max="300" value="${scanned}" aria-label="查找进度"></progress>${button('停止','data-af-stop')}`:`${button('常用','data-af-show-saved')}${button('清除','data-af-clear')}`}`;
  }
  function renderConditions(tab,context) {
    if(!active(tab))return '';const ctx=context(),cs=conditions(tab);
    return `<div class="af-conditionbar" data-af-conditions><div class="af-capsules">${Object.entries(cs).map(([category,c])=>`<span class="af-capsule${c.not?' is-not':''}">${button(`${categories[category]} <i>|</i> ${c.not?'<em>不是</em> ':''}${escape(summary(category,c,ctx))}`,`data-af-category="${category}"`)}<button type="button" data-af-remove="${category}" aria-label="删除${categories[category]}条件">×</button></span>`).join('')}${button('+ 条件','data-af-add class="af-add"')}</div><div class="af-counts" data-af-counts>${counts(tab,ctx)}</div></div>`;
  }
  function empty(tab,ctx) {return `<div class="af-empty"><strong>没有符合的对局 · 已查 ${ctx.cursor()} 场</strong><div>${button('修改条件','data-af-add')}${button('再往前查 300 场','data-af-continue')}${tab.data?.pagination?.moreError?`<p>${escape(tab.data.pagination.moreError)}</p>`:''}</div></div>`;}
  function footer(tab,ctx) {return `<span>已查 ${ctx.cursor()} 场</span>${tab.advancedSearch?.running?button('停止','data-af-stop'):tab.data?.pagination?.hasMore?button('继续往前找','data-af-continue'):''}`;}
  function bind(container,tab,context) {
    if(!container)return;
    container._afBoundTab=tab;
    // One delegated listener per container, even when its filterbar is replaced.
    container._afContext=context;
    container.ownerDocument._afTarget={container,tab,context};
    if(container._afListeners)return;container._afListeners=true;
    const current=()=>({tab:container._afBoundTab,ctx:container._afContext()});
    const update=(t,ctx,category,c)=>{const old=container.ownerDocument.activeElement,attr=[...(old?.attributes || [])].find(a=>a.name.startsWith('data-af-'));(ui(t).drafts ||= {})[category]=clone(c);t.advancedConditions=normalize({...conditions(t),[category]:c});ctx.change();const fresh=attr && [...container.querySelectorAll('['+attr.name+']')].find(node=>node.getAttribute(attr.name)===attr.value);(fresh || container.querySelector('[data-af-open]'))?.focus();};
    const redraw=(t,ctx,focus=false)=>{ctx.render();const fresh=ctx.container?.() || container;if(focus)fresh.querySelector('[data-af-query]')?.focus();};
    // Electron does not implement window.prompt, so saved filters are renamed inline.
    const renamePreset=(t,ctx,i)=>{const m=ui(t),saved=readPresets(ctx.storage),input=(ctx.container?.() || container).querySelector(`[data-af-rename-input="${i}"]`),name=String(input?.value || '').trim().slice(0,60);
      if(!saved[i]){m.renaming=-1;redraw(t,ctx);return;}
      if(!name){input?.focus();return;}
      if(saved.some((p,index)=>index!==i && p.name===name)){ctx.error?.('已有同名常用筛选');input?.focus();return;}
      saved[i].name=name;if(!writePresets(ctx.storage,saved))ctx.error?.('常用筛选保存失败');m.renaming=-1;redraw(t,ctx);};
    const open=(t,ctx,category='',page='conditions')=>{if(category==='hero')ctx.ensureSearch?.();Object.assign(ui(t),{open:true,category,page,query:'',renaming:-1});redraw(t,ctx,true);};
    container.addEventListener('click',event=>{
      const target=event.target.closest('[data-af-open],[data-af-category],[data-af-remove],[data-af-add],[data-af-clear],[data-af-stop],[data-af-continue],[data-af-show-saved],[data-af-page],[data-af-back],[data-af-option],[data-af-not],[data-af-setting],[data-af-sort],[data-af-save],[data-af-apply],[data-af-rename],[data-af-rename-save],[data-af-rename-cancel],[data-af-delete]');if(!target)return;
      const {tab:t,ctx}=current(),m=ui(t),cs=conditions(t),category=m.category,c=clone(draft(t,category)),d=target.dataset;
      if('afOpen'in d){m.open=!m.open;redraw(t,ctx,m.open);}
      else if('afCategory'in d)open(t,ctx,d.afCategory);
      else if('afRemove'in d){const next={...cs};delete next[d.afRemove];t.advancedConditions=next;ctx.change();}
      else if('afAdd'in d)open(t,ctx);
      else if('afShowSaved'in d)open(t,ctx,'','saved');
      else if('afClear'in d){t.advancedConditions={};m.open=false;ctx.change();}
      else if('afStop'in d){ctx.stop();redraw(t,ctx);}
      else if('afContinue'in d)ctx.find();
      else if('afPage'in d){Object.assign(m,{page:d.afPage,category:'',query:'',renaming:-1});redraw(t,ctx);}
      else if('afBack'in d){Object.assign(m,{category:'',query:''});redraw(t,ctx,true);}
      else if('afSort'in d){m.sort=d.afSort;redraw(t,ctx);}
      else if('afOption'in d){c.values=c.values.includes(d.afOption)?c.values.filter(v=>v!==d.afOption):[...c.values,d.afOption];update(t,ctx,category,c);}
      else if('afNot'in d){c.not=d.afNot==='true';if(cs[category])update(t,ctx,category,c);else{(m.drafts ||= {})[category]=c;redraw(t,ctx);}}
      else if('afSetting'in d){c[d.afSetting]=d.afValue;if(cs[category])update(t,ctx,category,c);else{(m.drafts ||= {})[category]=c;redraw(t,ctx);}}
      else if('afRenameSave'in d)renamePreset(t,ctx,Number(d.afRenameSave));
      else if('afRenameCancel'in d){m.renaming=-1;redraw(t,ctx);}
      else if('afSave'in d){const name=container.querySelector('[data-af-name]')?.value;if(!savePreset(ctx.storage,name,cs))ctx.error?.('常用筛选保存失败');else redraw(t,ctx);}
      else {
        const saved=readPresets(ctx.storage),i=Number(d.afApply ?? d.afRename ?? d.afDelete);
        if('afApply'in d){t.advancedConditions=clone(saved[i]?.conditions || {});m.open=false;ctx.change();}
        else if('afDelete'in d){saved.splice(i,1);if(!writePresets(ctx.storage,saved))ctx.error?.('常用筛选保存失败');redraw(t,ctx);}
        else if('afRename'in d){if(!saved[i])return;m.renaming=i;redraw(t,ctx);const input=(ctx.container?.() || container).querySelector('[data-af-rename-input]');input?.focus();input?.select();}
      }
    });
    container.addEventListener('change',event=>{
      const {tab:t,ctx}=current(),m=ui(t),c=clone(draft(t,m.category)),d=event.target.dataset;
      if('afAtleast'in d){c.atLeast=event.target.checked;update(t,ctx,m.category,c);}
      if('afThreshold'in d){c.thresholds ||= {};if(event.target.value==='')delete c.thresholds[d.afThreshold];else c.thresholds[d.afThreshold]=Number(event.target.value);update(t,ctx,'performance',c);}
    });
    container.addEventListener('input',event=>{if(!event.target.matches('[data-af-query]'))return;const {tab:t,ctx}=current();ui(t).query=event.target.value;if(event.target.value)ctx.ensureSearch?.();const body=event.target.closest('.af-menu-body');const position=event.target.selectionStart;body.innerHTML=menuBody(t,ctx);const input=body.querySelector('[data-af-query]');input?.focus();input?.setSelectionRange(position,position);ctx.prepare?.(body);});
    container.addEventListener('keydown',event=>{
      const {tab:t,ctx}=current(),m=ui(t);
      if(event.target.matches('[data-af-rename-input]')){
        if(event.key==='Enter'){event.preventDefault();renamePreset(t,ctx,Number(event.target.dataset.afRenameInput));}
        else if(event.key==='Escape'){event.preventDefault();event.stopPropagation();m.renaming=-1;redraw(t,ctx);}
        return;
      }
      if(event.key==='Escape' && m.open){m.open=false;redraw(t,ctx);container.querySelector('[data-af-open]')?.focus();}
      if(event.key==='Enter' && event.target.matches('[data-af-query]')){event.preventDefault();const category=m.category || Object.keys(categories).find(k=>categories[k].includes(m.query.trim()) && m.query.trim()) || Object.keys(categories).find(k=>options(ctx.rows(),k,ctx,conditions(t)[k]).some(o=>searchOption(o,k,m.query.toLowerCase(),ctx)));if(category){const option=options(ctx.rows(),category,ctx,conditions(t)[category]).find(o=>searchOption(o,category,m.query.toLowerCase(),ctx));if(option){const c=clone(conditions(t)[category] || {values:[],not:false});c.values=[...new Set([...c.values,option.value])];update(t,ctx,category,c);}else open(t,ctx,category);}}
    });
    const doc=container.ownerDocument;
    doc._afTarget={container,tab,context};
    if(!doc._afGlobal){doc._afGlobal=true;
      doc.addEventListener('pointerdown',event=>{const target=doc._afTarget;if(!target?.container.isConnected || !ui(target.tab).open || target.container.contains(event.target) && event.target.closest('[data-af-root],[data-af-conditions]'))return;ui(target.tab).open=false;const menu=target.container.querySelector('[data-af-menu]');if(menu)menu.hidden=true;target.container.querySelector('[data-af-open]')?.setAttribute('aria-expanded','false');});
      doc.addEventListener('keydown',event=>{const target=doc._afTarget;if(event.key!=='/' || event.ctrlKey || event.metaKey || event.altKey || /INPUT|TEXTAREA|SELECT/.test(event.target.tagName) || event.target.isContentEditable || !target?.container.isConnected)return;event.preventDefault();open(target.tab,target.context());});
    }
  }
  function decorate(container,tab,ctx) {
    const cs=conditions(tab);
    if(!container || !active(tab) && !container._afDecorated)return;
    const rows=new Map(ctx.rows().map(row=>[String(row.gameId),row]));
    container._afDecorated=active(tab);
    for(const entry of container?.querySelectorAll('.match-entry') || []) {
      entry.querySelectorAll('[data-af-created]').forEach(node=>node.remove());
      entry.querySelectorAll('.af-hit').forEach(node=>node.classList.remove('af-hit'));
      const match=rows.get(entry.dataset.matchId);if(!match)continue;const subject=ctx.subject(match);
      const multi=cs.multikill;if(multi && !multi.not)for(const value of multi.values)if(multikill(subject,Number(value),multi.atLeast)) {
        let nodes=[...entry.querySelectorAll('[data-af-multikill]')].filter(node=>multi.atLeast ? Number(node.dataset.afMultikill)>=Number(value) : node.dataset.afMultikill===value);
        if(!nodes.length && ctx.multiTag) {
          const levels=[3,4,5].filter(level=>(multi.atLeast?level>=Number(value):level===Number(value)) && multikill(subject,level));
          const badge=entry.querySelector('.match-badges');let collection=badge?.querySelector('.match-tags-collection');
          if(badge && !collection){collection=entry.ownerDocument.createElement('span');collection.className='match-tags-collection';collection.setAttribute('data-af-created','');badge.append(collection);}
          if(collection)for(const level of levels){const span=entry.ownerDocument.createElement('span');span.setAttribute('data-af-created','');span.setAttribute('data-match-tag','');span.innerHTML=ctx.multiTag(level,match.gameId);collection.prepend(span);nodes.push(...span.querySelectorAll('[data-af-multikill]'));}
        }
        nodes.forEach(node=>node.classList.add('af-hit'));
      }
      const p=cs.performance;if(p && !p.not && matchesCategory('performance',p,match,ctx)) {
        for(const value of p.values)if(matchesValue('performance',value,p,match,ctx)) {
          if(value==='top3' && !entry.querySelector('.match-badge-chip')) {const collection=entry.querySelector('.match-tags-collection');if(collection){const span=entry.ownerDocument.createElement('span');span.setAttribute('data-match-tag','');span.setAttribute('data-af-created','');span.innerHTML='<b class="match-badge-chip">前三</b>';collection.prepend(span);}}
          const selector=value.startsWith('tag:')?`.is-data-${value.slice(4)}`:value==='zero'?'.match-zero-deaths':'.match-badge-chip';entry.querySelectorAll(selector).forEach(node=>node.classList.add('af-hit'));
        }
        if(p.thresholds?.score!=null)entry.querySelector('.match-score')?.classList.add('af-hit');
        if(p.thresholds?.kda!=null)entry.querySelector('.match-kda small')?.classList.add('af-hit');
        if(p.thresholds?.kp!=null)entry.querySelector('.is-data-participation')?.classList.add('af-hit');
      }
    }
  }
  return Object.freeze({categories,normalize,active,filter,options,multikill,participation,readPresets,savePreset,cancel,find,render,renderConditions,bind,decorate,empty,footer,counts});
});
