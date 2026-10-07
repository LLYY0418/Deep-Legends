"use strict";

const LICENSE_WIDTH = 860, LICENSE_HEIGHT = 580;
function lockedWindowBounds(area) {
  return { width: LICENSE_WIDTH, height: LICENSE_HEIGHT,
    x: Math.round(area.x + (area.width - LICENSE_WIDTH) / 2),
    y: Math.round(area.y + (area.height - LICENSE_HEIGHT) / 2) };
}
function waitForWindowEvent(window, name, change, timeout = 600, onError = () => {}) {
  return new Promise(resolve => {
    let timer, done = false;
    const finish = timedOut => { if (done) return; done = true; clearTimeout(timer); window.removeListener(name, onEvent); resolve(timedOut); };
    const onEvent = () => finish(false);
    window.once(name, onEvent); timer = setTimeout(() => finish(true), timeout);
    try { change(); } catch (error) { onError(error); finish(true); }
  });
}
const normalSize = bounds => Number.isFinite(bounds.width) && Number.isFinite(bounds.height) && bounds.width >= 780 && bounds.height >= 600;
// Electron Rectangle values and persisted DIP geometry use integer coordinates.
const nativeBounds = bounds => Object.fromEntries(["x", "y", "width", "height"].map(key => [key, Math.round(bounds[key])]));
const validRestore = bounds => bounds && normalSize(bounds) && Object.values(nativeBounds(bounds)).every(value => Number.isSafeInteger(value) && Math.abs(value) <= 1000000);
const errorType = error => ["Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"].includes(error?.name) ? error.name : "Error";
const nextNativeTurn = () => new Promise(resolve => setImmediate(resolve));
const sizeMatches = (actual, wanted) => Math.abs(actual.width - wanted.width) <= 2 && Math.abs(actual.height - wanted.height) <= 2;

// Opacity preserves visibility/focus and lets hidden render frames keep running.
// No geometry change is visible. The adapter has no Electron dependency.
function createLicenseWindowController({ window, normalBounds, getWorkArea, getDisplayScale = () => 1, onScale,
  getInitialBounds = () => normalBounds, writeBounds, waitForRender = async () => ({}), recordState = () => {}, eventTimeout = 600, renderTimeout = 1500 }) {
  let active = null, state = "PENDING", switching = false, restore = { ...window.getBounds(), ...normalBounds }, running = null, queued = null;
  const snapshot = () => {
    const bounds = window.isMaximized() || window.isFullScreen() || window.isMinimized?.() ? window.getNormalBounds() : window.getBounds();
    return { ...nativeBounds(validRestore(bounds) ? bounds : restore), maximized: window.isMaximized(), fullscreen: window.isFullScreen() };
  };
  const requestedContentBounds = target => {
    if (target !== "ACTIVE") return lockedWindowBounds(getWorkArea());
    // ACTIVE geometry is measured after applying the normal outer rectangle.
    // The locked frame can have different native insets (DPI/titlebar changes).
    // Subtracting those stale insets from the saved rectangle can yield NaN or
    // nonpositive target sizes even though the saved outer bounds are valid.
    return nativeBounds(window.getContentBounds());
  };
  async function transition(target, payload) {
    if (window.isDestroyed()) return null;
    const started = Date.now(), from = state, next = target === "ACTIVE", geometryChanged = active !== next;
    const record = { fromState: from, toState: target, wasMaximized: window.isMaximized(), wasFullscreen: window.isFullScreen(),
      displayScale: getDisplayScale(), statusReadMs: payload.statusReadMs || 0, retried: false, waitTimedOut: false, renderTimedOut: false, renderMismatch: false, sizeMismatch: false, fallback_used: false, native_error: null };
    const nativeError = error => { record.native_error = errorType(error); };
    const call = change => { try { change(); return true; } catch (error) { nativeError(error); return false; } };
    const eventChange = (name, change) => waitForWindowEvent(window, name, change, eventTimeout, nativeError);
    if (active === true && !next) restore = snapshot();
    active = next; state = target; switching = true;
    // This must precede every native geometry call, including unmaximize.
    call(() => window.setOpacity(0));
    try {
      record.requestedContent = requestedContentBounds(target);
      if (geometryChanged) {
        call(() => window.setMinimumSize(Math.round(0), Math.round(0))); call(() => window.setResizable(true));
        if (next) {
          call(() => window.setMaximizable(true));
          if (!validRestore(restore)) { restore = { ...window.getBounds(), ...getInitialBounds() }; record.fallback_used = true; }
          call(() => window.setBounds(nativeBounds(restore)));
          if (restore.maximized) call(() => window.maximize());
          if (restore.fullscreen) call(() => window.setFullScreen(true));
        } else {
          if (window.isFullScreen()) record.waitTimedOut = await eventChange("leave-full-screen", () => window.setFullScreen(false));
          if (window.isDestroyed()) return null;
          if (window.isMaximized()) record.waitTimedOut = await eventChange("unmaximize", () => window.unmaximize()) || record.waitTimedOut;
          if (window.isDestroyed()) return null;
          call(() => window.setContentBounds(nativeBounds(record.requestedContent)));
        }
        call(() => onScale(next));
      }
      await nextNativeTurn();
      if (window.isDestroyed()) return null;
      let usedInitial = false;
      const verifyActiveSize = async () => {
        const matchesRestore = () => {
          if (!normalSize(window.isMinimized?.() ? window.getNormalBounds() : window.getContentBounds())) return false;
          if (window.isMinimized?.()) return sizeMatches(window.getNormalBounds(), nativeBounds(restore)); // Some native platforms expose only normal bounds while minimized.
          if (restore.maximized && !window.isMaximized() || restore.fullscreen && !window.isFullScreen()) return false;
          const actual = window.getNormalBounds(), wanted = nativeBounds(restore);
          return sizeMatches(actual, wanted) && Math.abs(actual.x-wanted.x)<=2 && Math.abs(actual.y-wanted.y)<=2;
        };
        if (!next || (usedInitial ? normalSize(window.getContentBounds()) : matchesRestore()) && (!record.native_error || record.fallback_used)) return;
        record.fallback_used = true;
        call(() => window.setMinimumSize(Math.round(0), Math.round(0)));
        const applyRestore = () => {
          const ok = call(() => window.setBounds(nativeBounds(restore)));
          if (restore.maximized) call(() => window.maximize());
          if (restore.fullscreen) call(() => window.setFullScreen(true));
          return ok;
        };
        const restored = applyRestore(); await nextNativeTurn();
        if (!restored || !matchesRestore()) {
          usedInitial = true;
          const initial = nativeBounds({ ...window.getBounds(), ...getInitialBounds() });
          if (window.isFullScreen()) await eventChange("leave-full-screen", () => window.setFullScreen(false));
          if (window.isMaximized()) await eventChange("unmaximize", () => window.unmaximize());
          call(() => window.setBounds(initial)); await nextNativeTurn();
        }
        call(() => onScale(true));
      };
      await verifyActiveSize();
      if (next) record.requestedContent = requestedContentBounds(target);
      const verifyLockedSize = async () => {
        if (!next && !sizeMatches(window.getContentBounds(), record.requestedContent) && !record.retried) {
          record.retried = true; call(() => window.setContentBounds(nativeBounds(record.requestedContent))); await nextNativeTurn();
        }
      };
      await verifyLockedSize();
      record.nativeMs = Date.now() - started;
      const renderStarted = Date.now();
      let timer;
      const rendered = await Promise.race([
        Promise.resolve().then(() => waitForRender(target, payload)).catch(() => ({ timedOut: true })),
        new Promise(resolve => { timer = setTimeout(() => resolve({ timedOut: true }), renderTimeout); }),
      ]).finally(() => clearTimeout(timer));
      if (window.isDestroyed()) return null;
      record.renderMs = Date.now() - renderStarted;
      record.renderTimedOut = rendered?.timedOut === true; record.renderMismatch = rendered?.mismatch === true;
      // Check again after rendering: a late OS restore must not undo the size.
      await verifyLockedSize();
      await verifyActiveSize();
      if (!next) { call(() => window.setMaximizable(false)); call(() => window.setResizable(false)); }
      record.actualContent = nativeBounds(window.getContentBounds()); record.elapsedMs = Date.now() - started;
      record.sizeMismatch = next ? !normalSize(record.actualContent) : !sizeMatches(record.actualContent, record.requestedContent);
      if (!rendered?.stale && !queued) call(() => window.setOpacity(1));
      recordState(record);
      return record;
    } finally { switching = false; }
  }
  function setState(target, payload = {}) {
    if (running) {
      if (state === target && !queued) return running;
      return new Promise(resolve => { if (queued) queued.resolve(null); queued = { target, payload, resolve }; });
    }
    running = transition(target, payload).finally(() => {
      running = null;
      if (queued) { const next = queued; queued = null; setState(next.target, next.payload).then(next.resolve); }
    });
    return running;
  }
  return { isActive: () => active === true, isResolved: () => active !== null, canShow: () => active !== null && !switching,
    requestedContentBounds, setState,
    showInitial() { if (!switching && active !== null && !window.isDestroyed()) { window.setOpacity(1); window.show(); return true; } return false; },
    persistBounds() { if (active !== true || switching || window.isDestroyed()) return false; return writeBounds(snapshot()); },
  };
}
module.exports = { LICENSE_WIDTH, LICENSE_HEIGHT, lockedWindowBounds, waitForWindowEvent, sizeMatches, createLicenseWindowController };
