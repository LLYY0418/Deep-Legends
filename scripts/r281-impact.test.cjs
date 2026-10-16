"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),path=require("node:path");
const {query,identifiersIn}=require("./contract-impact.cjs"),{analyze}=require("./fixture-alignment-check.cjs"),{inventory}=require("./lib/source-inventory.cjs");
const root=path.resolve(__dirname,"..");
const fixture=files=>({files:Object.keys(files),read:file=>files[file]});
test("contract impact reports every direct reference and separately marks file assertions",()=>{
 const source=fixture({"scripts/a-browser.cjs":"const key='search-server-id';\nassert.equal(controls.searchServer,'');", "backend/web/a.test.cjs":"assert.ok(document.querySelector('#player-search-region'));", "backend/web/app.js":"const value='search-server-id';"});
 const result=query(source,["search-server-id","#player-search-region"]);
 assert.deepEqual(result.files.map(row=>row.file),["backend/web/a.test.cjs","scripts/a-browser.cjs"]);
 assert.equal(result.files[0].references[0].assertion,true);assert.equal(result.files[1].references[0].line,1);assert.equal(result.files[1].assertions[0].line,2);
 assert.deepEqual(query(source,["absent"]).files,[]);
});
test("changed identifiers include removed preference, added DOM/API and changed UI copy",()=>{
 const found=identifiersIn(`- preference('search-server-id','');\n+ document.getElementById('player-search-region');api('/api/announcements');const label='重新加载';`);
 for(const token of ['search-server-id','#player-search-region','/api/announcements','重新加载'])assert(found.includes(token),token);
});
test("real contract inventory includes all persisted controls guards and outage countdown assertions",()=>{
 const result=query(inventory(root),['search-server-id','search-region-manual','#player-search-region','retryText','秒']);
 for(const file of ['scripts/r265-persisted-ui.cjs','scripts/r266-persisted-ui.cjs','scripts/r269-persisted-ui.cjs','scripts/r272-persisted-ui.cjs','scripts/r264-browser.cjs'])assert(result.files.some(row=>row.file===file&&row.assertions.length),file);
});
test("fixture alignment catches direct API bypass and delegated guard; an import alone cannot mask it",()=>{
 const files={'backend/web/index.html':'<script src="/app.js"></script>','backend/web/app.js':"api('/api/announcements');initAnnouncements();",'scripts/a-browser.cjs':"const {browserAPI}=require('./r276-browser-api-fixtures.cjs');on('Fetch.requestPaused',()=>send('Fetch.continueRequest'));",'scripts/wrapper.cjs':"require('./a-browser.cjs');"};
 const entries=[['direct','scripts/a-browser.cjs'],['wrapper','scripts/wrapper.cjs']];
 let report=analyze(fixture(files),entries);assert.deepEqual(report.issues.map(row=>row.guard),['direct','wrapper']);assert(report.startupEndpoints.some(row=>row.path==='/api/announcements'));assert(report.automaticUI.length);
 files['scripts/a-browser.cjs']="const unused=browserAPI(new URL(url));on('Fetch.requestPaused',()=>send('Fetch.continueRequest'));";
 assert.equal(analyze(fixture(files),entries).issues.length,2,'unrelated shared call cannot hide bypass');
 files['scripts/a-browser.cjs']="on('Fetch.requestPaused',()=>{const fixture=browserAPI(new URL(url));if(fixture)send('Fetch.fulfillRequest',{body:JSON.stringify(fixture)});else send('Fetch.continueRequest');});";
 assert.equal(analyze(fixture(files),entries).issues.length,0);
});
test("automatic selector identifiers keep CSS syntax and omit embedded HTML copy",()=>{
 const tokens=identifiersIn(`document.querySelector('.account-facts dd');document.getElementById('field');const html='<div>说明文字</div>';`);
 assert(tokens.includes('.account-facts dd'));assert(tokens.includes('#field'));assert(!tokens.includes('#.account-facts dd'));assert(!tokens.some(token=>token.includes('<div>')));
});
test("fixture alignment distinguishes partial-shell/proxy fixtures from full application servers",()=>{
 const files={'scripts/fragment-browser.cjs':"createServer(()=>readFileSync('app.css'));Page.navigate();",'scripts/full-browser.cjs':"createServer(()=>readFileSync('index.html'));Page.navigate();"};
 const result=analyze(fixture(files),[['fragment','scripts/fragment-browser.cjs'],['full','scripts/full-browser.cjs']]);assert.deepEqual(result.issues.map(row=>row.guard),['full']);
});
test("current browser guards align shared startup fixtures",()=>{assert.deepEqual(analyze(inventory(root)).issues,[]);});
test("preflight automatically analyzes changed and removed product identifiers, but ignores test edits",()=>{
 const {analyzeChanges}=require('./lib/behavior-checks.cjs');
 const source=fixture({'backend/web/index.html':'<script src="/app.js"></script>','backend/web/app.js':"api('/api/new-startup');",'scripts/r267-browser.cjs':"on('Fetch.requestPaused',()=>send('Fetch.continueRequest'));const p='/api/new-startup';assert(p);"});
 const result=analyzeChanges(source,[{file:'backend/web/app.js',delta:"- preference('search-server-id');\n+ api('/api/new-startup');"}]);
 assert(result.contractImpact.identifiers.includes('search-server-id'));assert(result.fixtureAlignment.issues.length);assert(result.contractImpact.files.some(row=>row.file==='scripts/r267-browser.cjs'));
 assert.equal(analyzeChanges(source,[{file:'scripts/b.test.cjs',delta:"api('/api/new-startup');"}]).fixtureAlignment,null);
});
