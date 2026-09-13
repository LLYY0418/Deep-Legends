"""Run R81 behavioral mutations in disposable copies, never mutate the workspace."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
ENV = dict(os.environ, GOCACHE=str(ROOT / ".gocache"), GOMODCACHE=str(ROOT / ".gomodcache"), GOTMPDIR="/tmp", TMPDIR="/tmp")
CASES = [
 ("busy gate disabled", "update.go", 'switch u.status.State {\n\tcase "checking", "downloading", "verifying", "applying":\n\t\treturn true\n\t}\n\treturn false', 'return false'),
 ("equal version reported as newer", "update.go", "left.parts[i] > right.parts[i]", "left.parts[i] >= right.parts[i]"),
 ("v prefix removed", "update.go", "`^v?", "`^"),
 ("release below prerelease", "update.go", 'if left.pre == "" {\n\t\treturn 1', 'if left.pre == "" {\n\t\treturn -1'),
 ("dev branch removed", "update.go", 'if current == "dev" {\n\t\treturn u\n\t}', ''),
 ("mirror fallback stops after first", "update.go", '\t\tlast = err\n', '\t\tlast = err\n\t\treturn updateManifest{}, last\n'),
 ("cache TTL inverted", "update.go", 'now.Sub(u.cache.CheckedAt) < updateCacheTTL', 'now.Sub(u.cache.CheckedAt) > updateCacheTTL'),
 ("digest result inverted", "update_download.go", '!strings.EqualFold(actual, expected)', 'strings.EqualFold(actual, expected)'),
 ("digest verification bypassed", "update_download.go", 'if !strings.EqualFold(actual, expected) {', 'if false {'),
 ("bad checksum retains part", "update_download.go", '_ = os.Remove(part)', '// mutation: retain part'),
 ("cancel retains part", "update_download.go", '_ = os.Remove(filepath.Join(u.directory, manifest.Asset.Name+".part"))', '// mutation: retain cancelled part'),
 ("space safety margin removed", "update_download.go", 'float64(manifest.Asset.Size)*1.2', 'float64(manifest.Asset.Size)*1.0'),
 ("speed becomes lifetime average", "update_download.go", 'cutoff := at.Add(-5 * time.Second)', 'cutoff := time.Time{}'),
 ("upgrade updated flag removed", "installer/update.go", ' /S --updated /D=', ' /S /D='),
 ("upgrade forces shortcut removal", "installer/update.go", ' /S --updated /D=', ' /S --updated --no-desktop-shortcut /D='),
 ("upgrade initial progress page removed", "installer/update.go", '`<div class="page on" id="page-install">`', '`<div class="page" id="page-install">`'),
]
results=[]
with tempfile.TemporaryDirectory(prefix="r81-mutations-") as temporary:
 work=Path(temporary)
 for file in ROOT.glob("*.go"):
  if not file.name.endswith("_test.go") or file.name=="update_test.go":shutil.copy2(file,work/file.name)
 for name in ["go.mod","go.sum","prestige_chromas.json"]:shutil.copy2(ROOT/name,work/name)
 for name in ["web","data","installer"]:shutil.copytree(ROOT/name,work/name)
 def run(installer=False):
  pattern='^TestUpdate' if installer else '^TestUpdate(Busy|Versions|Dev|Mirror|Cache|Manifest|Download|Resume|Cancel|Five|Apply|ProgressSSE|After)'
  return subprocess.run(["go","test","-count=1","-run",pattern,"."],cwd=work/"installer" if installer else work,env=ENV,capture_output=True,text=True)
 for module in [False,True]:
  baseline=run(module)
  if baseline.returncode:raise RuntimeError(baseline.stdout+baseline.stderr)
 print("Both unmodified baselines PASS",flush=True)
 for label,filename,old,new in CASES:
  file=work/filename;original=file.read_text();assert original.count(old)==1,(filename,old,original.count(old))
  file.write_text(original.replace(old,new,1))
  result=run(filename.startswith("installer/"))
  caught=result.returncode!=0 and "--- FAIL:" in result.stdout
  results.append({"mutation":label,"caughtByTest":caught})
  print(label,"CAUGHT" if caught else "MISSED",flush=True)
  if not caught:print(result.stdout+result.stderr,flush=True)
  file.write_text(original)
output=ROOT/"output/update-r81/go-mutations.json";output.parent.mkdir(parents=True,exist_ok=True);output.write_text(json.dumps(results,ensure_ascii=False,indent=2)+"\n")
raise SystemExit(0 if all(item["caughtByTest"] for item in results) else 1)
