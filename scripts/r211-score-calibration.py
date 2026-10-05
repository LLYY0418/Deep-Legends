#!/usr/bin/env python3
"""Offline only: fit on 70%, evaluate the untouched 30%; never relax the gate.
Requires numpy (Codex bundled Python includes it). Input: collector paired.json.
Search is a deterministic coordinate grid (0.02 weight transfers, sum=1,
max=0.30), with independent k candidates; held-out matches never select params.
"""
import json,pathlib,math,random,statistics,numpy as np
ROOT=pathlib.Path(__file__).resolve().parent.parent;OUT=ROOT/'docs/history/reports/r211';SAMPLES=OUT/'opgg-samples'
KEYS=['kp','kill','death','dmg','gold','cs','vision','tank','util','obj']
ROLES=['TOP','JUNGLE','MIDDLE','BOTTOM','UTILITY','NONE']
INITIAL=np.array([[.12,.10,.12,.16,.08,.10,.05,.12,.03,.12],[.16,.10,.12,.12,.06,.08,.08,.08,.04,.16],[.14,.12,.12,.20,.08,.10,.05,.04,.03,.12],[.12,.12,.12,.22,.10,.12,.04,.02,.02,.12],[.20,.04,.12,.08,.02,0,.18,.08,.18,.10],[.18,.12,.12,.22,.06,0,0,.12,.10,.08]])
def raw_match(info):
 ps=sorted(info['participants'],key=lambda p:p['participantId']);mins=info['gameDuration']/60
 # Compare sets, preserving the prescribed role order rather than lexical order.
 role_ok=info['queueId'] in [400,420,430,440,490] and len(ps)==10 and all(set(p.get('teamPosition','') for p in ps if p['teamId']==t)==set(ROLES[:5]) and len([p for p in ps if p['teamId']==t])==5 for t in [100,200])
 rows=[];roles=[];util=[]
 for p in ps:
  team=[x for x in ps if x['teamId']==p['teamId']];tk=sum(x['kills'] for x in team);td=sum(x['totalDamageDealtToChampions'] for x in team);tg=sum(x['goldEarned'] for x in team)
  complete_tank=all('damageSelfMitigated' in x and 'totalDamageTaken' in x for x in ps)
  tank=sum(x['totalDamageTaken']+x['damageSelfMitigated'] for x in team) if complete_tank else None
  heals=(p.get('totalHealsOnTeammates',0)+p.get('totalDamageShieldedOnTeammates',0))/mins if all(k in p for k in ['totalHealsOnTeammates','totalDamageShieldedOnTeammates']) else None
  cc=p.get('timeCCingOthers');cc=cc/mins if cc is not None else None
  challenges=p.get('challenges',{})
  if all(all(k in x.get('challenges',{}) for k in ['dragonTakedowns','baronTakedowns','riftHeraldTakedowns']) for x in ps):obj=p.get('turretTakedowns',0)+sum(challenges[k] for k in ['dragonTakedowns','baronTakedowns','riftHeraldTakedowns'])
  else:
   # Building damage contributes a match-relative share, never an arbitrary /1000 count.
   building_total=sum(x.get('damageDealtToBuildings',0) for x in ps)
   obj=p['turretTakedowns']+(p['damageDealtToBuildings']/building_total if building_total else 0) if 'damageDealtToBuildings' in p and 'turretTakedowns' in p else None
  rows.append([(p['kills']+p['assists'])/tk if tk else 0,p['kills']/tk if tk else 0,p['deaths'],p['totalDamageDealtToChampions']/td if td else 0,p['goldEarned']/tg if tg else 0,(p['totalMinionsKilled']+p['neutralMinionsKilled'])/mins,p['visionScore']/mins if 'visionScore' in p else None,(p['totalDamageTaken']+p['damageSelfMitigated'])/tank if tank else (0 if tank==0 else None),None,obj]);util.append([heals,cc]);roles.append(ROLES.index(p['teamPosition']) if role_ok else 5)
 return ps,rows,roles,util,role_ok

def normalized(info,k):
 ps,rows,roles,util,role_ok=raw_match(info);n=np.zeros((10,10));available=np.ones(10,dtype=bool)
 epsilon=[.01,.01,1,.01,.01,.1,.1,.01,5,1]
 for j in range(10):
  values=[r[j] for r in rows]
  if j==8:
   if any(v is None for u in util for v in u):available[j]=False;continue
  elif any(v is None for v in values):available[j]=False;continue
  for i in range(10):
   peers=[z for z in range(10) if z!=i and (not role_ok or j not in [3,4,5,6,7,8] or roles[z]==roles[i])]
   if j==8:
    n[i,j]=sum(.5+.5*math.tanh(math.log((util[i][u]+5)/(statistics.median(util[z][u] for z in peers)+5))/k) for u in [0,1])/2;continue
   ref=statistics.median(values[z] for z in peers);value=values[i]
   ratio=(ref+1)/(value+1) if j==2 else (value+epsilon[j])/(ref+epsilon[j]);n[i,j]=.5+.5*math.tanh(math.log(ratio)/k)
 return n,available,np.array(roles),np.array([p['win'] for p in ps])

def v2(info):
 ps,_,roles,_,role_ok=raw_match(info);mins=info['gameDuration']/60;values=[]
 for p in ps:
  team=[x for x in ps if x['teamId']==p['teamId']];tk=sum(x['kills'] for x in team)
  values.append([(p['kills']+p['assists'])/max(1,p['deaths']),(p['kills']+p['assists'])/tk if tk else None,p['totalDamageDealtToChampions'],p['goldEarned'],(p['totalMinionsKilled']+p['neutralMinionsKilled'])/mins,p.get('visionScore')])
 weights=np.array([.3,.2,.22,.08,.12,.08]);active=[j for j in range(6) if all(r[j] is not None for r in values) and any(r[j]>0 for r in values)];total=sum(weights[j] for j in active);scores=[]
 for i,r in enumerate(values):
  score=2
  for j in active:
   peers=[z for z in range(10) if not role_ok or j<2 or roles[z]==roles[i]];v=[values[z][j] for z in peers];ref=statistics.median(v) or statistics.mean(v);x=r[j]
   if j==0:x,ref=math.sqrt(x),math.sqrt(ref)
   score+=8*weights[j]/total*(x/(x+ref) if ref>0 else .5)
  scores.append(score)
 return np.array(scores)
def ranks(scores):return np.argsort(np.argsort(-scores,axis=1,kind='stable'),axis=1,kind='stable')+1
def average_ranks(scores):
 # Spearman uses average ranks for tied published OP Scores.
 return 1+np.sum(scores[:,None,:]>scores[:,:,None],axis=2)+(np.sum(scores[:,None,:]==scores[:,:,None],axis=2)-1)/2
def metrics(scores,target,wins):
 predicted=average_ranks(scores);expected=average_ranks(target)
 a=predicted-predicted.mean(axis=1,keepdims=True);b=expected-expected.mean(axis=1,keepdims=True)
 rho=np.sum(a*b,axis=1)/np.sqrt(np.sum(a*a,axis=1)*np.sum(b*b,axis=1))
 mvp=np.argmax(np.where(wins,scores,-np.inf),axis=1)==np.argmax(np.where(wins,target,-np.inf),axis=1)
 svp=np.argmax(np.where(~wins,scores,-np.inf),axis=1)==np.argmax(np.where(~wins,target,-np.inf),axis=1)
 return {'mvp_agreement':float(np.mean(mvp)),'svp_agreement':float(np.mean(svp)),'spearman_median':float(np.median(rho))}
def score(norm,available,roles,weights):
 w=weights[roles]*available[:,None,:];return 10*np.sum(norm*w,axis=2)/np.sum(w,axis=2)
def objective(scores,target,wins):
 m=metrics(scores,target,wins)
 return 2*m['mvp_agreement']+2*m['svp_agreement']+m['spearman_median']-.02*np.mean((scores-target)**2)
def main():
 if (OUT/'score-calibration-initial.json').exists():
  raise SystemExit("Original split is frozen; use r211-refine-official-calibration.py and the one-shot new-account evaluator. Historical score-argmax labels are not official MVP labels.")
 paired=json.loads((SAMPLES/'paired.json').read_text())
 # OP.GG leaves remake/unscored matches at ten zero scores; no ranking exists.
 excluded=[x['match_id'] for x in paired if len(set(p['stats']['op_score'] for p in x['opgg']['team_blue']+x['opgg']['team_red']))<2]
 pairs=sorted([x for x in paired if x['match_id'] not in excluded],key=lambda x:x['match_id']);random.Random(211).shuffle(pairs)
 accounts=set(i for x in pairs for i in x['account_indices'])
 if len(pairs)<150 or len(accounts)<10:raise SystemExit('Need >=150 paired matches and >=10 accounts before fitting')
 infos=[json.loads((SAMPLES/(x['match_id']+'-match.json')).read_text())['info'] for x in pairs]
 if any(not x['match_id'].startswith('KR_') or info['queueId']!=420 or len(info['participants'])!=10 for x,info in zip(pairs,infos)):raise SystemExit('Only complete KR solo-ranked matches qualify')
 target=np.array([[p['stats']['op_score'] for p in sorted(x['opgg']['team_blue']+x['opgg']['team_red'],key=lambda p:p['participant_id'])] for x in pairs])
 split=int(len(pairs)*.7);best=None
 # Quantize each row to units of .02, preserving its unit sum and cap.
 initial=np.rint(INITIAL*50).astype(int)
 for row in initial:
  while row.sum()!=50:row[np.argmax(row)] += 1 if row.sum()<50 else -1
 for k in [.5,.7,.9,1.1,1.3,1.5]:
  raw=[normalized(info,k) for info in infos];norm=np.stack([r[0] for r in raw]);available=np.stack([r[1] for r in raw]);roles=np.stack([r[2] for r in raw]);wins=np.stack([r[3] for r in raw]);weights=initial/50
  quality=objective(score(norm[:split],available[:split],roles[:split],weights),target[:split],wins[:split]);evaluations=1
  for iteration in range(30):
   improved=False
   for role in range(5):
    selected=None;selected_quality=quality
    for a in range(10):
     for b in range(10):
      if a==b or weights[role,a]<.019 or weights[role,b]>.279:continue
      candidate=weights.copy();candidate[role,a]-=.02;candidate[role,b]+=.02
      q=objective(score(norm[:split],available[:split],roles[:split],candidate),target[:split],wins[:split]);evaluations+=1
      if q>selected_quality+1e-9:selected,selected_quality=candidate,q
    if selected is not None:weights,quality=selected,selected_quality;improved=True
   if not improved:break
  print(json.dumps({'k':k,'train_objective':quality,'evaluations':evaluations}),flush=True)
  if best is None or quality>best[0]:best=(quality,k,weights,norm,available,roles,wins)
 quality,k,weights,norm,available,roles,wins=best;new=score(norm,available,roles,weights);old=np.stack([v2(info) for info in infos])
 report={'excluded_unscored_match_ids':excluded,'matches':len(pairs),'accounts':len(accounts),'train_count':split,'holdout_count':len(pairs)-split,'split_seed':211,'k':k,'weights':{role:dict(zip(KEYS,(round(float(value),2) for value in row))) for role,row in zip(ROLES,weights)},'v2':metrics(old[split:],target[split:],wins[split:]),'v3':metrics(new[split:],target[split:],wins[split:]),'training_v3':metrics(new[:split],target[:split],wins[:split]),'grid_search':'coordinate weight transfers .02, <=.30, sum=1; k .5/.7/.9/1.1/1.3/1.5; train only','holdout_match_ids':[x['match_id'] for x in pairs[split:]],'train_match_ids':[x['match_id'] for x in pairs[:split]]}
 m=report['v3'];report['passed']=m['mvp_agreement']>=.8 and m['svp_agreement']>=.75 and m['spearman_median']>=.8
 (OUT/'score-calibration.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False),flush=True)
 if not report['passed']:raise SystemExit(3)
if __name__=='__main__':main()
