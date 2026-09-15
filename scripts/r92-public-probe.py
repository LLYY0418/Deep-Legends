import concurrent.futures,json,os,pathlib,subprocess,sys,time,urllib.request,urllib.parse
binary,out,mode=sys.argv[1:4];out=pathlib.Path(out);out.mkdir(parents=True,exist_ok=True)
data=out/'data';env=dict(os.environ,LOL_LOOT_DATA_DIR=str(data),DEEP_LEGENDS_ITEM_PREWARM='0');env.pop('RIOT_API_KEY',None)
paths=['/lol-game-data/assets/v1/profile-icons/%d.jpg'%i for i in range(20)]
paths += ['/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_boost.png', '/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_exhaust.png', '/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_flash.png', '/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_haste.png', '/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_heal.png', '/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_smite.png', '/lol-game-data/assets/DATA/Spells/Icons2D/Summoner_Teleport_New.png', '/lol-game-data/assets/DATA/Spells/Icons2D/SummonerIgnite.png', '/lol-game-data/assets/DATA/Spells/Icons2D/SummonerBarrier.png']
paths+=['/lol-game-data/assets/v1/perk-images/Styles/%s.png'%s for s in ['Precision/PressTheAttack/PressTheAttack','Precision/LethalTempo/LethalTempoTemp','Precision/FleetFootwork/FleetFootwork','Precision/Conqueror/Conqueror','Domination/Electrocute/Electrocute','Domination/DarkHarvest/DarkHarvest','Sorcery/SummonAery/SummonAery','Sorcery/ArcaneComet/ArcaneComet','Resolve/GraspOfTheUndying/GraspOfTheUndying','Inspiration/FirstStrike/FirstStrike']]
results=[]
for label in ['cold','restart']:
 with (out/(label+'.log')).open('w') as log:
  p=subprocess.Popen([binary,'-desktop','-listen','127.0.0.1:0'],env=env,stdout=subprocess.PIPE,stderr=log,text=True)
  try:
   ready=json.loads(p.stdout.readline().removeprefix('LOOT_READY '))
   def get(path,summary=False):
    s=time.perf_counter()
    try:
     with urllib.request.urlopen(urllib.request.Request(ready['baseUrl']+path,headers={'X-Local-Token':ready['token']}),timeout=35) as r:
      b=r.read();return json.loads(b) if summary else {'status':r.status,'bytes':len(b),'ms':(time.perf_counter()-s)*1000}
    except Exception as e:return {'status':getattr(e,'code',0),'bytes':0,'ms':(time.perf_counter()-s)*1000}
   if mode=='images':
    started=time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:rows=list(pool.map(get,['/api/image?'+urllib.parse.urlencode({'path':x}) for x in paths]))
    result={'label':label,'wall_ms':(time.perf_counter()-started)*1000,'count':len(rows),'bytes':sum(r['bytes'] for r in rows if r['status']==200),'rows':[dict(path=x,**r) for x,r in zip(paths,rows)]}
   else:
    get('/api/pro-players');deadline=time.monotonic()+55;events=[]
    while time.monotonic()<deadline:
     time.sleep(.2)
     events=[json.loads(l) for l in (data/'logs/diagnostics.jsonl').read_text().splitlines() if 'pro_directory_cost' in l]
     if any(e.get('stage')=='ladder' for e in events):break
    body=get('/api/pro-players',True);quality={k:body.get(k) for k in ['updating','partial','unavailable','playerCount','accountCount','ladderRankPartial']};quality['knownRanks']=sum(a.get('ladderRankKnown',False) for t in body.get('teams',[]) for p in t.get('players',[]) for a in p.get('accounts',[]));result={'label':label,'events':events,'quality':quality}
   results.append(result);(out/'results.json').write_text(json.dumps(results,indent=2)+'\n');print(json.dumps(result),flush=True)
  finally:p.terminate();p.wait(timeout=5)
 if mode!='images':break
