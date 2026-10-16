"use strict";

function proxyURLFromResolution(value) {
  for (const raw of String(value || "").split(";")) {
    const directive = raw.trim();
    if (/^DIRECT$/i.test(directive)) return "";
    const match = /^(PROXY|HTTPS|SOCKS5|SOCKS)\s+([^\s]+)$/i.exec(directive);
    if (!match) continue;
    const scheme = match[1].toUpperCase() === "HTTPS" ? "https" : match[1].toUpperCase().startsWith("SOCKS") ? "socks5" : "http";
    try {
      const parsed = new URL(`${scheme}://${match[2]}`);
      if (!parsed.hostname || parsed.username || parsed.password || (parsed.pathname && parsed.pathname !== "/")) continue;
      return parsed.toString().replace(/\/$/, "");
    } catch (_) {}
  }
  return "";
}

async function resolveSystemProxy(defaultSession, target, timeoutMs = 2500, fallback = "") {
  if (!defaultSession?.resolveProxy) return fallback;
  let timer;
  try {
    const result = await Promise.race([
      defaultSession.resolveProxy(target),
      new Promise((resolve) => { timer = setTimeout(() => resolve(null), timeoutMs); }),
    ]);
    return result === null ? fallback : proxyURLFromResolution(result);
  } catch (_) {
    return fallback;
  } finally {
    clearTimeout(timer);
  }
}

module.exports = { proxyURLFromResolution, resolveSystemProxy };
