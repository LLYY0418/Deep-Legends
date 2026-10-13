#!/usr/bin/env python3
"""Quota-overflow credential scope: fixed + actual mutants + restored race, portable snapshots."""
import importlib.util
import sys
from pathlib import Path
spec=importlib.util.spec_from_file_location('r277_ci_proof',Path(__file__).with_name('r277-ci-proof.py'))
helper=importlib.util.module_from_spec(spec);spec.loader.exec_module(helper)
AUDIT=('^TestR277Scope|^TestR277(HiddenDirectoryOverviewProgress|OverviewYieldBounded|'
 'InternalWaitTimeoutNotNetwork|ForegroundQuotaOverflow|OverflowRelay429Countdown|'
 'BackgroundNeverQuotaOverflows|UnhealthyRelayKeepsLocalCountdown|TimeRangeIDsBoundary|'
 'EmbeddedHeaderHintCannotPinRelayAfterRecovery|OverflowDoesNotJoinBlockedLocalFIFO|'
 'MethodQuotaDoesNotOverflow|ControlledClockOverflowNoLocalWait)|^TestR273Foreground|'
 '^TestR269(AccountFallbackResolvesInRelayScope|DecryptRecoveryKeepsReturnedCredentialScope|IdentityPinsKeyDuringLookup)')
if __name__=='__main__':
 out=helper.arguments(__doc__)
 with helper.Proof(out,'r277-riot-scope-proof-cache') as proof:
  proof.run('fixed',AUDIT)
  proof.mutate('transparent-account-overflow','backend/riot_api.go',[( 'if errors.Is(err, errRiotQuotaOverflow) && !accountBoundary {','if errors.Is(err, errRiotQuotaOverflow) && (accountBoundary || !accountBoundary) {')],'^TestR277Scope(AccountQuotaOverflow|PinnedAccountCannotTransparentlyOverflow)$',['TestR277ScopeAccountQuotaOverflow/user/disk=true','TestR277ScopeAccountQuotaOverflow/embedded/disk=true','TestR277ScopePinnedAccountCannotTransparentlyOverflow/user','TestR277ScopePinnedAccountCannotTransparentlyOverflow/embedded'])
  proof.mutate('relay-account-labeled-direct','backend/riot_api.go',[( 'account.scope = riotCredentialScope(ctx)','account.scope = riotCredentialScope(context.WithValue(ctx, riotRouteKey{}, "direct"))')],'^TestR277ScopeAccountQuotaOverflow$',['TestR277ScopeAccountQuotaOverflow/user/disk=true','TestR277ScopeAccountQuotaOverflow/embedded/disk=true'])
  proof.run('restored',AUDIT,race=True)
