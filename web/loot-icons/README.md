这些图标仅用于在本地助手中还原英雄联盟客户端的战利品显示。

来源：CommunityDragon 的 Riot 客户端资源镜像（`rcp-fe-lol-loot` 与
`rcp-fe-lol-static-assets`），下载于 2026-08-08。运行时不依赖网络，避免
国服客户端缺少 `/fe/` 静态资源路由时退化为统一占位图。

`hextech-chest-transparent.png` 由本地宝箱图标生成透明底变体，并压缩为
512 × 512；仅移除背景，不改变其在应用中的物品含义。

`promotion-chest.png` 来自 CommunityDragon 的
`rcp-fe-lol-loot/global/default/assets/loot_item_icons/chest_promotion.png`。
官方文件只有 303 × 303 RGB 版本，因此本地仅移除连续的深青背景并等比放入
512 × 512 RGBA 透明画布，宝箱主体未重绘。

League of Legends 与相关素材归 Riot Games / 腾讯所有；本项目不受其背书。
