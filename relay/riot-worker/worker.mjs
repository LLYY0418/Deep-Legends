const CLUSTERS = new Set(["americas", "asia", "europe", "sea"]);
const PLATFORMS = ["br1", "eun1", "euw1", "jp1", "kr", "la1", "la2", "me1", "na1", "oc1", "ru", "sg2", "tr1", "tw2", "vn2"];
const HOSTS = Object.fromEntries([...CLUSTERS, ...PLATFORMS].map(region => [region, `${region}.api.riotgames.com`]));
const MAX_BODY = 16 * 1024 * 1024;

export function allowedRoute(url) {
  const parts = url.pathname.split("/");
  if (parts[1] !== "r" || !Object.hasOwn(HOSTS, parts[2])) return null;
  let segments;
  try { segments = parts.slice(3).map(decodeURIComponent); } catch { return null; }
  if (segments.some(s => !s || s.length > 256 || /[\\/\u0000-\u001f?#%]/.test(s) || s === "." || s === "..")) return null;
  const path = "/" + segments.join("/");
  const rules = CLUSTERS.has(parts[2]) ? [
    [/^\/riot\/account\/v1\/accounts\/by-riot-id\/[^/]+\/[^/]+$/, "account", 3600],
    [/^\/riot\/account\/v1\/accounts\/by-puuid\/[^/]+$/, "account", 3600],
    [/^\/lol\/match\/v5\/matches\/by-puuid\/[^/]+\/ids$/, "match-ids", 60],
    [/^\/lol\/match\/v5\/matches\/[A-Z0-9]+_\d+$/, "match", 604800],
    [/^\/lol\/match\/v5\/matches\/[A-Z0-9]+_\d+\/timeline$/, "timeline", 604800],
  ] : [
    [/^\/lol\/summoner\/v4\/summoners\/by-puuid\/[^/]+$/, "summoner", 60],
    [/^\/lol\/league\/v4\/entries\/by-puuid\/[^/]+$/, "rank", 60],
    [/^\/lol\/champion-mastery\/v4\/champion-masteries\/by-puuid\/[^/]+\/top$/, "mastery", 60],
    [/^\/lol\/champion-mastery\/v4\/champion-masteries\/by-puuid\/[^/]+$/, "mastery", 60],
    [/^\/lol\/champion-mastery\/v4\/scores\/by-puuid\/[^/]+$/, "mastery", 60],
    [/^\/lol\/spectator\/v5\/active-games\/by-summoner\/[^/]+$/, "spectator", 30],
    [/^\/lol\/status\/v4\/platform-data$/, "status", 60],
  ];
  if (parts[2] === "sea" && path.startsWith("/riot/account/")) return null;
  const rule = rules.find(([pattern]) => pattern.test(path));
  if (!rule) return null;
  return { host: HOSTS[parts[2]], path: "/" + parts.slice(3).join("/"), category: rule[1], ttl: rule[2] };
}

function retrySeconds(value, now) {
  if (/^\d+$/.test(value || "")) return Math.max(1, Math.min(86400, Number(value)));
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? Math.max(1, Math.min(86400, Math.ceil((stamp - now) / 1000))) : 5;
}

function jsonError(message, status, headers = {}) {
  return new Response(JSON.stringify({ error: message }), {
    status, headers: { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store", ...headers },
  });
}

async function boundedText(body) {
  if (!body) return "";
  const reader = body.getReader();
  const chunks = [];
  let size = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_BODY) throw new Error("oversized upstream");
      chunks.push(value);
    }
  } catch (error) { await reader.cancel().catch(() => {}); throw error; }
  finally { reader.releaseLock(); }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
  return new TextDecoder().decode(bytes);
}

export function createRiotWorker(options = {}) {
  const fetchUpstream = options.fetch || ((...args) => fetch(...args));
  const now = options.now || Date.now;
  const log = options.log || (entry => console.log(JSON.stringify(entry)));
  const digest = async value => {
    const bytes = await (options.crypto || crypto).subtle.digest("SHA-256", new TextEncoder().encode(value));
    return [...new Uint8Array(bytes)].map(x => x.toString(16).padStart(2, "0")).join("");
  };
  return {
    async fetch(request, env) {
      const url = new URL(request.url);
      if (request.method === "GET" && url.pathname === "/health") return new Response(null,{status:204,headers:{"Cache-Control":"no-store"}});
      const route = request.method === "GET" && allowedRoute(url);
      if (!route) return new Response("Not found", { status: 404 });
      const finish = (response, hit = false, limitType = "") => {
        const entry = { category: route.category, status: response.status, cache_hit: hit };
        if (limitType) entry.limit_type = limitType;
        log(entry);
        return response;
      };
      if (!env.RIOT_API_KEY || !env.IP_LIMITER) return finish(jsonError("Service unavailable", 503));
      let permitted;
      try { permitted = await env.IP_LIMITER.limit({ key: request.headers.get("CF-Connecting-IP") || "unknown" }); }
      catch { return finish(jsonError("Service unavailable", 503)); }
      // Only expiring HTTP response caches are used. Cache keys contain neither
      // Riot IDs nor IPs; no KV/D1/R2 or application identity/request logs.
      const cache = options.cache || caches.default;
      // The caller's validated platform isolates cooldowns even when JP and KR
      // share ASIA. It is an enum, never an account identifier or credential.
      const platform = request.headers.get("X-Riot-Platform") || "";
      if (platform && !PLATFORMS.includes(platform)) return finish(jsonError("Invalid platform", 400));
      const routeRegion = url.pathname.split("/")[2];
      const cluster = ["na1", "br1", "la1", "la2"].includes(platform) ? "americas"
        : ["kr", "jp1"].includes(platform) ? "asia"
        : ["oc1", "sg2", "tw2", "vn2"].includes(platform) ? "sea" : "europe";
      const accountCluster = cluster === "sea" ? "asia" : cluster;
      const expectedRegion = CLUSTERS.has(routeRegion) ? (route.path.startsWith("/riot/account/") ? accountCluster : cluster) : platform;
      if (platform && routeRegion !== expectedRegion) return finish(jsonError("Invalid platform route", 400));
      const rateIdentity = route.host + "|" + (platform || routeRegion);
      const key = new Request(url.origin + "/_cache/" + await digest(rateIdentity + route.path + url.search));
      const cooldownKey = new Request(url.origin + "/_cooldown/" + await digest(rateIdentity + route.path));
      const applicationKey = new Request(url.origin + "/_application_cooldown/" + await digest(rateIdentity));
      // Count every request against the IP binding, while preserving the
      // application's host-wide Retry-After even when both limits apply.
      for (const markerKey of permitted.success ? [applicationKey, cooldownKey] : [applicationKey]) {
        const cooldown = await cache.match(markerKey);
        if (!cooldown) continue;
        const remaining = Math.ceil((Number(cooldown.headers.get("X-Relay-Cooldown-Until")) - now()) / 1000);
        // Expiring HTTP cache entries may survive briefly; never extend a
        // cooldown by returning its original Retry-After on every hit.
        if (!Number.isFinite(remaining) || remaining <= 0) continue;
        const type = cooldown.headers.get("X-Relay-Cooldown") || "service";
        return finish(jsonError("Too many requests", 429, {
          "Retry-After": String(remaining), "X-Relay-Cooldown": type,
        }), true, type);
      }
      if (!permitted.success) return finish(jsonError("Too many requests", 429, { "Retry-After": "60" }), false, "ip");
      const cached = await cache.match(key);
      if (cached) return finish(cached, true);
      let upstream;
      try {
        upstream = await fetchUpstream("https://" + route.host + route.path + url.search, {
          // workerd supports follow/manual, not redirect:"error". Reject
          // redirects ourselves so the Riot secret never reaches another host.
          method: "GET", redirect: "manual", signal: AbortSignal.timeout(15000),
          headers: { "X-Riot-Token": env.RIOT_API_KEY, Accept: "application/json" },
        });
      } catch { return finish(jsonError("Upstream unavailable", 502)); }
      if (upstream.status >= 300 && upstream.status < 400) {
        await upstream.body?.cancel();
        return finish(jsonError("Upstream unavailable", 502));
      }
      if (upstream.status === 429) {
        await upstream.body?.cancel();
        const rawType = upstream.headers.get("X-Rate-Limit-Type");
        const type = ["application", "method", "service"].includes(rawType) ? rawType : "service";
        const rawRetry = upstream.headers.get("Retry-After") || "5";
        const retry = rawRetry.includes(env.RIOT_API_KEY) ? "5" : rawRetry;
        const seconds = retrySeconds(retry, now());
        const marker = new Response("rate-limit", { headers: {
          "X-Relay-Cooldown": type, "X-Relay-Cooldown-Until": String(now() + seconds * 1000),
          "Cache-Control": "public, max-age=" + seconds,
        } });
        await cache.put(type === "application" ? applicationKey : cooldownKey, marker);
        return finish(jsonError("Too many requests", 429, {
          "Retry-After": String(seconds), "X-Relay-Cooldown": type,
        }), false, type);
      }
      let body;
      try { body = await boundedText(upstream.body); }
      catch { return finish(jsonError("Upstream unavailable", 502)); }
      // Do not expose an echoed secret or arbitrary upstream headers.
      body = body.split(env.RIOT_API_KEY).join("[redacted]");
      const headers = new Headers({ "Content-Type": "application/json; charset=utf-8" });
      for (const name of ["X-App-Rate-Limit", "X-App-Rate-Limit-Count", "X-Method-Rate-Limit", "X-Method-Rate-Limit-Count", "X-Rate-Limit-Type"]) {
        const value = upstream.headers.get(name);
        if (value && !value.includes(env.RIOT_API_KEY)) headers.set(name, value);
      }
      if (upstream.status === 200) headers.set("Cache-Control", "public, max-age=" + route.ttl);
      else headers.set("Cache-Control", "no-store");
      const response = new Response([204, 205, 304].includes(upstream.status) ? null : body, { status: upstream.status, headers });
      if (response.status === 200) await cache.put(key, response.clone());
      return finish(response);
    },
  };
}

export default createRiotWorker();
