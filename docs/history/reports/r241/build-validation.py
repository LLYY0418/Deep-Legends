from pathlib import Path
import subprocess,json,os,hashlib,datetime,tempfile
root=Path.cwd();out=root/'docs/history/reports/r241';dist=root/'dist/r241-validation';dist.mkdir(parents=True,exist_ok=True)
version=json.loads((root/'desktop/package.json').read_text())['version'];fingerprint=subprocess.check_output(['node','desktop/source-fingerprint.cjs'],text=True).strip();artifacts=[]
for platform,arch,suffix in [('darwin','arm64','darwin-arm64-public'),('windows','amd64','public.exe')]:
 target=dist/f'loot-service-{version}-r241-{suffix}'
 flags=(' -H=windowsgui' if platform=='windows' else '')+f' -s -w -buildid= -X main.version={version} -X main.buildFingerprint={fingerprint} -X main.riotAPIKey= -X main.riotAPIKeyCipher='
 cmd=['go','build','-buildvcs=false','-trimpath','-ldflags',flags.strip(),'-o',str(target),'./backend']
 env=os.environ.copy();env.update(GOOS=platform,GOARCH=arch,CGO_ENABLED='0');subprocess.run(cmd,check=True,env=env)
 subprocess.run(['node','desktop/verify-build-fingerprint.cjs',str(target),fingerprint],check=True)
 subprocess.run(['node','-e','require("./desktop/verify-embedded-riot-key.cjs").verifyRiotKeyPolicy(process.argv[1],"public");console.log("public Riot key policy verified")',str(target)],check=True)
 artifacts.append({'platform':platform,'arch':arch,'path':str(target),'bytes':target.stat().st_size,'sha256':hashlib.sha256(target.read_bytes()).hexdigest(),'command':cmd})
 if platform=='darwin':
  with tempfile.TemporaryDirectory(prefix='r241-self-test-') as temp:
   env=os.environ.copy();env['LOL_LOOT_DATA_DIR']=temp;subprocess.run([str(target),'--self-test'],env=env,check=True)
assert fingerprint==subprocess.check_output(['node','desktop/source-fingerprint.cjs'],text=True).strip()
receipt={'built_at':datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(),'version':version,'source_fingerprint':fingerprint,'key_mode':'public','scope':'Backend validation binaries only; shared workspace source. No installer/package/publication. Windows execution and upgrade not verified.','artifacts':artifacts}
(out/'public-build.json').write_text(json.dumps(receipt,indent=2)+'\n');print('Built',version,'public',fingerprint)
