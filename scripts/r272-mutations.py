"""R272 real behavior regressions, using disposable Go overlays and JS sources."""
import argparse,json,os,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
import runpy
_proof=runpy.run_path(str(ROOT/'scripts/mutation-source-proof.py'));_snapshot=_proof['snapshot'](ROOT)
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);args=p.parse_args();args.out=args.out.resolve();args.out.mkdir(parents=True,exist_ok=True);rows=[]
def run(name,command,env=None,negative=False):
 started=time.monotonic();result=subprocess.run(command,cwd=ROOT,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120)
 log=result.stdout+result.stderr;(args.out/(name+'.log')).write_text(log)
 ok=result.returncode!=0 and any(s in log for s in ['--- FAIL:','AssertionError']) and not any(s in log for s in ['build failed','SyntaxError','ReferenceError','TypeError','ENOENT']) if negative else result.returncode==0
 rows.append(dict(name=name,negative=negative,exitCode=result.returncode,assertionVerified=ok,seconds=round(time.monotonic()-started,3)))
 (args.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True)
 assert ok,name
with tempfile.TemporaryDirectory(prefix='r272-mutants-') as temp:
 temp=Path(temp)
 go=['go','test','./backend','-run','^TestR272','-count=1'];js=['node','--test','backend/web/r272-lp.test.cjs','backend/web/r272-history.test.cjs','backend/web/r272-career.test.cjs','backend/web/r272-collection.test.cjs','backend/web/r272-score-tags.test.cjs']
 run('baseline-go',go);run('baseline-js',js)
 def overlay(name,changes,pattern="^TestR272(LP|Synthetic|Shape)"):
  mapping={}
  for index,(relative,before,after) in enumerate(changes):
   source=(ROOT/relative).read_text();assert before in source,(name,relative)
   target=temp/(name+str(index)+'.go');target.write_text(source.replace(before,after,1));mapping[str(ROOT/relative)]=str(target)
  target=temp/(name+'.json');target.write_text(json.dumps({'Replace':mapping}));run(name,['go','test','-overlay='+str(target),'./backend','-run',pattern,'-count=1'],negative=True)
 overlay('lp-revision',[('backend/lp_tracker.go','\n\t\tupdateMatchRevision(&matches[index])','')])
 overlay('lp-recorded-invalidation',[('backend/lp_tracker.go','if t.onRecorded != nil {','if false && t.onRecorded != nil {')])
 overlay('lp-riot-self',[('backend/overview_lp.go','response.Player.IsCurrent = a.riotOverviewIsCurrent(response.Player)','response.Player.IsCurrent = false')])
 overlay('full-cumulative-stream',[('backend/overview_response_shape.go','partial.Matches = delta','_ = delta')])
 overlay('raw-riot-response',[('backend/gameplay.go','type gameplayMatch struct {','type gameplayMatch struct {\n RawResponse *riotMatch `json:"rawResponse,omitempty"`'),('backend/riot_api.go','return convertRiotMatchInfo(&match.Info, subjectPUUID, names, labels, region, "")','result := convertRiotMatchInfo(&match.Info, subjectPUUID, names, labels, region, "");result.RawResponse=match;return result')])
 source=(ROOT/'backend/web/gameplay.js').read_text();before=' || old.lpDelta!==match.lpDelta';assert before in source
 target=temp/'lp-frontend.js';target.write_text(source.replace(before,''));run('lp-frontend',js,{'R272_GAMEPLAY_SOURCE':str(target)},True)

 overlay('credential-scoped-details',[('backend/riot_api.go','key := "riot-match-v4|" + matchID','key := "riot-match-v4|" + riotCredentialScope(ctx) + "|" + matchID')],'^TestR272DetailDiskCrossCredential$')
 overlay('lost-embedded-80-guard',[('backend/riot_routing.go','nearQuota := count*5 >= limit*4','nearQuota := count >= limit')],'^TestR272Embedded80GuardAndUserIsolation$')
 overlay('remakes-counted-as-missing',[('backend/match_score.go','if m.Result == "remake" {','if false && m.Result == "remake" {')],'^TestR272ScoreBatchRemakesDistinctFromMissing$')
 overlay('old-15s-total',[('backend/aramkit_rating.go','aramkitRatingTotalTimeout  = 45 * time.Second','aramkitRatingTotalTimeout  = 15 * time.Second')],'^TestR272MayhemContinuousAndIdleBody$/continuous_beyond_old15s')
 overlay('lost-postgame-refresh',[('backend/gameplay_refresh.go','a.scheduleCollectionEventRefresh(0)','// discarded postgame inventory')],'^TestR272CollectionEventAndPostgame$/InProgress')
 overlay('lost-known-arena-squad',[('backend/arena_live_grouping.go','for _, ally := range remembered {\n\t\tmatchIndex := -1','for _, ally := range remembered[:0] {\n\t\tmatchIndex := -1')],'^TestR272ArenaKnownTrioSurvivesMissingMember$')
 def javascript(name,relative,before,after,envkey):
  original=(ROOT/relative).read_text();assert before in original,(name,relative)
  target=temp/(name+'.js');target.write_text(original.replace(before,after,1));run(name,js,{envkey:str(target)},True)
 javascript('badge-only-no-placement','backend/web/gameplay.js','const evaluation=scorePlacementChip(record,true);','const evaluation=scoreBadgeChip(record);','R272_GAMEPLAY_SOURCE')
 javascript('score-placement-not-pinned','backend/web/gameplay.js','tag.priority===60000','tag.priority===-1','R272_GAMEPLAY_SOURCE')
 javascript('old-empty-ready','backend/web/gameplay.js',"return Boolean(tab?.loading && (!tab.matchesReceived || tab.data?.pagination?.partial)","return Boolean(tab?.loading && tab.data?.pagination?.partial",'R272_GAMEPLAY_SOURCE')
 javascript('old-reference-expired-error','backend/web/gameplay.js','if(error.status!==404 || !/引用|reference/i.test(error.message)','if(true || error.status!==404 || !/引用|reference/i.test(error.message)','R272_GAMEPLAY_SOURCE')
 javascript('old-time-cursor','backend/web/gameplay.js','cursor:()=>tab.advancedHistory?.active?tab.advancedHistory.next:Number(tab.nextBegIndex || Number(tab.data?.pagination?.begIndex || 0)+Number(tab.data?.pagination?.count || 0))','cursor:()=>Number(tab.historyTimeCursor || 0)','R272_GAMEPLAY_SOURCE')
 javascript('full-career-render','backend/web/gameplay.js',"const container=overviewContainer(tab),root=(container?._overviewSubpageStash || container)?.querySelector('.career-column');if(!root || !tab.data)return;","rerenderTab(tab,reason);return;const container=overviewContainer(tab),root=(container?._overviewSubpageStash || container)?.querySelector('.career-column');if(!root || !tab.data)return;",'R272_GAMEPLAY_SOURCE')
 javascript('index-career-patch','backend/web/history-recovery.js',"const old=[...column.children].find(node=>node.dataset.careerKey===key);","const old=[...column.children][entries.findIndex(entry=>entry[0]===key)];",'R272_RECOVERY_SOURCE')
 javascript('lost-server-invalidation','backend/web/app.js',' || state.collectionScope!==scope','', 'R272_APP_SOURCE')
 javascript('old-server-response-accepted','backend/web/app.js','collectionAccount!==[state.status?.clientRegion,state.status?.serverId,state.status?.collectionAccount].join("|")','false', 'R272_APP_SOURCE')
 javascript('matches-prematurely-ready-aggregates','backend/web/gameplay.js',"(pendingMatches || pendingAggregates || pendingStats || key==='champions' && championStatsPending(championRows,championOverall,championProgress,tab)) && titles[key]",'pendingMatches && titles[key]', 'R272_GAMEPLAY_SOURCE')
 run('restored-go-target',['go','test','./backend','-run','^TestR272LPRevisionAndDeltaFrames$','-count=1','-v']);run('restored-js',js)
 assert '--- PASS: TestR272LPRevisionAndDeltaFrames' in (args.out/'restored-go-target.log').read_text()

_proof['unchanged'](ROOT,_snapshot)
