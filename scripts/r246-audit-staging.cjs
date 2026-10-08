"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Read-only audit of the actual public staging Setup/dir and a public release
// backend. Only public trust material and synthetic vector markers are read.
const fs = require("node:fs"), path = require("node:path"), crypto = require("node:crypto"), assert = require("node:assert/strict");
const root = path.resolve(__dirname, ".."), desktop = path.join(root, "desktop");
const digest = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
async function audit() {
  const output = path.resolve(process.argv[2] || path.join(root, "dist/R246-staging-public"));
  const receipt = JSON.parse(fs.readFileSync(path.join(output, "staging-build.json"), "utf8"));
  const archive = path.join(receipt.directory, "resources/app.asar"), backend = path.join(receipt.directory, "resources/app.asar.unpacked/backend/loot-service.exe"), exe = path.join(receipt.directory, "Deep Legends.exe");
  const asar = require(path.join(desktop, "node_modules/@electron/asar"));
  const binary = fs.readFileSync(backend), entries = asar.listPackage(archive);
  const pkg = JSON.parse(asar.extractFile(archive, "package.json").toString());
  assert.equal(pkg.version, "0.12.75"); assert.equal(receipt.key_mode, "public");
  for (const [field, file] of [["setup_sha256", receipt.setup], ["backend_sha256", backend], ["archive_sha256", archive]]) assert.equal(digest(file), receipt[field]);
  require(path.join(desktop, "verify-license-staging-pack.cjs")).verifyStagingBackend(backend);
  require(path.join(desktop, "verify-packaged-runtime.cjs")).verifyPackagedRuntime(archive);
  assert.ok(asar.extractFile(archive, "backend-digest.cjs").toString().includes(receipt.backend_sha256));
  assert.ok(asar.extractFile(archive, "app-title.cjs").toString().includes('Deep Legends-STAGING'));
  assert.ok(asar.extractFile(archive, "license-window.cjs").toString().includes('LICENSE_WIDTH = 860, LICENSE_HEIGHT = 580'));
  assert.ok(asar.extractFile(archive, "main.cjs").toString().includes('desktop-license-rendered'));
  assert.ok(asar.extractFile(archive, "preload.cjs").toString().includes('desktop-license-apply'));
  assert.ok(binary.includes(Buffer.from('data-license="pending"')));
  assert.ok(binary.includes(Buffer.from("注册码已在其他设备使用或已被重置")));
  assert.ok(!entries.some(entry => /(?:testdata|\.test\.|_test\.go|protocol-vectors|PRIVATE KEY|verify-license|build-license-staging)/i.test(entry)));
  for (const marker of ['注册码已过期', 'terminal_reason', 'license_expires_at', '授权到期：永久', 'setting-license-expiry']) assert.ok(binary.includes(Buffer.from(marker)), 'R242 marker missing: ' + marker);
  assert.ok(asar.extractFile(archive, 'main.cjs').toString().includes('license_expires_at: waiter.snapshot.license_expires_at'));
  const controller=asar.extractFile(archive,'license-window.cjs').toString(),main=asar.extractFile(archive,'main.cjs').toString();
  for(const marker of ['verifyActiveSize','getInitialBounds','fallback_used','nativeMs','renderMs','statusReadMs'])assert.ok(controller.includes(marker),'R243 geometry marker missing: '+marker);
  assert.ok(main.includes('getInitialBounds: initialWindowBounds'));
  assert.ok(main.includes('backgroundThrottling: false'));
  for(const marker of ['license_window_state_invalid','unsigned_error','bad_signature','elapsed_ms','cf_ray','正在激活…'])assert.ok(binary.includes(Buffer.from(marker)),'R243 backend/UI marker missing: '+marker);
  assert.ok(binary.includes(Buffer.from('36000')),'R245 combined activation wait absent');
  for(const marker of ['Math.round(bounds[key])','native_error','validRestore'])assert.ok(controller.includes(marker),'R245 native geometry marker missing: '+marker);
  for(const marker of ['dns_ms','connect_ms','tls_ms','ttfb_ms','body_ms','scale_factor','non_integer','non_positive','out_of_range','正在连接激活服务…'])assert.ok(binary.includes(Buffer.from(marker)),'R245 diagnostic marker missing: '+marker);
  assert.ok(binary.includes(Buffer.from('有效租约最多2小时。')), 'R246 privacy lease text absent');
  assert.ok(!binary.includes(Buffer.from('有效租约最多15分钟。')), 'obsolete lease text remains');
  assert.ok(!binary.includes(Buffer.from('__collectionStateForTest')), 'R247 test observation leaked into runtime');
  const privacy = "服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。";
  assert.ok(binary.includes(Buffer.from(privacy)), "R237 finalized privacy must be embedded");
  assert.ok(binary.includes(Buffer.from("#license-form, #license-privacy-dialog[open]")), "P1 CSS fix must be embedded");
  const { getCurrentFuseWire, FuseV1Options } = require(path.join(desktop, "node_modules/@electron/fuses"));
  const { FuseState } = require(path.join(desktop, "node_modules/@electron/fuses/dist/constants.js"));
  const wire = await getCurrentFuseWire(exe), fuses = {};
  for (const [name, expected] of [["RunAsNode", false], ["EnableNodeOptionsEnvironmentVariable", false], ["EnableNodeCliInspectArguments", false], ["EnableEmbeddedAsarIntegrityValidation", true], ["OnlyLoadAppFromAsar", true]]) {
    fuses[name] = wire[FuseV1Options[name]] === FuseState.ENABLE; assert.equal(wire[FuseV1Options[name]], expected ? FuseState.ENABLE : FuseState.DISABLE);
  }
  const { NtExecutable, NtExecutableResource } = await import(path.join(desktop, "node_modules/resedit/dist/index.js"));
  const resources = NtExecutableResource.from(NtExecutable.from(fs.readFileSync(exe)));
  const integrity = resources.entries.find(entry => entry.type === "INTEGRITY" && entry.id === "ELECTRONASAR");
  assert.ok(integrity, "Electron ASAR integrity resource missing");
  const records = JSON.parse(Buffer.from(integrity.bin).toString("utf8"));
  const { header } = await require(path.join(desktop, "node_modules/app-builder-lib/out/asar/asar.js")).readAsarHeader(archive);
  const headerHash = crypto.createHash("sha256").update(header).digest("hex");
  assert.ok(records.some(row => row.file === "resources\\app.asar" && row.alg === "SHA256" && row.value === headerHash));
  const setupBytes = fs.readFileSync(receipt.setup), peOffset = setupBytes.readUInt32LE(0x3c);
  assert.equal(setupBytes.readUInt16LE(peOffset + 4), 0x8664); assert.equal(setupBytes.readUInt16LE(peOffset + 24 + 68), 2, "Setup must be PE amd64 GUI");
  assert.ok(setupBytes.includes(Buffer.from('当前2小时租约')), 'R246 installer lease text absent');
  assert.ok(!setupBytes.includes(Buffer.from('当前15分钟租约')), 'obsolete installer lease text remains');
  const releaseBackend = process.argv[3]; assert.ok(releaseBackend, "public release backend required for trust isolation audit");
  const release = fs.readFileSync(releaseBackend), pub = "E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE";
  for (const marker of [Buffer.from(receipt.license_origin), Buffer.from(receipt.license_kid), Buffer.from(pub), Buffer.from(pub, "base64url")]) assert.ok(!release.includes(marker), "staging trust leaked into release");
  require(path.join(desktop, "verify-embedded-riot-key.cjs")).verifyRiotKeyPolicy(releaseBackend, "public");
  const { verifyLicenseRelease } = require(path.join(desktop, "verify-license-release.cjs"));
  verifyLicenseRelease(releaseBackend);
  assert.throws(() => verifyLicenseRelease(backend), /staging license origin|test trust marker/);
  const result = { ...receipt, scope: "Read-only macOS static audit of actual Windows public artifacts; Windows execution pending", packaged_version: pkg.version,
    setup_bytes: setupBytes.length, backend_bytes: binary.length, license_origin_and_kid_and_key_match: true, finalized_privacy_embedded: true, r240_css_and_window_module_embedded: true, r242_protocol_and_settings_embedded: true, r243_failure_retry_and_window_guard_embedded: true, r245_geometry_trace_and_startup_embedded: true, r246_two_hour_lease_copy_embedded: true, r247_test_observation_absent: true,
    test_fixtures_and_known_private_material_absent: true, personal_riot_key_absent: true, fixed_backend_digest_matches: true, fuses, asar_integrity: records,
    staging_rejected_by_release_checker: true, release_staging_material_absent: true, release_backend_sha256: digest(releaseBackend) };
  fs.mkdirSync(path.dirname(evidencePath('r246/package-audit.json')), { recursive: true });
  fs.writeFileSync(evidencePath('r246/package-audit.json'), JSON.stringify(result, null, 2) + "\n"); console.log(JSON.stringify(result));
}
audit().catch(error => { console.error(error); process.exitCode = 1; });
