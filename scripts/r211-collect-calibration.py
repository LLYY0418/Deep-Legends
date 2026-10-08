#!/usr/bin/env python3
"""R211 read-only samples. Run locally; credentials never enter output/files.
python3 scripts/r211-collect-calibration.py --target 150
Uses riot_key.local.txt or RIOT_API_KEY, otherwise the built-in relay. Resumes
from original response files, serializes Riot reads at <=100 per 130 seconds,
stops on shared quota/IP/application cooling, and never refreshes OP.GG players.
"""
import argparse,datetime,json,os,pathlib,re,time,urllib.request,urllib.error,urllib.parse
from local_evidence import evidence_path, require_evidence
ROOT=pathlib.Path(__file__).resolve().parent.parent
OUT=evidence_path('r211/opgg-samples')
UA='Mozilla/5.0'
ACTION='409a2b9ca50d15e50a4dace93552e3a40113dc2753'
ACCOUNTS=[('TOPKING','asd'),('JUGKlNG','kr'),('Hide on bush','KR1'),('DK ShowMaker','KR1'),('허거덩','0303'),('kiin','KR1'),('오 너','111'),('Athene','lll'),('suis','kr7'),('DK Lucid','KR1')]
class ProbeBlocked(Exception): pass
class Client:
 def __init__(self):
  self.key=os.getenv('RIOT_API_KEY','').strip()
  p=ROOT/'riot_key.local.txt'
  if not self.key and p.exists():self.key=p.read_text().strip()
  self.last=0
 def read(self,url,file,data=None,headers=None,riot=False):
  if file.exists():return file.read_bytes()
  hs={'User-Agent':UA,**(headers or {})}
  if riot:
   time.sleep(max(0,1.3-(time.monotonic()-self.last)));self.last=time.monotonic()
   if self.key:hs['X-Riot-Token']=self.key
  try:
   with urllib.request.urlopen(urllib.request.Request(url,data=data,headers=hs),timeout=40) as r:raw=r.read()
  except urllib.error.HTTPError as e:
   body=e.read(8192).decode(errors='replace')
   safe={k:v for k,v in e.headers.items() if k.lower() not in ['authorization','x-riot-token','set-cookie']}
   if self.key:body=body.replace(self.key,'[REDACTED]')
   (OUT/'last-network-error.json').write_text(json.dumps({'status':e.code,'headers':safe,'body':body},ensure_ascii=False,indent=2))
   raise ProbeBlocked('HTTP '+str(e.code)+'; sanitized response saved') from None
  except (urllib.error.URLError,TimeoutError) as e:raise ProbeBlocked(type(e).__name__+'; network unavailable') from None
  file.parent.mkdir(parents=True,exist_ok=True);file.write_bytes(raw);return raw
 def riot(self,cluster,path,file,query=None):
  base=('https://'+cluster+'.api.riotgames.com') if self.key else 'https://riot.yinxiaobia.net/r/'+cluster
  url=base+path+('?' + urllib.parse.urlencode(query) if query else '')
  return json.loads(self.read(url,file,riot=True))
 def games(self,name,tag,index,page,ended=''):
  url='https://op.gg/zh-cn/lol/summoners/kr/'+urllib.parse.quote(name+'-'+tag)
  raw=self.read(url,OUT/f'account-{index}-page.html')
  html=raw.decode().replace('\\"','"')
  match=re.search(r'"puuid":"([^"]+)"',html)
  if not match:raise ProbeBlocked('OP.GG page identity missing for account '+str(index))
  args=[{'locale':'zh-cn','region':'kr','puuid':match.group(1),'gameType':'SOLORANKED','endedAt':ended,'champion':''}]
  raw=self.read(url,OUT/f'account-{index}-games-{page}.flight',json.dumps(args).encode(),{'Content-Type':'text/plain;charset=UTF-8','Next-Action':ACTION})
  for line in raw.decode().splitlines():
   if line.startswith('1:'):return json.loads(line[2:]).get('data',[])
  raise ProbeBlocked('OP.GG action changed')
def fingerprint_op(game):
 return sorted((p['champion_id'],p['stats']['kill'],p['stats']['death'],p['stats']['assist']) for p in game['team_blue']+game['team_red'])
def fingerprint_riot(game):
 return sorted((p['championId'],p['kills'],p['deaths'],p['assists']) for p in game['info']['participants'])
def main():
 global OUT,ACCOUNTS
 args=argparse.ArgumentParser();args.add_argument('--target',type=int,default=150);args.add_argument('--pages',type=int,default=2)
 args.add_argument('--fresh-accounts',type=pathlib.Path,help='JSON pairs of new account names/tags; separate, unscored validation dataset')
 opts=args.parse_args();excluded_matches=set();excluded_accounts=set()
 if opts.fresh_accounts:
  original=OUT
  excluded_matches={x['match_id'] for x in json.loads((original/'paired.json').read_text())}
  excluded_accounts={json.loads(file.read_text())['puuid'] for file in original.glob('account-*-riot.json')}
  ACCOUNTS=[tuple(row) for row in json.loads(opts.fresh_accounts.read_text())]
  OUT=OUT.parent/'opgg-validation-new-accounts'
 OUT.mkdir(parents=True,exist_ok=True);client=Client();paired={};included=set();keywords={}
 for index,(name,tag) in enumerate(ACCOUNTS):
  games=[];ended=''
  for page in range(opts.pages):
   rows=client.games(name,tag,index,page,ended)
   games.extend(rows)
   if not rows:break
   ended=rows[-1]['created_at']
  games=[g for g in games if len(g.get('team_blue',[]))+len(g.get('team_red',[]))==10 and all(p['stats'].get('op_score') is not None for p in g['team_blue']+g['team_red'])]
  for g in games:
   kw=g.get('stats',{}).get('keyword')
   if isinstance(kw,dict):keywords[kw['keyword']]=kw
  print(json.dumps({'stage':'opgg','account_index':index,'rows':len(games)},ensure_ascii=False),flush=True)
  if not games:continue
  fp={}
  for game in games:
   fingerprint=tuple(fingerprint_op(game))
   if fingerprint in fp:raise ProbeBlocked('Ambiguous OP.GG game fingerprint; manual verification required')
   fp[fingerprint]=game
  account=client.riot('asia','/riot/account/v1/accounts/by-riot-id/'+urllib.parse.quote(name)+'/'+urllib.parse.quote(tag),OUT/f'account-{index}-riot.json')
  if account['puuid'] in excluded_accounts:raise ProbeBlocked('Validation account overlaps original Riot PUUID; refused')
  timestamps=[datetime.datetime.fromisoformat(g['created_at']).timestamp() for g in games]
  # Query only the OP.GG sample interval, padded for end-time versus start-time.
  ids=client.riot('asia','/lol/match/v5/matches/by-puuid/'+urllib.parse.quote(account['puuid'])+'/ids',OUT/f'account-{index}-ids.json',{'queue':420,'start':0,'count':100,'startTime':int(min(timestamps))-7200,'endTime':int(max(timestamps))+7200})
  for id in ids:
   if id in excluded_matches:continue
   match=client.riot('asia','/lol/match/v5/matches/'+id,OUT/(id+'-match.json'))
   if not id.startswith('KR_') or match['info'].get('queueId')!=420:continue
   # Also reject matches involving any of the ten original source accounts.
   if any(p.get('puuid') in excluded_accounts for p in match['info']['participants']):continue
   game=fp.get(tuple(fingerprint_riot(match)))
   if not game:continue
   if len(set(p['stats']['op_score'] for p in game['team_blue']+game['team_red']))<2:continue
   paired[id]={'match_id':id,'opgg':game,'account_indices':sorted(set(paired.get(id,{}).get('account_indices',[])+[index]))}
   included.add(index)
   client.riot('asia','/lol/match/v5/matches/'+id+'/timeline',OUT/(id+'-timeline.json'))
   # Include at least one paired match from every account even after the target.
   if len(paired)>=opts.target and index in included:break
  (OUT/'paired.json').write_text(json.dumps(list(paired.values()),ensure_ascii=False))
  (OUT/'keyword-labels.json').write_text(json.dumps(keywords,ensure_ascii=False,indent=2))
  print(json.dumps({'stage':'paired','accounts':len(included),'unique_matches':len(paired),'account_index':index}),flush=True)
 summary={'matches':len(paired),'accounts':len(included),'target':opts.target,'complete':len(paired)>=opts.target and len(included)>=len(ACCOUNTS),'key_mode':'personal' if client.key else 'relay','fresh_accounts':bool(opts.fresh_accounts),'excluded_original_match_ids':len(excluded_matches),'excluded_original_account_puuids':len(excluded_accounts)}
 (OUT/'collection-summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary),flush=True)
if __name__=='__main__':
 try:main()
 except ProbeBlocked as e:print(json.dumps({'blocked':str(e)}),flush=True);raise SystemExit(2)
