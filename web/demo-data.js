// 演示数据预览层（仅在显式开启时生效，用于未登录客户端时查看页面样式）。
// 开启方式：URL 追加 ?demo / #demo，或 localStorage.setItem("lol-loot-demo","1")。
// 所有数据均为虚构示例，界面右下角会常驻「演示数据」角标以避免误认。
(() => {
  "use strict";
  const query = new URLSearchParams(location.search);
  const enabled = query.has("demo") || location.hash.includes("demo") || (() => {
    try { return localStorage.getItem("lol-loot-demo") === "1"; } catch (_) { return false; }
  })();
  if (!enabled) return;
  const demoCatalogFailure = query.get("demoCatalogFailure") === "1";
  // `?demo=arena` keeps the normal demo intact while exposing the live Arena
  // recommendation layout for visual review. `?demo=arena-full` shows the
  // defensive 6 x 3 in-progress grouping. Both are intentionally demo-only.
  const arenaLiveDemo = query.get("demo") === "arena" || query.has("demoArena") || location.hash.includes("arena");
  const arenaFullDemo = query.get("demo") === "arena-full" || query.has("demoArenaFull") || location.hash.includes("arena-full");
  const hextechLiveDemo = query.get("demo") === "hextech" || query.has("demoHextech") || location.hash.includes("hextech");
  const currentGameDemo = query.get("demo") === "current-game" || query.has("demoCurrentGame") || location.hash.includes("current-game");
  try { if (query.has("demo") || location.hash.includes("demo")) localStorage.setItem("lol-loot-demo", "1"); } catch (_) {}

  const now = Date.now();
  const iso = (minutesAgo) => new Date(now - minutesAgo * 60_000).toISOString();

  /* ---------- 通用素材映射（把 LCU 图标路径改写到本地代理的公共 CDN 源） ---------- */
  const PATCH = "16.16.1";
  const championKeys = {
    103: "Ahri", 222: "Jinx", 157: "Yasuo", 92: "Riven", 145: "Kaisa", 99: "Lux",
    412: "Thresh", 64: "LeeSin", 164: "Camille", 81: "Ezreal", 24: "Jax", 254: "Vi",
    238: "Zed", 111: "Nautilus", 266: "Aatrox", 517: "Sylas", 875: "Sett", 39: "Irelia",
    121: "Khazix", 61: "Orianna", 51: "Caitlyn", 235: "Senna", 799: "Ambessa",
  };
  const spellNames = { 1: "SummonerBoost", 3: "SummonerExhaust", 4: "SummonerFlash", 6: "SummonerHaste", 7: "SummonerHeal", 11: "SummonerSmite", 12: "SummonerTeleport", 14: "SummonerDot", 21: "SummonerBarrier", 32: "SummonerSnowball" };
  const perkPaths = {
    8005: "/cdn/img/perk-images/Styles/Precision/PressTheAttack/PressTheAttack.png",
    8010: "/cdn/img/perk-images/Styles/Precision/Conqueror/Conqueror.png",
    8128: "/cdn/img/perk-images/Styles/Domination/DarkHarvest/DarkHarvest.png",
    8437: "/cdn/img/perk-images/Styles/Resolve/GraspOfTheUndying/GraspOfTheUndying.png",
    8351: "/cdn/img/perk-images/Styles/Inspiration/GlacialAugment/GlacialAugment.png",
    5001: "/cdn/img/perk-images/StatMods/StatModsHealthPlusIcon.png",
    5005: "/cdn/img/perk-images/StatMods/StatModsAttackSpeedIcon.png",
    5007: "/cdn/img/perk-images/StatMods/StatModsCDRScalingIcon.png",
    5008: "/cdn/img/perk-images/StatMods/StatModsAdaptiveForceIcon.png",
    5010: "/cdn/img/perk-images/StatMods/StatModsMovementSpeedIcon.png",
    5011: "/cdn/img/perk-images/StatMods/StatModsHealthScalingIcon.png",
    5013: "/cdn/img/perk-images/StatMods/StatModsTenacityIcon.png",
  };
  const proxied = (path) => `/api/champion-asset?source=ddragon&path=${encodeURIComponent(path)}`;

  function rewriteImage(image) {
    const src = image.getAttribute("src") || "";
    if (!src.startsWith("/api/image?path=")) return;
    const assetPath = decodeURIComponent(src.slice("/api/image?path=".length));
    let match = assetPath.match(/champion-icons\/(\d+)\.png$/);
    if (match) {
      // 走和真实生产环境完全相同的资源形状（CommunityDragon 镜像的
      // rcp-be-lol-game-data 方形头像，backend 在客户端未连接/资源缺失时用的就是这一份），
      // 而不是掺进英雄页选用的 gtimg 头像特写图。两者裁切构图不一样：gtimg 那张本身
      // 就已经是接近头肩特写的紧凑构图，如果演示数据用它，再叠加英雄头像暗角裁切的
      // CSS 缩放就会二次收紧、把脸切没了——演示模式必须能如实反映真实资源，
      // 否则会把只在演示数据里才出现的构图问题误判成产品缺陷。
      image.src = `/api/champion-asset?source=communitydragon&path=${encodeURIComponent(`/latest/plugins/rcp-be-lol-game-data/global/default/v1/champion-icons/${match[1]}.png`)}`;
      return;
    }
    match = assetPath.match(/profile-icons\/(\d+)\.jpg$/);
    if (match) { image.src = proxied(`/cdn/${PATCH}/img/profileicon/${match[1]}.png`); return; }
    match = assetPath.match(/summoner-spells\/(\d+)\.png$/);
    if (match && spellNames[match[1]]) { image.src = proxied(`/cdn/${PATCH}/img/spell/${spellNames[match[1]]}.png`); return; }
    match = assetPath.match(/items\/(\d+)\.png$/);
    if (match) { image.src = proxied(`/cdn/${PATCH}/img/item/${match[1]}.png`); return; }
    const perkImages = assetPath.indexOf("/perk-images/");
    if (perkImages >= 0) { image.src = proxied(`/cdn/img${assetPath.slice(perkImages)}`); return; }
    match = assetPath.match(/demo-spell\/([A-Za-z0-9]+\.png)$/);
    if (match) { image.src = proxied(`/cdn/${PATCH}/img/spell/${match[1]}`); return; }
    match = assetPath.match(/perks\/(\d+)\.png$/);
    if (match && perkPaths[match[1]]) { image.src = proxied(perkPaths[match[1]]); }
  }

  const observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.type === "attributes" && mutation.target.tagName === "IMG") rewriteImage(mutation.target);
      for (const node of mutation.addedNodes || []) {
        if (node.nodeType !== 1) continue;
        if (node.tagName === "IMG") rewriteImage(node);
        for (const image of node.querySelectorAll?.("img") || []) rewriteImage(image);
      }
    }
  });
  observer.observe(document.documentElement, { subtree: true, childList: true, attributes: true, attributeFilter: ["src"] });

  /* ---------- 演示数据 ---------- */
  const summoner = {
    displayName: "青钢影不加班", gameName: "青钢影不加班", tagLine: "CN1", profileIconId: 4568, summonerLevel: 436,
    backgroundSource: "ddragon", backgroundPath: "/cdn/img/champion/splash/Camille_0.jpg",
  };

  const status = {
    version: "demo", connected: true, snapshotReady: true, connectionState: "connected", eventStream: true,
    syncing: false, lastSync: iso(3), lastAttempt: iso(3), summoner,
    ownedCount: 312, chromaOwnedCount: 87, poolTotal: 554, poolMatched: 554, remainingCount: 173,
    calculationOK: true, poolSource: "内置奖池（演示）", poolVersion: "14.5", poolId: "builtin", poolHash: "demo000000000000", storageReady: true,
  };

  const skin = (id, name, championId, championName, rarity, owned, extra = {}) => ({
    id, name, championId, championName, rarity, owned,
    splashPath: `/lol-game-data/assets/demo/${id}/splash.jpg`,
    tilePath: `/lol-game-data/assets/demo/${id}/tile.jpg`,
    championMasteryPoints: extra.mastery || 120_000, championMasteryLevel: extra.masteryLevel || 7,
    acquiredAt: extra.acquiredAt || "", poolName: extra.poolName || "", isLegacy: Boolean(extra.legacy),
  });
  const ownedSkins = [
    skin(103015, "K/DA 阿狸", 103, "阿狸", "kEpic", true, { acquiredAt: "2023-06-12T10:00:00Z", mastery: 486_200 }),
    skin(103028, "灵魂莲华 阿狸", 103, "阿狸", "kLegendary", true, { acquiredAt: "2024-01-28T10:00:00Z", mastery: 486_200 }),
    skin(103007, "偶像歌手 阿狸", 103, "阿狸", "kEpic", true, { acquiredAt: "2022-09-01T10:00:00Z", mastery: 486_200 }),
    skin(222004, "星之守护者 金克丝", 222, "金克丝", "kLegendary", true, { acquiredAt: "2022-11-05T10:00:00Z", mastery: 355_400 }),
    skin(222002, "爆竹 金克丝", 222, "金克丝", "kEpic", true, { acquiredAt: "2023-02-14T10:00:00Z", mastery: 355_400 }),
    skin(222013, "奥德赛 金克丝", 222, "金克丝", "kEpic", true, { acquiredAt: "2024-05-20T10:00:00Z", mastery: 355_400 }),
    skin(157009, "夜幽 亚索", 157, "亚索", "kLegendary", true, { acquiredAt: "2023-08-08T10:00:00Z", mastery: 268_000 }),
    skin(92016, "灵魂莲华 锐雯", 92, "锐雯", "kEpic", true, { acquiredAt: "2024-03-03T10:00:00Z", mastery: 199_500 }),
    skin(145014, "K/DA ALL OUT 卡莎", 145, "卡莎", "kLegendary", true, { acquiredAt: "2023-12-24T10:00:00Z", mastery: 158_700 }),
    skin(64012, "神龙尊者 李青", 64, "李青", "kLegendary", true, { acquiredAt: "2022-05-11T10:00:00Z", mastery: 142_300 }),
    skin(164001, "钢铁军团 卡蜜尔", 164, "卡蜜尔", "kEpic", true, { acquiredAt: "2024-07-15T10:00:00Z", mastery: 132_800 }),
    skin(412001, "深海 锤石", 412, "锤石", "kEpic", true, { acquiredAt: "2021-12-01T10:00:00Z", mastery: 96_400 }),
  ];
  const unownedSkins = [
    skin(103085, "永猎双子 阿狸", 103, "阿狸", "kMythic", false, { poolName: "三合一奖池" }),
    skin(99007, "光明使者 拉克丝", 99, "拉克丝", "kUltimate", false, {}),
    skin(81005, "未来战士 伊泽瑞尔", 81, "伊泽瑞尔", "kEpic", false, { poolName: "三合一奖池" }),
    skin(157001, "高原血统 亚索", 157, "亚索", "kEpic", false, {}),
  ];

  const chromas = [
    { id: 222902, name: "爆竹 金克丝 红宝石", parentSkinId: 222002, parentSkinName: "爆竹 金克丝", championId: 222, championName: "金克丝", owned: true, colors: ["#C43C3C", "#7B1F1F"], tilePath: "/lol-game-data/assets/demo/222902/tile.jpg", championMasteryPoints: 355_400 },
    { id: 222903, name: "爆竹 金克丝 青玉", parentSkinId: 222002, parentSkinName: "爆竹 金克丝", championId: 222, championName: "金克丝", owned: false, colors: ["#2C8C7C", "#134A42"], tilePath: "/lol-game-data/assets/demo/222903/tile.jpg", championMasteryPoints: 355_400 },
    { id: 10082, name: "K/DA ALL OUT 阿狸 臻藏版", parentSkinId: 103071, parentSkinName: "K/DA ALL OUT 阿狸", championId: 103, championName: "阿狸", owned: true, isPrestige: true, prestigeImageId: "demo", colors: [], tilePath: "/lol-game-data/assets/demo/10082/tile.jpg", championMasteryPoints: 486_200 },
  ];

  const account = {
    summoner,
    account: {
      profile: { backgroundSkinId: 103028, backgroundSkinName: "灵魂莲华 阿狸" },
      sanctumSparksKnown: true, sanctumSparks: 120,
      loot: [
        { lootId: "CHEST_CHAMPION_MASTERY", displayName: "战利品宝箱", category: "材料", kind: "宝箱", count: 4 },
        { lootId: "CHEST_224", displayName: "杰作宝箱", category: "材料", kind: "宝箱", count: 1 },
        { lootId: "CHEST_PROMOTION", displayName: "紫色宝箱", category: "材料", kind: "宝箱", count: 1 },
        { lootId: "CHEST_GENERIC", displayName: "海克斯科技宝箱", category: "材料", kind: "宝箱", count: 1 },
        { lootId: "MATERIAL_KEY_FRAGMENT", displayName: "钥匙碎片", category: "材料", kind: "材料", count: 7 },
        { lootId: "MATERIAL_KEY", displayName: "海克斯钥匙", category: "材料", kind: "材料", count: 6 },
        { lootId: "CURRENCY_CHAMPION", displayName: "蓝色精粹", category: "材料", kind: "货币", count: 48_230 },
        { lootId: "CURRENCY_COSMETIC", displayName: "橙色精粹", category: "材料", kind: "货币", count: 3_845 },
        { lootId: "CHAMPION_RENTAL_266", displayName: "亚托克斯 英雄碎片", category: "英雄", kind: "英雄碎片", count: 2, disenchantValue: 270 },
        { lootId: "CHAMPION_RENTAL_517", displayName: "塞拉斯 英雄碎片", category: "英雄", kind: "英雄碎片", count: 1, disenchantValue: 270 },
        { lootId: "SKIN_RENTAL_103071", displayName: "K/DA ALL OUT 阿狸", category: "皮肤", kind: "皮肤碎片", count: 1, upgradeEssenceValue: 1520, skinOwnedKnown: true, skinOwned: false },
        { lootId: "SKIN_RENTAL_157009", displayName: "夜幽 亚索", category: "皮肤", kind: "皮肤碎片", count: 1, disenchantValue: 364, skinOwnedKnown: true, skinOwned: true },
        { lootId: "COMPANION_DEMO", displayName: "噬齿獾 彩蛋", category: "小小英雄", kind: "彩蛋", count: 2 },
        { lootId: "STATSTONE_DEMO", displayName: "卡蜜尔 系列一", category: "永恒星碑", kind: "星碑", count: 1 },
        { lootId: "EMOTE_DEMO", displayName: "「稳住别浪」", category: "表情", kind: "表情", count: 1 },
        { lootId: "WARD_DEMO", displayName: "灵魂莲华守卫", category: "守卫", kind: "守卫皮肤", count: 1 },
        { lootId: "ICON_DEMO", displayName: "2026 季前赛图标", category: "图标", kind: "召唤师图标", count: 1 },
      ],
      rewards: [
        { title: "排位赛季奖励", status: "PENDING", dateCreated: iso(60 * 24), items: [{ title: "翡翠段位边框", quantity: 1 }], description: "2026 赛季 S1 · 单双排" },
        { title: "事件通行证", status: "PENDING", dateCreated: iso(60 * 48), items: [{ title: "海克斯宝珠", quantity: 2 }], description: "灵魂莲华 2026 通行证" },
        { title: "荣誉等级 4", status: "PENDING", dateCreated: iso(60 * 96), items: [{ title: "荣誉胶囊", quantity: 1 }] },
      ],
      capabilities: [
		{ name: "loot", state: "available", count: 16, detail: "本机客户端已返回数据" },
        { name: "rewards", state: "available", count: 3, detail: "本机客户端已返回数据" },
        { name: "store", state: "unsupported", count: 0, detail: "当前客户端不支持，已降级" },
      ],
    },
  };

  const participant = (participantId, teamId, championId, championName, kills, deaths, assists, win) => ({
	participantId, teamId, championId, championName, championLevel: 15 + (participantId % 4),
	position: ["top", "jungle", "middle", "bottom", "utility"][(participantId - 1) % 5],
    spell1Id: 4, spell2Id: teamId === 100 ? 12 : 14,
    primaryStyleId: participantId % 2 ? 8000 : 8100, subStyleId: 8400,
    perkIds: participantId % 2 ? [8010, 9111, 9104, 8299, 8446, 8453, 5005, 5008, 5011] : [8128, 8139, 8138, 8135, 8473, 8453, 5008, 5008, 5001],
    kills, deaths, assists, kda: deaths ? Number(((kills + assists) / deaths).toFixed(2)) : kills + assists,
    cs: 180 + participantId * 7, laneCs: 150 + participantId * 6, jungleCs: 30 + participantId,
    csPerMinute: Number((5 + ((participantId * 7) % 20) / 10).toFixed(1)), damage: 14_000 + participantId * 2_300, damageTaken: 12_000 + ((participantId * 3_137) % 14_000),
    gold: 8_800 + participantId * 620 + (win ? 1_400 : 0), visionScore: 11 + ((participantId * 13) % 34),
	wardsPlaced: 7 + (participantId % 9), wardsKilled: 1 + (participantId % 6), controlWardsBought: (participantId * 3) % 7,
	itemIds: [6630, 3071, 3053, 3065, 3026, 0, 3340],
    playerRef: "", gameName: `演示玩家${participantId}`, tagLine: "DEMO", multiKill: participantId === 1 ? 3 : 0, win,
  });
  const demoModeParticipant = (item, index, mode) => {
    const augmentIDs = (mode.augmentIds || []).map(Number).filter((id) => id > 0);
    if (!augmentIDs.length) return item;
    const { primaryStyleId, subStyleId, perkIds, ...withoutRunes } = item;
    return {
      ...withoutRunes,
      augmentIds: augmentIDs.map((_, offset) => augmentIDs[(index + offset) % augmentIDs.length]),
    };
  };
  const demoMatch = (gameId, minutesAgo, win, subjectChampion, lpDelta, mode = {}) => ({
    gameId,
    queueId: mode.queueId ?? 420,
    queueLabel: mode.queueLabel || "单排/双排",
    modeGroup: mode.modeGroup || "solo",
    gameMode: mode.gameMode || "CLASSIC",
    gameType: "MATCHED_GAME",
    mapId: mode.mapId ?? 11,
    result: win ? "win" : "loss",
    createdAt: now - minutesAgo * 60_000, duration: mode.duration || 1_820, subjectParticipantId: 1,
    ...(lpDelta === undefined ? {} : { lpDelta }),
    averageTier: { tier: "EMERALD", division: "II", samples: 10 },
    teams: [
      { teamId: 100, win, kills: 32, gold: 61_400, damage: 128_500, visionScore: 187, damageTaken: 96_300, cs: 985 },
      { teamId: 200, win: !win, kills: 21, gold: 54_800, damage: 104_200, visionScore: 152, damageTaken: 112_800, cs: 921 },
    ],
    participants: [
      participant(1, 100, subjectChampion[0], subjectChampion[1], 12, 3, 9, win),
      participant(2, 100, 64, "李青", 6, 4, 11, win),
      participant(3, 100, 103, "阿狸", 8, 2, 7, win),
      participant(4, 100, 222, "金克丝", 4, 5, 12, win),
      participant(5, 100, 412, "锤石", 2, 6, 18, win),
      participant(6, 200, 24, "贾克斯", 7, 6, 4, !win),
      participant(7, 200, 254, "蔚", 4, 7, 8, !win),
      participant(8, 200, 238, "劫", 6, 5, 3, !win),
      participant(9, 200, 145, "卡莎", 3, 6, 5, !win),
      participant(10, 200, 111, "深海泰坦", 1, 8, 10, !win),
    ].map((item, index) => demoModeParticipant(item, index, mode)),
  });
  const demoRemakeMatch = (gameId, minutesAgo, subjectChampion) => {
    const match = demoMatch(gameId, minutesAgo, false, subjectChampion, undefined, { duration: 185 });
    match.result = "remake";
    return match;
  };
  // 斗魂竞技场演示对局：21 名玩家、3 人一队共 7 支小队，带名次。
  // ID、品质与图标路径来自 Riot 的 cherry-augments 中文目录；描述仅用于演示 tooltip。
  const arenaAugmentCatalog = [
    { id: 1205, name: "物理转魔法", rarity: "kSilver", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/ADAPt_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/ADAPt_small.png", description: "将额外攻击力转化为法术强度。" },
    { id: 1141, name: "全心为你", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/AllForYou_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/AllForYou_small.png", description: "强化你为友军提供的治疗与护盾效果。" },
    { id: 1002, name: "尖端发明家", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/ApexInventor_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/ApexInventor_small.png", description: "你的装备技能获得大量技能急速。" },
    { id: 2087, name: "大法师", rarity: "kPrismatic", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_small.png", description: "根据最大法力值获得额外法术强度。" },
    { id: 1004, name: "回归基本功", rarity: "kPrismatic", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BackToBasics_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BackToBasics_small.png", description: "禁用终极技能，并大幅强化基础技能。" },
    { id: 2103, name: "狙神飞星", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/QuestBangBang_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/QuestBangBang_small.png", description: "从远距离命中敌人时造成额外伤害。" },
    { id: 1180, name: "超强大脑", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BigBrain_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BigBrain_small.png", description: "战斗开始时获得基于法术强度的护盾。" },
    { id: 1006, name: "利刃华尔兹", rarity: "kPrismatic", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BladeWaltz_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BladeWaltz_small.png", description: "获得利刃华尔兹，突进并连续攻击附近敌人。" },
    { id: 1007, name: "大力", rarity: "kSilver", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BluntForce_large.png", fallbackIconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BluntForce_small.png", description: "获得额外攻击力。" },
    { id: 1103, name: "面包和黄油", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/GenericAbilityAugmentIcon_Gold.png", description: "你的 Q 技能获得大量技能急速。" },
    { id: 1151, name: "面包和奶酪", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/GenericAbilityAugmentIcon_Gold.png", description: "你的 E 技能获得大量技能急速。" },
    { id: 1150, name: "面包和果酱", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/GenericAbilityAugmentIcon_Gold.png", description: "你的 W 技能获得大量技能急速。" },
  ];
  const arenaAugmentIDs = arenaAugmentCatalog.map((augment) => augment.id);
  const arenaChampionPool = [[799, "安蓓萨"], [103, "阿狸"], [222, "金克丝"], [64, "李青"], [412, "锤石"], [24, "贾克斯"], [254, "蔚"], [238, "劫"], [145, "卡莎"], [157, "亚索"], [92, "锐雯"], [111, "深海泰坦"], [266, "亚托克斯"], [517, "塞拉斯"], [875, "瑟提"], [39, "艾瑞莉娅"], [121, "卡兹克"], [61, "奥莉安娜"], [51, "凯特琳"], [235, "赛娜"], [81, "伊泽瑞尔"]];
  const demoArenaMatch = (gameId, minutesAgo) => {
    const placements = [1, 2, 3, 4, 5, 6, 7];
    const participants = arenaChampionPool.map(([championId, championName], index) => {
      const subteamId = Math.floor(index / 3) + 1;
      const placement = placements[subteamId - 1];
      const win = placement <= 4;
      const { primaryStyleId, subStyleId, perkIds, ...arenaParticipant } = participant(index + 1, index < 12 ? 100 : 200, championId, championName, 3 + (index % 9), 2 + (index % 6), 5 + (index % 8), win);
      return {
        ...arenaParticipant,
        subteamId, placement, position: "",
        augmentIds: [0, 3, 6, 9].map((offset) => arenaAugmentIDs[(index + offset) % arenaAugmentIDs.length]),
        cs: 0, laneCs: 0, jungleCs: 0, csPerMinute: 0, wardsPlaced: 0, wardsKilled: 0, visionScore: 0,
        spell1Id: 4, spell2Id: 32, itemIds: [6630, 3071, 3053, 3065, 3026, 6653, 0],
      };
    });
    return {
      gameId, queueId: 1700, queueLabel: "斗魂竞技场", modeGroup: "arena", result: "win",
      createdAt: now - minutesAgo * 60_000, duration: 1_140, subjectParticipantId: 1,
      averageTier: { tier: "PLATINUM", division: "I", samples: 21 },
      teams: [], participants,
    };
  };
  // 对局时间线演示：装备路线（含出售）与技能加点（主升 Q 副升 E）。
  const demoTimeline = () => ({
    available: true, source: "lcu",
    itemGroups: [
      { minute: 0, events: [{ itemId: 1055 }, { itemId: 2003 }] },
      { minute: 6, events: [{ itemId: 3134 }, { itemId: 1036 }] },
      { minute: 11, events: [{ itemId: 6630 }, { itemId: 1001 }] },
      { minute: 16, events: [{ itemId: 3071 }, { itemId: 1055, sold: true }] },
      { minute: 23, events: [{ itemId: 3053 }] },
      { minute: 28, events: [{ itemId: 3065 }, { itemId: 3026 }] },
    ],
    skillOrder: [1, 2, 3, 1, 1, 4, 1, 3, 1, 3, 4, 3, 3, 2, 2, 4, 2, 2].map((slot, index) => ({ level: index + 1, slot })),
  });
  const overview = {
    player: {
      playerRef: currentGameDemo ? "demo-current-game" : "", displayName: summoner.displayName, gameName: summoner.gameName, tagLine: summoner.tagLine,
      profileIconId: summoner.profileIconId, summonerLevel: summoner.summonerLevel, hidden: false, isCurrent: true,
      backgroundSkinId: 164000, backgroundSkinName: "卡蜜尔", backgroundSource: "ddragon", backgroundPath: "/cdn/img/champion/splash/Camille_0.jpg",
    },
    ranks: [
      { queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "II", leaguePoints: 47, wins: 68, losses: 55, winRate: 55.3 },
      { queueType: "RANKED_FLEX_SR", tier: "platinum", division: "I", leaguePoints: 82, wins: 31, losses: 28, winRate: 52.5 },
    ],
    // 仅韩服 OP.GG 链路会返回这个多赛段形状；国服 historicalRanks 恒空，改走 rankMilestones。
    historicalRanks: [
      { season: "S2026 S1", queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "III", leaguePoints: 63, winRate: 54 },
      { season: "S2025 S3", queueType: "RANKED_SOLO_5x5", tier: "diamond", division: "IV", leaguePoints: 18, winRate: 52 },
      { season: "S2025 S2", queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "I", leaguePoints: 72, winRate: 56 },
      { season: "S2025 S1", queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "II", leaguePoints: 31, winRate: 53 },
      { season: "S2024 S3", queueType: "RANKED_SOLO_5x5", tier: "platinum", division: "I", leaguePoints: 88, winRate: 55 },
      { season: "S2024 S2", queueType: "RANKED_SOLO_5x5", tier: "platinum", division: "III", leaguePoints: 42, winRate: 51 },
      { season: "S2024 S1", queueType: "RANKED_SOLO_5x5", tier: "gold", division: "I", leaguePoints: 67, winRate: 54 },
      { season: "S2023 S2", queueType: "RANKED_SOLO_5x5", tier: "gold", division: "III", leaguePoints: 24, winRate: 52 },
      { season: "S2023 S1", queueType: "RANKED_SOLO_5x5", tier: "silver", division: "I", leaguePoints: 70, winRate: 53 },
    ],
    seasonStatsProgress: { season: "S26", scanned: 137, complete: false, message: "已统计 137 场，正在后台补全本赛季" },
    ability: {
      sampleGames: 14, baselineGames: 14, queueId: 420, queueLabel: "单双排", position: "top", positionLabel: "上路",
      baselineLabel: "近期同位置对手样本", sourceLabel: "七项指标参考 OP.GG · 本机国服对局计算",
      metrics: [
        { key: "kda", label: "KDA", description: "平均击杀与助攻相对于死亡的比例。", player: 3.42, baseline: 2.71, playerScore: 78.2, grade: "A+" },
        { key: "killParticipation", label: "参团率", description: "参与击杀数占所在队伍总击杀的比例。", unit: "%", player: 57.4, baseline: 51.8, playerScore: 68.7, grade: "A" },
        { key: "damageShare", label: "伤害占比", description: "对英雄伤害占所在队伍英雄总伤害的比例。", unit: "%", player: 26.8, baseline: 24.9, playerScore: 66.7, grade: "A" },
        { key: "dpm", label: "DPM", description: "每分钟对英雄造成的平均伤害。", player: 724, baseline: 638, playerScore: 70.3, grade: "A" },
        { key: "csm", label: "CSM", description: "每分钟获得的小兵与野怪补刀数。", player: 7.12, baseline: 6.58, playerScore: 67.1, grade: "A" },
        { key: "gpm", label: "GPM", description: "每分钟获得的平均金币。", player: 438, baseline: 412, playerScore: 65.9, grade: "A-" },
        { key: "vspm", label: "VSPM", description: "每分钟获得的平均视野得分。", player: 1.08, baseline: 1.21, playerScore: 55.3, grade: "B-" },
      ],
    },
    recentRanked: {
      queueId: 420, queueLabel: "单双排", games: 20, wins: 13, losses: 7, winRate: 65,
      kills: 5.5, deaths: 7.0, assists: 9.6, kda: 2.15,
      killParticipation: 55, killParticipationGames: 20,
      positions: [
        { position: "middle", label: "中路", games: 12, wins: 7, winRate: 58 },
        { position: "top", label: "上路", games: 8, wins: 6, winRate: 75 },
      ],
    },
    championStats: [
      { championId: 164, championName: "卡蜜尔", games: 42, winRate: 59.5, kda: 3.4, kills: 7.1, deaths: 3.8, assists: 5.9, cs: 201, csPerMinute: 6.9 },
      { championId: 103, championName: "阿狸", games: 25, winRate: 56.0, kda: 3.9, kills: 8.2, deaths: 3.1, assists: 6.4, cs: 188, csPerMinute: 6.5 },
      { championId: 222, championName: "金克丝", games: 18, winRate: 50.0, kda: 2.8, kills: 6.6, deaths: 4.5, assists: 6.1, cs: 210, csPerMinute: 7.2 },
    ],
    overall: { games: 85, winRate: 55.3, kda: 3.3, kills: 7.3, deaths: 3.9, assists: 6.0, cs: 199, csPerMinute: 6.8 },
    positions: [
      { position: "top", label: "上单", share: 46 },
      { position: "jungle", label: "打野", share: 10 },
      { position: "middle", label: "中单", share: 28 },
      { position: "bottom", label: "下路", share: 16 },
      { position: "utility", label: "辅助", share: 0 },
    ],
    masteries: [
      { championId: 164, championName: "卡蜜尔", championPoints: 486_200, championLevel: 12 },
      { championId: 103, championName: "阿狸", championPoints: 355_400, championLevel: 10 },
      { championId: 222, championName: "金克丝", championPoints: 268_000, championLevel: 9 },
      { championId: 64, championName: "李青", championPoints: 142_300, championLevel: 7 },
    ],
    recentPlayers: [
      { profileIconId: 5205, gameName: "永远滴神李青", tagLine: "CN1", games: 12, playerRef: "" },
      { profileIconId: 6296, gameName: "狐狸不吃鱼", tagLine: "CN2", games: 8, playerRef: "" },
      { profileIconId: 3543, gameName: "魂锁典狱长", tagLine: "CN1", games: 6, playerRef: "" },
    ],
    activityHours: [0, 0, 0, 0, 0, 0, 0, 1, 2, 1, 0, 2, 3, 2, 4, 3, 2, 1, 4, 6, 8, 7, 5, 2],
    matches: [
      demoMatch(90001, 42, true, [164, "卡蜜尔"], 24),
      demoRemakeMatch(90016, 72, [64, "李青"]),
      demoMatch(90005, 95, true, [222, "金克丝"], undefined, { queueId: 440, queueLabel: "灵活组排", modeGroup: "flex" }),
      demoMatch(90006, 150, true, [103, "阿狸"], undefined, { queueId: 2300, queueLabel: "海克斯大乱斗", modeGroup: "hextech-aram", gameMode: "KIWI", mapId: 12, duration: 1_360, augmentIds: [1205, 1141, 1002, 2087] }),
      demoMatch(90017, 160, false, [64, "李青"], undefined, { queueId: 2600, queueLabel: "海克斯大乱斗 海选赛", modeGroup: "hextech-qualifier", gameMode: "KIWI", mapId: 12, duration: 1_410, augmentIds: [1205, 1141, 1002, 2087] }),
      demoMatch(90015, 170, true, [222, "金克丝"], undefined, { queueId: 2400, queueLabel: "海克斯大乱斗 经典模式版", modeGroup: "hextech-classic", gameMode: "ARAM_MAYHEM_CLASSIC", mapId: 12, duration: 1_280 }),
      demoMatch(90002, 190, false, [103, "阿狸"], -18),
      demoMatch(90007, 260, false, [81, "伊泽瑞尔"], undefined, { queueId: 450, queueLabel: "极地大乱斗", modeGroup: "aram", gameMode: "ARAM", mapId: 12, duration: 1_240 }),
      demoArenaMatch(90004, 60 * 7),
      demoMatch(90008, 60 * 9, true, [64, "李青"], undefined, { queueId: 430, queueLabel: "匹配模式", modeGroup: "other" }),
      demoMatch(90009, 60 * 12, true, [24, "贾克斯"], undefined, { queueId: 850, queueLabel: "人机对战", modeGroup: "other", gameMode: "BOT" }),
      demoMatch(90010, 60 * 16, false, [92, "锐雯"], undefined, { queueId: 900, queueLabel: "无限火力", modeGroup: "urf", gameMode: "URF", duration: 1_080 }),
      demoMatch(90011, 60 * 20, true, [145, "卡莎"], undefined, { queueId: 700, queueLabel: "冠军杯赛", modeGroup: "other", gameMode: "CLASH" }),
      demoMatch(90012, 60 * 23, false, [238, "劫"], undefined, { queueId: 1300, queueLabel: "极限闪击", modeGroup: "nexus-blitz", gameMode: "NEXUSBLITZ", mapId: 21, duration: 1_120 }),
      demoMatch(90013, 60 * 25, true, [412, "锤石"], undefined, { queueId: 950, queueLabel: "末日人工智能", modeGroup: "other", gameMode: "DOOMBOTS", mapId: 12 }),
      demoMatch(90003, 60 * 26, true, [164, "卡蜜尔"]),
      demoMatch(90014, 60 * 30, false, [254, "蔚"], undefined, { queueId: 1400, queueLabel: "特殊模式", modeGroup: "other", gameMode: "SPECIAL", mapId: 22 }),
    ],
    pagination: { begIndex: 0, count: 17, hasMore: false },
    capabilities: [
      { name: "summoner", state: "available", count: 1 },
      { name: "match-history", state: "available", count: 17 },
      { name: "ranked-stats", state: "available", count: 2 },
      { name: "mastery", state: "available", count: 4 },
    ],
  };

  /* ---------- 英雄页 / 斗魂竞技场演示数据 ---------- */
  const championTitles = { 799: "铁血狼母", 103: "九尾妖狐", 222: "暴走萝莉", 64: "盲僧", 412: "魂锁典狱长", 24: "武器大师", 254: "皮城执法官", 238: "影流之主", 145: "虚空之女", 157: "疾风剑豪", 92: "放逐之刃", 111: "深海泰坦" };
  const arenaDemoChampions = arenaChampionPool.map(([id, name]) => {
    const key = championKeys[id] || String(id);
    return {
      id, key, slug: key.toLowerCase(), nameZh: name, titleZh: championTitles[id] || name, nameEn: key, titleEn: "",
      imageSource: "gtimg", imagePath: `/images/lol/act/img/champion/${key}.png`,
      artworkSource: "ddragon", artworkPath: `/cdn/img/champion/splash/${key}_0.jpg`, searchTerms: [name, key],
    };
  });
	const akaliDemoChampion = { id: 84, key: "Akali", slug: "akali", nameZh: "阿卡丽", titleZh: "离群之刺", nameEn: "Akali", titleEn: "the Rogue Assassin", imageSource: "gtimg", imagePath: "/images/lol/act/img/champion/Akali.png", artworkSource: "ddragon", artworkPath: "/cdn/img/champion/splash/Akali_0.jpg", searchTerms: ["阿卡丽", "Akali", "adk"] };
  const championsCatalogFixture = {
    source: "Riot Data Dragon", region: "KR", patch: PATCH, fetchedAt: new Date(now).toISOString(),
    tiers: [["all", "全部段位"], ["emerald_plus", "翡翠以上"], ["diamond_plus", "钻石以上"]].map(([value, label]) => ({ value, label })),
	champions: [...arenaDemoChampions, akaliDemoChampion],
  };
  const arenaRankingsFixture = {
    mode: "arena", region: "GLOBAL", patch: "16.16", source: "OP.GG JSON", fetchedAt: new Date(now).toISOString(), entertainmentSample: true,
    rows: arenaDemoChampions.slice(0, 12).map((champion, index) => ({
      championId: champion.id, key: champion.slug, name: champion.nameZh, imageSource: champion.imageSource, imagePath: champion.imagePath,
      rank: index + 1, tier: index === 11 ? -1 : Math.min(5, Math.floor(index / 2)), play: 36_809 - index * 2_360,
      winRate: Number((56.05 - index * 1.12).toFixed(2)), pickRate: Number((8.61 - index * .45).toFixed(2)), banRate: Number((18.06 - index * .71).toFixed(2)),
      kda: Number((3.44 - index * .07).toFixed(2)), averagePlacement: Number((3.26 + index * .055).toFixed(2)), firstPlaceRate: Number((22.08 - index * .82).toFixed(2)),
    })),
  };
  const arenaRankingsWithoutCatalog = () => ({
    ...arenaRankingsFixture,
    rows: arenaRankingsFixture.rows.map(({ key: _key, name: _name, imageSource: _imageSource, imagePath: _imagePath, ...row }) => row),
  });
  const demoItemNames = {
    223008: "暴食胫甲", 223031: "无尽之刃", 223033: "凡性的提醒", 223053: "斯特拉克的挑战护手", 223074: "贪欲九头蛇", 223078: "三相之力",
    226692: "星蚀", 226631: "挺进破坏者", 226653: "玛莫提乌斯之噬", 226656: "永霜", 226672: "焚天", 226710: "血手",
    447100: "幻影之刃", 447101: "赌徒之刃", 447102: "神圣干涉", 447104: "巨龙之心", 447107: "斩首者", 447115: "弑君", 447116: "均衡宗师", 447120: "无尽之力",
  };
  const demoItemAsset = (id) => ({ id, kind: "item", name: demoItemNames[id] || `装备 ${id}`, source: "ddragon", path: `/cdn/${PATCH}/img/item/${id}.png` });
  const demoMetric = (ids, index, extra = {}) => ({
    assets: ids.map(demoItemAsset), games: 12_579 - index * 790, winRate: Number((63.6 - index * 1.15).toFixed(2)),
    averagePlacement: Number((2.88 + index * .045).toFixed(2)), firstPlaceRate: Number((25.1 - index * .72).toFixed(2)), pickRate: Number((8.4 - index * .31).toFixed(2)), ...extra,
  });
  const demoAugmentAsset = (augment) => ({
    id: augment.id, kind: "arena-augment", name: augment.name, description: augment.description, source: "communitydragon",
    path: `/latest/game/assets/${augment.iconPath.split("/ASSETS/").at(-1).toLowerCase()}`,
    fallbackPath: augment.fallbackIconPath ? `/latest/game/assets/${augment.fallbackIconPath.split("/ASSETS/").at(-1).toLowerCase()}` : "",
  });
  const augmentRarityNumber = { kSilver: 1, kGold: 4, kPrismatic: 8 };
  const augmentRarityKey = { kSilver: "silver", kGold: "gold", kPrismatic: "prismatic" };
  const arenaAugmentGroupsFixture = ["kSilver", "kGold", "kPrismatic"].map((rarity) => ({
    rarity: augmentRarityNumber[rarity],
    rows: arenaAugmentCatalog.filter((augment) => augment.rarity === rarity).map((augment, index) => ({
      assets: [demoAugmentAsset(augment)], rarity: augmentRarityKey[rarity], games: 8_020 - index * 430,
      winRate: Number((63.6 - index * 1.1).toFixed(2)), averagePlacement: Number((2.88 + index * .06).toFixed(2)), firstPlaceRate: Number((25.1 - index * .9).toFixed(2)), pickRate: Number((9.2 - index * .5).toFixed(2)),
    })),
  }));
  const demoChampionRef = (id) => {
    const meta = arenaDemoChampions.find((champion) => champion.id === id) || arenaDemoChampions[0];
    return { id: meta.id, key: meta.key, name: meta.nameZh, imageSource: meta.imageSource, imagePath: meta.imagePath };
  };
  const arenaDetailFixture = {
    mode: "arena", region: "GLOBAL", patch: "16.16", source: "OP.GG JSON", fetchedAt: new Date(now).toISOString(), entertainmentSample: true,
    arenaStats: { tier: 0, rank: 9, rankPrevPatch: 12, games: 36_809, kda: 3.44, averagePlacement: 3.26, firstPlaceRate: 22.08, pickRate: 8.61, winRate: 56.05, banRate: 18.06 },
    arenaAugmentGroups: arenaAugmentGroupsFixture,
    arenaAugments: arenaAugmentGroupsFixture.flatMap((group) => group.rows),
    teamCompositions: [
      { champions: [demoChampionRef(799), demoChampionRef(111), demoChampionRef(24)], averagePlacement: 2.72, firstPlaceRate: 27.4, pickRate: 3.8, winRate: 64.1, games: 1_284 },
      { champions: [demoChampionRef(799), demoChampionRef(103), demoChampionRef(412)], averagePlacement: 2.88, firstPlaceRate: 24.1, pickRate: 3.2, winRate: 61.8, games: 986 },
      { champions: [demoChampionRef(799), demoChampionRef(64), demoChampionRef(222)], averagePlacement: 3.01, firstPlaceRate: 21.9, pickRate: 2.7, winRate: 59.4, games: 744 },
    ],
    build: {
      prismItems: [[447116], [447115], [447104], [447107], [447100], [447101], [447102], [447120]].map((ids, index) => demoMetric(ids, index)),
      coreItems: [[226672, 223053, 223074], [226692, 223078, 223033], [226631, 223053, 226653], [226692, 223031, 223033], [226672, 223074, 223053], [226631, 223078, 223031], [226692, 226653, 223074], [226672, 223033, 223031], [226631, 223053, 223033]].map((ids, index) => demoMetric(ids, index, { games: 1_055 - index * 72 })),
      boots: [[223008], [223031], [223053]].map((ids, index) => demoMetric(ids, index, { games: 15_600 - index * 3_900 })),
      skills: [{
        assets: [
          { kind: "skill", name: "狡诈扫荡", description: "向前挥砍并获得充能。", source: "ddragon", path: `/cdn/${PATCH}/img/spell/AmbessaQ.png` },
          { kind: "skill", name: "铁腕拒斥", description: "获得护盾并对周围敌人造成伤害。", source: "ddragon", path: `/cdn/${PATCH}/img/spell/AmbessaE.png` },
          { kind: "skill", name: "裂阵", description: "强化下一次攻击并位移。", source: "ddragon", path: `/cdn/${PATCH}/img/spell/AmbessaW.png` },
        ], skillPriority: ["Q", "E", "W"], games: 12_043, winRate: 57.4, averagePlacement: 3.11, firstPlaceRate: 23.8,
      }],
    },
  };
  const demoArenaMatchDetails = new Map();
  const demoArenaFirstMatch = (gameId, minutesAgo, damage) => {
    const completeMatch = demoArenaMatch(gameId, minutesAgo);
    const subject = completeMatch.participants[0];
    subject.kills = 12 + (gameId % 5); subject.deaths = 4; subject.assists = 14; subject.kda = Number(((subject.kills + subject.assists) / subject.deaths).toFixed(2));
    subject.damage = damage; subject.damageTaken = Math.round(damage * .48); subject.gold = 17_340 + (gameId % 800); subject.itemIds = [447116, 226692, 223033, 223031, 223008, 223074, 0];
    demoArenaMatchDetails.set(gameId, structuredClone(completeMatch));
    const summaryMatch = structuredClone(completeMatch);
    for (const participant of summaryMatch.participants.slice(1)) {
      participant.championLevel = 0; participant.kills = 0; participant.deaths = 0; participant.assists = 0; participant.kda = 0;
      participant.damage = 0; participant.damageTaken = 0; participant.gold = 0; participant.itemIds = []; participant.augmentIds = [];
    }
    return summaryMatch;
  };
  const arenaFirstPlacesFixture = {
    source: "YOUR.GG", region: "KR", patch: "16.16", fetchedAt: new Date(now).toISOString(),
    matches: [demoArenaFirstMatch(98001, 120, 98_209), demoArenaFirstMatch(98002, 220, 76_480)],
  };
  const demoItemsCatalog = {
    items: Object.keys(demoItemNames).map(Number).concat([6630, 3071, 3053, 3065, 3026, 6653]).map((id) => ({ id, name: demoItemNames[id] || `装备 ${id}`, description: "斗魂竞技场演示装备", iconPath: `ddragon:/cdn/${PATCH}/img/item/${id}.png` })),
  };

  const demoChampionPool = [[164, "卡蜜尔"], [103, "阿狸"], [222, "金克丝"], [64, "李青"], [412, "锤石"], [24, "贾克斯"], [254, "蔚"], [238, "劫"], [145, "卡莎"], [157, "亚索"], [92, "锐雯"], [111, "深海泰坦"]];
  const recentGamesFor = (championId, championName, wins, losses, seed) => {
    const total = Math.min(8, wins + losses);
    return Array.from({ length: total }, (_, index) => {
      const win = (index * 7 + seed) % (wins + losses) < wins;
      const alt = demoChampionPool[(seed + index * 3) % demoChampionPool.length];
      const useMain = (index + seed) % 3 !== 0;
      return {
        championId: useMain ? championId : alt[0], championName: useMain ? championName : alt[1],
        win, kills: 3 + ((seed + index * 5) % 10), deaths: 1 + ((seed * 3 + index) % 7), assists: 4 + ((seed + index * 2) % 11),
        cs: 128 + ((seed * 11 + index * 23) % 120), queueLabel: "单排/双排", createdAt: now - (index + 1) * 3_600_000 * 5,
      };
    });
  };
  const livePlayer = (championId, championName, position, teamId, isCurrent, tier, wins, losses, kdaValue, seed) => {
    const recentGames = recentGamesFor(championId, championName, wins, losses, seed);
    const recentWins = recentGames.filter((game) => game.win).length;
    return {
      championId, championName, position, teamId, isCurrent,
      playerRef: "", gameName: isCurrent ? summoner.gameName : `演示玩家`, tagLine: "DEMO",
      rank: { tier, division: "II", leaguePoints: 45 },
      modeStats: { games: wins + losses, wins, losses, winRate: Number((wins * 100 / (wins + losses)).toFixed(1)), kda: kdaValue },
      recentPositions: [{ position, games: Math.max(1, recentGames.length) }],
      recentGames, recentRankedRecord: { games: recentGames.length, wins: recentWins, losses: recentGames.length - recentWins },
    };
  };
  const runePage = (key, title, primaryStyleId, subStyleId, perkIds, statModIds, stats) => ({ key, title, primaryStyleId, subStyleId, selectedPerkIds: [...perkIds, ...statModIds], statModIds, stats });
  const live = {
    available: true, phase: "ChampSelect", queueId: 420, queueLabel: "单排/双排", gameMode: "CLASSIC", mapId: 11, gameId: 0,
    players: [
      livePlayer(164, "卡蜜尔", "top", 100, true, "emerald", 7, 3, 3.4, 1),
      livePlayer(64, "李青", "jungle", 100, false, "emerald", 6, 4, 2.9, 2),
      livePlayer(103, "阿狸", "middle", 100, false, "diamond", 7, 3, 4.2, 3),
      livePlayer(222, "金克丝", "bottom", 100, false, "emerald", 5, 5, 2.9, 4),
      livePlayer(412, "锤石", "utility", 100, false, "emerald", 5, 5, 3.1, 5),
      livePlayer(24, "贾克斯", "top", 200, false, "diamond", 7, 3, 3.9, 6),
      livePlayer(254, "蔚", "jungle", 200, false, "emerald", 5, 5, 2.6, 7),
      livePlayer(238, "劫", "middle", 200, false, "emerald", 4, 6, 2.1, 8),
      livePlayer(145, "卡莎", "bottom", 200, false, "emerald", 6, 4, 3.6, 9),
      livePlayer(111, "深海泰坦", "utility", 200, false, "emerald", 4, 6, 2.3, 10),
    ],
    recommendations: {
      source: "ranked", resolvedMode: "ranked", resolvedRegion: "KR", dataVersion: PATCH, currentVersion: PATCH,
      isFallback: false, isStale: false, hasRunes: true, hasAugments: false, hasTopPlayers: true, hasItemDepths: true,
      positions: [{ position: "top", roleRate: 76.4 }, { position: "mid", roleRate: 23.6 }],
      resolvedPosition: "top", positionSource: "requested",
      hero: {
        winRate: 52.13, pickRate: 8.2, banRate: 6.4,
        strongAgainst: [{ championId: 24, championName: "贾克斯" }, { championId: 238, championName: "劫" }, { championId: 92, championName: "锐雯" }],
        weakAgainst: [{ championId: 111, championName: "深海泰坦" }, { championId: 412, championName: "锤石" }],
      },
      runes: {
        opgg: [
          runePage("opgg", "征服者 · 精密 + 坚决", 8000, 8400, [8010, 9111, 9104, 8299, 8446, 8453], [5005, 5008, 5011], { pickRate: 75.9, games: 3521, winRate: 49.62 }),
          runePage("opgg-1", "强攻 · 精密 + 主宰", 8000, 8100, [8005, 9101, 9105, 8014, 8139, 8143], [5008, 5010, 5001], { pickRate: 17.6, games: 815, winRate: 51.66 }),
        ],
        specialists: [
          { ...runePage("specialist-0", "목숨뿐#아초록스", 8000, 8400, [8010, 9111, 9104, 8299, 8473, 8453], [5005, 5008, 5011], { games: 1451, winRate: 51 }), playerName: "목숨뿐", tagLine: "아초록스", championName: "卡蜜尔", tier: "master", leaguePoints: "211", championGames: 1451, playedAt: now - 3 * 86_400_000, result: "win", region: "kr", position: "top", opponentChampionId: 24, opponentChampionName: "贾克斯", opponentPlayerName: "LaneRival", opponentTagLine: "KR1", opponentTier: "diamond", opponentDivision: "I", opponentWinRate: 57, itemIds: [6630, 3071, 3053, 3047] },
        ],
        pros: [],
      },
      build: {
        position: "top",
        skillPriority: ["Q", "E", "W"],
        skillOrder: ["Q", "W", "E", "Q", "Q", "R", "Q", "E", "Q", "E", "R", "E", "E", "W", "W"],
        skillStats: { pickRate: 61.2, games: 21_384, winRate: 53.6 },
        spellOptions: [
          { ids: [4, 12], stats: { pickRate: 78.2, games: 29_884, winRate: 54.1 } },
          { ids: [4, 14], stats: { pickRate: 14.6, games: 5_580, winRate: 52.3 } },
        ],
        starterOptions: [
          { ids: [1054, 2003], stats: { pickRate: 56.6, games: 12_140, winRate: 50.95 } },
          { ids: [1055, 2003], stats: { pickRate: 21.7, games: 4_656, winRate: 52.48 } },
        ],
        bootOptions: [
          { ids: [3047], stats: { pickRate: 49.6, games: 10_640, winRate: 52.87 } },
          { ids: [3111], stats: { pickRate: 30.1, games: 6_456, winRate: 48.34 } },
        ],
        coreOptions: [
          { ids: [6630, 3071, 3053], stats: { pickRate: 29.82, games: 8_417, winRate: 58.63 } },
          { ids: [6630, 3065, 3071], stats: { pickRate: 23.09, games: 5_203, winRate: 55.16 } },
          { ids: [6630, 3071, 3026], stats: { pickRate: 12.18, games: 3_946, winRate: 49.46 } },
          { ids: [6653, 3071, 3053], stats: { pickRate: 8.72, games: 2_108, winRate: 53.91 } },
          { ids: [6630, 3026, 3065], stats: { pickRate: 6.44, games: 1_557, winRate: 51.83 } },
        ],
        fourthOptions: [
          { ids: [3026], stats: { games: 8_161, winRate: 63.78 } },
          { ids: [3053], stats: { games: 4_104, winRate: 61.04 } },
          { ids: [3065], stats: { games: 2_451, winRate: 57.61 } },
          { ids: [3071], stats: { games: 752, winRate: 67.42 } },
          { ids: [6653], stats: { games: 570, winRate: 65.79 } },
        ],
        fifthOptions: [
          { ids: [3053], stats: { games: 1_235, winRate: 59.84 } },
          { ids: [3065], stats: { games: 831, winRate: 60.17 } },
          { ids: [3026], stats: { games: 410, winRate: 61.22 } },
          { ids: [6653], stats: { games: 309, winRate: 61.81 } },
        ],
        sixthOptions: [
          { ids: [3089], stats: { games: 532, winRate: 58.12 } },
          { ids: [3135], stats: { games: 418, winRate: 57.04 } },
          { ids: [3157], stats: { games: 201, winRate: 56.22 } },
        ],
        itemSource: "OP.GG",
        itemWindow: "当前版本",
        itemChainStatus: "ready",
        fourthSample: 16_038,
        fifthSample: 2_785,
        sixthSample: 1_151,
        prismOptions: [],
      },
    },
    championAbilities: [
      { slot: "Q", name: "精准礼仪", iconPath: `ddragon:/cdn/${PATCH}/img/spell/CamilleQ.png`, description: "下次攻击造成额外伤害；再次施放造成真实伤害。" },
      { slot: "W", name: "战术横扫", iconPath: `ddragon:/cdn/${PATCH}/img/spell/CamilleW.png`, description: "扇形横扫，外缘命中减速并回复生命。" },
      { slot: "E", name: "钩索", iconPath: `ddragon:/cdn/${PATCH}/img/spell/CamilleE.png`, description: "钩住地形二段冲刺，命中英雄眩晕。" },
      { slot: "R", name: "海克斯最后通牒", iconPath: `ddragon:/cdn/${PATCH}/img/spell/CamilleR.png`, description: "锁定目标形成决斗领域。" },
    ],
  };

  // A small, deterministic live Arena session for the visual demo. The real
  // client payload is still used in production; this branch only changes the
  // fixture returned when the page was opened with `?demo=arena`.
  const arenaLiveAugments = arenaAugmentCatalog.slice(0, 9).map((augment, index) => ({
    id: augment.id,
    rarity: augmentRarityKey[augment.rarity],
    grade: ["S", "A", "A", "S", "A", "B", "A", "S", "B"][index],
    assets: [demoAugmentAsset(augment)],
    games: 8_020 - index * 430,
    winRate: Number((63.6 - index * 1.1).toFixed(2)),
    averagePlacement: Number((2.88 + index * .06).toFixed(2)),
    firstPlaceRate: Number((25.1 - index * .9).toFixed(2)),
    pickRate: Number((9.2 - index * .5).toFixed(2)),
  }));
  const arenaLiveCoreRoutes = [
    [226672, 223053, 223074], [226692, 223078, 223033], [226631, 223053, 226653],
    [226692, 223031, 223033], [226672, 223074, 223053], [226631, 223078, 223031],
    [226692, 226653, 223074], [226672, 223033, 223031], [226631, 223053, 223033],
    [226692, 223074, 223078], [226672, 223031, 223074], [226631, 223033, 223078],
    [226692, 223053, 226653], [226672, 223078, 223031], [226631, 223074, 226653],
  ];
  const arenaLiveBuild = {
    position: "other",
    skillPriority: ["Q", "E", "W"],
    skillOrder: ["Q", "E", "W", "Q", "Q", "R", "Q", "E", "Q", "E", "R", "E", "E", "W", "W"],
    skillStats: { pickRate: 61.2, games: 12_043, winRate: 57.4 },
    spellOptions: [],
    starterOptions: [],
    bootOptions: arenaDetailFixture.build.boots.slice(0, 2).map((row) => ({
      ids: row.assets.map((asset) => asset.id),
      stats: { pickRate: row.pickRate, games: row.games, winRate: row.winRate },
    })),
    coreOptions: arenaLiveCoreRoutes.map((ids, index) => ({
      ids,
      grade: ["S", "A", "B", "S", "A", "B", "A", "S", "B", "A", "B", "A", "S", "B", "A"][index],
      stats: {
        games: 1_055 - index * 42,
        winRate: Number((63.6 - index * .17).toFixed(2)),
        averagePlacement: Number((2.88 + index * .02).toFixed(2)),
        firstPlaceRate: Number((25.1 - index * .18).toFixed(2)),
        pickRate: Number((8.4 - index * .12).toFixed(2)),
      },
    })),
    prismOptions: arenaDetailFixture.build.prismItems.slice(0, 8).map((row, index) => ({
      ids: row.assets.map((asset) => asset.id),
      grade: ["S", "A", "B", "A", "S", "B", "A", "B"][index],
      stats: {
        games: row.games,
        winRate: row.winRate,
        averagePlacement: row.averagePlacement,
        firstPlaceRate: row.firstPlaceRate,
        pickRate: row.pickRate,
      },
    })),
    fourthOptions: [], fifthOptions: [], sixthOptions: [], itemChainStatus: "ready",
  };
  const arenaLivePlayers = arenaChampionPool.slice(0, 18).map(([championId, championName], index) => ({
    ...livePlayer(championId, championName, "other", index < 5 ? 100 : 200, index === 0, index % 3 === 0 ? "diamond" : "emerald", 6 + (index % 4), 3 + (index % 3), 2.8 + index * .2, index + 1),
    subteamId: Math.floor(index / 3) + 1,
    arenaGroup: String(Math.floor(index / 3) + 1),
    placement: Math.floor(index / 3) + 1,
    isAlly: index < 3,
    championLocked: true,
    spell1Id: 4,
    spell2Id: 32,
    augmentIds: arenaLiveAugments.slice(index % 4, (index % 4) + 4).map((row) => row.id),
  }));
  const arenaLive = {
    ...structuredClone(live),
    available: true,
    phase: "ChampSelect",
    queueId: 1700,
    queueLabel: "斗魂竞技场",
    modeGroup: "arena",
    gameMode: "CHERRY",
    mapId: 30,
    gameId: 97001,
    currentChampionId: 799,
	champSelectNotice: "斗魂英雄选择阶段只展示小队玩家信息",
	players: arenaLivePlayers.slice(0, 3).map((player) => ({ ...player, isAlly: true })),
    recommendations: {
      source: "OP.GG JSON",
      resolvedMode: "arena",
      resolvedRegion: "GLOBAL",
      dataVersion: PATCH,
      currentVersion: PATCH,
      isFallback: false,
      isStale: false,
      hasRunes: false,
      hasAugments: true,
      hasTopPlayers: false,
      hasItemDepths: false,
      hasCounters: false,
      hasBanRate: false,
      positions: [],
      resolvedPosition: "other",
      positionSource: "mode",
      hero: { tier: 0, winRate: 56.05, pickRate: 8.61, banRate: 18.06 },
      augments: arenaLiveAugments,
      build: arenaLiveBuild,
    },
    championAbilities: [
      ...arenaDetailFixture.build.skills[0].assets.map((asset, index) => ({
        slot: ["Q", "E", "W"][index], name: asset.name, iconPath: `${asset.source}:` + asset.path, description: asset.description,
      })),
      { slot: "R", name: "终极技能", iconPath: `ddragon:/cdn/${PATCH}/img/spell/AmbessaR.png`, description: "向前突进并锁定目标。" },
    ],
  };
  const arenaFullLive = {
    ...structuredClone(arenaLive),
    phase: "InProgress",
    champSelectNotice: "",
    players: structuredClone(arenaLivePlayers),
    arenaGrouped: true,
    arenaMascotMapping: true,
  };

  const hextechLiveAugments = arenaLiveAugments.map((augment, index) => ({
    id: augment.id,
    rarity: augment.rarity,
    grade: augment.grade,
    assets: augment.assets,
    score: Number((92.4 - index * 3.1).toFixed(1)),
    games: 12_480 - index * 610,
    winRate: Number((61.8 - index * .85).toFixed(2)),
  }));
  const hextechLiveItemRanking = [
    [3071, "黑色切割者"], [3053, "斯特拉克的挑战护手"], [3026, "守护天使"], [3065, "振奋盔甲"],
    [6653, "兰德里的苦楚"], [3089, "灭世者的死亡之帽"], [3135, "虚空之杖"], [3157, "中娅沙漏"],
  ].map(([id, name], index) => ({
    assets: [{ id, name }],
    score: Number((91.8 - index * 4.2).toFixed(1)),
    games: 9_840 - index * 740,
    winRate: Number((59.6 - index * .7).toFixed(2)),
  }));
  const hextechLive = {
    ...structuredClone(live),
    available: true,
    phase: "ChampSelect",
    queueId: 2300,
    queueLabel: "海克斯大乱斗",
    modeGroup: "hextech-aram",
    gameMode: "KIWI",
    mapId: 12,
    gameId: 98001,
    currentChampionId: 164,
    players: live.players.map((player, index) => ({
      ...structuredClone(player),
      position: "other",
      championLocked: true,
      spell1Id: 4,
      spell2Id: 32,
      augmentIds: hextechLiveAugments.slice(index % 4, (index % 4) + 4).map((row) => row.id),
    })),
    recommendations: {
      source: "Hexdata",
      resolvedMode: "hextech",
      resolvedRegion: "GLOBAL",
      dataVersion: PATCH,
      currentVersion: PATCH,
      isFallback: false,
      isStale: false,
      hasRunes: false,
      hasAugments: true,
      hasTopPlayers: false,
      hasItemDepths: false,
      hasCounters: false,
      hasBanRate: false,
      positions: [],
      resolvedPosition: "other",
      positionSource: "mode",
      hero: { tier: 1, winRate: 54.72, pickRate: 7.83 },
      augments: hextechLiveAugments,
      itemRanking: hextechLiveItemRanking,
      build: {
        ...structuredClone(live.recommendations.build),
        position: "other",
        spellOptions: [
          { ids: [4, 32], stats: { pickRate: 84.6, games: 18_420, winRate: 55.1 } },
          { ids: [6, 32], stats: { pickRate: 9.8, games: 2_134, winRate: 53.7 } },
        ],
      },
    },
  };

  /* 完整符文树（真实符文 ID 与官方图标路径，供对局符文板展示） */
  const perkEntry = (id, name, icon) => ({ id, name, iconPath: `/lol-game-data/assets/v1/perk-images/${icon}` });
  const statModEntry = (id, name) => ({ id, name, iconPath: `ddragon:${perkPaths[id]}` });
  const perksCatalog = {
    styles: [
      { id: 8000, name: "精密", iconPath: "/lol-game-data/assets/v1/perk-images/Styles/7201_Precision.png", slots: [
        { perks: [perkEntry(8005, "强攻", "Styles/Precision/PressTheAttack/PressTheAttack.png"), perkEntry(8008, "致命节奏", "Styles/Precision/LethalTempo/LethalTempoTemp.png"), perkEntry(8021, "迅捷步法", "Styles/Precision/FleetFootwork/FleetFootwork.png"), perkEntry(8010, "征服者", "Styles/Precision/Conqueror/Conqueror.png")] },
        { perks: [perkEntry(9101, "吸收生命力", "Styles/Precision/AbsorbLife/AbsorbLife.png"), perkEntry(9111, "凯旋", "Styles/Precision/Triumph.png"), perkEntry(8009, "气定神闲", "Styles/Precision/PresenceOfMind/PresenceOfMind.png")] },
        { perks: [perkEntry(9104, "传说：欢欣", "Styles/Precision/LegendAlacrity/LegendAlacrity.png"), perkEntry(9105, "传说：急速", "Styles/Precision/LegendHaste/LegendHaste.png"), perkEntry(9103, "传说：血统", "Styles/Precision/LegendBloodline/LegendBloodline.png")] },
        { perks: [perkEntry(8014, "致命一击", "Styles/Precision/CoupDeGrace/CoupDeGrace.png"), perkEntry(8017, "砍倒", "Styles/Precision/CutDown/CutDown.png"), perkEntry(8299, "坚毅不倒", "Styles/Sorcery/LastStand/LastStand.png")] },
      ] },
      { id: 8100, name: "主宰", iconPath: "/lol-game-data/assets/v1/perk-images/Styles/7200_Domination.png", slots: [
        { perks: [perkEntry(8112, "电刑", "Styles/Domination/Electrocute/Electrocute.png"), perkEntry(8128, "黑暗收割", "Styles/Domination/DarkHarvest/DarkHarvest.png"), perkEntry(9923, "丛刃", "Styles/Domination/HailOfBlades/HailOfBlades.png")] },
        { perks: [perkEntry(8126, "恶意中伤", "Styles/Domination/CheapShot/CheapShot.png"), perkEntry(8139, "血之滋味", "Styles/Domination/TasteOfBlood/GreenTerror_TasteOfBlood.png"), perkEntry(8143, "突然冲击", "Styles/Domination/SuddenImpact/SuddenImpact.png")] },
        { perks: [perkEntry(8120, "幽灵魄罗", "Styles/Domination/GhostPoro/GhostPoro.png"), perkEntry(8136, "僵尸守卫", "Styles/Domination/ZombieWard/ZombieWard.png"), perkEntry(8138, "眼球收集器", "Styles/Domination/EyeballCollection/EyeballCollection.png")] },
        { perks: [perkEntry(8135, "贪欲猎手", "Styles/Domination/TreasureHunter/TreasureHunter.png"), perkEntry(8105, "无情猎手", "Styles/Domination/RelentlessHunter/RelentlessHunter.png"), perkEntry(8106, "终极猎手", "Styles/Domination/UltimateHunter/UltimateHunter.png")] },
      ] },
      { id: 8400, name: "坚决", iconPath: "/lol-game-data/assets/v1/perk-images/Styles/7204_Resolve.png", slots: [
        { perks: [perkEntry(8437, "不灭之握", "Styles/Resolve/GraspOfTheUndying/GraspOfTheUndying.png"), perkEntry(8439, "余震", "Styles/Resolve/VeteranAftershock/VeteranAftershock.png"), perkEntry(8465, "守护者", "Styles/Resolve/Guardian/Guardian.png")] },
        { perks: [perkEntry(8446, "爆破", "Styles/Resolve/Demolish/Demolish.png"), perkEntry(8463, "生命源泉", "Styles/Resolve/FontOfLife/FontOfLife.png"), perkEntry(8401, "护盾猛击", "Styles/Resolve/MirrorShell/MirrorShell.png")] },
        { perks: [perkEntry(8429, "调节", "Styles/Resolve/Conditioning/Conditioning.png"), perkEntry(8444, "复苏之风", "Styles/Resolve/SecondWind/SecondWind.png"), perkEntry(8473, "骸骨镀层", "Styles/Resolve/BonePlating/BonePlating.png")] },
        { perks: [perkEntry(8451, "过度生长", "Styles/Resolve/Overgrowth/Overgrowth.png"), perkEntry(8453, "复苏", "Styles/Resolve/Revitalize/Revitalize.png"), perkEntry(8242, "坚定", "Styles/Sorcery/Unflinching/Unflinching.png")] },
      ] },
    ],
    statModSlots: [
      { type: "kStatMod", perks: [statModEntry(5005, "攻击速度"), statModEntry(5008, "适应之力"), statModEntry(5007, "技能急速")] },
      { type: "kStatMod", perks: [statModEntry(5008, "适应之力"), statModEntry(5010, "移动速度"), statModEntry(5001, "成长生命值")] },
      { type: "kStatMod", perks: [statModEntry(5011, "生命值"), statModEntry(5013, "韧性"), statModEntry(5001, "成长生命值")] },
    ],
    perks: [],
    augments: arenaAugmentCatalog,
  };

  /* ---------- 好友面板演示数据 ---------- */
  const demoFriend = (gameName, tagLine, icon, groupId, availability, extra = {}) => ({
	playerRef: `player_${String(icon).padStart(32, "0").slice(-32)}`, gameName, tagLine, icon, availability,
    groupId, displayGroupId: groupId, note: extra.note || "", statusMessage: extra.statusMessage || "",
    product: extra.product || "league_of_legends", productName: extra.productName || "",
    lastSeenAt: extra.lastSeenAt || "", gameStatus: extra.gameStatus || "",
    championId: extra.championId || 0, championName: extra.championName || "",
    queueLabel: extra.queueLabel || "", gameStartedAt: extra.gameStartedAt || 0,
  });
  const friendsFixture = () => ({
    groups: [
      { id: 101, name: "开黑车队", priority: 0, collapsed: false, isMetaGroup: false },
      { id: 102, name: "峡谷小学同学", priority: 1, collapsed: true, isMetaGroup: false },
      { id: 0, name: "默认分组", priority: 2, collapsed: false, isMetaGroup: true },
    ],
    friends: [
      demoFriend("追风剑豪本人", "52048", 5205, 101, "dnd", { gameStatus: "inGame", queueLabel: "单排/双排", championId: 157, championName: "亚索", gameStartedAt: now - 23 * 60_000 }),
      demoFriend("峡谷第一莫甘娜", "Morg1", 685, 101, "dnd", { note: "表妹", gameStatus: "inGame", queueLabel: "极地大乱斗", championId: 61, championName: "奥莉安娜", gameStartedAt: now - 7 * 60_000 }),
      demoFriend("永远滴神李青", "11007", 588, 101, "dnd", { gameStatus: "championSelect" }),
      demoFriend("狐狸不吃鱼", "77123", 4027, 101, "chat", {}),
      demoFriend("兔子警官", "02330", 23, 102, "chat", {}),
      demoFriend("蹲草丛专业户", "44551", 512, 102, "away", {}),
      demoFriend("暴走萝莉金克丝", "00001", 3546, 0, "chat", { statusMessage: "晚上八点开车" }),
      demoFriend("云顶老登", "TFT99", 6296, 0, "dnd", { product: "tft", productName: "云顶之弈", gameStatus: "inGame" }),
      demoFriend("魂锁典狱长", "40002", 1389, 0, "away", {}),
      demoFriend("发条魔灵", "31415", 3543, 0, "mobile", {}),
      demoFriend("排队等观战", "GG123", 7, 0, "dnd", { gameStatus: "inQueue" }),
      demoFriend("深海泰坦", "88420", 29, 101, "offline", { lastSeenAt: iso(3 * 60) }),
      demoFriend("解脱者塞拉斯", "02331", 1665, 0, "offline", { lastSeenAt: iso(26 * 60) }),
      demoFriend("赏金猎人", "66210", 4368, 102, "offline", { lastSeenAt: iso(5 * 24 * 60) }),
    ],
  });

  /* ---------- 工具：自动 / 维护 / 生涯 / 领奖 / 征召 ---------- */
  let suiteWatch = {
    schemaVersion: 3,
    masterEnabled: true,
    rules: {
      autoAccept: { enabled: true, delayMs: 1500 },
      autoReconnect: { enabled: true, delayMs: 10000 },
      autoPlayAgain: { enabled: true },
      autoHonor: { enabled: true, strategy: "prefer-party" },
      skipCelebration: { enabled: true },
      positionBroadcast: { enabled: true, visibility: "self" },
      promoteLeader: { enabled: false },
      invitations: { enabled: false, policies: { "420": "ignore", "440": "ignore", "450": "ignore", default: "ignore" } },
      autoMatchmaking: { enabled: false, delayMs: 5000, minPartySize: 1 },
    },
    facade: { statusMessageEnabled: false, statusMessage: "", rankEnabled: false, rank: {} },
  };
  const suiteChampSelectGroups = [
    { groupId: "ranked", name: "排位（单双 / 灵活）", banLimit: 5, pickLimit: 5, positions: ["top", "jungle", "middle", "bottom", "utility", "default"], hasBan: true, hasBench: false, hasTrade: true },
    { groupId: "normal", name: "普通征召", banLimit: 5, pickLimit: 5, positions: ["default"], hasBan: true, hasBench: false, hasTrade: true },
    { groupId: "aram", name: "大乱斗类", banLimit: 0, pickLimit: 5, positions: ["default"], hasBan: false, hasBench: true, hasTrade: true },
    { groupId: "arena", name: "斗魂竞技场", banLimit: 3, pickLimit: 3, positions: ["default"], hasBan: true, hasBench: false, hasTrade: false },
    { groupId: "event", name: "活动模式", banLimit: 3, pickLimit: 3, positions: ["default"], hasBan: true, hasBench: false, hasTrade: true },
    { groupId: "practice", name: "人机 / 自定义", banLimit: 5, pickLimit: 5, positions: ["default"], hasBan: true, hasBench: false, hasTrade: true },
  ];
  const demoChampSelectSide = (delayMs, champions = {}) => ({ enabled: false, strategy: "show-then-lock", delayMs, avoidTeammateIntent: true, champions });
  suiteWatch.champSelect = { enabled: true, groups: Object.fromEntries(suiteChampSelectGroups.map((definition) => {
    const pools = Object.fromEntries(definition.positions.map((position) => [position, []]));
    return [definition.groupId, { ban: demoChampSelectSide(2000, { default: [] }), pick: demoChampSelectSide(500, structuredClone(pools)), bench: { enabled: definition.hasBench, holdMs: 1000, preferFirst: definition.hasBench, handleTrade: false } }];
  })) };
  suiteWatch.champSelect.groups.ranked.ban.enabled = true;
  suiteWatch.champSelect.groups.ranked.ban.champions.default = [157, 238, 103, 61];
  suiteWatch.champSelect.groups.ranked.pick.enabled = true;
  suiteWatch.champSelect.groups.ranked.pick.strategy = "lock-now";
  suiteWatch.champSelect.groups.ranked.pick.champions.middle = [103, 61, 517];
  suiteWatch.champSelect.groups.normal.ban.enabled = true;
  suiteWatch.champSelect.groups.normal.ban.champions.default = [157, 238, 103];
  suiteWatch.champSelect.groups.aram.pick.enabled = true;
  suiteWatch.champSelect.groups.aram.pick.champions.default = [222, 145, 99, 412];
  suiteWatch.champSelect.groups.aram.bench.enabled = true;
  let suiteChampSelectState = {
    active: true, phase: "ChampSelect", groupId: "ranked", position: "middle", remainingMs: 18000, benchEnabled: false, sessionPaused: false, unsupported: false,
    banCapabilityKnown: true, hasBanAction: true, activeBanId: 103, activePickId: 0,
    banStates: { "157": "gone", "238": "intent", "103": "available", "61": "available", "517": "available" },
    pickStates: { "157": "gone", "238": "intent", "103": "available", "61": "available", "517": "unavailable" },
    owned: { "157": true, "238": true, "103": true, "61": true, "517": false },
    records: [
      { at: iso(0.7), kind: "ok", message: "进入英雄选择，识别为排位（单双 / 灵活），禁用使用共用序列，选用使用中单序列" },
      { at: iso(0.3), kind: "warn", message: "已顺延前 2 个不可用备选" },
      { at: iso(0.1), kind: "ok", message: "已排定：2.0 秒后亮出禁用英雄 103" },
    ],
  };
  let suiteRig = {
    connected: true, region: "TENCENT", platform: "HN1",
    installRoot: "C:\\Riot Games\\League of Legends\\TCLS",
    configRoot: "C:\\Riot Games\\League of Legends\\Game\\Config",
    settingsFile: "C:\\Riot Games\\League of Legends\\Game\\Config\\PersistedSettings.json",
    settingsKnown: true, settingsLocked: true, uxState: "Running",
  };
	const facadeSkins = [
    [99000, "拉克丝", true], [99001, "星之守护者 拉克丝", true], [99007, "大元素使 拉克丝", false],
    [103000, "阿狸", true], [103028, "灵魂莲华 阿狸", true], [103071, "K/DA ALL OUT 阿狸", true],
    [222000, "金克丝", true], [222002, "爆竹 金克丝", true],
	].map(([id, name, owned]) => ({ id, name, championId: id < 100000 ? 99 : id < 200000 ? 103 : 222, championName: id < 100000 ? "拉克丝" : id < 200000 ? "阿狸" : "金克丝", splashPath: `/lol-game-data/assets/v1/champion-icons/${id < 100000 ? 99 : id < 200000 ? 103 : 222}.png`, tilePath: `/lol-game-data/assets/v1/champion-icons/${id < 100000 ? 99 : id < 200000 ? 103 : 222}.png`, owned }))
	  .concat(championsCatalogFixture.champions.filter((champion) => ![99, 103, 222].includes(champion.id)).map((champion) => ({ id: champion.id * 1000, name: champion.nameZh, championId: champion.id, championName: champion.nameZh, splashPath: `/lol-game-data/assets/v1/champion-icons/${champion.id}.png`, tilePath: `/lol-game-data/assets/v1/champion-icons/${champion.id}.png`, owned: true })));
  let suiteFacade = {
    connected: true,
    summoner: { ...summoner, summonerLevel: 452 },
    profile: { backgroundSkinId: 99001, backgroundSkinName: "星之守护者 拉克丝" },
	chat: { availability: "chat", statusMessage: "今晚九点峡谷见", lol: { rankedLeagueQueue: "RANKED_SOLO_5X5", rankedLeagueTier: "DIAMOND", rankedLeagueDivision: "II", playerTitleSelected: 101, gameStatus: "outOfGame" } },
	regalia: { preferredBannerType: "lastSeasonHighestRank" },
	challengeSummary: { title: { name: "峡谷先锋", contentId: "demo-title-content-id", itemId: 101 }, topChallenges: [], selectedChallengesString: "", categoryProgress: [] },
	challenges: [{ id: "101", name: "不破不立" }, { id: "202", name: "峡谷收藏家" }, { id: "303", name: "团队之星" }],
	challengesReady: true,
    skins: facadeSkins,
    loginReset: structuredClone(suiteWatch.facade),
  };
  const rewardItem = (id, title, quantity = 1, itemType = "LOOT") => ({ id, itemId: id, itemType, title, quantity, iconUrl: "/lol-game-data/assets/v1/champion-icons/99.png" });
  const suiteClaimItems = [
    { key: "grant:spirit-2023", source: "grant", id: "spirit-2023", rewardGroupId: "choice-a", title: "灵魂莲华 2023 · 通行证赠礼", description: "你从未做出选择，这份发放单一直挂在服务端账本上", dateCreated: "2023-06-14T08:00:00Z", items: [rewardItem("chest-star", "星之守护者宝箱"), rewardItem("be-1350", "1350 蓝色精粹"), rewardItem("keys-3", "海克斯钥匙", 3)], minSelections: 1, maxSelections: 1, historical: true, needsChoice: true },
    { key: "grant:arena-2023", source: "grant", id: "arena-2023", rewardGroupId: "arena-a", title: "斗魂竞技场 · 首赛季参与奖", dateCreated: "2023-07-21T08:00:00Z", items: [rewardItem("arena-avatar", "斗魂头像"), rewardItem("arena-icon", "斗魂图标")], historical: true, needsChoice: false, overlapWith: "event" },
    { key: "grant:pass-orange", source: "grant", id: "pass-orange", rewardGroupId: "pass-a", displayGroup: "pass", title: "通行证奖励", dateCreated: iso(60 * 30), items: [rewardItem("orange-25", "25 橙色精粹", 25)], historical: false, needsChoice: false },
    { key: "grant:pass-blue", source: "grant", id: "pass-blue", rewardGroupId: "pass-b", displayGroup: "pass", title: "通行证奖励", dateCreated: iso(60 * 30), items: [rewardItem("blue-750", "750 蓝色精粹", 750)], historical: false, needsChoice: false },
    { key: "mission:weekly-1", source: "mission", id: "weekly-1", rewardGroupIds: ["weekly-a"], title: "周常任务 · 完成 3 场对局", description: "领取后服务端会解锁链上的下一个任务；工具会重扫但不会自动续领", items: [rewardItem("mission-points", "任务积分", 250)], historical: false, needsChoice: false, chainId: "weekly", chainIndex: 1, chainCount: 3 },
    { key: "event:hextech-2024", source: "event", id: "hextech-2024", title: "海克斯狂欢 2024 · 通行证轨道", description: "事件中心会一次领取当前轨道的全部未领取项", dateCreated: "2024-03-01T08:00:00Z", items: [rewardItem("orb", "海克斯宝珠", 6)], historical: true, needsChoice: false },
    { key: "grant:champion-road", source: "grant", id: "champion-road", rewardGroupId: "road-a", title: "冠军之路 · 段位达成奖励", dateCreated: "2025-11-02T08:00:00Z", items: [rewardItem("victory-skin", "胜利皮肤碎片")], historical: true, needsChoice: false, failure: { statusCode: 400, errorCode: "RewardGrantAlreadyFulfilled", message: "该发放单已被处理", consequence: "重新扫描后可能消失；本批次其它条目不受影响。" } },
    { key: "mission:daily", source: "mission", id: "daily", rewardGroupIds: ["daily-a"], title: "每日首胜奖励", items: [rewardItem("xp", "赛季经验", 400)], historical: false, needsChoice: false },
    { key: "event:summer", source: "event", id: "summer", title: "夏日庆典 · 里程碑", items: [rewardItem("token", "夏日代币", 100)], historical: false, needsChoice: false },
    { key: "grant:honor", source: "grant", id: "honor", rewardGroupId: "honor-a", title: "荣誉等级奖励", items: [rewardItem("honor-capsule", "荣誉胶囊")], historical: false, needsChoice: false },
    { key: "mission:weekly-2", source: "mission", id: "weekly-2", rewardGroupIds: ["weekly-b"], title: "周常任务 · 赢得 1 场对局", items: [rewardItem("mission-points-2", "任务积分", 300)], historical: false, needsChoice: false, chainId: "weekly-b", chainIndex: 2, chainCount: 3 },
    { key: "grant:anniversary", source: "grant", id: "anniversary", rewardGroupId: "anniversary-a", title: "联盟周年纪念奖励", dateCreated: "2022-10-08T08:00:00Z", items: [rewardItem("anniversary-icon", "周年纪念图标")], historical: true, needsChoice: false },
    { key: "grant:choice-small", source: "grant", id: "choice-small", rewardGroupId: "choice-c", title: "赛季材料二选一", items: [rewardItem("orange", "橙色精粹", 500), rewardItem("blue", "蓝色精粹", 1200)], minSelections: 1, maxSelections: 1, historical: false, needsChoice: true },
  ];
  let suiteClaims = { connected: true, scannedAt: iso(0.2), items: suiteClaimItems, sources: { grant: { count: 8, state: "available" }, mission: { count: 3, state: "available" }, event: { count: 2, state: "available" } }, historicalEvidence: true };

  /* ---------- 正在进行的游戏演示（仅 ?demo=current-game） ---------- */
  const currentGamePlayers = [
    [64, "李青", "峡谷先锋", "KR1", "jungle", "win", 4, 11, 8010, 8100, "CHALLENGER", 1284],
    [266, "亚托克斯", "不灭剑魔", "KR2", "top", "", 4, 12, 8010, 8400, "GRANDMASTER", 742],
    [103, "阿狸", "九尾妖狐", "KR3", "middle", "loss", 4, 14, 8128, 8200, "MASTER", 516],
    [145, "卡莎", "虚空之女", "KR4", "bottom", "win", 4, 7, 8005, 8300, "GRANDMASTER", 681],
    [412, "锤石", "魂锁典狱长", "KR5", "utility", "", 4, 14, 8351, 8400, "MASTER", 447],
    [24, "贾克斯", "武器大师", "KR6", "top", "loss", 4, 12, 8010, 8300, "CHALLENGER", 1138],
    [121, "卡兹克", "虚空掠夺者", "KR7", "jungle", "", 4, 11, 8128, 8200, "GRANDMASTER", 809],
    [61, "奥莉安娜", "发条魔灵", "KR8", "middle", "win", 4, 12, 8005, 8200, "MASTER", 593],
    [51, "凯特琳", "皮城女警", "KR9", "bottom", "", 4, 7, 8005, 8300, "GRANDMASTER", 716],
    [235, "赛娜", "涤魂圣枪", "KR10", "utility", "loss", 4, 14, 8351, 8400, "MASTER", 472],
  ].map((entry, index) => ({
    playerRef: `demo-live-player-${index + 1}`,
    championId: entry[0], championName: entry[1], gameName: entry[2], tagLine: entry[3],
    preferredPosition: entry[4], streak: entry[5], summonerLevel: 320 + index * 37,
    spells: [entry[6], entry[7]], runes: [entry[8], entry[9]],
    rank: { tier: entry[10], division: "I", leaguePoints: entry[11] },
    recent: Array.from({ length: 8 }, (_, recentIndex) => ({
      championId: [entry[0], 164, 222, 92][recentIndex % 4],
      championName: [entry[1], "卡蜜尔", "金克丝", "锐雯"][recentIndex % 4],
      win: (recentIndex + index) % 3 !== 1,
      spells: [entry[6], entry[7]],
    })),
  }));
  const currentGame = currentGameDemo ? {
    status: "active", source: "OP.GG", checkedAt: iso(0), startedAt: iso(13), gameId: "demo-current-game",
    queue: "单排/双排", map: "召唤师峡谷",
    teams: [
      { side: "blue", averageRank: { tier: "GRANDMASTER", division: "I" }, averageLP: 734, players: currentGamePlayers.slice(0, 5) },
      { side: "red", averageRank: { tier: "GRANDMASTER", division: "I" }, averageLP: 746, players: currentGamePlayers.slice(5, 10) },
    ],
  } : { status: "none", source: "演示数据", checkedAt: iso(0) };

  /* ---------- fetch 拦截 ---------- */
  const fixtures = new Map([
    ["/api/status", () => status],
    ["/api/chromas", () => ({ items: chromas, count: chromas.length, ownedCount: 2, capability: { name: "chromas", state: "available", count: chromas.length } })],
    ["/api/account", () => account],
    ["/api/gameplay/overview", () => overview],
    ["/api/gameplay/phase", () => ({ phase: "None" })],
    ["/api/gameplay/current-game", () => currentGame],
    ["/api/gameplay/live", () => hextechLiveDemo ? hextechLive : arenaFullDemo ? arenaFullLive : arenaLiveDemo ? arenaLive : live],
    ["/api/gameplay/perks", () => perksCatalog],
    ["/api/gameplay/items", () => demoItemsCatalog],
    ["/api/champions/catalog", () => championsCatalogFixture],
    ["/api/social/friends", friendsFixture],
    ["/api/watch/rules", () => structuredClone(suiteWatch)],
	["/api/champselect/groups", () => structuredClone(suiteChampSelectGroups)],
	["/api/champselect/state", () => structuredClone(suiteChampSelectState)],
    ["/api/rig/status", () => structuredClone(suiteRig)],
    ["/api/facade/state", () => structuredClone(suiteFacade)],
    ["/api/claim/scan", () => structuredClone(suiteClaims)],
  ]);
  const nativeFetch = window.deepLegendsDemoNativeFetch || window.fetch.bind(window);
  window.fetch = (input, init) => {
    const url = typeof input === "string" ? input : input?.url || "";
    const pathname = url.startsWith("/") ? url.split("?")[0] : "";
    const params = new URLSearchParams(url.split("?")[1] || "");
    if (pathname === "/api/champions/catalog" && demoCatalogFailure) {
      return Promise.resolve(new Response("demo catalog failure", { status: 503, headers: { "Content-Type": "text/plain" } }));
    }
    if (pathname === "/api/champions/rankings" && params.get("mode") === "arena") {
      const payload = demoCatalogFailure ? arenaRankingsWithoutCatalog() : arenaRankingsFixture;
      return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/champions/detail" && params.get("mode") === "arena") {
      return Promise.resolve(new Response(JSON.stringify(arenaDetailFixture), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/champions/arena-first-places") {
      const championID = Number(params.get("championId")) || 799;
      const payload = structuredClone(arenaFirstPlacesFixture);
      const meta = arenaDemoChampions.find((champion) => champion.id === championID) || arenaDemoChampions[0];
      for (const match of payload.matches) {
        const subject = match.participants.find((participant) => Number(participant.participantId) === Number(match.subjectParticipantId)) || match.participants[0];
        subject.championId = meta.id;
        subject.championName = meta.nameZh;
        const completeMatch = demoArenaMatchDetails.get(Number(match.gameId));
        const completeSubject = completeMatch?.participants.find((participant) => Number(participant.participantId) === Number(completeMatch.subjectParticipantId)) || completeMatch?.participants[0];
        if (completeSubject) {
          completeSubject.championId = meta.id;
          completeSubject.championName = meta.nameZh;
        }
      }
      return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    const arenaMatchDetail = pathname.match(/^\/api\/champions\/arena\/match\/KR_([0-9]+)$/);
    if (arenaMatchDetail) {
      const payload = demoArenaMatchDetails.get(Number(arenaMatchDetail[1]));
      return Promise.resolve(new Response(payload ? JSON.stringify(structuredClone(payload)) : "Riot 未找到这场对局", { status: payload ? 200 : 404, headers: { "Content-Type": payload ? "application/json" : "text/plain" } }));
    }
    if (pathname === "/api/skins") {
      const view = new URLSearchParams(url.split("?")[1] || "").get("view") || "owned";
      const items = view === "owned" ? ownedSkins : [...ownedSkins, ...unownedSkins];
      return Promise.resolve(new Response(JSON.stringify({ items, count: items.length }), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/gameplay/match-timeline") {
      return Promise.resolve(new Response(JSON.stringify(demoTimeline()), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/gameplay/item-sets/apply") {
      let request = {};
      try { request = JSON.parse(init?.body || "{}"); } catch (_) {}
      return Promise.resolve(new Response(JSON.stringify({ applied: true, verified: true, uid: "deep-legends-demo", title: `DL · ${request.title || "推荐出装"}` }), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/watch/rules" && String(init?.method || "GET").toUpperCase() === "POST") {
      try { suiteWatch = JSON.parse(init?.body || "{}"); } catch (_) {}
      return Promise.resolve(new Response(JSON.stringify(structuredClone(suiteWatch)), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
	if (pathname === "/api/champselect/pause" && String(init?.method || "GET").toUpperCase() === "POST") {
	  try { suiteChampSelectState.sessionPaused = Boolean(JSON.parse(init?.body || "{}").paused); } catch (_) {}
	  return Promise.resolve(new Response(JSON.stringify(structuredClone(suiteChampSelectState)), { status: 200, headers: { "Content-Type": "application/json" } }));
	}
    if (pathname === "/api/rig/settings-lock") {
      try { suiteRig.settingsLocked = Boolean(JSON.parse(init?.body || "{}").locked); } catch (_) {}
      return Promise.resolve(new Response(JSON.stringify(structuredClone(suiteRig)), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/rig/maintenance") {
      return Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/facade/apply") {
      let request = {};
      try { request = JSON.parse(init?.body || "{}"); } catch (_) {}
      if (request.action === "background") suiteFacade.profile.backgroundSkinId = request.skinId;
      if (request.action === "chat") { suiteFacade.chat.availability = request.availability || suiteFacade.chat.availability; suiteFacade.chat.statusMessage = request.statusMessage ?? suiteFacade.chat.statusMessage; }
      if (request.action === "rank") suiteFacade.chat.lol = { rankedLeagueQueue: request.queue, rankedLeagueTier: request.tier, rankedLeagueDivision: request.division };
      if (request.action === "login-reset") { suiteFacade.loginReset = request.loginReset; suiteWatch.facade = request.loginReset; }
      return Promise.resolve(new Response(JSON.stringify(structuredClone(suiteFacade)), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    if (pathname === "/api/claim/execute") {
      let request = {};
      try { request = JSON.parse(init?.body || "{}"); } catch (_) {}
      const key = request.items?.[0]?.key || "";
      const item = suiteClaims.items.find((entry) => entry.key === key);
      const failed = item?.failure;
      if (!failed) suiteClaims.items = suiteClaims.items.filter((entry) => entry.key !== key);
      suiteClaims.scannedAt = new Date().toISOString();
      const result = failed ? { key, ok: false, ...failed } : { key, ok: true };
      return Promise.resolve(new Response(JSON.stringify({ results: [result], succeeded: failed ? 0 : 1, failed: failed ? 1 : 0, scan: structuredClone(suiteClaims) }), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    const fixture = fixtures.get(pathname);
    if (fixture) return Promise.resolve(new Response(JSON.stringify(fixture()), { status: 200, headers: { "Content-Type": "application/json" } }));
    return nativeFetch(input, init);
  };

  /* ---------- 演示角标 ---------- */
  document.body.classList.add("is-demo");
  const flag = document.createElement("div");
  flag.className = "demo-flag";
  flag.textContent = "演示数据 · 仅样式预览";
  if (document.body) document.body.appendChild(flag);
  else document.addEventListener("DOMContentLoaded", () => document.body.appendChild(flag));
  window.deepLegendsDemoReady?.();
  delete window.deepLegendsDemoReady;
  delete window.deepLegendsDemoNativeFetch;
})();
