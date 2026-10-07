"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const { requireActiveLicense } = require("./license-gate.cjs");

test("R232 native IPC trusts Go state and rechecks generation after async work", async () => {
  let status = { state: "LOCKED", generation: 1 };
  let backend = { baseUrl: "http://127.0.0.1:1", token: "local-test" };
  const http = { request(url, options, callback) {
    assert.equal(url, backend.baseUrl + "/api/license/status");
    assert.equal(options.headers["X-Local-Token"], "local-test");
    const request = new EventEmitter();
    request.destroy = error => request.emit("error", error);
    request.end = () => queueMicrotask(() => {
      const response = new EventEmitter(); response.statusCode = 200; response.setEncoding = () => {};
      callback(response); response.emit("data", JSON.stringify(status)); response.emit("end");
    });
    return request;
  } };
  await assert.rejects(requireActiveLicense(http, () => backend, true), /尚未激活/);
  status = { state: "ACTIVE", generation: 2 };
  const recheck = await requireActiveLicense(http, () => backend, true); await recheck();
  status = { state: "ACTIVE", generation: 4 };
  await assert.rejects(recheck(), /尚未激活/, "a later activation cannot revive admitted IPC work");
  const next = await requireActiveLicense(http, () => backend, true);
  backend = { ...backend };
  await assert.rejects(next(), /尚未激活/, "a restarted backend invalidates the ticket");
  await assert.rejects(requireActiveLicense(http, () => null, true), /尚未激活/);
});
