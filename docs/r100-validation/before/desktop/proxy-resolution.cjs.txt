"use strict";

function proxyURLFromResolution(value) {
  for (const raw of String(value || "").split(";")) {
    const directive = raw.trim();
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

async function resolveSystemProxy(defaultSession, target, timeoutMs = 2500) {
  if (!defaultSession?.resolveProxy) return "";
  let timer;
  try {
    const result = await Promise.race([
      defaultSession.resolveProxy(target),
      new Promise((resolve) => { timer = setTimeout(() => resolve("DIRECT"), timeoutMs); }),
    ]);
    return proxyURLFromResolution(result);
  } catch (_) {
    return "";
  } finally {
    clearTimeout(timer);
  }
}

module.exports = { proxyURLFromResolution, resolveSystemProxy };
