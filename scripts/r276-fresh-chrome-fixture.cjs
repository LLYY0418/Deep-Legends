'use strict';
// Navigation does not promise a fresh renderer heap. R269's preceding scenarios
// must finish before the original R266 cold-fixture assertions start.
async function freshChromeFixture({send,targetId,sessionId,browserContextId:previousContextId,closedSessions,initialize}){
 closedSessions.add(sessionId);
 await send('Target.closeTarget',{targetId});
 if(previousContextId)await send('Target.disposeBrowserContext',{browserContextId:previousContextId});
 const {browserContextId}=await send('Target.createBrowserContext');
 const next=await send('Target.createTarget',{url:'about:blank',browserContextId});
 const attached=await send('Target.attachToTarget',{targetId:next.targetId,flatten:true});
 await initialize({targetId:next.targetId,sessionId:attached.sessionId,browserContextId});
 return {targetId:next.targetId,sessionId:attached.sessionId,browserContextId};
}
module.exports={freshChromeFixture};
