#!/usr/bin/env python3
"""R217 P1: transparent rule study on the explicitly authorized 560 training games.
Never invokes scorers, evaluates new holdout data, changes production or collects.
"""
import collections,datetime,hashlib,json,pathlib,sys
import numpy as np
ROOT=pathlib.Path(__file__).resolve().parent.parent
OUT=ROOT/'docs/history/reports/r217/opgg-rule-study.json'

def unique_last(curve):
 # OP.GG sometimes emits a regular minute and settlement at the same timestamp.
 # Interpolation selects the final emitted score, without deleting raw points.
 keep=np.r_[np.diff(curve[:,0])!=0,True];return curve[keep,0],curve[keep,1]

def load():
 reference=ROOT/'backend/testdata/r216/python-v2-golden.json';wanted={p['match_id']for p in json.loads(reference.read_text())};pairs=[];sources=[]
 for folder in ['opgg-samples','opgg-validation-new-accounts']:
  p=ROOT/'docs/history/reports/r211'/folder/'paired.json';sources.append(p);pairs.extend((row,'R211')for row in json.loads(p.read_text())if row['match_id']in wanted)
 assert len(pairs)==260
 p=ROOT/'docs/history/reports/r216/opgg-holdout/paired.json';sources.append(p);pairs.extend((row,'R216-now-training')for row in json.loads(p.read_text()))
 assert len(pairs)==560 and len({p['match_id']for p,_ in pairs})==560
 rows=[]
 for pair,origin in pairs:
  people=pair['opgg']['team_blue']+pair['opgg']['team_red'];assert len(people)==10
  curves=[np.array([[p['second']/60,p['score']]for p in x['stats']['op_score_timeline']],dtype=float)for x in people]
  for i,person in enumerate(people):
   s=person['stats'];a=s['op_score_timeline_analysis'];curve=curves[i];assert len(curve)>=2 and np.all(np.diff(curve[:,0])>=0) and curve[-1,0]>curve[0,0]and set(a)=={'left','right','last'}
   team=[j for j,p in enumerate(people)if p['team_key']==person['team_key']];peers=np.stack([np.interp(curve[:,0],*unique_last(c))for c in curves]);sorted_scores=sorted((people[j]['stats']['op_score']for j in team),reverse=True)
   rows.append({'matchId':pair['match_id'],'participantId':person['participant_id'],'source':origin,'win':s['result']=='WIN','teamMax':bool(s['is_opscore_max_in_team']),'rank':s['op_score_rank'],'finalScore':s['op_score'],'topGap':sorted_scores[0]-sorted_scores[1],'ownMinusSecond':s['op_score']-sorted_scores[1],'fullScore':s['op_score']==10,'keyword':s['keyword'],'analysis':a,'curve':curve,'teamMean':peers[team].mean(axis=0),'teamMedian':np.median(peers[team],axis=0),'globalMean':peers.mean(axis=0),'globalMedian':np.median(peers,axis=0)})
 return rows,[{'path':str(p.relative_to(ROOT)),'sha256':hashlib.sha256(p.read_bytes()).hexdigest()}for p in [reference,*sources]]

def binary_fit(x,y,default=0):
 order=np.argsort(x,kind='stable');sx=x[order];sy=y[order].astype(int);cuts=np.r_[0,np.flatnonzero(np.diff(sx)>0)+1,len(x)];positive=np.r_[0,np.cumsum(sy)];quality=cuts-positive[cuts]+positive[-1]-positive[cuts]
 best=int(quality.max());candidates=cuts[quality==best];thresholds=np.array([sx[0]-1e-9 if i==0 else sx[-1]+1e-9 if i==len(x)else(sx[i-1]+sx[i])/2 for i in candidates]);at=int(np.argmin(abs(thresholds-default)));threshold=float(thresholds[at]);return threshold,x>=threshold,best

def add(bank,name,values,kind,definition,sides=('left','right','last')):
 values=np.asarray(values,dtype=float)
 if len(values) and np.isfinite(values).all():bank[name]={'values':values,'kind':kind,'definition':definition,'sides':sides}

def aggregates(t,y):
 mean=float(y.mean());median=float(np.median(y));diff=float(y[-1]-y[0]);slope=float(np.sum((t-t.mean())*(y-mean))/np.sum((t-t.mean())**2))if len(y)>1 and np.ptp(t)>0 else 0.
 area=float(np.trapezoid(y,t)/(t[-1]-t[0]))if len(t)>1 and t[-1]>t[0]else mean
 return {'mean':mean,'median':median,'timeWeightedMean':area,'endpointDifference':diff,'olsSlope':slope,'range':float(np.ptp(y)),'minimum':float(y.min()),'maximum':float(y.max())}

def features(rows):
 bank={};raw=collections.defaultdict(list);defs={}
 def push(key,value,kind,definition,sides):raw[key].append(value);defs[key]=(kind,definition,sides)
 splits=[('points',f)for f in [1/3,.4,.5,.6,2/3,.75,.8,.9]]+[('time',f)for f in [1/3,.4,.5,.6,2/3,.75,.8,.9]]+[('fixedMinute',m)for m in [5,10,15,20]]
 for row in rows:
  curve=row['curve'];t=curve[:,0];y=curve[:,1];final=float(y[-1]);n=len(y)
  for measure,value in [('finalScore',final),('rank',row['rank']),('fullScore',float(row['fullScore'])),('topGap',row['topGap']),('ownMinusSecond',row['ownMinusSecond']),('wholeMean',y.mean()),('wholeMedian',np.median(y)),('minimum',y.min()),('maximum',y.max()),('range',np.ptp(y)),('finalMinusMin',final-y.min()),('finalMinusMax',final-y.max()),('finalMinusMean',final-y.mean()),('lastDelta',final-y[-2]),('lastSlope',(final-y[-2])/(t[-1]-t[-2]) if t[-1]>t[-2] else 0.),('wholeDifference',final-y[0])]:push(measure,value,'level'if measure in ['finalScore','wholeMean','wholeMedian','minimum','maximum']else'difference',measure,('lookup',) if measure in ['rank','fullScore','topGap','ownMinusSecond'] else ('left','right','last','lookup'))
  for ref in ['teamMean','teamMedian','globalMean','globalMedian']:
   push(f'ending:relativeTo:{ref}',final-row[ref][-1],'difference',f'Final curve score minus {ref} at same final timestamp',('last','lookup'))
  for mode,split in splits:
   k=int(n*split)if mode=='points'else int(np.searchsorted(t,t[-1]*split if mode=='time'else split,side='right'));k=max(1,min(n-1,k));label=f'{mode}:{split:g}'
   for side,sl in [('left',slice(0,k)),('right',slice(k,n))]:
    for metric,value in aggregates(t[sl],y[sl]).items():push(f'{label}/{side}/{metric}',value,'level'if metric in ['mean','median','timeWeightedMean','minimum','maximum']else'difference',f'{side} segment, split={mode} {split:g}; {metric}',(side,'last','lookup'))
    for ref in ['teamMean','teamMedian','globalMean','globalMedian']:push(f'{label}/{side}/relativeTo:{ref}',np.mean(y[sl]-row[ref][sl]),'difference',f'{side} segment mean minus {ref}, split={mode} {split:g}',(side,'last','lookup'))
  for window in [1,2,3,4,5,6,7,8,9,10,12,15]:
   for mode in ['points','minutes']:
    start=max(0,n-window-1)if mode=='points'else int(np.searchsorted(t,t[-1]-window));start=min(n-2,start);sl=slice(start,n-1)
    for metric,value in aggregates(t[sl],y[sl]).items():push(f'preEnd:{mode}:{window}/{metric}',value,'level'if metric in ['mean','median','timeWeightedMean','minimum','maximum']else'difference',f'Exclude final point; trailing {window} {mode}; {metric}',('right','last','lookup'))
  # Endpoint relative to extrema with dimensionless and score-proportional gaps.
  for mode,window in [('points',w)for w in [2,3,5,8,10,15]]+[('fraction',f)for f in [1/3,.5,2/3,1.]]:
   start=max(0,n-window-1)if mode=='points'else max(0,int(n*(1-window)));start=min(n-2,start);segment=y[start:];maximum=float(segment.max());minimum=float(segment.min());span=maximum-minimum
   for metric,value in [('peakRatio',final/maximum if maximum else 0.),('rangePosition',(final-minimum)/span if span>1e-9 else .5),('standardizedGap',(final-maximum)/max(float(segment.std()),1e-9)),('relativeTo5',final-5),('peakGapOverDistanceTo5',(final-maximum)/max(abs(maximum-5),.1))]:push(f'ending:{mode}:{window:g}/{metric}',value,'difference',f'Final {metric} over trailing {window:g} {mode}, include final point',('last','lookup'))
  # Decaying reference is an explicit exponentially weighted prior mean.
  for minutes in [1,2,3,5,8,10,15]:
   w=np.exp((t[:-1]-t[-1])/minutes);reference=float(np.sum(w*y[:-1])/w.sum());push(f'ending:decayMinutes:{minutes}',final-reference,'difference',f'Final minus pre-end exponentially weighted mean; decay {minutes} minutes',('last','lookup'))
  # Global least squares polynomials remain explicit formulas, no fitted black box.
  for degree in [1,2,3,4]:
   for anchor in [False,True]:
    tx=t/t[-1];yy=y
    if anchor:tx=np.r_[0.,tx];yy=np.r_[5.,y]
    coeff=np.polynomial.polynomial.polyfit(tx,yy,min(degree,len(tx)-1))
    for at in [0,.25,1/3,.5,2/3,.75,1.]:
     val=float(np.polynomial.polynomial.polyval(at,coeff));slope=float(np.polynomial.polynomial.polyval(at,np.polynomial.polynomial.polyder(coeff)));prefix=f'globalPolynomial:degree{degree}:anchor5={anchor}:at{at:g}'
     sides=('left','last','lookup')if at<=.5 else ('right','last','lookup')
     push(prefix+'/level',val,'level',f'OLS polynomial degree {degree}, normalized time, anchor at (0,5)={anchor}; value at {at:g}',sides);push(prefix+'/slope',slope,'difference',f'OLS polynomial degree {degree}, normalized time, anchor at (0,5)={anchor}; derivative at {at:g}',sides)
     if at==1:push(prefix+'/residual',final-val,'difference',f'Final value minus fitted degree {degree} endpoint, anchor5={anchor}',('last','lookup'))
 for name,values in raw.items():kind,definition,sides=defs[name];add(bank,name,values,kind,definition,sides)
 return bank

def confusion(actual,pred):return [{'actual':a,'predicted':p,'count':n}for(a,p),n in sorted(collections.Counter(zip(actual,pred)).items())]
def errors(rows,actual,pred,feature):
 wrong=[i for i,(a,p)in enumerate(zip(actual,pred))if a!=p];duration=collections.Counter();near=0;turns=collections.Counter();source=collections.Counter()
 for i in wrong:
  row=rows[i];curve=row['curve'];minutes=curve[-1,0];duration['<15'if minutes<15 else '15-25'if minutes<25 else '25-35'if minutes<35 else '>=35']+=1;source[row['source']]+=1
  signs=np.sign(np.diff(curve[:,1]));turns['manyReversals(>=6)'if np.sum(signs[1:]!=signs[:-1])>=6 else 'fewReversals(<6)']+=1;near+=abs(float(feature[i]))<.1
 breakdown={}
 for label in sorted(set(actual)):
  indices=[i for i,a in enumerate(actual)if a==label];mistakes=sum(actual[i]!=pred[i]for i in indices);breakdown[label]={'participants':len(indices),'wrong':mistakes,'errorRate':mistakes/len(indices)}
 groups=collections.defaultdict(lambda:[0,0])
 for i,row in enumerate(rows):
  curve=row['curve'];minutes=curve[-1,0];signs=np.sign(np.diff(curve[:,1]));reversals=int(np.sum(signs[1:]!=signs[:-1]));a=row['analysis']
  for group,value in [('duration','<15'if minutes<15 else '15-25'if minutes<25 else '25-35'if minutes<35 else '>=35'),('reversals','many(>=6)'if reversals>=6 else 'few(<6)'),('source',row['source']),('analysis',f"{a['left']}/{a['right']}/{a['last']}")]:
   groupkey=group+':'+value;groups[groupkey][0]+=1;groups[groupkey][1]+=int(actual[i]!=pred[i])
 return {'byActualClass':breakdown,'groupRates':{key:{'participants':total,'wrong':wrong_count,'errorRate':wrong_count/total}for key,(total,wrong_count)in sorted(groups.items())},'wrong':len(wrong),'confusion':confusion(actual,pred),'byDurationMinutes':dict(duration),'bySource':dict(source),'byReversals':dict(turns),'featureWithinPointOneOfBoundary':near,'examples':[{'matchId':rows[i]['matchId'],'participantId':rows[i]['participantId'],'actual':actual[i],'predicted':pred[i],'curve':[{'second':round(float(t)*60),'score':float(v)}for t,v in rows[i]['curve']]}for i in wrong[:20]]}

def lookup(rows,bank):
 cells=collections.defaultdict(collections.Counter)
 for row in rows:a=row['analysis'];cells[(row['win'],row['teamMax'],a['left'],a['right'],a['last'])][row['keyword']]+=1
 result=[];correct=0
 for key,counter in sorted(cells.items()):major,n=counter.most_common(1)[0];total=sum(counter.values());correct+=n;result.append({'win':key[0],'teamMax':key[1],'left':key[2],'right':key[3],'last':key[4],'counts':dict(counter),'majority':major,'majorityShare':n/total,'participants':total})
 amb=[i for i,row in enumerate(rows)if row['win'] and tuple(row['analysis'][k] for k in ['left','right','last'])==('UP','UP','GOOD')];target=np.array([rows[i]['keyword']=='UNSTOPPABLE'for i in amb]);candidates=[];best=None
 for name,feature in bank.items():
  if 'lookup'not in feature['sides']:continue
  x=feature['values'][amb];threshold,pred,count=binary_fit(x,target,5 if feature['kind']=='level'else 0);record={'feature':name,'definition':feature['definition'],'threshold':threshold,'predictUnstoppable':'value>=threshold','ambiguousCorrect':count,'ambiguousParticipants':len(amb),'overallCorrect':len(rows)-len(amb)+count,'overallAccuracy':(len(rows)-len(amb)+count)/len(rows)};candidates.append(record)
  if best is None or count>best[0]:best=(count,record,pred)
 peak=np.array([rows[i]['curve'][-1,1]-rows[i]['curve'][:,1].max()for i in amb]);natural=[]
 for tolerance in [0.,.001,.01,.05,.1]:
  count=int(np.sum((peak>=-tolerance)==target));natural.append({'peakTolerance':tolerance,'ambiguousCorrect':count,'overallCorrect':len(rows)-len(amb)+count,'overallAccuracy':(len(rows)-len(amb)+count)/len(rows)})
 checks=[]
 for name,flag in [('op_score_rank==1',np.array([rows[i]['rank']==1 for i in amb])),('op_score==10',np.array([rows[i]['fullScore']for i in amb]))]:
  for label,pred in [('UNSTOPPABLE',flag),('LEADER',~flag)]:
   count=int(np.sum(pred==target));checks.append({'condition':name,'conditionPredicts':label,'ambiguousCorrect':count,'overallAccuracy':(len(rows)-len(amb)+count)/len(rows)})
 failures=[{'matchId':rows[i]['matchId'],'participantId':rows[i]['participantId'],'actual':rows[i]['keyword'],'predicted':'UNSTOPPABLE'if best[2][j]else'LEADER'}for j,i in enumerate(amb)if best[2][j]!=target[j]]
 return {'naturalOwnPeakTolerances':natural,'requestedFactorChecks':checks,'disambiguationErrors':failures,'combinations':len(result),'participants':len(rows),'majorityCorrect':correct,'majorityAccuracy':correct/len(rows),'table':result,'disambiguationCandidates':candidates,'selectedDisambiguation':best[1],'target97Reached':best[1]['overallAccuracy']>=.97}

def fit_last(x,right,final,actual,default=0,baseline=5):
 poor=(right=='UP')&(final<baseline);excellent=(right=='DOWN')&(final>=baseline);forced=poor|excellent;order=np.argsort(x,kind='stable');sx=x[order];good=((actual=='GOOD')&~forced)[order].astype(int);fair=((actual=='FAIR')&~forced)[order].astype(int);cuts=np.r_[0,np.flatnonzero(np.diff(sx)>0)+1,len(x)];cg=np.r_[0,np.cumsum(good)];cf=np.r_[0,np.cumsum(fair)];quality=cf[cuts]+cg[-1]-cg[cuts];maximum=int(quality.max());candidates=cuts[quality==maximum];ts=np.array([sx[0]-1e-9 if i==0 else sx[-1]+1e-9 if i==len(x)else(sx[i-1]+sx[i])/2 for i in candidates]);threshold=float(ts[np.argmin(abs(ts-default))]);pred=np.where(x>=threshold,'GOOD','FAIR').astype('<U9');pred[poor]='POOR';pred[excellent]='EXCELLENT';return threshold,pred

def main():
 rows,sources=load();bank=features(rows);n=len(rows);candidates=[];selected={};predictions={};truth={side:np.array([r['analysis'][side]for r in rows])for side in ['left','right','last']}
 for side in ['left','right']:
  best=None
  for name,feature in bank.items():
   if side not in feature['sides']:continue
   threshold,pred,count=binary_fit(feature['values'],truth[side]=='UP',5 if feature['kind']=='level'else 0);labels=np.where(pred,'UP','DOWN');r={'target':side,'feature':name,'definition':feature['definition'],'threshold':threshold,'predictUP':'value>=threshold','correct':count,'participants':n,'accuracy':count/n};candidates.append(r)
   if best is None or count>best[0]:best=(count,r,labels,feature['values']-threshold)
  selected[side]=best[1];predictions[side]=best[2];selected[side]['naturalThreshold5Accuracy']=float(np.mean(np.where(bank[best[1]['feature']]['values']>=5,'UP','DOWN')==truth[side])) if bank[best[1]['feature']]['kind']=='level' else None;selected[side]['errorAnalysis']=errors(rows,truth[side].tolist(),best[2].tolist(),best[3])
 final=np.array([r['curve'][-1,1]for r in rows]);best=None
 for name,feature in bank.items():
  if 'last'not in feature['sides']:continue
  # Interpret final-minus-reference for level features, otherwise an explicit
  # slope/difference signal; right is the chosen CURVE prediction, not target.
  signal=final-feature['values']if feature['kind']=='level'else feature['values']
  threshold,pred=fit_last(signal,predictions['right'],final,truth['last']);assert set(pred)<= {'POOR','FAIR','GOOD','EXCELLENT'};count=int(np.sum(pred==truth['last']));r={'target':'last','feature':name,'definition':('final minus '+feature['definition'])if feature['kind']=='level'else feature['definition'],'threshold':threshold,'rule':'if predicted right=UP and final<5: POOR; if predicted right=DOWN and final>=5: EXCELLENT; else GOOD iff signal>=threshold, otherwise FAIR','rightDefinition':selected['right']['feature'],'correct':count,'participants':n,'accuracy':count/n,'usesOracleRight':False};candidates.append(r)
  if best is None or count>best[0]:best=(count,r,pred,signal-threshold)
 selected['last']=best[1];predictions['last']=best[2];selected['last']['errorAnalysis']=errors(rows,truth['last'].tolist(),best[2].tolist(),best[3])
 table=lookup(rows,bank);all95=all(s['accuracy']>=.95 for s in selected.values());lookup97=table['target97Reached'];report={'schemaVersion':1,'reproduction':{'script':'scripts/r217-study-opgg-rules.py','scriptSha256':hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),'python':sys.version.split()[0],'numpy':np.__version__},'generatedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'stage':'P1 only','scope':'R211 effective260 + R216 previous holdout300 explicitly reclassified as TRAINING by R217; no new holdout evaluated','sources':sources,'matches':560,'participants':n,'features':len(bank),'method':'Deterministic threshold search over explicit segmented endpoint/OLS/mean/median/extrema/time-weighted levels, polynomial formulas, own/team/global references; no black-box models','duplicateTimestampPolicy':'20 participant curves in 2 games have repeated terminal timestamp; keep both for point-based statistics; zero-duration last slope is 0; peer interpolation explicitly keeps last value per timestamp', 'dataQuality':{'duplicateTimestampParticipants':sum(bool(np.any(np.diff(r['curve'][:,0])==0))for r in rows),'duplicateTimestampMatches':len({r['matchId']for r in rows if np.any(np.diff(r['curve'][:,0])==0)})},'thresholdSelection':'highest training correct count; ties threshold nearest natural 5(level) or0(difference); left/right and last selected separately','warning':'All reported accuracies are fitted training agreement, not independent validation','lookup':table,'curveCandidates':candidates,'selectedDefinitions':selected,'gate':{'lookupAtLeast97':lookup97,'eachCurveAtLeast95':all95,'proceedToP2':lookup97 and all95,'failedTargets':[side for side,r in selected.items()if r['accuracy']<.95]},'productionChanged':False,'settlementScoreChanged':False,'r216EvaluationRerun':False}
 OUT.parent.mkdir(parents=True,exist_ok=True);OUT.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'lookup':table['selectedDisambiguation'],'selected':{k:{x:v[x]for x in ['feature','threshold','accuracy']}for k,v in selected.items()},'gate':report['gate']},ensure_ascii=False))
if __name__=='__main__':main()
