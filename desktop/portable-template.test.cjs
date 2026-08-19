"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const templatePath = path.join(__dirname, "nsis", "portable.nsi");

test("定制便携版模板存在且保留缓存秒开的关键行为", () => {
  const script = fs.readFileSync(templatePath, "utf8");
  // 缓存目录必须版本化，且命中缓存时静默直启。
  assert.match(script, /\$LOCALAPPDATA\\\$\{APP_FILENAME\}\\app-\$\{VERSION\}/);
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

test("同时产出便携版与安装版，且文件名不冲突", () => {
  const config = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  const targets = config.build.win.target.map((entry) => entry.target);
  assert.deepEqual(targets.sort(), ["nsis", "portable"]);
  assert.equal(config.build.portable.artifactName, "Deep Legends.${ext}");
  assert.equal(config.build.nsis.artifactName, "Deep Legends Setup.${ext}");
  // 安装版必须是用户级免管理员安装，且允许自选目录。
  assert.equal(config.build.nsis.oneClick, false);
  assert.equal(config.build.nsis.perMachine, false);
  assert.equal(config.build.nsis.allowToChangeInstallationDirectory, true);
  // 两个 NSIS 系目标必须分两次调用；同一次构建会互相踩包文件。
  assert.equal(config.scripts["pack:win"], "electron-builder --win portable --x64");
  assert.equal(config.scripts["pack:win-setup"], "electron-builder --win nsis --x64");
});
