'use strict';

// Native intersections include clipping by nested scroll containers. A viewport
// rectangle alone also includes lazy images hidden above/below their scroller.
async function waitForVisibleImages(page,selector='img') {
 await page.evaluate(`(async()=>{
  const images=[...document.querySelectorAll(${JSON.stringify(selector)})];
  const target=img=>img.closest('.champion-asset,.game-icon')||img;
  const targets=new Set(images.map(target)),visible=new Set();
  if(targets.size)await new Promise(resolve=>{
   const pending=new Set(targets);
   const observer=new IntersectionObserver(entries=>{
    for(const entry of entries){pending.delete(entry.target);if(entry.isIntersecting&&entry.intersectionRatio>0)visible.add(entry.target);}
    if(!pending.size){observer.disconnect();resolve();}
   });
   for(const node of targets)observer.observe(node);
  });
  // Intersections do not account for sticky headers covering an icon. Probe
  // its painted area too, retaining pointer-events:none artwork via ancestors.
  const exposed=node=>{
   const r=node.getBoundingClientRect(),left=Math.max(0,r.left),right=Math.min(innerWidth,r.right),top=Math.max(0,r.top),bottom=Math.min(innerHeight,r.bottom);
   if(right<=left||bottom<=top)return false;
   const inset=Math.min(1,(right-left)/4,(bottom-top)/4);
   return [[(left+right)/2,(top+bottom)/2],[left+inset,top+inset],[right-inset,top+inset],[left+inset,bottom-inset],[right-inset,bottom-inset]].some(([x,y])=>{const hit=document.elementFromPoint(x,y);return hit&&(node.contains(hit)||hit.contains(node));});
  };
  window.__captureVisibleImages=images.filter(img=>visible.has(target(img))&&exposed(target(img)));
 })()`);
 try {
  await page.until(`window.__captureVisibleImages.every(img=>img.getAttribute('src')&&img.complete&&img.naturalWidth>0&&!img.hidden)`);
 } catch(error) {
  const pending=await page.evaluate(`window.__captureVisibleImages.filter(img=>!img.getAttribute('src')||!img.complete||!img.naturalWidth||img.hidden).map(img=>({alt:img.alt,src:img.getAttribute('src'),queued:img.dataset.queuedSrc,hidden:img.hidden,natural:img.naturalWidth,holder:img.parentElement.className,visibility:getComputedStyle(img.parentElement).visibility,hiddenParent:img.parentElement.closest('[hidden]')?.id,rect:JSON.stringify(img.parentElement.getBoundingClientRect())}))`);
  error.message+='; pending visible images: '+JSON.stringify(pending);
  throw error;
 }
 await page.evaluate(`Promise.all(window.__captureVisibleImages.map(img=>img.decode()))`);
}

module.exports={waitForVisibleImages};
