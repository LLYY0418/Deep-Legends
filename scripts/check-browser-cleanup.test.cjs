'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),path=require('node:path'),fs=require('node:fs'),os=require('node:os');
const {inspect,audit}=require('./check-browser-cleanup.cjs');
test('browser cleanup gate rejects direct kill/removal, unowned deletion timers and lost await',()=>{
 for(const source of [
  'proc.kill();fs.rmSync(temp,{recursive:true});',
  'chrome.kill();setTimeout(()=>fs.rmSync(temp),250).unref();',
  "const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');async function main(){await launchChrome(opts)}main().finally(()=>{cleanupChromeTest({})});",
  'const child=spawn("chrome");child.kill();fs.rmSync(profile);'
  ,"const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');async function capture(){await launchChrome(opts);fs.rmSync(arbitraryProfile);await cleanupChromeTest({})}"
 ])assert(inspect(source,'new-browser.cjs').length,source);
});
test('browser cleanup gate accepts awaited shared ownership and ignores fixture strings/comments',()=>{
 for(const source of [
  "const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');async function main(){await launchChrome(opts)}main().finally(()=>cleanupChromeTest({children:[proc],directories:[temp]}));",
  "const {cleanupChromeTest}=require('./chrome-test-cleanup.cjs');async function capture(){await launchChrome(opts);try{await guard()}finally{await cleanupChromeTest({})}}",
  "const fixture=`proc.kill();fs.rmSync(temp)`;// proc.kill();fs.rmSync(temp)\nsetTimeout(()=>finish(),5);"
 ])assert.deepEqual(inspect(source,'safe-browser.cjs'),[],source);
});
test('all actual browser entry points pass the shared cleanup audit',()=>assert.deepEqual(audit(path.resolve(__dirname,'..')).issues,[]));
test('cleanup audit includes a newly added guard in a nested directory',t=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'r276-cleanup-gate-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 fs.mkdirSync(path.join(root,'desktop'));fs.mkdirSync(path.join(root,'scripts','browser'),{recursive:true});
 fs.copyFileSync(path.join(__dirname,'chrome-test-cleanup.cjs'),path.join(root,'scripts','chrome-test-cleanup.cjs'));
 fs.writeFileSync(path.join(root,'scripts','browser','future-guard.cjs'),'child.kill();fs.rmSync(profile);');
 assert(audit(root).issues.some(row=>row.file==='scripts/browser/future-guard.cjs'));
});
