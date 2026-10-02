'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const source = fs.readFileSync(process.env.R189_APP_SOURCE || path.join(__dirname, 'app.js'), 'utf8');
const f = compile(source, ['lootToken', 'lootName', 'lootNamePending', 'lootTypeLabel', 'lootImagePaths', 'lootCategorySlug', 'lootCategoryIcon', 'lootCard'], {escapeHTML, formatNumber: String});
const item = {lootId: 'EMOTE_1468', displayName: '演示表情（测试夹具）', category: '表情', type: 'EMOTE', dataPending: false, count: 1, asset: '/lol-game-data/assets/ASSETS/Loadouts/SummonerEmotes/fixture.png'};
function card(data, verify) {
  const d = new JSDOM(f.lootCard(data));
  try { verify(d.window.document.querySelector('.loot-card'), d.window); }
  finally { d.window.close(); }
}
test('R189 directory emote name and icon render without a pending hint', () => {
  card(item, root => {
    assert.equal(root.querySelector('strong').textContent, item.displayName);
    assert.equal(root.querySelector('.loot-name-pending'), null);
    const image = root.querySelector('.loot-art img');
    assert.ok(image); assert.equal(decodeURIComponent(image.dataset.paths), item.asset);
  });
});
test('R189 fallback emote with dataPending false never implies an unsynced name', () => {
  card({...item, displayName: '表情 1468', asset: ''}, root => {
    assert.equal(root.querySelector('strong').textContent, '表情 1468');
    assert.equal(root.querySelector('.loot-art img'), null);
    assert.equal(root.querySelector('.loot-name-pending'), null);
    assert.doesNotMatch(root.textContent, /暂未同步/);
  });
  // Only dataPending governs the hint, including an older/raw-name response.
  // The old equality guard cannot trigger on the new category+ID name itself.
  card({...item, displayName: item.lootId, dataPending: false}, root => {
    assert.equal(root.querySelector('.loot-name-pending'), null);
    assert.equal(root.classList.contains('is-name-pending'), false);
  });
});
test('R189 blank loot retains its pending hint only during bounded retries', () => {
  card({blank: true, dataPending: true, kind: '类型未知', count: 74}, root => {
    assert.match(root.textContent, /客户端数据暂未同步，可稍后重试/);
    assert.equal(root.classList.contains('is-name-pending'), true);
  });
  card({blank: true, dataPending: false, kind: '类型未知', count: 74}, root => {
    assert.equal(root.querySelector('.loot-name-pending'), null);
    assert.match(root.textContent, /客户端返回的空白条目/);
  });
});
test('R189 emote ward and icon show known ownership, unknown stays absent', () => {
  for (const category of ['表情', '守卫', '图标']) for (const owned of [true, false]) {
    card({...item, category, ownedKnown: true, owned}, root => {
      const status = root.querySelector('.loot-ownership'); assert.ok(status);
      assert.equal(status.textContent, owned ? '已拥有' : '未拥有');
      assert.equal(status.classList.contains(owned ? 'is-owned' : 'is-missing'), true);
    });
    card({...item, category, ownedKnown: false, owned: true}, root => assert.equal(root.querySelector('.loot-ownership'), null));
  }
  const css = fs.readFileSync(path.join(__dirname, 'app.css'), 'utf8');
  assert.match(css, /\.loot-ownership\.is-missing\s*\{[^}]*color:\s*var\(--muted\)/);
});
test('R189 skin ownership and upgrade wording remain unchanged', () => {
  for (const owned of [true, false]) card({...item, category: '皮肤', skinOwnedKnown: true, skinOwned: owned, ownedKnown: true, owned: !owned}, root => {
    assert.equal(root.querySelector('.loot-ownership').textContent, owned ? '已拥有' : '可升级');
    assert.equal(root.querySelector('.loot-ownership').classList.contains(owned ? 'is-owned' : 'is-unowned'), true);
    assert.doesNotMatch(root.textContent, /未拥有/);
  });
});
