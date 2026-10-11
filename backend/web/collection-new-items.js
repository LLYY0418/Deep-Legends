/* R272: account-local ownership baseline and read markers; never writes to LCU. */
(function(root,factory){const api=factory();if(typeof module==='object' && module.exports)module.exports=api;else root.deepLegendsCollectionNewItems=api.create(root.localStorage);})(typeof window==='undefined'?globalThis:window,()=>{
 'use strict';
 function create(storage){
  let account='',record=null;
  const prefix='deep-legends-collection-new-v1:';
  function persist(){if(!account || !record)return;try{storage.setItem(prefix+account,JSON.stringify(record));}catch{}}
  function setAccount(status){
   const s=status?.summoner, id=status?.collectionAccount || s?.summonerId || s?.puuid || s?.playerRef;
   const next=status?.connected && id?`${status.clientRegion || ''}:${status.serverId || ''}:${id}`:'';
   if(next===account)return account;account=next;record=null;
   if(account){try{record=JSON.parse(storage.getItem(prefix+account) || 'null');}catch{}if(!record || record.version!==1 || !record.kinds || typeof record.kinds!=='object')record={version:1,kinds:{}};}
   return account;
  }
  function update(kind,items){
   if(!account || !record || !Array.isArray(items))return;
   const initial=!record.kinds[kind],prior=record.kinds[kind] || {seen:[],unread:[],read:[]};
   const clean=values=>Array.isArray(values)?values.filter(v=>typeof v==='string').slice(-50000):[];
   const seen=new Set(clean(prior.seen)),unread=new Set(clean(prior.unread)),read=new Set(clean(prior.read));
   for(const item of items){if(!item?.owned || item.id==null)continue;const id=String(item.id);
    if(!initial && !read.has(id) && (item.newItem===true || !seen.has(id)))unread.add(id);
    seen.add(id);
   }
   record.kinds[kind]={seen:[...seen],unread:[...unread],read:[...read]};persist();
  }
  function isNew(kind,id){return Boolean(record?.kinds[kind]?.unread?.includes(String(id)));}
  function read(kind,id,owner=account){
   if(owner!==account)return;const entry=record?.kinds[kind];if(!entry)return;
   id=String(id);entry.unread=entry.unread.filter(v=>v!==id);if(!entry.read.includes(id))entry.read.push(id);persist();
  }
  function decorate(card,kind,id){
   const owner=account;card.classList.toggle('is-new-collection-item',isNew(kind,id));
   card._newCollectionItem={owner,kind,id};
   if(!card._newCollectionBound){card._newCollectionBound=true;card.addEventListener('mouseenter',()=>{const item=card._newCollectionItem;read(item.kind,item.id,item.owner);card.classList.remove('is-new-collection-item');});}
  }
  return Object.freeze({setAccount,update,isNew,read,decorate});
 }
 return Object.freeze({create});
});
