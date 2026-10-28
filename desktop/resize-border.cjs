'use strict';
function installResizeBorder(window,{platform=process.platform,load=()=>require('./native/resize-border.node')}={}){
 if(platform!=='win32')return false;
 return load().install(window.getNativeWindowHandle());
}
module.exports={installResizeBorder};
