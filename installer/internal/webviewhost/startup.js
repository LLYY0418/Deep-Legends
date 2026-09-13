(function () {
  "use strict";
  const send = (type, detail) => window.chrome.webview.postMessage(JSON.stringify({ type, detail }));
  window.addEventListener("error", event => send("shell-error", `安装页面脚本执行失败（${event.lineno || 0}:${event.colno || 0}）`));
  window.addEventListener("unhandledrejection", () => send("shell-error", "安装页面初始化失败"));
  window.addEventListener("DOMContentLoaded", () => {
    if (!window.host || typeof window.host.init !== "function" || !window.__INIT__ ||
        !document.querySelector(".shell") || !document.querySelector(".page.on")) {
      send("shell-error", "安装页面未完成初始化");
      return;
    }
    send("shell-ready", "");
  }, { once: true });
})();
