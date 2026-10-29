'use strict';
let reporter=()=>{};
const reported=new WeakSet();
function setResizeBorderReporter(report){reporter=report;}
function installResizeBorder(window,{platform=process.platform,load=()=>require('./native/resize-border.node'),report=reporter}={}){
 if(platform!=='win32')return false;
 try {const installed=load().install(window.getNativeWindowHandle());if(!installed)throw Error("native installation unavailable");return true;}
 catch(error){
  const key=window && (typeof window==='object'||typeof window==='function')?window:installResizeBorder;
  if(!reported.has(key)){
   reported.add(key);
   const error_type=['Error','TypeError','RangeError','SyntaxError','ReferenceError'].includes(error?.name)?error.name:'Error';
   try{report({event:'resize_border_unavailable',error_type});}catch(_){}
  }
  return false;
 }
}
module.exports={installResizeBorder,setResizeBorderReporter};
