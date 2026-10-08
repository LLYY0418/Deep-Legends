#!/usr/bin/env python3
"""Fit keyword thresholds solely on the fixed R211 historical 260 matches."""
import hashlib,importlib.util,json,pathlib,sys
from local_evidence import evidence_path, require_evidence
ROOT=pathlib.Path(__file__).resolve().parent.parent
require_evidence('r211/opgg-samples/paired.json', 'r211/opgg-validation-new-accounts/paired.json')
spec=importlib.util.spec_from_file_location('keywords',ROOT/'scripts/r216-keyword-utils.py');k=importlib.util.module_from_spec(spec);spec.loader.exec_module(k)
output=ROOT/'backend/testdata/r216/keyword-thresholds.json'
if output.exists():raise SystemExit('Frozen keyword thresholds already exist; refusing overwrite')
curves=json.loads(pathlib.Path(sys.argv[1]).read_text());historical_ids={r['match_id']for r in curves};official={};op_curves=0;analysis=0
for name in ['opgg-samples','opgg-validation-new-accounts']:
 for pair in json.loads((evidence_path('r211')/name/'paired.json').read_text()):
  if pair['match_id']not in historical_ids:continue
  for person in pair['opgg']['team_blue']+pair['opgg']['team_red']:
   stats=person['stats'];official[pair['match_id'],person['participant_id']]=k.OP.get(stats.get('keyword'));op_curves+=bool(stats.get('op_score_timeline'));analysis+=bool(stats.get('op_score_timeline_analysis'))
rows=[{**r,'expected':official[r['match_id'],r['participantId']]}for r in curves if official.get((r['match_id'],r['participantId'])) and len(r['checkpoints'])>=3]
initial=dict(high=6.5,low=4.5,rise=2,delta=1.5,turn=.8,amplitude=3,weak=5,good=6,excellent=7,struggle=4.5)
assert all(k.predict(r,initial)==r['keyword']for r in rows),'Python calibration classifier differs from Go'
grid={'high':[5.5,6,6.5,7],'low':[3.5,4,4.5,5,5.5,6],'rise':[.2,.4,.6,.8,1,1.5,2], 'delta':[.1,.2,.3,.4,.6,.8,1,1.5], 'turn':[.1,.2,.3,.4,.6,.8], 'amplitude':[.5,1,1.5,2,2.5,3], 'weak':[4.5,5,5.5,6,6.5], 'good':[5,5.5,6,6.5,7], 'excellent':[5.5,6,6.5,7,7.5], 'struggle':[4,4.5,5,5.5,6]}
def objective(p):
 report=k.compare(rows,[k.predict(r,p)for r in rows]);return report['macroF1'],-sum((p[x]-initial[x])**2 for x in initial)
p=initial.copy();trace=[]
for iteration in range(8):
 before=p.copy()
 for key,values in grid.items():
  p=max(({**p,key:v}for v in values),key=objective)
 trace.append({'iteration':iteration+1,'params':p.copy(),'macroF1':objective(p)[0]})
 if before==p:break
output.write_text(json.dumps(p,indent=2)+'\n');sha=hashlib.sha256(output.read_bytes()).hexdigest()
report={'historicalMatches':260,'opTimelinePresent':op_curves,'opTimelineAnalysisPresent':analysis,'criterion':'macro F1 over 14 official OP.GG keyword labels; ties nearest original thresholds','consistencyDefinition':'precision = correct predictions / predictions; recall and confusion also reported','scope':'historical R211 only; no R216 holdout read','initial':k.compare(rows,[k.predict(r,initial)for r in rows]),'calibrated':k.compare(rows,[k.predict(r,p)for r in rows]),'params':p,'sha256':sha,'trace':trace}
evidence_path('r216').mkdir(parents=True,exist_ok=True)
(evidence_path('r216/keyword-calibration.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'participants':len(rows),'initial':report['initial']['accuracy'],'calibrated':report['calibrated']['accuracy'],'sha256':sha}))
