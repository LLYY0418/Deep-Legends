# R282 recorded public esports fixtures

Captured on 2026-10-17 from Riot official persisted esports and livestats endpoints. These are public tournament data; no private Riot developer key or local client identity is included.

- `worlds-event.json` / `worlds-window.json`: 2025 Worlds MKOI vs KT, event 113475798006664574, game 113475798006664575. Window retains original game metadata and first frame.
- `playin-schedule.json` / `playin-event.json`: completed **2024 Worlds Play-Ins**, event 112966697102367347, game 112966697102367348, scheduled 2024-09-25 12:00 UTC. Official pagination supplied the original event, without changing dates or labels. Both team IDs (107251245690956393 and 103461966965149786) are outside the old six-team map. The current official event API calls them MVK and MKOI; historical participant metadata still says VKE and MDK. Tests retain those original responses.
- `playin-window.json`, `playin-end.json`, `playin-details.json`: original first frame (12:12:37.279 UTC), terminal frame (12:49:40.456 UTC), and terminal participant/perk data from `https://feed.lolesports.com/livestats/v1/{window,details}/112966697102367348`. End time parameters are rounded to the API-required 10-second boundary.
- `playin-champions.json`: the ten participants' champion IDs, keys and Chinese names from Riot Data Dragon 14.18.1.
- `playin-pro-ui.json`: **derived by the production recommendation provider** from the above captured terminal/perk data. Requested champion K'Sante (897), requested lane middle; the recorded top-lane game still produces a rune card. Browser screenshots use this output, not a fabricated tournament card.

The provider test clock is set from the original recorded frame to test its unchanged 30-day freshness window. This is a historical replay, not a claim of a completed 2026 play-in. Reproduce the derived UI fixture with:

```sh
R282_PRO_UI_OUTPUT=backend/testdata/r282/playin-pro-ui.json go test ./backend -run '^TestR282RecordedPlayinProducesCrossLaneRuneCard$' -count=1
```

`TestR282RecordedWorldsIndex` exercises both recordings; separate deterministic tests cover all five leagues, arbitrary numeric team IDs, cross-lane timestamp ordering, team abbreviations and diagnostic counts.
