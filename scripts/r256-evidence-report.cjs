"use strict";
// Read-only diagnostic analysis. Only explicit JSONL arguments are opened;
// credentials and unrelated record fields are never copied to the report.
const fs = require("node:fs"), path = require("node:path");
const TZ_OFFSET_MS = 8 * 3600000;
const number = value => typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
function timestamp(value) {
  if (typeof value === "number") return number(value); // diagnostic numeric times are Unix milliseconds
  if (typeof value !== "string" || !/(?:Z|[+-]\d{2}:\d{2})$/.test(value)) return null;
  const ms = Date.parse(value);
  return Number.isFinite(ms) ? ms : null;
}
function percentile(values, fraction) {
  const sorted = values.filter(value => number(value) !== null).sort((a, b) => a - b);
  return sorted.length ? sorted[Math.max(0, Math.ceil(sorted.length * fraction) - 1)] : null;
}
function distribution(values) {
  const valid = values.filter(value => number(value) !== null);
  return { samples: valid.length, missing: values.length - valid.length,
    p50: percentile(valid, .5), p90: percentile(valid, .9),
    min: valid.length ? Math.min(...valid) : null, max: valid.length ? Math.max(...valid) : null };
}
const ref = row => ({ file: row.file, line: row.line });
function readLogs(files) {
  if (!files.length) throw Error("Pass one or more diagnostic *.jsonl paths");
  const rows = [], errors = [];
  for (const input of files) {
    const resolved = fs.realpathSync(input);
    if (!input.endsWith(".jsonl") || !resolved.endsWith(".jsonl") ||
        /(?:^|[/\\])(?:manage|deep-legends-manage|secrets?|\.dev\.vars|\.ssh|\.aws)(?:[/\\]|$)/i.test(resolved) ||
        /(?:private[-_]?key|secret|credential|\.dev\.vars)/i.test(path.basename(resolved))) throw Error("Only diagnostic JSONL files are allowed: " + input);
    if (!fs.statSync(resolved).isFile()) throw Error("Not a regular diagnostic file: " + input);
    fs.readFileSync(resolved, "utf8").split(/\r?\n/).forEach((line, index) => {
      if (!line.trim()) return;
      try {
        const record = JSON.parse(line);
        if (!record || typeof record !== "object" || Array.isArray(record)) throw Error("record");
        rows.push({ record, file: resolved, line: index + 1, time: timestamp(record.time) });
      } catch { errors.push({ file: resolved, line: index + 1, reason: "invalid JSON record" }); }
    });
  }
  return { rows, errors };
}
function mayhemReport(rows) {
  const groups = new Map(), excluded = [];
  for (const row of rows.filter(row => row.record.event === "mayhem_rating_lookup")) {
    if (row.time === null) { excluded.push({ ...ref(row), reason: "缺有效time（须带时区ISO或Unix毫秒）" }); continue; }
    const local = new Date(row.time + TZ_OFFSET_MS), hour = local.getUTCHours();
    if (hour < 20 || hour >= 22) { excluded.push({ ...ref(row), reason: "不在北京时间[20:00,22:00)" }); continue; }
    const key = local.toISOString().slice(0, 10);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(row);
  }
  return { event: "mayhem_rating_lookup", timezone: "Asia/Shanghai", window: "[20:00,22:00)", percentile_method: "nearest-rank",
    groups: [...groups].sort(([a], [b]) => a.localeCompare(b)).map(([date, members]) => {
      const success = new Set(["ok", "success", "available"]), failures = new Set(["failed", "failure", "error", "unavailable", "timeout", "rate-limit"]);
      const outcomes = members.map(({ record: r }) => success.has(r.result) ? false : failures.has(r.result) || (number(r.http_status) !== null && r.http_status >= 400) ? true : null);
      const failed = outcomes.filter(value => value === true).length, unknown = outcomes.filter(value => value === null).length;
      return { beijing_date: date, count: members.length, hourly_counts: [20, 21].map(hour => ({ hour, count: members.filter(row => new Date(row.time + TZ_OFFSET_MS).getUTCHours() === hour).length })),
        failures: failed, unknown_outcomes: unknown, failure_rate: unknown ? null : failed / members.length,
        known_failure_rate: members.length === unknown ? null : failed / (members.length - unknown),
        ttfb_ms: distribution(members.map(row => row.record.ttfb_ms)), duration_ms: distribution(members.map(row => row.record.duration_ms)),
        response_bytes: distribution(members.map(row => row.record.bytes)),
        lines: members.map(row => ({ ...ref(row), time: row.record.time, result: row.record.result ?? null,
          ttfb_ms: number(row.record.ttfb_ms), duration_ms: number(row.record.duration_ms), bytes: number(row.record.bytes) })) };
    }), excluded };
}
function facadeReport(rows) {
  const relevant = new Set(["facade_load_cost", "client_cold_launch_timeline", "overview_card_ready", "lcu_request", "overview_load_cost"]);
  const runs = new Map(), missing = [];
  for (const row of rows.filter(row => relevant.has(row.record.event))) {
    const fields = [];
    if (!row.record.run_id) fields.push("run_id");
    if (row.time === null) fields.push("time");
    if (fields.length) { missing.push({ ...ref(row), status: "缺关联字段", missing_fields: fields }); continue; }
    if (!runs.has(row.record.run_id)) runs.set(row.record.run_id, []);
    runs.get(row.record.run_id).push(row);
  }
  return { event: "facade_load_cost", missing, runs: [...runs].map(([run_id, members]) => {
    members.sort((a, b) => a.time - b.time || a.line - b.line);
    const origin = members[0].time;
    const cards = members.filter(row => row.record.event === "overview_card_ready");
    const first = cards[0];
    return { run_id, origin: new Date(origin).toISOString(), origin_kind: "最早相关日志事件时间（非进程启动时间）",
      first_card_line: first ? ref(first) : null,
      timeline: members.map(row => {
        const r = row.record, relative_ms = row.time - origin;
        const data = { ...ref(row), event: r.event, relative_ms };
        if (r.event === "facade_load_cost") {
          const duration = number(r.total_ms);
          Object.assign(data, { start_relative_ms: duration === null ? null : relative_ms - duration, total_ms: duration,
            chat_ms: number(r.chat_ms), challenges_ms: number(r.challenges_ms), catalog_ms: number(r.catalog_ms) });
        } else if (r.event === "client_cold_launch_timeline") {
          data.process_relative_milestones_ms = Object.fromEntries(["process_ms", "port_open_ms", "connected_ms", "identity_ms", "overview_first_card_ms", "overlay_hidden_ms"].map(key => [key, number(r[key])]));
          data.note = "里程碑相对process_ms=0；记录写出时间不是启动时间，不能反推绝对起点";
        } else if (r.event === "lcu_request") Object.assign(data, { path: r.path, count: number(r.count), window_start: r.window_start ?? null,
          conn_wait_p90_ms: number(r.conn_wait_ms?.p90), ttfb_p90_ms: number(r.ttfb_ms?.p90), duration_p90_ms: number(r.duration_ms?.p90), note: "聚合窗口，无法关联单次共享连接" });
        else if (r.event === "overview_card_ready") Object.assign(data, { card: r.card, source: r.source, duration_ms: number(r.duration_ms) });
        else Object.assign(data, { duration_ms: number(r.duration_ms), sgp_requests: number(r.sgp_requests) });
        return data;
      }), facade_assessments: members.filter(row => row.record.event === "facade_load_cost").map(row => {
        const duration = number(row.record.total_ms);
        const after = first && duration !== null && row.time - duration >= first.time;
        return { ...ref(row), first_card_line: first ? ref(first) : null,
          status: "无证据",
          relationship: after ? "开始前该运行已有首卡事件" : "可能重叠或字段不足",
          conclusion: after ? "只证明该运行已有卡片；缺overview_request_id，无法判定是否同一加载或facade是否拖慢目标首卡" : first ? "存在时间重叠或顺序关系；无法证明facade拖慢首卡" : "缺首卡事件，无法判断",
          missing_fields: ["facade_request_id", "lcu_connection_id", "lcu_request_id", "overview_request_id", "connection_queue_start_ms", "connection_queue_end_ms"] };
      }) };
  }) };
}
function matchIDsReport(rows) {
  const groups = new Map(), missing = [];
  for (const row of rows.filter(row => row.record.event === "specialist_runes_step_failed" && row.record.step === "match_ids" && /timeout|deadline/i.test(row.record.errorKind || row.record.reason || ""))) {
    const r = row.record, fields = ["run_id", "player_hash", "request_id"].filter(key => !r[key]);
    if (fields.length) { missing.push({ ...ref(row), status: "缺关联字段", missing_fields: fields }); continue; }
    const key = JSON.stringify([r.run_id, r.player_hash, r.request_id]);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(row);
  }
  return { event: "match_ids timeout", missing, correlated: [...groups.values()].map(members => {
    const r = members[0].record;
    const uniqueSequences = new Set(members.map(row => row.record.log_seq).filter(value => value !== undefined));
    const allSequences = members.every(row => row.record.log_seq !== undefined);
    return { run_id: r.run_id, player_hash: r.player_hash, request_id: r.request_id, count: members.length,
      missing_fields: ["attempt_id", "event_id"].filter(key => members.some(row => !row.record[key])),
      repeated_events: members.length > 1, duplicated_input_only: allSequences && members.length > 1 && uniqueSequences.size === 1,
      conclusion: members.length > 1 ? "同一player/request关联多条记录；缺attempt_id/event_id时不能判定非法重复还是合法重试/日志副本" : "未发现同一关联重复事件",
      lines: members.map(row => ({ ...ref(row), time: row.record.time ?? null, log_seq: row.record.log_seq ?? null, errorKind: row.record.errorKind })) };
  }) };
}
function analyze(rows, errors = []) {
  return { schema_version: 1, evidence_kind: "输入诊断日志；是否真实取决于输入来源", parse_errors: errors,
    mayhem_rating_lookup: mayhemReport(rows), facade_load_cost: facadeReport(rows), match_ids_timeout: matchIDsReport(rows) };
}
module.exports = { timestamp, percentile, readLogs, analyze };
if (require.main === module) {
  try { const { rows, errors } = readLogs(process.argv.slice(2)); process.stdout.write(JSON.stringify(analyze(rows, errors), null, 2) + "\n"); if (errors.length) process.exitCode = 1; }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
