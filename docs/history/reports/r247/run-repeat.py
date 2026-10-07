import subprocess,datetime,time,json,pathlib,sys
root=pathlib.Path(__file__).resolve().parents[4];out=pathlib.Path(__file__).parent
mode=sys.argv[1];count=20 if mode=="normal" else 10
load=None;rows=[]
try:
 if mode=="load":load=subprocess.Popen(["node","-e","for(;;){}"],cwd=root,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 for i in range(1,count+1):
  if load is not None and load.poll() is not None:raise RuntimeError("CPU load exited")
  started=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));at=time.monotonic();cmd=["node","--test","--test-name-pattern=dirty collection rescans","desktop/refresh-orchestration.test.cjs"]
  result=subprocess.run(cmd,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=30)
  (out/f"{mode}-{i:02d}.log").write_text(result.stdout)
  row={"run":i,"started":started.isoformat(),"finished":datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(),"duration_s":round(time.monotonic()-at,3),"exit_code":result.returncode,"last5":result.stdout.rstrip().splitlines()[-5:]};rows.append(row)
  (out/f"{mode}.json").write_text(json.dumps({"command":cmd,"cpu_load":"node -e for(;;){}; one continuous separate process" if load else None,"load_pid":load.pid if load else None,"runs":rows},ensure_ascii=False,indent=2)+"\n");print(row,flush=True)
  if result.returncode:sys.exit(result.returncode)
finally:
 if load is not None:
  load.terminate()
  try:load.wait(timeout=3)
  except subprocess.TimeoutExpired:load.kill();load.wait()
