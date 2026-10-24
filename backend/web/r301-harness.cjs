'use strict';
function clock(){let now=0,id=0;const timers=new Map();const flush=async()=>{for(let i=0;i<16;i++)await Promise.resolve();};return {now:()=>now,setTimeout(fn,ms=0){const key=++id;timers.set(key,{fn,at:now+ms});return key;},clearTimeout(key){timers.delete(key);},async advance(ms){const end=now+ms;await flush();for(;;){const row=[...timers].filter(([,v])=>v.at<=end).sort((a,b)=>a[1].at-b[1].at||a[0]-b[0])[0];if(!row)break;now=row[1].at;timers.delete(row[0]);row[1].fn();await flush();}now=end;await flush();},flush};}
module.exports={clock};
