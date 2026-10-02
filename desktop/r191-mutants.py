from pathlib import Path
import subprocess,json,os
root=Path.cwd();out=Path('/private/tmp/r191-mutants');out.mkdir(exist_ok=True)
results=[]
def check(name,relative,old,new,test,javascript=False):
 source=(root/relative).read_text();assert old in source,name
 target=out/(name+('.js' if javascript else '.go'));target.write_text(source.replace(old,new,1))
 env=dict(os.environ)
 if javascript:
  env['R191_SOURCE_FILE']=str(target)
  cmd=['node','--test','--test-name-pattern='+test,'backend/web/r191.test.cjs']
 else:
  overlay=out/(name+'-overlay.json');overlay.write_text(json.dumps({'Replace':{str(root/relative):str(target)}}))
  cmd=['go','test','-overlay='+str(overlay),'./backend','-run','^'+test+'$','-count=1']
 run=subprocess.run(cmd,env=env,capture_output=True,text=True);log=run.stdout+run.stderr;(out/(name+'.log')).write_text(log)
 assert run.returncode!=0 and ('ERR_ASSERTION' in log if javascript else '--- FAIL: '+test in log) and 'build failed' not in log and 'ReferenceError' not in log,name+' not killed by assertion: '+log[-1500:]
 results.append({'name':name,'test':test,'result':'assertion FAIL (mutant killed)'})
check('missing-as-zero','backend/gameplay_build.go','if entry.Vars[0] == nil && entry.Vars[1] == nil && entry.Vars[2] == nil {','if false {','TestR191_01RiotSGPPresence')
check('global-rerender','backend/web/gameplay.js','rerenderAugmentDescriptionViews(missing);','rerenderCatalogViews();','R191 09',True)
check('directory-direct-error','backend/hexdata.go','p.reportHexdataFallback("augment-directory", err)','return championAugmentDetailResponse{}, err','TestR191_11DirectoryFallbackHTTP')
check('sample-unbounded','backend/gameplay_build.go','if a.perkDiagnosticCounts[key] >= 3 {','if false {','TestR191_07SampleCap')
report=root/'docs/history/reports/r191';report.mkdir(parents=True,exist_ok=True);(report/'mutations.json').write_text(json.dumps(results,ensure_ascii=False,indent=2));print(json.dumps(results,ensure_ascii=False))
