"""Run real one-at-a-time R75 source mutations; always restore exact bytes."""
import pathlib, subprocess, os, json
root=pathlib.Path(__file__).resolve().parents[1]
files=['pro_runes.go','pro_runes_recommendations.go']
original={f:(root/f).read_bytes() for f in files}
out=root/'testdata/r75/mutations';out.mkdir(parents=True,exist_ok=True)
mutations=[
('M1','pro_runes.go','var proRuneLeagues = map[string]string{','var proRuneLeagues = map[string]string{\n "98767991335774713": "LCK Challengers",','TestProRuneWhiteLists'),
('M2','pro_runes.go','if proRuneTeams[t.ID] != "" {','if t.Code == "DK" || t.Code == "GEN" || t.Code == "T1" {','TestProRuneWhiteLists'),
('M3','pro_runes.go','row = proRuneIndexGame{ID: g.ID,','for _, team := range g.Teams { if team.Side == "blue" { w.Metadata.Blue.TeamID = team.ID }; if team.Side == "red" { w.Metadata.Red.TeamID = team.ID } }; row = proRuneIndexGame{ID: g.ID,','TestProRuneCapturedLivestatsSides'),
('M4','pro_runes_recommendations.go','if f.Blue.Inhibitors >= 1 && f.Red.Inhibitors == 0 {','if f.Blue.TotalGold > f.Red.TotalGold {','TestProRuneCapturedAllSeries|TestProRuneWinnerConstraints'),
('M5','pro_runes.go','t.UTC().Truncate(10 * time.Second).Format(time.RFC3339)','t.UTC().Format(time.RFC3339Nano)','TestProRunePublicTransportAndAlignedSupplement'),
('M6','pro_runes_recommendations.go','if len(perks) != 9 {\n\t\treturn empty, false','if len(perks) != 9 {\n\t\treturn []int64{5008,5008,5011}, true','TestProRuneNewestAndShards'),
('M7','pro_runes_recommendations.go','result[i].Game.Start.After(result[j].Game.Start)','result[i].Game.Start.Before(result[j].Game.Start)','TestProRuneNewestAndShards'),
('M8','pro_runes.go','strings.EqualFold(name, player.Name)','strings.Contains(strings.ToLower(name), strings.ToLower(player.Name))','TestProRuneExactIdentityAndPersistentMapping')]
results=[]
try:
 for name,file,before,after,test in mutations:
  for f,b in original.items(): (root/f).write_bytes(b)
  text=original[file].decode();assert text.count(before)==1,(name,text.count(before))
  text=text.replace(before,after)
  if name=='M3':text=text.replace('type proEventGame struct {','type proEventGame struct {\n Teams []struct { ID string `json:"id"`; Side string `json:"side"` } `json:"teams"`')
  if name=='M4':text=text.replace('if f.Red.Inhibitors >= 1 && f.Blue.Inhibitors == 0 {','if f.Red.TotalGold > f.Blue.TotalGold {')
  (root/file).write_text(text)
  result=subprocess.run(['go','test','-count=1','-run','^('+test+')$','-v','.'],cwd=root,env={**os.environ,'GOCACHE':str(root/'.gocache')},capture_output=True,text=True)
  log=result.stdout+result.stderr;(out/(name+'.log')).write_text(log)
  killed=result.returncode!=0 and '--- FAIL:' in log and 'build failed' not in log
  results.append({'mutation':name,'killed':killed,'exit':result.returncode,'test':test})
  print(name,'KILLED' if killed else 'SURVIVED/INVALID',flush=True)
finally:
 for f,b in original.items(): (root/f).write_bytes(b)
 (out/'results.json').write_text(json.dumps(results,indent=2))
assert len(results)==8 and all(r['killed'] for r in results),results
print('All 8 killed; original source bytes restored.')
