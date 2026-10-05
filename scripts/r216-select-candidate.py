#!/usr/bin/env python3
"""Historical-only, grouped five-fold parameter search; never reads P3 data.
Each fold fits parameters on four account components and evaluates the fifth.
The deployment candidate is refit on all 260 historical training games. Frozen
candidate fold metrics are descriptive; out-of-fold metrics use fold-fitted params.
"""
import collections,datetime,hashlib,itertools,json,pathlib,statistics
import numpy as np
ROOT=pathlib.Path(__file__).resolve().parent.parent
OUT=ROOT/'docs/history/reports/r216/v21-candidate.json'
BASE=np.array([.30,.20,.22,.08,.12,.08])
FACTORS=[.3,.4,.5,.6,.7,.8,1.0]

def historical():
 rows=[];sources={}
 for directory in ['opgg-samples','opgg-validation-new-accounts']:
  folder=ROOT/'docs/history/reports/r211'/directory
  for f in sorted(folder.glob('account-*-riot.json')):
   sources[json.loads(f.read_text())['puuid']]=directory+':'+f.stem
  for p in json.loads((folder/'paired.json').read_text()):
   op=sorted(p['opgg']['team_blue']+p['opgg']['team_red'],key=lambda x:x['participant_id'])
   if len(set(x['stats']['op_score'] for x in op))<2:continue
   raw=folder/(p['match_id']+'-match.json');info=json.loads(raw.read_text())['info']
   rows.append({'match_id':p['match_id'],'path':str(raw.relative_to(ROOT)),'info':info,'op':op,'source_directory':directory,'account_indices':p['account_indices']})
 rows.sort(key=lambda x:x['match_id']);assert len(rows)==260
 return rows,sources

def account_folds(rows,sources):
 parent={p:p for p in sources}
 def find(p):
  if parent[p]!=p:parent[p]=find(parent[p])
  return parent[p]
 for row in rows:
  actual={p['puuid'] for p in row['info']['participants'] if p['puuid'] in sources}
  listed={p for p,name in sources.items() if name in {row['source_directory']+':account-'+str(i)+'-riot' for i in row['account_indices']}}
  row['source_puuids']=sorted(actual|listed)
  assert row['source_puuids']
  for p in row['source_puuids'][1:]:parent[find(p)]=find(row['source_puuids'][0])
 groups=collections.defaultdict(list)
 for p in sources:groups[find(p)].append(p)
 components=sorted(groups.values(),key=lambda g:(-sum(any(p in g for p in r['source_puuids'])for r in rows),sorted(sources[p]for p in g)))
 if len(components)<5:raise SystemExit('Cannot form five source-disjoint folds; no account leakage permitted')
 bins=[[]for _ in range(5)];sizes=[0]*5
 for group in components:
  indices=[i for i,r in enumerate(rows)if any(p in group for p in r['source_puuids'])];fold=min(range(5),key=lambda i:sizes[i]);bins[fold].extend(indices);sizes[fold]+=len(indices)
 fold_accounts=[{p for i in ids for p in rows[i]['source_puuids']} for ids in bins]
 assert all(not fold_accounts[i]&fold_accounts[j] for i in range(5)for j in range(i))
 assert sorted(i for ids in bins for i in ids)==list(range(260))
 return bins,[{'accounts':sorted(sources[p]for p in g),'matches':sum(any(p in g for p in r['source_puuids'])for r in rows)}for g in components]

def normalized_v2(info,factor):
 ps=sorted(info['participants'],key=lambda p:p['participantId']);minutes=info['gameDuration']/60
 role=info['queueId']in[400,420,430,440,490]and len(ps)==10 and all(len([p for p in ps if p['teamId']==t])==5 and {p.get('teamPosition')for p in ps if p['teamId']==t}=={'TOP','JUNGLE','MIDDLE','BOTTOM','UTILITY'}for t in [100,200])
 values=[]
 for p in ps:
  tk=sum(x['kills']for x in ps if x['teamId']==p['teamId'])
  values.append([(p['kills']+factor*p['assists'])/max(1,p['deaths']),min(1,(p['kills']+p['assists'])/tk)if tk else None,p.get('totalDamageDealtToChampions'),p.get('goldEarned'),(p['totalMinionsKilled']+p['neutralMinionsKilled'])/minutes,p.get('visionScore')])
 norm=np.zeros((10,6));available=np.array([all(r[j]is not None and r[j]>=0 for r in values)and any(r[j]>0 for r in values)for j in range(6)])
 for i,p in enumerate(ps):
  for j in range(6):
   if not available[j]:continue
   peers=[values[k][j]for k,q in enumerate(ps)if not role or j<2 or p['teamPosition']==q['teamPosition']];ref=statistics.median(peers)or statistics.mean(peers);x=values[i][j]
   if j==0:x,ref=x**.5,ref**.5
   norm[i,j]=x/(x+ref)if ref>0 else .5
 return norm,available

def metric_counts(scores,wins,official):
 # Official team-max flag handles published OP.GG ties; no op_score argmax.
 m=np.argmax(np.where(wins[None,:,:],scores,-np.inf),axis=2)
 s=np.argmax(np.where(~wins[None,:,:],scores,-np.inf),axis=2)
 indices=np.arange(len(wins))[None,:]
 return official[indices,m],official[indices,s]

def main():
 if OUT.exists():raise SystemExit('Frozen candidate exists; refusing overwrite')
 rows,sources=historical();folds,components=account_folds(rows,sources)
 weights=np.array([BASE+.02*np.array(d)for d in itertools.product(range(-3,4),repeat=6)if sum(d)==0]);weights=weights[np.lexsort(tuple(weights[:,i]for i in range(5,-1,-1)))];distance=np.sum((weights-BASE)**2,axis=1)
 wins=np.array([[p['win']for p in sorted(r['info']['participants'],key=lambda p:p['participantId'])]for r in rows]);official=np.array([[bool(p['stats']['is_opscore_max_in_team'])for p in r['op']]for r in rows]);assert np.all(np.sum(official&wins,axis=1)>=1)and np.all(np.sum(official&~wins,axis=1)>=1)
 best=[None]*6
 def choose(slot,quality,factor,weight_index):
  w=weights[weight_index];dist=(factor-1)**2+float(distance[weight_index]);key=(-int(quality),round(dist,12),factor,tuple(np.round(w,8)))
  if best[slot]is None or key<best[slot]['key']:best[slot]={'key':key,'assistFactor':factor,'weights':w.tolist()}
 for factor in FACTORS:
  data=[normalized_v2(r['info'],factor)for r in rows];norm=np.stack([x[0]for x in data]);available=np.stack([x[1]for x in data])
  for start in range(0,len(weights),256):
   ws=weights[start:start+256];score=2+8*np.einsum('mpd,cd->cmp',norm,ws)/np.einsum('md,cd->cm',available,ws)[:,:,None];m,s=metric_counts(score,wins,official);correct=m.astype(int)+s
   for slot in range(6):
    train=sorted(set(range(260))-set(folds[slot]))if slot<5 else list(range(260));q=np.sum(correct[:,train],axis=1);maximum=int(q.max())
    for index in np.flatnonzero(q==maximum):choose(slot,maximum,factor,start+int(index))
  print(json.dumps({'assistFactor':factor,'weight_sets':len(weights),'stage':'historical-grid'}),flush=True)
 def predictions(params):
  data=[normalized_v2(r['info'],params['assistFactor'])for r in rows];norm=np.stack([x[0]for x in data]);avail=np.stack([x[1]for x in data]);w=np.array(params['weights']);score=2+8*np.einsum('mpd,d->mp',norm,w)/np.sum(avail*w,axis=1)[:,None];m,s=metric_counts(score[None],wins,official);return m[0],s[0]
 baseline={'assistFactor':1,'weights':BASE.tolist()};bm,bs=predictions(baseline);candidate={k:v for k,v in best[5].items()if k!='key'};cm,cs=predictions(candidate);reports=[];oofm=[];oofs=[]
 def metrics(m,s,ids):return {'matches':len(ids),'mvp_correct':int(m[ids].sum()),'svp_correct':int(s[ids].sum()),'mvp_agreement':float(m[ids].mean()),'svp_agreement':float(s[ids].mean())}
 for f,ids in enumerate(folds):
  params={k:v for k,v in best[f].items()if k!='key'};m,s=predictions(params);oofm.extend(m[ids]);oofs.extend(s[ids]);reports.append({'fold':f+1,'match_ids':[rows[i]['match_id']for i in ids],'source_accounts':sorted({sources[p]for i in ids for p in rows[i]['source_puuids']}),'train_matches':260-len(ids),'fold_trained_params':params,'cross_validated':metrics(m,s,ids),'v2_same_fold':metrics(bm,bs,ids),'frozen_candidate_descriptive':metrics(cm,cs,ids)})
 report={'schemaVersion':1,'frozenAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'params':candidate,'baseline':baseline,'matches':260,'source_accounts':20,'search':{'assistFactors':FACTORS,'weightRange':'.02 step, original ±.06, sum 1','weight_sets':len(weights),'objective':'MVP correct + SVP correct; ties minimum squared distance from v2','total_parameter_sets':len(weights)*len(FACTORS)},'grouping':'Connected components of source-account PUUIDs appearing in each match or account_indices; deterministic greedy packing; all sources disjoint between folds','account_components':components,'folds':reports,'out_of_fold':{'mvp_agreement':float(np.mean(oofm)),'svp_agreement':float(np.mean(oofs))},'candidate_historical_descriptive':metrics(cm,cs,list(range(260))),'v2_historical':metrics(bm,bs,list(range(260))),'holdout_used':False,'adoption_rules':{'mvp_change_min':0,'svp_change_min':.02,'rho_change_min':-.01,'ordinary_mvp_change_min':-.02}}
 OUT.parent.mkdir(parents=True,exist_ok=True)
 with OUT.open('x')as file:json.dump(report,file,ensure_ascii=False,indent=2);file.write('\n')
 print(json.dumps({'frozen_candidate':candidate,'fold_sizes':[len(x)for x in folds],'sha256':hashlib.sha256(OUT.read_bytes()).hexdigest()},ensure_ascii=False),flush=True)
if __name__=='__main__':main()
