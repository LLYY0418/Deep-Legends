#!/usr/bin/env python3
"""R84 backend mutation checks using disposable Go overlays; no application build."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / 'backend'
CASES = [
    ("legacy phase field renamed", "desktop_startup.go", '"spawn_to_ready":', '"spawn_to_ready_changed":',
     "TestDesktopStartupStageShellAndExportEndToEnd/normal$", "Missing/invalid measured phase: spawn_to_ready"),
    ("stage acknowledged without disk write", "desktop_startup_stage.go", "a.storage.appendDiagnostic(map[string]any{",
     "func(map[string]any) error { return nil }(map[string]any{", "TestDesktopStartupStageHTTPValidationAndDiskWrite/backend$",
     "successful HTTP did not persist the bounded event"),
    ("stage accepts arbitrary fields", "desktop_startup_stage.go", "decoder.DisallowUnknownFields()", "// unknown fields accepted",
     "TestDesktopStartupStageHTTPValidationAndDiskWrite/unknown_field$", "status 204, want 400"),
]


def main():
    env = dict(os.environ, GOCACHE=str(ROOT / ".gocache"), GOMODCACHE=str(ROOT / ".gomodcache"), GOTMPDIR=tempfile.gettempdir(), GOPROXY="off")
    for name in ("GOOS", "GOARCH", "R84_STAGE_SOURCE"):
        env.pop(name, None)
    with tempfile.TemporaryDirectory(prefix="r84-mutations-") as directory:
        work = Path(directory)

        def run(pattern, overlay=None):
            args = ["go", "test", "-count=1", "-timeout=45s", "-run", "^" + pattern]
            if overlay:
                args += ["-overlay", str(overlay)]
            return subprocess.run(args + ["./backend"], cwd=ROOT, env=env, text=True,
                                  stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)

        baseline = run("TestDesktopStartup")
        if baseline.returncode:
            raise SystemExit("BASELINE FAILED:\n" + baseline.stdout)
        print("PASS real HTTP/shell/export baseline", flush=True)
        evidence = []
        for name, relative, before, after, pattern, message in CASES:
            original = (BACKEND / relative).read_text()
            if original.count(before) != 1:
                raise SystemExit("Mutation target changed: " + name)
            changed = work / relative
            changed.parent.mkdir(parents=True, exist_ok=True)
            changed.write_text(original.replace(before, after))
            overlay = work / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {str(BACKEND / relative): str(changed)}}))
            result = run(pattern, overlay)
            if not result.returncode or "--- FAIL:" not in result.stdout or message not in result.stdout:
                raise SystemExit(f"SURVIVED or invalid mutation: {name}\n{result.stdout}")
            evidence.append({"mutation": name, "assertion": message})
            print("KILLED " + name + ": " + message, flush=True)
        print(json.dumps({"killed": len(evidence), "mutations": evidence}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
