#!/usr/bin/env python3
"""R217 mode interval probe using public OP.GG pages only, without Riot IDs/Key."""
import collections, datetime, json, pathlib, re, urllib.request
ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / 'docs/history/reports/r217/mode-interval-probe'
ACTION = '409a2b9ca50d15e50a4dace93552e3a40113dc2753'

def read(url, path, data=None, headers=None):
    if path.exists(): return path.read_bytes()
    request = urllib.request.Request(url, data=data, headers={'User-Agent': 'Mozilla/5.0', **(headers or {})})
    with urllib.request.urlopen(request, timeout=40) as response: raw=response.read()
    path.write_bytes(raw)
    return raw

def main():
    evidence=[];probes=[]
    for index,url in enumerate(json.loads((OUT/'public-source-pages.json').read_text())):
        assert url.startswith('https://op.gg/zh-cn/lol/summoners/kr/')
        html=read(url,OUT/f'public-page-{index}.html').decode().replace('\\"','"')
        # This is OP.GG's own opaque ID, published on that same public page.
        # No Riot PUUID, local account record or API credential is accessed.
        match=re.search(r'"puuid":"([^"]+)"',html)
        if not match: raise SystemExit('OP.GG public identity unavailable')
        for mode in ['ARAM','ARAM_MAYHEM','ARAM_MAYHEM_CLASSIC']:
            args=[{'locale':'zh-cn','region':'kr','puuid':match.group(1),'gameType':mode,'endedAt':'','champion':''}]
            path=OUT/f'public-page-{index}-{mode}.flight'
            raw=read(url,path,json.dumps(args).encode(),{'Content-Type':'text/plain;charset=UTF-8','Next-Action':ACTION})
            rows=[]
            for line in raw.decode().splitlines():
                if line.startswith('1:'): rows=json.loads(line[2:]).get('data',[])
            probes.append({'sourcePage':url,'requestedMode':mode,'records':len(rows)})
            for game in rows:
                people=game.get('team_blue',[])+game.get('team_red',[])
                curves=[p.get('stats',{}).get('op_score_timeline',[]) for p in people]
                valid=[c for c in curves if len(c)>=3]
                if not valid: continue
                diffs=collections.Counter(b['second']-a['second']for c in valid for a,b in zip(c,c[1:])if b['second']>a['second'])
                evidence.append({'sourcePage':url,'requestedMode':mode,'actualGameType':game.get('game_type'),'opggGameId':game.get('id'),'participantsWithCurve':len(valid),'positiveSecondDifferences':dict(diffs),'sampleSeconds':[p['second']for p in valid[0]],'sourceFile':str(path.relative_to(ROOT))})
        print(json.dumps({'page':index,'gamesWithCurves':len(evidence)}),flush=True)
        if {e['requestedMode']for e in evidence}>={'ARAM','ARAM_MAYHEM'}:break
    report={'generatedAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'publicPagesOnly':True,'riotCredentialsAccessed':False,'probes':probes,'evidence':evidence}
    (OUT/'summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'gamesWithCurves':len(evidence)}))
if __name__=='__main__':main()
