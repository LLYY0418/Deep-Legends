"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const source = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const styles = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");

function extract(name) {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === "async ") start -= 6;
  const tail = source.slice(start), end = tail.indexOf("\n  }\n");
  assert.ok(end >= 0, name);
  return tail.slice(0, end + tail.slice(end).indexOf("}") + 1);
}

function compile(names, dependencies) {
  return Function(...Object.keys(dependencies), `${names.map(extract).join("\n")}\nreturn {${names.join(",")}};`)(...Object.values(dependencies));
}

const escapeHTML = value => String(value ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll('"', "&quot;");

test("R114 rank card popover presents only the supported Mayhem estimate", () => {
  const { renderRankMMRPopover } = compile(["mayhemRatingStatus", "renderRankMMRPopover"], {
    riotTab: tab => tab.region === "kr", number: String, escapeHTML,
  });
  const tab = { region: "cn", playerRef: "opaque-player-ref", mayhemRating: { status: "ready", playerRef: "opaque-player-ref", data: {
    available: true, rating: 2176, ratingType: 1, marginOfError: 42, matches: [{ championId: 99 }],
  } } };
  const html = renderRankMMRPopover(tab);
  assert.match(html, /is-mayhem[^]*海克斯大乱斗[^]*2176[^]*±42/);
  assert.match(html, /± 表示估算误差范围，数值越小代表结果越稳定/);
  assert.doesNotMatch(html, /<em>推算<\/em>|<em>估算<\/em>|数值来自|查看 ARAMKit|aramkit\.com/);
  assert.doesNotMatch(html, /is-solo|is-flex|单双排|灵活组排/);
  assert.doesNotMatch(html, /data-query-mayhem-rating|敏感名称|至少 10/);
  assert.match(styles, /\.is-mayhem/);
  assert.doesNotMatch(styles, /\.is-solo|\.is-flex/);
});

test("R114 overview automatically queries once with an opaque reference", async () => {
  const calls = [], renders = [];
  const helpers = compile(["mayhemRatingStatus", "loadMayhemRating"], {
    riotTab: tab => tab.region === "kr", URLSearchParams,
    rerenderTab: tab => renders.push(tab.key),
    api: async (url, options, key) => { calls.push({ url, options, key }); return { available: true, rating: 2000, ratingType: 0 }; },
  });
  const tab = { key: "cn-player", region: "cn", data: { player: { playerRef: "opaque-reference-only" } }, mayhemRating: { status: "idle", playerRef: "", data: null, error: "" } };
  await helpers.loadMayhemRating(tab);
  assert.equal(calls.length, 1);
  assert.match(calls[0].url, /^\/api\/gameplay\/mayhem-rating\?playerRef=opaque-reference-only$/);
  assert.doesNotMatch(JSON.stringify(calls), /gameName|tagLine|昵称/);
  assert.equal(tab.mayhemRating.data.rating, 2000);
  assert.deepEqual(renders, ["cn-player", "cn-player"]);
  await helpers.loadMayhemRating(tab);
  assert.equal(calls.length, 1, "ready result must not be queried again");

  assert.match(source, /if \(tab\.data && !force\)[^]*void loadMayhemRating\(tab\)/);
  assert.match(source, /renderCapabilitySettings\(\);[^]*void loadMayhemRating\(tab, true\)/);
});

test("R114 Korean overview never calls the CN-only ARAMKit endpoint", async () => {
  const helpers = compile(["mayhemRatingStatus", "loadMayhemRating"], {
    riotTab: tab => tab.region === "kr", URLSearchParams, rerenderTab: () => {},
    api: async () => assert.fail("KR must not query ARAMKit"),
  });
  const tab = { key: "kr", region: "kr", data: { player: { playerRef: "opaque-kr-reference" } } };
  await helpers.loadMayhemRating(tab);
  assert.equal(tab.mayhemRating, undefined);
});
