#!/usr/bin/env python3
"""Run R288 behavior mutants serially, restoring source bytes after each run."""
from pathlib import Path
import subprocess, os, json
root=Path(__file__).resolve().parents[1]
out=Path(os.environ.get('R288_MUTATION_OUT',str(root/'output/evidence/r288/mutations')))
out.mkdir(parents=True,exist_ok=True)
env={**os.environ,'GOCACHE':'/private/tmp/r288-go-cache'}
results=[]

def run(label,command):
 p=subprocess.run(command,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (out/(label+'.log')).write_text(p.stdout)
 return p.returncode

def mutate(label,file,before,after,command):
 p=root/file;original=p.read_bytes();source=original.decode()
 if before not in source:raise RuntimeError('missing mutation anchor: '+label)
 try:
  p.write_text(source.replace(before,after,1))
  status=run(label,command)
  results.append({'mutation':label,'exitCode':status,'killed':status!=0})
 finally:p.write_bytes(original)
 if status==0:raise RuntimeError('survived: '+label)

go=lambda pattern:['go','test','./backend','-count=1','-run',pattern]
node=lambda file:['node','--test',file]
basego=go('TestR288(Announcement|Autostart|WeGame|ProFreshness|ProFirstScreen|ProRiotCache|Backfill|Score|Automatic|Release)')
basenode=['node','--test','backend/web/r288-announcements.test.cjs','backend/web/r288-privacy.test.cjs','backend/web/r288-pro.test.cjs','backend/web/r288-settings.test.cjs']
for label,command in [('baseline-go',basego),('baseline-node',basenode)]:
 if run(label,command):raise RuntimeError(label+' failed')
mutate('p1-wait-mayhem','backend/season_stats.go','a.publishSeasonSnapshot(reference, playerRef, scan.cache)\n\t\t\t\ta.recordRankWindow','/* delayed until the round ends */\n\t\t\t\ta.recordRankWindow',go('^TestR288Backfill'))
mutate('p2-no-bold','backend/web/announcements.js','<strong class="announcement-em">$1</strong>','$1',node('backend/web/r288-announcements.test.cjs'))
mutate('p3-older-wegame','backend/client_candidate_versions.go','a.Parts[i] > b.Parts[i]','a.Parts[i] < b.Parts[i]',go('^TestR288WeGame'))
mutate('p4-thirty-second-failure','backend/pro_scoped_refresh.go','return 5 * time.Minute','return 30 * time.Second',go('^TestR288ProFreshness'))
mutate('p4-whole-table','backend/pro_scoped_refresh.go','if body.Team == "all" {','if false {',go('^TestR288ProFirstScreen'))
mutate('p4-failure-erases-success','backend/pro_scoped_profile.go','if failed {\n\t\tnext.Failures','if failed {\n        next.LastMatchAt, next.LastMatchAtKnown = "", false\n\t\tnext.Failures',go('^TestR288ProRiotCache'))
mutate('p6-history-name-leak','backend/web/gameplay.js','function playerParticipantName(player, index) { return state.settings.maskNames ? privacyName(player,{championName:player.championName,index,context:"match"}) : playerLabel(player); }','function playerParticipantName(player, index) { return playerLabel(player); }',node('backend/web/r288-privacy.test.cjs'))
mutate('p5-no-window-protection','desktop/main.cjs','mainWindow?.setContentProtection(next.contentProtection);','/* content protection omitted */',node('backend/web/r288-settings.test.cjs'))
for label,command in [('restored-go',basego),('restored-node',basenode)]:
 if run(label,command):raise RuntimeError(label+' failed')
(out/'results.json').write_text(json.dumps({'results':results,'restored':True},indent=2))
print(json.dumps(results,indent=2))
