#!/usr/bin/env python3
"""R218-authorized R217 P2 training only. Never reads the new R217 holdout.
P1 shape/table are immutable; grouped CV selects timeline policy and numerical
thresholds. Settlement stays v2.1. No once-only evaluator is invoked.
"""
import collections,copy,datetime,hashlib,importlib.util,json,os,pathlib,subprocess,tempfile
import numpy as np
spec=importlib.util.spec_from_file_location('ku',pathlib.Path(__file__).with_name('r217-keyword-utils.py'));ku=importlib.util.module_from_spec(spec);spec.loader.exec_module(ku)
ROOT=ku.ROOT;OUT=ku.REPORT

def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def write(p,x):p.parent.mkdir(parents=True,exist_ok=True);p.write_text(json.dumps(x,ensure_ascii=False,indent=2)+'\n')
def load_training():
 wanted={r['match_id']for r in json.loads((ROOT/'backend/testdata/r216/python-v2-golden.json').read_text())};fixtures=[];rows=[];sources=set();owners=[];all_accounts=set()
 accounts216=json.loads((ROOT/'docs/history/reports/r216/holdout-accounts.json').read_text())
 for name in ['opgg-samples','opgg-validation-new-accounts','r216']:
  folder=ROOT/'docs/history/reports/r216/opgg-holdout'if name=='r216'else ROOT/'docs/history/reports/r211'/name
  accounts={i:a['puuid']for i,a in enumerate(accounts216)}if name=='r216'else{int(p.name.split('-')[1]):json.loads(p.read_text())['puuid']for p in folder.glob('account-*-riot.json')}
  all_accounts.update(accounts.values());pairs=json.loads((folder/'paired.json').read_text());sources.add(folder/'paired.json')
  for pair in pairs:
   mid=pair['match_id']
   if name!='r216'and mid not in wanted:continue
   mp=folder/(mid+'-match.json');tp=folder/(mid+'-timeline.json');assert mp.exists()and tp.exists();raw=json.loads(mp.read_text());people=raw['info']['participants'];assert len(people)==10
   own={accounts[i]for i in pair['account_indices']};assert own<={p['puuid']for p in people}
   owners.append(own);fixtures.append({'matchId':mid,'matchPath':str(mp),'timelinePath':str(tp)})
   index=collections.defaultdict(list)
   for p in pair['opgg']['team_blue']+pair['opgg']['team_red']:index[p['champion_id'],p['stats']['kill'],p['stats']['death'],p['stats']['assist']].append(p)
   for p in people:
    aligned=index[p['championId'],p['kills'],p['deaths'],p['assists']];assert len(aligned)==1;op=aligned[0];s=op['stats'];assert s['keyword']in ku.OP
    rows.append({'matchId':mid,'participantId':p['participantId'],'matchIndex':len(fixtures)-1,'sourceAccounts':own,'participants':{q['puuid']for q in people},'expected':ku.OP[s['keyword']],'analysis':s['op_score_timeline_analysis'],'opWin':s['result']=='WIN','opTeamMax':bool(s['is_opscore_max_in_team']),'opCurve':s['op_score_timeline']})
 assert len(fixtures)==560 and len(rows)==5600 and len({r['matchId']for r in fixtures})==560 and len(all_accounts)==44
 parent={a:a for a in all_accounts}
 def find(a):
  while parent[a]!=a:parent[a]=parent[parent[a]];a=parent[a]
  return a
 for own,fixture in zip(owners,fixtures):
  ps={p['puuid']for p in json.loads(pathlib.Path(fixture['matchPath']).read_text())['info']['participants']};present=sorted(own|ps&all_accounts)
  for a in present[1:]:parent[find(a)]=find(present[0])
 counts=collections.Counter(find(next(iter(a)))for a in owners);folds=[[]for _ in range(5)];loads=[0]*5;assignment={}
 for group,count in sorted(counts.items(),key=lambda p:(-p[1],p[0])):
  fold=min(range(5),key=lambda i:loads[i]);assignment[group]=fold;loads[fold]+=count;folds[fold].append(group)
 assert all(loads),'not enough independent account groups'
 for row in rows:row['group']=find(next(iter(row['sourceAccounts'])));row['fold']=assignment[row['group']]
 fold_accounts=[]
 for fold in range(5):fold_accounts.append({a for a in all_accounts if assignment[find(a)]==fold})
 for i in range(5):
  for j in range(i):assert not fold_accounts[i]&fold_accounts[j]
 return fixtures,rows,{'sourceAccounts':44,'connectedAccountGroups':len(counts),'foldMatches':loads,'foldSourceAccountSHA256':[[hashlib.sha256(a.encode()).hexdigest()for a in sorted(ps)]for ps in fold_accounts],'accountOverlapAcrossFolds':0,'policy':'connected components of all 44 source PUUIDs present in a game; all ten people and the whole game stay in one fold'},sources

def threshold(x,positive,negative,default):
 order=np.argsort(x,kind='stable');v=x[order];p=positive[order].astype(int);n=negative[order].astype(int);cuts=np.r_[0,np.flatnonzero(np.diff(v)>0)+1,len(v)];cp=np.r_[0,np.cumsum(p)];cn=np.r_[0,np.cumsum(n)];score=cn[cuts]+cp[-1]-cp[cuts];best=cuts[score==score.max()];ts=np.array([v[0]-1e-9 if c==0 else v[-1]+1e-9 if c==len(v)else(v[c-1]+v[c])/2 for c in best]);t=float(ts[np.argmin(abs(ts-default))]);return t,int(score.max())

def array_features(tags):
 early=[];late=[];final=[];gap=[];peak=[]
 for row in tags:
  y=row['checkpoints'];k=len(y)//2;assert k>=1
  early.append(np.median(y[:k]));late.append(np.median(y[k:]));final.append(y[-1]);gap.append(y[-1]-max(y[max(0,len(y)-8):-1]));peak.append(y[-1]-max(y))
 return [np.array(x)for x in [early,late,final,gap,peak]]

def fit(rows,tags,model):
 model=copy.deepcopy(model);p=model['rules'];early,late,final,gap,peak=array_features(tags);truth={side:np.array([r['analysis'][side]for r in rows])for side in ['left','right','last']}
 p['leftThreshold'],_=threshold(early,truth['left']=='UP',truth['left']=='DOWN',6);p['rightThreshold'],_=threshold(late,truth['right']=='UP',truth['right']=='DOWN',6);left=early>=p['leftThreshold'];right=late>=p['rightThreshold'];best=None
 for baseline in np.linspace(5.4,6.6,25):
  poor=right&(final<baseline);excellent=~right&(final>=baseline);forced=poor|excellent;cut,quality=threshold(gap,(truth['last']=='GOOD')&~forced,(truth['last']=='FAIR')&~forced,-.3);correct=quality+int(np.sum(poor&(truth['last']=='POOR')))+int(np.sum(excellent&(truth['last']=='EXCELLENT')));ranking=(correct,-abs(baseline-6),-abs(cut+.3))
  if best is None or ranking>best[0]:best=(ranking,float(baseline),cut)
 p['endingBaseline']=best[1];p['endingGap']=best[2]
 pred=[ku.analyze(t['checkpoints'],p)for t in tags];amb=np.array([i for i,(tag,a)in enumerate(zip(tags,pred))if tag['win']and a=={'left':'UP','right':'UP','last':'GOOD'}],dtype=int)
 if len(amb):p['peakGap'],_=threshold(peak[amb],np.array([rows[i]['expected']=='unstoppable'for i in amb]),np.array([rows[i]['expected']=='leader'for i in amb]),-.01)
 return model

def predict(tags,model):
 out=[]
 for tag in tags:
  key,a=ku.predict(tag['checkpoints'],tag['win'],tag['teamMax'],model);out.append({'rawKeyword':key,'analysis':a})
 return out

def export(fixtures,models,path):
 with tempfile.TemporaryDirectory(prefix='r217-training-')as tmp:
  inp=pathlib.Path(tmp)/'input.json';write(inp,{'matches':fixtures,'models':models});env={**os.environ,'R217_KEYWORD_DUMP_INPUT':str(inp),'R217_KEYWORD_DUMP_OUTPUT':str(path)};subprocess.run(['go','test','./backend','-run','^TestR217KeywordDump$','-count=1'],cwd=ROOT,env=env,check=True)
 return json.loads(path.read_text())

def main():
 candidate=OUT/'keyword-candidate.json'
 if candidate.exists():raise SystemExit('Candidate already frozen; training must not resume')
 frozen=OUT/'rule-freeze.json';freeze=json.loads(frozen.read_text());assert digest(OUT/'opgg-rule-study.json')==freeze['studySHA256'];fixtures,rows,groups,sources=load_training();seed=json.loads((ROOT/'backend/testdata/r217/keyword-candidate.json').read_text());models={}
 for policy in ['omit-and-renormalize','neutral-baseline']:
  for smoothing in [0.,2.,5.]:
   model=copy.deepcopy(seed);model['timeline']['missingPolicy']=policy;model['timeline']['smoothingMinutes']=smoothing;models[f'{policy}/smooth{int(smoothing)}']=model
 exported=export(fixtures,models,OUT/'training-curves.json');evaluations=[];aligned={}
 for name,model in models.items():
  index={(r['matchId'],r['participantId']):r for batch in exported[name]for r in batch};assert len(index)==5600,'missing minute scores must be reported, not silently excluded';tags=[index[r['matchId'],r['participantId']]for r in rows];aligned[name]=tags;oof=[None]*5600;folds=[]
  for fold in range(5):
   train=[i for i,r in enumerate(rows)if r['fold']!=fold];valid=[i for i,r in enumerate(rows)if r['fold']==fold];chosen=fit([rows[i]for i in train],[tags[i]for i in train],model);pred=predict([tags[i]for i in valid],chosen)
   for i,v in zip(valid,pred):oof[i]=v
   folds.append({'fold':fold+1,'trainingMatches':len(train)//10,'validationMatches':len(valid)//10,'rules':chosen['rules'],'metrics':ku.summarize([rows[i]for i in valid],pred)})
  evaluations.append({'name':name,'timeline':model['timeline'],'oof':ku.summarize(rows,oof),'folds':folds})
 best=max(evaluations,key=lambda x:(x['oof']['accuracy'],x['oof']['macroF1'],-x['timeline']['smoothingMinutes'],x['timeline']['missingPolicy']=='omit-and-renormalize'));name=best['name'];chosen=fit(rows,aligned[name],models[name]);selected=predict(aligned[name],chosen);upper=[];correlations=[]
 for row,tag in zip(rows,aligned[name]):
  curve=[p['score']for p in row['opCurve']];key,a=ku.predict(curve,row['opWin'],row['opTeamMax'],{'rules':freeze['rules'],'table':freeze['table']});upper.append({'rawKeyword':key,'analysis':a});op=np.array(curve);own=np.interp([p['second']for p in row['opCurve']],tag['seconds'],tag['checkpoints'])
  if np.std(op)>1e-12 and np.std(own)>1e-12:correlations.append(float(np.corrcoef(op,own)[0,1]))
 report={'stage':'P2 training only','generatedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'matches':560,'participants':5600,'settlementParamsUnchanged':True,'settlementVersion':'v2.1','ruleFreezeSHA256':digest(frozen),'groupedCrossValidation':groups,'selectionObjective':'highest grouped out-of-fold end-to-end keyword accuracy, then macro F1, then less smoothing / renormalize','thresholdStartingPoint':6.0,'lastBaselineSearch':[5.4,6.6,25],'endingWindowFixed':7,'curveVariants':evaluations,'selectedVariant':name,'selectedRules':chosen['rules'],'trainingRefit':ku.summarize(rows,selected),'groupedOOF':best['oof'],'opggOwnCurveUpperReference':ku.summarize(rows,upper),'pointCorrelation':{'definition':'per-participant Pearson correlation after interpolating our minute curve to OP.GG seconds; same participant, no scores borrowed','median':float(np.median(correlations)),'validParticipants':len(correlations),'undefinedParticipants':5600-len(correlations)},'productionDisabledKeywordsUnchanged':True,'newHoldoutRead':False,'goExportSHA256':digest(OUT/'training-curves.json'),'sources':[{'path':str(p.relative_to(ROOT)),'sha256':digest(p)}for p in sorted(sources)]}
 report_path=OUT/'training-keyword-report.json';write(report_path,report);chosen.update({'schemaVersion':1,'frozenAt':report['generatedAt'],'ruleFreezeSHA256':digest(frozen),'trainingReportSHA256':digest(report_path),'trainingMatches':560,'trainingParticipants':5600,'settlementVersion':'v2.1','openingRule':{'precisionAtLeast':.5,'evaluationOwner':'Claude','evaluateOnce':True},'disabledKeywordsPendingEvaluation':['leader','victorious','latebloomer','dedication','average','rollercoaster','decline','innocent','slowstarter','unyielding']})
 # Go/Python parity on historical data before the candidate is frozen.
 verified=export(fixtures,{'selected':chosen},OUT/'training-selected-go.json')['selected'];goindex={(r['matchId'],r['participantId']):r for batch in verified for r in batch}
 for row,pred in zip(rows,selected):
  actual=goindex[row['matchId'],row['participantId']];assert actual['rawKeyword']==pred['rawKeyword']and actual['analysis']==pred['analysis'],'Python/production Go rule mismatch'
 with candidate.open('x')as f:json.dump(chosen,f,ensure_ascii=False,indent=2);f.write('\n')
 (ROOT/'backend/testdata/r217/keyword-candidate.json').write_bytes(candidate.read_bytes())
 print(json.dumps({'frozenCandidateSHA256':digest(candidate),'selected':name,'rules':chosen['rules'],'trainingAccuracy':report['trainingRefit']['accuracy'],'OOFAccuracy':report['groupedOOF']['accuracy'],'OPGGUpper':report['opggOwnCurveUpperReference']['accuracy'],'pointCorrelation':report['pointCorrelation'],'groups':groups['foldMatches']},ensure_ascii=False))
if __name__=='__main__':main()
