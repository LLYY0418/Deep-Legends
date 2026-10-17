'use strict';
const fs=require('node:fs'),{decodePNG}=require('./png-pixels.cjs'),{scanContrast}=require('./r282-contrast.cjs');
const luminance=c=>c.map(x=>x/255).map(x=>x<=.04045?x/12.92:((x+.055)/1.055)**2.4).reduce((v,x,i)=>v+x*[.2126,.7152,.0722][i],0);
const contrast=(fg,bg)=>{const a=fg[3],color=fg.slice(0,3).map((v,i)=>v*a+bg[i]*(1-a)),x=luminance(color),y=luminance(bg);return(Math.max(x,y)+.05)/(Math.min(x,y)+.05)};
async function inspect(p,out,name,root=''){
 const sampled=await p.evaluate('('+scanContrast.toString()+')('+JSON.stringify(root)+')');
 let screenshot;
 try{await p.evaluate("(()=>{const s=document.createElement('style');s.id='r282-background-sample';s.textContent='*{-webkit-text-fill-color:transparent!important;text-shadow:none!important}[data-r282-bg-icon]{visibility:hidden!important}';document.head.append(s);return new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))})()");screenshot=await p.call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});}
 finally{await p.evaluate("document.getElementById('r282-background-sample')?.remove()");}
 const bytes=Buffer.from(screenshot.data,'base64'),png=decodePNG(bytes),scaleX=png.width/sampled.viewport.width,scaleY=png.height/sampled.viewport.height,violations=[],exceptions=[...sampled.exceptions.filter(e=>e.reason.startsWith('decorative'))],measurements=[];
 fs.writeFileSync(out+'/'+p.theme+'-'+name+'-background.png',bytes);
 for(const sample of sampled.samples){let minimum=Infinity,point;
  for(const coordinate of sample.points){const x=coordinate.x*scaleX,y=coordinate.y*scaleY;if(x<0||y<0||x>=png.width||y>=png.height)continue;const bg=png.pixel(x,y),ratio=contrast(sample.rgba,bg);if(ratio<minimum){minimum=ratio;point={x:Math.floor(x),y:Math.floor(y),background:bg}}}
  if(!Number.isFinite(minimum))continue;const {points,...row}=sample;row.contrast=Number(minimum.toFixed(2));row.sample=point;measurements.push(row);
  if(minimum>=sample.required)continue;
  if(sample.disabled){exceptions.push({...row,reason:'inactive control: WCAG exception'});continue;}
  if(sample.rgba[3]<.01){exceptions.push({...row,reason:'gradient foreground: flat foreground color is transparent; inspect original screenshot'});continue;}
  violations.push(row);
 }
 return {viewport:sampled.viewport,png:{width:png.width,height:png.height},checked:measurements.length,violations,exceptions,measurements,method:'foreground RGBA against 15 samples per text rectangle from a Chromium screenshot with text fill hidden; SVG foreground hidden for icon backgrounds'};
}
module.exports={inspect,contrast};
