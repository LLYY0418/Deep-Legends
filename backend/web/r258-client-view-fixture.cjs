'use strict';
// Convert historical test scenarios into the current server presentation model.
// Production has no legacy-field fallback. New view-only tests use literal frames.
let sequence=0;
function clientView(status, generation=++sequence) {
 const region=status.clientRegion||'',serverId=status.serverId||'';
 const ready=Boolean(status.connected && status.identityReady && (region||serverId) && (region!=='TENCENT'||status.sgpReady));
 return {type:'client-view',state:status.clientDiscovery==='exiting'?'exiting':ready?'ready':'no-client',summoner:status.summoner||{},region,serverId,sgpReady:status.sgpReady===true,generation,clientVersion:status.clientVersion||''};
}
function viewStatus(status,generation) {return {...status,clientView:clientView(status,generation)};}
module.exports={clientView,viewStatus};
