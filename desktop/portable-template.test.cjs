"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const crypto = require("node:crypto");
const path = require("node:path");

const templatePath = path.join(__dirname, "nsis", "portable.nsi");
const applyTemplate = require("./apply-portable-template.cjs");
const verifyHook = require("./verify-embedded-riot-key.cjs");
const runtimeVerifier = require("./verify-packaged-runtime.cjs");

// A test forgetting its fixture root must fail before touching a user's build.
// Other test files run in separate Node processes; restore our wrappers on exit.
const protectedBuildFiles = new Set([
  path.join(__dirname, "backend", "loot-service.exe"),
  path.join(__dirname, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi"),
]);
const writeMethods = { writeFileSync: 0, appendFileSync: 0, copyFileSync: 1 };
const originalWriteMethods = new Map();
test.before(() => {
  for (const [name, destinationIndex] of Object.entries(writeMethods)) {
    const original = fs[name];
    originalWriteMethods.set(name, original);
    fs[name] = (...args) => {
      const destination = args[destinationIndex];
      if (typeof destination === "string") {
        assert.ok(!protectedBuildFiles.has(path.resolve(destination)), `test attempted to overwrite a real build input: ${destination}`);
      }
      return original(...args);
    };
  }
});
test.after(() => { for (const [name, original] of originalWriteMethods) fs[name] = original; });

function isolatedDesktop(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-portable-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const backend = path.join(root, "backend", "loot-service.exe");
  const template = path.join(root, "node_modules", "app-builder-lib", "templates", "nsis", "portable.nsi");
  fs.mkdirSync(path.dirname(backend), { recursive: true });
  fs.mkdirSync(path.dirname(template), { recursive: true });
  fs.mkdirSync(path.join(root, "nsis"));
  fs.copyFileSync(templatePath, path.join(root, "nsis", "portable.nsi"));
  fs.writeFileSync(backend, "synthetic test backend, never a packaged executable");
  fs.writeFileSync(template, "synthetic upstream template");
  for (const name of ["main.cjs", "preload.cjs", "proxy-resolution.cjs"]) {
    fs.writeFileSync(path.join(root, name), `// synthetic ${name}\n`);
  }
  return { root, backend, template };
}

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
  assert.ok(config.build.files.includes("window-bounds-store.cjs"));
  assert.ok(!config.build.files.includes("runtime-backend.cjs"));
});

test("所有桌面运行时本地模块都进入 app.asar 和源码指纹", () => {
  const config = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  const packaged = new Set(config.build.files);
  const runtimeModules = config.build.files.filter((name) => name.endsWith(".cjs"));
  for (const moduleName of runtimeModules) {
    const source = fs.readFileSync(path.join(__dirname, moduleName), "utf8");
    for (const match of source.matchAll(/require\(["']\.\/([^"']+)["']\)/g)) {
      const dependency = path.posix.normalize(path.posix.join(path.posix.dirname(moduleName), match[1]));
      assert.ok(packaged.has(dependency), `${moduleName} requires unpackaged runtime module ${dependency}`);
    }
  }
  const fingerprintSource = fs.readFileSync(path.join(__dirname, "source-fingerprint.cjs"), "utf8");
  assert.match(fingerprintSource, /desktopConfig\.build\.files/);
  assert.match(fingerprintSource, /name\.endsWith\("\.cjs"\)/);

  const required = runtimeVerifier.requiredRuntimeEntries();
  assert.ok(required.includes("window-bounds-store.cjs"));
  assert.doesNotThrow(() => runtimeVerifier.verifyArchiveEntries(required.map((name) => `/${name}`)));
  assert.throws(
    () => runtimeVerifier.verifyArchiveEntries(required.filter((name) => name !== "window-bounds-store.cjs")),
    /window-bounds-store\.cjs/,
  );

  const shellBuild = fs.readFileSync(path.join(__dirname, "..", "build-desktop.sh"), "utf8");
  const windowsBuild = fs.readFileSync(path.join(__dirname, "..", "build-desktop-windows.ps1"), "utf8");
  assert.match(shellBuild, /verify-packaged-runtime\.cjs/);
  assert.match(windowsBuild, /verify-packaged-runtime\.cjs/);
});

test("Shell 只构建 setup 并复用 Electron 下载缓存", () => {
  const source = fs.readFileSync(path.join(__dirname, "..", "build-desktop.sh"), "utf8");

  assert.doesNotMatch(source, /\bgo build\s+-a\b/);
  assert.match(source, /electron_zip_name="electron-v\$\{electron_version\}-win32-x64\.zip"/);
  assert.match(source, /\$HOME\/Library\/Caches\/electron/);
  assert.match(source, /\$XDG_CACHE_HOME\/electron/);
  assert.match(source, /command -v find/);
  assert.match(source, /command -v unzip/);
  assert.match(source, /unzip -tq "\$candidate"/);
  assert.match(source, /AUTO_ELECTRON_DIST/);
  assert.match(source, /if \[\[ -n "\$\{ELECTRON_DIST:-\}" \]\]; then\s+printf '%s\\n' "\$ELECTRON_DIST"/);
  assert.match(source, /Reusing cached Electron distribution/);

  const resolutionIndexes = [...source.matchAll(/electron_dist="\$\(resolve_electron_dist \|\| true\)"/g)].map((match) => match.index);
  assert.equal(resolutionIndexes.length, 1, "setup 前只需查一次缓存");
  assert.match(source, /setup_args\+=\(--config\.electronDist="\$electron_dist"\)/);

  const setupIndex = source.indexOf('npm run pack:win-setup -- "${setup_args[@]}"');
  assert.ok(setupIndex > resolutionIndexes[0], "setup 构建前应先尝试复用已有缓存");
  for (const file of ["build-desktop.sh", "build-desktop-windows.ps1"]) {
    const entry = fs.readFileSync(path.join(__dirname, "..", file), "utf8");
    assert.equal((entry.match(/npm run pack:win-setup/g) || []).length, 1);
    assert.doesNotMatch(entry, /npm run pack:win\s|Compress-Archive|zip -qr|apply-portable-template|SKIP_SETUP/);
  }
});

test("历史 portable 模板工具替换指纹占位符并拒绝过期模板", (t) => {
  const { root, template } = isolatedDesktop(t);
  const fingerprint = applyTemplate.applyPortableTemplate(root);
  const applied = fs.readFileSync(template, "utf8");
  assert.doesNotMatch(applied, /@@BUILD_FINGERPRINT@@/);
  assert.match(applied, new RegExp(`DEEP_LEGENDS_BUILD_FINGERPRINT\\s+"${fingerprint}"`));
  assert.doesNotThrow(() => verifyHook.verifyPortableTemplate(root));
  fs.writeFileSync(template, applied.replace(fingerprint, "000000000000"), "utf8");
  assert.throws(() => verifyHook.verifyPortableTemplate(root), /指纹过期/);
});

test("模板前置校验拒绝未替换占位符，后端内容变化会改变指纹", (t) => {
  const { root, template, backend } = isolatedDesktop(t);
  fs.copyFileSync(templatePath, template);
  assert.throws(() => verifyHook.verifyPortableTemplate(root), /占位符/);
  const before = applyTemplate.applyPortableTemplate(root);
  fs.appendFileSync(backend, "changed backend");
  assert.notEqual(applyTemplate.buildFingerprint(root), before);
  assert.throws(() => verifyHook.verifyPortableTemplate(root), /指纹过期/);
});

test("模板指纹仅读取指定工作目录的四个输入", (t) => {
  const { root } = isolatedDesktop(t);
  const hash = crypto.createHash("sha256");
  for (const name of ["backend/loot-service.exe", "main.cjs", "preload.cjs", "proxy-resolution.cjs"]) {
    hash.update(fs.readFileSync(path.join(root, name)));
  }
  assert.equal(applyTemplate.buildFingerprint(root), hash.digest("hex").slice(0, 12));
});

test("交错执行破坏性模板测试不会污染另一个构建的后端和模板", (t) => {
  const build = isolatedDesktop(t), probe = isolatedDesktop(t);
  applyTemplate.applyPortableTemplate(build.root);
  const originalBackend = fs.readFileSync(build.backend), originalTemplate = fs.readFileSync(build.template);
  assert.doesNotThrow(() => verifyHook.verifyPortableTemplate(build.root));
  applyTemplate.applyPortableTemplate(probe.root);
  fs.writeFileSync(probe.backend, ""); // even the truncation phase stays inside the fixture
  assert.throws(() => verifyHook.verifyPortableTemplate(probe.root), /指纹过期/);
  fs.copyFileSync(templatePath, probe.template);
  assert.throws(() => verifyHook.verifyPortableTemplate(probe.root), /占位符/);
  assert.doesNotThrow(() => verifyHook.verifyPortableTemplate(build.root));
  assert.deepEqual(fs.readFileSync(build.backend), originalBackend);
  assert.deepEqual(fs.readFileSync(build.template), originalTemplate);
});

test("默认只有 setup 目标，安装选项和外观资源保持完整", () => {
  const config = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  const targets = config.build.win.target.map((entry) => entry.target);
  assert.deepEqual(targets, ["nsis"]);
  assert.equal(config.build.portable, undefined);
  assert.match(config.build.nsis.artifactName, /Deep Legends Setup \$\{version\}/);
  // 安装版必须是用户级免管理员安装，且允许自选目录。
  assert.equal(config.build.nsis.oneClick, false);
  assert.equal(config.build.nsis.perMachine, false);
  // The stock directory page is replaced, not added alongside our branded page.
  assert.equal(config.build.nsis.allowToChangeInstallationDirectory, false);
  assert.equal(config.build.nsis.include, "nsis/installer.nsh");
  const installer = fs.readFileSync(path.join(__dirname, config.build.nsis.include), "utf8");
  assert.match(installer, /Page custom DLDirectoryPage DLDirectoryLeave/);
  assert.equal(config.scripts["pack:win"], "npm run pack:win-setup --");
  assert.equal(config.scripts["pack:win-setup"], "electron-builder --win nsis --x64");
});

test("R71 executable resource editing stays enabled and branded setup has exactly two choices", () => {
  for (const file of ["build-desktop.sh", "build-desktop-windows.ps1"]) {
    const source = fs.readFileSync(path.join(__dirname, "..", file), "utf8");
    assert.doesNotMatch(source, /--config\.win\.signAndEditExecutable=false/);
    assert.match(source, /--config\.win\.signExecutable=false/);
  }
  const { build } = require("./package.json");
  for (const icon of [build.win.icon, build.nsis.installerIcon, build.nsis.uninstallerIcon]) {
    assert.equal(icon, "assets/hexcore-icon.ico");
    const data = fs.readFileSync(path.join(__dirname, icon));
    assert.equal(data.readUInt16LE(2), 1, "ICO resource required");
  }
  const source = fs.readFileSync(path.join(__dirname, build.nsis.include), "utf8");
  assert.equal((source.match(/Page custom /g) || []).length, 2);
  assert.match(source, /NSD_CreateDirRequest/);
  assert.match(source, /NSD_CreateCheckbox/);
  assert.match(source, /customInstallMode/);
  assert.ok(source.indexOf("kernel32::GetFullPathNameW") < source.indexOf("StrLen $2 $1"), "validate canonical destination, not user spelling");
  assert.match(source, /ExecShellAsUser/);
  assert.match(source, /CreateDirectory "\$1"/);
  assert.match(source, /GetTempFileName \$2 "\$1"/);
  assert.ok(source.indexOf("GetTempFileName") < source.indexOf("StrCpy $INSTDIR $1"));
});


test("NSIS page functions wait until standard builder plugins are available", () => {
  const script = fs.readFileSync(path.join(__dirname, "nsis", "installer.nsh"), "utf8");
  const start = script.indexOf("!macro customHeader");
  const end = script.indexOf("!macroend", start);
  assert.ok(start >= 0 && end > start);
  for (const match of script.matchAll(/^Function /gm)) {
    assert.ok(match.index > start && match.index < end, "functions must be deferred beyond the async custom include");
  }
  assert.match(script, /!define DL_INSTALLER_ICON/);
  assert.match(script, /\$\{If\} \$\{isForAllUsers\}/);
  assert.match(script, /\$\{ElseIf\} \$\{isForCurrentUser\}/);
});
