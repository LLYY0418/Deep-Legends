'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {runeFitGeometry}=require('./auto-config.js');
test('R313 rune fit keeps minimum diameters, uniform scale, four-keystone width and capped row gaps',()=>{
 for(const four of [false,true])for(const width of [320,640,800,1200,2000])for(const height of [200,382,450,600,1000]){const f=runeFitGeometry(width,height,four),w=f.primaryWidth+f.secondaryWidth+f.columnGap;assert.equal(f.scale,Math.max(1,Math.min(width/w,height/382,1.5)));assert(f.scale*60>=60&&f.scale*60<=90);assert((69*f.scale+f.minorExtra)/(46*f.scale)<=1.700001);assert((60*f.scale+f.subExtra)/(40*f.scale)<=1.700001);assert((44*f.scale+f.shardExtra)/(38*f.scale)<=1.700001);if(height<382)assert.equal(f.scale,1);if(four)assert(f.primaryWidth>=58+8+4*60);}
});
