'use strict';
// Development checks only. Every source file is restored byte-for-byte in finally.
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2] || path.join(root,'output/r280-mutations'));fs.mkdirSync(out,{recursive:true});const results=[];
const tests=[
 ['no-fallback','backend/network_routes.go','res, err = directRoute.RoundTrip(retry)','return nil,err','TestR280AutomaticDeadProxyAndRecovery'],
 ['unbounded-fallback','backend/network_routes.go','res, err = directRoute.RoundTrip(retry)','res, err = directRoute.RoundTrip(retry);if err!=nil {return t.roundTripRoutes(retry,proxyRoute,directRoute)}','TestR280FallbackBoundedWhenBothFail'],
 ['manual-fallback','backend/network_routes.go','if t.settings.Mode == "manual" {\n\t\t\treturn nil, &proxyConnectionError','if false {\n\t\t\treturn nil, &proxyConnectionError','TestR280ManualNeverFallsBackOrProbes'],
 ['retain-backoff','backend/champion_network.go','riotRelays.nextTry = time.Time{}','// mutant retains nextTry','TestR280RecoveryClearsFailuresAndBroadcastsOnce'],
 ['pool-invalidation','backend/web/app.js','el.poolSkinGrid?.replaceChildren();state.poolCatalogSignature="";state.poolRenderPending=true;','el.poolSkinGrid?.replaceChildren();','R280 P4'],
];
for(const [id,file,from,to,pattern] of tests){const filename=path.join(root,file),original=fs.readFileSync(filename,'utf8');assert(original.includes(from),id+' anchor');try{fs.writeFileSync(filename,original.replace(from,to));const isGo=file.endsWith('.go'),r=spawnSync(isGo?'go':process.execPath,isGo?['test','./backend','-run','^'+pattern+'$','-count=1','-timeout=20s']:['--test','--test-name-pattern',pattern,'backend/web/r280.test.cjs'],{cwd:root,encoding:'utf8',timeout:60000});const log=(r.stdout || '')+(r.stderr || '');fs.writeFileSync(path.join(out,id+'.log'),log);assert(r.status!==0,id+' survived');assert(!/build failed|syntax error|SyntaxError/.test(log),id+' compile-only failure');results.push({id,killed:true,status:r.status});}finally{fs.writeFileSync(filename,original);}}
fs.writeFileSync(path.join(out,'mutations.json'),JSON.stringify(results,null,2));console.log(JSON.stringify(results));
