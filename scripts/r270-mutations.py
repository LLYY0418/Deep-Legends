import argparse,pathlib,tempfile,subprocess,os,json,time
p=argparse.ArgumentParser();p.add_argument('--out',required=True);args=p.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=pathlib.Path(args.out).resolve();out.mkdir(parents=True,exist_ok=True);rows=[]
def run(name,command,extra,negative,signature=None):
    env=dict(os.environ,RIOT_API_KEY='RGAPI-00000000-0000-0000-0000-000000000000',**extra);start=time.monotonic()
    result=subprocess.run(command,cwd=root,env=env,capture_output=True,text=True,timeout=180);log=result.stdout+result.stderr;(out/(name+'.log')).write_text(log)
    verified=result.returncode==0 if not negative else result.returncode==1 and signature in log and 'SyntaxError' not in log
    row=dict(name=name,negative=negative,exitCode=result.returncode,assertionVerified=verified,seconds=round(time.monotonic()-start,3));rows.append(row);(out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');assert verified,row
    print(json.dumps(row),flush=True)
test=['node','--test','backend/web/r270.test.cjs']
run('baseline',test,{},False)
with tempfile.TemporaryDirectory(prefix='r270-mutants-') as temporary:
    folder=pathlib.Path(temporary);game=(root/'backend/web/gameplay.js').read_text();guard='if(tab.advancedMenu?.open && container._afBoundTab===tab)';start=game.index('  function renderFilteredMatchView(');assert game[start:].count(guard)==1
    mutant=folder/'gameplay.js';mutant.write_text(game[:start]+game[start:].replace(guard,'if(tab.advancedMenu?.open)',1))
    run('unbound-open-deferred',test,{'R270_GAMEPLAY_SOURCE':str(mutant)},True,'the real filter trigger must replace the cold fallback')
    probes=folder/'probes';probes.mkdir()
    for n in [265,266,269]:
        source=(root/f'scripts/r{n}-persisted-ui.cjs').read_text();selector='dialog[data-af-menu]:modal .af-saved .af-empty';assert source.count(selector)==1;(probes/f'r{n}-persisted-ui.cjs').write_text(source.replace(selector,'.af-empty'))
    run('unscoped-saved-empty',test,{'R270_PERSISTED_DIR':str(probes)},True,'must see the current modal')
    lifecycle=folder/'geometry.cjs';source=(root/'scripts/r269-geometry.cjs').read_text();assert source.count('await Promise.race')==1;lifecycle.write_text(source.replace('await Promise.race','void Promise.race'))
    run('delete-before-process-exit',test,{'R270_GEOMETRY_SOURCE':str(lifecycle)},True,'a late writer must not recreate the deleted profile')
    run('native-unbound-open-deferred',['node','scripts/r269-browser.cjs'],{'R266_WEB_OVERRIDE':str(folder),'R269_BROWSER_OUT':str(out/'native-unbound-open-deferred'),'R269_BACKEND':os.environ.get('R269_BACKEND') or os.environ.get('R266_BACKEND') or '/tmp/r270-public'},True,"timeout document.querySelector('dialog[data-af-menu]:modal')")
run('restored',test,{},False)
