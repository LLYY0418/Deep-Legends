"""R217 credential-safe serial HTTP and dataset provenance helpers."""
import datetime,hashlib,json,pathlib,re,time,urllib.request,urllib.error,urllib.parse
ROOT=pathlib.Path(__file__).resolve().parent.parent
REPORT=ROOT/'docs/history/reports/r217'
UA='Mozilla/5.0'
ACTION='409a2b9ca50d15e50a4dace93552e3a40113dc2753'
def now():return datetime.datetime.now(datetime.timezone.utc).isoformat()
def write_json(path,value):path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n')
def old_exclusions():
 ids=set();accounts=set()
 for name in ['opgg-samples','opgg-validation-new-accounts']:
  folder=ROOT/'docs/history/reports/r211'/name
  ids.update(p['match_id']for p in json.loads((folder/'paired.json').read_text()))
  accounts.update(json.loads(f.read_text())['puuid']for f in folder.glob('account-*-riot.json'))
 assert len(accounts)==20
 ids.update(['KR_8401412521','KR_8397796525'])
 folder=ROOT/'docs/history/reports/r216'
 ids.update(p['match_id']for p in json.loads((folder/'opgg-holdout/paired.json').read_text()))
 accounts.update(a['puuid']for a in json.loads((folder/'holdout-accounts.json').read_text()))
 assert len(accounts)==44 and len(ids)==562
 return ids,accounts
class RequestFailure(Exception):
 def __init__(self,status,kind):self.status=status;self.kind=kind;super().__init__(str(status)+' '+kind)
class Client:
 def __init__(self,folder,logname='collection-log.jsonl'):
  self.folder=folder;folder.mkdir(parents=True,exist_ok=True);self.key=(ROOT/'riot_key.local.txt').read_text().strip()
  if not self.key:raise SystemExit('R217 requires personal riot_key.local.txt')
  self.last=0.;self.run=now();self.logpath=folder/logname
 def log(self,**entry):
  entry={'time':now(),'run':self.run,**entry};text=json.dumps(entry,ensure_ascii=False)
  if self.key in text:raise RuntimeError('Credential output refused')
  with self.logpath.open('a')as f:f.write(text+'\n')
 def read(self,url,file,kind,data=None,headers=None,riot=False,account_index=None):
  if file.exists():self.log(event='cache_hit',url_class=kind,file=str(file.relative_to(REPORT)),account_index=account_index);return file.read_bytes()
  hs={'User-Agent':UA,**(headers or {})}
  if riot:hs['X-Riot-Token']=self.key
  for attempt in range(4):
   if riot:time.sleep(max(0,1.35-(time.monotonic()-self.last)));self.last=time.monotonic()
   started=time.monotonic()
   try:
    with urllib.request.urlopen(urllib.request.Request(url,data=data,headers=hs),timeout=40)as response:raw=response.read();status=response.status
    self.log(event='request',url_class=kind,status=status,duration_ms=round(1000*(time.monotonic()-started)),retries=attempt,account_index=account_index)
    file.parent.mkdir(parents=True,exist_ok=True);file.write_bytes(raw);return raw
   except urllib.error.HTTPError as e:
    raw=e.read(8192);body=raw.decode(errors='replace').replace(self.key,'[REDACTED]');safe={k:v for k,v in e.headers.items()if k.lower()not in ['authorization','x-riot-token','set-cookie']}
    layer='http';parsed={}
    try:parsed=json.loads(body)
    except ValueError:pass
    if e.code==403:layer='riot_json_forbidden'if parsed.get('status',{}).get('status_code')==403 else 'proxy_connect'if 'CONNECT' in body or 'from proxy' in body else 'edge_or_origin_non_riot'
    self.log(event='request',url_class=kind,status=e.code,duration_ms=round(1000*(time.monotonic()-started)),retries=attempt,account_index=account_index,error_kind=layer)
    write_json(self.folder/'last-network-error.json',{'time':now(),'url_class':kind,'status':e.code,'headers':safe,'body':body,'layer':layer})
    if e.code==429 and attempt<3:
     wait=max(1,min(300,float(e.headers.get('Retry-After','130'))));self.log(event='cooldown',url_class=kind,seconds=wait,limit_type=e.headers.get('X-Rate-Limit-Type','unknown'));time.sleep(wait);continue
    if e.code in [500,502,503,504]and attempt<3:time.sleep(2**attempt);continue
    raise RequestFailure(e.code,layer)from None
   except (urllib.error.URLError,TimeoutError)as e:
    kind_error='proxy_connect'if '403' in str(e)and('tunnel'in str(e).lower()or'proxy'in str(e).lower())else 'network_unreachable'
    self.log(event='request',url_class=kind,status=None,duration_ms=round(1000*(time.monotonic()-started)),retries=attempt,account_index=account_index,error_kind=kind_error)
    if attempt<3:time.sleep(2**attempt);continue
    raise RequestFailure(None,kind_error)from None
 def riot(self,cluster,path,file,kind,query=None,account_index=None):
  url='https://'+cluster+'.api.riotgames.com'+path+('?' + urllib.parse.urlencode(query)if query else '')
  return json.loads(self.read(url,file,kind,riot=True,account_index=account_index))
 def page(self,name,tag,file,index):
  url='https://op.gg/zh-cn/lol/summoners/kr/'+urllib.parse.quote(name+'-'+tag);raw=self.read(url,file,'OP.GG page',account_index=index);html=raw.decode().replace('\\"','"');match=re.search(r'"puuid":"([^"]+)"',html)
  if not match:raise RequestFailure(200,'opgg_identity_missing')
  return url,match.group(1)
 def games(self,url,op_puuid,file,ended,index):
  args=[{'locale':'zh-cn','region':'kr','puuid':op_puuid,'gameType':'SOLORANKED','endedAt':ended,'champion':''}]
  raw=self.read(url,file,'Flight',json.dumps(args).encode(),{'Content-Type':'text/plain;charset=UTF-8','Next-Action':ACTION},account_index=index)
  for line in raw.decode().splitlines():
   if line.startswith('1:'):return json.loads(line[2:]).get('data',[])
  raise RequestFailure(200,'opgg_action_changed')
def op_people(game):return game.get('team_blue',[])+game.get('team_red',[])
def fingerprint_op(game):return sorted((p['champion_id'],p['stats']['kill'],p['stats']['death'],p['stats']['assist'])for p in op_people(game))
def fingerprint_riot(raw):return sorted((p['championId'],p['kills'],p['deaths'],p['assists'])for p in raw['info']['participants'])
def freeze_manifest(folder):
 files=[]
 for p in sorted(folder.rglob('*')):
  if p.is_file()and p.name!='manifest-sha256.json':files.append({'path':str(p.relative_to(folder)),'bytes':p.stat().st_size,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()})
 manifest={'frozenAt':now(),'files':files};write_json(folder/'manifest-sha256.json',manifest);return hashlib.sha256((folder/'manifest-sha256.json').read_bytes()).hexdigest()
