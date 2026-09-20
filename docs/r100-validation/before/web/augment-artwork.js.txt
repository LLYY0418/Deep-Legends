(() => {
  "use strict";
  // Riot also ships neutral gray/teal glyphs (including Bonk_large), not just
  // pre-colored artwork. Tint only those glyphs, using the metadata rarity.
  // Never infer rarity from a filename or recolor already-colored artwork.
  function isNeutralGlyph(pixels) {
    let visible = 0, neutral = 0, transparent = 0;
    for (let i = 0; i < pixels.length; i += 4) {
      const r = pixels[i], g = pixels[i + 1], b = pixels[i + 2], a = pixels[i + 3];
      if (a < 32) { transparent++; continue; }
      if (a < 160) continue; // Ignore anti-aliased edges.
      visible++;
      const high = Math.max(r, g, b), low = Math.min(r, g, b);
      const gray = high - low <= 12;
      const riotTeal = r <= g && Math.abs(g - b) <= 12 && high - low <= high * .42;
      if (gray || riotTeal) neutral++;
    }
    return visible >= 16 && transparent > pixels.length / 4 * .12 && neutral / visible >= .995;
  }

  function tintPixels(pixels, width, height, rarity) {
    // Map glyph luminance to artwork tones (not a second multiplication by
    // source brightness). Riot gold art has an olive rim and a cream/tan center.
    const palettes = {
      gold: [[76, 78, 64], [174, 148, 108], [255, 237, 198]],
      prismatic: [[66, 59, 83], [174, 141, 224], [224, 244, 255]],
      silver: [[62, 77, 86], [153, 178, 195], [234, 243, 249]],
    };
    const palette = palettes[rarity];
    if (!palette || !isNeutralGlyph(pixels)) return false;
    const luminance = (i) => .2126 * pixels[i] + .7152 * pixels[i + 1] + .0722 * pixels[i + 2];
    const levels = [];
    for (let i = 0; i < pixels.length; i += 4) if (pixels[i + 3] >= 160) levels.push(luminance(i));
    levels.sort((a, b) => a - b);
    const low = levels[Math.floor(levels.length * .05)], high = levels[Math.floor(levels.length * .95)];
    for (let i = 0; i < pixels.length; i += 4) {
      if (!pixels[i + 3]) continue;
      const light = high - low > 1 ? Math.max(0, Math.min(1, (luminance(i) - low) / (high - low))) : .7;
      const offset = light * 2, index = Math.min(1, Math.floor(offset)), t = offset - index;
      for (let c = 0; c < 3; c++) pixels[i + c] = Math.round(palette[index][c] * (1 - t) + palette[index + 1][c] * t);
    }
    return true;
  }

  function prepare(image) {
    const holder = image.parentElement;
    if (!holder || (!holder.classList.contains("augment-icon") && !holder.closest(".arena-augment-icon"))) return;
    const tone = holder.closest(".is-gold, .is-prismatic, .is-silver");
    const rarity = ["gold", "prismatic", "silver"].find((value) => tone?.classList.contains(`is-${value}`));
    const key = `${image.currentSrc || image.src}|${rarity}`;
    if (!rarity || image.dataset.augmentArtworkKey === key || !image.naturalWidth) return;
    image.dataset.augmentArtworkKey = key;
    holder.querySelector(".augment-colored-glyph")?.remove();
    holder.classList.remove("has-colored-glyph");
    try {
      const canvas = document.createElement("canvas");
      canvas.width = Math.min(256, image.naturalWidth);
      canvas.height = Math.min(256, image.naturalHeight);
      const context = canvas.getContext("2d", { willReadFrequently: true });
      if (!context) return;
      context.drawImage(image, 0, 0, canvas.width, canvas.height);
      const data = context.getImageData(0, 0, canvas.width, canvas.height);
      if (!tintPixels(data.data, canvas.width, canvas.height, rarity)) return;
      context.putImageData(data, 0, 0);
      canvas.className = "augment-colored-glyph";
      canvas.setAttribute("aria-hidden", "true");
      holder.append(canvas);
      holder.classList.add("has-colored-glyph");
    } catch (_) { /* Keep original artwork if canvas access/decoding is unavailable. */ }
  }
  window.deepLegendsAugmentArtwork = { prepare, isNeutralGlyph, tintPixels };
})();
