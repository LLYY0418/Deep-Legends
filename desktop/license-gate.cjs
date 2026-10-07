"use strict";

// The renderer cannot grant permission to native IPC. Read the authenticated
// Go state, and revalidate its generation after asynchronous rendering/dialogs.
async function readLicenseSnapshot(http, getBackend, enabled = false) {
  if (!enabled) return { state: "DISABLED" };
  const backend = getBackend();
  const denied = () => new Error("软件尚未激活");
  if (!backend?.baseUrl || !backend?.token) throw denied();
  return new Promise((resolve, reject) => {
    if (getBackend() !== backend) { reject(denied()); return; }
    const request = http.request(`${backend.baseUrl}/api/license/status`, {
      method: "GET", headers: { "X-Local-Token": backend.token }, timeout: 3000,
    }, response => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", chunk => { body += chunk; if (body.length > 2048) request.destroy(denied()); });
      response.once("error", reject);
      response.once("end", () => {
        try {
          const value = JSON.parse(body);
          if (response.statusCode !== 200 || !["ACTIVE", "LOCKED", "NETWORK_LOCKED", "REPLACED", "REVOKED", "DEVICE_ERROR"].includes(value.state) || !Number.isSafeInteger(value.generation) || getBackend() !== backend) throw denied();
          resolve(value);
        } catch (_) { reject(denied()); }
      });
    });
    request.once("error", () => reject(denied()));
    request.once("timeout", () => request.destroy(denied()));
    request.end();
  });
}
async function requireActiveLicense(http, getBackend, enabled = false) {
  if (!enabled) return async () => {};
  const backend = getBackend(), denied = () => new Error("软件尚未激活");
  const read = async () => {
    if (getBackend() !== backend) throw denied();
    const value = await readLicenseSnapshot(http, getBackend, true);
    if (value.state !== "ACTIVE") throw denied();
    return value.generation;
  };
  const generation = await read();
  return async () => { if (await read() !== generation) throw denied(); };
}
module.exports = { requireActiveLicense, readLicenseSnapshot };
