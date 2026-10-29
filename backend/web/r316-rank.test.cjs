'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {read,compile,escapeHTML}=require('./r188-harness.cjs');
test('R316 tooltip displays backend labels and dates without guessing from the start month',()=>{
 const source=read('gameplay.js');const {renderRankWindowNote:f}=compile(source,['renderRankWindowNote'],{escapeHTML});
 const rank={recordSplitStart:Date.UTC(2040,8,1),recordSplitLabel:'2027 赛季第一赛段',recordSplitFrom:'1月7日'};
 assert.match(f({},[rank]),/数据范围：2027 赛季第一赛段\n统计自 1月7日 起/);
 assert.match(f({},[{...rank,recordSplitLabel:''}]),/数据范围：当前赛段\n统计自 1月7日 起/);
 assert.match(f({},[rank],true),/统计的是当前整个赛季的排位数据/);
});
