'use strict';
// Only local protocol endpoints absent from the corresponding demo are substituted.
// All page rendering, CSP, other API calls and guards stay real.
function browserAPI(url){
 if(url.pathname==='/api/gameplay/client-recommendation'){
  const championId=Number(url.searchParams.get('championId')),position=url.searchParams.get('position'),mapId=Number(url.searchParams.get('mapId')||11);
  if(!Number.isSafeInteger(championId)||championId<=0||!['top','jungle','mid','adc','support','other'].includes(position)||![11,12,30].includes(mapId))return;
  return {championId,clientRecommendation:{id:`client-fixture-${championId}-${position}-${mapId}`,name:'客户端内置推荐',source:'synthetic',championId,position,primaryStyleId:8000,subStyleId:8400,selectedPerkIds:[8005,9111,9104,8014,8473,8451]}};
 }
 if(url.pathname==='/api/announcements')return {source:'synthetic',announcements:[]};
 if(url.pathname==='/api/gameplay/summoner-spells')return {source:'synthetic',spells:[{id:4,name:'闪现'},{id:14,name:'引燃'}]};
 if(url.pathname==='/api/champions/rankings' && url.searchParams.get('mode')==='ranked')return {
  mode:'ranked',region:'KR',source:'synthetic',patch:'16.18',fetchedAt:'2026-09-20T00:00:00Z',
  rows:[{championId:157,key:'yasuo',name:'疾风剑豪',rank:1,tier:2,play:100,winRate:52.5,pickRate:10,tierLocallyCalculated:false}],
  tierBands:[{tier:1,label:'前15%'}]
 };
}
function r264API(url){
 if(['/api/announcements','/api/gameplay/client-recommendation'].includes(url.pathname))return browserAPI(url);
 if(url.pathname==='/api/champions/rankings'&&url.searchParams.get('mode')==='ranked'&&url.searchParams.get('tier')==='emerald_plus'&&url.searchParams.get('position')==='all')return browserAPI(url);
}
function r265API(url,method){
 if(method==='GET'&&url.pathname==='/api/gameplay/summoner-spells')return browserAPI(url);
 if(method==='GET')return r264API(url);
 if(method==='POST'&&url.pathname==='/api/gameplay/match-tiers')return {tiers:[]};
}
module.exports={browserAPI,r264API,r265API};

// The three legacy static layout servers lack these local protocol endpoints.
// Exact method/path fixtures; unlisted requests retain their original routing.
function serveStaticAPI(req,res){
 const url=new URL(req.url,'http://fixture'),pathname=url.pathname,method=req.method;
 if(method==='POST'&&['/api/diagnostics/client','/api/pro-players/visibility','/api/client-launch-overview-ready'].includes(pathname)){res.statusCode=204;res.end();return true;}
 if(method==='GET'&&pathname==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});res.write(': synthetic fixture stream\n\n');return true;}
 if(method==='GET'&&['/api/image','/api/champion-asset'].includes(pathname)){res.setHeader('Content-Type','image/svg+xml');res.end('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#587c83"/></svg>');return true;}
 let data;
 if(method==='GET'&&pathname==='/api/gameplay/client-recommendation')data=browserAPI(url);
 if(method==='GET'&&pathname==='/api/announcements')data={source:'synthetic',announcements:[]};
 if(method==='GET'&&pathname==='/api/gameplay/summoner-spells')data={source:'synthetic',spells:[{id:4,name:'闪现'},{id:11,name:'惩戒'},{id:14,name:'引燃'}]};
 if(method==='GET'&&pathname==='/api/champions/network')data={mode:'direct',active:'synthetic fixture'};
 if(method==='GET'&&pathname==='/api/gameplay/mayhem-rating')data={available:false,unavailableReason:'synthetic fixture'};
 if(method==='GET'&&pathname==='/api/gameplay/pro-runes')data={pros:[],reason:'synthetic fixture'};
 if(method==='POST'&&pathname==='/api/gameplay/season-summary')data={available:false,champions:[],overall:{}};
 if(data){res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));return true;}
 return false;
}
module.exports.serveStaticAPI=serveStaticAPI;
