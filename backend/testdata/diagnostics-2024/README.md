# 2026-09-09 20:24 regression fixtures

Fetched from the public source on 2026-09-09; no account data or credentials.

- `rarity.html`: https://hexdata.com.cn/augment-rarity — Patch 16.17, canonical Dataset `dateModified=2026-08-29`. The four rows are **selected-augment distributions**, not offer/refresh/pity probabilities. Preserve the page's attribution and measurement limitations.
- `bonk-large.png`: https://raw.communitydragon.org/latest/game/assets/ux/kiwi/augments/icons/bonk_large.png — actual neutral teal/gray 256px glyph. Metadata ID 2111, `kGold`. HTTP 200 and `_large` do not imply pre-colored artwork.
- `drop-bear.png`: https://raw.communitydragon.org/latest/game/assets/ux/cherry/augments/icons/drop_bear.png — already-colored prismatic artwork. Must remain unchanged by neutral-glyph presentation.

Images are unmodified source assets used solely to test rendering and colored-artwork protection. Rarity colors are a presentation layer, not fabricated source images or rarity data.

`desktop/diagnostics-2024-render.cjs` runs explicitly under Electron and measures both teams' seven columns at 1360/1024/920/640px, tests equipment overflow, and checks actual PNG pixels for Bonk/Drop Bear. The Node suite does not pretend JSDOM supplies layout geometry.
