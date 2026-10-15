'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const registry=require('./r279-release-assertions.json');
function preserved(source,assertions){const counts=new Map();for(const a of assertions)counts.set(a,(counts.get(a)||0)+1);for(const [text,count]of counts)assert(source.split(text).length-1>=count,'published assertion must remain verbatim: '+text);}
for(const [file,assertions]of Object.entries(registry))test('published assertions remain verbatim '+file,()=>preserved(fs.readFileSync(path.join(__dirname,'..',file),'utf8'),assertions));
test('mutation: deleting a published assertion is detected',()=>{const [file,assertions]=Object.entries(registry)[0];const source=fs.readFileSync(path.join(__dirname,'..',file),'utf8');assert.throws(()=>preserved(source.replace(assertions[0],'true'),assertions),/must remain verbatim/);});
