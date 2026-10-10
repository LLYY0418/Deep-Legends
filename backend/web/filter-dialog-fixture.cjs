const install=dom=>{const proto=dom.window.HTMLDialogElement.prototype;proto.showModal=function(){this.setAttribute('open','');this._modal=true;};proto.close=function(){this.removeAttribute('open');this._modal=false;};const matches=proto.matches;proto.matches=function(selector){return selector===':modal'?Boolean(this._modal):matches.call(this,selector);};};
const q=(root,selector)=>root.querySelector(selector) || (root._afDialog?.matches(selector)?root._afDialog:root._afDialog?.querySelector(selector));
const qa=(root,selector)=>{const nodes=root.querySelectorAll(selector);return nodes.length?nodes:root._afDialog?.querySelectorAll(selector) || [];};
module.exports={install,q,qa};
