# Automatic accept foreground investigation

This change only observes. It does not claim to prevent client activation or
identify the internal instruction which caused it. No window mutation, hooks,
injection, simulated input, configuration writes or arbitrary endpoint crawling.

## Automatic capture

When automatic accept is armed, `accept_focus_trace` creates a unique trace ID.
The accept request's before/after `watch_accept_window` observations now share it.
`accept_focus_request` records the actual request status (including successful
204), success/error class and duration immediately, not only in 10-second HTTP
aggregate buckets. Status 0 means no HTTP status was observed.

For a connected client with diagnostic recording, one sampler per client runs
for at most 20 seconds, every 200ms. It records `accept_focus_window`: foreground
category/change/original-window comparison; League/Riot/game window visibility,
minimized state, topmost state, bounding rectangle and sampled Z-order rank.
Traversal is bounded to 256 windows and 16 relevant windows. Last-input age is a
timestamp age only, not key/button contents. Handles, PIDs, process paths, window
titles, screenshots, keystrokes and chat contents are not serialized.

A separate bounded GET loop records `accept_focus_phase` once per second with
600ms request deadlines. Sampling does not sit in the accept-request path. Context
cancellation and lost credentials stop the capture; overlapping captures are
explicitly reported as skipped. Completion includes sample/change counts.

Limitations: sampled state is not a Win32 call stack, not every activation event,
and not an atomic desktop snapshot. Sub-200ms changes can be missed. A last-input
age cannot prove whether input was manual or synthetic. No identifying window
text is collected to resolve these ambiguities.

## Export-time read-only inspection

The existing export button additionally probes `/Help?format=Full` and two fixed
local settings categories (`lol-user-experience`, `lol-notifications`) with a
5-second overall network budget. `/Help` spelling/format is grounded in
https://github.com/MingweiSamuel/lcu-schema/blob/master/update.ps1 ; endpoint
availability on the user's Tencent client is not assumed. Failed probes log their
HTTP status. Candidate symbols are not presented as verified configuration flags.

Only the three named YAML files in the client installation's Config are inspected;
not Game/Config and not whole-directory collection. Symlink escapes are rejected.
Each file is capped at 256KiB. The lightweight parser captures plain YAML key
structure, not a complete YAML schema. Authentication/identity/session branches
are removed, all string values omitted, and only relevant boolean/numeric setting
values retained. This specifically excludes lastSessionId JWTs. API strings and
help descriptions/examples are likewise omitted. Structural caps are explicit.

Export snapshots use the latest trace ID and `snapshot_timing: export-not-at-accept`:
they must not be mistaken for settings captured at the exact activation instant.
No full Help response or preference file is persisted.

## Reproduction

Run the newly built Windows application, enable automatic accept, and put another
application in front. Let one normal match auto-accept without manually accepting
or deliberately switching windows. After at least 20 seconds, export diagnostics.
The user should not share raw authentication-bearing preference files again.

Tests cover shared trace/exact successful HTTP status, cancellation/single-flight,
read-only export probes, preference/API/help secret suppression and absence of
private paths. Native observation still requires Windows runtime validation.

## Extended coverage

Before websocket dispatch, a 128-entry in-memory ring retains only fixed phase,
ready-check and champion-select event paths, whitelisted phase/response values,
timestamps and foreground categories. No raw event bodies are retained. Trace
start and export include this history and its eviction count. History timestamps
are observation times; the enclosing trace timestamp is the snapshot time.

Matchmaking also starts a non-blocking, single-flight read-only settings inspection
with `snapshot_timing: matchmaking-not-at-accept`. Its trace ID may refer to the
previous accept: correlate by observation time, not solely by that ID. Export
still reads settings again, allowing comparison without changing the files.

Every armed accept emits `accept_focus_action_end`, including delay cancellation,
custom-lobby suppression, stale ready-check, request failure and success. Request
attempted means the watch request wrapper was entered; status zero is not proof
that a packet reached the server. Samplers record disabled/overlap reasons, an
immediately scheduled baseline, unavailable sample counts, maximum sample gap,
actual duration, phase-read error class and available HTTP failure status.
Windows snapshots expose last-input/rectangle availability and unclassified
window counts rather than silently treating missing observations as evidence.

Inspection records safe error classes for missing/unreadable files, permission
failures, unsafe paths and request timeouts, plus overall budget exhaustion.
Help candidates include whitelisted method/primitive type/boolean-numeric default
metadata when exposed; missing defaults do not imply false. Hidden internal
defaults and call stacks remain unavailable. No arbitrary endpoint is invoked.

Exports now include both retained log generations in chronological order, under
the rotation lock. This preserves the preceding segment across one rotation;
it is not unlimited retention. Export promptly after the reproduction.
