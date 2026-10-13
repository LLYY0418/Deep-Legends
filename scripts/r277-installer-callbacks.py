#!/usr/bin/env python3
"""P11-only local proof / native Windows race+GC mutation runner.

From repo root: python3 scripts/r277-installer-callbacks.py --out <evidence-dir>
On Windows add --native (requires the Go race detector's C compiler).
No production tree mutations: all unsafe signatures are restored in scratch copies.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
CALLBACKS = {
    "installer/window_windows.go": ("shellWndProc", ["uintptr", "uint32", "uintptr", "uintptr"], "Win32 WNDPROC: HWND, UINT, WPARAM and message-dependent LPARAM; RECT read only in WM_DPICHANGED"),
    "installer/uninstall/window_windows.go": ("shellWndProc", ["uintptr", "uint32", "uintptr", "uintptr"], "Win32 WNDPROC: HWND, UINT, WPARAM and message-dependent LPARAM; RECT read only in WM_DPICHANGED"),
    "installer/handoff_windows.go": ("inspectApplicationWindow", ["uintptr", "*windowSearch"], "EnumWindows synchronously echoes &search allocated in hasApplicationWindow"),
    "installer/update_parent_windows.go": ("inline", ["uintptr", "*legacyWindowOperation"], "EnumWindows synchronously echoes &operation allocated in legacyUpdateWindows"),
    "installer/dialog_windows.go": ("inline", ["uintptr", "uint32", "uintptr", "*uint16"], "SHBrowseForFolderW message LPARAM stays integer; initial is our UTF16PtrFromString allocation in browseInfo.Param; runtime.KeepAlive(initial) after modal call"),
    "installer/internal/webviewhost/surface_windows_test.go": ("inline COM fixtures", [], "COM this handle stays integer; NavigationResult supplies a live Go output allocation through comCall"),
    "installer/internal/webviewhost/settings_windows_test.go": ("fakeCOMObject.bind(callback)", ["uintptr", "(uintptr, *unsafe.Pointer)", "(uintptr, uintptr)"], "Release/Notify this and BOOL are scalars; GetCoreWebView2/GetSettings outputs are addresses of live local variables from coreFromController/configureController"),
    "installer/internal/p11callbacktest/fixture_windows.go": ("inline PutIsVisible", ["uintptr", "uintptr"], "COM this handle and BOOL are scalars; growStack+runtime.GC while actual shell callback LPARAM remains live"),
    "tools/window-focus-probe/main_windows.go": ("inline", ["uintptr", "uintptr", "uintptr", "uintptr"], "Win32 WNDPROC handles, message and WPARAM/LPARAM all scalar"),
    "tools/window-focus-probe/actor_windows.go": ("inline", ["uintptr", "uintptr", "uintptr", "uintptr"], "Win32 WNDPROC handles, message and WPARAM/LPARAM all scalar"),
    "backend/riot_launcher_focus_windows.go": ("inline", ["uintptr", "uintptr"], "EnumWindows HWND plus unused LPARAM; both scalar"),
}


def audit(out):
    rows = []
    # Audit every tracked/new Go source, without traversing local notes or generated outputs.
    listing = subprocess.run(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.go"],
                             cwd=ROOT, check=True, stdout=subprocess.PIPE).stdout.decode()
    for name in sorted(set(listing.split("\0")) - {""}):
        path = ROOT / name
        source = path.read_text()
        relative = path.relative_to(ROOT).as_posix()
        for number, line in enumerate(source.splitlines(), 1):
            if not re.search(r"(?:windows|syscall)\.NewCallback\(", line):
                continue
            if relative not in CALLBACKS:
                raise RuntimeError("unaudited callback: %s:%d" % (relative, number))
            function, types, proof = CALLBACKS[relative]
            signature = line.strip()
            if function in {"shellWndProc", "inspectApplicationWindow"}:
                signature = next(s for s in source.splitlines() if s.startswith("func " + function + "("))
            bindings = []
            if relative == "installer/internal/webviewhost/surface_windows_test.go":
                types = ["uintptr", "*int32"] if "*int32" in line else ["uintptr"]
                if "*int32" not in line:
                    proof = "COM this handle is scalar, with no pointer parameters"
                else:
                    proof += "; output is &success for get_IsSuccess or &status for get_WebErrorStatus (surface_windows.go:109-117)"
            if relative == "installer/internal/webviewhost/settings_windows_test.go":
                bindings = [dict(line=i, signature=s.strip()) for i, s in enumerate(source.splitlines(), 1)
                            if ".bind(t," in s and "func(" in s]
            rows.append(dict(path=relative, line=number, callback=function,
                             parameter_types=types, registration=line.strip(), signature=signature,
                             argument_source_and_pointer_proof=proof, dynamic_bindings=bindings,
                             sha256=hashlib.sha256(source.encode()).hexdigest()))
    rows.sort(key=lambda r: (r["path"], r["line"]))
    (out / "callback-audit.json").write_text(json.dumps(dict(callback_count=len(rows), callbacks=rows), indent=2) + "\n")
    if len(rows) != 14:
        raise RuntimeError("callback inventory changed: expected 14, got %d" % len(rows))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--native", action="store_true")
    args = parser.parse_args()
    if args.native and os.name != "nt":
        parser.error("--native requires actual Windows; cross-compilation is not native proof")
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    scratch = Path(os.environ.get("PI_SCRATCH_DIR") or os.environ.get("RUNNER_TEMP") or tempfile.gettempdir())
    scratch.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["GOCACHE"] = str(scratch / "r277-p11-gocache")
    env["GODEBUG"] = env.get("GODEBUG", "") + ",invalidptr=1"
    if args.native:
        # Never inherit a developer's cross-compilation or CGO-disabled settings.
        env.update(GOOS="windows", GOARCH="amd64", CGO_ENABLED="1")
    results = dict(host=os.name, native_windows_executed=args.native, gocache=env["GOCACHE"], runs=[])

    def run(label, command, cwd=ROOT / "installer", overrides=None, expect_failure=False, markers=()):
        started = time.monotonic()
        log_path = out / (label + ".log")
        try:
            completed = subprocess.run(command, cwd=cwd, env={**env, **(overrides or {})},
                                       stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=300)
        except subprocess.TimeoutExpired as error:
            output = error.stdout or ""
            if isinstance(output, bytes):
                output = output.decode(errors="replace")
            log_path.write_text("COMMAND: " + " ".join(command) + "\n" + output + "\nTIMEOUT (not mutation proof)\n")
            results["runs"].append(dict(label=label, command=command, timeout=True))
            raise
        log_path.write_text("COMMAND: " + " ".join(command) + "\n" + completed.stdout)
        row = dict(label=label, command=command, exit_code=completed.returncode,
                   expected_failure=expect_failure, seconds=round(time.monotonic() - started, 3))
        results["runs"].append(row)
        print(label, "exit=" + str(completed.returncode), flush=True)
        if expect_failure:
            if completed.returncode == 0 or any(m not in completed.stdout for m in markers):
                raise RuntimeError(label + ": mutation survived or failed for an unrelated reason")
        elif completed.returncode != 0:
            raise RuntimeError(label + ": see failure log")
        return completed.stdout

    try:
        audit(out)
        run("after-structural", ["go", "test", "-count=1", "-v", "-run", "^TestR277ShellCallbackScalarLPARAM$", "."])
        run("after-suite", ["go", "test", "-race", "-count=1", "./..."])
        win = {"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0"} if not args.native else {}
        run("after-windows-vet", ["go", "vet", "./..."], overrides=win)
        run("after-windows-unsafeptr", ["go", "vet", "-unsafeptr", "./..."], overrides=win)
        run("after-windows-license-vet", ["go", "vet", "-tags=license", "./..."], overrides=win)
        with tempfile.TemporaryDirectory(prefix="r277-p11-", dir=scratch) as temporary:
            temporary = Path(temporary)
            if not args.native:
                for label, package in [("installer", "."), ("uninstall", "./uninstall"), ("webviewhost", "./internal/webviewhost")]:
                    run("after-compile-" + label, ["go", "test", "-c", "-o", str(temporary / (label + ".test.exe")), package], overrides=win)
                run("after-compile-license", ["go", "test", "-tags=license", "-c", "-o", str(temporary / "license.test.exe"), "."], overrides=win)
            else:
                run("after-native-race", ["go", "test", "-race", "-count=1", "-v", "-run", "^TestR277ShellWndProcGCStress$", ".", "./uninstall"])
            copy = temporary / "installer"
            shutil.copytree(ROOT / "installer", copy)
            for relative in ["window_windows.go", "uninstall/window_windows.go"]:
                path = copy / relative
                fixed = path.read_text()
                anchor = "func shellWndProc(hwnd uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {"
                if fixed.count(anchor) != 1:
                    raise RuntimeError("unexpected signature anchor: " + relative)
                try:
                    path.write_text(fixed.replace(anchor, anchor.replace("lParam uintptr", "lParam unsafe.Pointer")))
                    label = "mutation-" + ("uninstall" if relative.startswith("uninstall/") else "installer")
                    run(label + "-structural", ["go", "test", "-count=1", "-v", "-run", "^TestR277ShellCallbackScalarLPARAM$", "."], cwd=copy, expect_failure=True,
                        markers=("parameter 3 must be uintptr", "FAIL"))
                    package = "./uninstall" if relative.startswith("uninstall/") else "."
                    if args.native:
                        # The actual runtime fatal, not merely the source-type guard, is required here.
                        run(label + "-native-race", ["go", "test", "-race", "-count=1", "-v", "-run", "^TestR277ShellWndProcGCStress$", package], cwd=copy, expect_failure=True,
                            markers=("shellWndProc", "invalid pointer found on stack"))
                    else:
                        run(label + "-windows-compile", ["go", "test", "-c", "-o", str(temporary / (label + ".test.exe")), package], cwd=copy, overrides=win)
                finally:
                    path.write_text(fixed)
        results["status"] = "PASS"
        results["native_limitation"] = None if args.native else "Mac local proof only: Windows callback execution, old-signature fatal GC crash, native race and real-device installer/uninstaller smoke remain pending."
    except Exception as error:
        results["status"] = "FAIL"
        results["error"] = str(error)
        raise
    finally:
        try:
            run("cache-clean", ["go", "clean", "-cache", "-testcache"])
        finally:
            (out / "results.json").write_text(json.dumps(results, indent=2) + "\n")


if __name__ == "__main__":
    main()
