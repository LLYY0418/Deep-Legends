"""Historical-only keyword calibration helpers; production predictions come from Go."""
import collections,math
KEYS=['unstoppable','leader','victorious','latebloomer','resilience','dedication','average','rollercoaster','decline','innocent','unlucky','slowstarter','unyielding','struggling']
OP=dict(zip(['UNSTOPPABLE','LEADER','VICTOR','LATE_BLOOMER','RESILIENT','DEVOTED','AVERAGE','ROLLERCOASTER','DOWNFALL','INNOCENT','UNLUCKY','SLOW_STARTER','UNYIELDING','STRUGGLE'],KEYS))
def predict(row,p):
 s=row['checkpoints'];n=len(s)
 if n<3:return ''
 third=max(1,n//3);early=sum(s[:third])/third;late=sum(s[-third:])/third;delta=late-early;low=min(s[1:-1]);final=s[-1];direction=0;turns=0
 for a,b in zip(s,s[1:]):
  if abs(b-a)<p['turn']:continue
  new=1 if b>a else -1
  turns+=bool(direction and direction!=new);direction=new
 if row['win']:
  checks=[(row['badge']=='MVP' and min(s)>=p['high'],'unstoppable'),(low<=p['low'] and final-low>=p['rise'],'resilience'),(delta>=p['delta'],'latebloomer'),(early>=p['high'] and delta>-p['delta'],'victorious'),(row['badge']=='MVP','leader'),(delta<=-p['delta'],'decline'),(turns>=2 and max(s)-min(s)>=p['amplitude'],'rollercoaster'),(final<p['weak'],'dedication')]
 else:
  checks=[(row['badge']=='SVP' and final>=p['excellent'],'innocent'),(low<=p['low'] and final-low>=p['rise'],'unyielding'),(delta>=p['delta'],'slowstarter'),(final>=p['good'],'unlucky'),(delta<=-p['delta'],'decline'),(turns>=2 and max(s)-min(s)>=p['amplitude'],'rollercoaster'),(final<p['struggle'] and late<p['struggle'],'struggling')]
 return next((k for yes,k in checks if yes),'average')
def compare(rows,predictions):
 confusion=collections.Counter((r['expected'],v)for r,v in zip(rows,predictions));metrics={}
 for key in KEYS:
  tp=confusion[key,key];pred=sum(n for (a,b),n in confusion.items()if b==key);actual=sum(n for(a,b),n in confusion.items()if a==key)
  metrics[key]={'correct':tp,'predicted':pred,'actual':actual,'precision':tp/pred if pred else None,'recall':tp/actual if actual else None,'f1':2*tp/(pred+actual)if pred+actual else 0}
 return {'participants':len(rows),'accuracy':sum(confusion[k,k]for k in KEYS)/len(rows),'macroF1':sum(v['f1']for v in metrics.values())/len(KEYS),'perKeyword':metrics,'confusion':[{'actual':a,'predicted':b,'count':n}for(a,b),n in sorted(confusion.items())]}
