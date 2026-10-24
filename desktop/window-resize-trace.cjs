'use strict';
// Observe only: never alter bounds, minimum size or window decoration.
function attachWindowResizeTrace(window,{scale,now=Date.now,setTimer=setTimeout,clearTimer=clearTimeout}={}){
 let drag=0,count=0,last=-Infinity,timer=null;
 const emit=(reason,next)=>{if(window.isDestroyed())return;window.webContents.send('desktop-window-resize-trace',{reason,drag,sample:count,bounds:window.getBounds(),contentBounds:window.getContentBounds(),minimum:window.getMinimumSize(),zoom:scale(),...(next?{nextBounds:next}:{})});};
 const finish=()=>{clearTimer(timer);timer=null;if(!drag)return;emit('summary');drag=0;};
 const sample=(reason,next)=>{if(!drag){drag=now()||1;count=0;last=-Infinity;}clearTimer(timer);timer=setTimer(finish,250);if(count>=60||now()-last<32)return;last=now();count++;emit(reason,next);};
 const will=(_event,bounds)=>sample('will-resize',bounds),resize=()=>sample('resize');
 window.on('will-resize',will);window.on('resize',resize);window.on('resized',finish);
 window.once('closed',()=>{clearTimer(timer);window.removeListener('will-resize',will);window.removeListener('resize',resize);window.removeListener('resized',finish);});
}
module.exports={attachWindowResizeTrace};
