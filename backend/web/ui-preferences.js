(() => {
  "use strict";
  const values = new Map(),
    pending = new Map();
  const valid = (key) =>
    /^(?:lol-loot-(?!demo)[a-z0-9-]{1,64}|deep-legends-announcements-(?:read|dismissed)-v1)$/.test(
      key,
    );
  let timer = 0,
    saving = null;
  const local = {
    get(key) {
      try {
        return localStorage.getItem(key);
      } catch (_) {
        return null;
      }
    },
    set(key, value) {
      try {
        value === null
          ? localStorage.removeItem(key)
          : localStorage.setItem(key, value);
      } catch (_) {}
    },
  };
  async function request(body, keepalive = false) {
    const response = await fetch("/api/ui-preferences", {
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      ...(body
        ? { method: "POST", body: JSON.stringify(body), keepalive }
        : {}),
      signal: keepalive ? undefined : AbortSignal.timeout(4000),
    });
    if (!response.ok) throw Error("偏好保存失败");
    return response.json();
  }
  async function flush(keepalive = false) {
    clearTimeout(timer);
    timer = 0;
    if (saving && !keepalive) {
      await saving;
      if (pending.size) return flush();
      return;
    }
    if (!pending.size) return;
    const batch = Object.fromEntries(pending);
    pending.clear();
    const task = request({ mode: "update", values: batch }, keepalive).catch(
      () => {
        for (const [k, v] of Object.entries(batch))
          if (!pending.has(k)) pending.set(k, v);
      },
    );
    saving = task;
    await task;
    if (saving === task) saving = null;
  }
  const storage = {
    getItem(key) {
      return values.has(key) ? values.get(key) : local.get(key);
    },
    setItem(key, value) {
      value = String(value);
      values.set(key, value);
      local.set(key, value);
      if (valid(key)) {
        pending.set(key, value);
        clearTimeout(timer);
        timer = setTimeout(() => void flush(), 300);
      }
    },
    removeItem(key) {
      values.set(key, null);
      local.set(key, null);
      if (valid(key)) {
        pending.set(key, null);
        clearTimeout(timer);
        timer = setTimeout(() => void flush(), 300);
      }
    },
    flush,
  };
  window.deepLegendsPreferenceStorage = storage;
  document.documentElement.dataset.uiPreferences = "pending";
  storage.ready = (async () => {
    try {
      const saved = await request();
      const migrated = {};
      try {
        for (let i = 0; i < localStorage.length; i++) {
          const key = localStorage.key(i);
          if (valid(key) && !Object.hasOwn(saved.values || {}, key))
            migrated[key] = local.get(key);
        }
      } catch (_) {}
      const result = Object.keys(migrated).length
        ? await request({ mode: "migrate", values: migrated })
        : saved;
      for (const [key, value] of Object.entries(result.values || {})) {
        if (valid(key)) {
          values.set(key, value);
          local.set(key, value);
        }
      }
    } catch (_) {
    } finally {
      delete document.documentElement.dataset.uiPreferences;
    }
  })();
  for (const event of ["pagehide", "beforeunload"])
    window.addEventListener(event, () => void flush(true));
})();
