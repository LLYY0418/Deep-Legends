'use strict';
// Only the two catalog endpoints missing from the ordinary R266 demo are
// substituted. All page rendering, CSP, other API calls and guards stay real.
function browserAPI(url){
 if(url.pathname==='/api/gameplay/summoner-spells')return {source:'synthetic',spells:[{id:4,name:'闪现'},{id:14,name:'引燃'}]};
 if(url.pathname==='/api/champions/rankings' && url.searchParams.get('mode')==='ranked')return {
  mode:'ranked',region:'KR',source:'synthetic',patch:'16.18',fetchedAt:'2026-09-20T00:00:00Z',
  rows:[{championId:157,key:'yasuo',name:'疾风剑豪',rank:1,tier:2,play:100,winRate:52.5,pickRate:10,tierLocallyCalculated:false}],
  tierBands:[{tier:1,label:'前15%'}]
 };
}
module.exports={browserAPI};
