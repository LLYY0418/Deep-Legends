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
  try { if (query.has("demo") || location.hash.includes("demo")) localStorage.setItem("lol-loot-demo", "1"); } catch (_) {}

  const now = Date.now();
  const iso = (minutesAgo) => new Date(now - minutesAgo * 60_000).toISOString();

  /* ---------- 通用素材映射（把 LCU 图标路径改写到本地代理的公共 CDN 源） ---------- */
  const PATCH = "16.16.1";
  const championKeys = {
    103: "Ahri", 222: "Jinx", 157: "Yasuo", 92: "Riven", 145: "Kaisa", 99: "Lux",
    412: "Thresh", 64: "LeeSin", 164: "Camille", 81: "Ezreal", 24: "Jax", 254: "Vi",
    238: "Zed", 111: "Nautilus", 266: "Aatrox", 517: "Sylas", 875: "Sett", 39: "Irelia",
    121: "Khazix", 61: "Orianna", 51: "Caitlyn", 235: "Senna",
  };
  const spellNames = { 1: "SummonerBoost", 3: "SummonerExhaust", 4: "SummonerFlash", 6: "SummonerHaste", 7: "SummonerHeal", 11: "SummonerSmite", 12: "SummonerTeleport", 14: "SummonerDot", 21: "SummonerBarrier", 32: "SummonerSnowball" };
  const perkPaths = {
    8005: "/cdn/img/perk-images/Styles/Precision/PressTheAttack/PressTheAttack.png",
    8010: "/cdn/img/perk-images/Styles/Precision/Conqueror/Conqueror.png",
    8128: "/cdn/img/perk-images/Styles/Domination/DarkHarvest/DarkHarvest.png",
    8437: "/cdn/img/perk-images/Styles/Resolve/GraspOfTheUndying/GraspOfTheUndying.png",
    8351: "/cdn/img/perk-images/Styles/Inspiration/GlacialAugment/GlacialAugment.png",
    5001: "/cdn/img/perk-images/StatMods/StatModsHealthScalingIcon.png",
    5005: "/cdn/img/perk-images/StatMods/StatModsAttackSpeedIcon.png",
    5007: "/cdn/img/perk-images/StatMods/StatModsCDRScalingIcon.png",
    5008: "/cdn/img/perk-images/StatMods/StatModsAdaptiveForceIcon.png",
    5010: "/cdn/img/perk-images/StatMods/StatModsMovementSpeedIcon.png",
    5011: "/cdn/img/perk-images/StatMods/StatModsHealthPlusIcon.png",
    5013: "/cdn/img/perk-images/StatMods/StatModsTenacityIcon.png",
  };
  const proxied = (path) => `/api/champion-asset?source=ddragon&path=${encodeURIComponent(path)}`;

  function rewriteImage(image) {
    const src = image.getAttribute("src") || "";
    if (!src.startsWith("/api/image?path=")) return;
    const assetPath = decodeURIComponent(src.slice("/api/image?path=".length));
    let match = assetPath.match(/champion-icons\/(\d+)\.png$/);
    if (match && championKeys[match[1]]) {
      image.src = `/api/champion-asset?source=gtimg&path=${encodeURIComponent(`/images/lol/act/img/champion/${championKeys[match[1]]}.png`)}`;
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
  const summoner = { displayName: "青钢影不加班", gameName: "青钢影不加班", tagLine: "CN1", profileIconId: 4568, summonerLevel: 436 };

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
        { lootId: "CHEST_GENERIC", displayName: "海克斯科技宝箱", category: "材料", kind: "宝箱", count: 4 },
        { lootId: "CHEST_224", displayName: "杰作宝箱", category: "材料", kind: "宝箱", count: 1 },
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
        { name: "loot", state: "available", count: 15, detail: "本机客户端已返回数据" },
        { name: "rewards", state: "available", count: 3, detail: "本机客户端已返回数据" },
        { name: "store", state: "unsupported", count: 0, detail: "当前客户端不支持，已降级" },
      ],
    },
  };

  const participant = (participantId, teamId, championId, championName, kills, deaths, assists, win) => ({
    participantId, teamId, championId, championName, championLevel: 15 + (participantId % 4),
    spell1Id: 4, spell2Id: teamId === 100 ? 12 : 14,
    primaryStyleId: participantId % 2 ? 8000 : 8100, subStyleId: 8400,
    perkIds: participantId % 2 ? [8010, 9111, 9104, 8299, 8446, 8453, 5005, 5008, 5011] : [8128, 8139, 8138, 8135, 8473, 8453, 5008, 5008, 5001],
    kills, deaths, assists, kda: deaths ? Number(((kills + assists) / deaths).toFixed(2)) : kills + assists,
    cs: 180 + participantId * 7, laneCs: 150 + participantId * 6, jungleCs: 30 + participantId,
    csPerMinute: Number((5 + ((participantId * 7) % 20) / 10).toFixed(1)), damage: 14_000 + participantId * 2_300, damageTaken: 12_000 + ((participantId * 3_137) % 14_000),
    gold: 8_800 + participantId * 620 + (win ? 1_400 : 0), visionScore: 11 + ((participantId * 13) % 34),
    wardsPlaced: 9, wardsKilled: 3, itemIds: [6630, 3071, 3053, 3065, 3026, 0, 3340],
    playerRef: "", gameName: `演示玩家${participantId}`, tagLine: "DEMO", multiKill: participantId === 1 ? 3 : 0, win,
  });
  const demoMatch = (gameId, minutesAgo, win, subjectChampion, lpDelta) => ({
    gameId, queueId: 420, queueLabel: "单排/双排", modeGroup: "solo", result: win ? "win" : "loss",
    createdAt: now - minutesAgo * 60_000, duration: 1_820, subjectParticipantId: 1,
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
    ],
  });
  // 斗魂竞技场演示对局：21 名玩家、3 人一队共 7 支小队，带名次。
  // ID、品质与图标路径来自 Riot 的 cherry-augments 中文目录；描述仅用于演示 tooltip。
  const arenaAugmentCatalog = [
    { id: 1205, name: "物理转魔法", rarity: "kSilver", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/ADAPt_small.png", description: "将额外攻击力转化为法术强度。" },
    { id: 1141, name: "全心为你", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/AllForYou_small.png", description: "强化你为友军提供的治疗与护盾效果。" },
    { id: 1002, name: "尖端发明家", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/ApexInventor_small.png", description: "你的装备技能获得大量技能急速。" },
    { id: 2087, name: "大法师", rarity: "kPrismatic", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Eureka_small.png", description: "根据最大法力值获得额外法术强度。" },
    { id: 1004, name: "回归基本功", rarity: "kPrismatic", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BackToBasics_small.png", description: "禁用终极技能，并大幅强化基础技能。" },
    { id: 2103, name: "狙神飞星", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/QuestBangBang_small.png", description: "从远距离命中敌人时造成额外伤害。" },
    { id: 1180, name: "超强大脑", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BigBrain_small.png", description: "战斗开始时获得基于法术强度的护盾。" },
    { id: 1006, name: "利刃华尔兹", rarity: "kPrismatic", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BladeWaltz_small.png", description: "获得利刃华尔兹，突进并连续攻击附近敌人。" },
    { id: 1007, name: "大力", rarity: "kSilver", iconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/BluntForce_small.png", description: "获得额外攻击力。" },
    { id: 1103, name: "面包和黄油", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/GenericAbilityAugmentIcon_Gold.png", description: "你的 Q 技能获得大量技能急速。" },
    { id: 1151, name: "面包和奶酪", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/GenericAbilityAugmentIcon_Gold.png", description: "你的 E 技能获得大量技能急速。" },
    { id: 1150, name: "面包和果酱", rarity: "kGold", iconPath: "/lol-game-data/assets/ASSETS/UX/Kiwi/Augments/Icons/GenericAbilityAugmentIcon_Gold.png", description: "你的 W 技能获得大量技能急速。" },
  ];
  const arenaAugmentIDs = arenaAugmentCatalog.map((augment) => augment.id);
  const arenaChampionPool = [[164, "卡蜜尔"], [103, "阿狸"], [222, "金克丝"], [64, "李青"], [412, "锤石"], [24, "贾克斯"], [254, "蔚"], [238, "劫"], [145, "卡莎"], [157, "亚索"], [92, "锐雯"], [111, "深海泰坦"], [266, "亚托克斯"], [517, "塞拉斯"], [875, "瑟提"], [39, "艾瑞莉娅"], [121, "卡兹克"], [61, "奥莉安娜"], [51, "凯特琳"], [235, "赛娜"], [99, "拉克丝"]];
  const demoArenaMatch = (gameId, minutesAgo) => {
    const placements = [2, 1, 3, 4, 5, 6, 7];
    const participants = arenaChampionPool.map(([championId, championName], index) => {
      const subteamId = Math.floor(index / 3) + 1;
      const placement = placements[subteamId - 1];
      const win = placement <= 4;
      return {
        ...participant(index + 1, index < 12 ? 100 : 200, championId, championName, 3 + (index % 9), 2 + (index % 6), 5 + (index % 8), win),
        subteamId, placement, position: "",
        augmentIds: [0, 3, 6, 9].map((offset) => arenaAugmentIDs[(index + offset) % arenaAugmentIDs.length]),
        cs: 0, laneCs: 0, jungleCs: 0, csPerMinute: 0, wardsPlaced: 0, wardsKilled: 0, visionScore: 0,
        spell1Id: 4, spell2Id: 32, itemIds: [6630, 3071, 3053, 3065, 3026, 6653, 0],
      };
    });
    return {
      gameId, queueId: 1700, queueLabel: "斗魂竞技场", modeGroup: "arena", result: "win",
      createdAt: now - minutesAgo * 60_000, duration: 1_140, subjectParticipantId: 1,
      averageTier: { tier: "PLATINUM", division: "I", samples: 18 },
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
    player: { playerRef: "", displayName: summoner.displayName, gameName: summoner.gameName, tagLine: summoner.tagLine, profileIconId: summoner.profileIconId, summonerLevel: summoner.summonerLevel, hidden: false, isCurrent: true },
    ranks: [
      { queueType: "RANKED_SOLO_5x5", tier: "emerald", division: "II", leaguePoints: 47, wins: 68, losses: 55, winRate: 55.3 },
      { queueType: "RANKED_FLEX_SR", tier: "platinum", division: "I", leaguePoints: 82, wins: 31, losses: 28, winRate: 52.5 },
    ],
    championStats: [
      { championId: 164, championName: "卡蜜尔", games: 42, winRate: 59.5, kda: 3.4, kills: 7.1, deaths: 3.8, assists: 5.9, cs: 201, csPerMinute: 6.9 },
      { championId: 103, championName: "阿狸", games: 25, winRate: 56.0, kda: 3.9, kills: 8.2, deaths: 3.1, assists: 6.4, cs: 188, csPerMinute: 6.5 },
      { championId: 222, championName: "金克丝", games: 18, winRate: 50.0, kda: 2.8, kills: 6.6, deaths: 4.5, assists: 6.1, cs: 210, csPerMinute: 7.2 },
    ],
    overall: { games: 85, winRate: 55.3, kda: 3.3, kills: 7.3, deaths: 3.9, assists: 6.0, cs: 199, csPerMinute: 6.8 },
    positions: [
      { position: "top", label: "上单", share: 46 },
      { position: "middle", label: "中单", share: 28 },
      { position: "bottom", label: "下路", share: 16 },
      { position: "jungle", label: "打野", share: 10 },
    ],
    masteries: [
      { championId: 164, championName: "卡蜜尔", championPoints: 486_200, championLevel: 12 },
      { championId: 103, championName: "阿狸", championPoints: 355_400, championLevel: 10 },
      { championId: 222, championName: "金克丝", championPoints: 268_000, championLevel: 9 },
      { championId: 64, championName: "李青", championPoints: 142_300, championLevel: 7 },
    ],
    sevenDayRank: { games: 23, wins: 14, losses: 9, winRate: 60.9, kda: 3.6, sampled: false },
    recentPlayers: [
      { profileIconId: 5205, gameName: "永远滴神李青", tagLine: "CN1", games: 12, playerRef: "" },
      { profileIconId: 6296, gameName: "狐狸不吃鱼", tagLine: "CN2", games: 8, playerRef: "" },
      { profileIconId: 3543, gameName: "魂锁典狱长", tagLine: "CN1", games: 6, playerRef: "" },
    ],
    activityHours: [0, 0, 0, 0, 0, 0, 0, 1, 2, 1, 0, 2, 3, 2, 4, 3, 2, 1, 4, 6, 8, 7, 5, 2],
    matches: [demoMatch(90001, 42, true, [164, "卡蜜尔"], 24), demoMatch(90002, 190, false, [103, "阿狸"], -18), demoArenaMatch(90004, 60 * 7), demoMatch(90003, 60 * 26, true, [164, "卡蜜尔"])],
    pagination: { begIndex: 0, count: 4, hasMore: false },
    capabilities: [
      { name: "summoner", state: "available", count: 1 },
      { name: "match-history", state: "available", count: 3 },
      { name: "ranked", state: "available", count: 2 },
      { name: "mastery", state: "available", count: 4 },
    ],
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
  const livePlayer = (championId, championName, position, teamId, isCurrent, tier, wins, losses, kdaValue, seed) => ({
    championId, championName, position, teamId, isCurrent,
    playerRef: "", gameName: isCurrent ? summoner.gameName : `演示玩家`, tagLine: "DEMO",
    rank: { tier, division: "II", leaguePoints: 45 },
    modeStats: { games: wins + losses, wins, losses, winRate: Number((wins * 100 / (wins + losses)).toFixed(1)), kda: kdaValue },
    recentGames: recentGamesFor(championId, championName, wins, losses, seed),
  });
  const runePage = (key, title, primaryStyleId, subStyleId, perkIds, statModIds, stats) => ({ key, title, primaryStyleId, subStyleId, selectedPerkIds: [...perkIds, ...statModIds], statModIds, stats });
  const live = {
    available: true, phase: "ChampSelect", queueLabel: "单排/双排", gameMode: "经典模式", mapId: 11, gameId: 0,
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
      hero: {
        winRate: 0.5213, pickRate: 0.082, banRate: 0.064,
        strongAgainst: [{ championId: 24, championName: "贾克斯" }, { championId: 238, championName: "劫" }, { championId: 92, championName: "锐雯" }],
        weakAgainst: [{ championId: 111, championName: "深海泰坦" }, { championId: 412, championName: "锤石" }],
      },
      runes: {
        opgg: [
          runePage("opgg", "征服者 · 精密 + 坚决", 8000, 8400, [8010, 9111, 9104, 8299, 8446, 8453], [5005, 5008, 5011], { pickRate: 0.759, play: 3521, winRate: 0.4962 }),
          runePage("opgg-1", "强攻 · 精密 + 主宰", 8000, 8100, [8005, 9101, 9105, 8014, 8139, 8143], [5008, 5010, 5001], { pickRate: 0.176, play: 815, winRate: 0.5166 }),
        ],
        specialists: [
          { ...runePage("specialist-0", "목숨뿐#아초록스", 8000, 8400, [8010, 9111, 9104, 8299, 8473, 8453], [5005, 5008, 5011], { games: 1451, winRate: 51 }), playerName: "목숨뿐", tagLine: "아초록스", championName: "卡蜜尔", tier: "master", leaguePoints: "211", championGames: 1451, playedAt: now - 3 * 86_400_000, result: "win", region: "kr" },
        ],
        pros: [],
      },
      build: {
        position: "top",
        skillPriority: ["Q", "E", "W"],
        skillOrder: ["Q", "W", "E", "Q", "Q", "R", "Q", "E", "Q", "E", "R", "E", "E", "W", "W"],
        skillStats: { pickRate: 0.612, play: 21_384, winRate: 0.536 },
        spellOptions: [
          { ids: [4, 12], stats: { pickRate: 0.782, play: 29_884, winRate: 0.541 } },
          { ids: [4, 14], stats: { pickRate: 0.146, play: 5_580, winRate: 0.523 } },
        ],
        starterOptions: [
          { ids: [1054, 2003], stats: { pickRate: 0.566, winRate: 0.5095 } },
          { ids: [1055, 2003], stats: { pickRate: 0.217, winRate: 0.5248 } },
        ],
        bootOptions: [
          { ids: [3047], stats: { pickRate: 0.496, winRate: 0.5287 } },
          { ids: [3111], stats: { pickRate: 0.301, winRate: 0.4834 } },
        ],
        itemRoutes: [
          { ids: [6630, 3071, 3053, 6333, 3065], stats: { pickRate: 0.2982, play: 8_417, winRate: 0.5863 } },
          { ids: [6630, 3065, 3071, 3742, 3053], stats: { pickRate: 0.2309, play: 5_203, winRate: 0.5516 } },
          { ids: [6630, 3071, 6333, 3143, 3026], stats: { pickRate: 0.1218, play: 3_946, winRate: 0.4946 } },
        ],
      },
    },
    championAbilities: [
      { slot: "Q", name: "精准礼仪", iconPath: "/lol-game-data/assets/demo-spell/CamilleQ.png", description: "下次攻击造成额外伤害；再次施放造成真实伤害。" },
      { slot: "W", name: "战术横扫", iconPath: "/lol-game-data/assets/demo-spell/CamilleW.png", description: "扇形横扫，外缘命中减速并回复生命。" },
      { slot: "E", name: "钩索", iconPath: "/lol-game-data/assets/demo-spell/CamilleE.png", description: "钩住地形二段冲刺，命中英雄眩晕。" },
      { slot: "R", name: "海克斯最后通牒", iconPath: "/lol-game-data/assets/demo-spell/CamilleR.png", description: "锁定目标形成决斗领域。" },
    ],
  };

  /* 完整符文树（真实符文 ID 与官方图标路径，供对局符文板展示） */
  const perkEntry = (id, name, icon) => ({ id, name, iconPath: `/lol-game-data/assets/v1/perk-images/${icon}` });
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
    perks: [],
    augments: arenaAugmentCatalog,
  };

  /* ---------- 好友面板演示数据 ---------- */
  const demoFriend = (gameName, tagLine, icon, groupId, availability, extra = {}) => ({
    puuid: `demo-${gameName}`, summonerId: 0, gameName, tagLine, icon, availability,
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

  /* ---------- fetch 拦截 ---------- */
  const fixtures = new Map([
    ["/api/status", () => status],
    ["/api/chromas", () => ({ items: chromas, count: chromas.length, ownedCount: 2, capability: { name: "chromas", state: "available", count: chromas.length } })],
    ["/api/account", () => account],
    ["/api/gameplay/overview", () => overview],
    ["/api/gameplay/live", () => live],
    ["/api/gameplay/perks", () => perksCatalog],
    ["/api/social/friends", friendsFixture],
  ]);
  const nativeFetch = window.fetch.bind(window);
  window.fetch = (input, init) => {
    const url = typeof input === "string" ? input : input?.url || "";
    const pathname = url.startsWith("/") ? url.split("?")[0] : "";
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
      return Promise.resolve(new Response(JSON.stringify({ applied: true, verified: true, uid: "deep-legends-demo", title: `Deep Legends · ${request.title || "推荐出装"}` }), { status: 200, headers: { "Content-Type": "application/json" } }));
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
})();
