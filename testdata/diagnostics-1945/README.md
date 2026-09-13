# 2026-09-09 19:45 regression fixtures

- `yourgg-arena-rankings.json`: real read-only response from https://api.your.gg/kr/api/arena/champions, captured 2026-09-09; patch 16.17, 173 champions. Full response, not locally sorted. Contains S/A/B/C/D/F grades.
- `hall-of-legends-skins.json`: six complete entries selected by ID (67/103/145 base and HoL parent skins) from https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/skins.json, same date. Nested quest tiers and chromas unmodified. Vayne's chromas must not be promoted to quest skins.
- `akari-user-verified.json`: unmodified user-supplied `akari1-13-ranked-kr-all-top-16.17.json`, provided as a working game-file comparison. Tests check encoder byte parity, NOT Windows rendering. Contains only public item recommendations, no account/session identifiers.

Live network verification is opt-in: `DEEP_LEGENDS_1945_LIVE=1 go test -run Test1945LivePublicSources -v`.
