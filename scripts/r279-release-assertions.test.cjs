'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const publishedRegistry=require('./r279-release-assertions.json');
// The target release already changed this contract before integration; retain both explicit version baselines.
const targetVersion=require('../desktop/package.json').version;
const registry=targetVersion==='0.12.86'?require('./r280-release-assertions.json'):publishedRegistry;
function preserved(source,assertions){const counts=new Map();for(const a of assertions)counts.set(a,(counts.get(a)||0)+1);for(const [text,count]of counts)assert(source.split(text).length-1>=count,'published assertion must remain verbatim: '+text);}
for(const [file,assertions]of Object.entries(registry))test('published assertions remain verbatim '+file,()=>preserved(fs.readFileSync(path.join(__dirname,'..',file),'utf8'),assertions));
test('mutation: deleting a published assertion is detected',()=>{const [file,assertions]=Object.entries(registry)[0];const source=fs.readFileSync(path.join(__dirname,'..',file),'utf8');assert.throws(()=>preserved(source.replace(assertions[0],'true'),assertions),/must remain verbatim/);});

test('the 0.12.86 target registry keeps every original guard and only follows its existing reserved-rank-height contract',()=>{
 const expected=structuredClone(publishedRegistry),file='scripts/r277-frontend-browser-checks.cjs';
 expected[file]=expected[file].map(a=>a==='assert(arrival.after>arrival.before,JSON.stringify({region,...arrival}))'?'assert(Math.abs(arrival.after-arrival.before)<=2,JSON.stringify({region,...arrival}))':a);
 assert.deepEqual(require('./r280-release-assertions.json'),expected);
});
