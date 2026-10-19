'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const api=require('./announcements.js');
test('R288 only strong syntax is added and HTML remains escaped',()=>{
 const html=api.markdown('从每晚**21**点左右 **<b>注入</b>** [官网](https://example.com/) ![图](images/a.png) *普通*', ['images/a.png']);
 const dom=new JSDOM(html);
 try {
  const doc=dom.window.document;
  assert.deepEqual([...doc.querySelectorAll('strong')].map(n=>[n.className,n.textContent]),[['announcement-em','21'],['announcement-em','<b>注入</b>']]);
  assert.equal(doc.querySelector('b'),null);
  assert.equal(doc.querySelector('a').href,'https://example.com/');
  assert.equal(doc.querySelector('img').getAttribute('src'),'/api/announcements/image?path=images%2Fa.png');
  assert.match(doc.body.textContent,/\*普通\*/);
 } finally {dom.window.close()}
});
test('R288 a body edit keeps read revision closed and legacy renderer has no stars',()=>{
 const n=require('../announcements/announcements.json')[0],memory=new Map(),storage={getItem:k=>memory.get(k),setItem:(k,v)=>memory.set(k,v)};
 api.createReadState(storage).read({...n,content:'旧正文'});
 assert.equal(n.revision,'1');assert(api.createReadState(storage).isRead(n));
 const legacy=n.content.replaceAll('**21**','21');
 assert.match(legacy,/从每晚21点左右/);assert(!legacy.includes('**'));
});
