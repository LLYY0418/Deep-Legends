import json,os,pathlib,subprocess,sys,urllib.request,urllib.error
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,LOL_LOOT_DATA_DIR=str(out/'data'),DEEP_LEGENDS_ITEM_PREWARM='0')
with (out/'service.log').open('w') as log:
 p=subprocess.Popen([sys.argv[2],'-desktop','-listen','127.0.0.1:0'],env=env,stdout=subprocess.PIPE,stderr=log,text=True)
 try:
  ready=json.loads(p.stdout.readline().removeprefix('LOOT_READY '));results=[]
  for endpoint in ['items','perks','summoner-spells']:
   request=urllib.request.Request(ready['baseUrl']+'/api/gameplay/'+endpoint,headers={'X-Local-Token':ready['token']})
   try:
    with urllib.request.urlopen(request,timeout=25) as r:
     body=json.load(r);result={'endpoint':endpoint,'status':r.status}
     result['items']=len(body.get('items',body.get('perks',body.get('spells',[]))))
   except urllib.error.HTTPError as e:result={'endpoint':endpoint,'status':e.code}
   results.append(result);print(json.dumps(result),flush=True)
  (out/'results.json').write_text(json.dumps(results,indent=2)+'\n')
 finally:p.terminate();p.wait(timeout=5)
