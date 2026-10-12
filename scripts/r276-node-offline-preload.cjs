'use strict';
// Node fetch ignores the proxy variables on some CI Node versions. Enforce the
// same loopback-only rule at its socket boundary, including future guard code.
const net=require('node:net');
const connect=net.Socket.prototype.connect;
function loopback(host){return !host||host==='localhost'||host==='::1'||host==='[::1]'||/^127\./.test(host);}
net.Socket.prototype.connect=function(...args){
 const value=Array.isArray(args[0])?args[0]:args;
 const options=typeof value[0]==='object'?value[0]:typeof value[0]==='number'?{port:value[0],host:typeof value[1]==='string'?value[1]:undefined}:{path:value[0]};
 if(!options.path&&!loopback(options.host)){
  const error=Error('R276 external Node socket blocked: '+options.host);error.code='R276_UPSTREAM_BLOCKED';throw error;
 }
 return connect.apply(this,args);
};
