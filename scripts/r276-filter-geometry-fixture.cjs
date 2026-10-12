'use strict';
// Opening the real modal locks scrolling. Its ResizeObserver/tag layout work
// belongs to setup, and must complete before the original draft-click guard.
async function settleFilterGeometry(evaluate){
 await evaluate('(async()=>{await document.fonts?.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));})()');
}
module.exports={settleFilterGeometry};
