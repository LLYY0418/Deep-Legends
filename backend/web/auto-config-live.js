(() => {
 "use strict";
 const {state,renderLive,iconFigure}=window.deepLegendsGameAutoConfig;
 const escapeHTML=window.deepLegendsRuntime.escapeHTML;
 function modeLabel(key) {const [mode,position]=String(key||"").split("-");return ({ranked:"排位",normal:"匹配",aram:"极地大乱斗",hextech:"海克斯大乱斗",urf:"无限火力",event:"活动模式"})[mode]+(position?" · "+({top:"上路",jungle:"打野",middle:"中路",bottom:"下路",utility:"辅助"})[position]:"");}
 function banner(auto,keySettings,open) {
 const pair=auto.results.spells === "applied" ? [auto.spellPair.spell1Id,auto.spellPair.spell2Id] : [];
 const keys=keySettings?.keys || [];
 const banner = `<button type="button" class="ac-applied" data-auto-config-toggle aria-expanded="${Boolean(open)}"><span class="txt"><strong>已使用自定义符文${pair.length ? "和召唤师技能" : ""}配置，推荐符文已收起</strong><small>${escapeHTML(auto.pageName)} · ${escapeHTML(modeLabel(auto.modeKey))}</small></span><span class="mini">${pair.map((id,i)=>`<span>${iconFigure("spell",id,"召唤师技能","small",false)}<kbd>${escapeHTML(keys[i]||"")}</kbd></span>`).join("")}</span><span class="more">${open ? "收起推荐 ⌃" : "展开推荐 ⌄"}</span></button>`;
 return banner;
 }
 function update(next) {
 const previous=state.autoConfig;
 if(next.session!==previous?.session || next.championId!==previous?.championId) {state.autoConfigOpen=false;state.autoConfigSelected=false;}
 state.autoConfig=next;
 if(state.section==="live")renderLive("sse");
 }
 async function refresh() {
 try {const r=await fetch("/api/auto-config/state");if(r.ok)update(await r.json());}catch(_){}
 try {const r=await fetch("/api/settings/summoner-keys");if(r.ok){state.summonerKeys=await r.json();if(state.section==="live")renderLive("settings");}}catch(_){}
 }
 window.deepLegendsAutoConfigLive={banner};
 window.addEventListener("deep-legends:auto-config",e=>update(e.detail||{}));
 window.addEventListener("deep-legends:resync",()=>void refresh());
 window.addEventListener("deep-legends:settings-imported",()=>void refresh());
 window.addEventListener("deep-legends:section",e=>{if(e.detail?.name==="live")void refresh();});
 if(window.deepLegendsAutoConfigState)update(window.deepLegendsAutoConfigState);
 window.deepLegendsSections.register("auto-config-live",()=>void refresh());
})();
