"""Read-only official R75 capture, bounded to four concurrent public requests.
Usage: python3 scripts/r75-live-probe.py /tmp/r75-probe
No account data, Cookie, Riot key or local credentials are used.
"""
import concurrent.futures as cf
import datetime as dt
import json
import pathlib
import subprocess
import sys
import time

ROOT = pathlib.Path(sys.argv[1]); ROOT.mkdir(parents=True, exist_ok=True)
API = 'https://esports-api.lolesports.com/persisted/gw/'
FEED = 'https://feed.lolesports.com/livestats/v1/'
KEY = '0TvQnueqKa5mxJntVWt0w4LpLfEkrV1Ta8rQBb9Z'
metrics = []
def get(path, name, feed=False):
    file = ROOT / (name + '.json')
    if file.exists():
        try: return json.loads(file.read_text())
        except Exception: pass
    start = time.monotonic()
    args = ['curl', '-fsS', '--max-time', '8', (FEED if feed else API) + path]
    if not feed: args += ['-H', 'x-api-key: ' + KEY]
    for attempt in range(3):
        p = subprocess.run(args, capture_output=True)
        if p.returncode == 0:
            try:
                result = json.loads(p.stdout)
                file.write_bytes(p.stdout)
                metrics.append({'name':name, 'ms':round((time.monotonic()-start)*1000), 'url':args[4], 'headers':['x-api-key'] if not feed else []})
                return result
            except Exception: pass
    print('FAILED', name, p.stderr.decode()[:150], flush=True)
    return None

def parallel(fn, values):
    with cf.ThreadPoolExecutor(max_workers=4) as pool: return list(pool.map(fn, values))

leagues = get('getLeagues?hl=zh-CN','leagues')['data']['leagues']
ids = [l['id'] for l in leagues if l['slug'] in ['lpl','lck','first_stand','msi','worlds']]
assert len(ids)==5
teams = get('getTeams?hl=zh-CN&id=bilibili-gaming,invictus-gaming,t1,hanwha-life-esports,geng,dwg-kia,dwg-kia-challengers','teams')['data']['teams']
teamids = {t['id'] for t in teams if t['slug']!='dwg-kia-challengers'}
now=dt.datetime.now(dt.timezone.utc); cutoff=now-dt.timedelta(days=30)
schedule = get('getSchedule?hl=zh-CN&leagueId='+','.join(ids),'schedule')['data']['schedule']
events = schedule['events']
# A series may start before cutoff while later games are inside it.
while events and min(dt.datetime.fromisoformat(e['startTime']) for e in events)>cutoff-dt.timedelta(days=1) and schedule['pages'].get('older'):
    import urllib.parse
    token = schedule['pages']['older']
    schedule = get('getSchedule?hl=zh-CN&leagueId='+','.join(ids)+'&pageToken='+urllib.parse.quote(token), 'schedule-'+str(len(events)))['data']['schedule']
    events += schedule['events']
selected=[e for e in events if e['state']=='completed' and dt.datetime.fromisoformat(e['startTime'])>=cutoff-dt.timedelta(days=1)]
raw=parallel(lambda e:get('getEventDetails?hl=zh-CN&id='+e['match']['id'],'event-'+e['match']['id']), selected)
matches=[r['data']['event'] for r in raw if r and r['data']['event']['league']['id'] in ids and any(t['id'] in teamids for t in r['data']['event']['match']['teams'])]
print('MATCHES',len(matches),flush=True)
games=[g for m in matches for g in m['match']['games'] if g['state']=='completed']
windows=parallel(lambda g:get('window/'+g['id'],'window-'+g['id'],True),games)
def terminal(pair):
    g,w=pair
    if not w or not w.get('frames'): return None
    start=dt.datetime.fromisoformat(w['frames'][0]['rfc460Timestamp'])
    aligned=(start+dt.timedelta(hours=3)).replace(microsecond=0); aligned-=dt.timedelta(seconds=aligned.second%10)
    return get('window/'+g['id']+'?startingTime='+aligned.isoformat().replace('+00:00','Z'),'end-'+g['id'],True)
ends=parallel(terminal,zip(games,windows))
def details(pair):
    g,w=pair
    if not w or not w.get('frames'):return
    stamp=dt.datetime.fromisoformat(w['frames'][-1]['rfc460Timestamp']);stamp=stamp.replace(microsecond=0)-dt.timedelta(seconds=stamp.second%10)
    return get('details/'+g['id']+'?startingTime='+stamp.isoformat().replace('+00:00','Z'),'details-'+g['id'],True)
# Captured full details support repeatable real-data regression and latency samples.
parallel(details,zip(games,ends))
(ROOT/'probe-metrics.json').write_text(json.dumps({'now':now.isoformat(),'matches':len(matches),'games':len(games),'requests':metrics},indent=2))
print('DONE',len(games), 'games',len(metrics),'requests',flush=True)
