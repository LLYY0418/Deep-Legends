"""R266 assertion mutations run in temporary copies; production files are never edited."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument("--out", type=Path, required=True)
OUT = parser.parse_args().out
OUT.mkdir(parents=True, exist_ok=True)
results = []
env = dict(os.environ, RIOT_API_KEY="RGAPI-00000000-0000-0000-0000-000000000000")


def run(name, command, cwd=ROOT, extra=None, mutant=False):
    started = time.time()
    completed = subprocess.run(command, cwd=cwd, env={**env, **(extra or {})}, capture_output=True, text=True, timeout=90)
    output = completed.stdout + completed.stderr
    (OUT / (name + ".log")).write_text(output)
    valid = completed.returncode != 0 if mutant else completed.returncode == 0
    if mutant:
        valid = valid and any(marker in output for marker in ("FAIL", "✖", "not ok"))
        valid = valid and not any(marker in output for marker in ("build failed", "SyntaxError:", "Cannot find module", "no tests to run"))
    result = {"name": name, "command": command, "exitCode": completed.returncode, "expectedFailure": mutant,
              "durationSeconds": round(time.time()-started, 3), "assertionVerified": valid,
              "lastLines": output.splitlines()[-5:]}
    results.append(result)
    (OUT / "result.json").write_text(json.dumps(results, ensure_ascii=False, indent=2))
    print(json.dumps({"name": name, "expectedFailure": mutant, "verified": valid}), flush=True)
    if not valid:
        raise RuntimeError("Mutation or restoration assertion failed: " + name)


def replace(source, old, new):
    if old not in source:
        raise RuntimeError("Mutation target missing: " + old[:80])
    return source.replace(old, new)


with tempfile.TemporaryDirectory(prefix="r266-mutations-") as temporary:
    temp = Path(temporary)
    node_cases = [
        ("p1-sort-redraw", "backend/web/history-filters.js", "if(structure)menu.querySelector", "if(true)menu.querySelector", "R266_FILTER_SOURCE", "backend/web/r266-filters.test.cjs"),
        ("p2-find-redraw", "backend/web/history-filters.js", "(ctx.progress || ctx.render)()", "ctx.render()", "R266_FILTER_SOURCE", "backend/web/r266-filters.test.cjs"),
        ("p2-no-options-cache", "backend/web/history-filters.js", "if(!cached.has(cacheKey))", "if(true)", "R266_FILTER_SOURCE", "backend/web/r266-filters.test.cjs"),
        ("p3-no-pending", "backend/web/gameplay.js", "return Boolean(tab?.loading && !tab.matchesReceived && !tab.overviewCardLoad?.ready?.has('matches') && !(tab.data?.matches?.length));", "return false;", "R266_GAMEPLAY_SOURCE", "backend/web/r266-loading.test.cjs"),
        ("p4-no-session-reset", "backend/web/app.js", "function syncCollectionSession(data) {", "function syncCollectionSession(data) { return;", "R266_APP_SOURCE", "backend/web/r266-loading.test.cjs"),
        ("p7-cancel-inflight", "backend/web/gameplay.js", "if(riotTab(tab) && (tab.loading || tab.loadingMore))return false;", "", "R266_GAMEPLAY_SOURCE", "backend/web/r266-loading.test.cjs"),
        ("p74-public-no-cipher", "desktop/verify-embedded-riot-key.cjs", "if (!encrypted) throw new Error(expectFake ? \"expected fake embedded Riot API key ciphertext was not found\" : \"embedded Riot API key ciphertext was not found or could not be decrypted\");", "", "R266_KEY_POLICY_SOURCE", "scripts/r266-key-policy.test.cjs"),
    ]
    run("node-original", ["node", "--test", "backend/web/r266-filters.test.cjs", "backend/web/r266-loading.test.cjs", "scripts/r266-key-policy.test.cjs"])
    for name, file, old, new, variable, test in node_cases:
        variant = temp / (name + ".js")
        variant.write_text(replace((ROOT / file).read_text(), old, new))
        run(name, ["node", "--test", test], extra={variable: str(variant)}, mutant=True)
    go_root = temp / "go"
    go_root.mkdir()
    shutil.copytree(ROOT / "backend", go_root / "backend")
    for file in ("go.mod", "go.sum"):
        shutil.copy2(ROOT / file, go_root / file)
    go_cases = [
        ("p7-ignore-puuid", "gameplay.go", "reference.PlayerRef = a.knownProPUUID(reference)", "reference.PlayerRef = \"\"", "TestR266ProPUUIDSkipsAccountAndRetriesJoinFlight"),
        ("p7-no-foreground-pause", "riot_routing.go", "if p.foreground == nil {\n\t\treturn nil", "if true {\n\t\treturn nil", "TestR266ForegroundResumesAfterFiveSeconds"),
        ("p7-no-account-interval", "pro_background.go", "now.Sub(at) < 10*time.Minute", "now.Sub(at) < 0", "TestR266ProVisibilityAndAccountInterval"),
        ("p74-background-direct", "riot_routing.go", "if isRiotBackground(ctx) {", "if false {", "TestR266EmbeddedSourceAndBackgroundRelay"),
        ("p74-rejected-still-direct", "riot_routing.go", "s.embeddedRejected = true", "s.embeddedRejected = false", "TestR266EmbeddedRejectedAndQuota"),
        ("p74-log-key", "riot_routing.go", '"event": "riot_request"', '"credential": riotEmbeddedKey(), "event": "riot_request"', "TestR266EmbeddedSourceAndBackgroundRelay"),
        ("p74-fallback-hedge", "riot_relay_hedge.go", "if fallback, _ := ctx.Value(riotFallbackAttemptKey{}).(bool); fallback {", "if fallback, _ := ctx.Value(riotFallbackAttemptKey{}).(bool); fallback && false {", "TestR266EmbeddedMultiEntryFallbackIsOneAdditionalRequest"),
        ("p74-fallback-retry", "riot_relay_hedge.go", "if fallback {\n\t\tnetworkAttemptLimit, attemptLimit = 1, 1", "if false {\n\t\tnetworkAttemptLimit, attemptLimit = 1, 1", "TestR266EmbeddedMultiEntryFallbackIsOneAdditionalRequest"),
        ("p74-ignore-retry-date", "riot_api.go", 'riotRetryAfter(response.Header.Get("Retry-After"), time.Now(), 3)', "3", "TestR266EmbeddedHTTPDateRetryAfter"),
        ("p10-no-line-limit", "storage.go", "limit := 4095", "limit := 999999", "TestR266PartialSummaryAndBoundedScoreDiagnostics"),
    ]
    run("go-original", ["go", "test", "./backend", "-count=1", "-run", "^TestR266"], cwd=go_root)
    for name, file, old, new, test in go_cases:
        target = go_root / "backend" / file
        original = target.read_text()
        target.write_text(replace(original, old, new))
        try:
            run(name, ["go", "test", "./backend", "-count=1", "-run", "^"+test+"$"], cwd=go_root, mutant=True)
        finally:
            target.write_text(original)
    run("go-restored", ["go", "test", "./backend", "-count=1", "-run", "^TestR266"], cwd=go_root)
    hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in [ROOT / "backend" / case[1] for case in go_cases]}
    (OUT / "production-hashes.json").write_text(json.dumps(hashes, indent=2))
