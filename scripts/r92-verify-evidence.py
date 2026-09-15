"""Assert the ticket's real-network gates against curated observations."""
import json,pathlib,statistics,sys
kind=sys.argv[1]
rows=json.loads(pathlib.Path(sys.argv[2]).read_text())
if kind=='images':
 cold,restart=rows
 valid=all(r['status']==200 for x in rows for r in x['rows']) and cold['count']==restart['count']==39 and cold['bytes']==restart['bytes']
 ratio=restart['wall_ms']/cold['wall_ms'];passed=valid and restart['wall_ms']<150 and ratio<=.25
 result={'valid':valid,'restart_ms':restart['wall_ms'],'restart_to_cold':ratio,'passed':passed}
elif kind=='riot':
 # R93: latency remains an observation; disk correctness has no RTT threshold.
 row=rows[-1];cost=row['cost'];passed=row['status']==200 and row['matches']==20 and cost.get('matches_failed',0)==0 and cost['matches_from_disk']==20
 result={'matches_from_disk':cost['matches_from_disk'],'duration_ms':cost['duration_ms'],'acceptance':'20-complete-disk-matches','passed':passed}
elif kind=='concurrency':
 four=[r for r in rows if r['concurrency']==4]
 eight=json.loads(pathlib.Path(sys.argv[3]).read_text())
 valid=len(four)>=5 and len(eight)>=5 and all(r['status']==200 and r['matches']==20 and r['cost']['matches_from_disk']==0 and r['cost']['details_inflight_peak']==r['concurrency'] for r in four+eight)
 old=statistics.median(r['cost']['duration_ms'] for r in four);new=statistics.median(r['cost']['duration_ms'] for r in eight)
 reduction=(1-new/old)*100
 result={'valid':valid,'four_median_ms':old,'default_eight_median_ms':new,'reduction_percent':reduction,'passed':valid and reduction>=25}
else:raise SystemExit('use images, riot, or concurrency')
print(json.dumps(result,indent=2))
raise SystemExit(0 if result['passed'] else 1)
