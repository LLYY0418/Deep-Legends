"use strict";
// R133 P2 的日志判读脚本自身的测试。
//
// 下面的日志全部是合成夹具，只验证计数与判定口径——**不是**任何一次真机 5 分钟
// 手动验证的结果。真机跑完之后拿真实 jsonl 过一遍脚本，结论才算数。
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { parseDiagnostics, summarizeDiagnostics, formatReport, readDiagnostics, parseArgs, main, compareVersions, VERDICT_TEXT, STALL_EVENT_MIN_VERSION } = require("./r133-stall-log-report.cjs");

const BASE = Date.parse("2026-09-23T10:00:00.000Z");
const at = (offsetMs) => new Date(BASE + offsetMs).toISOString();
const line = (record) => JSON.stringify(record);

const imageFailure = (offsetMs, errorKind = "timeout") => ({
  build_fingerprint: "62b5b7b6b536", event: "local_request_client", reason: "failed", endpoint: "image",
  error_kind: errorKind, http_status: 0, image_source: "gtimg", queue_wait_ms: 4210, load_ms: 10001,
  log_seq: 1, run_id: "794083ef4812f210ecde108e", time: at(offsetMs),
});
const stall = (offsetMs, extra = {}) => ({
  build_fingerprint: "62b5b7b6b536", event: "card_image_stalled", reason: "watchdog",
  active_card_images: 8, queued: 6, source_index: 0, image_source: "gtimg",
  log_seq: 2, run_id: "794083ef4812f210ecde108e", time: at(offsetMs), ...extra,
});
const rejected = (offsetMs, rawEvent, reason = "unknown-event") => ({
  build_fingerprint: "62b5b7b6b536", event: "client_diagnostic_rejected", reason,
  raw_event: rawEvent, log_seq: 3, run_id: "794083ef4812f210ecde108e", time: at(offsetMs),
});
const jsonl = (records) => `${records.map(line).join("\n")}\n`;
// 真实日志里只有 app_start 记录带 version，这也是判断「这个包会不会上报
// card_image_stalled」的唯一可靠依据。
const appStart = (version, offsetMs = -600000) => ({
  build_fingerprint: "62b5b7b6b536", event: "app_start", version, riot_key: true,
  log_seq: 0, run_id: "794083ef4812f210ecde108e", time: at(offsetMs),
});

function summarize(records, options) {
  const parsed = parseDiagnostics(jsonl(records));
  assert.equal(parsed.malformed, 0, "夹具本身不该有坏行");
  const summary = summarizeDiagnostics(parsed.records, options);
  summary.totals.malformed = parsed.malformed;
  return summary;
}

test("R133 一条 stall 都没有、且日志出自支持该事件的包时判 clean，P2 可以直接结项", () => {
  const summary = summarize([appStart(STALL_EVENT_MIN_VERSION), imageFailure(-5000), imageFailure(-4000), { event: "asset_fetch", host: "ddragon.leagueoflegends.com", time: at(-3000) }]);
  assert.equal(summary.verdict, "clean");
  assert.equal(summary.totals.stalls, 0);
  assert.equal(summary.totals.imageFailures, 2);
  assert.equal(summary.eventSupport.sufficient, true);
  assert.match(formatReport(summary), /P2 可以结项/);
});

test("R133 对抗变异：旧包的日志里 0 条 stall 不得判成 clean", () => {
  // card_image_stalled 是 0.12.18 / R130 才加的事件。拿一份 0.12.15 的日志跑这个
  // 脚本，「0 条 stall」只是因为这个包根本不会上报它——判成 clean 就等于凭空
  // 宣布 P2 通过。这正是拿真实旧日志（docs/r89-kr-player/）冒烟时暴露出来的。
  for (const version of ["0.12.15", "0.12.17", "0.11.99"]) {
    const summary = summarize([appStart(version), imageFailure(-5000)]);
    assert.equal(summary.verdict, "clean-unverified", `${version} 的日志不该判 clean`);
    assert.equal(summary.eventSupport.sufficient, false);
    assert.match(formatReport(summary), /还不能结项/);
    assert.match(formatReport(summary), /0 条 stall 不算证据|不支持/);
  }
  // 日志里根本没有 app_start（导出片段、被截断）时同样不能判 clean。
  const noStart = summarize([imageFailure(-5000)]);
  assert.equal(noStart.verdict, "clean-unverified");
  assert.equal(noStart.eventSupport.known, false);
  assert.match(formatReport(noStart), /无法确认/);
  // 更新的包可以判 clean；同一份日志里混了多个版本时按最新的那个算。
  assert.equal(summarize([appStart("0.12.15", -900000), appStart("0.13.0", -800000)]).verdict, "clean");
  // 有 stall 的时候版本不影响结论：能报出 stall 就说明这个包支持该事件。
  assert.equal(summarize([appStart("0.12.15"), stall(0)]).verdict, "notify-gap");
});

test("R133 版本比较不引入 semver 依赖也够用", () => {
  assert.equal(compareVersions("0.12.18", "0.12.18"), 0);
  assert.equal(compareVersions("0.12.19", "0.12.18"), 1);
  assert.equal(compareVersions("0.12.17", "0.12.18"), -1);
  assert.equal(compareVersions("0.13.0", "0.12.99"), 1);
  assert.equal(compareVersions("1.0.0", "0.99.99"), 1);
  assert.equal(compareVersions("0.12.18-beta", "0.12.18"), 0, "非数字后缀不参与比较");
  assert.deepEqual(["0.12.9", "0.12.18", "0.13.0", "0.12.10"].sort(compareVersions), ["0.12.9", "0.12.10", "0.12.18", "0.13.0"], "必须按数值排序，不能按字符串排");
});

test("R133 stall 窗口里有大量图片失败 → 第二层积压，不是 R130 的修复失效", () => {
  const summary = summarize([
    imageFailure(-40000), imageFailure(-30000), imageFailure(-20000), imageFailure(-10000), imageFailure(-1000),
    stall(0),
  ]);
  assert.equal(summary.verdict, "backlog-only");
  assert.equal(summary.stalls.length, 1);
  assert.equal(summary.stalls[0].windowImageFailures, 5);
  assert.equal(summary.stalls[0].classification, "backlog");
  assert.deepEqual(summary.failureKinds, { timeout: 5 });
  assert.match(formatReport(summary), /该调的是第二层的远程\/本机道名额/);
});

test("R134 对抗变异：另一次启动的图片失败不能把通知缺口判成积压", () => {
  const records = [
    { ...appStart(STALL_EVENT_MIN_VERSION, -30000), run_id: "runA" },
    { ...stall(0), run_id: "runA" },
    { ...appStart(STALL_EVENT_MIN_VERSION, -30000), run_id: "runB" },
    ...[-20000, -15000, -10000, -5000].map((offset) => ({ ...imageFailure(offset), run_id: "runB" })),
  ];
  const summary = summarize(records);
  assert.equal(summary.totals.imageFailures, 4, "全文件仍统计四条图片失败");
  assert.equal(summary.stalls[0].windowImageFailures, 0, "runB 的失败不能计入 runA 的窗口");
  assert.equal(summary.verdict, "notify-gap");
  assert.match(formatReport(summary), /本文件跨 2 次启动/);
});

test("R134 同一次启动的图片失败仍能判积压，缺 run_id 的旧日志沿用全局窗口", () => {
  const ownFailures = [-20000, -15000, -10000].map((offset) => ({ ...imageFailure(offset), run_id: "runA" }));
  const summary = summarize([
    { ...appStart(STALL_EVENT_MIN_VERSION), run_id: "runA" },
    ...ownFailures,
    { ...stall(0), run_id: "runA" },
    { ...appStart(STALL_EVENT_MIN_VERSION), run_id: "runB" },
  ]);
  assert.equal(summary.stalls[0].windowImageFailures, 3);
  assert.equal(summary.verdict, "backlog-only");

  const withoutRunID = ({ run_id, ...record }) => record;
  const legacy = summarize([...ownFailures.map(withoutRunID), withoutRunID(stall(0))]);
  assert.equal(legacy.stalls[0].windowImageFailures, 3);
  assert.equal(legacy.verdict, "backlog-only");
  const unknownStall = summarize([...ownFailures, withoutRunID(stall(0))]);
  assert.equal(unknownStall.stalls[0].windowImageFailures, 3, "stall 没有 run_id 时继续按全局窗口计数");
});

test("R133 stall 窗口里一条图片失败都没有 → 通知失败，必须另开工单", () => {
  // 第二层放弃时如果没能通知第一层，它自己的 failed 上报也不会出现，
  // 所以「有 stall 但窗口里零图片失败」正是合成 error 闸门有漏洞的指纹。
  const summary = summarize([{ event: "catalog_client", reason: "loaded", endpoint: "items", time: at(-20000) }, stall(0)]);
  assert.equal(summary.verdict, "notify-gap");
  assert.equal(summary.stalls[0].windowImageFailures, 0);
  assert.equal(summary.stalls[0].classification, "notify-gap");
  assert.match(formatReport(summary), /另开工单/);
  assert.match(formatReport(summary), /不要和积压混在一起改/);
});

test("R133 图片失败数不够阈值时判 inconclusive，不猜方向", () => {
  const summary = summarize([imageFailure(-20000), imageFailure(-5000), stall(0)]);
  assert.equal(summary.verdict, "inconclusive");
  assert.equal(summary.stalls[0].windowImageFailures, 2);
  assert.match(formatReport(summary), /不要据此改任何代码/);
  // 阈值可调：同一份数据把阈值降到 2 就够判成积压了。
  assert.equal(summarize([imageFailure(-20000), imageFailure(-5000), stall(0)], { backlogThreshold: 2 }).verdict, "backlog-only");
});

test("R133 对抗变异：把窗口算错（全量计数当成窗口计数）必须被测出来", () => {
  // 5 条失败里只有 2 条落在 stall 的窗口内，另外 3 条远在窗口之外。
  // 如果脚本拿全局总数当窗口计数，这条会判成 backlog-only 而不是 inconclusive。
  const records = [
    imageFailure(-600000), imageFailure(-500000), imageFailure(-400000),
    imageFailure(-20000), imageFailure(-5000),
    stall(0),
  ];
  const summary = summarize(records);
  assert.equal(summary.totals.imageFailures, 5, "全局计数是 5");
  assert.equal(summary.stalls[0].windowImageFailures, 2, "窗口内只有 2 条");
  assert.equal(summary.verdict, "inconclusive", "窗口计数没生效就会误判成积压");
  // 边界：正好在 -windowMs 与 +forwardMs 上的算进窗口，再远一毫秒就不算。
  const boundary = summarize([imageFailure(-60000), imageFailure(-60001), imageFailure(10000), imageFailure(10001), stall(0)]);
  assert.equal(boundary.stalls[0].windowImageFailures, 2);
  // 窗口可调。
  assert.equal(summarize(records, { windowMs: 700000 }).stalls[0].windowImageFailures, 5);
});

test("R133 白名单被回退时优先级最高，盖过其它判定", () => {
  const summary = summarize([
    rejected(-1000, "card_image_stalled"),
    imageFailure(-900), imageFailure(-800), imageFailure(-700),
    stall(0),
  ]);
  assert.equal(summary.verdict, "whitelist-regression");
  assert.match(formatReport(summary), /上报根本没落盘/);
  // 拒收的是别的事件时不算白名单回退。
  const unrelated = summarize([rejected(-1000, "some_other_event"), imageFailure(-900), imageFailure(-800), imageFailure(-700), stall(0)]);
  assert.equal(unrelated.verdict, "backlog-only");
  assert.equal(unrelated.totals.rejected, 1);
});

test("R133 缺 time 字段的 stall 判 undated，不在缺时间戳的情况下判成因", () => {
  const noTime = stall(0);
  delete noTime.time;
  const summary = summarize([imageFailure(-1000), noTime]);
  assert.equal(summary.verdict, "undated");
  assert.equal(summary.stalls[0].windowImageFailures, null);
  assert.match(formatReport(summary), /无法计算（缺 time）/);
});

test("R133 多条 stall 各自判，不拿全局总数摊平", () => {
  // 前一批卡顿是积压（窗口内 4 条失败），后一批是通知失败（窗口内 0 条）。
  // 只要有一条 notify-gap，整体就必须是 notify-gap——不能因为另一条是积压就放行。
  const summary = summarize([
    imageFailure(-40000), imageFailure(-35000), imageFailure(-30000), imageFailure(-25000),
    stall(-20000),
    stall(120000),
  ]);
  assert.equal(summary.totals.stalls, 2);
  assert.equal(summary.stalls[0].classification, "backlog");
  assert.equal(summary.stalls[1].classification, "notify-gap");
  assert.equal(summary.verdict, "notify-gap");
});

test("R133 坏行不静默跳过，避免把截断的日志当成没有 stall", () => {
  const parsed = parseDiagnostics(`${line(imageFailure(-1000))}\n{"event": "card_image_sta\n\nnot json at all\n${line(stall(0))}\n`);
  assert.equal(parsed.records.length, 2);
  assert.equal(parsed.malformed, 2);
  const summary = summarizeDiagnostics(parsed.records);
  summary.totals.malformed = parsed.malformed;
  assert.equal(summary.totals.stalls, 1);
  assert.match(formatReport(summary), /解析失败 2 行/);
});

test("R133 报告里带上构建指纹与时间跨度，读数的人能确认是不是 0.12.18 那个包", () => {
  const summary = summarize([imageFailure(-300000), stall(0), imageFailure(1000)]);
  const report = formatReport(summary);
  assert.match(report, /62b5b7b6b536/, "R130 的诊断日志是 0.12.15 的包，指纹必须一眼能看到");
  assert.match(report, /794083ef4812f210ecde108e/);
  assert.match(report, /5 分 1 秒/);
  assert.match(report, /active=8 queued=6 sourceIndex=0 imageSource=gtimg/);
  assert.match(report, /窗口口径：stall 前 60000ms ~ 后 10000ms；积压判定阈值 3 条图片失败/);
  // 报告不能只给结论，原始数字必须在，方便复核。
  assert.match(report, /card_image_stalled：1 条/);
  assert.match(report, /local_request_client\/failed\(endpoint=image\)：2 条/);
  assert.match(report, /timeout 2/);
});

test("R133 命令行：参数解析、退出码与 --json", () => {
  assert.deepEqual(parseArgs(["log.jsonl"]), { file: "log.jsonl", json: false });
  assert.deepEqual(parseArgs(["log.jsonl", "--json", "--window", "30000", "--forward", "5000", "--backlog-threshold", "2"]),
    { file: "log.jsonl", json: true, windowMs: 30000, forwardMs: 5000, backlogThreshold: 2 });
  assert.throws(() => parseArgs(["--nope"]), /不认识的参数/);
  assert.throws(() => parseArgs(["a.jsonl", "b.jsonl"]), /只能指定一个日志文件/);
  assert.throws(() => summarizeDiagnostics([], { windowMs: 0 }), /取值不合法/);
  assert.throws(() => summarizeDiagnostics([], { backlogThreshold: 0 }), /取值不合法/);

  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "r133-"));
  try {
    const backlogFile = path.join(dir, "backlog.jsonl");
    fs.writeFileSync(backlogFile, jsonl([imageFailure(-40000), imageFailure(-30000), imageFailure(-20000), stall(0)]));
    assert.deepEqual(readDiagnostics(backlogFile).verdict, "backlog-only");
    assert.equal(readDiagnostics(backlogFile).source.lines, 5);
    const gapFile = path.join(dir, "gap.jsonl");
    fs.writeFileSync(gapFile, jsonl([stall(0)]));
    // 退出码：0 = 干净或只是积压，1 = 需要开工单或证据不足。
    assert.equal(main([backlogFile]), 0);
    assert.equal(main([gapFile]), 1);
    assert.equal(main([gapFile, "--json"]), 1);
    assert.equal(main([]), 2);
  } finally { fs.rmSync(dir, { recursive: true, force: true }); }
});

test("R133 每种判定都有对应的人话结论，不会输出空判词", () => {
  for (const verdict of ["clean", "clean-unverified", "backlog-only", "notify-gap", "inconclusive", "undated", "whitelist-regression"]) {
    assert.ok(typeof VERDICT_TEXT[verdict] === "string" && VERDICT_TEXT[verdict].length > 10, `${verdict} 缺判词`);
  }
});
