#!/usr/bin/env python3
"""Actual HTTP manual reread mutation; isolated snapshot, restored in finally."""
import importlib.util
from pathlib import Path
spec=importlib.util.spec_from_file_location('r277_ci_proof',Path(__file__).with_name('r277-ci-proof.py'))
helper=importlib.util.module_from_spec(spec);spec.loader.exec_module(helper)
PATTERN='^TestR277ManualRefreshHTTPReconnectsNonReadyView$'
if __name__=='__main__':
 out=helper.arguments(__doc__)
 with helper.Proof(out,'r277-http-refresh-proof-cache') as proof:
  proof.run('fixed',PATTERN,race=True)
  proof.mutate('no-http-reconnect','backend/main.go',[( 'source != "dirty_rescan" && a.currentClientView().State != "ready" {', 'source != "dirty_rescan" && false && a.currentClientView().State != "ready" {')],PATTERN,['TestR277ManualRefreshHTTPReconnectsNonReadyView'])
  proof.run('restored',PATTERN,race=True)
