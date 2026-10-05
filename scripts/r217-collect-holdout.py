#!/usr/bin/env python3
"""Collect only; never imports/scans scores or evaluates P4. Resume frozen P3.
Round-robin source quotas, unique games, maximum15 including actual selected
participants. Serial Riot personal-key reads share the R208 pacing/cooldown.
"""
import argparse,collections,datetime,hashlib,json,pathlib,urllib.parse
from r217_data import *
OUT=REPORT/'opgg-holdout'
def main():
 parser=argparse.ArgumentParser();parser.add_argument('--target',type=int,default=200);opts=parser.parse_args();assert opts.target==200,'R217 target is frozen at 200'
 if (OUT/'manifest-sha256.json').exists():raise SystemExit('Holdout frozen; no more collection writes')
 candidate=REPORT/'keyword-candidate.json';accounts_file=REPORT/'holdout-accounts.json'
 if not candidate.exists()or not accounts_file.exists():raise SystemExit('Freeze candidate and accounts before P3')
 candidate_hash=hashlib.sha256(candidate.read_bytes()).hexdigest();assert candidate_hash=='50bbd1866cee896945b4d57810dfc65b3c7300077c8e11980ff93ab98b90282a';assert hashlib.sha256((ROOT/'backend/testdata/r217/keyword-candidate.json').read_bytes()).hexdigest()==candidate_hash;accounts_hash=hashlib.sha256(accounts_file.read_bytes()).hexdigest();accounts=json.loads(accounts_file.read_text());assert len(accounts)==16
 old_ids,old_puuids=old_exclusions();puuids={a['puuid']for a in accounts};assert len(puuids)==len(accounts)and not puuids&old_puuids
 client=Client(OUT);client.log(event='run_start',key_mode='personal',target=opts.target,candidate_sha256=candidate_hash,accounts_sha256=accounts_hash)
 paired={p['match_id']:p for p in json.loads((OUT/'paired.json').read_text())}if (OUT/'paired.json').exists()else {};actual=collections.Counter();source=collections.Counter();groups=collections.Counter();exclusions=collections.defaultdict(collections.Counter);keywords={};pools=[]
 for p in paired.values():
  owner=p['account_index'];source[owner]+=1;groups[accounts[owner]['group']]+=1
  raw=json.loads((OUT/(p['match_id']+'-match.json')).read_text());actual.update(x['puuid']for x in raw['info']['participants']if x['puuid']in puuids)
 def save(complete=False):
  write_json(OUT/'paired.json',sorted(paired.values(),key=lambda p:p['match_id']));write_json(OUT/'keyword-labels.json',keywords)
  tier_dist=collections.Counter();op_tier_dist=collections.Counter()
  for p in paired.values():
   owner=p['account_index'];tier_dist[accounts[owner]['tier']]+=1;own=next((q for q in op_people(p['opgg'])if q['participant_id']==p['opgg'].get('participant_id')),None)
   tier_info=own.get('tier_info')if own else None
   if tier_info:op_tier_dist[tier_info.get('tier','UNKNOWN')]+=1
   else:op_tier_dist['MISSING']+=1
  summary={'updatedAt':now(),'matches':len(paired),'target':opts.target,'complete':complete,'accounts':len(accounts),'source_accounts_with_matches':sum(source[i]>0 for i in range(len(accounts))),'key_mode':'personal','candidate_sha256':candidate_hash,'accounts_sha256':accounts_hash,'groups':dict(groups),'tiers':dict(tier_dist),'opgg_tier_info_distribution':dict(op_tier_dist),'old_match_exclusions':len(old_ids),'old_source_puuids':len(old_puuids),'per_account':[{'index':i,'gameName':a['gameName'],'tagLine':a['tagLine'],'tier':a['tier'],'group':a['group'],'puuid':a['puuid'],'source_games':source[i],'actual_participating_games':actual[a['puuid']],'opgg_tier_info':a.get('opggTierInfo'),'exclusions':dict(exclusions[i])}for i,a in enumerate(accounts)],'no_evaluation_run':True}
  write_json(OUT/'collection-summary.json',summary);return summary
 # Acquire OP history and match IDs, independent of outcomes. Cached inputs resume.
 for i,a in enumerate(accounts):
  url,op_puuid=client.page(a['gameName'],a['tagLine'],OUT/f'account-{i}-page.html',i);account=client.riot('asia','/riot/account/v1/accounts/by-riot-id/'+urllib.parse.quote(a['gameName'])+'/'+urllib.parse.quote(a['tagLine']),OUT/f'account-{i}-riot.json','Riot account',account_index=i)
  if account['puuid']!=a['puuid']or account['puuid']in old_puuids:raise SystemExit('Frozen account changed or overlaps old PUUID')
  client.log(event='account_resolution',account_index=i,puuid=account['puuid'],opgg_found=True,opgg_puuid=op_puuid)
  games=[];ended=''
  for page in range(4):
   rows=client.games(url,op_puuid,OUT/f'account-{i}-games-{page}.flight',ended,i)
   if not rows:break
   games.extend(rows);ended=rows[-1]['created_at']
  valid=[];fp={}
  for g in games:
   if (g.get('game_type',{}).get('game_type') if isinstance(g.get('game_type'),dict) else g.get('game_type'))!='SOLORANKED':exclusions[i]['non420']+=1;continue
   ps=op_people(g)
   if len(ps)!=10:exclusions[i]['not10']+=1;continue
   if any(not p.get('stats',{}).get('op_score_timeline_analysis') or len(p.get('stats',{}).get('op_score_timeline',[]))<2 for p in ps):exclusions[i]['opgg_missing_curve_analysis']+=1;continue
   key=tuple(fingerprint_op(g))
   if key in fp and fp[key].get('id')!=g.get('id'):raise SystemExit('Ambiguous game fingerprint')
   fp[key]=g;valid.append(g)
  ids=[]
  if valid:
   ts=[datetime.datetime.fromisoformat(g['created_at']).timestamp()for g in valid]
   ids=client.riot('asia','/lol/match/v5/matches/by-puuid/'+urllib.parse.quote(a['puuid'])+'/ids',OUT/f'account-{i}-ids.json','Riot IDs',{'queue':420,'start':0,'count':100,'startTime':int(min(ts))-7200,'endTime':int(max(ts))+7200},i)
  pools.append({'ids':ids,'at':0,'fp':fp});client.log(event='account_pool',account_index=i,opgg_rows=len(games),eligible_opgg=len(valid),riot_ids=len(ids),excluded=dict(exclusions[i]));print(json.dumps({'stage':'pool','account_index':i,'eligible_opgg':len(valid),'riot_ids':len(ids)}),flush=True)
 group_targets={'pro_high':opts.target//2,'ordinary':opts.target-opts.target//2}
 # One successful source game per pass prevents early accounts dominating.
 while len(paired)<opts.target:
  progress=False
  for i,a in enumerate(accounts):
   if source[i]>=15 or groups[a['group']]>=group_targets[a['group']]:continue
   pool=pools[i]
   while pool['at']<len(pool['ids']):
    match_id=pool['ids'][pool['at']];pool['at']+=1
    if match_id in paired:exclusions[i]['duplicate_new']+=1;client.log(event='excluded',account_index=i,match_id=match_id,reason='duplicate_new');continue
    if match_id in old_ids:exclusions[i]['duplicate_old']+=1;client.log(event='excluded',account_index=i,match_id=match_id,reason='duplicate_old');continue
    try:raw=client.riot('asia','/lol/match/v5/matches/'+match_id,OUT/(match_id+'-match.json'),'Riot match',account_index=i)
    except RequestFailure as e:exclusions[i]['request_failure']+=1;client.log(event='excluded',account_index=i,match_id=match_id,reason='request_failure',status=e.status);save();raise
    info=raw['info'];ps=info.get('participants',[]);reason=None
    if not match_id.startswith('KR_')or info.get('queueId')!=420:reason='non420'
    elif len(ps)!=10:reason='not10'
    elif any(p.get('puuid')in old_puuids for p in ps):reason='contains_old_source_puuid'
    elif any(actual[p.get('puuid')]>=15 for p in ps if p.get('puuid')in puuids):reason='selected_participant_cap15'
    game=pool['fp'].get(tuple(fingerprint_riot(raw)))if not reason else None
    if not reason and game is None:reason='opgg_pair_not_found'
    if reason:exclusions[i][reason]+=1;client.log(event='excluded',account_index=i,match_id=match_id,reason=reason);continue
    assert a['puuid']in {p.get('puuid')for p in ps}
    try:client.riot('asia','/lol/match/v5/matches/'+match_id+'/timeline',OUT/(match_id+'-timeline.json'),'Riot timeline',account_index=i)
    except RequestFailure as e:exclusions[i]['request_failure']+=1;client.log(event='excluded',account_index=i,match_id=match_id,reason='timeline_request_failure',status=e.status);save();raise
    paired[match_id]={'match_id':match_id,'opgg':game,'account_index':i,'account_indices':[i],'group':a['group'],'source_tier':a['tier']};source[i]+=1;groups[a['group']]+=1;actual.update(p['puuid']for p in ps if p.get('puuid')in puuids);progress=True;client.log(event='included',account_index=i,match_id=match_id,source_games=source[i],unique_games=len(paired));save();break
  print(json.dumps({'stage':'paired','matches':len(paired),'groups':dict(groups),'source_max':max(source.values(),default=0),'actual_max':max(actual.values(),default=0)}),flush=True)
  if not progress:break
 complete=len(paired)>=opts.target and all(source[i]>0 for i in range(len(accounts)))and max(actual.values(),default=0)<=15 and max(source.values(),default=0)<=15 and groups==group_targets
 summary=save(complete);client.log(event='run_complete',matches=len(paired),complete=complete,groups=dict(groups),per_account=[{'index':i,'included':source[i],'excluded':dict(exclusions[i])}for i in range(len(accounts))])
 if not complete:print(json.dumps(summary,ensure_ascii=False),flush=True);raise SystemExit('Collection incomplete; keep frozen inputs, resume without evaluating')
 assert hashlib.sha256(candidate.read_bytes()).hexdigest()==candidate_hash and hashlib.sha256(accounts_file.read_bytes()).hexdigest()==accounts_hash
 sha=freeze_manifest(OUT);print(json.dumps({'complete':True,'matches':len(paired),'groups':dict(groups),'manifest_sha256':sha}),flush=True)
if __name__=='__main__':
 try:main()
 except RequestFailure as e:print(json.dumps({'blocked':str(e),'layer':e.kind}),flush=True);raise SystemExit(2)
