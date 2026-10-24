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
  const journalKey="deep-legends-ui-pending-v1",unconfirmed=new Map();
  const saveJournal=()=>local.set(journalKey,unconfirmed.size?JSON.stringify(Object.fromEntries(unconfirmed)):null);
  try{const raw=local.get(journalKey)||"{}";if(raw.length<=128*1024){const rows=Object.entries(JSON.parse(raw));if(rows.length<=256)for(const [key,value]of rows)if(valid(key)&&(value===null||typeof value==="string"&&value.length<=8192)){unconfirmed.set(key,value);pending.set(key,value);}}}catch(_){}
  const acknowledge=batch=>{for(const [key,value]of Object.entries(batch))if(unconfirmed.has(key)&&unconfirmed.get(key)===value)unconfirmed.delete(key);saveJournal();};
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
    if (saving) {
      await saving;
      if (pending.size) return flush(keepalive);
      return;
    }
    if (!pending.size) return;
    const batch = Object.fromEntries(pending);
    pending.clear();
    const task = request({ mode: "update", values: batch }, keepalive).then(()=>acknowledge(batch)).catch(
      () => {
        for (const [k, v] of Object.entries(batch))
          if (!pending.has(k)&&unconfirmed.get(k)===v) pending.set(k, v);
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
        pending.set(key, value);unconfirmed.set(key,value);saveJournal();
        clearTimeout(timer);
        timer = setTimeout(() => void flush(), 300);
      }
    },
    removeItem(key) {
      values.set(key, null);
      local.set(key, null);
      if (valid(key)) {
        pending.set(key, null);unconfirmed.set(key,null);saveJournal();
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
      let result = Object.keys(migrated).length
        ? await request({ mode: "migrate", values: migrated })
        : saved;
      if(unconfirmed.size){const batch=Object.fromEntries(unconfirmed);result=await request({mode:"update",values:batch});acknowledge(batch);for(const key of Object.keys(batch))if(pending.get(key)===batch[key])pending.delete(key);}
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
