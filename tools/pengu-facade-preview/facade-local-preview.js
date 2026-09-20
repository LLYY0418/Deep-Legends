/**
 * facade-local-preview.js —— 未拥有头像/旗帜「仅本地可见」实验脚本
 *
 * 用途
 *   在 LoL 客户端里把「你选的头像/旗帜」渲染出来，包括你**没有拥有**的那些。
 *   只改客户端读到的数据，不改服务端真值：生涯页、藏品、选人页、好友栏、hovercard
 *   本地都会显示成目标外观。
 *
 * 原理（L1：响应改写）
 *   客户端所有界面都消费同一批 LCU 端点。我们在客户端进程内拦下三类数据通道，
 *   把响应体里的头像/旗帜字段替换成目标值，客户端就会当成真值去渲染：
 *     1) XMLHttpRequest  —— RCP databinding 的主要通道
 *     2) window.fetch    —— 部分新 bundle 走这条
 *     3) WebSocket 事件帧 [8, endpoint, {uri, eventType, data}] —— 实时推送，
 *        不改这条的话，客户端下一次 OnUpdate 就会把我们的改写冲掉
 *
 * 运行环境
 *   Pengu Loader（https://github.com/PenguLoader/PenguLoader）。脚本以 ES module
 *   方式被 import，导出 init(context) / load()，context = { rcp, socket }。
 *   契约已对照 PenguLoader 源码核对：plugins/src/preload/loader.ts、rcp/socket.ts。
 *
 * 安装
 *   1. 装 Pengu Loader，打开「Pengu Loader/scripts」目录
 *   2. 把本文件放进去：scripts/facade-local-preview.js
 *      （也可以放成 scripts/facade-preview/index.js，两种都能被加载）
 *   3. 改下面 CONFIG.icon.id / CONFIG.banner.id
 *   4. 重启 LoL 客户端，按 F12（或 window.openDevTools()）看 Console 里的 [FacadePreview] 日志
 *
 * 边界与风险（务必知悉）
 *   - 这是**修改客户端行为**，属 Riot ToS 灰区；风险由使用者承担。不改游戏文件、不联网、
 *     不上传任何数据、不写持久化存储。
 *   - 别人看不到。服务端真值没变，好友那边看到的还是你真实的头像旗帜。
 *   - 已知副作用：同一客户端里的**其它 Pengu 插件**若读这些端点，也会读到改写后的值。
 *     介意就把 CONFIG.rewriteWebSocket 设 false（代价：客户端推送会把界面冲回真实值）。
 *   - 拥有态被改写后，客户端选择器里未拥有项不再置灰——这是刻意的，不是 bug。
 *   - 端点名/字段名会随客户端版本漂移。失效时先开 diagnostic 打印结构再对照修，
 *     不要凭猜测改字段名。
 *   - 关闭办法见 README「关闭」一节：只关 icon/banner 的 enabled **不足以**停止拦截，
 *     必须同时关掉三个 rewrite* 开关或直接删掉脚本文件。
 *
 * 参考实现（均已核对源码，非推测）
 *   sona  src/lib/xhr/core.ts            —— XHR 改写思路来源（该实现用 addEventListener
 *           + defineProperty 写死值；本脚本改为惰性 getter，理由见「通道 1」注释）
 *   sona  src/lib/xhr/profile-privacy-rules.ts —— 改写 LCU 响应骗过客户端 UI 的先例
 *   sona  src/lib/features/rank-disguise.ts    —— presence(lol.*) 可写且不校验的证据
 *   akari src/shared/types/league-client/chat.ts —— ChatLol 字段表
 *   PenguLoader plugins/src/preload/loader.ts、rcp/socket.ts —— 生命周期与 socket 契约
 */

// ============================================================
// 配置：只需要改这里
// ============================================================
const CONFIG = {
  // 目标头像 ID。填未拥有的也可以，例如 4379。填 0 表示不改头像。
  icon: { enabled: true, id: 0 },

  // 目标旗帜 ID。注意是 regalia 目录里的**字符串 id**（不是 contentId、不是 idSecondary）。
  // 例：'12'。留空表示不改旗帜。只接受 [A-Za-z0-9_-]，其它输入会被拒绝（防原型污染）。
  banner: { enabled: false, id: '' },

  // 三个数据通道的开关。排查时可逐个关掉定位是哪条通道在起作用。
  // 注意：这三个才是「是否拦截」的总闸，关掉它们才真正停止改写。
  rewriteXhr: true,
  rewriteFetch: true,
  rewriteWebSocket: true,

  // 诊断模式：把命中端点的结构（顶层键 + 含 icon/banner/regalia 的键名）记进 stats()。
  // 第一次跑建议开着——旗帜在若干端点里的字段名尚未实证，用它一次性把契约定下来。
  // 关掉后 summarize 完全不执行，不付任何扫描开销。
  diagnostic: true,

  // 客户端 window load 后自动弹 DevTools（本轮验证用，省得找入口）。嫌烦设 false，
  // 改用在客户端窗口按 Ctrl+K 唤出 Pengu 命令栏 → 选「打开开发者工具」。
  autoOpenDevTools: true,

  // DOM 层（v0.4.2）：生涯页头像/旗帜组件不吃数据推送（显示时才发请求或直接按属性渲染），
  // 诊断扫描会把「自己」那些 regalia 元素的标签与全部属性打出来（拿死属性词汇表），
  // 并对已存在 banner-id / 头像类属性的元素直接设目标值。真机验证项。
  domPatch: false,   // 属性补丁默认关：它会触发客户端自己的保存路径，撞服务器所有权墙（第十一轮实证 400 RPC_ERROR）

  // 像素层补丁（v0.5.0，默认开）：只改 shadow DOM 里渲染出来的 img src，
  // 不碰任何输入属性、不触发保存、不撞服务器所有权墙。纯本地可见。
  pixelPatch: true,

  // 日志前缀，方便在 Console 里过滤。
  tag: '[FacadePreview]',
}

// ============================================================
// 运行时状态
// ============================================================
const SCRIPT_VERSION = 'v0.5.0'

const state = {
  ownPuuid: '',
  ownSummonerId: 0,
  installed: { xhr: false, fetch: false, ws: false },
  counts: {},            // ruleId -> 实际改写次数（每个通道每次改写只计一次）
  seen: {},              // ruleId -> 最近一次命中时的**服务端真值**结构摘要（改写前采集）
  warnings: [],          // 收集告警，供 dump() 一次性导出（上限 200 条，防无界增长）
  wsPathFragments: [],   // WS 预筛片段表：新增规则必须同步维护（否则该端点推送不会被改写）
  pushTargets: [],       // (ws, 原始监听器) 引用表：applyNow() 伪造推送用，上限 64
  lastTruth: {},         // 让位机制：每种预览最近一次观察到的服务端真值（挂 state 上便于诊断与测试重置）
  socket: null,
  rcp: null,
}

const log = (...args) => console.info(CONFIG.tag, ...args)

// 把 args 压成一行短文本，只用于告警记录，绝不整体序列化大对象
function flatten(args) {
  return args.map((a) => {
    if (a instanceof Error) return `${a.name}: ${a.message}`
    if (typeof a === 'object' && a !== null) {
      try { return JSON.stringify(a).slice(0, 160) } catch { return String(a) }
    }
    return String(a)
  }).join(' ').slice(0, 240)
}

function warn(...args) {
  if (state.warnings.length < 200) state.warnings.push(flatten(args))
  console.warn(CONFIG.tag, ...args)
}
const diag = (...args) => { if (CONFIG.diagnostic) console.info(CONFIG.tag, '[diag]', ...args) }

// getter 与 WS 回调会被客户端高频调用，同类异常只报一次，避免刷爆控制台拖慢客户端
const warnedKeys = new Set()
function warnOnce(key, ...args) {
  if (warnedKeys.has(key)) return
  warnedKeys.add(key)
  warn(key, ...args)
}

// ---------- 选择持久化 ----------
// 顶部栏、好友列表上方等界面在**启动时**读取并缓存，中途 setIcon 追不上（第三/四轮实测）。
// 把选择记下来，重启客户端后从第一个请求起就改写，全界面一次到位。
// 存储优先用 Pengu 的 DataStore，退路 localStorage；都不可用则只在内存（本轮生效）。
const STORE_KEY = 'facadePreviewSelection'

function readPersisted() {
  try {
    const ds = window.DataStore
    if (ds && typeof ds.get === 'function') {
      const v = ds.get(STORE_KEY, null)
      if (v && typeof v === 'object') return v
    }
  } catch { /* DataStore 不可用，退路 */ }
  try {
    const raw = window.localStorage ? window.localStorage.getItem(STORE_KEY) : null
    if (raw) {
      const v = JSON.parse(raw)
      if (v && typeof v === 'object') return v
    }
  } catch { /* localStorage 不可用 */ }
  return null
}

function writePersisted(sel) {
  try {
    const ds = window.DataStore
    if (ds && typeof ds.set === 'function') { ds.set(STORE_KEY, sel); return true }
  } catch { /* 退路 */ }
  try {
    if (window.localStorage) { window.localStorage.setItem(STORE_KEY, JSON.stringify(sel)); return true }
  } catch { /* 忽略 */ }
  return false
}

function currentSelection() {
  return {
    iconId: CONFIG.icon.enabled ? CONFIG.icon.id : 0,
    bannerId: CONFIG.banner.enabled ? CONFIG.banner.id : '',
  }
}

function applyPersistedSelection() {
  const v = readPersisted()
  if (!v) return false
  if (Number(v.iconId) > 0) { CONFIG.icon.enabled = true; CONFIG.icon.id = Number(v.iconId) }
  if (typeof v.bannerId === 'string' && v.bannerId) { CONFIG.banner.enabled = true; CONFIG.banner.id = v.bannerId }
  return true
}

const restoredSelection = applyPersistedSelection()

// 模块被 import 就会打这一行。Console 里**连这行都没有** = Pengu 根本没加载本文件
// （目录形状不对 / 客户端没重启 / 插件被禁用），不是脚本逻辑问题。
console.info(CONFIG.tag, 'module evaluated', SCRIPT_VERSION,
  restoredSelection ? `（已恢复上次选择：头像 ${CONFIG.icon.id || '无'} / 旗帜 ${CONFIG.banner.id || '无'}）` : '（无持久化选择）')

function bump(ruleId) {
  state.counts[ruleId] = (state.counts[ruleId] || 0) + 1
}

/** 只认自有属性，且拒绝 __proto__ / constructor / prototype 这类危险键。 */
function safeOwn(obj, key) {
  if (!obj || typeof obj !== 'object') return false
  if (key === '__proto__' || key === 'constructor' || key === 'prototype') return false
  return Object.prototype.hasOwnProperty.call(obj, key)
}

/** CONFIG 里的目标 id 必须形如普通标识符，否则一律拒绝。 */
function safeToken(value) {
  return typeof value === 'string' && /^[A-Za-z0-9_-]+$/.test(value)
}

// ============================================================
// 路径归一化与规则匹配
// ============================================================

/** 把任意形式的 URL 归一成纯 path（去掉 origin、query、fragment），失败返回空串。 */
function normalizePath(rawUrl) {
  let url
  if (typeof rawUrl === 'string') url = rawUrl
  else if (rawUrl instanceof URL) url = rawUrl.href
  else if (rawUrl && typeof rawUrl === 'object' && typeof rawUrl.url === 'string') url = rawUrl.url
  else url = String(rawUrl || '')
  if (!url) return ''
  try {
    if (/^https?:\/\//i.test(url)) url = new URL(url).pathname
    else url = url.split(/[?#]/)[0]
  } catch {
    url = url.split(/[?#]/)[0]
  }
  if (!url.startsWith('/')) url = '/' + url
  // 去掉尾部斜杠，但保留根路径
  if (url.length > 1 && url.endsWith('/')) url = url.slice(0, -1)
  return url
}

/** 路径里的 puuid / 长数字 id 脱敏：dump 与 diag 都会落这些字符串，不能带真值。 */
function redactPath(path) {
  return String(path || '')
    .replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, '{puuid}')
    .replace(/\/\d{6,}(?=\/|$)/g, '/{id}')
}

/** 有界的值摘要：绝不整体 JSON.stringify（响应可能有几百条）。 */
function shortValue(v) {
  if (v === null) return 'null'
  if (typeof v !== 'object') return String(v)
  if (Array.isArray(v)) return `[array len=${v.length}]`
  return `{${Object.keys(v).slice(0, 8).join(',')}}`
}

/**
 * 采集响应结构摘要，用于定契约。**必须在 apply 改写之前调用**，
 * 这样 state.seen 里存的始终是服务端真值，而不是我们伪造后的值。
 */
function summarize(ruleId, path, json) {
  if (!CONFIG.diagnostic) return          // 关掉诊断就完全不扫，不付开销
  if (!json || typeof json !== 'object') return
  const keys = Array.isArray(json) ? `[array len=${json.length}]` : Object.keys(json).join(',')
  const interesting = []
  const scan = (obj, prefix) => {
    if (!obj || typeof obj !== 'object') return
    for (const k of Object.keys(obj)) {
      if (!/icon|banner|regalia|owned|crest|title/i.test(k)) continue
      interesting.push(`${prefix}${k}=${shortValue(obj[k]).slice(0, 80)}`)
    }
  }
  if (Array.isArray(json)) {
    scan(json[0], '[0].')
  } else {
    scan(json, '')
    // myTeam/theirTeam 这类嵌套结构也扫一层
    for (const k of ['myTeam', 'theirTeam', 'lol']) {
      if (Array.isArray(json[k])) scan(json[k][0], `${k}[0].`)
      else if (json[k] && typeof json[k] === 'object') scan(json[k], `${k}.`)
    }
  }
  // path 里可能带 puuid / summonerId（hovercard），落盘与打日志前必须脱敏
  const safePath = redactPath(path)
  state.seen[ruleId] = { path: safePath, keys, interesting }
  diag(ruleId, safePath, '\n  keys:', keys, '\n  相关字段(服务端真值):',
    interesting.length ? interesting.join(' | ') : '(无)')
}

// ============================================================
// 归属判定：只允许改「自己」的记录
// ============================================================
const HOVERCARD_PREFIX = '/lol-hovercard/v1/friend-info/'

/** hovercard 路径里的 puuid 段是不是自己（按段比较，不是字符串后缀）。 */
function hovercardIsSelf(path) {
  const own = String(state.ownPuuid || '').toLowerCase()
  if (!own) return false
  const seg = path.slice(HOVERCARD_PREFIX.length).split('/')[0].toLowerCase()
  return !!seg && seg === own
}

/** 选人页成员是不是自己。puuid 优先；puuid 缺失时才退回 summonerId。 */
function memberIsSelf(member) {
  if (!member || typeof member !== 'object') return false
  const ownPuuid = String(state.ownPuuid || '').toLowerCase()
  const memberPuuid = String(member.puuid || '').toLowerCase()
  if (ownPuuid && memberPuuid) return memberPuuid === ownPuuid
  // 走到这里说明至少一方没有 puuid 可比，退回 summonerId（同样是账号唯一标识）。
  // 两个 id 都拿不到就判定为「不是自己」——宁可不生效，也不能改到别人头上。
  const ownId = Number(state.ownSummonerId || 0)
  const memberId = Number(member.summonerId || 0)
  if (!ownId || !memberId) return false
  return memberId === ownId
}

// ============================================================
// 各端点的改写实现。返回 true 表示确实改动了
// ============================================================

// ---------- 让位机制（v0.4.3）----------
// 预览绝不能压过客户端原生功能（第八轮使用者反馈：原生切换头像/旗帜被预览顶掉）。
// 我们的改写只作用于响应副本，从不改变服务端真值；因此「服务端真值发生变化」
// 只可能来自使用者在客户端的原生操作。一旦检测到，立即停用对应种类的预览、
// 清除持久化、还原 DOM 补丁，客户端功能随即恢复。

const domPatched = []   // [{el, attr, prev, kind, target}]，上限 200；让位时还原

function restoreDomPatched(kind) {
  for (let i = domPatched.length - 1; i >= 0; i--) {
    const entry = domPatched[i]
    if (entry.kind !== kind) continue
    try {
      if (entry.prev === null) entry.el.removeAttribute(entry.attr)
      else entry.el.setAttribute(entry.attr, entry.prev)
    } catch { /* 元素可能已销毁 */ }
    domPatched.splice(i, 1)
  }
}

function recordDomPatch(el, attr, prev, kind, target) {
  const existing = domPatched.findIndex((e) => e.el === el && e.attr === attr)
  if (existing >= 0) domPatched[existing] = { el, attr, prev: domPatched[existing].prev, kind, target }
  else {
    domPatched.push({ el, attr, prev, kind, target })
    if (domPatched.length > 200) domPatched.shift()
  }
}

function yieldPreview(kind, reason) {
  const enabled = kind === 'icon' ? CONFIG.icon.enabled : CONFIG.banner.enabled
  if (!enabled) return
  if (kind === 'icon') CONFIG.icon.enabled = false
  else CONFIG.banner.enabled = false
  writePersisted(currentSelection())
  restoreDomPatched(kind)
  log(`检测到客户端原生切换（${reason}），${kind === 'icon' ? '头像' : '旗帜'}预览已自动让位，`,
      '客户端功能恢复。要重新预览请再调用 set' + (kind === 'icon' ? 'Icon' : 'Banner') + '。')
}

function noteTruth(key, value) {
  if (value === undefined || value === null) return
  // 键必须是「源:字段」：不同端点对同一种类本来就会报不同值
  // （账号头像 vs 聊天 presence 头像 vs hovercard 头像），按种类存会在启动时误让位。
  const kind = key.indexOf(':') >= 0 ? key.slice(key.lastIndexOf(':') + 1) : key
  const prev = state.lastTruth[key]
  state.lastTruth[key] = value
  if (prev === undefined || String(value) === String(prev)) return
  const enabled = kind === 'icon' ? CONFIG.icon.enabled : CONFIG.banner.enabled
  if (!enabled) return
  const target = String(kind === 'icon' ? CONFIG.icon.id : CONFIG.banner.id)
  if (String(value) === target) return   // 真值切到我们的目标，不算冲突
  yieldPreview(kind, `${key} 真值 ${prev} -> ${value}`)
}

/** /lol-summoner/v1/current-summoner —— 生涯头像的权威来源，顺便记下自己的身份。 */
function applyCurrentSummoner(json) {
  if (!json || typeof json !== 'object') return false
  if (typeof json.puuid === 'string' && json.puuid) state.ownPuuid = json.puuid
  if (json.summonerId) state.ownSummonerId = json.summonerId
  noteTruth('current-summoner:icon', safeOwn(json, 'profileIconId') ? json.profileIconId : undefined)
  if (!CONFIG.icon.enabled || !CONFIG.icon.id) return false
  if (!safeOwn(json, 'profileIconId')) return false
  if (json.profileIconId === CONFIG.icon.id) return false
  diag('current-summoner', 'profileIconId', json.profileIconId, '->', CONFIG.icon.id)
  json.profileIconId = CONFIG.icon.id
  return true
}

/**
 * /lol-chat/v1/me —— 好友栏/聊天身份卡。
 * 只有响应里确实带 lol 对象时才动 lol.*，绝不凭空造结构。
 */
function applyChatMe(json) {
  if (!json || typeof json !== 'object') return false
  if (typeof json.puuid === 'string' && json.puuid) state.ownPuuid = json.puuid
  if (json.summonerId) state.ownSummonerId = json.summonerId
  noteTruth('chat-me:icon', safeOwn(json, 'icon') ? json.icon : undefined)
  let changed = false
  if (CONFIG.icon.enabled && CONFIG.icon.id && safeOwn(json, 'icon') && json.icon !== CONFIG.icon.id) {
    diag('chat-me', 'icon', json.icon, '->', CONFIG.icon.id)
    json.icon = CONFIG.icon.id
    changed = true
  }
  if (CONFIG.banner.enabled && safeToken(String(CONFIG.banner.id))
      && json.lol && typeof json.lol === 'object') {
    const target = String(CONFIG.banner.id)
    if (safeOwn(json.lol, 'bannerIdSelected') && json.lol.bannerIdSelected !== target) {
      diag('chat-me', 'lol.bannerIdSelected', json.lol.bannerIdSelected, '->', target)
      json.lol.bannerIdSelected = target
      changed = true
    }
    // lol.regalia / lol.iconOverride 的格式尚未实证（都是 string），只观测不写，绝不猜格式
    if (safeOwn(json.lol, 'regalia')) diag('chat-me', 'lol.regalia 原值 =', shortValue(json.lol.regalia))
    if (safeOwn(json.lol, 'iconOverride')) diag('chat-me', 'lol.iconOverride 原值 =', shortValue(json.lol.iconOverride))
  }
  return changed
}

const ICON_ID_KEYS = ['itemId', 'itemID', 'id', 'iconId', 'profileIconId', 'summonerIconId']

/**
 * /lol-inventory/v2/inventory/SUMMONER_ICON —— 头像拥有态。
 * 不改这里的话，客户端选择器会把未拥有项置灰。
 */
function applyIconInventory(json) {
  if (!Array.isArray(json)) return false
  const target = Number(CONFIG.icon.id)
  if (!CONFIG.icon.enabled || !target) return false

  let hit = null
  for (const item of json) {
    if (!item || typeof item !== 'object') continue
    for (const k of ICON_ID_KEYS) {
      if (safeOwn(item, k) && Number(item[k]) === target) { hit = item; break }
    }
    if (hit) break
  }

  if (hit) {
    let changed = false
    // 三个键口径统一：只在键**已存在**且值不是「已拥有」时才改，不凭空加键
    if (safeOwn(hit, 'owned') && hit.owned !== true) { hit.owned = true; changed = true }
    if (safeOwn(hit, 'isOwned') && hit.isOwned !== true) { hit.isOwned = true; changed = true }
    if (safeOwn(hit, 'ownershipType') && hit.ownershipType !== 'OWNED') { hit.ownershipType = 'OWNED'; changed = true }
    if (changed) diag('icon-inventory', `itemId=${target} 标记为已拥有`)
    return changed
  }

  // 目录里没有这一项：克隆一条真实条目的形状，只覆写它本来就有的 id 键，
  // 并把 uuid 换成唯一值——RCP databinding 常以 uuid 为 key，重复会导致藏品页丢项。
  const template = json.find((x) => x && typeof x === 'object')
  if (!template) {
    diag('icon-inventory', '库存为空，无法构造合成条目')
    return false
  }
  const synthetic = { ...template }
  const presentIdKeys = ICON_ID_KEYS.filter((k) => safeOwn(synthetic, k))
  if (presentIdKeys.length) presentIdKeys.forEach((k) => { synthetic[k] = target })
  else synthetic.itemId = target
  if (safeOwn(synthetic, 'uuid')) synthetic.uuid = `facade-local-${target}`
  if (safeOwn(synthetic, 'owned')) synthetic.owned = true
  if (safeOwn(synthetic, 'isOwned')) synthetic.isOwned = true
  if (safeOwn(synthetic, 'ownershipType')) synthetic.ownershipType = 'OWNED'
  if (safeOwn(synthetic, 'quantity')) synthetic.quantity = 1
  json.push(synthetic)
  diag('icon-inventory', `itemId=${target} 不在库存里，已追加合成条目（覆写键：${presentIdKeys.join(',') || 'itemId'}）`)
  return true
}

/** /lol-regalia/v3/inventory/REGALIA_BANNER —— 旗帜拥有态的唯一权威来源（v2 inventory 那条是错的）。 */
function applyRegaliaBannerInventory(json) {
  if (!json || typeof json !== 'object') return false
  if (!CONFIG.banner.enabled) return false
  const target = String(CONFIG.banner.id || '')
  if (!safeToken(target)) {
    if (target) warn('regalia-banner-inventory', '旗帜 id 非法，已忽略：', target.slice(0, 32))
    return false
  }

  if (Array.isArray(json)) {
    let changed = false
    for (const item of json) {
      if (!item || typeof item !== 'object') continue
      const id = safeOwn(item, 'id') ? item.id : (safeOwn(item, 'itemId') ? item.itemId : undefined)
      if (String(id ?? '') !== target) continue
      if (safeOwn(item, 'isOwned') && item.isOwned !== true) { item.isOwned = true; changed = true }
      if (safeOwn(item, 'owned') && item.owned !== true) { item.owned = true; changed = true }
    }
    if (changed) diag('regalia-banner-inventory', `banner ${target} (数组形态) isOwned -> true`)
    return changed
  }

  // 实测形状：以数字 id 字符串为键的对象，值里带 isOwned
  if (!safeOwn(json, target)) {
    warn('regalia-banner-inventory', `没找到 banner ${target}，实际键：`, Object.keys(json).slice(0, 20).join(','))
    diag('regalia-banner-inventory', '首个条目样本 =', shortValue(json[Object.keys(json)[0]]))
    return false
  }
  const entry = json[target]
  if (!entry || typeof entry !== 'object') return false
  diag('regalia-banner-inventory', `banner ${target} 条目 =`, shortValue(entry))
  let changed = false
  if (safeOwn(entry, 'isOwned') && entry.isOwned !== true) { entry.isOwned = true; changed = true }
  if (safeOwn(entry, 'owned') && entry.owned !== true) { entry.owned = true; changed = true }
  if (changed) diag('regalia-banner-inventory', `banner ${target} isOwned -> true`)
  return changed
}

/** /lol-challenges/v1/summary-player-data/local-player —— 生涯页旗帜的读取来源。 */
function applyChallengeSummary(json) {
  if (!json || typeof json !== 'object') return false
  noteTruth('challenge-summary:banner', safeOwn(json, 'bannerId') ? json.bannerId : undefined)
  if (!CONFIG.banner.enabled || !safeToken(String(CONFIG.banner.id || ''))) return false
  const target = String(CONFIG.banner.id)
  if (!safeOwn(json, 'bannerId') || json.bannerId === target) return false
  diag('challenge-summary', 'bannerId', json.bannerId, '->', target)
  json.bannerId = target
  return true
}

/**
 * 生涯外观聚合端点。字段名尚未全部实证，所以只改**确实存在**的键，
 * 其余交给 diagnostic 打出来，绝不凭空造字段。
 */
function applySummonerProfile(json) {
  if (!json || typeof json !== 'object') return false
  let changed = false
  if (CONFIG.icon.enabled && CONFIG.icon.id) {
    for (const k of ['profileIconId', 'iconId', 'summonerIconId']) {
      if (safeOwn(json, k) && json[k] !== CONFIG.icon.id) {
        diag('summoner-profile', k, json[k], '->', CONFIG.icon.id)
        json[k] = CONFIG.icon.id
        changed = true
      }
    }
  }
  if (CONFIG.banner.enabled && safeToken(String(CONFIG.banner.id || ''))) {
    const target = String(CONFIG.banner.id)
    for (const k of ['bannerId', 'selectedBannerId', 'banner', 'regaliaBannerId']) {
      if (safeOwn(json, k) && json[k] !== target) {
        diag('summoner-profile', k, json[k], '->', target)
        json[k] = target
        changed = true
      }
    }
    // 假设 #2（第六轮：regalia v2 bannerType 与 challenge-summary bannerId 均被证伪后剩下唯一载体）：
    // 生涯页旗帜读 summoner-profile.regalia 这个 JSON 字符串里的 bannerType（数值口径）。
    // 单变量：只改 bannerType，crestType / selectedPrestigeCrest 不动；解析失败就放弃不改。
    if (safeOwn(json, 'regalia') && typeof json.regalia === 'string') {
      try {
        const reg = JSON.parse(json.regalia)
        if (reg && typeof reg === 'object' && safeOwn(reg, 'bannerType')
            && String(reg.bannerType) !== target) {
          diag('summoner-profile', 'regalia.bannerType', reg.bannerType, '->', target, '（假设#2）')
          reg.bannerType = Number.isFinite(Number(target)) ? Number(target) : target
          json.regalia = JSON.stringify(reg)
          changed = true
        }
      } catch { /* regalia 不是合法 JSON，放弃 */ }
    }
  }
  return changed
}

/**
 * /lol-regalia/v2|v3/.../regalia。
 * 真机实测（2026-09-18 国服）：生涯页头像读的是**这里的 profileIconId**，
 * 不是 current-summoner——只改 current-summoner 时生涯页纹丝不动、好友栏却生效。
 * 所以这里必须改写 profileIconId。旗帜字段（bannerType/crestType/selectedPrestigeCrest）
 * 本轮仍只观测不写，等下一轮实证。
 * 归属门禁：/summoners/{id}/regalia 可能是别人，id 对不上就一律不动。
 */
function applyRegalia(json, path) {
  if (!json || typeof json !== 'object') return false
  noteTruth('regalia:icon', safeOwn(json, 'profileIconId') ? json.profileIconId : undefined)
  noteTruth('regalia:banner', safeOwn(json, 'bannerType') ? json.bannerType : undefined)
  diag('regalia', 'bannerType=', shortValue(json.bannerType), 'crestType=', shortValue(json.crestType),
       'selectedPrestigeCrest=', shortValue(json.selectedPrestigeCrest),
       'profileIconId=', shortValue(json.profileIconId))
  if (!regaliaIsSelf(path)) return false
  let changed = false
  if (CONFIG.icon.enabled && CONFIG.icon.id
      && safeOwn(json, 'profileIconId') && json.profileIconId !== CONFIG.icon.id) {
    diag('regalia', 'profileIconId', json.profileIconId, '->', CONFIG.icon.id)
    json.profileIconId = CONFIG.icon.id
    changed = true
  }
  // 生涯页旗帜的读取源（第五轮实测：生涯页读 regalia v2 共 12 次，challenge-summary / summoner-profile 0 次）。
  // **待实证假设**：装备某面生涯旗帜时 bannerType = 该旗帜的目录 id（字符串）；未装备时为 'blank'。
  // 单变量假设：本轮只改 bannerType，crestType / selectedPrestigeCrest 一律不动，
  // 下一轮真机看生涯页旗帜是否变化来证实或证伪。
  if (CONFIG.banner.enabled && safeToken(String(CONFIG.banner.id || ''))
      && safeOwn(json, 'bannerType') && json.bannerType !== CONFIG.banner.id) {
    diag('regalia', 'bannerType', json.bannerType, '->', CONFIG.banner.id, '（待实证假设）')
    json.bannerType = CONFIG.banner.id
    changed = true
  }
  return changed
}
/** regalia 路径是不是自己：current-summoner 必然是；summoners/{id}/ 要和自己的 summonerId 对上。 */
function regaliaIsSelf(path) {
  if (path.indexOf('/current-summoner/') >= 0) return true
  const m = /\/summoners\/(\d+)\//.exec(path)
  if (!m) return false
  const own = Number(state.ownSummonerId || 0)
  return !!own && Number(m[1]) === own
}

/**
 * /lol-champ-select/v1/session —— 选人页。
 * 注意：mySelection 里**没有** profileIconId/bannerId（akari 的类型表已确认），
 * 头像来自 myTeam[]/theirTeam[] 的成员对象，所以只改「自己那一条」。
 */
function applyChampSelectSession(json) {
  if (!json || typeof json !== 'object') return false
  let changed = false
  for (const key of ['myTeam', 'theirTeam']) {
    const list = json[key]
    if (!Array.isArray(list)) continue
    for (const member of list) {
      // 认不准归属就不动，宁可不生效也不能改到别人头上
      if (!memberIsSelf(member)) continue
      if (CONFIG.icon.enabled && CONFIG.icon.id && safeOwn(member, 'profileIconId')
          && member.profileIconId !== CONFIG.icon.id) {
        diag('champ-select', `${key}.profileIconId`, member.profileIconId, '->', CONFIG.icon.id)
        member.profileIconId = CONFIG.icon.id
        changed = true
      }
      if (CONFIG.banner.enabled && safeToken(String(CONFIG.banner.id || ''))) {
        const target = String(CONFIG.banner.id)
        for (const k of ['bannerId', 'regaliaBannerId', 'banner']) {
          if (safeOwn(member, k) && member[k] !== target) {
            diag('champ-select', `${key}.${k}`, member[k], '->', target)
            member[k] = target
            changed = true
          }
        }
      }
    }
  }
  if (CONFIG.diagnostic && Array.isArray(json.myTeam) && json.myTeam[0]) {
    diag('champ-select', 'myTeam[0] keys =', Object.keys(json.myTeam[0]).join(','))
  }
  return changed
}

/**
 * /lol-hovercard/v1/friend-info/{puuid} —— 只改自己的那张卡。
 * 真机实测：头像在这份响应里有**三个字段**（icon / summonerIcon / lol.profileIcon），
 * 不同界面读的不一样，三个都改才稳。
 */
function applyHovercard(json, path) {
  if (!json || typeof json !== 'object') return false
  noteTruth('hovercard:icon', safeOwn(json, 'icon') ? json.icon : undefined)
  if (!hovercardIsSelf(path)) return false
  let changed = false
  if (CONFIG.icon.enabled && CONFIG.icon.id) {
    if (safeOwn(json, 'icon') && json.icon !== CONFIG.icon.id) { json.icon = CONFIG.icon.id; changed = true }
    if (safeOwn(json, 'summonerIcon') && json.summonerIcon !== CONFIG.icon.id) { json.summonerIcon = CONFIG.icon.id; changed = true }
    if (json.lol && typeof json.lol === 'object'
        && safeOwn(json.lol, 'profileIcon') && json.lol.profileIcon !== CONFIG.icon.id) {
      json.lol.profileIcon = CONFIG.icon.id
      changed = true
    }
  }
  // 旗帜：自己的卡在好友栏/组队里展示的旗帜 id（挑战旗帜口径，与 challenge-summary 一致）
  if (CONFIG.banner.enabled && safeToken(String(CONFIG.banner.id || ''))
      && json.lol && typeof json.lol === 'object'
      && safeOwn(json.lol, 'bannerIdSelected') && json.lol.bannerIdSelected !== CONFIG.banner.id) {
    json.lol.bannerIdSelected = CONFIG.banner.id
    changed = true
  }
  return changed
}

// ---- 规则表 ----
const RULES = [
  { id: 'current-summoner',         match: (p) => p === '/lol-summoner/v1/current-summoner',                   apply: applyCurrentSummoner },
  { id: 'chat-me',                  match: (p) => p === '/lol-chat/v1/me',                                     apply: applyChatMe },
  { id: 'icon-inventory',           match: (p) => p === '/lol-inventory/v2/inventory/SUMMONER_ICON',           apply: applyIconInventory },
  { id: 'regalia-banner-inventory', match: (p) => p === '/lol-regalia/v3/inventory/REGALIA_BANNER',            apply: applyRegaliaBannerInventory },
  { id: 'challenge-summary',        match: (p) => p === '/lol-challenges/v1/summary-player-data/local-player', apply: applyChallengeSummary },
  { id: 'summoner-profile',         match: (p) => /^\/lol-summoner\/v1\/(current-summoner\/)?summoner-profile$/.test(p), apply: applySummonerProfile },
  { id: 'regalia',                  match: (p) => /^\/lol-regalia\/v[23]\//.test(p) && p.endsWith('/regalia'), apply: applyRegalia },
  { id: 'champ-select',             match: (p) => p === '/lol-champ-select/v1/session',                        apply: applyChampSelectSession },
  { id: 'hovercard',                match: (p) => p.startsWith(HOVERCARD_PREFIX),                              apply: applyHovercard },
]

function findRule(rawUrl) {
  const path = normalizePath(rawUrl)
  if (!path) return null
  for (const rule of RULES) {
    try {
      if (rule.match(path)) return { rule, path }
    } catch {
      // matcher 抛错不影响请求
    }
  }
  return null
}

/**
 * 对一段 JSON 文本应用规则。**不做计数**——计数由各通道负责，避免重复 +1。
 * @returns {string|null} 改写后的文本；null 表示无需/无法改写（调用方应原样放行）
 */
function rewriteText(rule, path, text) {
  if (typeof text !== 'string' || !text) return null
  let json
  try { json = JSON.parse(text) } catch { return null }
  try {
    summarize(rule.id, path, json)            // 必须在 apply 之前，保证记录的是真值
    if (rule.apply(json, path) !== true) return null
    return JSON.stringify(json)
  } catch (err) {
    warnOnce('rewrite:' + rule.id, err)
    return null
  }
}

// ============================================================
// 通道 1：XMLHttpRequest（惰性 getter，与监听器注册顺序无关）
//
// 为什么不用 addEventListener：事件派发到 xhr 自身时，监听器按**注册顺序**执行，
// capture 标志在 target 阶段不起作用。客户端常见写法是 `xhr.onload = fn` 然后才
// send()，那样它的 handler 会比我们在 send() 里挂的监听器先跑，读到的仍是原始响应
// （sona 的 xhr/core.ts 就是这个路线，存在同样的时序依赖）。
// 这里改成在实例上定义 own getter 遮蔽原型的 responseText/response：无论谁、在什么
// 时机读，拿到的都是改写后的值。挂载点是 open()，比 send() 更早。
// ============================================================
function installXhrHook() {
  if (state.installed.xhr || !CONFIG.rewriteXhr) return
  const ctor = window.XMLHttpRequest
  const proto = ctor && ctor.prototype
  if (!proto) return
  const DONE = ctor.DONE || 4
  const originalOpen = proto.open
  const textDesc = Object.getOwnPropertyDescriptor(proto, 'responseText')
  const responseDesc = Object.getOwnPropertyDescriptor(proto, 'response')
  if (!textDesc && !responseDesc) {
    warn('XHR hook 跳过：找不到 responseText/response 原型访问器')
    return
  }

  proto.open = function (method, url) {
    try {
      const found = findRule(url)
      if (found) installLazyRewrite(this, found, DONE, textDesc, responseDesc)
      // 同一个实例可能被复用于别的请求，必须清掉旧规则，否则会拿旧规则去改新响应
      else if (this.__facadeLazy) this.__facadeRule = null
    } catch (err) {
      warnOnce('xhr-open', err)
    }
    // 原生 open 的异常语义（SyntaxError 等）必须原样抛出，所以放在 try 之外
    return originalOpen.apply(this, arguments)
  }

  state.installed.xhr = true
  log('XHR hook 已安装（惰性 getter 模式）')
}

/** 在实例上用 own getter 遮蔽 responseText / response，读取时惰性改写。 */
function installLazyRewrite(xhr, found, DONE, textDesc, responseDesc) {
  // 每次 open 都刷新规则与缓存；getter 只定义一次
  xhr.__facadeRule = found
  xhr.__facadeCache = { rawText: undefined, outText: null, rawObj: undefined, outObj: null, objDone: false, logged: false }
  if (xhr.__facadeLazy) return
  xhr.__facadeLazy = true

  // 计数与日志都从**当前**规则取，不能用闭包里首次安装时的规则（实例会复用）
  const noteRewrite = (self) => {
    const cache = self.__facadeCache
    if (!cache || cache.logged) return
    cache.logged = true
    const active = self.__facadeRule
    if (!active) return
    bump(active.rule.id)
    log('XHR 改写', active.rule.id, active.path)
  }

  const activeRule = (self) => (self.__facadeLazy ? self.__facadeRule : null)

  if (textDesc && textDesc.get) {
    try {
      Object.defineProperty(xhr, 'responseText', {
        configurable: true,
        get() {
          // 先走原生 getter，保持它原本的异常语义（responseType 不兼容时会抛 InvalidStateError）
          const raw = textDesc.get.call(this)
          try {
            const active = activeRule(this)
            if (!active || this.readyState !== DONE) return raw
            if (typeof raw !== 'string' || !raw) return raw
            const cache = this.__facadeCache
            if (raw === cache.rawText) return cache.outText === null ? raw : cache.outText
            const next = rewriteText(active.rule, active.path, raw)
            cache.rawText = raw
            cache.outText = next
            if (next !== null) noteRewrite(this)
            return next === null ? raw : next
          } catch (err) {
            // 任何意外都退化成原值：绝不能把异常抛进客户端代码
            warnOnce('xhr-responseText', err)
            return raw
          }
        },
      })
    } catch (err) {
      warn('responseText 遮蔽失败', err)
    }
  }

  if (responseDesc && responseDesc.get) {
    try {
      Object.defineProperty(xhr, 'response', {
        configurable: true,
        get() {
          const raw = responseDesc.get.call(this)
          try {
            const active = activeRule(this)
            if (!active || this.readyState !== DONE) return raw
            const cache = this.__facadeCache

            // responseType==='json'：原生返回对象，改写后要还原成对象
            if (this.responseType === 'json') {
              if (!raw || typeof raw !== 'object') return raw
              // 缓存必须存**对象**：客户端可能多次读 response，第二次不能退化成字符串
              if (cache.objDone && raw === cache.rawObj) return cache.outObj
              let parsed = raw
              const next = rewriteText(active.rule, active.path, JSON.stringify(raw))
              if (next !== null) {
                parsed = JSON.parse(next)
                noteRewrite(this)
              }
              cache.rawObj = raw
              cache.outObj = parsed
              cache.objDone = true
              return parsed
            }

            // arraybuffer / blob 等非字符串响应原样放行
            if (typeof raw !== 'string' || !raw) return raw
            if (raw === cache.rawText) return cache.outText === null ? raw : cache.outText
            const next = rewriteText(active.rule, active.path, raw)
            cache.rawText = raw
            cache.outText = next
            if (next !== null) noteRewrite(this)
            return next === null ? raw : next
          } catch (err) {
            warnOnce('xhr-response', err)
            return raw
          }
        },
      })
    } catch (err) {
      warn('response 遮蔽失败', err)
    }
  }
}

/** 未包装的 fetch，供 suggestIcons() 读真值库存用（installFetchHook 里赋值）。 */
let originalFetchRef = null

/** 用未包装的 fetch 读 LCU，绕开我们自己的改写规则。 */
async function rawFetch(url) {
  const fn = typeof originalFetchRef === 'function' ? originalFetchRef : window.fetch
  // 必须带 window 作为 this，否则原生 fetch 抛 Illegal invocation
  const res = await fn.call(window, url)
  return res.ok ? res.json() : null
}

/**
 * 从客户端目录里挑出几个「你没有拥有、且当前区域可用」的头像 ID。
 * 用途：省掉手工找未拥有 ID 这一步——验证脚本必须拿未拥有的头像来测才有意义。
 * 排除项依据项目已有实测：imagePath 为空的 86 条、disabledRegions 非空的 3 条。
 */
async function suggestIcons(limit = 8) {
  const [catalog, inventory] = await Promise.all([
    rawFetch('/lol-game-data/assets/v1/summoner-icons.json'),
    rawFetch('/lol-inventory/v2/inventory/SUMMONER_ICON'),
  ])
  if (!Array.isArray(catalog) || !catalog.length) {
    warn('suggestIcons', '读不到头像目录（客户端未登录？），返回空')
    return []
  }
  const stats = collectInventoryStats(inventory)
  state.inventoryStats = stats
  // 真机第二轮实测（2026-09-18）：候选曾恰好等于目录开头连续若干条，
  // 说明拥有集合为空——库存要么没读到、要么字段形状与假设不符。
  // 这种情况下「未拥有候选」是不可信结论，按红线不得输出。
  if (!stats.available) {
    warn('suggestIcons', '库存拥有态读取失败（端点非 200 或非数组），候选不可信，返回空。',
         '请改用 searchIcons(关键字) 并结合你自己是否拥有来判断。')
    return []
  }
  if (stats.ownedCount === 0) {
    warn('suggestIcons', `库存读到 ${stats.itemCount} 条但 owned 标记全为空——`,
         '国服字段形状与假设不符，候选不可信，返回空。首条样本见 [diag]。')
    diag('suggestIcons', '库存首条样本 =', shortValue(inventory[0]))
    return []
  }
  const picks = []
  for (const icon of catalog) {
    if (!icon || typeof icon !== 'object') continue
    const id = Number(icon.id)
    if (!Number.isFinite(id) || id <= 0) continue
    if (typeof icon.imagePath !== 'string' || !icon.imagePath) continue
    if (Array.isArray(icon.disabledRegions) && icon.disabledRegions.length) continue
    if (stats.owned.has(id)) continue
    picks.push({ id, name: String(icon.title || icon.name || ''), setId: icon.setId ?? null })
    if (picks.length >= limit) break
  }
  log(`未拥有头像候选（库存 ${stats.itemCount} 条 / 判定已拥有 ${stats.ownedCount} 个）：`,
    picks.map((p) => `${p.id}${p.name ? '(' + p.name + ')' : ''}`).join('  ') || '(无)')
  return picks
}

/** 库存拥有态统计。available=false 表示端点不可用，此时任何「未拥有」结论都不成立。 */
function collectInventoryStats(inventory) {
  if (!Array.isArray(inventory)) return { available: false, itemCount: 0, ownedCount: 0, owned: new Set() }
  const owned = new Set()
  for (const item of inventory) {
    if (!item || typeof item !== 'object') continue
    const isOwned = item.owned === true || item.isOwned === true || item.ownershipType === 'OWNED'
      || Number(item.ownedQuantity) > 0 || Number(item.quantity) > 0
    if (!isOwned) continue
    for (const k of ICON_ID_KEYS) {
      if (safeOwn(item, k)) {
        const n = Number(item[k])
        if (Number.isFinite(n)) owned.add(n)
      }
    }
  }
  return { available: true, itemCount: inventory.length, ownedCount: owned.size, owned }
}

/**
 * 按名称搜头像目录，返回 id + 名称 + 库存判定。
 * 用途：库存判定在国服尚未证实可靠，所以把「库存说已拥有」只作为参考列给出，
 * 最终是否未拥有由使用者自己的记忆确认（例如你确定没买过的至臻/限定）。
 */
async function searchIcons(keyword, limit = 20) {
  const kw = String(keyword || '').toLowerCase()
  const [catalog, inventory] = await Promise.all([
    rawFetch('/lol-game-data/assets/v1/summoner-icons.json'),
    rawFetch('/lol-inventory/v2/inventory/SUMMONER_ICON'),
  ])
  if (!Array.isArray(catalog)) { warn('searchIcons', '读不到头像目录，返回空'); return [] }
  const stats = collectInventoryStats(inventory)
  state.inventoryStats = stats
  const out = []
  for (const icon of catalog) {
    if (!icon || typeof icon !== 'object') continue
    const name = String(icon.title || icon.name || '')
    if (kw && name.toLowerCase().indexOf(kw) < 0) continue
    const id = Number(icon.id)
    if (!Number.isFinite(id) || id <= 0) continue
    out.push({ id, name, ownedPerInventory: stats.available ? stats.owned.has(id) : null })
    if (out.length >= limit) break
  }
  log(`searchIcons("${keyword}") 库存状态：${stats.available ? stats.itemCount + ' 条 / owned ' + stats.ownedCount + '（ownedPerInventory 可信）' : '不可用（ownedPerInventory=null，需自行确认）'}`)
  return out
}

/**
 * 按 id 倒序列出头像（id 越大越新）。用途：最新头像最可能未拥有，
 * 使用者建议（2026-09-18 第三轮）：验证未拥有 case 时优先挑最新的。
 * 仍带 ownedPerInventory 参考列（国服库存=已拥有子集，可信）。
 */
async function newestIcons(limit = 10) {
  const [catalog, inventory] = await Promise.all([
    rawFetch('/lol-game-data/assets/v1/summoner-icons.json'),
    rawFetch('/lol-inventory/v2/inventory/SUMMONER_ICON'),
  ])
  if (!Array.isArray(catalog)) { warn('newestIcons', '读不到头像目录，返回空'); return [] }
  const stats = collectInventoryStats(inventory)
  state.inventoryStats = stats
  const rows = []
  for (const icon of catalog) {
    if (!icon || typeof icon !== 'object') continue
    const id = Number(icon.id)
    if (!Number.isFinite(id) || id <= 0) continue
    if (typeof icon.imagePath !== 'string' || !icon.imagePath) continue
    if (Array.isArray(icon.disabledRegions) && icon.disabledRegions.length) continue
    rows.push({ id, name: String(icon.title || icon.name || ''), ownedPerInventory: stats.available ? stats.owned.has(id) : null })
  }
  rows.sort((a, b) => b.id - a.id)
  const out = rows.slice(0, limit)
  log(`newestIcons(${limit}) 库存状态：${stats.available ? stats.itemCount + ' 条 / owned ' + stats.ownedCount : '不可用'}；`,
    '挑 ownedPerInventory=false 的就是未拥有')
  return out
}

/**
 * 一次性导出排障所需的全部信息，返回 JSON 字符串。
 * 在 DevTools Console 里执行：copy(__facadePreview.dump())  → 直接粘给开发者。
 *
 * 隐私：puuid / summonerId **不落值**，只报「是否已捕获」和长度——这份 dump 会被
 * 贴到对话或文档里，遵循项目一贯的诊断脱敏口径。
 */
function dump() {
  const pengu = window.Pengu || {}
  const own = state.ownPuuid || ''
  return JSON.stringify({
    script: SCRIPT_VERSION,
    // __llver 是 Pengu 暴露的客户端版本；契约漂移时这是第一判据
    clientVersion: window.__llver || null,
    penguVersion: pengu.version || null,
    userAgent: (navigator && navigator.userAgent) || null,
    config: {
      icon: { ...CONFIG.icon },
      banner: { ...CONFIG.banner },
      rewriteXhr: CONFIG.rewriteXhr,
      rewriteFetch: CONFIG.rewriteFetch,
      rewriteWebSocket: CONFIG.rewriteWebSocket,
      diagnostic: CONFIG.diagnostic,
    },
    hooksInstalled: { ...state.installed },
    identity: {
      puuidCaptured: !!own,
      puuidLength: own.length,
      summonerIdCaptured: !!state.ownSummonerId,
    },
    counts: { ...state.counts },
    // seen 里是改写**前**采集的服务端真值结构，这是定契约的关键载荷
    seen: { ...state.seen },
    wsPathFragments: [...state.wsPathFragments],
    // 库存拥有态统计：available=false 或 ownedCount=0 时，任何「未拥有」结论都不可信
    inventoryStats: state.inventoryStats
      ? { available: state.inventoryStats.available, itemCount: state.inventoryStats.itemCount,
          ownedCount: state.inventoryStats.ownedCount }
      : null,
    warnings: [...state.warnings],
  }, null, 2)
}
// ============================================================
// 通道 2：fetch
// ============================================================
function installFetchHook() {
  if (state.installed.fetch || !CONFIG.rewriteFetch) return
  if (typeof window.fetch !== 'function') return
  const originalFetch = window.fetch
  // 存一份未包装的 fetch：suggestIcons() 要用它读**真值**库存，
  // 否则会被我们自己的改写规则污染（把目标头像算成已拥有）
  originalFetchRef = originalFetch

  window.fetch = async function (input) {
    const response = await originalFetch.apply(this, arguments)
    try {
      // input 可能是 string / URL / Request，三种都要能取出地址
      let url = ''
      if (typeof input === 'string') url = input
      else if (input instanceof URL) url = input.href
      else if (input && typeof input.url === 'string') url = input.url
      else url = String(input || '')

      const found = findRule(url)
      if (!found) return response
      // 204/205/304 是无 body 状态码，构造带 body 的 Response 会抛，必须直接放行
      if (response.status === 204 || response.status === 205 || response.status === 304) return response
      const text = await response.clone().text()
      const next = rewriteText(found.rule, found.path, text)
      if (next === null) return response
      bump(found.rule.id)
      log('fetch 改写', found.rule.id, found.path)
      return new Response(next, {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
      })
    } catch (err) {
      warnOnce('fetch', err)
      return response
    }
  }

  state.installed.fetch = true
  log('fetch hook 已安装')
}

// ============================================================
// 通道 3：WebSocket 事件帧
// ============================================================
function installWebSocketHook() {
  if (state.installed.ws || !CONFIG.rewriteWebSocket) return
  const proto = window.WebSocket && window.WebSocket.prototype
  if (!proto) return

  // WS 帧在 JSON.parse 之前先做子串预筛：客户端推送量很大，每帧都 parse 纯属浪费。
  // **新增或修改规则时必须同步维护这份片段表**，否则该端点的 WS 推送不会被改写。
  const pathFragments = [
    '/lol-summoner/v1/current-summoner',
    '/lol-chat/v1/me',
    '/lol-inventory/v2/inventory/SUMMONER_ICON',
    '/lol-regalia/v3/inventory/REGALIA_BANNER',
    '/lol-challenges/v1/summary-player-data/local-player',
    'summoner-profile',
    '/lol-regalia/v2/',
    '/lol-regalia/v3/summoners/',
    '/lol-champ-select/v1/session',
    HOVERCARD_PREFIX,
  ]
  state.wsPathFragments = pathFragments

  /** 改写一帧 LCU 事件。frame 形如 [8, endpoint, {uri, eventType, data}] */
  function rewriteFrame(raw) {
    // 二进制帧（Blob/ArrayBuffer）不是字符串，直接放行
    if (typeof raw !== 'string' || raw.charCodeAt(0) !== 0x5b /* '[' */) return null
    if (!pathFragments.some((fragment) => raw.indexOf(fragment) >= 0)) return null
    let frame
    try { frame = JSON.parse(raw) } catch { return null }
    if (!Array.isArray(frame) || frame[0] !== 8) return null
    const payload = frame[2]
    if (!payload || typeof payload !== 'object' || typeof payload.uri !== 'string') return null
    const found = findRule(payload.uri)
    if (!found) return null
    if (!payload.data || typeof payload.data !== 'object') return null
    try {
      summarize(found.rule.id, found.path, payload.data)   // 改写前采集真值
      if (found.rule.apply(payload.data, found.path) !== true) return null
      bump(found.rule.id)
      log('WS 改写', found.rule.id, payload.uri, payload.eventType || '')
      return JSON.stringify(frame)
    } catch (err) {
      warnOnce('ws-rewrite:' + found.rule.id, err)
      return null
    }
  }

  // original -> wrapper 双向映射：保证幂等（重复 addEventListener 不会注册两次），
  // 并让 removeEventListener 传原始函数也能真正摘掉
  const wrapperByListener = new WeakMap()
  const listenerByWrapper = new WeakMap()

  function wrapListener(listener, ws) {
    if (typeof listener !== 'function') return listener
    const cached = wrapperByListener.get(listener)
    if (cached) return cached
    // 记下 (ws, 原始监听器)：applyNow() 要靠这些引用**伪造推送**，让已渲染的界面立刻重渲染
    if (ws && state.pushTargets.length < 64
        && !state.pushTargets.some((t) => t.listener === listener)) {
      state.pushTargets.push({ ws, listener })
    }
    const wrapped = function (event) {
      let ev = event
      try {
        const next = rewriteFrame(event && event.data)
        if (next !== null) {
          try {
            // 优先在事件对象上定义 own property 遮蔽只读的 data，
            // 这样同一帧派发给的所有客户端监听器都能看到改写值
            Object.defineProperty(event, 'data', { configurable: true, value: next })
          } catch {
            ev = new MessageEvent('message', { data: next })
          }
        }
      } catch (err) {
        warnOnce('ws-frame', err)
        ev = event
      }
      // 客户端监听器**恰好调用一次**，且不在 try 内：它自己的异常照常向上抛，
      // 我们既不吞掉也不重试，避免一帧被处理多遍
      return listener.call(this, ev)
    }
    wrapperByListener.set(listener, wrapped)
    listenerByWrapper.set(wrapped, listener)
    return wrapped
  }

  const originalAdd = proto.addEventListener
  proto.addEventListener = function (type, listener, options) {
    if (type === 'message') {
      try { listener = wrapListener(listener, this) } catch (err) { warnOnce('ws-add', err) }
    }
    return originalAdd.call(this, type, listener, options)
  }

  const originalRemove = proto.removeEventListener
  proto.removeEventListener = function (type, listener, options) {
    if (type === 'message' && typeof listener === 'function') {
      const wrapped = wrapperByListener.get(listener)
      if (wrapped) return originalRemove.call(this, type, wrapped, options)
    }
    return originalRemove.call(this, type, listener, options)
  }

  // 有些实现直接赋值 ws.onmessage，这里也要包一层；getter 还原成客户端自己赋的函数
  try {
    const descriptor = Object.getOwnPropertyDescriptor(proto, 'onmessage')
      || Object.getOwnPropertyDescriptor(window.EventTarget.prototype, 'onmessage')
    if (descriptor && descriptor.set && descriptor.get) {
      Object.defineProperty(proto, 'onmessage', {
        configurable: true,
        enumerable: descriptor.enumerable,
        get() {
          const value = descriptor.get.call(this)
          return (typeof value === 'function' && listenerByWrapper.get(value)) || value
        },
        set(handler) {
          let next = handler
          // 第六轮实测：客户端 databinding 走 onmessage 属性而非 addEventListener，
          // 之前这里没登记 ws，导致 applyNow 的合成推送送不到客户端（pushTargets 只有 1 个）。
          try { next = wrapListener(handler, this) } catch (err) { warnOnce('ws-onmessage', err) }
          descriptor.set.call(this, next)
        },
      })
    }
  } catch (err) {
    warn('onmessage 包装失败（addEventListener 路径仍有效）', err)
  }

  state.installed.ws = true
  log('WebSocket hook 已安装')
}

// ============================================================
// 通道 4（只读）：Pengu socket.observe —— 只确认「有没有推送」，不用来看值
//
// 重要：这条通道拿到的 data 可能**已被通道 3 改写**（Pengu 自己的 socket 也走
// WebSocket.prototype.addEventListener，我们无法把它排除在外）。所以这里只用来
// 判断客户端在哪些 uri 上收到了实时推送；要看服务端真值请用 stats().seen，
// 那份摘要是改写**之前**采集的。
// ============================================================
function installSocketObservers(socket) {
  if (!socket || typeof socket.observe !== 'function') return
  const watch = [
    '/lol-summoner/v1/current-summoner',
    '/lol-summoner/v1/current-summoner/summoner-profile',
    '/lol-chat/v1/me',
    '/lol-inventory/v2/inventory/SUMMONER_ICON',
    '/lol-regalia/v3/inventory/REGALIA_BANNER',
    '/lol-challenges/v1/summary-player-data/local-player',
    '/lol-champ-select/v1/session',
  ]
  let mounted = 0
  for (const uri of watch) {
    try {
      socket.observe(uri, (event) => {
        diag('WS 推送到达', uri, (event && event.eventType) || '?',
             '（值可能已被改写，真值看 stats().seen）')
      })
      mounted++
    } catch (err) {
      warnOnce('socket.observe:' + uri, err)
    }
  }
  log('socket.observe 观测已挂载：', mounted, '/', watch.length, '条')
}

// ============================================================
// 立即生效：伪造 OnJsonApiEvent 推送（v0.4.0）
//
// 客户端界面是数据绑定的：已渲染的视图不会重新发请求，只会在收到 WS 推送时重渲染。
// 我们手里有全部 message 监听器的引用（wrapListener 记录），于是可以：
//   拉一次真值 → 套用改写规则 → 把改写后的 payload 包成 OnJsonApiEvent 帧喂给客户端自己的监听器。
// 纯本地、不写服务端、不需要重启或切页。事件名规则照抄 Pengu socket.ts 的 buildApi。
// ============================================================
function endpointNameFor(uri) {
  return 'OnJsonApiEvent_' + String(uri).toLowerCase().replace(/^\/+|\/+$/g, '').replace(/\//g, '_')
}

function pushSynthetic(uri, data) {
  const frame = JSON.stringify([8, endpointNameFor(uri), { uri, eventType: 'Update', data }])
  let dispatched = 0
  for (const target of state.pushTargets) {
    try {
      target.listener.call(target.ws, new MessageEvent('message', { data: frame }))
      dispatched++
    } catch (err) {
      warnOnce('push:' + uri, err)
    }
  }
  return dispatched
}

async function applyNow() {
  const uris = [
    '/lol-summoner/v1/current-summoner',
    '/lol-chat/v1/me',
    '/lol-regalia/v2/current-summoner/regalia',
    '/lol-challenges/v1/summary-player-data/local-player',
    '/lol-summoner/v1/current-summoner/summoner-profile',
  ]
  if (state.ownSummonerId) uris.push(`/lol-regalia/v2/summoners/${state.ownSummonerId}/regalia`)
  if (state.ownPuuid) uris.push(`/lol-hovercard/v1/friend-info/${state.ownPuuid}`)
  let pushed = 0
  for (const uri of uris) {
    try {
      const truth = await rawFetch(uri)
      if (!truth || typeof truth !== 'object') continue
      const found = findRule(uri)
      if (!found) continue
      const clone = JSON.parse(JSON.stringify(truth))
      if (found.rule.apply(clone, found.path) !== true) continue
      pushed += pushSynthetic(uri, clone)
    } catch (err) {
      warnOnce('applyNow:' + uri, err)
    }
  }
  log('applyNow：向', state.pushTargets.length, '个客户端监听器伪造了', pushed, '次推送（立即生效，无需重启）')
  return pushed
}

// ============================================================
// DOM 层（v0.4.2）：诊断扫描 + 尽力补丁
//
// 第七轮实测：合成推送能让顶部栏立即更新，但生涯页头像/旗帜不吃推送——
// 这类组件要么「显示时才发请求」，要么直接按自定义元素属性渲染
// （sona 的自定义旗帜功能就是改 lol-regalia-*-element 的属性，已产品验证）。
// 所以这里：
//   1) 诊断：把「自己」那些 regalia 元素的标签与全部属性打出来（属性签名变化才重复打），
//      一轮即可拿死旗帜/头像渲染的属性词汇表；
//   2) 尽力补丁：元素已存在 banner-id / 头像类属性时直接设目标值。
// ============================================================
const REGALIA_TAGS = [
  'lol-regalia-profile-v2-element',
  'lol-regalia-banner-v2-element',
  'lol-regalia-parties-v2-element',
  'lol-regalia-crest-v2-element',
  'lol-regalia-hovercard-v2-element',
  'lol-regalia-identity-customizer-element',
]
const ICON_ATTR_CANDIDATES = ['icon-id', 'profile-icon-id', 'summoner-icon-id', 'icon', 'profile-icon']
const domSignatures = new Set()

function hostOf(el) {
  const root = el.getRootNode ? el.getRootNode() : null
  return root && root.host ? root.host : el
}

function isSelfElement(el) {
  const host = hostOf(el)
  const ga = (node, name) => (node && node.getAttribute ? node.getAttribute(name) : null)
  const memberType = ga(host, 'member-type') || ga(el, 'member-type')
  if (memberType) return memberType === 'current-player'
  const ownId = String(state.ownSummonerId || '')
  const sid = ga(host, 'summoner-id') || ga(el, 'summoner-id')
  if (ownId && sid) return sid === ownId
  const ownPuuid = String(state.ownPuuid || '').toLowerCase()
  const p = (ga(host, 'puuid') || ga(el, 'puuid') || '').toLowerCase()
  if (ownPuuid && p) return p === ownPuuid
  // 无任何身份标识 = 选择器列表项等非本人作用域元素（第九轮 DOM 诊断：
  // lol-regalia-banner-v2-element 列表项只有 banner-id/banner-type/class）。
  // 绝不补丁、绝不让位——否则会把每个列表项改成目标值，再被客户端重渲染触发误让位。
  return false
}

function attrsSignature(el) {
  const out = []
  for (const a of el.attributes) out.push(`${a.name}=${a.value}`)
  return out.join('|')
}

function domSweep() {
  if (typeof document === 'undefined' || !CONFIG.domPatch) return 0
  // 让位检测：我们设过的属性被客户端改成别的值 = 原生操作，立即让位并还原
  for (let i = domPatched.length - 1; i >= 0; i--) {
    const entry = domPatched[i]
    let cur = null
    try { cur = entry.el.getAttribute(entry.attr) } catch { domPatched.splice(i, 1); continue }
    if (cur !== entry.target) yieldPreview(entry.kind, `DOM ${entry.attr} 被客户端改为 ${cur}`)
  }
  let patched = 0
  for (const tag of REGALIA_TAGS) {
    let list
    try { list = document.querySelectorAll(tag) } catch { continue }
    for (const el of list) {
      if (!isSelfElement(el)) continue
      const sig = tag + '#' + attrsSignature(el)
      if (!domSignatures.has(sig)) {
        domSignatures.add(sig)
        diag('dom', tag, attrsSignature(el) || '(无属性)')
      }
      if (CONFIG.banner.enabled && CONFIG.banner.id
          && el.hasAttribute('banner-id') && el.getAttribute('banner-id') !== CONFIG.banner.id) {
        recordDomPatch(el, 'banner-id', el.getAttribute('banner-id'), 'banner', CONFIG.banner.id)
        el.setAttribute('banner-id', CONFIG.banner.id)
        patched++
      }
      if (CONFIG.icon.enabled && CONFIG.icon.id) {
        for (const attr of ICON_ATTR_CANDIDATES) {
          if (el.hasAttribute(attr) && el.getAttribute(attr) !== String(CONFIG.icon.id)) {
            recordDomPatch(el, attr, el.getAttribute(attr), 'icon', String(CONFIG.icon.id))
            el.setAttribute(attr, String(CONFIG.icon.id))
            patched++
          }
        }
      }
    }
  }
  return patched
}

// ---------- 像素层补丁（v0.5.0）----------
// 第十一轮实证：改输入属性会让客户端组件尝试向服务端保存未拥有物品 → 400 RPC_ERROR
// "Player does not own REGALIA_BANNER 12" → 组件回滚。所以本地可见只能走像素层：
// 直接替换 shadow DOM 里渲染出来的 img src，不碰输入、不触发保存、不撞所有权墙。
let iconUrlMapPromise = null
const pixelSeen = new WeakSet()
function iconUrlMap() {
  if (!iconUrlMapPromise) {
    iconUrlMapPromise = rawFetch('/lol-game-data/assets/v1/summoner-icons.json')
      .then((list) => {
        const m = new Map()
        if (Array.isArray(list)) {
          for (const it of list) {
            const id = Number(it && it.id)
            if (Number.isFinite(id) && typeof it.imagePath === 'string' && it.imagePath) m.set(id, it.imagePath)
          }
        }
        return m
      })
      .catch(() => new Map())
  }
  return iconUrlMapPromise
}

function collectImgs(root, out) {
  if (!root || !root.querySelectorAll) return out
  for (const img of root.querySelectorAll('img')) out.push(img)
  for (const el of root.querySelectorAll('*')) if (el.shadowRoot) collectImgs(el.shadowRoot, out)
  return out
}

async function pixelSweep() {
  if (typeof document === 'undefined' || !CONFIG.pixelPatch) return 0
  const map = await iconUrlMap()
  const curId = Number(state.lastTruth['current-summoner:icon'])
  const targetId = CONFIG.icon.enabled ? CONFIG.icon.id : 0
  if (!curId || !targetId || curId === targetId) return 0
  const curPath = map.get(curId)
  const targetPath = map.get(targetId)
  if (!curPath || !targetPath) return 0
  const curTail = curPath.split('/').pop()
  let patched = 0
  for (const tag of REGALIA_TAGS) {
    let list
    try { list = document.querySelectorAll(tag) } catch { continue }
    for (const el of list) {
      if (!isSelfElement(el)) continue
      const imgs = collectImgs(el.shadowRoot, [])
      if (el.querySelectorAll) for (const img of el.querySelectorAll('img')) imgs.push(img)
      for (const img of imgs) {
        const src = img.getAttribute ? img.getAttribute('src') : null
        if (!src || src.indexOf(curTail) < 0) continue
        if (!pixelSeen.has(img)) {
          pixelSeen.add(img)
          diag('dom-shadow', tag, 'img src=', src, '->', targetPath)
        }
        img.setAttribute('src', targetPath)
        patched++
      }
    }
  }
  return patched
}

function installDomObserver() {
  if (typeof document === 'undefined' || typeof MutationObserver !== 'function') return
  let timer = null
  const schedule = () => {
    if (timer) return
    timer = setTimeout(() => {
      timer = null
      try { domSweep() } catch (err) { warnOnce('dom-sweep', err) }
    }, 300)
  }
  try {
    const mo = new MutationObserver(schedule)
    mo.observe(document.documentElement, { attributes: true, childList: true, subtree: true })
  } catch (err) { warnOnce('dom-observer', err) }
}

// 运行时 API：在 Console 里直接调，不用重启客户端
// ============================================================
function installRuntimeApi() {
  window.__facadePreview = {
    config: CONFIG,
    state,
    // seen 里是**服务端真值**的结构摘要（改写前采集），counts 是每个端点实际改写次数
    stats: () => ({ counts: { ...state.counts }, seen: { ...state.seen },
                    own: { puuid: state.ownPuuid, summonerId: state.ownSummonerId } }),
    setIcon: (id) => {
      CONFIG.icon.enabled = true; CONFIG.icon.id = Number(id) || 0
      const saved = writePersisted(currentSelection())
      log('目标头像 =', CONFIG.icon.id,
          saved ? '｜已记住（重启后启动缓存界面也生效）' : '｜未能持久化，仅本轮生效')
      // 立即生效：伪造推送让已渲染的界面马上重渲染，不用切页不用重启
      applyNow().catch((err) => warnOnce('applyNow-icon', err))
    },
    setBanner: (id) => {
      CONFIG.banner.enabled = true; CONFIG.banner.id = String(id ?? '')
      const saved = writePersisted(currentSelection())
      log('目标旗帜 =', CONFIG.banner.id, saved ? '｜已记住' : '｜未能持久化，仅本轮生效')
      applyNow().catch((err) => warnOnce('applyNow-banner', err))
    },
    applyNow: () => applyNow().catch((err) => { warnOnce('applyNow', err); return 0 }),
    // 手动跑一次 DOM 诊断扫描+尽力补丁；返回补丁了几处。诊断行看 [diag] dom
    domSweep: () => { try { return domSweep() } catch (err) { warnOnce('dom-sweep', err); return 0 } },
    pixelSweep: () => pixelSweep().catch((err) => { warnOnce('pixel-sweep', err); return 0 }),
    forget: () => {
      CONFIG.icon.enabled = false; CONFIG.icon.id = 0
      CONFIG.banner.enabled = false; CONFIG.banner.id = ''
      writePersisted(currentSelection())
      log('已清除记住的选择（重启客户端后恢复真实外观）')
    },
    // 只停改写，不停拦截（仍会 parse 响应）。要彻底停请用 stopAll 或删脚本文件。
    disable: () => { CONFIG.icon.enabled = false; CONFIG.banner.enabled = false; log('已停用改写（hook 仍在，彻底停用请调 stopAll() 后重载客户端）') },
    stopAll: () => {
      CONFIG.icon.enabled = false; CONFIG.banner.enabled = false
      CONFIG.rewriteXhr = false; CONFIG.rewriteFetch = false; CONFIG.rewriteWebSocket = false
      CONFIG.diagnostic = false
      log('已请求全面停用。注意：已安装的 hook 无法卸载，需重载客户端才真正干净。')
    },
    reloadClient: () => { if (typeof window.reloadClient === 'function') window.reloadClient(); else location.reload() },
    openDevTools: () => { if (typeof window.openDevTools === 'function') window.openDevTools() },
    // 一次性导出排障信息：Console 里执行 copy(__facadePreview.dump()) 后直接粘贴
    dump,
    // 自动挑几个「未拥有且本区域可用」的头像 ID，省掉手工找 ID 这一步
    suggestIcons,
    searchIcons,
    newestIcons,
  }
}

// ============================================================
// Pengu Loader 生命周期
// ============================================================

/** 客户端脚本初始化之前调用，是安装 hook 最早的时机。 */
export function init(context) {
  state.rcp = (context && context.rcp) || null
  state.socket = (context && context.socket) || null

  installRuntimeApi()
  // 每个 hook 单独兜底：任何一个抛异常都不能让 init 整体失败（否则 Pengu 会放弃整个插件，
  // 表现为 Console 里一条 [FacadePreview] 都没有、__facadePreview 不存在）
  for (const step of [installXhrHook, installFetchHook, installWebSocketHook]) {
    try { step() } catch (err) { warn('hook 安装失败：', step.name, err) }
  }
  try { installSocketObservers(state.socket) } catch (err) { warn('socket 观测挂载失败：', err) }

  log('已加载。目标 头像 =', CONFIG.icon.enabled ? CONFIG.icon.id : '(关)',
      ' 旗帜 =', CONFIG.banner.enabled ? (CONFIG.banner.id || '(未填)') : '(关)')
  if (!CONFIG.icon.id && !CONFIG.banner.id) {
    warn('还没填目标 ID：改 CONFIG.icon.id / CONFIG.banner.id，或在 Console 里 __facadePreview.setIcon(4379)')
  }
}

/** window load 之后调用，打一条汇总，方便确认脚本确实跑起来了。 */
export function load() {
  log('client loaded. hooks =', JSON.stringify(state.installed),
      ' ownPuuid =', state.ownPuuid || '(尚未捕获)')
  // 不开 DevTools 也能确认插件活着：Pengu 提供全局 Toast
  try { window.Toast?.success?.(`FacadePreview ${SCRIPT_VERSION} 已加载`) } catch { /* Toast 缺失不影响 */ }
  if (CONFIG.autoOpenDevTools) {
    // 稍等一拍，让客户端自己的初始化先跑完，避免 DevTools 抢焦点导致登录页输入困难
    setTimeout(() => {
      try { window.openDevTools?.() } catch (err) { warnOnce('auto-devtools', err) }
    }, 1500)
  }
  // DOM 层：观察器 + 首次扫描（稍等客户端首屏渲染完）
  installDomObserver()
  setTimeout(() => {
    try { domSweep() } catch (err) { warnOnce('dom-sweep', err) }
    pixelSweep().catch((err) => warnOnce('pixel-sweep', err))
  }, 2000)
  log('提示：copy(__facadePreview.dump()) 一键导出排障 JSON；await __facadePreview.suggestIcons() 挑未拥有头像')
}
