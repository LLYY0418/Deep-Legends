#!/usr/bin/env python3
# R220：评估已完成，原始数据已删除，不可再运行。
"""CLAUDE ONLY: one-shot evaluation. Never tune candidates using this dataset.
Computes scores AND timeline keyword predictions with production Go functions.
"""
import collections,hashlib,importlib.util,json,math,os,pathlib,statistics,subprocess,tempfile
from r216_data import ROOT,REPORT,old_exclusions
OUT=REPORT/'opgg-holdout';RESULT=REPORT/'holdout-evaluation.json';MARKER=REPORT/'holdout-evaluation.started.json'
EXPECTED_CANDIDATE='44b9a6d7c4c71b7800aefe9b81bbc2ad6b16bbdeee9eca5cc2ef95a7e81a51f7'
EXPECTED_ACCOUNTS='30baeeaa8fec9d3e0a2aceb712df9e62538d519e0ddfedce2237747f495aa9c5'
EXPECTED_KEYWORDS='98ce919664c776bac3565b96a100a9a782eee0182c478123458f9de03b688d82'
EXPECTED_MANIFEST='232ead1a63d388571c3152c28e1b50f7f7862e200eb6abf829a84aec59c50f71'
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def ranks(values):
 order=sorted(range(len(values)),key=lambda i:values[i]);out=[0.]*len(values);at=0
 while at<len(order):
  end=at+1
  while end<len(order)and values[order[end]]==values[order[at]]:end+=1
  for i in order[at:end]:out[i]=(at+end+1)/2
  at=end
 return out
def rho(a,b):
 x=ranks(a);y=ranks(b);mx=statistics.mean(x);my=statistics.mean(y);den=math.sqrt(sum((v-mx)**2 for v in x)*sum((v-my)**2 for v in y))
 return sum((u-mx)*(v-my)for u,v in zip(x,y))/den if den else None
def main():
 if RESULT.exists()or MARKER.exists():raise SystemExit('Evaluation already run or started; refusing a second evaluation')
 assert digest(ROOT/'backend/testdata/r216/keyword-thresholds.json')==EXPECTED_KEYWORDS
 assert digest(REPORT/'v21-candidate.json')==EXPECTED_CANDIDATE
 assert digest(REPORT/'holdout-accounts.json')==EXPECTED_ACCOUNTS
 assert digest(OUT/'manifest-sha256.json')==EXPECTED_MANIFEST
 manifest=json.loads((OUT/'manifest-sha256.json').read_text());files={p['path']for p in manifest['files']}
 assert files=={str(p.relative_to(OUT))for p in OUT.rglob('*')if p.is_file()and p.name!='manifest-sha256.json'}
 for f in manifest['files']:
  path=OUT/f['path'];assert path.stat().st_size==f['bytes']and digest(path)==f['sha256'],f['path']
 candidate=json.loads((REPORT/'v21-candidate.json').read_text());accounts=json.loads((REPORT/'holdout-accounts.json').read_text());pairs=json.loads((OUT/'paired.json').read_text());old_ids,old_accounts=old_exclusions();assert len(pairs)==300 and len({p['match_id']for p in pairs})==300
 selected={a['puuid']for a in accounts};assert not selected&old_accounts
 matches=[];timelines=[];actual=collections.Counter();sources=collections.Counter();official=[]
 for pair in pairs:
  mid=pair['match_id'];assert mid not in old_ids
  raw=json.loads((OUT/(mid+'-match.json')).read_text());people=raw['info']['participants'];assert raw['info']['queueId']==420 and len(people)==10 and not{p['puuid']for p in people}&old_accounts
  actual.update(p['puuid']for p in people if p['puuid']in selected);sources[pair['account_index']]+=1
  ops=pair['opgg']['team_blue']+pair['opgg']['team_red'];index=collections.defaultdict(list)
  for p in ops:index[p['champion_id'],p['stats']['kill'],p['stats']['death'],p['stats']['assist']].append(p)
  aligned={}
  for p in people:
   rows=index[p['championId'],p['kills'],p['deaths'],p['assists']];assert len(rows)==1,'Ambiguous participant pairing'
   aligned[str(p['participantId'])]=rows[0]
  official.append(aligned);matches.append(raw);timelines.append(json.loads((OUT/(mid+'-timeline.json')).read_text()))
 assert max(actual.values())<=20 and max(sources.values())<=20 and len(sources)==len(accounts)
 assert collections.Counter(p['group']for p in pairs)=={'pro_high':150,'ordinary':150}
 # Exclusive marker before any scoring. A failed run requires explicit review,
 # never silently rerun a partly observed holdout.
 with MARKER.open('x')as f:json.dump({'manifestSha256':EXPECTED_MANIFEST,'candidateSha256':EXPECTED_CANDIDATE},f)
 predictions={};tag_predictions={}
 with tempfile.TemporaryDirectory(prefix='r216-eval-')as tmp:
  tmp=pathlib.Path(tmp)
  for model,params in [('v2',candidate['baseline']),('v21',candidate['params'])]:
   inp=tmp/(model+'-input.json');score=tmp/(model+'-scores.json');tags=tmp/(model+'-tags.json');inp.write_text(json.dumps({'params':params,'matches':matches,'timelines':timelines,'keywordParams':json.loads((ROOT/'backend/testdata/r216/keyword-thresholds.json').read_text())}))
   env={**os.environ,'R216_SCORE_DUMP_INPUT':str(inp),'R216_SCORE_DUMP_OUTPUT':str(score),'R216_TAG_DUMP_OUTPUT':str(tags)}
   subprocess.run(['go','test','./backend','-run','^TestR216ScoreDump$','-count=1'],cwd=ROOT,env=env,check=True)
   predictions[model]=json.loads(score.read_text());tag_predictions[model]=json.loads(tags.read_text());assert len(predictions[model])==len(tag_predictions[model])==len(pairs)
 spec=importlib.util.spec_from_file_location('keywords',ROOT/'scripts/r216-keyword-utils.py');k=importlib.util.module_from_spec(spec);spec.loader.exec_module(k)
 per_model={};outcomes={}
 for model,scores in predictions.items():
  outcomes[model]=[];keywords=[];kwpred=[]
  for i,(pair,rows)in enumerate(zip(pairs,scores)):
   people=matches[i]['info']['participants'];mvp=svp=False
   for p in people:
    pid=str(p['participantId']);score=rows[pid];correct=bool(official[i][pid]['stats']['is_opscore_max_in_team'])
    if score['badge']=='MVP':mvp=correct
    if score['badge']=='SVP':svp=correct
   correlation=rho([rows[str(p['participantId'])]['rawScore']for p in people],[-official[i][str(p['participantId'])]['stats']['op_score_rank']for p in people])
   outcomes[model].append({'mvp':mvp,'svp':svp,'rho':correlation,'group':pair['group']})
   for tag in tag_predictions[model][i]:
    expected=k.OP.get(official[i][str(tag['participantId'])]['stats'].get('keyword'))
    if expected and len(tag['checkpoints'])>=3:keywords.append({'expected':expected});kwpred.append(tag.get('keyword',''))
  per_model[model]={}
  for group in ['all','pro_high','ordinary']:
   rows=[r for r in outcomes[model]if group=='all'or r['group']==group];valid=[r['rho']for r in rows if r['rho']is not None]
   per_model[model][group]={'matches':len(rows),'MVP':sum(r['mvp']for r in rows)/len(rows),'SVP':sum(r['svp']for r in rows)/len(rows),'rhoMedian':statistics.median(valid),'rhoSamples':len(valid)}
  per_model[model]['keywords']=k.compare(keywords,kwpred)
  per_model[model]['keywords']['disableBelow40Percent']=[key for key,v in per_model[model]['keywords']['perKeyword'].items()if v['precision']is not None and v['precision']<.4]
 paired={}
 for badge in ['mvp','svp']:
  counts=collections.Counter((a[badge],b[badge])for a,b in zip(outcomes['v2'],outcomes['v21']));paired[badge]={'bothCorrect':counts[True,True],'onlyV2Correct':counts[True,False],'onlyV21Correct':counts[False,True],'bothWrong':counts[False,False]}
 v2=per_model['v2'];v21=per_model['v21'];rules={'MVP_not_lower':v21['all']['MVP']>=v2['all']['MVP'],'SVP_plus_2pp':v21['all']['SVP']-v2['all']['SVP']>=.02-1e-12,'rho_decline_at_most_001':v21['all']['rhoMedian']>=v2['all']['rhoMedian']-.01-1e-12,'ordinary_MVP_decline_at_most_2pp':v21['ordinary']['MVP']>=v2['ordinary']['MVP']-.02-1e-12}
 result={'manifestSha256':EXPECTED_MANIFEST,'candidateSha256':EXPECTED_CANDIDATE,'keywordParamsSha256':EXPECTED_KEYWORDS,'metrics':per_model,'paired':paired,'adoptionRules':rules,'adoptV21':all(rules.values()),'keywordConsistencyDefinition':'precision = correct / predictions; disable individual keyword below 40%; do not tune thresholds on this holdout'}
 with RESULT.open('x')as f:json.dump(result,f,ensure_ascii=False,indent=2);f.write('\n')
 print('Evaluation saved to',RESULT)
if __name__=='__main__':main()
