(() => {
  "use strict";
  const root=document.documentElement;
  root.classList.add("appearance-boot");
  let cache={};
  try { cache=JSON.parse(localStorage.getItem("deep-legends-appearance-v1")||"{}"); } catch (_) {}
  root.dataset.theme=["light","dark","azure","emerald","violet","crimson","aurora","oled"].includes(cache.theme)?cache.theme:"dark";
  root.dataset.density=cache.density==="compact"?"compact":"comfortable";
  document.addEventListener("DOMContentLoaded",()=>requestAnimationFrame(()=>requestAnimationFrame(()=>root.classList.remove("appearance-boot"))),{once:true});
})();
