"""R100 executable mutations. Temporary Go overlays / JS overrides, never edit production."""
import json, os, subprocess, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
OUT=ROOT/'docs/r100-validation/mutations';OUT.mkdir(parents=True,exist_ok=True)
ENV=dict(os.environ,GOCACHE='/tmp/deep-legends-go-cache',GOTMPDIR='/tmp/deep-legends-go-tmp')
results=[]
def probe(name,file,old,new,test='',kind='go',envkey=''):
 source=(ROOT/file).read_text();assert source.count(old)==1,(name,source.count(old))
 cmd=['go','test','-count=1','-v','-timeout=60s','-run',test,'.'] if kind=='go' else (['node','desktop/r100-browser.cjs'] if kind=='browser' else ['node','--test',test])
 env=ENV.copy();env['R100_BROWSER_OUTPUT']=str(OUT/(name+'-baseline-browser'))
 clean=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=120)
 (OUT/(name+'-baseline.txt')).write_text(clean.stdout+clean.stderr)
 assert clean.returncode==0,(name,'baseline',clean.stdout[-1500:],clean.stderr[-1500:])
 with tempfile.TemporaryDirectory(prefix='r100-mut-') as tmp:
  changed=Path(tmp)/Path(file).name;changed.write_text(source.replace(old,new))
  if kind=='go':
   overlay=Path(tmp)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(ROOT/file):str(changed)}}));cmd=cmd[:2]+['-overlay',str(overlay)]+cmd[2:]
  else:env[envkey]=str(changed)
  env['R100_BROWSER_OUTPUT']=str(OUT/(name+'-browser'))
  result=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=120)
 log=result.stdout+result.stderr;(OUT/(name+'.txt')).write_text(log)
 killed=result.returncode!=0 and ('AssertionError' in log or 'FAIL: TestR100' in log) and not any(x in log for x in ['build failed','test timed out','SyntaxError'])
 results.append(dict(id=name,file=file,killed=killed,baseline_exit=clean.returncode,exit=result.returncode));(OUT/'matrix.json').write_text(json.dumps(results,indent=2)+'\n')
 print(name,'KILLED' if killed else 'INVALID/SURVIVED',flush=True);assert killed,log[-2000:]
probe('P0-absolute-round-deadline','riot_api.go','ctx = withRiotSingleWaitLimit(ctx, 5*time.Second)','ctx = withRiotQueueLimit(ctx, 5*time.Second)','^TestR100OverviewLongRoundUsesFreshSingleWaitBudget$')
probe('P1-six-image-sockets','web/image-queue.js','const limit = 2,','const limit = 6,',kind='browser',envkey='R100_QUEUE_SOURCE')
probe('P1-status-fatal','web/app.js','window.deepLegendsStatusRecovery?.(true);','showFatal(error.message);',kind='browser',envkey='R100_APP_SOURCE')
probe('P2-wait-for-optional-augments','gameplay.go','payload = a.enrichPerkAugments(cacheKey, client, payload)','''payload = a.enrichPerkAugments(cacheKey, client, payload)
        a.perkCatalogMu.Lock(); pending := a.perkAugmentJobs[cacheKey]; a.perkCatalogMu.Unlock()
        if pending != nil { <-pending }''','^TestR100PerksNeverWaitForOptionalAugmentsAndPersist$')
probe('P4-null-overwrites-profile','web/gameplay.js','value != null && !(partial.profilePending','!(partial.profilePending',test='web/r100.test.cjs',kind='node',envkey='R100_GAMEPLAY_SOURCE')
probe('P4-background-self-abort','web/gameplay.js','if (tab.loading || tab.loadingMore) { tab.pendingHistoricalRanks = detail; return false; }','/* mutation: background reload may abort foreground */',test='web/r100.test.cjs',kind='node',envkey='R100_GAMEPLAY_SOURCE')
probe('P5-raw-mode-fallback','gameplay.go','return "其他模式"\n}\n\nfunc containsChineseQueueLabel','return mode\n}\n\nfunc containsChineseQueueLabel','^TestR100QueueLabelsAndArenaRegistry$')
probe('P7-no-persistence','desktop/diagnostics-directory.cjs','fs.writeFileSync(temp,JSON.stringify({diagnosticsSaveDirectory:saved}),{mode:0o600});fs.renameSync(temp,settings);','/* mutation: memory only */',test='desktop/r100.test.cjs',kind='node',envkey='R100_DIRECTORY_SOURCE')
probe('P7-trust-bypass','desktop/diagnostics-directory.cjs','return event?.sender===getMainWindow()?.webContents&&isTrustedRenderer(event.sender);','return true;',test='desktop/r100.test.cjs',kind='node',envkey='R100_DIRECTORY_SOURCE')
probe('P7-inode-revalidation-removed','desktop/diagnostics-directory.cjs','return now.dev===record.dev&&now.ino===record.ino;','return true;',test='desktop/r100.test.cjs',kind='node',envkey='R100_DIRECTORY_SOURCE')
