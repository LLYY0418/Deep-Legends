"""Assertion-killing R252 reversions; Go overlays and JS copies never edit source."""
import argparse, datetime, hashlib, json, os, pathlib, re, subprocess, tempfile, time
parser=argparse.ArgumentParser();parser.add_argument("--output",required=True);args=parser.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=pathlib.Path(args.output);out.mkdir(parents=True,exist_ok=False)
rows=[]
tracked=["backend/arena_result.go","backend/season_stats.go","backend/web/gameplay.js","backend/license_r233_test.go","backend/license_r242_test.go","backend/frontend_assets_disabled.go","desktop/verify-license-release.cjs"]
before={name:hashlib.sha256((root/name).read_bytes()).hexdigest() for name in tracked}
def base(name):return subprocess.check_output(["git","show","0505fbb:"+name],cwd=root,text=True)
def run(name,target,text,command,envname=None):
 with tempfile.TemporaryDirectory(prefix="r252-mutant-") as temp:
  assert text != (root/target).read_text(), "No-op mutation: " + name
  source=pathlib.Path(temp)/(pathlib.Path(target).name);source.write_text(text)
  env=os.environ.copy()
  if envname:env[envname]=str(source)
  else:
   overlay=pathlib.Path(temp)/"overlay.json";overlay.write_text(json.dumps({"Replace":{str(root/target):str(source)}}));command=["go","test","-overlay",str(overlay),*command]
  tick=time.monotonic();start=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat()
  with (out/(name+".log")).open("x") as log:r=subprocess.run(command,cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT)
  log=(out/(name+".log")).read_text(errors="replace")
  killed=r.returncode!=0 and bool(re.search(r"--- FAIL:|ERR_ASSERTION|AssertionError",log)) and not bool(re.search(r"SyntaxError|ReferenceError|Unbalanced function|setup failed|failed to listen|undefined:|cannot find module|Cannot find module",log))
  expected=("TestR252Arena" if name.startswith("arena-") else "TestR252Manual" if name.startswith("manual-") else "R252 missing map" if name.startswith("tags-") else "R252 arena detail" if name.startswith("js-arena-") else "R252 visible tag hydration" if name.startswith("hydration-") else "TestR233FailedCounterSave" if name=="old-license_r233_test" else "TestR242LicenseExpiry" if name=="old-license_r242_test" else "TestR252DefaultEmbeddedAssets" if name=="default-embeds-activation" else "R252 disabled release rejects embedded activation text")
  failure_lines=[line for line in log.splitlines() if re.match(r"--- FAIL:|✖ (?!failing tests:)",line)]
  killed=killed and bool(failure_lines) and all(expected in line for line in failure_lines)
  row=dict(expected_assertion=expected,failed_tests=failure_lines,name=name,target=target,start=start,elapsed_seconds=round(time.monotonic()-tick,3),exit_code=r.returncode,assertion_killed=killed,mutated_sha256=hashlib.sha256(text.encode()).hexdigest(),command=command)
  rows.append(row);print(json.dumps(row),flush=True)
source=(root/"backend/arena_result.go").read_text()
run("arena-observed-only","backend/arena_result.go",base("backend/arena_result.go"),["-count=1","-run","^TestR252Arena","./backend"])
run("arena-fixed-truncates-observed","backend/arena_result.go",source.replace("max(count, 6)","6").replace("max(count, 8)","8"),["-count=1","-run","^TestR252Arena","./backend"])
source=(root/"backend/season_stats.go").read_text()
run("manual-persisted-gate","backend/season_stats.go",source.replace("if !fresh && accountHash !=", "if accountHash !="),["-count=1","-run","^TestR252Manual","./backend"])
run("manual-query-gate","backend/season_stats.go",source.replace("!fresh && !previous.IsZero()", "!previous.IsZero()"),["-count=1","-run","^TestR252Manual","./backend"])
source=(root/"backend/web/gameplay.js").read_text()
run("tags-no-map-fallback","backend/web/gameplay.js",source.replace("map===0 ? fallback : \"other\"", "\"other\""),["node","--test","backend/web/r241.test.cjs"],"R252_GAMEPLAY_SOURCE")
old_count="const count=teams.size>=2 ? teams.size : queue===1750 ? 6 : [1700,1710].includes(queue) ? 8 : 0;"
run("js-arena-observed-only","backend/web/gameplay.js",source.replace("const count=Math.max(teams.size, queue===1750 ? 6 : [1700,1710,1701,1704,1720,1731,1732,1740].includes(queue) ? 8 : 0);",old_count),["node","--test","backend/web/r252.test.cjs"],"R252_GAMEPLAY_SOURCE")
run("js-arena-fixed-truncates-observed","backend/web/gameplay.js",source.replace("Math.max(teams.size, queue===1750 ? 6 : [1700,1710,1701,1704,1720,1731,1732,1740].includes(queue) ? 8 : 0)","queue===1750 ? 6 : [1700,1710,1701,1704,1720,1731,1732,1740].includes(queue) ? 8 : teams.size"),["node","--test","backend/web/r252.test.cjs"],"R252_GAMEPLAY_SOURCE")
start=source.index("    // Index once:");end=source.index("    tab.matchTagsHydrating=true",start)
old=next(line for line in base("backend/web/gameplay.js").splitlines() if "const matches=(tab.data?.matches" in line and "matchTierNodeIsVisible" in line)
run("hydration-per-match-full-tree","backend/web/gameplay.js",source[:start]+old+"\n"+source[end:],["node","--test","backend/web/r252.test.cjs"],"R252_GAMEPLAY_SOURCE")
for name,test in [("license_r233_test.go","TestR233FailedCounterSaveDoesNotRefreshCachedLifetime"),("license_r242_test.go","TestR242LicenseExpiryStrictSignedField")]:
 run("old-"+name[:-3],"backend/"+name,base("backend/"+name),["-count=1","-tags","license","-run","^"+test+"$","./backend"])
source=(root/"backend/frontend_assets_disabled.go").read_text()
mutant=source.replace("web/default/index.html web/default/license-ui.js","web/index.html web/license-ui.js").replace('"web/default"','"web"')
run("default-embeds-activation","backend/frontend_assets_disabled.go",mutant,["-count=1","-run","^TestR252DefaultEmbeddedAssets","./backend"])
source=(root/"desktop/verify-license-release.cjs").read_text();start=source.index('    for (const value of ["注册码"');end=source.index('    const flag =',start)
run("default-audit-misses-text","desktop/verify-license-release.cjs",source[:start]+source[end:],["node","--test","desktop/disabled-build.test.cjs"],"R252_LICENSE_VERIFIER")
after={name:hashlib.sha256((root/name).read_bytes()).hexdigest() for name in tracked};assert before==after
result=dict(source_inputs=before,unchanged_after_mutations=True,rows=rows,all_assertion_killed=all(row["assertion_killed"] for row in rows))
with (out/"results.json").open("x") as f:json.dump(result,f,indent=2);f.write("\n")
raise SystemExit(0 if result["all_assertion_killed"] else 1)
