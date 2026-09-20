(() => {
  "use strict";
  const escape = value => String(value).replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)");
  const videos = new Map();
  const holders = new Set();
  // Coordinates belong to a composition, not just a champion. Cover the entire
  // banner first, then center the vertical focal point without exposing edges.
  const focal = { 103086: .31, 126000: .30, 68000: .36, 101000: .29 };
  // GTIMG / Data Dragon / v1 champion-splashes are the ordinary composition.
  // Do not use the centered catalogue coordinates when it is unavailable.
  const ordinaryFocal = { 103086: .16, 101000: .20 };
  const resize = typeof ResizeObserver === "function" ? new ResizeObserver(entries => {
    for (const { target } of entries) position(target);
  }) : null;
  function position(holder) {
    for (const media of holder.querySelectorAll(".overview-art-focus")) {
      const ratio = (media.naturalWidth || media.videoWidth) / (media.naturalHeight || media.videoHeight);
      if (!ratio) continue;
      const table = holder.dataset.composition === "centered" ? focal : ordinaryFocal;
      const y = table[Number(holder.dataset.focusSkin)] ?? .32;
      // Near-edge faces may require a little extra zoom at narrow widths;
      // otherwise clamping a cover image to its edge cannot center the face.
      const h = Math.max(holder.clientHeight, holder.clientWidth / ratio, holder.clientHeight / (2 * y), holder.clientHeight / (2 * (1 - y)));
      const top = Math.min(0, Math.max(holder.clientHeight - h, holder.clientHeight / 2 - h * y));
      Object.assign(media.style, { inset: "auto", width: `${h * ratio}px`, height: `${h}px`, left: `${(holder.clientWidth - h * ratio) / 2}px`, top: `${top}px`, transform: "none" });
    }
  }
  function render(player) {
    const fallback = player.backgroundSource && player.backgroundPath ? `/api/champion-asset?source=${encodeURIComponent(player.backgroundSource)}&path=${encodeURIComponent(player.backgroundPath)}` : "";
    const poster = player.backgroundPosterPath ? `/api/image?path=${encodeURIComponent(player.backgroundPosterPath)}` : fallback;
    if (!poster) return "";
    const animation = player.backgroundVideoPath && !reduced?.matches ? `<video class="overview-art-focus" data-overview-video="${escape(player.backgroundVideoPath)}" muted loop playsinline preload="none" aria-hidden="true"></video>` : "";
    const focusID = player.backgroundSkinId || "";
    const composition = player.backgroundPosterPath?.toLowerCase().includes("_centered_") ? "centered" : "ordinary";
    return `<div class="summoner-strip-art overview-art" data-focus-skin="${escape(focusID)}" data-composition="${composition}" aria-hidden="true"><img class="overview-art-focus" data-overview-poster data-fallback="${escape(fallback)}" src="${escape(poster)}" alt="" decoding="async">${animation}</div>`;
  }
  function sync() {
    for (const [video, visible] of videos) {
      if (!video.isConnected) { video.pause(); video.removeAttribute("src"); observer?.unobserve(video); videos.delete(video); continue; }
      if (!visible || document.hidden || reduced?.matches) { video.pause(); if (reduced?.matches) video.classList.remove("is-playing"); continue; }
      if (!video.src) video.src = `/api/media?path=${encodeURIComponent(video.dataset.overviewVideo)}`;
      video.play().catch(() => {});
    }
  }
  const observer = typeof IntersectionObserver === "function" ? new IntersectionObserver(entries => {
    for (const entry of entries) videos.set(entry.target, entry.isIntersecting);
    sync();
  }) : null;
  function prepare(root) {
    for (const holder of holders) if (!holder.isConnected) { resize?.unobserve(holder); holders.delete(holder); }
    for (const image of root.querySelectorAll("[data-overview-poster]")) {
      if (image.dataset.bound) continue;
      image.dataset.bound = "1";
      const holder = image.closest(".overview-art");
      holders.add(holder); resize?.observe(holder);
      const loaded = () => { image.closest(".summoner-strip,.account-hero")?.classList.add("has-loaded-image"); position(holder); };
      image.addEventListener("load", loaded);
      image.addEventListener("error", () => {
        if (image.dataset.fallback && !image.dataset.retried) { image.dataset.retried = "1"; holder.dataset.composition = "ordinary"; image.removeAttribute("style"); image.src = image.dataset.fallback; }
      });
      if (image.complete && image.naturalWidth) loaded();
    }
    for (const video of root.querySelectorAll("[data-overview-video]")) {
      if (videos.has(video)) continue;
      videos.set(video, !observer); video.muted = true;
      video.addEventListener("playing", () => { position(video.closest(".overview-art")); video.closest(".summoner-strip,.account-hero")?.classList.add("has-loaded-image"); video.classList.add("is-playing"); });
      video.addEventListener("error", () => { video.classList.remove("is-playing"); videos.delete(video); observer?.unobserve(video); });
      observer?.observe(video);
    }
    sync();
  }
  document.addEventListener("visibilitychange", sync);
  reduced?.addEventListener("change", sync);
  window.deepLegendsOverviewArt = { render, prepare };
})();
