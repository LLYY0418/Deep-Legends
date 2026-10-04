import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { webcrypto } from "node:crypto";
import { createRiotWorker } from "./worker.mjs";

function fixture(upstream = async () => new Response('{"ok":true}')) {
  let stamp = 100000;
  const entries = new Map(), counts = new Map(), requests = [], logs = [];
  const cache = {
    async match(key) { const e = entries.get(key.url); return e && e.until > stamp ? e.response.clone() : undefined; },
    async put(key, response) {
      const ttl = Number(response.headers.get("Cache-Control").match(/max-age=(\d+)/)?.[1] || 0);
      entries.set(key.url, { until: stamp + ttl * 1000, response: response.clone() });
    },
  };
  const env = { RIOT_API_KEY: "fixture-secret-not-a-real-key", IP_LIMITER: {
    async limit({ key }) { const slot = key + ":" + Math.floor(stamp / 60000); const n = (counts.get(slot) || 0) + 1; counts.set(slot, n); return { success: n <= 300 }; },
  } };
  const worker = createRiotWorker({ cache, now: () => stamp, crypto: webcrypto, log: e => logs.push(e), fetch: async (...args) => { requests.push(args); return upstream(...args); } });
  return { env, requests, entries, logs, advance(ms) { stamp += ms; }, get(path, method = "GET", ip = "192.0.2.1") {
    return worker.fetch(new Request("https://relay.example" + path, { method, headers: { "CF-Connecting-IP": ip, Cookie: "not-forwarded", "X-Riot-Token": "client-token-not-forwarded" } }), env);
  } };
}

test("R206 Worker forwards only the actual Riot GET route whitelist", async () => {
  const paths = [
    ["asia", "/riot/account/v1/accounts/by-riot-id/name%20with%20space/tag"],
    ["asia", "/riot/account/v1/accounts/by-puuid/fixture-puuid"],
    ["asia", "/lol/match/v5/matches/by-puuid/fixture-puuid/ids?count=10&start=0"],
    ["asia", "/lol/match/v5/matches/KR_123"], ["asia", "/lol/match/v5/matches/KR_123/timeline"],
    ["kr", "/lol/summoner/v4/summoners/by-puuid/fixture-puuid"], ["kr", "/lol/league/v4/entries/by-puuid/fixture-puuid"],
    ["kr", "/lol/champion-mastery/v4/champion-masteries/by-puuid/fixture-puuid/top?count=5"], ["kr", "/lol/status/v4/platform-data"],
  ];
  const f = fixture();
  for (const [region, path] of paths) {
    assert.equal((await f.get("/r/" + region + path)).status, 200);
    const [url, options] = f.requests.at(-1);
    assert.equal(url, "https://" + region + ".api.riotgames.com" + path);
    assert.deepEqual(options.headers, { "X-Riot-Token": f.env.RIOT_API_KEY, Accept: "application/json" });
    assert.equal(options.redirect, "manual");
  }
});

test("R206 Worker rejects arbitrary paths/hosts/methods without Riot I/O", async () => {
  const f = fixture();
  for (const path of ["/r/evil/lol/status/v4/platform-data", "/r/kr/https://evil.example", "/r/kr/lol/unknown", "/r/kr/riot/account/v1/accounts/by-puuid/foo", "/r/asia/lol/status/v4/platform-data", "/r/asia/lol/match/v5/matches/KR_1/extra", "/r/asia/riot/account/v1/accounts/by-puuid/a%2Fb", "/r/asia/riot/account/v1/accounts/by-puuid/a%252Fb"]) {
    assert.equal((await f.get(path)).status, 404, path);
  }
  assert.equal((await f.get("/r/kr/lol/status/v4/platform-data", "POST")).status, 404);
  assert.equal(f.requests.length, 0);
});

test("R206 Worker response caching honors category TTL and contains no identity cache keys", async () => {
  const f = fixture();
  const path = "/r/asia/lol/match/v5/matches/KR_123";
  await f.get(path); await f.get(path);
  assert.equal(f.requests.length, 1);
  assert.equal(f.logs.at(-1).cache_hit, true);
  f.advance(604800000); await f.get(path);
  assert.equal(f.requests.length, 2);
  for (const key of f.entries.keys()) assert.ok(!key.includes("KR_123"));
  await f.get("/r/asia/riot/account/v1/accounts/by-riot-id/private-name/tag");
  for (const key of f.entries.keys()) assert.ok(!key.includes("private-name"));
  for (const log of f.logs) assert.deepEqual(Object.keys(log).sort(), ["cache_hit", "category", "status"]);
});

test("R206 Worker per-IP limit rejects request 301 and allows another IP", async () => {
  const config = readFileSync(new URL("./wrangler.toml", import.meta.url), "utf8");
  assert.match(config, /limit = 300\nperiod = 60/);
  const f = fixture();
  for (let i = 0; i < 300; i++) assert.equal((await f.get("/r/kr/lol/status/v4/platform-data")).status, 200);
  const denied = await f.get("/r/kr/lol/status/v4/platform-data");
  assert.equal(denied.status, 429); assert.equal(denied.headers.get("Retry-After"), "60");
  assert.equal((await f.get("/r/kr/lol/status/v4/platform-data", "GET", "192.0.2.2")).status, 200);
});

test("R206 Worker preserves Riot Retry-After and avoids same-path I/O during cooldown", async () => {
  const f = fixture(async () => new Response("do not expose upstream", { status: 429, headers: { "Retry-After": "7" } }));
  const path = "/r/asia/lol/match/v5/matches/by-puuid/fixture/ids";
  for (const suffix of ["?count=10", "?count=20"]) {
    const r = await f.get(path + suffix); assert.equal(r.status, 429); assert.equal(r.headers.get("Retry-After"), "7");
  }
  assert.equal(f.requests.length, 1);
  f.advance(7000); await f.get(path + "?count=30"); assert.equal(f.requests.length, 2);
});

test("R206 Worker never exposes or logs its secret, including echoed upstream values", async () => {
  const secret = "fixture-secret-not-a-real-key";
  const f = fixture(async () => new Response(JSON.stringify({ echo: secret }), { headers: { "X-Riot-Token": secret, "X-App-Rate-Limit": secret } }));
  const r = await f.get("/r/kr/lol/status/v4/platform-data");
  assert.ok(!(await r.text()).includes(secret)); assert.ok(!JSON.stringify([...r.headers]).includes(secret));
  assert.ok(!JSON.stringify(f.logs).includes(secret));
  const denied = fixture(async () => new Response(secret, { status: 429, headers: { "Retry-After": secret } }));
  const d = await denied.get("/r/kr/lol/status/v4/platform-data");
  assert.ok(!(await d.text()).includes(secret)); assert.equal(d.headers.get("Retry-After"), "5");
});

test("R206 Worker fails closed without a secret or rate limiter", async () => {
  for (const key of ["RIOT_API_KEY", "IP_LIMITER"]) {
    const f = fixture(); delete f.env[key];
    assert.equal((await f.get("/r/kr/lol/status/v4/platform-data")).status, 503);
    assert.equal(f.requests.length, 0);
  }
});

test("R206 Worker rejects upstream redirects without forwarding the secret or Location", async () => {
  for (const status of [301, 302, 303, 307, 308]) {
    const f = fixture(async (_url, options) => {
      // Cloudflare's runtime rejects redirect:error before making a request.
      assert.equal(options.redirect, "manual");
      return new Response("redirect", { status, headers: { Location: "https://other.example" } });
    });
    const response = await f.get("/r/kr/lol/status/v4/platform-data");
    assert.equal(response.status, 502);
    assert.equal(response.headers.get("Location"), null);
    assert.equal(f.requests.length, 1);
    assert.equal(f.entries.size, 0);
  }
});

test("R208 application cooldown blocks every path on its host, with decreasing remaining time", async () => {
  let limited = true;
  const a = "/r/asia/riot/account/v1/accounts/by-puuid/a";
  const b = "/r/asia/riot/account/v1/accounts/by-puuid/b";
  const f = fixture(async url => limited && url.endsWith("/a")
    ? new Response("limited", { status: 429, headers: { "Retry-After": "7", "X-Rate-Limit-Type": "application" } })
    : new Response('{"ok":true}'));
  await f.get(b); // Even an already cached path must honor global cooldown.
  const first = await f.get(a);
  assert.equal(first.headers.get("X-Relay-Cooldown"), "application");
  f.advance(2000);
  const blocked = await f.get(b);
  assert.equal(blocked.status, 429);
  assert.equal(blocked.headers.get("Retry-After"), "5");
  assert.equal(f.requests.length, 2);
  assert.equal((await f.get("/r/kr/lol/status/v4/platform-data")).status, 200);
  assert.equal(f.requests.length, 3);
  assert.deepEqual(Object.keys(f.logs.find(x => x.limit_type)).sort(), ["cache_hit", "category", "limit_type", "status"]);
  limited = false; f.advance(5000);
  assert.equal((await f.get(a)).status, 200);
  assert.equal(f.requests.length, 4);
});

test("R208 method/service/missing-header cooldowns remain path scoped and default to five seconds", async () => {
  for (const type of ["method", "service", null]) {
    let limited = true;
    const f = fixture(async url => limited && url.endsWith("/a")
      ? new Response("limited", { status: 429, headers: type ? { "X-Rate-Limit-Type": type } : {} })
      : new Response('{"ok":true}'));
    const path = "/r/asia/riot/account/v1/accounts/by-puuid/";
    const first = await f.get(path + "a");
    assert.equal(first.headers.get("X-Relay-Cooldown"), type || "service");
    assert.equal(first.headers.get("Retry-After"), "5");
    assert.equal((await f.get(path + "b")).status, 200);
    f.advance(2000);
    const blocked = await f.get(path + "a");
    assert.equal(blocked.status, 429); assert.equal(blocked.headers.get("Retry-After"), "3");
    assert.equal(f.requests.length, 2);
    limited = false; f.advance(3000);
    assert.equal((await f.get(path + "a")).status, 200);
    assert.equal(f.requests.length, 3);
  }
});

test("R208 IP 429 and controlled upstream failures are JSON rather than false daily-quota pages", async () => {
  const f = fixture();
  for (let i = 0; i < 300; i++) await f.get("/r/kr/lol/status/v4/platform-data");
  const ip = await f.get("/r/kr/lol/status/v4/platform-data");
  assert.equal(ip.status, 429);
  assert.equal(ip.headers.get("X-Relay-Cooldown"), null);
  assert.equal(ip.headers.get("Retry-After"), "60");
  assert.match(ip.headers.get("Content-Type"), /^application\/json/);
  assert.deepEqual(await ip.json(), { error: "Too many requests" });
  const broken = fixture(async () => { throw new Error("fixture network failure"); });
  const response = await broken.get("/r/kr/lol/status/v4/platform-data");
  assert.equal(response.status, 502); assert.match(response.headers.get("Content-Type"), /^application\/json/);
});

test("R208 application remaining time takes precedence when IP limit also expires", async () => {
  const f = fixture(async () => new Response("limited", {status: 429, headers: {"X-Rate-Limit-Type": "application", "Retry-After": "7"}}));
  await f.get("/r/asia/riot/account/v1/accounts/by-puuid/a");
  f.advance(2000);
  for (let i = 0; i < 300; i++) {
    const r = await f.get("/r/asia/riot/account/v1/accounts/by-puuid/b");
    assert.equal(r.status, 429);
    assert.equal(r.headers.get("X-Relay-Cooldown"), "application");
    assert.equal(r.headers.get("Retry-After"), "5");
  }
  assert.equal(f.requests.length, 1);
  const kr = await f.get("/r/kr/lol/status/v4/platform-data");
  assert.equal(kr.headers.get("X-Relay-Cooldown"), null);
  assert.equal(kr.headers.get("Retry-After"), "60");
});
