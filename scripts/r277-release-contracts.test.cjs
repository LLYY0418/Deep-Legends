"use strict";
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'..');
test('R277 release version and Chinese user-facing changelog are prepared',()=>{
 const pkg=JSON.parse(fs.readFileSync(path.join(root,'desktop/package.json'),'utf8'));
 const lock=JSON.parse(fs.readFileSync(path.join(root,'desktop/package-lock.json'),'utf8'));
 for(const version of [pkg.version,lock.version,lock.packages[''].version])assert.equal(version,pkg.version);
 const log=fs.readFileSync(path.join(root,'CHANGELOG.md'),'utf8');
 const entry=log.split('## 0.12.84')[1]?.split('\n## ')[0];assert.ok(entry,'missing 0.12.84 changelog');
 for(const topic of ['客户端','战绩','倒计时','收藏','筛选','缩放','赛段','安装器'])assert.ok(entry.includes(topic),'missing user-facing topic: '+topic);
 assert.doesNotMatch(entry,/goroutine|LPARAM|flight|SHA|P1–P12/);
});
