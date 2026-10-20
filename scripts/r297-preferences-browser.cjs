'use strict';
const {browser}=require('./r297-browser-harness.cjs'),path=require('node:path'),fs=require('node:fs'),assert=require('node:assert/strict');
const out=path.resolve(__dirname,'../docs/history/reports/r297/evidence/p3-browser'),backend=process.argv[2];
(async()=>{
 const results=[];
 for(const run of [0,1]){
  const b=await browser({out:path.join(out,'run-'+run),backend});
  try{
   await b.call('Page.navigate',{url:b.origin+'/?demo'});
   await b.until('window.__r297 && !document.documentElement.dataset.uiPreferences');
   if(run===0){
    await b.until("document.getElementById('announcements-dialog').open");
    await b.evaluate("document.querySelector('[data-announcements-close]').click()");
    assert(await b.evaluate("document.getElementById('announcements-button').classList.contains('has-announcement-unread')"));
    await b.evaluate("window.dispatchEvent(new CustomEvent('deep-legends:navigate',{detail:{section:'settings',page:'app'}}))");
    await b.until("window.deepLegendsSettingsBackup && !document.getElementById('settings-app-panel').hidden");
    await b.evaluate("deepLegendsPreferenceStorage.setItem('lol-loot-theme','light');deepLegendsPreferenceStorage.flush()");
    assert.equal(await b.evaluate("deepLegendsPreferenceStorage.getItem('lol-loot-settings-app-seen')"),'true');
   }else{
    assert.equal(await b.evaluate("document.documentElement.dataset.theme"),'light');
    await b.until("document.getElementById('announcements-button').classList.contains('has-announcement-unread')");
    assert.equal(await b.evaluate("document.getElementById('announcements-dialog').open"),false);
    await b.evaluate("document.querySelector('[data-section=settings]').click()");
    await b.until('window.deepLegendsSettingsBackup');
    assert.equal(await b.evaluate("document.querySelector('.settings-new-dot').hidden"),true);
    await b.evaluate("document.getElementById('announcements-button').click()");
    await b.until("document.getElementById('announcements-dialog').open");
    await b.evaluate("document.querySelector('[data-announcement-read]').click();deepLegendsPreferenceStorage.flush()");
    assert.equal(await b.evaluate("document.getElementById('announcements-button').classList.contains('has-announcement-unread')"),false);
   }
   await b.shot('state');assert.deepEqual(b.errors,[]);results.push({run,origin:b.origin,theme:await b.evaluate('document.documentElement.dataset.theme'),dotHidden:await b.evaluate("document.querySelector('.settings-new-dot').hidden"),errors:b.errors});
  }catch(e){await b.shot('failure');throw e}finally{await b.stop()}
 }
 assert.notEqual(results[0].origin,results[1].origin);fs.writeFileSync(path.join(out,'result.json'),JSON.stringify({results},null,2));console.log(JSON.stringify({results}));
})().catch(e=>{console.error(e);process.exitCode=1});
