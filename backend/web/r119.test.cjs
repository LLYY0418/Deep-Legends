'use strict';
// R119：对局详情（召唤师峡谷单双排 / 灵活组排）里给队友与对手打「补位」标签。
//
// 这里执行真实的 autofillChip 与 matchTableRows，不是对源码做正则断言——
// R118 已经把「只匹配源码文本」的护栏判定为假护栏。
const test = require('node:test'), assert = require('node:assert/strict'), fs = require('node:fs'), vm = require('node:vm'), path = require('node:path');

const read = (file) => fs.readFileSync(path.join(process.env.R119_WEB_ROOT || __dirname, file), 'utf8');

function extract(file, name) {
  const source = read(file);
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `${name} 必须存在于 ${file}`);
  return source.slice(start, source.indexOf('\n  }', start) + 4);
}

function compile(file, names, deps = {}) {
  const context = { ...deps };
  vm.runInNewContext(names.map((name) => extract(file, name)).join('\n'), context);
  return context;
}

// positionLabel 在 gameplay.js 里是单行函数，extract 的「\n  }」终止符抓不到它，
// 因此按真实映射表提供同名替身；被测对象是 autofillChip / matchTableRows 本身。
const positionLabel = (value) => ({ top: '上路', jungle: '打野', middle: '中路', bottom: '下路', utility: '辅助', other: '其他' })[value] || '位置未知';
const escapeHTML = (value) => String(value).replace(/&/g, '&amp;').replace(/"/g, '&quot;');

function detailHarness() {
  return compile('gameplay.js', ['matchTableRows', 'autofillChip'], {
    positionLabel,
    escapeHTML,
    playerParticipantName: (player) => player.gameName,
    proBadgeAttributes: () => '',
    iconFigure: () => '<i class="champion-icon"></i>',
    renderMatchScoreCell: () => '<span class="score"></span>',
    renderItemIcons: () => '',
    number: (value) => String(value ?? 0),
    kda: () => '2.0',
  });
}

test('R119 补位标签只认后端下发的布尔 true，证据缺失时一个都不渲染', () => {
  const context = detailHarness();
  const chip = context.autofillChip({ autofill: true, position: 'utility' });
  assert.match(chip, /^<b class="match-autofill-chip"[^>]*>补位<\/b>$/, chip);
  assert.match(chip, /个人位置（推算位置：辅助）/, 'R121 P1-2：提示只陈述数据事实，并带上推算位置');
  assert.doesNotMatch(chip, /由系统补位|他自己选到的/, 'R121 P1-2：不得把未证实的推断写成对玩家的事实陈述');
  assert.match(chip, /data-tooltip-size="compact"/);
  for (const player of [{}, { autofill: false }, { autofill: undefined }, { autofill: null }, { autofill: 'true' }, { autofill: 1 }, null, undefined]) {
    assert.equal(context.autofillChip(player), '', `${JSON.stringify(player)} 不该出标签`);
  }
});

test('R119 详情概览的队友/对手行按参与者逐个决定是否带补位标签', () => {
  const context = detailHarness();
  const scores = new Map();
  const players = [
    { participantId: 1, gameName: '正常选位', position: 'top' },
    { participantId: 2, gameName: '补位辅助', position: 'utility', autofill: true },
    { participantId: 3, gameName: '换位中单', position: 'middle' },
  ];
  const html = context.matchTableRows(players, scores);
  assert.equal((html.match(/match-autofill-chip/g) || []).length, 1, '十个人里只有拿到证据的那个人该被标注');
  assert.match(html, /<span class="participant-name">补位辅助<\/span><b class="match-autofill-chip"/);
  // 标签必须落在 participant-link 按钮内部：该按钮在详情表里是 width:100% 的
  // flex 容器，放到按钮外面会被挤到第二行，破坏单元格布局。
  assert.match(html, /<button class="participant-link"[\s\S]*match-autofill-chip[\s\S]*<\/button><\/td>/);
  assert.doesNotMatch(html, /正常选位<\/span><b/, '正常选位的行不能带标签');
  assert.doesNotMatch(html, /换位中单<\/span><b/, '换位的行不能带标签');
  const withoutEvidence = context.matchTableRows([{ participantId: 1, gameName: '正常选位', position: 'top' }], scores);
  assert.equal(withoutEvidence.includes('补位'), false, '没有 autofill 字段时整行不该出现「补位」二字');
});

test('R119 补位标签复用现有 chip 尺寸与主题令牌，不写死颜色', () => {
  const css = read('gameplay.css');
  const rules = [...css.matchAll(/[^{}\n]*\.match-autofill-chip[^{}\n]*\{[^{}]*\}/g)].map((match) => match[0]);
  assert.equal(rules.length, 1, '只应有一条 .match-autofill-chip 规则');
  assert.match(rules[0], /var\(--warning\)/, '颜色必须走主题令牌，深色/浅色主题才都能读');
  assert.match(rules[0], /flex: none/, '作为 participant-link 的 flex 子项不能被压缩');
  assert.match(rules[0], /white-space: nowrap/, '「补位」两个字不能折行');
  assert.doesNotMatch(rules[0], /#[0-9a-fA-F]{3,8}/, '不允许硬编码十六进制颜色');
});
