#!/usr/bin/env python3
"""Freeze 24 KR accounts without inspecting match outcomes or OP scores.
Candidate order: lexicographic Riot PUUID within official tier directory. Only
criteria: requested tier, >=20 solo games in the last 30 days, fresh PUUID and
resolvable Riot/OP.GG identity. Selection uses IDs, not match/timeline payloads.
"""
import datetime,hashlib,json,pathlib,urllib.parse
from r216_data import Client,REPORT,now,old_exclusions,write_json,RequestFailure
OUT=REPORT/'holdout-accounts.json'
def main():
 if not (REPORT/'v21-candidate.json').exists():raise SystemExit('Freeze P2 before account selection')
 if OUT.exists():raise SystemExit('Frozen account list exists; refusing overwrite')
 folder=REPORT/'account-selection';client=Client(folder,'selection-log.jsonl');_,old=old_exclusions();selected=[];seen=set(old)
 cutoff=int((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(days=30)).timestamp())
 tiers=[('CHALLENGER',6,'pro_high','/lol/league/v4/challengerleagues/by-queue/RANKED_SOLO_5x5'),('GRANDMASTER',6,'pro_high','/lol/league/v4/grandmasterleagues/by-queue/RANKED_SOLO_5x5'),('DIAMOND',6,'ordinary','/lol/league/v4/entries/RANKED_SOLO_5x5/DIAMOND/I?page=1'),('PLATINUM',6,'ordinary','/lol/league/v4/entries/RANKED_SOLO_5x5/PLATINUM/I?page=1')]
 for tier,target,group,path in tiers:
  # Probe snapshots were fetched after candidate freeze; keep their exact source.
  snap=folder/(tier.lower()+'-league.json');league=json.loads(snap.read_text())if snap.exists()else client.riot('kr',path,snap,'Riot league');entries=league.get('entries',[])if isinstance(league,dict)else league;count=0
  for order,entry in enumerate(sorted(entries,key=lambda p:p['puuid'])):
   puuid=entry['puuid']
   if puuid in seen:client.log(event='candidate_excluded',tier=tier,order=order,reason='old_or_duplicate_puuid');continue
   candidate=folder/(tier.lower()+'-'+str(order));index=len(selected)
   try:
    ids=client.riot('asia','/lol/match/v5/matches/by-puuid/'+urllib.parse.quote(puuid)+'/ids',candidate.with_suffix('.ids.json'),'Riot IDs',{'queue':420,'start':0,'count':20,'startTime':cutoff},index)
    if len(ids)<20:client.log(event='candidate_excluded',tier=tier,order=order,reason='fewer_than_20_recent_solo',recent_solo=len(ids));continue
    account=client.riot('asia','/riot/account/v1/accounts/by-puuid/'+urllib.parse.quote(puuid),candidate.with_suffix('.riot.json'),'Riot account',account_index=index)
    assert account['puuid']==puuid
    url,op_puuid=client.page(account['gameName'],account['tagLine'],candidate.with_suffix('.page.html'),index)
    games=client.games(url,op_puuid,candidate.with_suffix('.games.flight'),'',index)
    if not games:client.log(event='candidate_excluded',tier=tier,order=order,reason='opgg_no_solo_records');continue
    # tier_info is provenance, never an outcome/prediction selection criterion.
    op_tier=None
    for game in games:
     own=next((p for p in game.get('team_blue',[])+game.get('team_red',[])if p['participant_id']==game.get('participant_id')),None)
     if own and own.get('tier_info'):op_tier=own['tier_info'];break
    record={'gameName':account['gameName'],'tagLine':account['tagLine'],'region':'KR','puuid':puuid,'opggPuuid':op_puuid,'group':group,'tier':tier,'division':entry.get('rank','I'),'recentSoloGames':len(ids),'recentSoloSince':datetime.datetime.fromtimestamp(cutoff,datetime.timezone.utc).isoformat(),'selectedAt':now(),'sourcePage':url,'tierSource':'https://kr.api.riotgames.com'+path,'tierSourceSnapshot':str(snap.relative_to(REPORT)),'tierSourceSHA256':hashlib.sha256(snap.read_bytes()).hexdigest(),'opggTierInfo':op_tier,'selectionEvidencePrefix':str(candidate.relative_to(REPORT)),'selectionOrder':order}
    selected.append(record);seen.add(puuid);count+=1;client.log(event='account_selected',account_index=index,puuid=puuid,opgg_found=True,tier=tier,recent_solo=len(ids));print(json.dumps({'selected':len(selected),'tier':tier,'name':record['gameName']+'#'+record['tagLine']},ensure_ascii=False),flush=True)
   except RequestFailure as e:
    if e.status==404 or e.kind in ['opgg_identity_missing']:client.log(event='candidate_excluded',tier=tier,order=order,reason=e.kind,status=e.status);continue
    raise
   if count==target:break
  if count!=target:raise SystemExit('Insufficient qualifying accounts for '+tier)
 assert len(selected)==24 and len({a['puuid']for a in selected})==24 and not {a['puuid']for a in selected}&old
 with OUT.open('x')as f:json.dump(selected,f,ensure_ascii=False,indent=2);f.write('\n')
 summary={'frozenAt':now(),'accounts':24,'pro_high':12,'ordinary':12,'tiers':{t:6 for t in ['CHALLENGER','GRANDMASTER','DIAMOND','PLATINUM']},'recent_solo_min':20,'old_puuid_overlap':0,'selection_rule':'tier + >=20 solo in last30days; directory sorted by PUUID; no outcomes or scores','user_confirmation':'waived by user 2026-10-05','sha256':hashlib.sha256(OUT.read_bytes()).hexdigest()};write_json(REPORT/'account-selection-summary.json',summary);print(json.dumps(summary),flush=True)
if __name__=='__main__':
 try:main()
 except RequestFailure as e:print(json.dumps({'blocked':str(e),'layer':e.kind}),flush=True);raise SystemExit(2)
