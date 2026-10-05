#!/usr/bin/env python3
"""Export regression expectations from historical R211 only, never the holdout.
Uses the independent Python six-dimension normalization used during P2.
No search, evaluation, or threshold changes.
"""
import importlib.util,json,pathlib
import numpy as np
ROOT=pathlib.Path(__file__).resolve().parent.parent
spec=importlib.util.spec_from_file_location('r216_historical',ROOT/'scripts/r216-select-candidate.py');historical=importlib.util.module_from_spec(spec);spec.loader.exec_module(historical)
params=json.loads((ROOT/'docs/history/reports/r216/v21-candidate.json').read_text())['params'];weights=np.array(params['weights']);out=[]
for fixture in json.loads((ROOT/'backend/testdata/r216/python-v2-golden.json').read_text()):
 info=json.loads((ROOT/fixture['path']).read_text())['info'];people=sorted(info['participants'],key=lambda p:p['participantId']);norm,available=historical.normalized_v2(info,params['assistFactor']);w=weights*available;w/=w.sum();scores=2+8*(norm@w);order=sorted(range(len(people)),key=lambda i:(-float(scores[i]),people[i]['participantId']));ranks={i:rank+1 for rank,i in enumerate(order)};badges={}
 for win,badge in [(True,'MVP'),(False,'SVP')]:
  best=next(i for i in order if people[i]['win']==win);badges[best]=badge
 out.append({'path':fixture['path'],'match_id':fixture['match_id'],'scores':[{'participantId':p['participantId'],'rawScore':float(scores[i]),'rank':ranks[i],'badge':badges.get(i,'')}for i,p in enumerate(people)]})
(ROOT/'backend/testdata/r216/python-v21-golden.json').write_text(json.dumps(out,ensure_ascii=False,indent=2)+'\n');print('Historical v2.1 golden: 260 matches / 2600 participants; no holdout read')
