(() => {
 "use strict";
 const {state,renderLive,escapeHTML,renderItemIcon,renderRuneSpellIcons,renderUnifiedRuneBoard,runeConfigurationTitle,nodes,selectedRuneRecommendation,recommendedRuneSpellIDs,bindLiveNode,renderRecommendationArea,rememberLiveRecommendationMarkup,api,showToast}=window.deepLegendsGameAutoConfig;
 const labels={ranked:"排位",normal:"匹配",aram:"极地大乱斗",hextech:"海克斯大乱斗",urf:"无限火力",event:"活动模式"};
 const positions={top:"上路",jungle:"打野",middle:"中路",bottom:"下路",utility:"辅助"};
 function mode(data) {const q=Number(data?.queueId),m=String(data?.gameMode||"").toUpperCase();if([1700,1710,1750].includes(q)||m==="CHERRY")return "";if([2300,2400,3270].includes(q)||["KIWI","ARAM_MAYHEM"].includes(m))return "hextech";if([420,440].includes(q))return "ranked";if([450,930,3220,720].includes(q)||m==="ARAM")return "aram";if([900,1900].includes(q)||m==="URF")return "urf";if([1300,1400].includes(q)||["ULTBOOK","NEXUSBLITZ"].includes(m))return "event";if([400,430,480,490,700,820,830,840,850,860,870,880,890,950,960,3100].includes(q)||["CLASSIC","PRACTICETOOL"].includes(m)&&[0,11].includes(Number(data?.mapId)||0)||!q&&!m&&Number(data?.mapId)===11)return "normal";return "";}
 function resolveModeKey(all,m,position) {const keys=m==="ranked"?["ranked-"+position,"ranked"]:m==="normal"?["normal","ranked-"+position,"ranked"]:[m];return keys.find(k=>all?.[k])||m;}
 function selection(data) {
  const self=data?.players?.find(p=>p.isCurrent),id=Number(self?.championId)||Number(self?.championPickIntent)||Number(data?.currentChampionId),m=mode(data),all=state.autoConfigSettings?.champions?.[id];
  const position=({mid:"middle",adc:"bottom",support:"utility"})[self?.position]||self?.position;
  if(!m||!state.autoConfigSettings?.enabled)return null;
  const key=resolveModeKey(all,m,position),saved=all?.[key],settings=state.autoConfigSettings;
  const entry=saved?{...saved,runes:settings.runes?saved.runes:null,spells:settings.spells?saved.spells:null,items:settings.items?saved.items:null}:null;
  if(!entry||!entry.runes&&!entry.spells&&!entry.items?.starter?.length&&!entry.items?.core?.length&&!entry.items?.situational?.length)return null;
  return {key:"custom",sourceLabel:"自配置",championId:id,modeKey:key,modeLabel:(labels[key.startsWith("ranked")?"ranked":key]||"")+(key.startsWith("ranked")?" · "+(positions[key.slice(7)]||"通用"):""),entry,...entry.runes,...entry.spells,selectedComplete:entry.runes?.selectedPerkIds?.length===9};
 }
 function prepare(data) {const c=selection(data),self=data?.players?.find(p=>p.isCurrent),key=`${data?.gameId}:${Number(self?.championId)||Number(self?.championPickIntent)||0}:${mode(data)}:${c?.modeKey||""}:${!!c}`;if(key!==state.autoConfigViewKey){state.autoConfigViewKey=key;state.runeSourceTab=c?"custom":"opgg";state.selectedRecommendation=c?"custom":"opgg";}return c;}
 function update(next){state.autoConfig=next;if(state.section==="live")renderLive("sse");}
 let flight=null;
 async function refresh(){if(flight)return flight;flight=(async()=>{await Promise.all([fetch("/api/auto-config/state").then(async r=>{if(r.ok)state.autoConfig=await r.json();}),fetch("/api/auto-config/settings").then(async r=>{if(r.ok)state.autoConfigSettings=await r.json();}),fetch("/api/settings/summoner-keys").then(async r=>{if(r.ok)state.summonerKeys=await r.json();})].map(p=>p.catch(()=>{})));if(state.section==="live")renderLive("settings");})().finally(()=>flight=null);return flight;}
 let shifts={total:0,max:0,count:0},shiftTimer=null;
 const flushShifts=()=>{if(shifts.count)window.reportFlowDiagnostic?.("live_layout_shift","aggregated",{layoutShiftTotal:shifts.total,layoutShiftMax:shifts.max,layoutShiftCount:shifts.count});shifts={total:0,max:0,count:0};shiftTimer=null;};
 let shiftObserver;
 if(typeof PerformanceObserver==="function"&&PerformanceObserver.supportedEntryTypes?.includes("layout-shift")){
  shiftObserver=new PerformanceObserver(list=>{if(state.section!=="live")return;for(const entry of list.getEntries()){if(!Number.isFinite(entry.value))continue;shifts.total+=entry.value;shifts.max=Math.max(shifts.max,entry.value);shifts.count++;}if(shifts.count&&!shiftTimer)shiftTimer=setTimeout(flushShifts,60000);});
  shiftObserver.observe({type:"layout-shift",buffered:false});
 }
 window.addEventListener("pagehide",flushShifts);
 window.addEventListener("deep-legends:dispose",()=>{shiftObserver?.disconnect();clearTimeout(shiftTimer);flushShifts();},{once:true});
  function renderCustomConfiguration(config) {
    const settings=state.autoConfigSettings;if(!settings?.enabled)return "";
    const entry={...config.entry,runes:settings.runes?config.entry.runes:null,spells:settings.spells?config.entry.spells:null,items:settings.items?config.entry.items:null},auto=state.autoConfig?.championId===config.championId?state.autoConfig:null;
    if(!entry.runes&&!entry.spells&&!["starter","core","situational"].some(k=>entry.items?.[k]?.length))return "";
    const status=auto?.yielded?.runes||auto?.yielded?.spells?"已手动应用其他推荐，本局不自动应用":Object.values(auto?.results||{}).includes("applied")?"已应用到客户端":"锁定后自动应用";
    const groups=[["出门装",entry.items?.starter||[]],["最终装备",(entry.items?.core||[]).map(id=>({id,count:1}))]].filter(([,items])=>items.length);
    const items=groups.map(([label,ids])=>`<div class="sg-items"><span class="sg-label">${label}</span><div class="sg-icons">${ids.map(item=>`<span class="custom-config-item">${renderItemIcon(item.id)}${item.count>1?`<b>×${item.count}</b>`:""}</span>`).join("")}</div></div>`).join("");
    const spells=entry.spells?renderRuneSpellIcons(entry.spells):"";
    return `<article class="specialist-game-row custom-config-card is-selected" data-custom-config><header class="custom-config-header"><strong>${escapeHTML(entry.runes?runeConfigurationTitle(config):"自配置")}</strong><span class="cs-live-pill">${escapeHTML(config.modeLabel)}</span><small role="status">${status}</small></header>${entry.runes?renderUnifiedRuneBoard(config,spells):spells?`<div class="sg-spells"><span data-card-spells>${spells}</span></div>`:""}${items?`<div class="specialist-game-footer">${items}</div>`:""}</article>`;
  }
  function updateLiveRuneSelection(root){
    const panel=nodes.liveContent?.querySelector("#recommendation-panel-runes")||root;
    for(const node of panel?.querySelectorAll("[data-rune-choice]")||[]){const picked=node.dataset.runeChoice===state.selectedRecommendation;node.classList.toggle("is-selected",picked);node.setAttribute("aria-checked",String(picked));}
    const selected=selectedRuneRecommendation(state.live),complete=selected?.sourceLabel==="自配置"||!!selected&&selected.selectedComplete!==false&&(selected.selectedComplete!==true||selected.selectedPerkIds?.length===9)&&(selected.selectedPerkIds||selected.perkIds||[]).length>=6;
    const footer=panel?.querySelector(".recommendation-action");if(footer){const title=footer.querySelector("strong"),copy=title?.nextElementSibling,button=footer.querySelector("[data-apply-runes]");if(title)title.textContent=selected?"已选择："+runeConfigurationTitle(selected):"请选择一套完整符文";if(copy)copy.textContent=complete?"将更新 [DL] 符文页并设为当前页":"这套数据尚不完整，暂时不能应用";if(button){button.disabled=!complete||state.live?.phase!=="ChampSelect";button.textContent=state.live?.phase==="ChampSelect"?"应用所选符文":"仅英雄选择阶段可应用";}let toggle=footer.querySelector(".rune-spell-toggle");const pair=selected?.sourceLabel==="自配置"?[]:recommendedRuneSpellIDs(state.live,selected);if(!toggle&&pair.length){toggle=document.createElement("label");toggle.className="rune-spell-toggle";const input=document.createElement("input");input.type="checkbox";input.dataset.applyRuneSpells="";input.checked=state.applyRuneSpells!==false;toggle.append(input,document.createTextNode("同时应用召唤师技能"));footer.querySelector("div").append(toggle);bindLiveNode(input,"change","data-apply-rune-spells",event=>{state.applyRuneSpells=event.currentTarget.checked;});}if(toggle)toggle.hidden=!pair.length;}
    if(panel&&state.live){const snapshot=document.createElement("div");snapshot.innerHTML=renderRecommendationArea(state.live);const expected=snapshot.querySelector("#recommendation-panel-runes");if(expected){panel._livePanelMarkup=expected.innerHTML;const rows=new Map([...expected.querySelectorAll("[data-rune-choice]")].map(n=>[n.dataset.runeChoice,n.outerHTML]));for(const row of panel.querySelectorAll("[data-rune-choice]")){const html=rows.get(row.dataset.runeChoice);if(html)row._liveMarkup=html;}}}
    rememberLiveRecommendationMarkup();
  }
  async function applyCustomConfiguration(button,config){button.disabled=true;button.textContent="正在应用…";try{const result=await api("/api/auto-config/apply",{method:"POST",body:JSON.stringify({championId:config.championId})},"apply-runes",15000);showToast(Object.values(result.results||{}).includes("failed")?"部分配置未能应用，请重试":Object.values(result.results||{}).includes("applied")?"自配置已应用到客户端":"配置尚不完整或已取消，未写入客户端");renderLive("selection");}catch(error){showToast(error.message);}finally{button.disabled=false;button.textContent="应用所选配置";}}

 // Move the same pair once, using the title row's actual intrinsic widths.
 const layoutRows=new Map(),dirtyRows=new Set(),widthChecks=new Set(),targetRows=new WeakMap(),targetWidths=new WeakMap();let spellFrame=0,spellDisposed=false;
 const spellWidthSignature=({head,pair,footer})=>[head.clientWidth,footer?.clientWidth,pair.scrollWidth,...[...head.querySelectorAll("*")].filter(n=>n!==pair&&!pair.contains(n)).map(n=>n.scrollWidth)].join(":");
 function scheduleCardSpells(row,widthOnly=false){if(spellDisposed)return;(widthOnly?widthChecks:dirtyRows).add(row);if(!spellFrame)spellFrame=requestAnimationFrame(()=>{spellFrame=0;const rows=new Set([...dirtyRows,...widthChecks]);for(const row of rows){const config=layoutRows.get(row);if(config&&(dirtyRows.has(row)||config.widthSignature!==spellWidthSignature(config)))layoutCardSpells(row);}dirtyRows.clear();widthChecks.clear();});}
 const layoutObserver=typeof ResizeObserver==="function"?new ResizeObserver(entries=>{for(const entry of entries){const row=targetRows.get(entry.target),width=entry.target.clientWidth,previous=targetWidths.get(entry.target);targetWidths.set(entry.target,width);if(row&&width!==previous)scheduleCardSpells(row);}}):null;
 function forgetSpellRow(row){const old=layoutRows.get(row);for(const target of old?.targets||[]){layoutObserver?.unobserve(target);targetRows.delete(target);targetWidths.delete(target);}layoutRows.delete(row);dirtyRows.delete(row);widthChecks.delete(row);}
 function syncSpellRows(){
  for(const row of layoutRows.keys())if(!row.isConnected)forgetSpellRow(row);
  for(const row of document.querySelectorAll(".specialist-game-row")){
   const head=row.querySelector(".specialist-game-head,.custom-config-header"),pair=row.querySelector("[data-card-spells]"),fallback=row.querySelector(".rune-corner-spells,.sg-spells"),old=layoutRows.get(row);
   if(!head||!pair||!fallback){if(old)forgetSpellRow(row);continue;}
   const footer=row.querySelector(".specialist-game-footer"),targets=[row,...head.children,...(footer?[footer,...footer.children]:[])].filter(n=>n!==pair);
   if(old?.head===head&&old.pair===pair&&old.fallback===fallback&&old.footer===footer&&old.targets.length===targets.length&&targets.every((n,i)=>old.targets[i]===n))continue;
   if(old)forgetSpellRow(row);layoutRows.set(row,{head,pair,fallback,footer,targets});
   // Header children also expose intrinsic title/status width changes without
   // watching unrelated live-page text updates or rereading every card's style.
   for(const target of targets){targetRows.set(target,row);targetWidths.set(target,target.clientWidth);layoutObserver?.observe(target);}
   scheduleCardSpells(row);
  }
 }
 function layoutCardSpells(row){
   const config=layoutRows.get(row);if(!row.isConnected||!config)return;
   const {head,pair,fallback,footer}=config;
   const style=getComputedStyle(head),gap=parseFloat(style.columnGap)||0,padding=(parseFloat(style.paddingLeft)||0)+(parseFloat(style.paddingRight)||0);
   const children=[...head.children].filter(n=>n!==pair),previous=head.style.gridTemplateColumns;
   if(head.classList.contains("specialist-game-head"))head.style.gridTemplateColumns="max-content max-content max-content";
   const needed=children.reduce((w,n)=>w+n.scrollWidth,0)+pair.scrollWidth+gap*children.length+padding;head.style.gridTemplateColumns=previous;
   const wide=needed<=head.clientWidth;
   let bottom=false;
   if(footer){
     const order=footer.querySelector(".skill-order");if(order)order.style.setProperty("--card-skill-count",String(order.children.length));
     const css=getComputedStyle(footer),pad=parseFloat(css.paddingLeft)+parseFloat(css.paddingRight),children=[...footer.children].filter(n=>n!==pair);
     // Use intrinsic equipment widths and the complete skill grid at its usual cell width.
     const footerNeeded=children.reduce((sum,n)=>{const order=n.querySelector(".skill-order");return sum+(order?order.children.length*30+(order.children.length-1)*3+[...n.children].filter(c=>c!==order).reduce((w,c)=>w+c.scrollWidth,0)+(parseFloat(getComputedStyle(n).columnGap)||0)*(n.children.length-1)+(parseFloat(getComputedStyle(n).paddingLeft)||0)+(parseFloat(getComputedStyle(n).paddingRight)||0):n.scrollWidth);},0)+pair.scrollWidth+(parseFloat(css.columnGap)||0)*children.length+pad;
     bottom=children.length>0&&footerNeeded<=footer.clientWidth;
   }
   if(bottom){if(pair.parentElement!==footer)footer.append(pair);}
   else if(wide){const status=head.querySelector('[role="status"]');if(pair.parentElement!==head)head.insertBefore(pair,status||null);}else if(pair.parentElement!==fallback)fallback.append(pair);
   row.dataset.spellLayout=bottom?"footer":wide?"head":"board";
   config.widthSignature=spellWidthSignature(config);
 }
 const spellStructure=".specialist-game-row,.specialist-game-head,.custom-config-header,.rune-corner-spells,.sg-spells,.specialist-game-footer,[data-card-spells]";
 const spellMutation=new MutationObserver(records=>{let structure=false;for(const record of records){const pair=record.target.closest?.("[data-card-spells]"),head=record.target.closest?.(".specialist-game-head,.custom-config-header,.specialist-game-footer"),row=(pair||head)?.closest(".specialist-game-row");if(layoutRows.has(row))scheduleCardSpells(row,!pair);if([...record.addedNodes,...record.removedNodes].some(n=>n.nodeType===1&&(n.matches(spellStructure)||n.querySelector(spellStructure))))structure=true;}if(structure)syncSpellRows();});spellMutation.observe(nodes?.liveContent||document.body,{childList:true,subtree:true});
 const checkSpellWidths=()=>{if(layoutObserver)return;for(const config of layoutRows.values())for(const target of config.targets){const width=target.clientWidth;if(width!==targetWidths.get(target)){targetWidths.set(target,width);scheduleCardSpells(targetRows.get(target));}}};
 window.addEventListener("resize",checkSpellWidths);
 window.addEventListener("deep-legends:dispose",()=>{spellDisposed=true;cancelAnimationFrame(spellFrame);dirtyRows.clear();widthChecks.clear();layoutRows.clear();layoutObserver?.disconnect();spellMutation.disconnect();window.removeEventListener("resize",checkSpellWidths);},{once:true});
 syncSpellRows();
 window.deepLegendsAutoConfigLive={selection,prepare,renderCustomConfiguration,updateLiveRuneSelection,applyCustomConfiguration};
 window.addEventListener("deep-legends:auto-config",e=>update(e.detail||{}));
 window.addEventListener("deep-legends:auto-config-settings",e=>{state.autoConfigSettings=e.detail;state.autoConfigViewKey="";prepare(state.live);if(state.section==="live")renderLive("settings");});
 for(const name of ["deep-legends:resync","deep-legends:settings-imported"])window.addEventListener(name,()=>void refresh());
 window.addEventListener("deep-legends:section",e=>{if(e.detail?.name==="live")void refresh();});
 if(window.deepLegendsAutoConfigState)state.autoConfig=window.deepLegendsAutoConfigState;
 window.deepLegendsSections.register("auto-config-live",()=>void refresh());
})();
