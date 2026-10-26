'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events'),{attachWindowResizeTrace}=require('./window-resize-trace.cjs');
test('R307 native resize trace caps each drag at sixty samples and always ends with summary',()=>{
 const w=new EventEmitter(),sent=[];Object.assign(w,{isDestroyed:()=>false,getBounds:()=>({x:0,y:0,width:1200,height:900}),getContentBounds:()=>({x:0,y:0,width:1200,height:860}),getMinimumSize:()=>[780,600],getBackgroundColor:()=>"#0e0c0d",webContents:{send:(channel,value)=>sent.push({channel,...value})}});let at=1000,finish;
 attachWindowResizeTrace(w,{scale:()=>1.1,theme:()=>"crimson",now:()=>at,setTimer:fn=>(finish=fn,1),clearTimer(){}});
 for(let i=0;i<300;i++){w.emit('will-resize',{},w.getBounds());w.emit('resize');at+=40;}w.emit('resized');assert.equal(sent.length,61);assert.equal(sent.at(-1).reason,'summary');assert.equal(sent.at(-1).sample,60);assert.equal(sent.at(-1).nativeBackground,'#0e0c0d');assert.equal(sent.at(-1).theme,'crimson');assert.equal(sent[0].zoom,1.1);assert.deepEqual(sent[0].minimum,[780,600]);
 w.emit('resize');finish();assert.equal(sent.at(-1).sample,1);w.emit('closed');assert.equal(w.listenerCount('resize'),0);
});
