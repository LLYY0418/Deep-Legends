const HOSTS = { asia: "asia.api.riotgames.com", kr: "kr.api.riotgames.com" };
const MAX_BODY = 16 * 1024 * 1024;

export function allowedRoute(url) {
  const parts = url.pathname.split("/");
  if (parts[1] !== "r" || !Object.hasOwn(HOSTS, parts[2])) return null;
  let segments;
  try { segments = parts.slice(3).map(decodeURIComponent); } catch { return null; }
  if (segments.some(s => !s || s.length > 256 || /[\\/\u0000-\u001f?#%]/.test(s) || s === "." || s === "..")) return null;
  const path = "/" + segments.join("/");
  const rules = parts[2] === "asia" ? [
    [/^\/riot\/account\/v1\/accounts\/by-riot-id\/[^/]+\/[^/]+$/, "account", 3600],
    [/^\/riot\/account\/v1\/accounts\/by-puuid\/[^/]+$/, "account", 3600],
    [/^\/lol\/match\/v5\/matches\/by-puuid\/[^/]+\/ids$/, "match-ids", 60],
    [/^\/lol\/match\/v5\/matches\/[A-Z0-9]+_\d+$/, "match", 604800],
    [/^\/lol\/match\/v5\/matches\/[A-Z0-9]+_\d+\/timeline$/, "timeline", 604800],
  ] : [
    [/^\/lol\/summoner\/v4\/summoners\/by-puuid\/[^/]+$/, "summoner", 60],
    [/^\/lol\/league\/v4\/entries\/by-puuid\/[^/]+$/, "rank", 60],
    [/^\/lol\/champion-mastery\/v4\/champion-masteries\/by-puuid\/[^/]+\/top$/, "mastery", 60],
    [/^\/lol\/status\/v4\/platform-data$/, "status", 60],
  ];
  const rule = rules.find(([pattern]) => pattern.test(path));
  if (!rule) return null;
  return { host: HOSTS[parts[2]], path: "/" + parts.slice(3).join("/"), category: rule[1], ttl: rule[2] };
}

function retrySeconds(value, now) {
  if (/^\d+$/.test(value || "")) return Math.max(1, Math.min(86400, Number(value)));
  const stamp = Date.parse(value || "");
  return Number.isFinite(stamp) ? Math.max(1, Math.min(86400, Math.ceil((stamp - now) / 1000))) : 3;
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
      const route = request.method === "GET" && allowedRoute(url);
      if (!route) return new Response("Not found", { status: 404 });
      const finish = (response, hit = false) => {
        log({ category: route.category, status: response.status, cache_hit: hit });
        return response;
      };
      if (!env.RIOT_API_KEY || !env.IP_LIMITER) return finish(new Response("Service unavailable", { status: 503 }));
      let permitted;
      try { permitted = await env.IP_LIMITER.limit({ key: request.headers.get("CF-Connecting-IP") || "unknown" }); }
      catch { return finish(new Response("Service unavailable", { status: 503 })); }
      if (!permitted.success) return finish(new Response("Too many requests", { status: 429, headers: { "Retry-After": "60" } }));
      // Only expiring HTTP response caches are used. Cache keys contain neither
      // Riot IDs nor IPs; no KV/D1/R2 or application identity/request logs.
      const cache = options.cache || caches.default;
      const key = new Request(url.origin + "/_cache/" + await digest(route.host + route.path + url.search));
      const cooldownKey = new Request(url.origin + "/_cooldown/" + await digest(route.host + route.path));
      const cooldown = await cache.match(cooldownKey);
      if (cooldown) {
        return finish(new Response("Too many requests", { status: 429, headers: { "Retry-After": cooldown.headers.get("Retry-After") || "3" } }), true);
      }
      const cached = await cache.match(key);
      if (cached) return finish(cached, true);
      let upstream;
      try {
        upstream = await fetchUpstream("https://" + route.host + route.path + url.search, {
          method: "GET", redirect: "error", signal: AbortSignal.timeout(15000),
          headers: { "X-Riot-Token": env.RIOT_API_KEY, Accept: "application/json" },
        });
      } catch { return finish(new Response("Upstream unavailable", { status: 502 })); }
      if (upstream.status === 429) {
        await upstream.body?.cancel();
        const rawRetry = upstream.headers.get("Retry-After") || "3";
        const retry = rawRetry.includes(env.RIOT_API_KEY) ? "3" : rawRetry;
        const marker = new Response("rate-limit", { headers: { "Retry-After": retry, "Cache-Control": "public, max-age=" + retrySeconds(retry, now()) } });
        await cache.put(cooldownKey, marker);
        return finish(new Response("Too many requests", { status: 429, headers: { "Retry-After": retry } }));
      }
      let body;
      try { body = await boundedText(upstream.body); }
      catch { return finish(new Response("Upstream unavailable", { status: 502 })); }
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
