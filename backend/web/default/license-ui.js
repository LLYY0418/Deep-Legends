(() => {
  "use strict";
  const frame = document.getElementById("app-frame");
  frame.hidden = false;
  frame.removeAttribute("inert");
  document.documentElement.dataset.license = "disabled";
  let started = false;
  window.deepLegendsLicense = Object.freeze({ isActive: () => true, poll() {
    if (started) return;
    started = true;
    window.dispatchEvent(new CustomEvent("deep-legends:license", { detail: { active: true, generation: 0 } }));
  } });
})();
