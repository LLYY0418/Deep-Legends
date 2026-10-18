"use strict";
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'..');
test('R278 P15 version and Chinese changelog match the release worklist',()=>{
 const pkg=require(process.env.R278_PACKAGE_FILE || '../desktop/package.json'),lock=require('../desktop/package-lock.json');for(const v of [pkg.version,lock.version,lock.packages[''].version])assert.equal(v,'0.12.90');
 const entry=fs.readFileSync(path.join(root,'CHANGELOG.md'),'utf8').split('## 0.12.85')[1]?.split('\n## ')[0];assert(entry);for(const topic of ['登录','战绩','额度','筛选','收藏','公告','更新','日志'])assert(entry.includes(topic));assert.doesNotMatch(entry,/goroutine|LCU|SGP|HTTP|SHA|P1–P14/);
});

test('R278 P12 public and dormant license entry points both include announcement runtime',()=>{
 for(const name of ['backend/web/default/index.html','backend/web/index.html']){const html=fs.readFileSync(name.endsWith('default/index.html')?(process.env.R278_DEFAULT_HTML || path.join(root,name)):path.join(root,name),'utf8');for(const token of ['id="announcements-button"','id="announcements-dialog"','/app.js'])assert(html.includes(token),name+' '+token);assert(!html.includes('/announcements-bootstrap.js'),'bootstrap must stay out of first screen')}
 assert(fs.readFileSync(path.join(root,'desktop/source-fingerprint.cjs'),'utf8').includes('backend/announcements/announcements.json'));
});
