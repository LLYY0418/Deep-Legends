from pathlib import Path
import subprocess,json,os
root=Path.cwd();out=Path('/private/tmp/r192-mutants');out.mkdir(exist_ok=True)
source=(root/'backend/web/gameplay.js').read_text();results=[]
for name,old,new,tests in [
 ('hydrate-without-full','view.render(id, { full: true });','view.render(id);',['R192 1 ', 'R192 5 ']),
 ('retry-unbound','button.addEventListener("click", () => view.toggleMatch(button.dataset.retryMatchDetail));','void button;',['R192 2 '])
]:
 assert old in source,name
 file=out/(name+'.js');file.write_text(source.replace(old,new));env={**os.environ,'R192_SOURCE_FILE':str(file)}
 for test in tests:
  result=subprocess.run(['node','--test','--test-name-pattern='+test,'backend/web/r192.test.cjs'],env=env,capture_output=True,text=True)
  log=result.stdout+result.stderr;(out/(name+'-'+test.strip().replace(' ','-')+'.log')).write_text(log)
  assert result.returncode!=0 and 'ERR_ASSERTION' in log and 'ReferenceError' not in log and 'SyntaxError' not in log,name+' not killed by assertion: '+log[-2000:]
  results.append({'name':name,'test':test.strip(),'result':'assertion FAIL (mutant killed)'})
report=root/'docs/history/reports/r192';report.mkdir(parents=True,exist_ok=True);(report/'mutations.json').write_text(json.dumps(results,ensure_ascii=False,indent=2));print(json.dumps(results,ensure_ascii=False))
