(function(root,factory){const api=factory();if(typeof module==='object' && module.exports)module.exports=api;else{root.deepLegendsAnnouncements=api;root.deepLegendsSections?.register("announcements",()=>{});}})(typeof window==='undefined'?globalThis:window,()=>{
 'use strict';
 const escape=s=>String(s ?? '').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 function safeLink(s){try{const u=new URL(s);return ['https:','http:'].includes(u.protocol) && !u.username && !u.password?u.href:'';}catch{return '';}}
 function markdown(text,images=[]){
  text=String(text).replace(/<script\b[^>]*>[\s\S]*?<\/script\s*>/gi,'');
  const inline=s=>{
   const tokens=[];
   s=s.replace(/(!?)\[([^\]]*)\]\(([^\s)]+)\)/g,(_,img,label,url)=>{const index=images.indexOf(url);let html='';if(img){if(index>=0)html=`<img src="/api/announcements/image?path=${encodeURIComponent(url)}" alt="${escape(label)}" loading="lazy">`;}else{const href=safeLink(url);html=href?`<a href="${escape(href)}" target="_blank" rel="noopener noreferrer">${escape(label)}</a>`:escape(label);}tokens.push(html);return `\u0000${tokens.length-1}\u0000`;});
   return escape(s).replace(/\*\*([^*\n]+)\*\*/g,'<strong class="announcement-em">$1</strong>').replace(/\u0000(\d+)\u0000/g,(_,i)=>tokens[Number(i)] ?? '');
  };
  let list=false,result='';for(const line of text.split('\n')){const bullet=/^\s*[-*] (.*)$/.exec(line);if(bullet){if(!list){result+='<ul>';list=true;}result+=`<li>${inline(bullet[1])}</li>`;continue;}if(list){result+='</ul>';list=false;}const heading=/^(#{1,3}) (.*)$/.exec(line);if(heading)result+=`<h${heading[1].length+1}>${inline(heading[2])}</h${heading[1].length+1}>`;else if(/^> /.test(line))result+=`<blockquote>${inline(line.slice(2))}</blockquote>`;else if(line.trim())result+=`<p>${inline(line)}</p>`;}return result+(list?'</ul>':'');
 }
 function createReadState(storage){
  const key='deep-legends-announcements-read-v1',dismissedKey='deep-legends-announcements-dismissed-v1';
  const load=k=>{try{const value=JSON.parse(storage.getItem(k)||'[]');return Array.isArray(value)?value.filter(v=>typeof v==='string').slice(-256):[];}catch{return [];}};
  let read=load(key),dismissed=load(dismissedKey);const id=n=>n.id+':'+n.revision;
  const save=(key,list)=>{try{storage.setItem(key,JSON.stringify(list.slice(-256)));}catch{}};
  return {isRead:n=>read.includes(id(n)),isDismissed:n=>dismissed.includes(id(n)),dismiss(n){if(!dismissed.includes(id(n)))dismissed.push(id(n));save(dismissedKey,dismissed);},read(n){if(!read.includes(id(n)))read.push(id(n));save(key,read);}};
 }

 function mount(w,initial=null){
  const doc=w.document,button=doc.getElementById('announcements-button'),dialog=doc.getElementById('announcements-dialog');if(!button || !dialog)return;
  const requestJSON=async(path)=>{const response=await w.fetch(path,{headers:{Accept:'application/json'},signal:w.AbortSignal.timeout(4000)});if(!response.ok)throw Error('notice fetch failed');return response.json();};
  const read=createReadState(w.deepLegendsPreferenceStorage || w.localStorage),attempted=new Set();let notices=initial?.announcements || [],source=initial?.source || 'bundled',phase=initial?.phase || 'unknown',visible=initial?.visible ?? !w.desktopWindow,manualPending=false,fetching=false,stopped=false;
  const active=new Set(['ChampSelect','GameStart','InProgress','Reconnect']);
  const report=(n,action)=>w.reportFlowDiagnostic?.('announcement_state',action,{id:n?.id || 'fetch',notice_revision:n?.revision || '',source});
  const present=()=>notices.filter(n=>!n.expiresAt || Date.parse(n.expiresAt)>Date.now());
  const badge=()=>{const unread=present().filter(n=>!read.isRead(n));button.classList.toggle('has-announcement-unread',Boolean(unread.length));button.dataset.tooltip=unread.length?`公告：${unread[0].summary}`:'公告';};
  const busy=()=>doc.querySelector('dialog[open]:not(#announcements-dialog),[aria-modal="true"]:not(dialog):not([hidden]),[role="dialog"]:not(dialog):not([hidden])') || doc.activeElement?.matches('input,textarea,select,[contenteditable="true"]') || !doc.getElementById('license-overlay')?.hidden && doc.documentElement.dataset.license==='enabled';
  function render(){const content=dialog.querySelector('[data-announcements-content]');content.innerHTML=present().map(n=>{
   const first=String(n.content || '').trimStart().split('\n')[0],heading=/^#{1,3}\s+(.+?)\s*$/.exec(first);
   const body=heading && heading[1]===n.title?markdown(n.content,n.images || []):`<h3>${escape(n.title)}</h3>${markdown(n.content,n.images || [])}`;
   return `<article><div class="announcement-markdown">${body}</div><footer><button type="button" class="${read.isRead(n)?'text-button':'button button-primary'}" data-announcement-read="${escape(n.id)}">${read.isRead(n)?'关闭':'已读'}</button></footer></article>`;
  }).join('') || '<p>暂无公告</p>';}

  function show(){if(stopped || !visible || doc.hidden || busy())return false;render();dialog.showModal();for(const n of present())report(n,'shown');return true;}
  function pump(){badge();if(dialog.open || !visible || doc.hidden || busy())return;if(manualPending){manualPending=false;show();return;}if(phase==='unknown' || active.has(phase))return;const next=present().find(n=>n.popup && !read.isRead(n) && !read.isDismissed(n) && !attempted.has(n.id+':'+n.revision));if(next && show())for(const n of present())attempted.add(n.id+':'+n.revision);}
  function dismiss(){if(!dialog.open)return;for(const n of present()){read.dismiss(n);report(n,'dismissed');}void w.deepLegendsPreferenceStorage?.flush?.();dialog.close();badge();w.dispatchEvent(new w.CustomEvent('deep-legends:modal-idle'));}
  button.addEventListener('click',()=>{manualPending=true;pump();});dialog.querySelector('[data-announcements-close]').addEventListener('click',dismiss);
  dialog.addEventListener('cancel',e=>{e.preventDefault();dismiss();});dialog.addEventListener('click',e=>{const b=e.target.closest('[data-announcement-read]');if(b){const n=present().find(n=>n.id===b.dataset.announcementRead);if(n && !read.isRead(n)){read.read(n);void w.deepLegendsPreferenceStorage?.flush?.();report(n,'read');}dialog.close();badge();w.dispatchEvent(new w.CustomEvent('deep-legends:modal-idle'));return;}if(e.target===dialog){const r=dialog.getBoundingClientRect();if(e.clientX<r.left || e.clientX>r.right || e.clientY<r.top || e.clientY>r.bottom)dismiss();}});
  const observer=new w.MutationObserver(entries=>{if(entries.some(e=>e.target.matches('dialog,[aria-modal],[role="dialog"],#license-overlay')))pump();});observer.observe(doc.body,{subtree:true,attributes:true,attributeFilter:['open','hidden']});
  doc.addEventListener('focusout',()=>w.queueMicrotask(pump));w.addEventListener('deep-legends:modal-idle',pump);
  w.addEventListener('deep-legends:gameflow',e=>{phase=e.detail?.phase || 'None';pump();});doc.addEventListener('visibilitychange',pump);w.addEventListener('deep-legends:announcements-changed',()=>void load());
  w.desktopWindow?.onVisible(()=>{visible=true;pump();});
  w.addEventListener('deep-legends:riot-relay-recovered',()=>void load());
  async function load(){if(fetching || stopped)return;fetching=true;try{const data=await requestJSON('/api/announcements',{timeoutMs:4000});if(!Array.isArray(data?.announcements))return;notices=data.announcements.slice().sort((a,b)=>Date.parse(b.updatedAt)-Date.parse(a.updatedAt));source=data.source || 'bundled';pump();}catch{report(null,'fetch-failed');}finally{fetching=false;}}
  async function startup(){try{const data=await requestJSON('/api/gameplay/phase',{timeoutMs:2000});phase=data?.phase || 'None';}catch{phase='None';}await load();}
  const timer=w.setInterval(()=>void load(),3600000);w.addEventListener('beforeunload',()=>{stopped=true;w.clearInterval(timer);observer.disconnect();},{once:true});if(initial)pump();else void startup();
  return {pump,dismiss,load,open(){manualPending=true;pump();}};
 }
 return Object.freeze({markdown,safeLink,createReadState,mount});
});
