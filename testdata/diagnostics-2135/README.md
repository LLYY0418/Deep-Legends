# 2026-09-09 21:35 report regression evidence

- `current-action.json`: approved data fields selected from the real read-only `getInGameInfo` Flight response at https://op.gg/zh-cn/lol/summoners/kr/Maldives-0727 on 2026-09-09. Capture reports start `2026-09-09T22:47:41+09:00`, empty `game_id`, `record_info.is_finished: "$undefined"`, and 3 blue + 5 red participants, including the queried player. Identifiers/names/tags were replaced with fixtures; spectator scripts and unrelated fields were removed. Champion IDs were cross-checked against the already-captured Meraki catalogue, not assigned sequentially. No missing participants were manufactured.
- `criticalmissile-large.png`: unchanged Riot game artwork via https://raw.communitydragon.org/latest/game/assets/ux/kiwi/augments/icons/criticalmissile_large.png; pale gold center and muted olive rim are the reference for neutral-glyph tone mapping. Bonk/Drop Bear inputs remain in `../diagnostics-2024/`.

`Test2135ObservedCurrentActionUndefinedEmptyIDPartialRoster` exercises the actual response variants. `OPGG_2135_PROBE=1 go test -run Test2135LivePublicCurrentGame -v` performs an optional real public request; normal tests never depend on a player remaining in a game.

`desktop/diagnostics-2135-render.cjs` explicitly runs Chromium PNG rendering, dark/light theme color resolution and name/stat geometry. This is not Windows League shop or installer UI acceptance.
