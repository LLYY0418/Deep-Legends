const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const source = fs.readFileSync(path.join(__dirname, "suite.js"), "utf8");

function functionSource(script, name) {
  const marker = `function ${name}(`;
  const start = script.indexOf(marker);
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = script.indexOf("{", script.indexOf(")", start));
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < script.length; index += 1) {
    const char = script[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return script.slice(start, index + 1);
  }
  assert.fail(`unbalanced ${name}`);
}

function compile(names, dependencies = {}) {
  const keys = Object.keys(dependencies);
  return Function(...keys, `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}

test("R126 career draft uses explicit backdrop champion before skin-id inference", () => {
  const state = {
    facade: {
      profile: { backgroundSkinId: 0, backgroundChampionId: 64 },
      skins: [{ id: 1000, championId: 1, championName: "黑暗之女" }],
    },
  };
  const { hydrateFacadeDraft } = compile(["hydrateFacadeDraft"], { state });
  hydrateFacadeDraft();
  assert.equal(state.facadeDraft.hero, "64");
  assert.equal(state.facadeDraft.skinId, 0);
});

test("R126 career draft stays empty when no background champion evidence exists", () => {
  const state = {
    facade: {
      profile: { backgroundSkinId: 0, backgroundChampionId: 0 },
      skins: [{ id: 1000, championId: 1, championName: "黑暗之女" }],
    },
  };
  const { hydrateFacadeDraft } = compile(["hydrateFacadeDraft"], { state });
  hydrateFacadeDraft();
  assert.equal(state.facadeDraft.hero, "");
  assert.equal(state.facadeDraft.skinId, 0);
  assert.match(source, /champions\.unshift\(\["", "请选择英雄"\]\)/);
});

test("R126 facade refresh signature includes the resolved background champion", () => {
  const { facadeRenderSignature } = compile(["facadeRenderSignature"]);
  const lee = facadeRenderSignature({ profile: { backgroundSkinId: 0, backgroundChampionId: 64 } });
  const annie = facadeRenderSignature({ profile: { backgroundSkinId: 0, backgroundChampionId: 1 } });
  assert.notDeepEqual(lee, annie);
});
