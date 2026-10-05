"""Fixed R217 rule shape shared by training diagnostics and Claude's evaluator.
Production predictions are always exported by the Go bridge.
"""
import collections,json,math,pathlib,statistics
ROOT=pathlib.Path(__file__).resolve().parent.parent
REPORT=ROOT/'docs/history/reports/r217'
KEYS=['unstoppable','leader','victorious','latebloomer','resilience','dedication','average','rollercoaster','decline','innocent','unlucky','slowstarter','unyielding','struggling']
OP=dict(zip(['UNSTOPPABLE','LEADER','VICTOR','LATE_BLOOMER','RESILIENT','DEVOTED','AVERAGE','ROLLERCOASTER','DOWNFALL','INNOCENT','UNLUCKY','SLOW_STARTER','UNYIELDING','STRUGGLE'],KEYS))
def analyze(scores,p):
 if len(scores)<2 or not all(math.isfinite(x)and x>=0 for x in scores):return {'left':'','right':'','last':''}
 k=len(scores)//2;left='UP'if statistics.median(scores[:k])>=p['leftThreshold']else'DOWN';right='UP'if statistics.median(scores[k:])>=p['rightThreshold']else'DOWN';final=scores[-1]
 if right=='UP'and final<p['endingBaseline']:last='POOR'
 elif right=='DOWN'and final>=p['endingBaseline']:last='EXCELLENT'
 else:last='GOOD'if final-max(scores[max(0,len(scores)-1-p['endingWindow']):-1])>=p['endingGap']else'FAIR'
 return {'left':left,'right':right,'last':last}
def predict(scores,win,teammax,model):
 a=analyze(scores,model['rules']);index={(r['win'],r['teamMax'],r['left'],r['right'],r['last']):r['keyword']for r in model['table']};key=index.get((win,teammax,a['left'],a['right'],a['last']),'')
 if key and win and a=={'left':'UP','right':'UP','last':'GOOD'}:key='unstoppable'if scores[-1]-max(scores)>=model['rules']['peakGap']else'leader'
 return key,a
def metrics(expected,predicted):
 c=collections.Counter(zip(expected,predicted));per={}
 for key in KEYS:
  tp=c[key,key];actual=sum(v for(a,b),v in c.items()if a==key);p=sum(v for(a,b),v in c.items()if b==key)
  per[key]={'actual':actual,'predicted':p,'correct':tp,'precision':tp/p if p else None,'recall':tp/actual if actual else None,'f1':2*tp/(actual+p)if actual+p else 0}
 return {'participants':len(expected),'accuracy':sum(c[k,k]for k in KEYS)/len(expected),'macroF1':sum(r['f1']for r in per.values())/len(KEYS),'perKeyword':per,'confusion':[{'actual':a,'predicted':b,'count':v}for(a,b),v in sorted(c.items())]}
def summarize(rows,tags):
 assert len(rows)==len(tags);out=metrics([r['expected']for r in rows],[r['rawKeyword']for r in tags]);out['analysisAgreement']={side:sum(r['analysis'][side]==p['analysis'][side]for r,p in zip(rows,tags))/len(rows)for side in ['left','right','last']};return out
