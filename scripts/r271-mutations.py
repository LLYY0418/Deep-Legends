"""R271 regressions and negative assertions run against disposable source copies."""
import argparse,json,os,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);p.add_argument('--browser',action='store_true');args=p.parse_args();args.out.mkdir(parents=True,exist_ok=True);rows=[]
def run(name,env=None,negative=False,cmd=None):
 t=time.monotonic();r=subprocess.run(cmd or ['node','--test','backend/web/r271.test.cjs'],cwd=ROOT,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=180);log=r.stdout+r.stderr;(args.out/(name+'.log')).write_text(log)
 ok=(r.returncode!=0 and ('AssertionError' in log or 'ASSERTION FAILED:' in log) and not any(s in log for s in ['ParserError','SyntaxError','Cannot find module','ENOENT','ReferenceError','TypeError'])) if negative else r.returncode==0
 rows.append(dict(name=name,negative=negative,exitCode=r.returncode,assertionVerified=ok,seconds=round(time.monotonic()-t,3)));(args.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert ok,name
with tempfile.TemporaryDirectory(prefix='r271-mutants-') as tmp:
 tmp=Path(tmp);run('baseline')
 probes=tmp/'probes';probes.mkdir()
 for n in [265,266,269]:
  s=(ROOT/f'scripts/r{n}-persisted-ui.cjs').read_text();old='if(!await evaluate(visible))await click(\'[data-history-filter-load],[data-af-open]\');';assert old in s;s=s.replace(old,old+'\n  if(!await evaluate(visible))await click(\'[data-af-open]\');');(probes/f'r{n}-persisted-ui.cjs').write_text(s)
 run('double-queued-open',{'R271_PERSISTED_DIR':str(probes)},True)
 source=(ROOT/'backend/web/gameplay.js').read_text();old='if(!deferred && isActive() && container?._overviewViewTab===tab && container._afBoundTab===tab && !tab.overviewSubpage)renderFilteredMatchView(tab);';assert old in source;mutant=source.replace(old,"if(false)renderFilteredMatchView(tab);");file=tmp/'full-render.js';file.write_text(mutant);run('full-overview-filter-commit',{'R271_GAMEPLAY_SOURCE':str(file)},True)
 runsource=(ROOT/'scripts/r206-real-upgrade-windows.ps1').read_text();helper=(ROOT/'scripts/r271-installer-diagnostics.ps1').read_text()
 cases=[('lost-stderr','run',"-RedirectStandardError ($prefix + '-stderr.log')",''),('lost-startup-tail','helper',"Show-R271LogTail 'installer startup log' $startupCopy",''),('unrelated-events','helper','if ($related) {','if ($true) {'),('unredacted-key','helper',"-replace 'RGAPI-[A-Za-z0-9-]+', '[riot-key]'",''),('unflushed-output','run','$process.WaitForExit() # Flush the redirected readers after the original bounded wait.','')]
 for name,kind,a,b in cases:
  original=runsource if kind=='run' else helper;assert a in original;f=tmp/(name+'.ps1');f.write_text(original.replace(a,b));run(name,{'R271_RUN_SOURCE' if kind=='run' else 'R271_HELPER_SOURCE':str(f)},True)
 if args.browser:
  web=tmp/'web';web.mkdir();(web/'gameplay.js').write_text(mutant)
  run('native-full-overview-filter-commit',{'R266_WEB_OVERRIDE':str(web),'R269_BROWSER_OUT':str(args.out/'native-mutant')},True,['node','scripts/r269-browser.cjs'])
 run('restored')
