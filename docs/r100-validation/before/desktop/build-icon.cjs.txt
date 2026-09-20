const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const desktopRoot = __dirname;
const projectRoot = path.dirname(desktopRoot);
const source = path.join(desktopRoot, "assets", "hexcore-icon-1024.png");
const outputIco = path.join(desktopRoot, "assets", "hexcore-icon.ico");
const webIcon = path.join(projectRoot, "web", "app-icon.png");
const sizes = [16, 24, 32, 48, 64, 128, 256];
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "lol-loot-icon-"));

function sips(args) {
  execFileSync("sips", args, { stdio: "ignore" });
}

try {
  const framedSource = path.join(temporary, "icon-framed.png");
  const taskbarSource = path.join(temporary, "icon-taskbar.png");
  // The taskbar scales the entire transparent canvas. Tighten the source to a
  // symmetric 900px frame so the hexcore occupies the native 16–32px layers.
  sips(["-c", "900", "900", "--cropOffset", "62", "62", source, "--out", framedSource]);
  sips(["-z", "256", "256", framedSource, "--out", webIcon]);

  // Windows visually gives tall emblems less taskbar area than square glyphs.
  // Preserve the full vertical silhouette, but use a narrower centered source
  // frame for the ICO so the hexcore occupies more horizontal taskbar pixels.
  sips(["-c", "900", "800", "--cropOffset", "62", "112", source, "--out", taskbarSource]);

  const images = sizes.map((size) => {
    const file = path.join(temporary, `icon-${size}.png`);
    sips(["-z", String(size), String(size), taskbarSource, "--out", file]);
    return { size, data: fs.readFileSync(file) };
  });
  const directorySize = 6 + images.length * 16;
  const header = Buffer.alloc(directorySize);
  header.writeUInt16LE(0, 0);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(images.length, 4);
  let offset = directorySize;
  images.forEach(({ size, data }, index) => {
    const entry = 6 + index * 16;
    header[entry] = size === 256 ? 0 : size;
    header[entry + 1] = size === 256 ? 0 : size;
    header[entry + 2] = 0;
    header[entry + 3] = 0;
    header.writeUInt16LE(1, entry + 4);
    header.writeUInt16LE(32, entry + 6);
    header.writeUInt32LE(data.length, entry + 8);
    header.writeUInt32LE(offset, entry + 12);
    offset += data.length;
  });
  fs.writeFileSync(outputIco, Buffer.concat([header, ...images.map(({ data }) => data)]));
  process.stdout.write("Regenerated web PNG and multi-size ICO assets from the tightly framed source.\n");
} finally {
  fs.rmSync(temporary, { recursive: true, force: true });
}
