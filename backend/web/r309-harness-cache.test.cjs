'use strict';
const test = require('node:test'), assert = require('node:assert/strict');
const {extract, compile} = require('./r188-harness.cjs');
const {expand} = require('./r211-harness-support.cjs');

test('R309 cached source extraction keeps mutants distinct and compiled state independent', () => {
  const source = 'function preference() { state.count++; return state.count + 10; }';
  const first = compile(source, ['preference'], {state: {count: 0}});
  const second = compile(source, ['preference'], {state: {count: 0}});
  assert.equal(first.preference(), 11);
  assert.equal(first.preference(), 12);
  assert.equal(second.preference(), 11);
  const mutant = source.replace('+ 10', '+ 20');
  assert.equal(compile(mutant, ['preference'], {state: {count: 0}}).preference(), 21);
  assert.match(extract(source, 'preference'), /\+ 10/);
  assert.match(extract(mutant, 'preference'), /\+ 20/);
  assert.throws(() => extract(source.replace('preference()', 'other()'), 'preference'), /Missing function preference/);
});

test('R309 cached dependency expansion respects injected helpers and returns independent arrays', () => {
  const source = 'function readSetting() { return 10; }\nfunction preference() { return readSetting(); }';
  const names = ['preference'];
  const expanded = expand(source, names);
  assert.deepEqual(expanded, ['preference', 'readSetting']);
  expanded.push('mustNotLeak');
  assert.deepEqual(expand(source, names), ['preference', 'readSetting']);
  assert.deepEqual(expand(source, names, {readSetting: () => 20}), ['preference']);
  assert.equal(compile(source, names, {readSetting: () => 20}).preference(), 20);
  assert.equal(compile(source, names).preference(), 10);
  assert.deepEqual(names, ['preference']);
});
