from pathlib import Path
import subprocess,json,os
root=Path.cwd();out=Path('/private/tmp/r190-mutants');out.mkdir(exist_ok=True)
source=(root/'backend/web/gameplay.js').read_text()
mutations={
'roster-all':('isCurrentMatchParticipant(item, match) ? " is-current-player" : ""','true ? " is-current-player" : ""','R190 1'),
'summary-sum':('Math.max(0, ...row.lines.filter(line => line.kind === kind && typeof line.value === "number").map(line => line.value))','row.lines.filter(line => line.kind === kind && typeof line.value === "number").reduce((sum, line) => sum + line.value, 0)','R190 10'),
'no-overrides':('Object.freeze({ 8008: [], 8304: [] })','Object.freeze({})','R190 5'),
'board-structure':('<div class="unified-rune-board">${renderStyle','<div class="unified-rune-board" data-wrong-structure="true">${renderStyle','R190 12'),
'no-id-cache':('.filter(id => id > 0 && !state.augmentDescriptions.has(id))','.filter(id => id > 0)','R190 22'),
}
results=[]
for name,(old,new,test) in mutations.items():
 assert old in source,name
 file=out/(name+'.js');file.write_text(source.replace(old,new,1))
 env={**os.environ,'R190_SOURCE_FILE':str(file)}
 result=subprocess.run(['node','--test','--test-name-pattern='+test,'backend/web/r190.test.cjs'],env=env,capture_output=True,text=True)
 log=result.stdout+result.stderr;(out/(name+'.log')).write_text(log)
 assert result.returncode!=0 and 'ERR_ASSERTION' in log and 'ReferenceError' not in log,name+' not killed by assertion'
 results.append({'name':name,'status':'assertion FAIL','test':test})
for name,relative,old,new,test in [
 ('riot-no-vars','backend/riot_api.go','Vars: [3]int64{selection.Var1, selection.Var2, selection.Var3}','Vars: [3]int64{}','TestR190LCUAndRiotPerkStats'),
 ('seo-description','backend/hexdata.go','Description: hexdataAugmentGuideDescription(document)','Description: func() string { for _, node := range descendantElements(document, "meta") { if attribute(node, "name") == "description" { return attribute(node, "content") } }; return "" }()','TestR190AugmentDescriptionSources')]:
 text=(root/relative).read_text();assert old in text,name
 file=out/(name+'.go');file.write_text(text.replace(old,new,1))
 overlay=out/(name+'-overlay.json');overlay.write_text(json.dumps({'Replace':{str(root/relative):str(file)}}))
 result=subprocess.run(['go','test','-overlay='+str(overlay),'./backend','-run','^'+test+'$','-count=1'],capture_output=True,text=True)
 log=result.stdout+result.stderr;(out/(name+'.log')).write_text(log)
 assert result.returncode!=0 and '--- FAIL: '+test in log and 'build failed' not in log,name+' not killed by assertion: '+log[-1200:]
 results.append({'name':name,'status':'assertion FAIL','test':test})
(out/'results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2));print(json.dumps(results,ensure_ascii=False))
