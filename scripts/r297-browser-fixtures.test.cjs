'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {browserAPI,r264API,r265API,serveStaticAPI}=require('./r276-browser-api-fixtures.cjs');
test('R297 demo client recommendation fixtures follow the requested target and keep a complete rune page',()=>{
 for(const championId of [1,164,799]){
  const url=new URL(`http://fixture/api/gameplay/client-recommendation?championId=${championId}&position=top&mapId=11`);
  for(const read of [browserAPI,r264API,u=>r265API(u,'GET')]){
   const value=read(url);assert.equal(value.championId,championId);assert.equal(value.clientRecommendation.championId,championId);
   assert.equal(value.clientRecommendation.position,'top');assert.equal(new Set(value.clientRecommendation.selectedPerkIds).size,6);
  }
  let text;const res={setHeader(){},end(body){text=body}};
  assert.equal(serveStaticAPI({method:'GET',url:url.pathname+url.search},res),true);
  assert.equal(JSON.parse(text).championId,championId);
 }
 assert.equal(browserAPI(new URL('http://fixture/api/gameplay/client-recommendation?championId=-1')),undefined);
});
