#!/usr/bin/env python3
"""R82 addendum: mutate disposable installer copies, including Windows-only glue."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
CASES = [
    ("A: update skips portable handoff", "completion.go",
     "\thooks.Cleanup()", "\tif options.Update { return }\n\thooks.Cleanup()"),
    ("A: Windows update bypasses completion", "install_windows.go",
     "\t\tcase <-finished:\n", "\t\tcase <-finished:\n\t\t\tif a.options.Update { return }\n"),
    ("B: cleanup closes another window", "completion.go",
     "hooks.CloseWindow(installerWindow)", "hooks.CloseWindow(installerWindow + 1)"),
    ("B: Windows CloseInstaller kills application", "handoff_windows.go",
     "\t\tCloseInstaller: func(visible bool) {\n",
     '\t\tCloseInstaller: func(visible bool) {\n\t\t\t_ = exec.Command("taskkill", "/F", "/IM", "Deep Legends.exe").Run()\n'),
    ("D: template and replacement wording diverge", "ui/installer.html",
     "安装已完成，正在等待应用窗口…", "安装已完成了，正在等待应用窗口…"),
    ("D: upgrade returns unchanged template", "update.go",
     "if err != nil || !options.Update {", "if err != nil || options.Update || !options.Update {"),
]
PATTERN = "^Test(Installation|InstallerExit|WindowsInstall|WindowsHandoffExitAdapter|UpdateCompletion)"
FAST_HELPER = 'func installFast(a *installerApp) { postMessage.Call(a.window.hwnd, WM_CLOSE, 0, 0) }\n'
KILL_HELPER = '''import "os/exec"
func installFast(a *installerApp) {
    _ = exec.Command("taskkill", "/F", "/IM", "Deep Legends.exe").Run()
    postMessage.Call(a.window.hwnd, WM_CLOSE, 0, 0)
}
'''
INSTALL_CASE = '\tcase "install":\n'
CASE_BYPASS = INSTALL_CASE + '\t\tif a.options.Update { go installFast(a); return }\n'
ENTRY_CASES = [
    # Same-file and new-file helpers both evade checks of install/handoff bodies.
    ("A: install case adds same-file update bypass", INSTALL_CASE, CASE_BYPASS,
     "same", FAST_HELPER, "TestWindowsInstallMessageCannotBypassCompletion"),
    ("A: install case calls new-file update bypass", INSTALL_CASE, CASE_BYPASS,
     "new", FAST_HELPER, "TestWindowsInstallMessageCannotBypassCompletion"),
    ("A: update bypass before message dispatch", 'func (a *installerApp) onMessage(raw string) {\n',
     'func (a *installerApp) onMessage(raw string) {\n\tif a.options.Update && raw == `{"type":"install"}` { go installFast(a); return }\n',
     "new", FAST_HELPER, "TestWindowsInstallMessageCannotBypassCompletion"),
    ("A: WebView callback redirects update", 'w.dispatch(func() { a.onMessage(raw) })',
     'w.dispatch(func() { if a.options.Update && raw == `{"type":"install"}` { go installFast(a); return }; a.onMessage(raw) })',
     "new", FAST_HELPER, "TestWindowsInstallMessageCallbackUsesSharedDispatcher"),
    ("A: automatic update bypasses dispatcher", 'a.onMessage(`{"type":"install"}`)',
     'go installFast(a)', "new", FAST_HELPER, "TestWindowsInstallAutoUpdateUsesSharedDispatcher"),
    ("A: install call becomes normal-only", 'go a.install(message)',
     'if !a.options.Update { go a.install(message) }', None, "",
     "TestWindowsInstallMessageCannotBypassCompletion"),
    ("B: new update helper terminates application", INSTALL_CASE, CASE_BYPASS,
     "new", KILL_HELPER, "TestWindowsInstallMessageCannotBypassCompletion"),
]


def record_failure(results, name, outcome, expected=None, **evidence):
    failures = [line.strip() for line in outcome.stdout.splitlines() if "--- FAIL:" in line]
    if outcome.returncode == 0 or not failures or (expected and not any(expected in line for line in failures)):
        raise SystemExit(f"SURVIVED or failed outside expected assertions: {name}\n{outcome.stdout}")
    results.append({"mutation": name, "exit_code": outcome.returncode, "failed_tests": failures, **evidence})
    print(f"KILLED {name}: {'; '.join(failures)}", flush=True)


def main():
    env = dict(os.environ, GOCACHE=str(ROOT / ".gocache"), GOMODCACHE=str(ROOT / ".gomodcache"), GOTMPDIR=tempfile.gettempdir())
    # These must execute on the host; GOOS=windows would conceal the Unix proof.
    env.pop("GOOS", None)
    env.pop("GOARCH", None)
    results = []
    with tempfile.TemporaryDirectory(prefix="r82-hardening-") as directory:
        work = Path(directory) / "installer"
        shutil.copytree(ROOT / "installer", work, ignore=shutil.ignore_patterns("*.exe", "node_modules"))

        def run():
            return subprocess.run(["go", "test", "-count=1", "-timeout=30s", "-run", PATTERN, "."], cwd=work,
                                  env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=60)

        baseline = run()
        if baseline.returncode:
            raise SystemExit("BASELINE FAILED:\n" + baseline.stdout)
        print("PASS baseline (host tests, no GOOS override)", flush=True)

        # A new unused helper is a valid change: only wiring it into an entry
        # should fail. The guard must not just reject a filename/helper name.
        # Both caller and helper are Windows-only. Host tests parse the caller
        # as source, so no host compilation/linking of installFast is required.
        helper = work / "r82_bypass_windows.go"
        if helper.exists():
            raise SystemExit("Mutation helper path already exists")
        helper.write_text("package main\n" + FAST_HELPER)
        try:
            control = run()
        finally:
            helper.unlink()
        if control.returncode:
            raise SystemExit("UNUSED HELPER CONTROL FAILED:\n" + control.stdout)
        print("PASS negative control (new unused Windows helper)", flush=True)

        for name, filename, before, after in CASES:
            file = work / filename
            original = file.read_text()
            if original.count(before) != 1:
                raise SystemExit(f"Mutation target changed: {name}")
            file.write_text(original.replace(before, after))
            try:
                outcome = run()
            finally:
                file.write_text(original)
            record_failure(results, name, outcome)

        for name, before, after, location, helper_source, expected in ENTRY_CASES:
            file = work / "webview_windows.go"
            original = file.read_text()
            if original.count(before) != 1:
                raise SystemExit(f"Mutation target changed: {name}")
            changed = original.replace(before, after)
            if location == "same":
                changed += "\n" + helper_source
            file.write_text(changed)
            try:
                if location == "new":
                    helper.write_text("package main\n" + helper_source)
                # A malformed Windows mutation is not evidence of coverage.
                # vet type-checks Windows code but never runs the dangerous hook.
                vet = subprocess.run(["go", "vet", "./..."], cwd=work,
                                     env=dict(env, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0"),
                                     text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=60)
                if vet.returncode:
                    raise SystemExit(f"INVALID WINDOWS MUTATION: {name}\n{vet.stdout}")
                outcome = run()
            finally:
                file.write_text(original)
                if location == "new":
                    helper.unlink(missing_ok=True)
            record_failure(results, name, outcome, expected, windows_vet_exit=vet.returncode)

        restored = run()
        if restored.returncode:
            raise SystemExit("RESTORED BASELINE FAILED:\n" + restored.stdout)
        print("PASS restored baseline", flush=True)
    print(json.dumps(results, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
