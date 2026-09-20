"""R101 executable mutations. Temporary Go overlays / JS overrides, never edit production."""
import json, os, subprocess, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
OUT=ROOT/'docs/r101-validation/mutations';OUT.mkdir(parents=True,exist_ok=True)
ENV=dict(os.environ,GOCACHE='/tmp/deep-legends-go-cache',GOTMPDIR='/tmp/deep-legends-go-tmp')
results=json.loads((OUT/'matrix.json').read_text()) if os.environ.get('R101_MUTATION_FROM') and (OUT/'matrix.json').exists() else []
if os.environ.get('R101_MUTATION_FROM'): results=[r for r in results if r['id'] < os.environ['R101_MUTATION_FROM']]
def probe(name,file,old,new,test='',kind='go',envkey='',occurrence=0):
 source=(ROOT/file).read_text();assert source.count(old)>occurrence,(name,source.count(old))
 if os.environ.get('R101_MUTATION_FROM') and name < os.environ['R101_MUTATION_FROM']: return
 at=source.index(old)
 for _ in range(occurrence): at=source.index(old,at+len(old))
 mutated=source[:at]+new+source[at+len(old):]
 cmd=['go','test','-count=1','-v','-timeout=60s','-run',test,'.'] if kind=='go' else (['node','desktop/r99-facade-picker-layout.cjs'] if kind=='browser' else ['node','--test',test])
 env=ENV.copy();env['R99_BROWSER_OUTPUT']=str(OUT/(name+'-baseline-browser'))
 clean=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=120)
 (OUT/(name+'-baseline.txt')).write_text(clean.stdout+clean.stderr)
 assert clean.returncode==0,(name,'baseline',clean.stdout[-1500:],clean.stderr[-1500:])
 with tempfile.TemporaryDirectory(prefix='r101-mut-') as tmp:
  changed=Path(tmp)/Path(file).name;changed.write_text(mutated)
  if kind=='go':
   overlay=Path(tmp)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(ROOT/file):str(changed)}}));cmd=cmd[:2]+['-overlay',str(overlay)]+cmd[2:]
  else:env[envkey]=str(changed)
  env['R99_BROWSER_OUTPUT']=str(OUT/(name+'-browser'))
  result=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=120)
 log=result.stdout+result.stderr;(OUT/(name+'.txt')).write_text(log)
 killed=result.returncode!=0 and ('AssertionError' in log or 'FAIL: TestR101' in log) and not any(x in log for x in ['build failed','test timed out','SyntaxError'])
 if name=='19-no-nowrap': killed = killed and 'rank segment must not overflow or wrap' in log
 if name=='20-cards-right': killed = killed and 'cards must be inside left column' in log
 results.append(dict(id=name,file=file,killed=killed,baseline_exit=clean.returncode,exit=result.returncode));(OUT/'matrix.json').write_text(json.dumps(results,indent=2)+'\n')
 print(name,'KILLED' if killed else 'INVALID/SURVIVED',flush=True);assert killed,log[-2000:]

probe('01-current-icon','r101_probe.go','item.ItemID != *current.Icon','item.ItemID == *current.Icon','^TestR101IconProbePicksOwnedNonCurrentIcon$')
probe('02-no-icon-restore','r101_probe.go','defer func() {','defer func() { if true { return }','^TestR101IconProbeRestoresOnFailure$')
probe('03-log-icon-id','r101_probe.go','"probe": label, "status": code','"profileIconId":target, "probe": label, "status": code','^TestR101IconProbeNeverLogsIconID$')
probe('04-current-banner','r101_probe.go','key != r101ItemID(original["itemId"])','key == r101ItemID(original["itemId"])','^TestR101BannerProbeSwitchesToDifferentOwnedBanner$')
probe('05-drop-data','r101_probe.go','variant[k] = v','if k != "data" { variant[k] = v }','^TestR101BannerProbePreservesDataField$')
probe('06-status-means-changed','r101_probe.go','changed := readErr == nil && r101ItemID(observed["itemId"]) == target','_ = readErr; _ = observed; changed := status == 200','^TestR101BannerProbeReadsBackBeforeClaimingSuccess$')
probe('07-no-banner-restore','r101_probe.go','defer func() {','defer func() { if true { return }','^TestR101BannerProbeAlwaysRestores$',occurrence=1)
probe('08-owned-always','facade_icons.go','Owned: owned[row.ID]','Owned: true','^TestR101IconOwnershipParsedFromInventory$')
probe('09-style-only-unowned','web/suite.js','button.disabled = icon.disabled || unknown || !icon.owned;','button.disabled = icon.disabled;',test='web/r101.test.cjs',kind='node',envkey='R101_SUITE_SOURCE')
probe('10-owned-default-off','web/suite.js','data-picker-owned ${unknown ? "disabled" : "checked"}','data-picker-owned ${unknown ? "disabled" : ""}',test='web/r101.test.cjs',kind='node',envkey='R101_SUITE_SOURCE')
probe('11-wrong-banner-inventory','facade_banners.go','const facadeBannerInventoryPath = "/lol-regalia/v3/inventory/REGALIA_BANNER"','const facadeBannerInventoryPath = "/lol-inventory/v2/inventory/REGALIA_BANNER"','^TestR101BannerOwnershipUsesRegaliaV3Only$')
probe('12-title-not-preserved','profile_facade.go','body["title"] = candidate.Value','body["title"] = ""','^TestR101ClearChallengesStillPreservesTitle$')
probe('13-previous-banner-remains','profile_facade.go','case "clear-emotes":\n\t\treturn facadeApplyResult{}, clearFacadeEmotes(ctx, client)','case "previous-banner":\n return a.writeChallengePreferences(ctx, client, map[string]any{"bannerAccent":"2"})\n case "clear-emotes":\n return facadeApplyResult{}, clearFacadeEmotes(ctx, client)','^TestR101PreviousBannerActionRemoved$')
probe('14-save-zero','web/suite.js','const timingDisabled = blocked || !lockWait;','if (!lockWait) sideConfig.lockDelayMs = 0; const timingDisabled = blocked || !lockWait;',test='web/r101.test.cjs',kind='node',envkey='R101_SUITE_SOURCE')
probe('15-hide-time','web/suite.js','const timingControls = `<span','const timingControls = !lockWait ? "" : `<span',test='web/r101.test.cjs',kind='node',envkey='R101_SUITE_SOURCE')
probe('16-disabled-change-guard','web/suite.js','!champSelectSettings()?.enabled || input.matches(":disabled")','!champSelectSettings()?.enabled',test='web/r101.test.cjs',kind='node',envkey='R101_SUITE_SOURCE')
probe('17-overwrite-saved-false','watch_rules.go','return normalizeWatchSettings(settings)','settings.ChampSelect.Enabled = true; return normalizeWatchSettings(settings)','^TestR101ExistingProfileKeepsItsOwnSetting$')
probe('18-empty-pool-writes','champselect.go','if len(champions) == 0 {','if len(champions) == 0 { _ = client.RequestJSON(ctx, http.MethodPatch, "/lol-champ-select/v1/session/actions/42", map[string]any{"championId":1,"completed":false}, nil)','^TestR101EnabledWithEmptySequencesSendsNothing$')
probe('19-no-nowrap','web/suite.css','.suite-segment button { white-space: nowrap; }','.suite-segment button { white-space: normal; }',kind='browser',envkey='R99_CSS_SOURCE')
probe('20-cards-right','web/suite.js','<section class="suite-card facade-icon-card"','</div><div class="facade-controls"><section class="suite-card facade-icon-card"',kind='browser',envkey='R99_SUITE_SOURCE')
probe('21-commit-copy-deleted','web/suite.js','改动只在左侧预览，确认后才写入客户端。','',test='web/r101.test.cjs',kind='node',envkey='R101_SUITE_SOURCE')
probe('22-first-entry','pro_seed_accounts.go','if entry.QueueType != "RANKED_SOLO_5x5" {','if false && entry.QueueType != "RANKED_SOLO_5x5" {','^TestR101SeedResolvesSoloQueueRank$')
probe('23-division-one','pro_seed_accounts.go','division := map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4}[entry.Rank]','division := 1','^TestR101SeedRankMapsRomanDivision$')
probe('24-default-unranked','pro_seed_accounts.go','return nil\n\t})\n\treturn rank, err','if len(rank)==0 {rank=json.RawMessage(`{"tier":"UNRANKED"}`)}; return nil\n\t})\n\treturn rank, err','^TestR101SeedWithoutSoloQueueStaysUnavailable$')
probe('25-three-minute-ttl','pro_seed_accounts.go','return 24 * time.Hour','return 3 * time.Minute','^TestR104SeedRank24HourCacheSeparateFromOverview$')
