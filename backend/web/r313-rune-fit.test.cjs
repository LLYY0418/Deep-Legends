'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {runeFitGeometry}=require('./auto-config.js');
test('R313 rune fit keeps minimum diameters, uniform scale, four-keystone width and capped row gaps',()=>{
 for(const four of [false,true])for(const width of [320,640,800,1200,2000])for(const height of [200,315,382,450,600,1000]){
  const fit=runeFitGeometry(width,height,four),s=fit.scale,w=fit.primaryWidth+fit.secondaryWidth+fit.columnGap;
  assert.equal(s,Math.max(1,Math.min(width/w,height/315,1.2)));assert(s*54>=54&&s*54<=64.8);assert(fit.f>=1&&fit.f<=1.4);
  assert(Math.abs((78*s+fit.keyExtra)/(78*s)-fit.f)<1e-10);assert(Math.abs((64*s+fit.minorExtra)/(64*s)-fit.f)<1e-10);assert(Math.abs((54*s+fit.subExtra)/(54*s)-fit.f)<1e-10);
  const left=(56+238*fit.f+21)*s,right=56*s+3*(54*s+fit.subExtra)+fit.subShardGap+2.5*(32*s+fit.shardExtra)+14*s;
  assert(Math.abs(left-right)<1e-8);assert.equal(fit.treeHeight,left);assert(fit.subShardGap>=2*s-1e-8);assert(fit.f===1.4||fit.treeHeight>=height-1e-8);
  if(height<315)assert.equal(s,1);if(four)assert(fit.primaryWidth>=52.2+7.2+4*54);
 }
});
