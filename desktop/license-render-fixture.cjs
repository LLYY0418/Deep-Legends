"use strict";
// Test transport injection only; this module is absent from build.files.
function installLicenseRenderFixture(window) {
  const enabled = process.env.DEEP_LEGENDS_TEST_LICENSE === "1";
  window.document.documentElement.dataset.license = enabled ? "pending" : "disabled";
  const original = window.fetch.bind(window);
  window.fetch = (input, init) => {
    const url = typeof input === "string" ? input : input?.url || "";
    if (new URL(url, window.location.href).pathname === "/api/license/status") return Promise.resolve(new window.Response(JSON.stringify(enabled ? {state:"ACTIVE",generation:1,message:""} : {state:"DISABLED"})));
    return original(input, init);
  };
}
function licenseFixtureHTML(body) {
  const marker = Buffer.from('data-license="disabled"');
  if (!body.includes(marker)) return body;
  return Buffer.from(body.toString("utf8").replace('data-license="disabled"','data-license="pending"').replace('id="app-frame" class="app-frame"','id="app-frame" class="app-frame" inert hidden'));
}
module.exports = { installLicenseRenderFixture, licenseFixtureHTML };
