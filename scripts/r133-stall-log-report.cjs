"use strict";
// R133 P2：把「真机 5 分钟手动验证」最后一步的日志判读做成可复核的命令。
//
// 工单要求：导出诊断日志后搜索 card_image_stalled，并按同一时间窗有没有大量
// local_request_client / failed(endpoint=image) 把成因分成两类——「第二层积压」
// 与「第二层放弃却没通知到第一层」，而且明确要求这两种分开处理、不要混在一起改。
// 靠人眼在几千行 JSONL 里数窗口很容易数错，判错方向就会改错东西，所以这里把
// 计数与判定固化成脚本，人只需要看结论和原始数字。
//
// 用法：
//   node scripts/r133-stall-log-report.cjs <lol-loot-diagnostics-*.jsonl> [--json]
//       [--window 60000] [--forward 10000] [--backlog-threshold 3]
//
// 只读日志文件，不连客户端、不读存档、不改任何东西。
//
// 判定口径（证据不足一律降级，不猜）：
//   clean            日志里一条 card_image_stalled 都没有 → P2 可以结项
//   backlog-only     每条 stall 的窗口里都有 >= backlogThreshold 条图片失败
//                    → 第二层在正常超时/重试，名额是被排队等满 CARD_IMAGE_STALL_MS
//                      的卡片占住的；该调的是第二层的远程/本机道名额，不是 R130 的修复
//   notify-gap       有 stall 的窗口里一条图片失败都没有
//                    → 第二层放弃时又没能通知到第一层，合成 error 的某个闸门有漏洞，
//                      这是新缺陷，按项目惯例另开工单
//   inconclusive     图片失败数落在 1..backlogThreshold-1，不足以判方向
//   whitelist-regression  日志里出现 raw_event 含 card_image_stalled 的
//                    client_diagnostic_rejected → 白名单被回退了，优先级最高
//   undated          有 stall 但缺 time 字段，窗口无法计算，不判成因
const fs = require("fs");

const STALL_EVENT = "card_image_stalled";
const REQUEST_EVENT = "local_request_client";
const REJECTED_EVENT = "client_diagnostic_rejected";
const DEFAULT_WINDOW_MS = 60000;
const DEFAULT_FORWARD_MS = 10000;
const DEFAULT_BACKLOG_THRESHOLD = 3;
// card_image_stalled 是 R130 / 0.12.18 才加的事件。判「0 条 stall」之前必须先确认
// 这份日志出自支持该事件的包，否则「没有 stall」只是因为这个包根本不会上报它。
const STALL_EVENT_MIN_VERSION = "0.12.18";

// 逐行解析 JSONL。坏行不静默跳过——记下来一起报，避免「日志被截断了却当成没有 stall」。
function parseDiagnostics(text) {
  const records = [];
  let malformed = 0;
  const lines = String(text).split("\n");
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    let record;
    try {
      record = JSON.parse(trimmed);
    } catch (_) {
      malformed += 1;
      continue;
    }
    if (record && typeof record === "object") records.push(record);
    else malformed += 1;
  }
  return { records, malformed, lines: lines.length };
}

const toTime = (value) => {
  const parsed = Date.parse(String(value || ""));
  return Number.isFinite(parsed) ? parsed : null;
};

// 逐段比数字，非数字段按缺失处理；不引入 semver 依赖。
function compareVersions(left, right) {
  const parse = (value) => String(value || "").split(".").map((part) => {
    const digits = /^\d+/.exec(part);
    return digits ? Number(digits[0]) : NaN;
  });
  const a = parse(left);
  const b = parse(right);
  for (let index = 0; index < Math.max(a.length, b.length); index += 1) {
    const x = a[index];
    const y = b[index];
    const xOK = Number.isFinite(x);
    const yOK = Number.isFinite(y);
    if (!xOK && !yOK) return 0;
    if (!xOK) return -1;
    if (!yOK) return 1;
    if (x !== y) return x < y ? -1 : 1;
  }
  return 0;
}

function isImageFailure(record) {
  return record.event === REQUEST_EVENT && record.reason === "failed" && record.endpoint === "image";
}

function summarizeDiagnostics(records, options = {}) {
  const windowMs = Number.isFinite(options.windowMs) ? options.windowMs : DEFAULT_WINDOW_MS;
  const forwardMs = Number.isFinite(options.forwardMs) ? options.forwardMs : DEFAULT_FORWARD_MS;
  const backlogThreshold = Number.isFinite(options.backlogThreshold) ? options.backlogThreshold : DEFAULT_BACKLOG_THRESHOLD;
  if (windowMs <= 0 || forwardMs < 0 || backlogThreshold < 1) throw new Error("window/forward/backlog-threshold 取值不合法");

  const fingerprints = new Set();
  const runIDs = new Set();
  const versions = new Set();
  let imageFailures = 0;
  const failureKinds = new Map();
  const rejected = [];
  const stalls = [];
  let earliest = null;
  let latest = null;

  for (const record of records) {
    if (record.build_fingerprint) fingerprints.add(String(record.build_fingerprint));
    if (record.run_id) runIDs.add(String(record.run_id));
    if (record.version) versions.add(String(record.version));
    const at = toTime(record.time);
    if (at !== null) {
      if (earliest === null || at < earliest) earliest = at;
      if (latest === null || at > latest) latest = at;
    }
    if (isImageFailure(record)) {
      imageFailures += 1;
      const kind = String(record.error_kind || "none");
      failureKinds.set(kind, (failureKinds.get(kind) || 0) + 1);
    }
    if (record.event === REJECTED_EVENT) rejected.push({ at, reason: String(record.reason || ""), rawEvent: String(record.raw_event || "") });
    if (record.event === STALL_EVENT) {
      stalls.push({
        at,
        runId: record.run_id ? String(record.run_id) : null,
        time: record.time === undefined ? null : String(record.time),
        reason: String(record.reason || ""),
        activeCardImages: record.active_card_images,
        queued: record.queued,
        sourceIndex: record.source_index,
        imageSource: record.image_source === undefined ? null : String(record.image_source),
        buildFingerprint: record.build_fingerprint === undefined ? null : String(record.build_fingerprint),
      });
    }
  }

  // 每条 stall 单独数窗口，同一份导出日志可能包含多次启动。旧日志没有 run_id 时
  // 仍沿用全局窗口；已知 run_id 的 stall 只使用本次启动的图片失败。
  const datedFailures = [];
  const datedFailuresByRun = new Map();
  for (const record of records) {
    if (!isImageFailure(record)) continue;
    const at = toTime(record.time);
    if (at === null) continue;
    datedFailures.push(at);
    if (!record.run_id) continue;
    const runId = String(record.run_id);
    if (!datedFailuresByRun.has(runId)) datedFailuresByRun.set(runId, []);
    datedFailuresByRun.get(runId).push(at);
  }
  for (const stall of stalls) {
    if (stall.at === null) {
      stall.windowImageFailures = null;
      stall.classification = "undated";
      continue;
    }
    const from = stall.at - windowMs;
    const to = stall.at + forwardMs;
    let count = 0;
    const failures = stall.runId === null ? datedFailures : (datedFailuresByRun.get(stall.runId) || []);
    for (const at of failures) if (at >= from && at <= to) count += 1;
    stall.windowImageFailures = count;
    stall.classification = count >= backlogThreshold ? "backlog" : count === 0 ? "notify-gap" : "inconclusive";
  }

  const whitelistRegression = rejected.some((entry) => entry.rawEvent.includes(STALL_EVENT));
  const classifications = new Set(stalls.map((stall) => stall.classification));
  let verdict;
  const knownVersions = [...versions].sort(compareVersions);
  const newestVersion = knownVersions.length ? knownVersions[knownVersions.length - 1] : null;
  const eventSupport = {
    known: newestVersion !== null,
    versions: knownVersions,
    minRequired: STALL_EVENT_MIN_VERSION,
    sufficient: newestVersion !== null && compareVersions(newestVersion, STALL_EVENT_MIN_VERSION) >= 0,
  };
  if (whitelistRegression) verdict = "whitelist-regression";
  else if (stalls.length === 0) verdict = eventSupport.sufficient ? "clean" : "clean-unverified";
  else if (classifications.has("notify-gap")) verdict = "notify-gap";
  else if (classifications.has("undated")) verdict = "undated";
  else if (classifications.has("inconclusive")) verdict = "inconclusive";
  else verdict = "backlog-only";

  return {
    verdict,
    totals: {
      records: records.length,
      stalls: stalls.length,
      imageFailures,
      rejected: rejected.length,
      malformed: 0,
    },
    failureKinds: Object.fromEntries([...failureKinds].sort()),
    identity: {
      buildFingerprints: [...fingerprints].sort(),
      runIDs: [...runIDs].sort(),
      versions: knownVersions,
    },
    eventSupport,
    span: { earliest, latest, durationMs: earliest !== null && latest !== null ? latest - earliest : null },
    window: { windowMs, forwardMs, backlogThreshold },
    stalls,
    rejected: rejected.map((entry) => ({ reason: entry.reason, rawEvent: entry.rawEvent.slice(0, 160) })),
  };
}

const VERDICT_TEXT = {
  clean: "P2 可以结项：这份日志出自支持 card_image_stalled 的包，而整份日志里一条都没有，R130 的修复在真实负载下成立。",
  "clean-unverified": "日志里一条 card_image_stalled 都没有，但**还不能结项**：找不到 0.12.18 或更新的 app_start 版本记录（该事件是 0.12.18 / R130 才加的），「没有 stall」可能只是因为这个包根本不会上报它。请确认跑的是 0.12.18+ 的包再重跑；确认方法：看日志里 app_start 记录的 version 字段，或设置页显示的版本号。",
  "backlog-only": "第二层积压：每条 stall 的窗口里都有大量图片失败，说明第二层在正常超时/重试，名额是被排队等满看门狗时长的卡片占住的。该调的是第二层的远程/本机道名额，不是 R130 的合成 error / 看门狗。",
  "notify-gap": "通知失败：有 stall 的窗口里一条图片失败都没有，说明第二层放弃时又没能通知到第一层，合成 error 的某个闸门有漏洞。这是新缺陷，按项目惯例另开工单，不要和积压混在一起改。",
  inconclusive: "证据不足：窗口里的图片失败数落在 1..阈值-1，不足以判方向。不要据此改任何代码——把 --window 调大重跑，或者重做一次真机验证拿到更明确的样本。",
  undated: "有 stall 但缺 time 字段，窗口无法计算。先确认日志导出是否完整，不要在缺时间戳的情况下判成因。",
  "whitelist-regression": "白名单被回退：日志里出现了 raw_event 含 card_image_stalled 的 client_diagnostic_rejected。这条优先级最高——上报根本没落盘，后面的 stall 计数都是假的。",
};

function formatReport(summary) {
  const lines = [];
  const identity = summary.identity;
  lines.push(`构建指纹：${identity.buildFingerprints.length ? identity.buildFingerprints.join(", ") : "（日志里没有）"}`);
  lines.push(`app_start 版本：${identity.versions.length ? identity.versions.join(", ") : "（日志里没有带 version 的记录）"}`);
  const support = summary.eventSupport;
  lines.push(`card_image_stalled 事件支持：${support.known ? `最新 ${support.versions[support.versions.length - 1]}（需要 >= ${support.minRequired}）→ ${support.sufficient ? "支持" : "不支持，0 条 stall 不算证据"}` : "无法确认（日志里没有带 version 的 app_start 记录），0 条 stall 不算证据"}`);
  lines.push(`运行 ID：${identity.runIDs.length ? identity.runIDs.join(", ") : "（日志里没有）"}`);
  if (identity.runIDs.length > 1) lines.push(`本文件跨 ${identity.runIDs.length} 次启动；stall 窗口按运行 ID 隔离`);
  const span = summary.span;
  if (span.durationMs !== null) {
    const seconds = Math.round(span.durationMs / 1000);
    lines.push(`时间跨度：${new Date(span.earliest).toISOString()} ~ ${new Date(span.latest).toISOString()}（${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒）`);
  } else {
    lines.push("时间跨度：（日志里没有可用的 time 字段）");
  }
  lines.push("");
  lines.push(`记录总数：${summary.totals.records}（解析失败 ${summary.totals.malformed} 行）`);
  lines.push(`card_image_stalled：${summary.totals.stalls} 条`);
  lines.push(`local_request_client/failed(endpoint=image)：${summary.totals.imageFailures} 条${summary.totals.imageFailures ? `（${Object.entries(summary.failureKinds).map(([kind, count]) => `${kind} ${count}`).join("、")}）` : ""}`);
  lines.push(`client_diagnostic_rejected：${summary.totals.rejected} 条`);
  lines.push(`窗口口径：stall 前 ${summary.window.windowMs}ms ~ 后 ${summary.window.forwardMs}ms；积压判定阈值 ${summary.window.backlogThreshold} 条图片失败`);
  if (summary.stalls.length) {
    lines.push("");
    summary.stalls.forEach((stall, index) => {
      lines.push(`  #${index + 1} ${stall.time || "（无 time 字段）"} reason=${stall.reason} active=${stall.activeCardImages} queued=${stall.queued} sourceIndex=${stall.sourceIndex} imageSource=${stall.imageSource === null ? "（无）" : stall.imageSource}`);
      lines.push(`      窗口内图片失败：${stall.windowImageFailures === null ? "无法计算（缺 time）" : `${stall.windowImageFailures} 条`} → ${stall.classification}`);
    });
  }
  if (summary.rejected.length) {
    lines.push("");
    lines.push("被拒收的上报：");
    for (const entry of summary.rejected.slice(0, 10)) lines.push(`  ${entry.reason}: ${entry.rawEvent}`);
    if (summary.rejected.length > 10) lines.push(`  …另外 ${summary.rejected.length - 10} 条`);
  }
  lines.push("");
  lines.push(`判定：${summary.verdict}`);
  lines.push(VERDICT_TEXT[summary.verdict] || "");
  return lines.join("\n");
}

function readDiagnostics(file, options = {}) {
  const text = fs.readFileSync(file, "utf8");
  const parsed = parseDiagnostics(text);
  const summary = summarizeDiagnostics(parsed.records, options);
  summary.totals.malformed = parsed.malformed;
  summary.source = { file, lines: parsed.lines };
  return summary;
}

function parseArgs(argv) {
  const options = { file: null, json: false };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--json") options.json = true;
    else if (arg === "--window") options.windowMs = Number(argv[++index]);
    else if (arg === "--forward") options.forwardMs = Number(argv[++index]);
    else if (arg === "--backlog-threshold") options.backlogThreshold = Number(argv[++index]);
    else if (arg.startsWith("--")) throw new Error(`不认识的参数：${arg}`);
    else if (options.file) throw new Error("只能指定一个日志文件");
    else options.file = arg;
  }
  return options;
}

function main(argv) {
  const options = parseArgs(argv);
  if (!options.file) {
    process.stderr.write("用法：node scripts/r133-stall-log-report.cjs <lol-loot-diagnostics-*.jsonl> [--json] [--window 60000] [--forward 10000] [--backlog-threshold 3]\n");
    return 2;
  }
  const summary = readDiagnostics(options.file, options);
  if (options.json) process.stdout.write(`${JSON.stringify(summary, null, 2)}\n`);
  else process.stdout.write(`${formatReport(summary)}\n`);
  // 退出码给 CI/脚本用：0 = 干净或只是积压，1 = 需要开工单或证据不足，2 = 用法错误。
  return ["clean", "backlog-only"].includes(summary.verdict) ? 0 : 1;
}

if (require.main === module) process.exitCode = main(process.argv.slice(2));

module.exports = { parseDiagnostics, summarizeDiagnostics, formatReport, readDiagnostics, parseArgs, main, compareVersions, VERDICT_TEXT, STALL_EVENT, STALL_EVENT_MIN_VERSION, DEFAULT_WINDOW_MS, DEFAULT_FORWARD_MS, DEFAULT_BACKLOG_THRESHOLD };
