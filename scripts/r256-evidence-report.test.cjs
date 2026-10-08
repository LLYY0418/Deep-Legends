"use strict";
const test = require("node:test"), assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path"), os = require("node:os"), Module = require("node:module");
const report = require("./r256-evidence-report.cjs");
// Every record in this suite is synthetic, never a real-device observation.
function fixture() {
  const rows = [
    {event:"mayhem_rating_lookup",time:"2026-10-08T11:59:59Z",result:"ok",duration_ms:99999,ttfb_ms:99999,bytes:9},
    ...Array.from({length:10},(_, i)=>({event:"mayhem_rating_lookup",time:i === 0 ? Date.parse("2026-10-08T12:00:00Z") : `2026-10-08T13:0${i}:00Z`,result:i < 8 ? "ok" : "failed",duration_ms:(i+1)*100,ttfb_ms:i,bytes:(i+1)*10})),
    {event:"mayhem_rating_lookup",time:"2026-10-08T14:00:00Z",result:"ok",duration_ms:99999,ttfb_ms:99999,bytes:9},
    {event:"facade_load_cost",run_id:"synth-A",time:"2026-10-08T12:00:03Z",total_ms:2500,chat_ms:300,challenges_ms:900,catalog_ms:1300},
    {event:"lcu_request",run_id:"synth-A",time:"2026-10-08T12:00:00Z",path:"/lol-chat/v1/me",count:2,conn_wait_ms:{p90:40}},
    {event:"overview_card_ready",run_id:"synth-A",time:"2026-10-08T12:00:01Z",card:"matches",duration_ms:1000},
    {event:"facade_load_cost",run_id:"synth-A",time:"2026-10-08T12:00:05Z",total_ms:1000},
    {event:"client_cold_launch_timeline",run_id:"synth-A",time:"2026-10-08T12:00:05Z",process_ms:0,connected_ms:600,overview_first_card_ms:1000},
    {event:"overview_card_ready",run_id:"synth-B",time:"2026-10-08T12:00:00Z",card:"matches"},
    {event:"specialist_runes_step_failed",step:"match_ids",errorKind:"timeout",time:"2026-10-08T12:01:00Z",run_id:"synth-A",player_hash:"safe-p",request_id:"safe-r",log_seq:1},
    {event:"specialist_runes_step_failed",step:"match_ids",errorKind:"timeout",time:"2026-10-08T12:01:00Z",run_id:"synth-A",player_hash:"safe-p",request_id:"safe-r",log_seq:2},
    {event:"specialist_runes_step_failed",step:"match_ids",errorKind:"timeout",time:"2026-10-08T12:01:00Z",run_id:"synth-B",player_hash:"safe-p",request_id:"safe-r",log_seq:1},
    {event:"specialist_runes_step_failed",step:"match_ids",errorKind:"timeout",time:"2026-10-08T12:01:00Z",run_id:"synth-A"},
    {event:"facade_load_cost",time:"2026-10-08T12:00:03Z",total_ms:10}
  ];
  return rows.map((record,i)=>({record,file:"synthetic.jsonl",line:i+1,time:report.timestamp(record.time)}));
}
function check(api) {
  const output = api.analyze(fixture());
  assert.equal(output.mayhem_rating_lookup.groups.length,1);
  const peak = output.mayhem_rating_lookup.groups[0];
  assert.equal(peak.beijing_date,"2026-10-08");
  assert.equal(peak.count,10); assert.equal(peak.failure_rate,.2);
  assert.equal(peak.duration_ms.p50,500); assert.equal(peak.duration_ms.p90,900);
  assert.equal(peak.ttfb_ms.p50,4); assert.equal(peak.ttfb_ms.p90,8);
  assert.equal(peak.response_bytes.p90,90);
  assert.deepEqual(peak.lines.map(r=>r.line),[2,3,4,5,6,7,8,9,10,11]);
  const run = output.facade_load_cost.runs.find(r=>r.run_id === "synth-A");
  assert.equal(run.first_card_line.line,15);
  assert.equal(run.facade_assessments[0].status,"无证据");
  assert.equal(run.facade_assessments[1].status,"无证据");
  assert.equal(run.facade_assessments[1].relationship,"开始前该运行已有首卡事件");
  assert.equal(run.timeline.find(r=>r.event === "facade_load_cost").start_relative_ms,500);
  assert.equal(run.timeline.find(r=>r.event === "client_cold_launch_timeline").process_relative_milestones_ms.connected_ms,600);
  assert.deepEqual(output.facade_load_cost.missing[0].missing_fields,["run_id"]);
  const matches = output.match_ids_timeout;
  assert.equal(matches.correlated.length,2);
  assert.deepEqual(matches.correlated[0].missing_fields,["attempt_id","event_id"]);
  assert.equal(matches.correlated[0].count,2); assert.equal(matches.correlated[1].repeated_events,false);
  assert.deepEqual(matches.correlated[0].lines.map(r=>r.line),[19,20]);
  assert.equal(matches.missing[0].status,"缺关联字段");
  assert.deepEqual(matches.missing[0].missing_fields,["player_hash","request_id"]);
  return output;
}
test("R256 synthetic peak statistics, exact lines, same-run timeline and association gaps",()=>check(report));
test("R256 synthetic missing values are unknown rather than zero/success; timezone is mandatory",()=>{
  const row = {record:{event:"mayhem_rating_lookup",time:"2026-10-08T12:00:00Z"},file:"synthetic.jsonl",line:9,time:Date.parse("2026-10-08T12:00:00Z")};
  const group = report.analyze([row]).mayhem_rating_lookup.groups[0];
  assert.equal(group.failure_rate,null); assert.equal(group.unknown_outcomes,1);
  assert.equal(group.ttfb_ms.p50,null); assert.equal(group.ttfb_ms.missing,1);
  assert.equal(report.timestamp("2026-10-08T20:00:00"),null);
  assert.equal(report.timestamp(0),0);
  const dup=fixture().slice(18,20); dup[1].record.log_seq=1;
  assert.equal(report.analyze(dup).match_ids_timeout.correlated[0].duplicated_input_only,true);
});
test("R256 synthetic CLI input preserves blank/invalid line numbers, rejects secret-named and symlink inputs",t=>{
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),"r256-synthetic-"));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
  const file=path.join(dir,"diagnostics.jsonl");
  fs.writeFileSync(file,'\n{"event":"mayhem_rating_lookup","time":"2026-10-08T12:00:00Z"}\ninvalid\n');
  const parsed=report.readLogs([file]); assert.equal(parsed.rows[0].line,2);assert.equal(parsed.errors[0].line,3);
  const secret=path.join(dir,"secret.jsonl"); fs.writeFileSync(secret,"synthetic sentinel");
  assert.throws(()=>report.readLogs([secret]),/Only diagnostic/);
  const link=path.join(dir,"alias.jsonl");fs.symlinkSync(secret,link);assert.throws(()=>report.readLogs([link]),/Only diagnostic/);
  assert.throws(()=>report.readLogs([]),/Pass one/);
});
test("R256 synthetic assertion mutations are killed without source edits or syntax failures",()=>{
  const file=path.join(__dirname,"r256-evidence-report.cjs"),source=fs.readFileSync(file,"utf8");
  const mutations=[
    ["Beijing offset removed","8 * 3600000","0 * 3600000"],
    ["P90 rank understated","Math.ceil(sorted.length * fraction)","Math.floor(sorted.length * fraction) - 1"],
    ["cross-run association","[r.run_id, r.player_hash, r.request_id]","[r.player_hash, r.request_id]"],
    ["overlap misreported as evidence",'status: "无证据"','status: "拖慢首卡"'],
    ["missing request field suppressed",'["run_id", "player_hash", "request_id"].filter','["run_id", "player_hash"].filter'],
    ["line attribution discarded","line: row.line","line: 0"]
  ];
  for(const [name,from,to] of mutations) {
    assert.equal(source.split(from).length,2,"exact mutation replacement: "+name);
    const mutant=new Module(file,module);mutant.filename=file;mutant.paths=module.paths;
    mutant._compile(source.replace(from,to),file); // compilation errors are never a mutation kill
    assert.throws(()=>check(mutant.exports),{name:"AssertionError"},name);
  }
});
module.exports={fixture};
