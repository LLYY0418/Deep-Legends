"""R258 P7 assertion mutations, without changing production files."""
import argparse, pathlib, tempfile, subprocess, os, json, hashlib
parser=argparse.ArgumentParser();parser.add_argument('--output',required=True);args=parser.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=pathlib.Path(args.output);out.mkdir(parents=True,exist_ok=False)
mutations=[
 ('same-class-and','backend/web/history-filters.js',[('c.values.some(value=>matchesValue','c.values.every(value=>matchesValue')],'R258_FILTER_SOURCE'),
 ('without-atleast','backend/web/history-filters.js',[('function multikill(subject, level, atLeast = false) {','function multikill(subject, level, atLeast = false) { atLeast = false;')],'R258_FILTER_SOURCE'),
 ('without-300-limit','backend/web/history-filters.js',[(' || search.scanned>=300',''),('Math.min(20,300-search.scanned)','20')],'R258_FILTER_SOURCE'),
 ('without-player-switch-cancel','backend/web/gameplay.js',[('if(globalThis.deepLegendsHistoryFilters && activeTab() !== tab) cancelAdvancedMatchSearch(activeTab());','')],'R258_FILTER_GAMEPLAY_SOURCE')]
before={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for _,p,*_ in mutations};rows=[]
for name,target,edits,envkey in mutations:
 source=(root/target).read_text()
 for old,new in edits:
  assert old in source,name
  source=source.replace(old,new,1)
 with tempfile.TemporaryDirectory(prefix='r258-filter-mutant-') as directory:
  replacement=pathlib.Path(directory)/pathlib.Path(target).name;replacement.write_text(source)
  result=subprocess.run(['node','--test','backend/web/r258-filters.test.cjs'],cwd=root,env={**os.environ,envkey:str(replacement)},stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (out/(name+'.log')).write_text(result.stdout)
  killed=result.returncode!=0 and ('AssertionError' in result.stdout or 'ERR_ASSERTION' in result.stdout) and 'SyntaxError:' not in result.stdout and 'ReferenceError:' not in result.stdout
  row={'name':name,'assertion_killed':killed,'exit_code':result.returncode};rows.append(row);print(json.dumps(row),flush=True)
assert before=={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in before}
(out/'results.json').write_text(json.dumps({'unchanged_source':True,'all_assertion_killed':all(r['assertion_killed'] for r in rows),'rows':rows},indent=2)+'\n')
raise SystemExit(0 if all(r['assertion_killed'] for r in rows) else 1)
