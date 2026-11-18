// Real Chromium smoke test for CSS color resolution and row geometry.
"use strict";
const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { pathToFileURL } = require("node:url");
const web = path.join(__dirname, "..", "backend", "web");

function measure() {
  const get = (selector) => document.querySelector(selector);
  const read = (selector) => getComputedStyle(get(selector));
  const root = document.documentElement;
  const probe = get("#color-probe");
  const ctx = document.createElement("canvas").getContext("2d", { willReadFrequently: true });
  function luminance(color) {
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, 1, 1);
    const [r, g, b] = Array.from(ctx.getImageData(0, 0, 1, 1).data).map((byte) => {
      const srgb = byte / 255;
      return srgb <= 0.04045 ? srgb / 12.92 : ((srgb + 0.055) / 1.055) ** 2.4;
    });
    return r * 0.2126 + g * 0.7152 + b * 0.0722;
  }
  function variable(name) {
    probe.style.color = `var(${name})`;
    return getComputedStyle(probe).color;
  }
  const themes = ["light", "dark", "azure", "emerald", "violet", "crimson", "aurora", "oled"];
  const measurements = themes.map((theme) => {
    root.dataset.theme = theme;
    const name = read(".ladder-account .pro-name");
    const tag = read(".ladder-account .pro-tag");
    const mark = read(".ladder-account .pro-mark");
    const start = variable("--pro-name-start"), mid = variable("--pro-name-mid"), end = variable("--pro-name-end");
    const worstArt = `color-mix(in srgb, ${variable("--surface")} 82%, black)`;
    const backgrounds = [read(".ladder-table-scroll").backgroundColor, variable("--surface-strong"), variable("--primary-soft"), worstArt];
    const ratios = backgrounds.flatMap((background) => [start, mid, end].map((foreground) => {
      const a = luminance(foreground), b = luminance(background);
      return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
    }));
    return {
      theme, primary: variable("--primary-strong"), accent: variable("--pro-accent"),
      name: name.backgroundImage, tagColor: tag.color, mark: mark.backgroundImage,
      clip: mark.clipPath, start, end,
      overviewMatches: read(".summoner-strip .pro-name").backgroundImage === name.backgroundImage &&
        read(".summoner-strip .pro-name").color === name.color &&
        read(".summoner-strip .pro-tag").color === tag.color && read(".summoner-strip .pro-mark").color === mark.color,
      contrast: Math.min(...ratios),
    };
  });
  const table = get(".ladder-table-scroll");
  return {
    measurements, supportsRelative: CSS.supports("color", "oklch(from red calc(l + .03) calc(c * 1.15) calc(h + 45))"),
    skeletonCount: document.querySelectorAll(".ladder-skeleton").length,
    skeletonHeight: get(".ladder-skeleton").getBoundingClientRect().height,
    rowHeight: get("[data-ladder-row]").getBoundingClientRect().height,
    horizontalScroll: table.scrollWidth > table.clientWidth && document.documentElement.scrollWidth === innerWidth,
  };
}

test("R320 Chromium renders distinct theme colors, accessible light names and fixed rows", () => {
  const chrome = process.env.CHROME_BIN ||
    (process.platform === "darwin" ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" : "chromium");
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-r320-chrome-"));
  try {
    const css = fs.readFileSync(process.env.R320_APP_CSS || path.join(web, "app.css"), "utf8");
    const style = ["ladder.css", "gameplay.css"].map((name) => fs.readFileSync(path.join(web, name), "utf8")).join("\n");
    const shell = fs.readFileSync(path.join(web, "index.html"), "utf8");
    const ladder = shell.slice(shell.indexOf('<section id="ladder-panel"'), shell.indexOf('<section id="overview-panel"'));
    const mockRow = '<tr data-ladder-row><td>1</td><td><button class="ladder-account"><span class="pro-mark">P</span><span class="pro-name">Fixture</span><span class="pro-tag"><b>T1</b>Reviewed</span></button></td><td>大师</td><td>246</td><td></td><td>80</td><td>55%</td></tr>';
    const overview = '<section class="summoner-strip"><div class="summoner-strip-copy"><h2><span class="pro-mark">P</span><span class="pro-name">Fixture</span></h2><span class="pro-tag">Roster</span></div></section>';
    const fixture = path.join(root, "test.html");
    fs.writeFileSync(fixture, `<!doctype html><meta charset="UTF-8"><style>${css}\n${style}</style><div id="color-probe"></div><pre id="measurements"></pre>${ladder.replace(" data-ladder-style-pending", "").replace(" hidden>", ">").replace("</tbody>", mockRow + "</tbody>")}${overview}<script>document.getElementById('measurements').textContent=JSON.stringify((${measure})())</script>`);
    const result = execFileSync(chrome, [
      "--headless=new", "--no-sandbox", "--disable-background-networking", "--no-first-run", "--no-default-browser-check",
      "--window-size=720,900", `--user-data-dir=${path.join(root, "profile")}`, "--dump-dom", pathToFileURL(fixture).href,
    ], { timeout: 25000, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
    const match = result.match(/<pre id="measurements">(.+?)<\/pre>/);
    assert.ok(match, "Chromium did not measure the source styles");
    const { measurements, supportsRelative, skeletonCount, skeletonHeight, rowHeight, horizontalScroll } = JSON.parse(match[1].replace(/&gt;/g, ">").replace(/&amp;/g, "&"));
    assert.equal(supportsRelative, true);
    assert.equal(skeletonCount, 12);
    assert.equal(skeletonHeight, 52);
    assert.equal(rowHeight, 52);
    assert.equal(horizontalScroll, true);
    for (const field of ["name", "tagColor", "mark"]) {
      assert.equal(new Set(measurements.map((entry) => entry[field])).size, measurements.length, `${field} must follow every theme`);
    }
    for (const entry of measurements) {
      assert.notEqual(entry.accent, entry.primary, entry.theme);
      assert.notEqual(entry.tagColor, entry.primary, entry.theme);
      assert.notEqual(entry.start, entry.end, entry.theme);
      assert.match(entry.name, /^linear-gradient/);
      assert.match(entry.mark, /^linear-gradient/);
      assert.match(entry.clip, /^polygon/);
      assert.equal(entry.overviewMatches, true, `${entry.theme}: overview uses identical professional styles`);
    }
    assert.ok(measurements[0].contrast >= 4.5, `light theme contrast: ${measurements[0].contrast}`);
    console.log(`Light theme minimum contrast: ${measurements[0].contrast.toFixed(2)}:1`);
  } finally {
    fs.rmSync(root, { recursive: true, force: true, maxRetries: 4 });
  }
});
