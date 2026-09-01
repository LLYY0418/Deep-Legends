"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const templatePath = path.join(__dirname, "nsis", "portable.nsi");
const applyTemplate = require("./apply-portable-template.cjs");
const verifyHook = require("./verify-embedded-riot-key.cjs");

test("定制便携版模板存在且保留缓存秒开的关键行为", () => {
  const script = fs.readFileSync(templatePath, "utf8");
  // 缓存目录必须版本化，且命中缓存时静默直启。
  assert.match(script, /\$LOCALAPPDATA\\\$\{APP_FILENAME\}\\app-\$\{VERSION\}/);
  assert.match(script, /@@BUILD_FINGERPRINT@@/);
  assert.doesNotMatch(script, /DEEP_LEGENDS_CACHE_REVISION/);
  assert.match(script, /app-\$\{VERSION\}-\$\{DEEP_LEGENDS_BUILD_FINGERPRINT\}/);
  assert.match(script, /SetSilent silent/);
  // 解压完成后写就绪标记，未完成的解压下次自动重来。
  assert.match(script, /\.deep-legends-ready/);
  assert.match(script, /!insertmacro extractEmbeddedAppPackage/);
  // 主程序路径必须带引号（产品名含空格），且使用 Exec 立即退出启动器。
  assert.match(script, /Exec '"\$INSTDIR\\\$\{APP_EXECUTABLE_FILENAME\}" \$R0'/);
  const code = script.split("\n").filter((line) => !line.trimStart().startsWith("#")).join("\n");
  assert.ok(!code.includes("ExecWait"), "启动器不应等待主程序退出");
  // 退出后不得删除缓存目录（上游模板的 RMDir /r $INSTDIR 出现在 Exec 之后）。
  const execIndex = script.indexOf("Exec '");
  assert.ok(execIndex > 0);
  assert.ok(!script.slice(execIndex).includes("RMDir"), "Exec 之后不允许再清理缓存目录");
  // 便携环境变量与参数透传保持上游行为。
  assert.match(script, /PORTABLE_EXECUTABLE_DIR/);
  assert.match(script, /StdUtils\.GetAllParameters/);
});

test("打包配置与模板配套：asarUnpack 后端、不再使用解压期启动图", () => {
  const config = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  assert.deepEqual(config.build.asarUnpack, ["backend/loot-service.exe"]);
  assert.ok(!("splashImage" in (config.build.portable || {})));
  assert.ok(config.build.files.includes("backend/loot-service.exe"));
  assert.ok(!config.build.files.includes("runtime-backend.cjs"));
});

test("模板应用会替换指纹占位符，且 beforePack 拒绝过期模板", () => {
  const builderTemplate = path.join(__dirname, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
  const original = fs.existsSync(builderTemplate) ? fs.readFileSync(builderTemplate, "utf8") : null;
  try {
    const fingerprint = applyTemplate.applyPortableTemplate();
    const applied = fs.readFileSync(builderTemplate, "utf8");
    assert.doesNotMatch(applied, /@@BUILD_FINGERPRINT@@/);
    assert.match(applied, new RegExp(`DEEP_LEGENDS_BUILD_FINGERPRINT\\s+"${fingerprint}"`));
    assert.doesNotThrow(() => verifyHook.verifyPortableTemplate());
    fs.writeFileSync(builderTemplate, applied.replace(fingerprint, "000000000000"), "utf8");
    assert.throws(() => verifyHook.verifyPortableTemplate(), /指纹过期/);
  } finally {
    if (original !== null) fs.writeFileSync(builderTemplate, original, "utf8");
  }
});

test("模板前置校验拒绝未替换占位符，后端内容变化会改变指纹", () => {
  const builderTemplate = path.join(__dirname, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
  const backendPath = path.join(__dirname, "backend", "loot-service.exe");
  const originalTemplate = fs.readFileSync(builderTemplate, "utf8");
  const originalBackend = fs.readFileSync(backendPath);
  try {
    fs.writeFileSync(builderTemplate, fs.readFileSync(templatePath, "utf8"), "utf8");
    assert.throws(() => verifyHook.verifyPortableTemplate(), /占位符/);
    fs.writeFileSync(builderTemplate, originalTemplate, "utf8");
    const before = applyTemplate.buildFingerprint();
    const changed = Buffer.from(originalBackend);
    changed[changed.length - 1] ^= 1;
    fs.writeFileSync(backendPath, changed);
    assert.notEqual(applyTemplate.buildFingerprint(), before);
  } finally {
    fs.writeFileSync(backendPath, originalBackend);
    fs.writeFileSync(builderTemplate, originalTemplate, "utf8");
  }
});

test("同时产出便携版与安装版，且文件名不冲突", () => {
  const config = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  const targets = config.build.win.target.map((entry) => entry.target);
  assert.deepEqual(targets.sort(), ["nsis", "portable"]);
  assert.match(config.build.portable.artifactName, /Deep Legends \$\{env\.DEEP_LEGENDS_FINGERPRINT\}/);
  assert.match(config.build.nsis.artifactName, /Deep Legends Setup \$\{env\.DEEP_LEGENDS_FINGERPRINT\}/);
  // 安装版必须是用户级免管理员安装，且允许自选目录。
  assert.equal(config.build.nsis.oneClick, false);
  assert.equal(config.build.nsis.perMachine, false);
  assert.equal(config.build.nsis.allowToChangeInstallationDirectory, true);
  // 两个 NSIS 系目标必须分两次调用；同一次构建会互相踩包文件。
  assert.equal(config.scripts["pack:win"], "electron-builder --win portable --x64");
  assert.equal(config.scripts["pack:win-setup"], "electron-builder --win nsis --x64");
});
