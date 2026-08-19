"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { proxyURLFromResolution, resolveSystemProxy } = require("./proxy-resolution.cjs");

test("parses Chromium proxy resolution without accepting credentials", () => {
  assert.equal(proxyURLFromResolution("PROXY 127.0.0.1:7890; DIRECT"), "http://127.0.0.1:7890");
  assert.equal(proxyURLFromResolution("SOCKS5 localhost:1080"), "socks5://localhost:1080");
  assert.equal(proxyURLFromResolution("DIRECT"), "");
  assert.equal(proxyURLFromResolution("PROXY user:secret@example.com:8080"), "");
});

test("system proxy resolution times out or falls back to direct safely", async () => {
  assert.equal(await resolveSystemProxy({ resolveProxy: async () => "HTTPS proxy.example:8443" }, "https://example.com"), "https://proxy.example:8443");
  assert.equal(await resolveSystemProxy({ resolveProxy: async () => { throw new Error("PAC failed"); } }, "https://example.com"), "");
  assert.equal(await resolveSystemProxy({ resolveProxy: () => new Promise(() => {}) }, "https://example.com", 5), "");
});
