const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(process.env.R115_SCORING_SOURCE || path.join(__dirname, 'gameplay.js'), 'utf8');
const between = (name, next) => source.slice(source.indexOf(`function ${name}(`), source.indexOf(`function ${next}(`));
const compute = new Function(`${between('participantGroupKey', 'scoreBadgeChip')}; return computeMatchScores;`)();
const person = (id, team, extra = {}) => ({participantId:id, teamId:team, win:team===100,kills:3,assists:6,deaths:3,damage:10000,gold:9000,cs:100,visionScore:10,...extra});
const game = (participants, extra={}) => ({duration:1200,participants,...extra});

test('score v2 is unchanged by participant ordering, including ties and badges',()=>{
 const ps=[person(4,200),person(1,100),person(3,200),person(2,100)];
 const a=compute(game(ps)),b=compute(game([...ps].reverse()));
 for(const p of ps) assert.deepEqual(a.get(p.participantId),b.get(p.participantId));
});
test('missing, null, invalid and all-zero dimensions are omitted for everyone',()=>{
 for(const bad of [undefined,null,NaN,Infinity,-1]) {
  const scores=compute(game([person(1,100,{visionScore:bad}),person(2,200)]));
  for(const row of scores.values()) {assert.ok(!row.components.some(p=>p.key==='vision'));assert.ok(Number.isFinite(row.score));}
 }
 const scores=compute(game([person(1,100,{visionScore:0,wardsPlaced:20}),person(2,200,{visionScore:0,wardsPlaced:1})]));
 assert.ok(!scores.get(1).components.some(p=>p.key==='vision'));
 assert.equal(compute(game([person(1,100),person(1,200)])).size,0);
 assert.equal(compute(game([person(1,100),person(2,200)],{result:'remake'})).size,0);
});
test('equal relative performances in different roles receive equal scores',()=>{
 const roles=['top','jungle','middle','bottom','utility'];
 const ps=[100,200].flatMap((team,ti)=>roles.map((position,i)=>person(ti*5+i+1,team,{position,damage:(i+1)*1000,gold:(i+1)*1000,cs:(i+1)*20,visionScore:50-i*10})));
 const scores=compute(game(ps,{queueId:420}));
 for(const row of scores.values()) {assert.equal(row.score,6);assert.match(row.reference,/同位置/);}
 for (const queueId of [undefined,450,1700,9999]) {
  assert.match(compute(game(ps,{queueId})).get(5).reference,/本场整体/);
 }
 const before=scores.get(5).score;
 ps[0].damage=1e9;
 assert.equal(compute(game(ps,{queueId:420})).get(5).score,before,'top outlier cannot penalize support');
});
test('global median prevents one damage outlier suppressing unrelated players',()=>{
 const ps=[person(1,100),person(2,100),person(3,200),person(4,200)];
 const old=compute(game(ps)).get(2).score;
 ps[0].damage=1e9;
 assert.equal(compute(game(ps)).get(2).score,old);
});
test('assists have full KDA credit, zeros stay finite, victory adds no score',()=>{
 const ps=[person(1,100,{kills:10,assists:0,deaths:0}),person(2,200,{kills:0,assists:10,deaths:0}),person(3,200,{kills:10,assists:0,deaths:0})];
 const scores=compute(game(ps));
 assert.equal(scores.get(1).score,scores.get(2).score);
 const before=scores.get(1).score;ps[0].win=false;
 assert.equal(compute(game(ps)).get(1).score,before);
 assert.ok([...scores.values()].every(r=>r.score>=2&&r.score<=10));
 ps[1].win=undefined;
 assert.ok([...compute(game(ps)).values()].every(r=>!r.badge));
});
test('score tooltip exposes contributions that sum to the unrounded score',()=>{
 const score=compute(game([person(1,100),person(2,200,{damage:20000})])).get(1);
 assert.ok(Math.abs(2+score.components.reduce((s,p)=>s+p.contribution,0)-score.rawScore)<1e-10);
 const chip=new Function(`${between('scoreChip','scorePlacementChip')};return scoreChip;`)();
 assert.match(chip(score),/DL 单场相对评分 v2/);
 assert.match(chip(score),/权重/);assert.match(chip(score),/原值/);assert.match(chip(score),/贡献/);
});
test('incomplete radar preserves valid points without inventing a closed polygon',()=>{
 const render=new Function('escapeHTML','number',`${between('radarPoint','renderRecentRanked')}; return {renderAbility,abilityMetricValue};`)(String,String);
 const metrics=['kda','kp','damage','dpm','cs','gold','vision'].map((key,i)=>({key,label:key,player:10,baseline:10,playerScore:50,grade:'B+',unavailable:i===6}));
 const markup=render.renderAbility({metrics,sampleGames:3,baselineGames:3});
 assert.equal((markup.match(/<circle /g)||[]).length,6);
 assert.doesNotMatch(markup,/<polygon class="ability-radar-(player|baseline)"/);
 assert.equal(render.abilityMetricValue(metrics[6],'player'),'—');
 assert.equal(render.abilityMetricValue({player:null},'player'),'—');
});
