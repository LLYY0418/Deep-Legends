'use strict';
// Executed in Chromium so oklch/color-mix and alpha resolve in the rendering engine.
function scanContrast(rootSelector=""){
 const root=rootSelector?document.querySelector(rootSelector):document.body;
 const canvas=document.createElement('canvas');canvas.width=canvas.height=1;const ctx=canvas.getContext('2d',{willReadFrequently:true});
 const rgba=color=>{ctx.clearRect(0,0,1,1);ctx.fillStyle=color;ctx.fillRect(0,0,1,1);return [...ctx.getImageData(0,0,1,1).data].map((v,i)=>i===3?v/255:v);};
 const over=(a,b)=>[...a.slice(0,3).map((v,i)=>v*a[3]+b[i]*(1-a[3])),1];
 const luminance=c=>c.slice(0,3).map(x=>x/255).map(x=>x<=.04045?x/12.92:((x+.055)/1.055)**2.4).reduce((v,x,i)=>v+x*[.2126,.7152,.0722][i],0);
 const ratio=(a,b)=>{const x=luminance(a),y=luminance(b);return (Math.max(x,y)+.05)/(Math.min(x,y)+.05);};
 const selector=n=>n.id?'#'+n.id:n.tagName.toLowerCase()+(n.classList.length?'.'+[...n.classList].join('.'):''),out=[],exceptions=[],samples=[];let checked=0;
 const context=n=>{const chain=[];let opacity=1,complex=false;for(let p=n;p;p=p.parentElement){const s=getComputedStyle(p);chain.unshift(s);opacity*=Number(s.opacity);if(s.backgroundImage!=='none')complex=true;}let bg=[255,255,255,1];for(const s of chain)bg=over(rgba(s.backgroundColor),bg);return {bg,opacity,complex};};
 const visible=n=>{if(!n.checkVisibility({checkOpacity:true,checkVisibilityCSS:true}) || n.closest('.sr-only,[hidden],.gameplay-skeleton'))return false;const r=n.getBoundingClientRect();return r.width>0 && r.height>0 && r.bottom>0 && r.top<innerHeight && r.right>0 && r.left<innerWidth;};
 const obscured=(n,x,y)=>{const flag=document.querySelector('.demo-flag');if(!flag||flag.contains(n))return false;const r=flag.getBoundingClientRect();return x>=r.left&&x<=r.right&&y>=r.top&&y<=r.bottom;};
 const disabled=n=>n.closest(':disabled,[aria-disabled="true"]');
 const check=(n,text,color,limit,kind,boxes)=>{
  const {bg,opacity,complex}=context(n),fg=rgba(color);fg[3]*=opacity;const contrast=ratio(over(fg,bg),bg);checked++;const rects=boxes||[n.getBoundingClientRect().toJSON()],points=[];for(const b of rects)for(const xf of [.15,.35,.5,.65,.85])for(const yf of [.3,.5,.7]){const x=b.left+b.width*xf,y=b.top+b.height*yf;if(x<0||y<0||x>=innerWidth||y>=innerHeight)continue;const hit=document.elementFromPoint(x,y);if(hit && n.contains(hit) && !obscured(n,x,y))points.push({x,y});}if(kind==='icon')n.dataset.r282BgIcon='true';samples.push({selector:selector(n),text:text.slice(0,90),required:limit,kind,rgba:fg,points,disabled:Boolean(disabled(n))});
  if(contrast>=limit)return;
  const row={selector:selector(n),text:text.slice(0,90),contrast:Number(contrast.toFixed(2)),required:limit,kind,color,background:bg};
  if(disabled(n)){exceptions.push({...row,reason:'disabled control: WCAG inactive component exception'});return;}
  // Artwork and gradients require visual/screenshot verification, not a fabricated flat-color score.
  if((complex || n.closest('.multikill-tag')) && n.closest('.skin-copy,.mk-plate,.multikill-tag,.match-badge-chip,.facade-art-label,.summoner-strip,.account-hero')){exceptions.push({...row,reason:'artwork/gradient: verify against captured pixels and scrim'});return;}
  out.push(row);
 };
 const walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);for(let node;node=walker.nextNode();){const text=node.textContent.trim(),n=node.parentElement;if(!text || !n || !visible(n) || n.closest('svg,script,style,option'))continue;if(n.closest('.game-icon>[aria-hidden="true"]')){exceptions.push({selector:selector(n),text,reason:'decorative image fallback is aria-hidden; the labelled image/control supplies its meaning'});continue;}const r=document.createRange();r.selectNodeContents(node);const box=r.getBoundingClientRect();if(box.width<1 || box.height<1 || box.bottom<=0 || box.top>=innerHeight)continue;const hit=document.elementFromPoint(Math.min(innerWidth-1,Math.max(0,box.left+box.width/2)),Math.min(innerHeight-1,Math.max(0,box.top+box.height/2)));if(hit && !n.contains(hit))continue;const s=getComputedStyle(n),font=Number.parseFloat(s.fontSize),large=font>=24 || font>=18.66&&Number(s.fontWeight)>=700,tag=n.closest('[class*="tag"],[class*="badge"],[class*="chip"],.arena-option-rarity,.position-pill');check(n,text,s.color,large||tag||/^[›▸▾×↻◆◉★⬡🔒✓⌖◎!]$/.test(text)?3:4.5,'text',[...r.getClientRects()].map(x=>x.toJSON()));}
 for(const svg of root.querySelectorAll('button svg,.section-tab svg,.friend-open-hint svg')){if(!visible(svg))continue;const box=svg.getBoundingClientRect(),hit=document.elementFromPoint(Math.max(0,Math.min(innerWidth-1,box.x+box.width/2)),Math.max(0,Math.min(innerHeight-1,box.y+box.height/2)));if(hit && !svg.contains(hit))continue;const path=svg.querySelector('path,rect,circle,polygon');if(!path)continue;const s=getComputedStyle(path),color=s.stroke!=='none'?s.stroke:s.fill;if(color==='none' || color.startsWith('url('))continue;check(svg,'[icon]',color,3,'icon');}
 return {checked,violations:out,exceptions,samples,viewport:{width:innerWidth,height:innerHeight}};
}
module.exports={scanContrast};
