import tempfile,pathlib,subprocess,os,sys,shutil
root=pathlib.Path(__file__).resolve().parents[4];directory=pathlib.Path(tempfile.mkdtemp(prefix='r240-release-audit-'));backend=directory/'release-backend-public.exe'
env=os.environ.copy();env.update(GOOS='windows',GOARCH='amd64',CGO_ENABLED='0',GOFLAGS='',DEEP_LEGENDS_KEY_MODE='public')
try:
 r=subprocess.run(['go','build','-buildvcs=false','-trimpath','-ldflags','-s -w -H=windowsgui -buildid= -X main.version=0.12.75 -X main.riotAPIKey= -X main.riotAPIKeyCipher=','-o',str(backend),'./backend'],cwd=root,env=env)
 if r.returncode:sys.exit(r.returncode)
 r=subprocess.run(['node','scripts/r240-audit-staging.cjs','dist/R240-staging-public',str(backend)],cwd=root);sys.exit(r.returncode)
finally:shutil.rmtree(directory)
