(function(root,factory){const api=factory(root);if(typeof module==='object'&&module.exports)module.exports=api;else root.deepLegendsPrivacy=api;})(typeof window==='undefined'?globalThis:window,root=>{
 'use strict';
 const lists=new Map();
 function masked(){try{return root.localStorage?.getItem('lol-loot-mask-names')==='true'}catch{return false}}
 function name(player={},options={}){
  const enabled=options.masked ?? masked();
  if(!enabled){if(player.identityUnresolved)return '身份待公开';if(player.hidden)return '隐藏玩家';return `${player.gameName || player.displayName || '隐藏玩家'}${options.fullName===false?'':player.tagLine?'#'+player.tagLine:''}`;}
  const champion=options.championName || (['match','live'].includes(options.context)?player.championName:'');
  if(champion)return String(champion);
  const context=options.context || 'identity';let list=lists.get(context);if(!list){list=new Map();lists.set(context,list);}
  const identity=player.playerRef || player.gameName || player.displayName;
  let number=Math.max(1,Number(options.index || 0)+1);
  if(identity){if(!list.has(identity)){const used=new Set(list.values());while(used.has(number))number++;if(list.size>=1024)list.clear();list.set(identity,number);}number=list.get(identity);}
  return `召唤师 ${number}`;
 }
 return Object.freeze({name,masked});
});
