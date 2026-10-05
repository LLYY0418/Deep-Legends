#!/usr/bin/env python3
# R220：评估已完成，原始数据已删除，不可再运行。
"""CLAUDE ONLY: once-only R217 new holdout evaluation. Never run by GPT.
No tuning or model selection is performed here. Production Go returns all raw
candidate keywords; the existing display suppression stays unchanged until the
result is reviewed. Invoke explicitly with --operator Claude.
"""
import argparse,collections,hashlib,importlib.util,json,os,pathlib,subprocess,tempfile
from r217_data import ROOT,REPORT,old_exclusions
OUT=REPORT/'opgg-holdout';RESULT=REPORT/'holdout-keyword-evaluation.json';MARKER=REPORT/'holdout-keyword-evaluation.started.json'
EXPECTED_CANDIDATE='50bbd1866cee896945b4d57810dfc65b3c7300077c8e11980ff93ab98b90282a'
EXPECTED_RULES='877f999791c968f6052b49441c0d1fdbccbed604a8f3f16af75f7eb974c2fb5c'
EXPECTED_ACCOUNTS='810db8badfdc8f8729a6783ee6631124b486600dd76e8b7dc37a0c7474c9d0a1'
EXPECTED_MANIFEST='dacb2ba0c0eb837d09bead124bb3e0687f6b89feee2369f869f7432af344d146'

def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--operator',required=True,choices=['Claude']);parser.parse_args()
 if RESULT.exists()or MARKER.exists():raise SystemExit('Evaluation already run or started; refusing a second evaluation')
 assert '__'not in EXPECTED_MANIFEST,'P3 manifest has not been frozen'
 for path,expected in [(REPORT/'keyword-candidate.json',EXPECTED_CANDIDATE),(ROOT/'backend/testdata/r217/keyword-candidate.json',EXPECTED_CANDIDATE),(REPORT/'rule-freeze.json',EXPECTED_RULES),(REPORT/'holdout-accounts.json',EXPECTED_ACCOUNTS),(OUT/'manifest-sha256.json',EXPECTED_MANIFEST)]:assert digest(path)==expected,str(path)
 candidate=json.loads((REPORT/'keyword-candidate.json').read_text());assert candidate['openingRule']=={'precisionAtLeast':.5,'evaluationOwner':'Claude','evaluateOnce':True}
 assert digest(REPORT/'training-keyword-report.json')==candidate['trainingReportSHA256']
 freeze=json.loads((REPORT/'production-code-freeze.json').read_text())
 for f in freeze['files']:assert digest(ROOT/f['path'])==f['sha256'],f['path']
 manifest=json.loads((OUT/'manifest-sha256.json').read_text());files={f['path']for f in manifest['files']};assert files=={str(p.relative_to(OUT))for p in OUT.rglob('*')if p.is_file()and p.name!='manifest-sha256.json'}
 for f in manifest['files']:
  p=OUT/f['path'];assert p.stat().st_size==f['bytes']and digest(p)==f['sha256'],f['path']
 with MARKER.open('x')as f:json.dump({'operator':'Claude','manifestSHA256':EXPECTED_MANIFEST,'candidateSHA256':EXPECTED_CANDIDATE},f)
 accounts=json.loads((REPORT/'holdout-accounts.json').read_text());pairs=json.loads((OUT/'paired.json').read_text());old_ids,old_accounts=old_exclusions();selected={a['puuid']for a in accounts}
 assert len(accounts)==len(selected)==16 and not selected&old_accounts and collections.Counter(a['group']for a in accounts)=={'pro_high':8,'ordinary':8}
 assert all(a['recentSoloGames']>=20 for a in accounts)
 assert len(pairs)==len({p['match_id']for p in pairs})==200 and not {p['match_id']for p in pairs}&old_ids
 assert collections.Counter(p['group']for p in pairs)=={'pro_high':100,'ordinary':100}
 actual=collections.Counter();source=collections.Counter();fixtures=[]
 for pair in pairs:
  mid=pair['match_id'];raw=json.loads((OUT/(mid+'-match.json')).read_text());people=raw['info']['participants'];assert raw['info']['queueId']==420 and len(people)==10 and not{p['puuid']for p in people}&old_accounts
  owner=pair['account_index'];assert pair['group']==accounts[owner]['group']and accounts[owner]['puuid']in{p['puuid']for p in people}
  actual.update(p['puuid']for p in people if p['puuid']in selected);source[owner]+=1;fixtures.append({'matchId':mid,'matchPath':str(OUT/(mid+'-match.json')),'timelinePath':str(OUT/(mid+'-timeline.json'))})
 assert len(source)==16 and max(source.values())<=15 and max(actual.values())<=15
 # The marker has already reserved this run. Label/reference failures require
 # review, never an automatic retry of a partly observed holdout.
 spec=importlib.util.spec_from_file_location('ku',ROOT/'scripts/r217-keyword-utils.py');ku=importlib.util.module_from_spec(spec);spec.loader.exec_module(ku)
 rows=[]
 for pair in pairs:
  mid=pair['match_id'];people=json.loads((OUT/(mid+'-match.json')).read_text())['info']['participants'];index=collections.defaultdict(list)
  for op in pair['opgg']['team_blue']+pair['opgg']['team_red']:index[op['champion_id'],op['stats']['kill'],op['stats']['death'],op['stats']['assist']].append(op)
  for p in people:
   ops=index[p['championId'],p['kills'],p['deaths'],p['assists']];assert len(ops)==1,'Ambiguous participant pairing';s=ops[0]['stats'];assert s.get('keyword')in ku.OP,'Missing reference keyword must not be silently omitted';assert set(s['op_score_timeline_analysis'])=={'left','right','last'}
   rows.append({'matchId':mid,'participantId':p['participantId'],'group':pair['group'],'expected':ku.OP[s['keyword']],'analysis':s['op_score_timeline_analysis']})
 with tempfile.TemporaryDirectory(prefix='r217-once-eval-')as tmp:
  inp=pathlib.Path(tmp)/'input.json';out=pathlib.Path(tmp)/'output.json';inp.write_text(json.dumps({'matches':fixtures}))
  env={**os.environ,'R217_KEYWORD_DUMP_INPUT':str(inp),'R217_KEYWORD_DUMP_OUTPUT':str(out)};subprocess.run(['go','test','./backend','-run','^TestR217KeywordDump$','-count=1'],cwd=ROOT,env=env,check=True)
  batches=json.loads(out.read_text())['frozen'];assert len(batches)==200;flat=[p for batch in batches for p in batch];keys=[(p['matchId'],p['participantId'])for p in flat];assert len(keys)==len(set(keys)),'Duplicate Go participant prediction';expectedKeys={(r['matchId'],r['participantId'])for r in rows};assert set(keys)<=expectedKeys,'Unexpected Go prediction key';pred=dict(zip(keys,flat))
 missing=sum((r['matchId'],r['participantId'])not in pred for r in rows);predictions=[pred.get((r['matchId'],r['participantId']),{'rawKeyword':'','analysis':{'left':'','right':'','last':''}})for r in rows];groups={}
 for group in ['all','pro_high','ordinary']:
  indices=[i for i,r in enumerate(rows)if group=='all'or r['group']==group];groups[group]=ku.summarize([rows[i]for i in indices],[predictions[i]for i in indices]);groups[group]['matches']=len(indices)//10
 per=groups['all']['perKeyword'];opened=[key for key in ku.KEYS if per[key]['precision']is not None and per[key]['precision']>=.5];closed=[key for key in ku.KEYS if key not in opened]
 result={'operator':'Claude','manifestSHA256':EXPECTED_MANIFEST,'candidateSHA256':EXPECTED_CANDIDATE,'ruleFreezeSHA256':EXPECTED_RULES,'accountsSHA256':EXPECTED_ACCOUNTS,'metrics':groups,'missingGoParticipantPredictions':missing,'openingRule':'precision >= 50%; no predictions remain closed; no thresholds changed on this holdout','openKeywords':opened,'keepClosedKeywords':closed,'productionDisplayNotChanged':True}
 with RESULT.open('x')as f:json.dump(result,f,ensure_ascii=False,indent=2);f.write('\n')
 print('One-shot evaluation saved to',RESULT)
if __name__=='__main__':main()
