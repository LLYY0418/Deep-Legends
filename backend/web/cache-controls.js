(() => {
  "use strict";
  const controllers = new Set();
  window.deepLegendsSections.register("cache-controls", () => {
    const grid = document.querySelector("#settings-privacy-panel .settings-grid");
    if (!grid || grid.querySelector("[data-riot-cache-clear]")) return;
    const card = document.createElement("section");
    card.className = "setting-card setting-network-card setting-cache-card";
    card.innerHTML = '<div><h3>外服战绩缓存</h3></div><div class="setting-network-controls"><span class="inline-status" role="status" hidden></span><button class="text-button" type="button" data-riot-cache-clear>清除缓存</button></div>';
    grid.append(card);
    let fadeTimer,hideTimer;
    const button = card.querySelector("button"), status = card.querySelector("[role=status]");
    button.addEventListener("click", async () => {
      if (button.disabled) return;
      button.disabled = true;
      const controller = new AbortController();
      controllers.add(controller);
      const timeout = setTimeout(() => controller.abort(), 10000);
      try {
        const response = await fetch("/api/gameplay/cache/clear", {method:"POST", signal:controller.signal, headers:{"Accept":"application/json"}});
        if (!response.ok || !(await response.json()).cleared) throw new Error("failed");
        status.textContent = "缓存已清除";
      } catch (_) {
        status.textContent = "缓存清理失败，请重试";
      } finally {
        clearTimeout(timeout);
        controllers.delete(controller);
        button.disabled = false;status.hidden=false;status.style.opacity='1';clearTimeout(fadeTimer);clearTimeout(hideTimer);fadeTimer=setTimeout(()=>{status.style.opacity='0';hideTimer=setTimeout(()=>{status.hidden=true;status.textContent="";},200);},3000);
      }
    });
  });
  const dispose = () => { for (const controller of controllers) controller.abort(); controllers.clear(); };
  window.addEventListener("beforeunload", dispose, {once:true});
  window.addEventListener("deep-legends:dispose", dispose, {once:true});
})();
