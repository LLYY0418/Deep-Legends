# R285 game-log fixture provenance

`r285-game-log-signatures.json` contains only the six r3dlog tail blocks (22 lines each) and netlog tail blocks (3 lines each) supplied in the user's sanitized `docs/history/reports/r285-inputs/riot-logs/game-log-signatures.md` on 2026-10-18. Blocks are copied verbatim, preserving order and newlines. Directory names come from the same source. No inferred log messages are included.

The kill sample also contains `Quit Game Confirmed`; it has none of the closing records present in the other samples. Window X/Alt+F4 and quitting from the loading screen have no samples and no additional classification rules.

R286 adds the 2026-10-18T22-54-39 window-close/Alt+F4 and 2026-10-18T22-58-32 taskkill samples from `docs/history/reports/r286-inputs/game-log-signatures-r286.md`. Each new sample concatenates its supplied key-lines block and 22-line tail, plus the supplied three-line netlog tail. These are real sanitized excerpts; no generated log messages. The former is voluntary (`game-exit-early`); the latter lacks Quit Game Confirmed and is truncated (`game-truncated`). The erroneous `,X.X.X.X,…` suffixes on old Memory Allocations/logfile stopped lines are removed. Loading-screen quit remains untested.
