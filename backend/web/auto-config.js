(() => {
  "use strict";
  const MODES = [["ranked","排位","单双排 · 灵活组排"],["normal","匹配","含快速 · 人机 · 自定义"],["aram","极地大乱斗","含极地冠军杯"],["urf","无限火力","无限火力 · 无限乱斗"],["event","活动模式","极限闪击 · 终极魔典等"]];
  const POS = [["","通用"],["top","上路"],["jungle","打野"],["middle","中路"],["bottom","下路"],["utility","辅助"]];
  const PATH = {8000:"precision",8100:"domination",8200:"sorcery",8400:"resolve",8300:"inspiration"};
  const STYLE_ORDER = [8000,8100,8200,8400,8300];
  const ATTR = [["Boots","鞋子"],["Damage","攻击力"],["CriticalStrike","暴击"],["AttackSpeed","攻击速度"],["OnHit","攻击特效"],["ArmorPenetration","护甲穿透"],["SpellDamage","法术强度"],["Mana|ManaRegen","法力值 & 回复"],["MagicPenetration","法术穿透"],["Health|HealthRegen","生命值 & 回复"],["Armor","护甲"],["SpellBlock","魔法抗性"],["AbilityHaste|CooldownReduction","技能急速"],["NonbootsMovement|Boots","移动速度"],["LifeSteal|SpellVamp|Omnivamp","生命偷取 & 吸血"]];
  function spellAllowed(s,mode,gameMode) {
    if(mode==="hextech"&&s.id===3)return false;
    if(["aram","hextech"].includes(mode)&&[11,12].includes(s.id))return false;
    const want=mode==="event"?String(gameMode||"").toUpperCase():({ranked:"CLASSIC",normal:"CLASSIC",aram:"ARAM",hextech:"KIWI",urf:"URF"})[mode];
    return (s.gameModes||[]).some(m=>m.toUpperCase()===want || mode==="hextech"&&m.toUpperCase()==="ARAM_MAYHEM");
  }
  function itemAllowed(i,mode) {
    if(!i.inStore||!i.displayInItemSets||i.requiredChampion||i.requiredAlly||!i.maps?.[["aram","hextech"].includes(mode)?"12":"11"])return false;
    if((i.categories||[]).some(c=>["Consumable","Trinket"].includes(c)))return false;
    return (i.categories?.includes("Boots")&&i.from?.includes(1001))||(!i.to?.length&&i.from?.length>0&&!i.categories?.some(c=>["Lane","Jungle"].includes(c)));
  }
  // R300: official mode-specific amounts could not be verified; use map defaults.
  // See backend/auto_config_catalog.go for the dated source check.
  const starterBudget=mode=>["aram","hextech"].includes(mode)?1400:500;
  const consumable=i=>i?.categories?.includes("Consumable");
  const trinket=i=>i?.categories?.includes("Trinket");
  // Client shop at level one sells only Warding Totem and Oracle Lens (3340/3364).
  const starterTrinket=i=>!trinket(i)||[3340,3364].includes(i.id);
  function normalizeStarterTrinkets(value,catalog){let seen=false;const rows=value?.items?.starter;if(!rows||!catalog.items?.length)return false;const next=rows.filter(row=>{const item=catalog.items.find(i=>i.id===row.id);if(!trinket(item))return true;if(seen||!starterTrinket(item))return false;seen=true;return true;}).map(row=>trinket(catalog.items.find(i=>i.id===row.id))?{...row,count:1}:row);const changed=JSON.stringify(rows)!==JSON.stringify(next);if(changed)value.items.starter=next;return changed;}
  function starterAllowed(i,mode){return !!(starterTrinket(i)&&i.inStore&&i.displayInItemSets&&!i.requiredChampion&&!i.requiredAlly&&i.maps?.[["aram","hextech"].includes(mode)?"12":"11"]&&i.priceTotal>=0&&i.priceTotal<=starterBudget(mode));}
  function championImageURL(c){return c.imagePath?`/api/champion-asset?source=${encodeURIComponent(c.imageSource||"lcu")}&path=${encodeURIComponent(c.imagePath)}`:"/image-unavailable.svg";}
  function selectSecondary(order,row,id,picks) {
    if(!picks[row]){order.push(row);if(order.length>2)delete picks[order.shift()];}
    picks[row]=id;
  }
  function draftContent(value){return JSON.stringify({runes:value?.runes||null,spells:value?.spells||null,items:value?.items&&["starter","core","situational"].some(k=>value.items[k]?.length)?{starter:value.items.starter||[],core:value.items.core||[],situational:value.items.situational||[]}:null});}
  function validEditorDraft(value,catalog,mode){
    if(!value||typeof value!=="object"||Array.isArray(value)||Object.keys(value).some(k=>!["runes","spells","items","updatedAt"].includes(k)))return false;
    const r=value.runes;
    if(r){const primary=catalog.styles.find(s=>s.id===r.primaryStyleId),sub=catalog.styles.find(s=>s.id===r.subStyleId),ids=r.selectedPerkIds;
      if(mode==="hextech"||!primary||!sub||primary===sub||!Array.isArray(ids)||ids.length!==9||ids.some(id=>!Number.isInteger(id)||id<0))return false;
      if(ids.slice(0,4).some((id,i)=>id&&!primary.slots[i]?.perks.some(p=>p.id===id))||ids.slice(6).some((id,i)=>id&&!catalog.statModSlots[i]?.perks.some(p=>p.id===id)))return false;
      const rows=ids.slice(4,6).filter(Boolean).map(id=>sub.slots.findIndex((slot,i)=>i>0&&slot.perks.some(p=>p.id===id)));if(rows.some(i=>i<1)||new Set(rows).size!==rows.length)return false;
    }
    const pair=value.spells;if(pair&&(!Number.isInteger(pair.spell1Id)||!Number.isInteger(pair.spell2Id)||[pair.spell1Id,pair.spell2Id].some(id=>id!==0&&!catalog.spells.some(s=>s.id===id&&spellAllowed(s,mode,catalog.eventGameMode)))||pair.spell1Id&&pair.spell1Id===pair.spell2Id))return false;
    const items=value.items;if(items){if(typeof items!=="object"||Array.isArray(items)||Object.keys(items).some(k=>!["starter","core","situational"].includes(k)))return false;
      const all=[];let total=0;
      for(const group of ["starter","core","situational"]){const rows=items[group]||[];if(!Array.isArray(rows)||rows.length>6)return false;for(const row of rows){const id=group==="starter"?row?.id:row,item=catalog.items.find(i=>i.id===id);if(!Number.isInteger(id)||!item||!(group==="starter"?starterAllowed(item,mode):itemAllowed(item,mode)))return false;if(group==="starter"){if(!Number.isInteger(row.count)||row.count<1||row.count>99||!consumable(item)&&row.count!==1)return false;total+=item.priceTotal*row.count;}all.push(group==="starter"?"s"+id:id);}}
      if(new Set(all).size!==all.length||total>starterBudget(mode))return false;
    }
    return !!(r||pair||items)||Object.keys(value).every(k=>k==="updatedAt");
  }
  if(typeof module==="object"&&module.exports){module.exports={spellAllowed,itemAllowed,starterAllowed,starterBudget,championImageURL,selectSecondary,validEditorDraft,draftContent,normalizeStarterTrinkets};return;}
  const root=document.getElementById("suite-config-root");if(!root)return;
  const esc=window.deepLegendsRuntime.escapeHTML;
  const S={champ:0,mode:"ranked",pos:"",tab:"runes",draft:null,dirty:false,settings:null,catalog:null,champions:[],runtime:{},keys:{keys:["",""]},slot:0,itemGroup:"starter",budgetError:false,filters:new Set(),q:"",iq:"",configured:false,sort:"price",mastery:{},subOrder:[],subPicks:{},pending:null,error:"",watch:{},generation:0,importing:false,feedback:"",catalogLoading:false,spellEditing:false,itemEditing:false,clearing:false};
  const clone=v=>JSON.parse(JSON.stringify(v));
  const modeKey=()=>S.mode==="ranked"&&S.pos?`ranked-${S.pos}`:S.mode;
  const stored=()=>S.settings?.champions?.[S.champ]?.[modeKey()];
  const champ=()=>S.champions.find(c=>c.id===S.champ)||{};
  const modeName=()=>MODES.find(m=>m[0]===S.mode)?.[1]+(S.mode==="ranked"?` · ${POS.find(p=>p[0]===S.pos)?.[1]}`:"");
  const style=id=>S.catalog?.styles.find(s=>s.id===id);
  const icon=(p,name="")=>`<img src="${esc(p?.startsWith("ddragon:")?`/api/champion-asset?source=ddragon&path=${encodeURIComponent(p.slice(8))}`:p?`/api/image?path=${encodeURIComponent(p)}`:"/image-unavailable.svg")}" alt="${esc(name)}" decoding="async">`;
  const styleIcon=s=>PATH[s?.id]?`<img src="/rune-styles/${PATH[s.id]}.svg" alt="${esc(s.name)}" decoding="async">`:icon(s?.iconPath,s?.name);
  const portrait=c=>`<img src="${esc(championImageURL(c))}" alt="${esc(c.nameZh||c.nameZH||c.name||"")}" width="36" height="36" decoding="async">`;
  const title=c=>c.titleZh||c.titleZH||c.nameZh||c.nameZH||c.name||String(c.id||"");
  const subtitle=c=>c.nameZh||c.nameZH||"";
  const perk=id=>S.catalog?.perks.find(p=>p.id===id)||S.catalog?.styles.flatMap(s=>s.slots.flatMap(slot=>slot.perks)).find(p=>p.id===id)||S.catalog?.statModSlots.flatMap(s=>s.perks).find(p=>p.id===id);
  function description(value) {
    const template=document.createElement("template");
    template.innerHTML=String(value||"").replace(/<br\s*\/?\s*>/gi,"\n").replace(/<\/?(?:p|div|li)\b[^>]*>/gi,"\n");
    template.content.querySelectorAll("script,style,iframe,object").forEach(n=>n.remove());
    return template.content.textContent.replace(/\n[ \t]+/g,"\n").replace(/\n{3,}/g,"\n\n").trim();
  }
  const tip=p=>`${p?.name||"已失效"}\n${description(p?.longDesc||p?.longDescription||p?.description||p?.shortDesc||p?.shortDescription)}`;
  async function api(path,method="GET",body,signal){const r=await fetch(path,{method,signal,headers:{"Content-Type":"application/json"},...(body===undefined?{}:{body:JSON.stringify(body)})});if(!r.ok)throw Error(await r.text());return r.json();}
  function fresh(){const ids=S.catalog?.styles.map(s=>s.id)||[8100,8200];return { ...(S.mode==="hextech"?{}:{runes:{primaryStyleId:ids[0],subStyleId:ids.find(id=>id!==ids[0]),selectedPerkIds:Array(9).fill(0)}})};}
  const draftPrefix="lol-loot-auto-config-draft-",draftIndex=draftPrefix+"index";
  const preferences=()=>window.deepLegendsPreferenceStorage||localStorage;
  const draftKey=(id=S.champ,key=modeKey())=>draftPrefix+id+"-"+key;
  let draftTimer=0,lastContent="",draftBase=draftContent(null);
  function draftRows(){try{const rows=JSON.parse(preferences().getItem(draftIndex)||"[]");return Array.isArray(rows)?rows.filter(r=>r&&typeof r.key==="string"&&/^lol-loot-auto-config-draft-\d+-(?:ranked(?:-(?:top|jungle|middle|bottom|utility))?|normal|aram|hextech|urf|event)$/.test(r.key)&&Number.isFinite(r.at)):[];}catch(_){return [];}}
  function removeDraft(id=S.champ,key=modeKey()){const name=draftKey(id,key);preferences().removeItem(name);preferences().setItem(draftIndex,JSON.stringify(draftRows().filter(r=>r.key!==name)));}
  function flushDraft(){clearTimeout(draftTimer);draftTimer=0;if(!S.dirty||!S.champ)return;const key=draftKey(),at=Date.now();preferences().setItem(key,JSON.stringify({championId:S.champ,modeKey:modeKey(),baseContent:draftBase,at,value:S.draft}));const rows=draftRows().filter(r=>r.key!==key);rows.push({key,at});rows.sort((a,b)=>a.at-b.at);while(rows.length>50)preferences().removeItem(rows.shift().key);preferences().setItem(draftIndex,JSON.stringify(rows));}
  function loadDraft(){
    clearTimeout(draftTimer);draftTimer=0;S.draft=stored()?clone(stored()):null;S.dirty=false;S.error="";S.feedback="";draftBase=draftContent(stored());
    try{const raw=preferences().getItem(draftKey());if(raw){const row=JSON.parse(raw);normalizeStarterTrinkets(row.value,S.catalog);if(row.championId!==S.champ||row.modeKey!==modeKey()||!S.champions.some(c=>c.id===row.championId)||!(typeof row.baseContent==="string"||Number.isSafeInteger(row.revision)&&row.revision>=1)||!Number.isFinite(row.at)||!validEditorDraft(row.value,S.catalog,S.mode))removeDraft();else if(draftContent(row.value)!==draftContent(stored())){S.draft=clone(row.value);S.dirty=true;draftBase=typeof row.baseContent==="string"?row.baseContent:draftContent(stored());if(draftBase!==draftContent(stored()))S.error="配置已更新，请重新读取后保存";}else removeDraft();}}catch(_){removeDraft();}
    S.spellEditing=!!S.draft?.spells;S.itemEditing=hasSection(S.draft,"items");lastContent=draftContent(S.draft);if(normalizeStarterTrinkets(S.draft,S.catalog))dirty(false);S.itemGroup="starter";S.budgetError=false;readSecondary();if(S.mode==="hextech")S.tab="kit";
  }
  let pendingDialog=null,importController=null;
  function cancelImport(){S.generation++;importController?.abort();importController=null;S.importing=false;importStatus();}
  function move(change){
    flushDraft();
    cancelImport();change();S.tab="runes";loadDraft();render();
  }
  function dirty(cancel=true){const content=draftContent(S.draft);if(content===lastContent)return;if(cancel)cancelImport();lastContent=content;S.dirty=content!==draftContent(stored());S.feedback="";S.error="";clearTimeout(draftTimer);if(S.dirty)draftTimer=setTimeout(flushDraft,300);else removeDraft();const feedback=root.querySelector(".ac-feedback");if(feedback)feedback.hidden=true;for(const n of root.querySelectorAll("[data-save-state]"))n.outerHTML=saveState(n.dataset.saveState);for(const n of root.querySelectorAll('[data-act="save"]')){n.disabled=!canSave();n.textContent=S.dirty?"保存":"已保存";}for(const n of root.querySelectorAll('[data-act="revert"]'))n.disabled=!sectionDirty(n.dataset.section);updateDraftMarkers();}
  function openModal(title,content){
    const opener=document.activeElement;
    if(window.deepLegendsSettingsModal)window.deepLegendsSettingsModal(title,content);
    else {const dialog=document.createElement("dialog"),header=document.createElement("header"),heading=document.createElement("h2"),close=document.createElement("button");dialog.className="settings-modal";heading.textContent=title;close.type="button";close.className="text-button";close.textContent="关闭";header.append(heading,close);dialog.append(header,content);document.body.append(dialog);close.addEventListener("click",()=>dialog.close());dialog.addEventListener("close",()=>dialog.remove(),{once:true});dialog.showModal();}
    const dialog=content.closest("dialog");dialog.classList.add("ac-dialog");dialog.setAttribute("aria-label",title);
    const close=dialog.querySelector("header button");close.className="dialog-close-button control-icon-button";close.setAttribute("aria-label","关闭");close.setAttribute("data-ac-close","");close.innerHTML='<svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>';
    const main=document.querySelector(".app-main")||root.closest("main");
    const align=()=>{const rect=main?.getBoundingClientRect(),zoom=Number(getComputedStyle(document.documentElement).getPropertyValue("--ui-zoom"))||1;const left=Math.max(0,rect?.left||0),right=Math.min(innerWidth,rect?.right||innerWidth),top=Math.max(0,rect?.top||0),bottom=Math.min(innerHeight,rect?.bottom||innerHeight);dialog.style.setProperty("--ac-dialog-x",`${(left+right)/2/zoom}px`);dialog.style.setProperty("--ac-dialog-y",`${(top+bottom)/2/zoom}px`);dialog.style.setProperty("--ac-dialog-width",`${Math.max(0,right-left-32)/zoom}px`);dialog.style.setProperty("--ac-dialog-height",`${Math.max(0,bottom-top-32)/zoom}px`);};
    align();window.addEventListener("resize",align);const observer=typeof ResizeObserver!=="undefined"?new ResizeObserver(align):null;if(main)observer?.observe(main);
    dialog.addEventListener("close",()=>{window.removeEventListener("resize",align);observer?.disconnect();if(opener?.isConnected)opener.focus({preventScroll:true});},{once:true});
    return dialog;
  }
  let copyDialog=null;
  function openCopy(){
    if(copyDialog?.open)return;
    const content=document.createElement("div");content.className="ac-confirm";
    const option=(key,name)=>`<label class="ac-check"><input type="checkbox" data-copy-target="${key}" aria-label="${esc(key.startsWith("ranked-")?"排位 · "+name:name)}" ${key===modeKey()?"disabled":""}><span>${esc(name)}</span>${key===modeKey()?"<small>当前配置</small>":""}</label>`;
    const targets=MODES.map(([key,name])=>`<section data-copy-mode="${key}" aria-label="${name}">${option(key,key==="ranked"?"排位（通用）":name)}${key==="ranked"?`<div class="ac-copy-positions" role="group" aria-label="排位位置">${POS.filter(([pos])=>pos).map(([pos,label])=>option("ranked-"+pos,label)).join("")}</div>`:""}</section>`).join("");
    content.innerHTML=`<p>选择要使用这份已保存配置的模式或位置。</p><div class="ac-copy">${targets}</div><p class="ac-confirm-error" role="alert" hidden></p><div class="ac-confirm-actions"><button class="button button-primary" data-act="copy-save">复制</button><button class="button button-secondary" data-act="copy-cancel">取消</button></div>`;
    copyDialog=openModal("复制配置到…",content);const dialog=copyDialog;dialog.addEventListener("close",()=>{if(copyDialog===dialog)copyDialog=null;},{once:true});
    content.addEventListener("click",async e=>{const b=e.target.closest("[data-act]");if(!b||b.disabled)return;const buttons=[...content.querySelectorAll("button")].map(n=>[n,n.disabled]);buttons.forEach(([n])=>n.disabled=true);try{await action(b.dataset.act);}catch(error){copyDialog?.close();reportError(error);}finally{buttons.forEach(([n,disabled])=>n.disabled=disabled);}});
    content.querySelector("input:not(:disabled)")?.focus();
  }
  function confirmPending(){
    if(pendingDialog?.open)return;
    const opener=document.activeElement,content=document.createElement("div");
    content.className="ac-confirm";
    content.innerHTML=`<p>导入推荐会替换当前草稿，未保存的修改将会丢失。</p><p class="ac-confirm-error" role="alert" hidden></p><div class="ac-confirm-actions"><button class="button button-secondary" data-act="stay">取消</button><button class="button button-primary" data-act="discard">放弃修改并导入</button></div>`;
    pendingDialog=openModal("有未保存的修改",content);
    const dialog=pendingDialog;
    dialog.addEventListener("close",()=>{if(pendingDialog===dialog){pendingDialog=null;S.pending=null;}if(opener?.isConnected)opener.focus({preventScroll:true});},{once:true});
    content.addEventListener("click",async e=>{const button=e.target.closest("[data-act]");if(!button||button.disabled)return;const buttons=[...content.querySelectorAll("button")].map(n=>[n,n.disabled]);buttons.forEach(([n])=>n.disabled=true);try{await action(button.dataset.act);}catch(error){reportError(error);}finally{buttons.forEach(([n,disabled])=>n.disabled=disabled);}});
    (content.querySelector('[data-act="stay"]')||dialog.querySelector("[data-ac-close]")).focus({preventScroll:true});
  }
  function closePending(){const dialog=pendingDialog;pendingDialog=null;S.pending=null;dialog?.close();}
  function importStatus(){for(const b of root.querySelectorAll('[data-act="import"]')){b.disabled=S.importing||!["ranked","normal","hextech"].includes(S.mode);b.textContent=S.importing?"正在导入…":"从推荐导入";b.setAttribute("aria-busy",String(S.importing));}}
  function reportError(error){S.error=error.message;S.feedback="";render();}
  function syncSettings(next){
    if(!next)return;
    S.settings=next;
    if(!S.dirty)loadDraft();
    else if(draftContent(stored())!==draftBase)S.error="配置已更新，请重新读取后保存";
  }
  function publishSettings(next){syncSettings(next);window.dispatchEvent(new CustomEvent("deep-legends:auto-config-settings",{detail:next}));}


  const hasMode=(id,mode)=>Object.keys(S.settings.champions[id]||{}).some(k=>k===mode||mode==="ranked"&&k.startsWith("ranked-"));
  const hasConfig=id=>MODES.some(([mode])=>hasMode(id,mode));
  function master(){const applied=Object.values(S.runtime.results||{}).filter(v=>v==="applied").length;return `<section class="suite-card ac-master"><div class="ac-master-id"><div><h2>自动配置</h2><p>锁定英雄后按模式自动应用到客户端</p></div>${S.runtime.championId?`<span class="cs-live-pill">本局已应用 ${applied}/3</span>`:""}</div><div class="ac-master-right"><div class="ac-subswitches">${[["runes","符文"],["spells","召唤师技能"],["items","装备"]].map(([k,n])=>`<label class="suite-switch"><span>${n}</span><input type="checkbox" data-enable="${k}" ${S.settings[k]?"checked":""}></label>`).join("")}</div><label class="suite-switch is-label-left"><input type="checkbox" data-enable="enabled" ${S.settings.enabled?"checked":""}><span>总开关</span></label></div><div class="ac-flow"><span class="ac-flow-step">排队</span>→<button class="ac-flow-step" data-jump="watch">接受对局（自动·${S.watch.rules?.autoAccept?.enabled?"已开":"已关"}）</button>→<button class="ac-flow-step" data-jump="champselect">禁用 / 选用（征召·${S.watch.champSelect?.enabled?"已开":"已关"}）</button>→<span class="ac-flow-step is-here">符文·技能·装备（配置·本页）</span>→<span class="ac-flow-step">进入游戏</span></div></section>`;}
  function champList(){const keys=dirtyKeys(),configured=Object.keys(S.settings.champions).filter(hasConfig).length;return `<aside class="suite-card ac-champs"><label class="ac-search"><input data-champ-search aria-label="搜索英雄" placeholder="搜索称号、名字、拼音首字母" value="${esc(S.q)}"><button type="button" class="player-search-button player-search-clear" data-act="clear-champ-search" aria-label="清空英雄搜索" ${S.q?"":"hidden"}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" stroke="currentColor" stroke-width="2" stroke-linecap="round" fill="none"/></svg></button></label><div class="ac-seg"><button data-configured="0" class="${!S.configured?"is-active":""}">全部</button><button data-configured="1" class="${S.configured?"is-active":""}">已配置 ${configured}</button></div><div class="ac-champ-list">${[...S.champions].sort((a,b)=>(S.mastery[b.id]||0)-(S.mastery[a.id]||0)||title(a).localeCompare(title(b),"zh-CN")).filter(c=>(!S.configured||hasConfig(c.id))&&(!S.q||[title(c),subtitle(c),...(c.searchTerms||[])].some(t=>t.toLowerCase().includes(S.q.toLowerCase())))).map(c=>`<button class="ac-champ${c.id===S.champ?" is-active":""}" data-champ="${c.id}"><span class="ac-portrait">${portrait(c)}</span><span class="ac-champ-name"><strong>${esc(title(c))}</strong><small>${esc(subtitle(c))}</small></span><span class="ac-dots">${MODES.map(m=>stateDot(c.id,m[0],m[1],false,keys)).join("")}</span></button>`).join("")}</div></aside>`;}
  function complete(){const r=S.draft?.runes;return r?.selectedPerkIds.length===9&&r.selectedPerkIds.every(id=>id>0&&perk(id));}
  function validationMessage(){
    if(!S.dirty||!S.draft)return "";
    const r=S.draft.runes,p=S.draft.spells;
    if(r&&!complete()&&!(S.tab==="kit"&&JSON.stringify(r)===JSON.stringify(stored()?.runes))){const missing=r.selectedPerkIds.filter(id=>!id).length;return missing?`符文还差 ${missing} 个没选`:"已失效符文，请重新选择";}
    if(p&&!!p.spell1Id!==!!p.spell2Id)return "召唤师技能只选了一个";
    return "";
  }
  function canSave(){return S.dirty&&!!S.draft&&!validationMessage();}
  function statusMarkup(){const text=S.error||validationMessage()||S.feedback;return `<div class="ac-feedback" role="${S.error||validationMessage()?"alert":"status"}" data-tooltip="${esc(text)}" data-tooltip-size="compact" ${text?"":"hidden"}><span>${esc(text)}</span>${S.error?.includes("配置已更新")?'<button class="text-button" data-act="reload">重新读取</button>':""}</div>`;}
  function highlightValidation(){const message=validationMessage();if(!message)return;const nodes=message.startsWith("符文还差")?root.querySelectorAll('.ac-group:not([data-group="sub"]) .ac-row:not(.is-picked),.ac-group[data-group="sub"]:has([data-sub-progress]:not(.is-picked))'):message==="召唤师技能只选了一个"?root.querySelectorAll('.ac-slot:has(.ac-slot-empty)'):[];for(const n of nodes){n.classList.add("ac-validation-pulse");n.addEventListener("animationend",()=>n.classList.remove("ac-validation-pulse"),{once:true});}}
  function sectionContent(value,section) {const content=JSON.parse(draftContent(value));return JSON.stringify(section==="kit"?{spells:content.spells,items:content.items}:content[section]);}
  function hasSection(value,section) {const content=JSON.parse(draftContent(value));return section==="kit"?!!(content.spells||content.items):!!content[section];}
  function sectionDirty(section) {return sectionContent(S.draft,section)!==sectionContent(stored(),section);}
  function saveState(section="runes") {const exists=hasSection(S.draft,section),changed=sectionDirty(section),date=new Date(stored()?.updatedAt),stamp=Number.isNaN(date.getTime())?"":date.toLocaleString("zh-CN",{month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false});return `<span data-save-state="${section}" class="ac-save-state ${!exists?"":changed?"ac-dirty":"ac-saved"}">${!exists?"未配置":changed?"未保存":"已保存 "+stamp}</span>`;}
  function saveButton() {return `<span class="ac-save-attempt" data-save-attempt><button class="button button-primary" data-act="save" ${canSave()?"":"disabled"}>${S.dirty?"保存":"已保存"}</button></span>`;}
  function editorActions(section="runes") {return `<div class="ac-action-row">${statusMarkup()}<div class="ac-editor-actions">${section!=="spells"?'<button class="button button-secondary ac-import" data-act="import">从推荐导入</button>':""}<button class="button button-secondary" data-act="clear" data-section="${section}" ${hasSection(S.draft,section)?"":"disabled"}>清空${{runes:"符文",spells:"召唤师技能",items:"装备"}[section]}</button><button class="button button-secondary" data-act="revert" data-section="${section}" ${sectionDirty(section)?"":"disabled"}>还原</button>${saveButton()}</div></div>`;}
  function tabDot(section,name) {const changed=sectionDirty(section);if(!changed&&!hasSection(S.draft,section))return "";return dotMarkup(changed,name);}
  function dotMarkup(unsaved,name) {const label=(name?name+" · ":"")+(unsaved?"有未保存的修改":"已保存");return `<i class="ac-state-dot ${unsaved?"is-unsaved":"is-saved"}" data-tooltip="${esc(label)}" data-tooltip-size="compact" aria-label="${esc(label)}"></i>`;}
  function runeBtn(p,row,col,selected,i){return `<button class="ac-rune${selected?" is-picked":""}" data-rune="${p.id}" data-row="${row}" data-col="${col}" aria-pressed="${selected}" aria-label="${esc(p.name)}" data-ac-tooltip="${esc(tip(p))}" data-i="${i}">${col==="shards"?`<span class="ac-shard-icon">${icon(p.iconPath).replace("<img ","<img data-shard-art ")}</span>`:icon(p.iconPath)}<span class="ring"></span></button>`;}
  function paths(id,sub){const p=style(id);return `<div class="ac-paths${sub?" is-secondary":""}"><span class="ac-path-main">${styleIcon(p)}</span>${S.catalog.styles.filter(s=>s.id!==id&&(!sub||s.id!==S.draft.runes.primaryStyleId)).map(s=>`<button class="ac-path-btn${s.id===id?" is-current":""}" data-pick-path="${sub?"sub":"primary"}" data-style="${s.id}" aria-label="${esc(s.name)}" data-ac-tooltip="${esc(s.name+"\n"+(s.tooltip||s.description||""))}">${styleIcon(s)}</button>`).join("")}</div>`;}
  function group(slots,col){const r=S.draft.runes;return `<div class="ac-group" data-group="${col}">${col==="sub"?[0,1].map(i=>`<span class="ac-node${S.subOrder.length>i?" is-picked":""}" data-sub-progress="${i}"></span>`).join(""):""}<div class="ac-track"><div class="ac-track-fill"></div></div>${slots.map((slot,row)=>{const actual=col==="sub"?row+1:row;const selected=col==="primary"?r.selectedPerkIds[row]:col==="sub"?S.subPicks[actual]:r.selectedPerkIds[row+6];return `<div class="ac-row ${col==="primary"?(row?"is-minor":slot.perks.length>3?"is-key has-4":"is-key"):col==="sub"?"is-sub":"is-shard"}${selected?" is-picked":""}" data-row="${actual}">${col==="sub"?"":'<span class="ac-node"></span>'}${col==="primary"&&!row?'<span class="ac-row-label">基石</span>':""}${slot.perks.map((p,i)=>runeBtn(p,actual,col,p.id===selected,row*4+i)).join("")}</div>`;}).join("")}</div>`;}
  function runeStage(){const r=S.draft?.runes;if(!r)return `<div class="ac-section-head">${editorActions()}</div><div class="ac-empty"><button class="button" data-act="start-runes">配置符文</button></div>`;const p=style(r.primaryStyleId),q=style(r.subStyleId);if(!p||!q)return `<div class="ac-notice ac-invalid">符文系已失效 <button data-act="start-runes">重新配置</button></div>`;return `<div class="ac-stage" data-stage data-path="${PATH[p.id]}"><div class="ac-buildname"><span class="ac-bn-key${r.selectedPerkIds[0]?"":" is-empty"}" data-path="${PATH[p.id]}">${styleIcon(p)}${esc(perk(r.selectedPerkIds[0])?.name||"未选基石")}</span><span class="ac-bn-plus">+</span><span class="ac-bn-sub" data-path="${PATH[q.id]}">${styleIcon(q)}${esc(q.name)}</span>${editorActions()}</div><div class="ac-tree"><div class="ac-col" data-path="${PATH[p.id]}">${paths(p.id,false)}${group(p.slots.slice(0,4),"primary")}</div><div class="ac-col is-secondary" data-path="${PATH[q.id]}">${paths(q.id,true)}${group(q.slots.slice(1,4),"sub")}${group(S.catalog.statModSlots.slice(0,3),"shards")}</div></div><span class="ac-invalid" data-invalid-runes ${r.selectedPerkIds.some(id=>id&&!perk(id))?"":"hidden"}>已失效符文，请重新选择</span></div>`;}
  function poolStatus(kind){return S.catalog[kind+"Status"]||(S.catalog[kind]?.length?"ready":S.catalog.clientConnected===false?"disconnected":"failed");}
  function poolMessage(kind){
    if(S.catalogLoading)return "正在读取客户端目录…";
    if(poolStatus(kind)==="disconnected")return `连接客户端后加载${kind==="items"?"装备":"召唤师技能"}`;
    if(poolStatus(kind)!=="ready")return `${kind==="items"?"装备":"召唤师技能"}目录读取失败，请重新读取`;
    return kind==="items"?"没有同时符合这些属性的装备":"当前模式暂无可用召唤师技能";
  }
  function spellSlots(pair,spells,keys){return pair.map((id,i)=>{
    const spell=S.catalog.spells.find(s=>s.id===id),invalid=id&&poolStatus("spells")==="ready"&&!spells.some(s=>s.id===id);
    return `${i?'<button class="ac-swap" data-act="swap" aria-label="交换召唤师技能">⇄</button>':""}<button class="ac-slot${S.slot===i?" is-active":""}${invalid?" ac-invalid":""}" data-slot="${i}" aria-label="${i?"右":"左"}侧技能" ${spell?`data-ac-tooltip="${esc(tip(spell))}"`:""}>${id?icon(spell?.iconPath):'<span class="ac-slot-empty" aria-hidden="true">+</span>'}${keys[i]?`<kbd>${esc(keys[i])}</kbd>`:""}${id?`<span class="ac-spell-remove" data-clear-spell="${i}" role="button" aria-label="清空${i?"右":"左"}侧技能">×</span>`:""}</button>`;
  }).join("");}
  const itemLookup=id=>S.catalog.items.find(i=>i.id===id);
  const itemID=value=>value&&typeof value==="object"?value.id:value;
  const itemCount=value=>value&&typeof value==="object"?(value.count||1):1;
  function itemLists(){const items=S.draft.items||={starter:[],core:[],situational:[]};items.starter||=[];items.core||=[];items.situational||=[];return items;}
  const starterTotal=()=>itemLists().starter.reduce((sum,item)=>sum+(itemLookup(item.id)?.priceTotal||0)*item.count,0);
  const starterRemaining=()=>starterBudget(S.mode)-starterTotal();
  function shopItems(shop,items){return shop.length?shop.map(i=>{const starting=S.itemGroup==="starter",used=!starting&&[...items.core,...items.situational].includes(i.id),unaffordable=starting&&i.priceTotal>starterRemaining();return `<button class="ac-item${used?" is-used":""}" data-item="${i.id}" draggable="${!unaffordable}" aria-disabled="${unaffordable}" aria-label="${esc(i.name)}" data-ac-tooltip="${esc(tip(i)+"\n"+i.priceTotal+" 金币")}">${icon(i.iconPath)}<span>${i.priceTotal}</span></button>`;}).join(""):`<div class="ac-shop-empty" role="status">${esc(poolMessage("items"))}${poolStatus("items")!=="ready"?'<button class="text-button" data-act="refresh-catalog">重新读取</button>':""}</div>`;}
  function filteredItems(){const starting=S.itemGroup==="starter"||S.filters.has("Starter");const shop=S.catalog.items.filter(i=>(starting?starterAllowed(i,S.mode):itemAllowed(i,S.mode))&&[...S.filters].every(f=>f==="Starter"||f.split("|").some(c=>i.categories?.includes(c)))&&(!S.iq||i.name.includes(S.iq)));return shop.sort((a,b)=>S.sort==="name"?a.name.localeCompare(b.name,"zh-CN"):S.sort==="desc"?b.priceTotal-a.priceTotal:a.priceTotal-b.priceTotal);}
  function updateShop(){hideTip();const grid=root.querySelector(".ac-item-grid");if(grid){grid.innerHTML=shopItems(filteredItems(),itemLists());renderedEditor=editor();}}
  function buildBlock(k,n,items){const starting=k==="starter",active=S.itemGroup===k,total=items[k].reduce((sum,value)=>sum+(itemLookup(itemID(value))?.priceTotal||0)*itemCount(value),0);return `<section class="ac-build-block${active?" is-active":""}${starting&&S.budgetError?" is-budget-error":""}" data-item-group="${k}" tabindex="0" aria-label="${n}"><header><strong>${n}</strong><span class="gold">${total}${starting?" / "+starterBudget(S.mode):""} 金币</span></header>${starting&&S.budgetError?`<small class="ac-budget-error" role="status">超出出门预算（剩 ${starterRemaining()} 金币）</small>`:""}<div class="ac-build-slots">${Array.from({length:6},(_,i)=>{const value=items[k][i],id=itemID(value),item=itemLookup(id),invalid=id&&poolStatus("items")==="ready"&&(!item||!(starting?starterAllowed(item,S.mode):itemAllowed(item,S.mode)));return `<button class="ac-bslot${id?" has":""}${invalid?" ac-invalid":""}" data-build="${k}:${i}" draggable="${!!id}" aria-label="${n}第${i+1}格${id?item?.name||"目录未载入":"空"}" ${item?`data-ac-tooltip="${esc(tip(item))}"`:""}>${id?`<b>${starting?"×"+itemCount(value):i+1}</b>${icon(item?.iconPath)}<span class="x" data-remove="${k}:${i}">×</span>`:"+"}</button>`;}).join("")}</div></section>`;}
  function kit(){
    const spells=S.catalog.spells.filter(s=>spellAllowed(s,S.mode,S.catalog.eventGameMode)),pair=[S.draft?.spells?.spell1Id||0,S.draft?.spells?.spell2Id||0],items=S.itemEditing?itemLists():{starter:[],core:[],situational:[]},keys=S.keys.keys||[];
    return `<div class="ac-kit"><div class="ac-section-head"><h4>召唤师技能</h4>${saveState("spells")}<span class="ac-section-hint">点一个技能放进 ${esc(keys[S.slot]||["左侧","右侧"][S.slot])} 位</span>${editorActions("spells")}</div>${S.spellEditing?`<div class="ac-spells"><div class="ac-spell-slots"><div class="ac-slots">${spellSlots(pair,spells,keys)}</div></div><div><div class="ac-spell-grid">${spells.length?spells.map(s=>{const key=keys[pair.indexOf(s.id)];return `<button class="ac-spell${pair.includes(s.id)?" is-used":""}" data-spell="${s.id}" ${key?`data-spell-key="${esc(key)}"`:""} data-ac-tooltip="${esc(tip(s)+"\n冷却 "+s.cooldown+" 秒")}">${icon(s.iconPath)}${esc(s.name)}</button>`;}).join(""):`<p class="ac-catalog-empty" role="status">${esc(poolMessage("spells"))} <button class="text-button" data-act="refresh-catalog">重新读取</button></p>`}</div></div><p class="ac-spell-note">${S.mode==="hextech"?"海克斯大乱斗没有虚弱，技能以当前客户端为准":S.mode==="aram"?"极地大乱斗没有传送、惩戒":S.mode==="event"?"活动模式技能以当前客户端为准":""}</p></div>`:'<div class="ac-section-empty"><button class="button button-secondary" data-act="start-spells">配置召唤师技能</button></div>'}<div class="ac-section-head"><h4>装备</h4>${saveState("items")}${editorActions("items")}</div>${S.itemEditing?`<div class="ac-items"><div class="ac-filter"><div class="ac-filter-head">筛选 <button data-act="filters-clear">清空</button></div>${[["Starter","出门装"],...ATTR].map(([k,n])=>`<label class="ac-check"><input type="checkbox" data-filter="${k}" ${S.filters.has(k)?"checked":""}>${n}</label>`).join("")}</div><div class="ac-shop"><div class="ac-shop-bar"><label class="ac-search"><input data-item-search aria-label="搜索装备" placeholder="搜索装备" value="${esc(S.iq)}"></label><label class="select-wrap ac-sort"><select data-sort aria-label="装备排序"><option value="price" ${S.sort==="price"?"selected":""}>价格升序</option><option value="desc" ${S.sort==="desc"?"selected":""}>价格降序</option><option value="name" ${S.sort==="name"?"selected":""}>名称</option></select></label></div><div class="ac-item-grid">${shopItems(filteredItems(),items)}</div></div><div class="ac-build"><div class="ac-build-blocks">${[["starter","出门装"],["core","核心出装"],["situational","备选"]].map(([k,n])=>buildBlock(k,n,items)).join("")}</div><p class="ac-build-note">点击装备加入选中区域；可拖动排序、跨组移动。选中格后 Delete 删除。进入英雄选择后写入游戏内推荐装备页 [DL] ${esc(title(champ()))}。</p></div></div>`:'<div class="ac-section-empty"><button class="button button-secondary" data-act="start-items">配置装备</button></div>'}</div>`;
  }
  function editor(){const c=champ(),keys=dirtyKeys();return `<section class="suite-card ac-editor"><div class="ac-ed-head"><span class="ac-portrait">${portrait(c)}</span><div class="ac-ed-title"><h3>${esc(title(c))}<small>${esc(subtitle(c))}</small></h3></div><div class="ac-ed-actions"><button class="ac-ghost" data-act="copy" ${stored()?"":"disabled"}>复制到…</button></div></div><div class="ac-modes">${MODES.map(([k,n,t])=>`<button class="ac-mode${S.mode===k?" is-active":""}" data-mode="${k}" data-tooltip="${t}" data-tooltip-size="compact">${n}</button>`).join("")}<div class="ac-mode-actions"><button class="button button-secondary" data-act="clear-all" ${hasSection(S.draft,"runes")||hasSection(S.draft,"kit")||stored()?"":"disabled"}>清空</button>${saveButton()}</div></div>${S.mode==="ranked"?`<div class="ac-positions">${POS.map(([k,n])=>`<button class="ac-pos${S.pos===k?" is-active":""}" data-pos="${k}">${n}${stateDot(S.champ,k?"ranked-"+k:"ranked",n,true,keys)}</button>`).join("")}<span class="ac-pos-hint">位置没配时用「通用」</span></div>`:""}<div class="ac-tabs">${S.mode==="hextech"?"":`<button class="ac-tab${S.tab==="runes"?" is-active":""}" data-tab="runes">符文${tabDot("runes","符文")}</button>`}<button class="ac-tab${S.tab==="kit"?" is-active":""}" data-tab="kit">召唤师技能与装备${tabDot("kit","召唤师技能与装备")}</button>${S.tab==="runes"?saveState():""}</div>${S.tab==="runes"?runeStage():kit()}</section>`;}
  let locatePending=true;
  function locateChampion(){
    if(!locatePending||root.closest("[hidden]"))return;
    const list=root.querySelector(".ac-champ-list"),active=list?.querySelector(".is-active");if(!active)return;
    locatePending=false;
    const a=active.getBoundingClientRect(),r=list.getBoundingClientRect();
    list.scrollTop+=a.top-r.top-(r.height-a.height)/2;
    list.scrollLeft+=a.left-r.left-(r.width-a.width)/2;
  }
  function dirtyKeys(){
    const keys=new Set();
    for(const row of draftRows()){try{const d=JSON.parse(preferences().getItem(row.key));if(d&&draftContent(d.value)!==draftContent(S.settings?.champions?.[d.championId]?.[d.modeKey])&&validEditorDraft(d.value,S.catalog,d.modeKey.split("-")[0]))keys.add(row.key);}catch(_){} }
    const current=draftKey();if(S.dirty)keys.add(current);else keys.delete(current);
    return keys;
  }
  function stateDot(id,key,name="",exact=false,keys=dirtyKeys()){
    const prefix=draftPrefix+id+"-",unsaved=[...keys].some(k=>k===prefix+key||!exact&&key==="ranked"&&k.startsWith(prefix+"ranked-"));
    const saved=exact?!!S.settings.champions[id]?.[key]:hasMode(id,key);
    if(!unsaved&&!saved)return "";
    return dotMarkup(unsaved,name);
  }
  function updateDraftMarkers(){
    const keys=dirtyKeys();
    for(const node of root.querySelectorAll("[data-champ],[data-pos],[data-tab]")){
      const list=node.hasAttribute("data-champ");
      const markup=list?MODES.map(([key,name])=>stateDot(+node.dataset.champ,key,name,false,keys)).join(""):node.dataset.tab?tabDot(node.dataset.tab,node.dataset.tab==="runes"?"符文":"召唤师技能与装备"):stateDot(S.champ,node.dataset.pos?"ranked-"+node.dataset.pos:"ranked",POS.find(p=>p[0]===node.dataset.pos)?.[1],true,keys);
      if(list){const dots=node.querySelector(".ac-dots");if(dots.innerHTML!==markup)dots.innerHTML=markup;}
      else {const dot=node.querySelector(".ac-state-dot");if(dot?.outerHTML!==markup){dot?.remove();node.insertAdjacentHTML("beforeend",markup);}}
    }
  }
  let renderedMaster="",renderedChampions="",renderedEditor="";
  let observedRuneTree=null;
  const runeLayoutObserver=typeof ResizeObserver==="function"?new ResizeObserver(layout):null;
  function observeRuneLayout(){
    const tree=root.querySelector(".ac-tree")||root;if(tree===observedRuneTree)return;
    runeLayoutObserver?.disconnect();observedRuneTree=tree;
    if(tree){runeLayoutObserver?.observe(tree);tree.querySelectorAll(".ac-paths,.ac-group").forEach(node=>runeLayoutObserver?.observe(node));}
  }
  function render(){
    hideTip();if(!S.settings||!S.catalog)return;
    if(!root.querySelector(".ac-body")){root.className="ac-root";root.innerHTML='<div data-ac-master></div><div class="ac-body"><div data-ac-champs></div><div data-ac-editor></div></div>';renderedMaster=renderedChampions=renderedEditor="";}
    const focus=document.activeElement,field=focus?.hasAttribute("data-champ-search")?"data-champ-search":focus?.hasAttribute("data-item-search")?"data-item-search":null,pos=focus?.selectionStart;
    const masterHTML=master(),championsHTML=champList(),editorHTML=editor();
    const masterHost=root.querySelector("[data-ac-master]"),champHost=root.querySelector("[data-ac-champs]"),editorHost=root.querySelector("[data-ac-editor]");
    if(masterHTML!==renderedMaster){masterHost.innerHTML=masterHTML;renderedMaster=masterHTML;}
    if(championsHTML!==renderedChampions){const old=root.querySelector(".ac-champ-list"),top=old?.scrollTop||0,left=old?.scrollLeft||0;champHost.innerHTML=championsHTML;const next=root.querySelector(".ac-champ-list");next.scrollTop=top;next.scrollLeft=left;renderedChampions=championsHTML;}
    if(editorHTML!==renderedEditor){const selectors=[".ac-item-grid",".ac-filter",".ac-build"],scrolls=selectors.map(sel=>[sel,root.querySelector(sel)?.scrollTop||0,root.querySelector(sel)?.scrollLeft||0]),outer=[];for(let n=root.parentElement;n;n=n.parentElement)outer.push([n,n.scrollTop,n.scrollLeft]);editorHost.innerHTML=editorHTML;renderedEditor=editorHTML;window.deepLegendsSelects?.enhance(editorHost);for(const [sel,top,left]of scrolls){const n=root.querySelector(sel);if(n){n.scrollTop=top;n.scrollLeft=left;}}for(const [n,top,left]of outer){n.scrollTop=top;n.scrollLeft=left;}}
    normalizeShardImages();layout();observeRuneLayout();metrics();importStatus();updateDraftMarkers();locateChampion();
    if(field){const n=root.querySelector(`[${field}]`);n?.focus({preventScroll:true});n?.setSelectionRange(pos,pos);}
  }
  function updateRuneSelection(){
    const r=S.draft.runes;hideTip();
    root.querySelectorAll(".ac-rune").forEach(button=>{
      const {col,row,rune}=button.dataset,index=+row,selected=col==="primary"?r.selectedPerkIds[index]:col==="sub"?S.subPicks[index]:r.selectedPerkIds[6+index];
      const picked=selected===+rune;button.classList.toggle("is-picked",picked);button.setAttribute("aria-pressed",String(picked));
    });
    root.querySelectorAll(".ac-row").forEach(row=>row.classList.toggle("is-picked",!!row.querySelector(".ac-rune.is-picked")));
    const key=root.querySelector(".ac-bn-key");if(key){const label=perk(r.selectedPerkIds[0])?.name||"未选基石";key.lastChild.textContent=label;key.classList.toggle("is-empty",!r.selectedPerkIds[0]);}
    const warning=root.querySelector(".ac-stage-warn");if(warning)warning.hidden=complete();
    const invalid=root.querySelector("[data-invalid-runes]");if(invalid)invalid.hidden=!r.selectedPerkIds.some(id=>id&&!perk(id));
    for(const saveButton of root.querySelectorAll('[data-act="save"]')){saveButton.disabled=!canSave();saveButton.textContent=S.dirty?"保存":"已保存";}
    const feedback=root.querySelector(".ac-feedback");if(feedback)feedback.outerHTML=statusMarkup();updateDraftMarkers();renderedEditor=editor();layout();
  }

  const shardBounds=new Map();
  function normalizeShardImage(img){
    const apply=metrics=>{img.style.setProperty("--ac-shard-scale",String(metrics.scale));img.style.setProperty("--ac-shard-x",`${metrics.x}%`);img.style.setProperty("--ac-shard-y",`${metrics.y}%`);img.dataset.shardReady="true";};
    const key=img.currentSrc||img.src;if(shardBounds.has(key)){apply(shardBounds.get(key));return;}
    if(!img.complete||!img.naturalWidth||!img.naturalHeight)return;
    try {const canvas=document.createElement("canvas"),w=img.naturalWidth,h=img.naturalHeight;canvas.width=w;canvas.height=h;const ctx=canvas.getContext("2d",{willReadFrequently:true});if(!ctx)return;ctx.drawImage(img,0,0);const data=ctx.getImageData(0,0,w,h).data;let left=w,top=h,right=-1,bottom=-1,area=0;for(let y=0;y<h;y++)for(let x=0;x<w;x++){const alpha=data[(y*w+x)*4+3];area+=alpha/255;if(alpha>32){left=Math.min(left,x);right=Math.max(right,x);top=Math.min(top,y);bottom=Math.max(bottom,y);}}if(right<left)return;const extent=Math.max(right-left+1,bottom-top+1),density=area/(extent*extent),optical=Math.min(1,Math.sqrt(.45/density)),scale=Math.min(1.8,Math.max(w,h)/extent*optical),metrics={scale,x:(w/2-(left+right+1)/2)/w*scale*100,y:(h/2-(top+bottom+1)/2)/h*scale*100};shardBounds.set(key,metrics);apply(metrics);}catch(_){/* A missing image keeps its normal object-fit fallback. */}
  }
  function normalizeShardImages(){root.querySelectorAll("[data-shard-art]").forEach(normalizeShardImage);}
  root.addEventListener("error",e=>{if(e.target.matches?.(".ac-portrait img")&&e.target.getAttribute("src")!=="/image-unavailable.svg")e.target.src="/image-unavailable.svg";},true);
  root.addEventListener("load",e=>{if(e.target.matches?.("[data-shard-art]"))normalizeShardImage(e.target);},true);
  let layoutFrame=0;
  function scheduleLayout(){if(!layoutFrame)layoutFrame=requestAnimationFrame(()=>{layoutFrame=0;layout();});}
  function layout(){
    // Collect every geometry read before writing any track or progress styles.
    const body=root.querySelector(".ac-body"),zoom=Number(getComputedStyle(document.documentElement).getPropertyValue("--ui-zoom"))||1;
    const bodyValue=body&&!root.closest("[hidden]")?`${Math.max(0,(innerHeight-body.getBoundingClientRect().top-24)/zoom)}px`:null;
    const updates=[];
    for(const g of root.querySelectorAll(".ac-group")){
      const rows=[...g.querySelectorAll(":scope > .ac-row")];if(!rows.length)continue;
      const centers=rows.map(n=>n.offsetTop+n.offsetHeight/2),paths=g.previousElementSibling,main=paths?.classList.contains("ac-paths")?paths.querySelector(".ac-path-main"):null;
      const top=g.dataset.group==="shards"?centers[0]:main?paths.offsetTop+main.offsetTop+main.offsetHeight-g.offsetTop:-6;
      const count=Object.keys(S.subPicks).length,sub=g.dataset.group==="sub",selected=sub?(count?count===1?centers[0]:centers.at(-1):null):centers[rows.findLastIndex(n=>n.classList.contains("is-picked"))];
      updates.push({g,top,height:centers.at(-1)-top,fill:selected==null?0:selected-top,centers,count,sub});
    }
    if(bodyValue!==null&&body.style.getPropertyValue("--ac-body-min")!==bodyValue)body.style.setProperty("--ac-body-min",bodyValue);
    for(const n of root.querySelectorAll(".ac-rune[data-i]"))if(n.style.getPropertyValue("--i")!==n.dataset.i)n.style.setProperty("--i",n.dataset.i);
    for(const {g,top,height,fill,centers,count,sub}of updates){
      const track=g.querySelector(".ac-track");if(track.style.top!==`${top}px`)track.style.top=`${top}px`;if(track.style.height!==`${height}px`)track.style.height=`${height}px`;
      if(sub)g.querySelectorAll("[data-sub-progress]").forEach((node,index)=>{node.style.top=`${index?centers.at(-1):centers[0]}px`;node.classList.toggle("is-picked",index<count);});
      const style=track.querySelector(".ac-track-fill").style;if(style.getPropertyValue("--fill")!==`${fill}px`)style.setProperty("--fill",`${fill}px`);
    }
  }
  function ripple(btn){if(!btn)return;btn.querySelectorAll(".ac-ripple,.ac-burst").forEach(n=>n.remove());clearTimeout(btn._acFlashTimer);for(const cls of ["","is-fill"]){const n=document.createElement("span");n.className="ac-ripple "+cls;btn.append(n);setTimeout(()=>n.remove(),700);}const burst=document.createElement("span");burst.className="ac-burst";for(let i=0;i<8;i++){const n=document.createElement("i");n.style.setProperty("--a",`${i*45}deg`);burst.append(n);}btn.append(burst);setTimeout(()=>burst.remove(),700);btn.classList.remove("is-flash");void btn.offsetWidth;btn.classList.add("is-flash");btn._acFlashTimer=setTimeout(()=>btn.classList.remove("is-flash"),700);(btn.closest(".ac-row")?.querySelector(".ac-node")||btn.closest(".ac-group")?.querySelector(`[data-sub-progress="${Math.max(0,Object.keys(S.subPicks).length-1)}"]`))?.classList.add("is-pulse");btn.closest(".ac-group")?.querySelector(".ac-track")?.classList.add("is-run");}
  let saveQueue=Promise.resolve();
  async function save(clearAll=false){
    if(!clearAll&&!canSave())throw Error(validationMessage()||"没有可保存的修改");
    const id=S.champ,key=modeKey(),revision=S.generation,draft=clearAll?{}:clone(S.draft),base=draftBase;
    if(draft.spells&&!draft.spells.spell1Id&&!draft.spells.spell2Id)delete draft.spells;
    const empty=!draft.runes&&!draft.spells&&!["starter","core","situational"].some(k=>draft.items?.[k]?.length);
    const request=saveQueue.catch(()=>{}).then(async()=>{
      const latest=await api("/api/auto-config/settings");
      if(draftContent(latest.champions?.[id]?.[key])!==(S.champ===id&&modeKey()===key?draftBase:base)){if(!clearAll)syncSettings(latest);throw Error("配置已更新，请重新读取后保存");}
      return api(`/api/auto-config/champions/${id}/${key}`,empty?"DELETE":"PUT",empty?undefined:draft);
    });
    saveQueue=request;const next=await request;if(clearAll)removeDraft(id,key);
    const unchanged=S.champ===id&&modeKey()===key&&S.generation===revision;
    S.settings=next;
    if(unchanged){removeDraft(id,key);S.dirty=false;loadDraft();}
    else if(S.champ===id&&modeKey()===key){draftBase=draftContent(stored());flushDraft();}
    window.dispatchEvent(new CustomEvent("deep-legends:auto-config-settings",{detail:next}));
    S.error="";render();return unchanged;
  }
  async function importRecommendation(){
    cancelImport();const generation=S.generation,c=champ(),mode=S.mode,pos=mode==="ranked"?S.pos:"";
    const controller=new AbortController();importController=controller;S.importing=true;S.error="";S.feedback="正在读取推荐…";importStatus();
    const timer=setTimeout(()=>controller.abort(),25000);
    const current=()=>generation===S.generation;
    try {
      const positionKey=p=>({middle:"mid",bottom:"adc",utility:"support"})[p]||p;
      let position=positionKey(pos);
      if(mode!=="hextech"&&!position){
        const lanes=await api(`/api/gameplay/champion-lanes?champions=${c.id}&tier=emerald_plus`,"GET",undefined,controller.signal);
        if(!current())return;
        position=positionKey([...(lanes[c.id]||[])].filter(p=>["top","jungle","middle","mid","bottom","adc","utility","support"].includes(p.position)).sort((a,b)=>b.rate-a.rate)[0]?.position);
        if(!position)throw Error("暂无可用推荐位置，请选择具体位置后重试");
      }
      const detailAt=p=>api(`/api/champions/detail?mode=${mode==="hextech"?"aram-mayhem":"ranked"}&champion=${encodeURIComponent(c.key)}&position=${p||"all"}&tier=emerald_plus`,"GET",undefined,controller.signal);
      let detail=await detailAt(position);if(!current())return;
      if(mode!=="hextech"&&!pos&&detail.positions?.length){
        const best=[...detail.positions].filter(p=>["top","jungle","middle","mid","bottom","adc","utility","support"].includes(p.position)).sort((a,b)=>(b.play||b.games||0)-(a.play||a.games||0))[0];
        if(best&&positionKey(best.position)!==position){detail=await detailAt(positionKey(best.position));if(!current())return;}
      }
      const draft={},r=detail.runes?.[0],build=detail.build||{};
      let imported=0;
      if(mode!=="hextech"&&style(r?.primaryStyle?.id)&&style(r?.subStyle?.id)&&r.selected?.length===9){draft.runes={primaryStyleId:r.primaryStyle.id,subStyleId:r.subStyle.id,selectedPerkIds:r.selected.map(p=>p.id)};imported++;}
      const pair=build.summonerSpells?.[0]?.assets?.map(a=>a.id);
      if(pair?.length===2&&pair.every(id=>S.catalog.spells.some(s=>s.id===id&&spellAllowed(s,mode,S.catalog.eventGameMode)))){draft.spells={spell1Id:pair[0],spell2Id:pair[1]};imported++;}
      const core=(build.coreItems?.[0]?.assets||[]).map(a=>a.id).filter(id=>S.catalog.items.some(i=>i.id===id&&itemAllowed(i,mode)));
      const starter=[];let cost=0;
      for(const asset of build.starterItems?.[0]?.assets||[]){
        const item=itemLookup(asset.id),count=asset.count??1;
        if(!item||!starterAllowed(item,mode)||!Number.isInteger(count)||count<1||count>99||(!consumable(item)&&count!==1))continue;
        const same=starter.find(i=>i.id===asset.id);
        if(same&&(!consumable(item)||same.count+count>99)||!same&&starter.length>=6||cost+item.priceTotal*count>starterBudget(mode))continue;
        if(same)same.count+=count;else starter.push({id:asset.id,count});
        cost+=item.priceTotal*count;
      }
      if(core.length||starter.length){draft.items={starter,core:[...new Set(core)].slice(0,6),situational:[]};imported++;}
      if(!imported)throw Error("当前没有可导入的推荐配置，请稍后重试");
      normalizeStarterTrinkets(draft,S.catalog);S.draft=draft;S.spellEditing=!!draft.spells;S.itemEditing=hasSection(draft,"items");readSecondary();dirty(false);flushDraft();
      try{if(S.dirty)await save();if(current())S.feedback="推荐已导入并保存";}catch(error){if(current()){S.error="推荐已导入，但保存失败："+error.message;flushDraft();}}render();
    }catch(error){if(current())reportError(controller.signal.aborted?Error("推荐读取超时，请重试"):error);}
    finally{clearTimeout(timer);if(current()){S.importing=false;importController=null;importStatus();}}
  }
  let clearDialog=null;
  function confirmClearAll() {
    if(clearDialog?.open||S.clearing)return;
    const content=document.createElement("div"),label=`${title(champ())} · ${modeName()}`;content.className="ac-confirm";
    content.innerHTML=`<p>将立即清空「${esc(label)}」的<b class="ac-confirm-highlight">符文、召唤师技能和装备</b>全部配置，包括未保存的修改。</p><p class="ac-section-hint">清空后立刻生效，不需要再点「保存」，也无法恢复。</p><div class="ac-confirm-actions"><button class="button button-secondary" data-clear-cancel>取消</button><button class="button ac-danger" data-clear-confirm>清空全部</button></div>`;
    const dialog=clearDialog=openModal("清空全部配置",content);dialog.addEventListener("close",()=>{if(clearDialog===dialog)clearDialog=null;},{once:true});
    content.querySelector("[data-clear-cancel]").onclick=()=>dialog.close();
    content.querySelector("[data-clear-confirm]").onclick=async event=>{if(S.clearing)return;S.clearing=true;event.target.disabled=true;event.target.textContent="正在清空…";try{cancelImport();if(await save(true)){S.feedback=`已清空「${label}」的全部配置`;render();}}catch(error){S.error="清空失败："+error.message;render();}finally{S.clearing=false;dialog.close();}};
  }
  function readSecondary(){S.subPicks={};S.subOrder=[];for(const id of S.draft?.runes?.selectedPerkIds.slice(4,6)||[]){const row=style(S.draft.runes.subStyleId)?.slots.findIndex(s=>s.perks.some(p=>p.id===id));if(row>0){S.subPicks[row]=id;S.subOrder.push(row);}}}
  function normalizeCatalog(catalog){catalog.styles.sort((a,b)=>(STYLE_ORDER.indexOf(a.id)+1||99)-(STYLE_ORDER.indexOf(b.id)+1||99));return catalog;}
  let catalogFlight=null;
  async function refreshCatalog(){
    if(catalogFlight)return catalogFlight;
    S.catalogLoading=true;if(S.tab==="kit"&&S.draft)render();
    catalogFlight=(async()=>{try{const [catalog,keys]=await Promise.all([api("/api/auto-config/catalog"),api("/api/settings/summoner-keys").catch(()=>S.keys)]);S.catalog=normalizeCatalog(catalog);S.keys=keys;}finally{S.catalogLoading=false;catalogFlight=null;if(S.settings&&S.catalog)render();}})();
    return catalogFlight;
  }

  async function action(name,section=S.tab==="runes"?"runes":"items"){switch(name){case "clear-champ-search":S.q="";render();root.querySelector("[data-champ-search]").focus();break;case "retry":await load(true);break;case "refresh-catalog":await refreshCatalog();break;case "start":S.draft=fresh();dirty();render();break;case "start-runes":S.draft||={};S.draft.runes=fresh().runes;dirty();render();break;case "save":await save();break;case "reload":S.settings=await api("/api/auto-config/settings");removeDraft();loadDraft();render();break;case "start-spells":S.draft||={};S.spellEditing=true;render();break;case "start-items":S.draft||={};S.itemEditing=true;render();break;case "clear-all":confirmClearAll();break;case "revert":{S.draft||={};if(stored()?.[section])S.draft[section]=clone(stored()[section]);else delete S.draft[section];S.spellEditing=!!S.draft.spells;S.itemEditing=hasSection(S.draft,"items");readSecondary();dirty();render();break;}case "clear":if(S.draft){delete S.draft[section];if(section==="spells")S.spellEditing=false;if(section==="items")S.itemEditing=false;readSecondary();dirty();render();}break;case "stay":closePending();break;case "discard":{removeDraft();const next=S.pending,kind=S.pendingKind;closePending();cancelImport();await next?.();if(kind!=="import"){S.tab="runes";loadDraft();render();}break;}case "copy":openCopy();break;case "copy-cancel":copyDialog?.close();break;case "copy-save":{const targets=[...copyDialog.querySelectorAll("[data-copy-target]:checked:not(:disabled)")].map(n=>n.dataset.copyTarget);if(!targets.length)throw Error("请先选择复制目标");publishSettings(await api("/api/auto-config/copy","POST",{championId:S.champ,from:modeKey(),targets}));S.error="";copyDialog.close();render();break;}case "import":if(S.dirty){S.pendingKind="import";S.pending=importRecommendation;confirmPending();}else await importRecommendation();break;case "swap":{const p=S.draft.spells;if(p)[p.spell1Id,p.spell2Id]=[p.spell2Id,p.spell1Id];dirty();render();break;}case "filters-clear":S.filters.clear();render();}}
  root.addEventListener("click",async event=>{if(event.target.closest("[data-save-attempt]")&&!canSave()){highlightValidation();return;}const b=event.target.closest("button,[data-remove],[data-clear-spell],[data-item-group]");if(!b||b.disabled)return;if(b.dataset.itemGroup){S.itemGroup=b.dataset.itemGroup;S.budgetError=false;render();return;}try{if(b.dataset.act){await action(b.dataset.act,b.dataset.section);return;}if(b.dataset.champ){if(+b.dataset.champ===S.champ)return;move(()=>S.champ=+b.dataset.champ);return;}if(b.dataset.mode){if(b.dataset.mode===S.mode)return;move(()=>S.mode=b.dataset.mode);return;}if(b.hasAttribute("data-pos")){if(b.dataset.pos===S.pos)return;move(()=>S.pos=b.dataset.pos);return;}if(b.dataset.tab){if(S.mode==="hextech"&&b.dataset.tab==="runes")return;S.tab=b.dataset.tab;render();return;}if(b.dataset.jump){document.querySelector(`[data-suite-tab="${b.dataset.jump}"]`)?.click();return;}if(b.dataset.configured){S.configured=b.dataset.configured==="1";if(!S.configured)locatePending=true;render();return;}if(b.dataset.pickPath){const r=S.draft.runes,id=+b.dataset.style;if(b.dataset.pickPath==="primary"){const old=r.primaryStyleId;if(id===r.subStyleId){r.subStyleId=old;S.subPicks={};S.subOrder=[];r.selectedPerkIds[4]=r.selectedPerkIds[5]=0;}r.primaryStyleId=id;r.selectedPerkIds.splice(0,4,0,0,0,0);}else{r.subStyleId=id;S.subPicks={};S.subOrder=[];r.selectedPerkIds[4]=r.selectedPerkIds[5]=0;}dirty();render();root.querySelector(".ac-buildname")?.classList.add("is-swap");for(const col of root.querySelectorAll(b.dataset.pickPath==="primary"?".ac-col":".ac-col.is-secondary")){col.classList.add("is-entering");setTimeout(()=>col.classList.remove("is-entering"),700);}return;}if(b.dataset.rune){const id=+b.dataset.rune,row=+b.dataset.row,col=b.dataset.col,r=S.draft.runes;if(col==="primary")r.selectedPerkIds[row]=id;else if(col==="shards")r.selectedPerkIds[6+row]=id;else{selectSecondary(S.subOrder,row,id,S.subPicks);r.selectedPerkIds.splice(4,2,...[...Object.values(S.subPicks),0,0].slice(0,2));}dirty();updateRuneSelection();ripple(b);return;}if(b.hasAttribute("data-slot")){const slot=Number(b.dataset.slot);if(slot===0||slot===1)S.slot=slot;render();return;}if(b.hasAttribute("data-clear-spell")){if(S.draft.spells)S.draft.spells[+b.dataset.clearSpell?"spell2Id":"spell1Id"]=0;dirty();render();return;}if(b.dataset.spell){const pair=[S.draft.spells?.spell1Id||0,S.draft.spells?.spell2Id||0],id=+b.dataset.spell;const other=1-S.slot;if(pair[other]===id)pair[other]=pair[S.slot];pair[S.slot]=id;S.draft.spells={spell1Id:pair[0],spell2Id:pair[1]};dirty();render();return;}if(b.dataset.item){addItem(+b.dataset.item,S.itemGroup);return;}if(b.dataset.build){S.itemGroup=b.dataset.build.split(":")[0];S.budgetError=false;render();root.querySelector(`[data-build="${b.dataset.build}"]`)?.focus({preventScroll:true});return;}if(b.dataset.remove){const [group,index]=b.dataset.remove.split(":");S.draft.items[group].splice(+index,1);dirty();render();return;}}catch(error){reportError(error);}});
  function addItem(id,target,index,source=null){
    const items=itemLists(),item=itemLookup(id);if(!item)return false;
    const snapshot=clone(items),starting=target==="starter";
    let count=1;
    if(source){const [group,from]=source.split(":");const value=items[group][+from];if(!value)return false;count=itemCount(value);items[group].splice(+from,1);}
    const restore=()=>{S.draft.items=snapshot;return false;};
    if(starting){
      if(!starterAllowed(item,S.mode))return restore();
      if(trinket(item))items.starter=items.starter.filter(v=>!trinket(itemLookup(v.id)));
      const same=items.starter.find(v=>v.id===id);
      if((same&&!consumable(item))||(!same&&items.starter.length>=6))return restore();
      if(starterTotal()+item.priceTotal*count>starterBudget(S.mode)){
        restore();S.budgetError=true;render();return false;
      }
      if(same){if(same.count+count>99)return restore();same.count+=count;}else items.starter.splice(Math.min(index??items.starter.length,items.starter.length),0,{id,count});
    }else{
      if(!itemAllowed(item,S.mode)||items[target].length>=6||[...items.core,...items.situational].includes(id)||count!==1)return restore();
      items[target].splice(Math.min(index??items[target].length,items[target].length),0,id);
    }
    S.budgetError=false;S.itemGroup=target;
    if(target==="starter"&&(starterRemaining()===0||items.starter.length>=6||!S.catalog.items.some(i=>starterAllowed(i,S.mode)&&i.priceTotal<=starterRemaining()&&((!items.starter.some(v=>v.id===i.id)&&items.starter.length<6)||consumable(i)&&(items.starter.find(v=>v.id===i.id)?.count||0)<99))))S.itemGroup="core";
    else if(target==="core"&&items.core.length>=6)S.itemGroup="situational";
    dirty();render();return true;
  }
  root.addEventListener("input",e=>{if(e.target.hasAttribute("data-champ-search")){S.q=e.target.value;render();}if(e.target.hasAttribute("data-item-search")){S.iq=e.target.value;updateShop();}});
  root.addEventListener("change",async e=>{const n=e.target;try{if(n.dataset.enable){publishSettings(await api("/api/auto-config/settings","PUT",{...S.settings,[n.dataset.enable]:n.checked}));S.error="";render();}if(n.dataset.filter){n.checked?S.filters.add(n.dataset.filter):S.filters.delete(n.dataset.filter);updateShop();}if(n.hasAttribute("data-sort")){S.sort=n.value;updateShop();}}catch(error){reportError(error);}});
  let drag=null;
  function clearDrop(){root.querySelectorAll(".is-drop-target").forEach(n=>n.classList.remove("is-drop-target"));}
  root.addEventListener("dragstart",e=>{const b=e.target.closest("[data-item],[data-build]");if(!b)return;hideTip();drag=b.dataset.build?{source:b.dataset.build,id:itemID(itemLists()[b.dataset.build.split(":")[0]][+b.dataset.build.split(":")[1]])}:{id:+b.dataset.item};if(!drag.id||b.getAttribute("aria-disabled")==="true"){e.preventDefault();drag=null;return;}e.dataTransfer.effectAllowed="move";e.dataTransfer.setData("text/plain",String(drag.id));});
  root.addEventListener("dragover",e=>{const b=e.target.closest("[data-build]");if(b&&drag){e.preventDefault();clearDrop();b.classList.add("is-drop-target");}});
  root.addEventListener("dragleave",e=>{const b=e.target.closest("[data-build]");if(b&&!b.contains(e.relatedTarget))b.classList.remove("is-drop-target");});
  root.addEventListener("drop",e=>{const b=e.target.closest("[data-build]");if(!b||!drag)return;e.preventDefault();clearDrop();const {id,source}=drag;drag=null;const [target,index]=b.dataset.build.split(":");addItem(id,target,+index,source);});
  root.addEventListener("dragend",()=>{drag=null;clearDrop();});
  root.addEventListener("keydown",e=>{if(e.target.matches("[data-item-group]")&&["Enter"," "].includes(e.key)){e.preventDefault();S.itemGroup=e.target.dataset.itemGroup;render();return;}const n=e.target.closest("[data-build]");if(n&&["Delete","Backspace"].includes(e.key)){e.preventDefault();const [k,i]=n.dataset.build.split(":");S.draft.items?.[k]?.splice(+i,1);dirty();render();}});
  async function load(force=false){await window.deepLegendsPreferenceStorage?.ready;if(S.settings&&!force){locatePending=true;await refreshCatalog();return;}try{const [settings,catalog,champions,runtime,keys,watch]=await Promise.all([api("/api/auto-config/settings"),api("/api/auto-config/catalog"),api("/api/champions/catalog"),api("/api/auto-config/state").catch(()=>({})),api("/api/settings/summoner-keys").catch(()=>({keys:["",""]})),api("/api/watch/rules").catch(()=>({}))]);S.settings=settings;S.catalog=normalizeCatalog(catalog);S.champions=champions.champions.sort((a,b)=>title(a).localeCompare(title(b),"zh-CN"));S.runtime=runtime;S.keys=keys;S.watch=watch;S.champ||=S.champions[0]?.id;loadDraft();render();metrics();void refreshMastery();}catch(error){root.innerHTML=`<p role="alert">${esc(error.message)} <button data-act="retry">重新读取</button></p>`;}}
  let masteryFlight=null,locateAfterMastery=true;
  async function refreshMastery(){if(masteryFlight)return masteryFlight;masteryFlight=(async()=>{let points={};try{const data=await api("/api/gameplay/masteries");if(data.available)points=Object.fromEntries((data.items||[]).map(row=>[row.championId,row.championPoints]));}catch(_){}if(!window.document)return;S.mastery=points;if(S.settings&&S.catalog){if(locateAfterMastery){locatePending=true;locateAfterMastery=false;}render();}})().finally(()=>masteryFlight=null);return masteryFlight;}
  function fillBody(){const body=root.querySelector(".ac-body");if(!body||root.closest("[hidden]"))return;const zoom=Number(getComputedStyle(document.documentElement).getPropertyValue("--ui-zoom"))||1;const value=`${Math.max(0,(innerHeight-body.getBoundingClientRect().top-24)/zoom)}px`;if(body.style.getPropertyValue("--ac-body-min")!==value)body.style.setProperty("--ac-body-min",value);}
  function metrics(){fillBody();const n=document.getElementById("suite-config-configured"),a=document.getElementById("suite-config-applied");if(n)n.textContent=String(Object.keys(S.settings?.champions||{}).filter(hasConfig).length);if(a)a.textContent=`${Object.values(S.runtime.results||{}).filter(v=>v==="applied").length}/3`;}
  let tooltip=null,tipAnchor=null;
  function hideTip(){if(tipAnchor)tipAnchor.removeAttribute("aria-describedby");tipAnchor=null;tooltip?.remove();tooltip=null;}
  function showTip(node){if(!node?.dataset.acTooltip)return;hideTip();tipAnchor=node;tooltip=document.createElement("div");tooltip.id="auto-config-tooltip";tooltip.className="ac-tip is-on";tooltip.setAttribute("role","tooltip");const [name,...body]=node.dataset.acTooltip.split("\n");const image=node.querySelector("img")?.getAttribute("src");const row=node.closest(".ac-row"),col=node.closest(".ac-col");const path=col?.dataset.path;const pathName=Object.keys(PATH).find(id=>PATH[id]===path);const shard=row?.classList.contains("is-shard"),where=shard?"属性碎片":row?`${style(+pathName)?.name||""} · ${row.classList.contains("is-key")?"基石":"小符文"}`:"";tooltip.innerHTML=`<div class="ac-tip-head">${image?`<img class="ac-tip-ic" src="${esc(image)}" alt="">`:""}<div><strong>${esc(name)}</strong><small>${esc(where)}</small></div></div><p>${esc(body.join("\n")).replace(/(\d+(?:\.\d+)?%?)/g,'<b class="n">$1</b>').replaceAll("\n","<br>")}</p>`;if(col)tooltip.style.setProperty("--path",getComputedStyle(shard?row.closest(".ac-group"):col).getPropertyValue("--path"));document.body.append(tooltip);node.setAttribute("aria-describedby",tooltip.id);const r=node.getBoundingClientRect(),t=tooltip.getBoundingClientRect();tooltip.style.left=`${Math.max(8,Math.min(innerWidth-t.width-8,r.x+r.width/2-t.width/2))}px`;tooltip.style.top=`${Math.max(8,r.top>t.height+12?r.top-t.height-8:Math.min(innerHeight-t.height-8,r.bottom+8))}px`;}
  root.addEventListener("mouseover",e=>{const n=e.target.closest("[data-ac-tooltip]");if(n!==tipAnchor)showTip(n);});
  root.addEventListener("mouseout",e=>{if(!tipAnchor?.contains(e.relatedTarget))hideTip();});
  root.addEventListener("focusin",e=>showTip(e.target.closest("[data-ac-tooltip]")));
  root.addEventListener("focusout",hideTip);root.addEventListener("keydown",e=>{if(e.key==="Escape")hideTip();});
  window.addEventListener("scroll",hideTip,true);
  window.deepLegendsAutoConfig={load,flushDraft};
  window.addEventListener("deep-legends:auto-config-settings",e=>{if(e.detail&&JSON.stringify(e.detail)!==JSON.stringify(S.settings)){syncSettings(e.detail);render();}});
  window.addEventListener("deep-legends:auto-config",e=>{S.runtime=e.detail||{};metrics();if(S.settings&&S.catalog){const card=root.querySelector(".ac-master");if(card)card.outerHTML=master();}});
  window.addEventListener("deep-legends:gameflow",e=>{if(e.detail?.phase&&S.catalog)void refreshCatalog().catch(reportError);});
  window.addEventListener("deep-legends:live-disconnected",()=>{if(S.catalog)void refreshCatalog().catch(reportError);void refreshMastery();});
  window.addEventListener("deep-legends:client-state-changed",()=>{locatePending=true;if(S.catalog)void refreshCatalog().catch(reportError);void refreshMastery();});
  window.addEventListener("focus",()=>{if(S.catalog&&!root.closest("[hidden]"))void refreshCatalog().catch(reportError);});
  let lastLayoutHeight=innerHeight;
  window.addEventListener("resize",()=>{if(!runeLayoutObserver||innerHeight!==lastLayoutHeight){lastLayoutHeight=innerHeight;scheduleLayout();}});
  window.addEventListener("deep-legends:section",e=>{flushDraft();if(e.detail?.name==="suite"){locatePending=true;locateChampion();}fillBody();});
  window.addEventListener("deep-legends:dispose",()=>{cancelAnimationFrame(layoutFrame);runeLayoutObserver?.disconnect();observedRuneTree=null;},{once:true});
  window.addEventListener("pagehide",()=>{flushDraft();void window.deepLegendsPreferenceStorage?.flush(true);});
  window.deepLegendsSections.register("auto-config",()=>{});
})();
