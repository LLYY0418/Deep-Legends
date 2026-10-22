(() => {
  "use strict";
  const enumOf = (...values) => ({values});
  const integer = (min,max) => ({min,max});
  const toggle = {enabled:"boolean"};
  const timed = {...toggle,delayMs:integer(0,300000)};
  const pools = Object.fromEntries(["default","top","jungle","middle","bottom","utility"].map(key=>[key,{array:integer(1,10000),maxLength:20}]));
  const side = {...timed,strategy:enumOf("show-only","show-then-lock","lock-now"),lockDelayMs:integer(0,300000),avoidTeammateIntent:"boolean",champions:pools,legacyLaneChampions:pools};
  const group = {ban:side,pick:side,bench:{...toggle,holdMs:integer(0,300000),preferFirst:"boolean",handleTrade:"boolean"}};
  const watchSchema = {
    schemaVersion:integer(1,10),masterEnabled:"boolean",
    rules:{autoAccept:timed,autoReconnect:timed,autoPlayAgain:toggle,autoHonor:{...toggle,strategy:"string"},skipCelebration:toggle,
      positionBroadcast:{...toggle,visibility:"string",teamComposition:"boolean",assignedPosition:"boolean"},promoteLeader:toggle,
      invitations:{...toggle,policies:{map:enumOf("accept","decline","ignore"),keyPattern:/^(?:default|[0-9]{1,5})$/}},
      autoMatchmaking:{...timed,minPartySize:integer(1,5)}},
    facade:{statusMessageEnabled:"boolean",statusMessage:"string",rankEnabled:"boolean",rank:{rankedLeagueQueue:"string",rankedLeagueTier:"string",rankedLeagueDivision:"string"}},
    champSelect:{enabled:"boolean",groups:Object.fromEntries(["ranked","normal","aram","arena","event","practice"].map(key=>[key,group]))},
  };
  const rendererSchema = {
    theme:enumOf("auto","light","dark","azure","emerald","violet","crimson","aurora","oled"),density:enumOf("comfortable","compact"),sidebar:enumOf("expanded","collapsed"),
    "ui-scale":enumOf("auto","0.9","1","1.1","1.25","1.4","1.5","1.75","2"),
    "default-page":enumOf("overview","champions","live","favorites","suite"),"match-count":enumOf("10","20","30","40","50"),
    "default-match-filter":enumOf("all","solo","flex"),"champion-position":enumOf("all","top","jungle","mid","adc","support"),
    "live-order":enumOf("team","position","kda","win-rate"),"live-refresh":enumOf("true","false"),"live-interval":enumOf("3","5","10","15","30","60"),
    "mask-names":enumOf("true","false"),"confirm-replay":enumOf("true","false"),"auto-live":enumOf("true","false"),
    "live-tags":{array:enumOf("self","premade","pro","hidden","autofill","squad","positions"),maxLength:7},
  };
  const autoEntry = {runes:{primaryStyleId:integer(1,100000),subStyleId:integer(1,100000),selectedPerkIds:{array:integer(0,100000),maxLength:9}},spells:{spell1Id:integer(1,100000),spell2Id:integer(1,100000)},items:{core:{array:integer(1,1000000),maxLength:6},situational:{array:integer(1,1000000),maxLength:6}},updatedAt:"string"};
  const autoConfigSchema = {version:integer(1,Number.MAX_SAFE_INTEGER),enabled:"boolean",runes:"boolean",spells:"boolean",items:"boolean",champions:{map:{map:autoEntry,keyPattern:/^(?:ranked(?:-(?:top|jungle|middle|bottom|utility))?|normal|aram|hextech|urf|event)$/,maxKeys:11},keyPattern:/^[1-9][0-9]{0,3}$/,maxKeys:1000}};
  const schema = {exportDirectory:"string",renderer:rendererSchema,native:{contentProtection:"boolean",closeBehavior:enumOf("exit","tray"),disableHardwareAcceleration:"boolean"},automatic:"boolean",autoDownload:"boolean",blockAutostart:"boolean",network:{mode:enumOf("auto","direct","manual"),url:"string"},watch:watchSchema,autoConfig:autoConfigSchema,summonerKeys:{flashSlot:{nullable:integer(0,1)}}};
  function sanitize(value, shape = schema) {
    if (shape.nullable) return value === null ? null : sanitize(value,shape.nullable);
    if (shape === "boolean" || shape === "string") {
      if (typeof value !== shape || shape === "string" && value.length > 1024) throw new Error("设置格式无效");
      return value;
    }
    if (shape.values) { if (!shape.values.includes(value)) throw new Error("设置格式无效"); return value; }
    if (shape.min !== undefined) { if (!Number.isInteger(value) || value < shape.min || value > shape.max) throw new Error("设置格式无效"); return value; }
    if (shape.map) {
      if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).length > (shape.maxKeys || 64)) throw new Error("设置格式无效");
      return Object.fromEntries(Object.entries(value).filter(([key])=>shape.keyPattern.test(key)).map(([key,item])=>[key,sanitize(item,shape.map)]));
    }
    if (shape.array) {
      if (!Array.isArray(value) || value.length > shape.maxLength) throw new Error("设置格式无效");
      return value.map(item=>sanitize(item,shape.array));
    }
    if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("设置格式无效");
    const result = {};
    for (const key of Object.keys(shape)) if (Object.hasOwn(value,key)) result[key] = sanitize(value[key],shape[key]);
    if (shape === schema && result.network?.url) {
      const proxy = new URL(result.network.url);
      if (proxy.username || proxy.password) throw new Error("代理地址不能包含用户名或密码");
    }
    return result;
  }
  function exportProxyURL(value) {
    if (!value) return "";
    const proxy = new URL(value);
    if (!proxy.username && !proxy.password) return value;
    proxy.username = ""; proxy.password = "";
    return proxy.href;
  }
  const formatStars = count => Number.isSafeInteger(count) && count >= 0 ? count.toLocaleString("en-US") : "";
  const backup = Object.freeze({sanitize,rendererSchema,formatStars});
  if (typeof module === "object" && module.exports) { module.exports = backup; return; }
  window.deepLegendsSettingsBackup = backup;
  // Persistence can be blocked (including opaque origins). Keep the current
  // session usable without changing any existing storage key.
  const memory = new Map();
  const preferenceStorage = globalThis.deepLegendsPreferenceStorage || {
    getItem(key) { try { const value=localStorage.getItem(key); if(value!==null){memory.set(key,value);return value;} } catch(_) {} return memory.get(key) ?? null; },
    setItem(key,value) { memory.set(key,String(value)); try { localStorage.setItem(key,String(value)); } catch(_) {} },
    removeItem(key) { memory.delete(key); try { localStorage.removeItem(key); } catch(_) {} },
  };
  globalThis.deepLegendsPreferenceStorage = preferenceStorage;
  const get = id=>document.getElementById(id);
  const controls = {
    theme:"setting-theme","ui-scale":"setting-ui-scale","default-page":"setting-default-page","match-count":"setting-match-count","default-match-filter":"setting-default-match-filter",
    "champion-position":"setting-champion-position","live-order":"setting-live-order","live-refresh":"setting-live-refresh","live-interval":"setting-live-interval","mask-names":"setting-mask-names","confirm-replay":"setting-confirm-replay","auto-live":"setting-auto-live",
  };
  async function api(url,body) {
    const response = await fetch(url,{method:body === undefined ? "GET":"POST",headers:{"Accept":"application/json","Content-Type":"application/json"},...(body === undefined ? {}:{body:JSON.stringify(body)})});
    if (!response.ok) throw new Error("设置保存失败");
    return response.json();
  }
  async function putAPI(path,body) {const r=await fetch(path,{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});if(!r.ok)throw Error(await r.text());return r.json();}
  function toast(text) {
    const node=get("toast"); node.textContent=text; node.hidden=false;
    clearTimeout(toast.timer); toast.timer=setTimeout(()=>{node.hidden=true;},3200);
  }
  function modal(title,content) {
    const dialog=document.createElement("dialog"); dialog.className="settings-modal";
    const header=document.createElement("header"),heading=document.createElement("h2"),close=document.createElement("button");
    heading.textContent=title; close.textContent="关闭"; close.className="text-button"; close.type="button";
    header.append(heading,close); dialog.append(header,content); document.body.append(dialog);
    close.addEventListener("click",()=>dialog.close()); dialog.addEventListener("close",()=>dialog.remove(),{once:true});
    dialog.showModal();
  }
  window.deepLegendsSettingsModal = modal;
  let initialized=false;
  window.deepLegendsSections.register("settings-preferences",()=>{
    if(initialized)return; initialized=true;
    const measureScrollbar=()=>{const scroll=document.querySelector(".app-scroll");if(scroll)document.documentElement.style.setProperty("--settings-scrollbar",`${scroll.offsetWidth-scroll.clientWidth}px`);};
    measureScrollbar();window.addEventListener("resize",measureScrollbar);
    const dot=document.querySelector(".settings-new-dot"); if(dot)dot.hidden=preferenceStorage.getItem("lol-loot-settings-app-seen")==="true";
    const markAppSeen=()=>{if(!get("settings-panel").hidden && !get("settings-app-panel").hidden){preferenceStorage.setItem("lol-loot-settings-app-seen","true");if(dot)dot.hidden=true;void preferenceStorage.flush?.();}};
    const appPageObserver=new MutationObserver(markAppSeen);for(const node of [get("settings-panel"),get("settings-app-panel")])appPageObserver.observe(node,{attributes:true,attributeFilter:["hidden"]});
    markAppSeen();window.addEventListener("deep-legends:dispose",()=>appPageObserver.disconnect(),{once:true});
    get("settings-github").addEventListener("click",event=>{
      event.preventDefault();
      if(window.desktopPreferences)void window.desktopPreferences.openGithub();
      else window.open("https://github.com/LLYY0418/Deep-Legends","_blank","noopener,noreferrer");
    });
    const showStars = count => {
      const node=get("settings-github-count"),text=formatStars(count);
      node.textContent=text; node.hidden=!text;
    };
    void api("/api/github/stars").then(value=>showStars(value.count)).catch(()=>showStars(null));
    let flashKeys;
    function renderFlashKeys(value){flashKeys=value;const host=get("setting-flash-keys");if(!host)return;host.replaceChildren();for(let i=0;i<2;i++){const b=document.createElement("button"),kbd=document.createElement("kbd");kbd.className="ac-kbd";kbd.textContent=value.keys[i]||"—";b.append(kbd,document.createTextNode(i?"右侧技能":"左侧技能"));b.type="button";b.disabled=!value.keys[i];b.className=value.flashSlot===i?"is-active":"";b.setAttribute("aria-pressed",String(value.flashSlot===i));b.onclick=async()=>{try{renderFlashKeys(await putAPI("/api/settings/summoner-keys",{flashSlot:flashKeys.flashSlot===i?null:i}));}catch(e){toast(e.message);}};host.append(b);}}
    const refreshFlashKeys=()=>void api("/api/settings/summoner-keys").then(renderFlashKeys).catch(()=>{});
    refreshFlashKeys();window.addEventListener("deep-legends:section",e=>{if(e.detail?.name==="settings")refreshFlashKeys();});
    const nativeKeys={"setting-content-protection":"contentProtection","setting-close-behavior":"closeBehavior","setting-disable-hardware":"disableHardwareAcceleration"};
    const syncNative=value=>{
      for(const [id,key] of Object.entries(nativeKeys)){const node=get(id);if(node.type==="checkbox")node.checked=value[key];else node.value=value[key];}
      get("setting-hardware-restart").hidden=value.disableHardwareAcceleration===value.hardwareAccelerationDisabledAtStartup;
    };
    for(const [id,key] of Object.entries(nativeKeys)){
      const node=get(id);node.disabled=!window.desktopPreferences;
      node.addEventListener("change",async()=>{
        node.disabled=true;
        try{syncNative(await window.desktopPreferences.update({[key]:node.type==="checkbox"?node.checked:node.value}));}
        catch(error){syncNative(await window.desktopPreferences.get());toast(error.message);}
        finally{node.disabled=false;}
      });
    }
    void window.desktopPreferences?.get().then(syncNative).catch(()=>{for(const id of Object.keys(nativeKeys))get(id).disabled=true;});
    const syncUpdate = value => {
      get("setting-auto-update").checked = value.automatic !== false;
      get("setting-auto-download").checked = value.autoDownload !== false;
      get("setting-auto-download").disabled = value.automatic === false;
      get("setting-auto-download-row").classList.toggle("is-disabled", value.automatic === false);
    };
    for (const [id,key] of [["setting-auto-update","automatic"],["setting-auto-download","autoDownload"]]) {
      get(id).addEventListener("change",async()=>{
        const node=get(id);get("setting-auto-update").disabled=true;get("setting-auto-download").disabled=true;
        try { const settings=await api("/api/update/settings"); syncUpdate(await api("/api/update/settings",{...settings,[key]:node.checked})); }
        catch(error) { node.checked=!node.checked; toast(error.message); }
        finally { get("setting-auto-update").disabled=false; get("setting-auto-download").disabled=!get("setting-auto-update").checked; get("setting-auto-download-row").classList.toggle("is-disabled",!get("setting-auto-update").checked); }
      });
    }
    get("setting-block-autostart").addEventListener("change",async()=>{
      const node=get("setting-block-autostart");node.disabled=true;
      try{await api("/api/client-autostart",{block:node.checked});}catch(error){node.checked=!node.checked;toast(error.message);}finally{node.disabled=false;}
    });
    void api("/api/update/settings").then(syncUpdate).catch(()=>{get("setting-auto-update").disabled=true;get("setting-auto-download").disabled=true;});
    void api("/api/client-autostart").then(value=>{get("setting-block-autostart").checked=value.block!==false;}).catch(()=>{get("setting-block-autostart").disabled=true;});
    get("setting-release-notes").addEventListener("click",async()=>{
      try{const data=await api("/api/release-notes"),content=document.createElement("pre");content.textContent=data.notes;modal("本版更新内容",content);}catch(error){toast(error.message);}
    });
    for(const kind of ["data","logs"]){const node=get(`setting-open-${kind}`);node.disabled=!window.desktopPreferences;node.addEventListener("click",()=>void window.desktopPreferences.openDirectory(kind));}
    async function createBackup(){
      const renderer={};
      for(const key of Object.keys(rendererSchema)){
        let value=preferenceStorage.getItem(`lol-loot-${key}`);
        if(value===null && controls[key]){const node=get(controls[key]);value=node.type==="checkbox"?String(node.checked):node.value;}
        if(value!==null)renderer[key]=key==="live-tags"?JSON.parse(value):value;
      }
      const [native,update,autostart,network,watch,autoConfig,summonerKeys]=await Promise.all([window.desktopPreferences?.get()||{},api("/api/update/settings"),api("/api/client-autostart"),api("/api/champions/network"),api("/api/watch/rules"),api("/api/auto-config/settings"),api("/api/settings/summoner-keys")]);
      return sanitize({...(window.desktopShare?.getSaveDirectory ? {exportDirectory:(await window.desktopShare.getSaveDirectory()).directory||""}:{}),renderer,native,automatic:update.automatic!==false,autoDownload:update.autoDownload!==false,blockAutostart:autostart.block!==false,network:{mode:network.mode,url:exportProxyURL(network.url||"")},watch,autoConfig,summonerKeys:{flashSlot:summonerKeys.flashSlot}});
    }
    async function applyBackupValues(data, notify=true){
      let count=0;
      if(data.autoConfig){const current=await api("/api/auto-config/settings");await putAPI("/api/auto-config/settings",{...data.autoConfig,version:current.version});count++;}
      if(data.summonerKeys){await putAPI("/api/settings/summoner-keys",data.summonerKeys);count++;}

      if(data.watch){
        const merge=(current,patch)=>{const result={...current};for(const [key,value] of Object.entries(patch))result[key]=value&&typeof value==="object"&&!Array.isArray(value)?merge(current?.[key]||{},value):value;return result;};
        const current=sanitize({watch:await api("/api/watch/rules")}).watch;
        await api("/api/watch/rules",merge(current,data.watch));count++;
      }
      if(data.exportDirectory!==undefined && window.desktopShare?.setSaveDirectory){await window.desktopShare.setSaveDirectory(data.exportDirectory);count++;}
      if(data.network){await api("/api/champions/network",data.network);count++;}
      if(data.native && Object.keys(data.native).length && window.desktopPreferences){syncNative(await window.desktopPreferences.update(data.native));count+=Object.keys(data.native).length;}
      if(data.automatic!==undefined || data.autoDownload!==undefined){const settings=await api("/api/update/settings");syncUpdate(await api("/api/update/settings",{...settings,...(data.automatic===undefined?{}:{automatic:data.automatic}),...(data.autoDownload===undefined?{}:{autoDownload:data.autoDownload})}));count++;}
      if(data.blockAutostart!==undefined){await api("/api/client-autostart",{block:data.blockAutostart});get("setting-block-autostart").checked=data.blockAutostart;count++;}
      for(const [key,value] of Object.entries(data.renderer||{})){
        if(key==="density"||key==="sidebar"){
          const defaults=key==="density"?"comfortable":"expanded";
          if((preferenceStorage.getItem(`lol-loot-${key}`)||defaults)!==value)get(key==="density"?"density-toggle":"settings-sidebar-toggle").click();
        }else{
          preferenceStorage.setItem(`lol-loot-${key}`,key==="live-tags"?JSON.stringify(value):value);
          const node=controls[key] ? get(controls[key]) : null;if(node){if(node.type==="checkbox")node.checked=value==="true";else node.value=value;node.dispatchEvent(new Event("change",{bubbles:true}));}
        }
        count++;
      }
      window.dispatchEvent(new CustomEvent("deep-legends:settings-imported"));
      if(notify)toast(`已导入 ${count} 项`);
      return count;
    }
    async function applyBackup(raw){
      const data=sanitize(raw); // Validate every recognized field before any write.
      const previous=await createBackup();
      try { return await applyBackupValues(data); }
      catch(error) {
        try { await applyBackupValues(previous,false); }
        catch(rollbackError) { error.cause=rollbackError; }
        throw error;
      }
    }
    get("setting-backup-export").addEventListener("click",async()=>{
      try{const data=await createBackup(),url=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:"application/json"})),link=document.createElement("a");link.href=url;link.download="deep-legends-settings.json";link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(error){toast(error.message);}
    });
    get("setting-backup-import").addEventListener("click",()=>get("setting-backup-file").click());
    get("setting-backup-file").addEventListener("change",async event=>{
      try{const file=event.target.files[0];if(!file)return;if(file.size>2*1024*1024)throw new Error("设置格式无效");await applyBackup(JSON.parse(await file.text()));}catch(error){toast(error.message);}finally{event.target.value="";}
    });
    window.deepLegendsSettingsBackup = Object.freeze({...backup,createBackup,applyBackup});
  });
})();
