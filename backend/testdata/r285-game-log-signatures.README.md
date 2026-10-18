# R285 game-log fixture provenance

`r285-game-log-signatures.json` contains only the six r3dlog tail blocks (22 lines each) and netlog tail blocks (3 lines each) supplied in the user's sanitized `docs/history/reports/r285-inputs/riot-logs/game-log-signatures.md` on 2026-10-18. Blocks are copied verbatim, preserving order and newlines. Directory names come from the same source. No inferred log messages are included.

The kill sample also contains `Quit Game Confirmed`; it has none of the closing records present in the other samples. Window X/Alt+F4 and quitting from the loading screen have no samples and no additional classification rules.
