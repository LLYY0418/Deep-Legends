# 22:44 regression samples

Official centered artwork, fetched 2026-09-09 for visual regression checking:

- `ahri-centered.jpg`: https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/characters/ahri/skins/skin86/images/ahri_splash_centered_86.jpg
- `jayce-centered.jpg`: https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/characters/jayce/skins/base/images/jayce_splash_centered_0.jpg

- `rumble-centered.jpg` (added September 10): https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/characters/rumble/skins/base/images/rumble_splash_centered_0.jpg

These samples test the **centered** variant only. The GTIMG uncentered fallback
must not inherit these focal coordinates. All compositions now fill the whole
banner (September 10 feedback supersedes the former contain foreground).
Unknown faces use a fallback focal height, not a claim of automatic face detection.
Media remains Riot's artwork, not generated content.

## September 10, 10:42 regression additions

- `xerath-centered.jpg`: https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/characters/xerath/skins/base/images/xerath_splash_centered_0.jpg
- `xerath-ordinary.jpg`: https://game.gtimg.cn/images/lol/act/img/skin/big101000.jpg
- `ahri-ordinary.jpg`: https://ddragon.leagueoflegends.com/cdn/img/champion/splash/Ahri_86.jpg

`desktop/diagnostics-1042-render.cjs` checks both compositions, including ordinary
fallback without any catalogue metadata. The old centered-only fixture did not
exercise that production fallback. Known focal points are manually inspected;
this is not face detection or exhaustive coverage of every champion/skin.

Run `desktop/node_modules/.bin/electron desktop/diagnostics-2244-render.cjs` for
actual Chromium rendering at 920/1280/1800 CSS pixel widths and dark/light themes.
This is not Windows/LCU acceptance. It validates the static selected artwork;
animated playback additionally depends on the local client's media endpoint.

The Akari item set used for the exact authorized deletion regression is the
user-supplied `../diagnostics-1945/akari-user-verified.json`. Do not broaden it to
all Akari/OP.GG recommendations or versions.
