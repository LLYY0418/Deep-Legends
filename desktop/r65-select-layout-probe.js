// R65 real-browser acceptance probe.
// Paste this entire file into the browser console while the demo's 生涯 page is visible.
// It temporarily changes the selected hero/tier, restores both values, and returns numeric measurements.
(async () => {
  const frame = () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  const appScroll = document.querySelector(".app-scroll");
  const sync = (select) => window.deepLegendsSelects?.sync(select);
  const close = (root) => {
    const trigger = root.querySelector("[data-app-select-trigger]");
    if (trigger?.getAttribute("aria-expanded") === "true") trigger.click();
  };

  async function measure(select, temporaryValue) {
    if (!select) throw new Error("R65 probe: select not found");
    const originalValue = select.value;
    if (temporaryValue != null) {
      select.value = temporaryValue;
      sync(select);
    }
    const root = select._appSelectRoot || select.parentElement?.querySelector(":scope > .native-select-menu");
    const trigger = root?.querySelector("[data-app-select-trigger]");
    const menu = root?.querySelector("[data-app-select-menu]");
    const options = root?.querySelector("[data-app-select-options]");
    if (!root || !trigger || !menu || !options) throw new Error("R65 probe: enhanced select missing");
    close(root);
    const pageBefore = appScroll?.scrollTop ?? 0;
    trigger.click();
    await frame();
    const selected = options.querySelector('[role="menuitemradio"][aria-checked="true"]');
    const optionRect = options.getBoundingClientRect();
    const selectedRect = selected?.getBoundingClientRect();
    const centerDelta = selectedRect
      ? Math.abs((selectedRect.top + selectedRect.height / 2) - (optionRect.top + optionRect.height / 2))
      : Number.POSITIVE_INFINITY;
    const result = {
      optionCount: select.options.length,
      menuHeight: Number(menu.getBoundingClientRect().height.toFixed(2)),
      clientHeight: options.clientHeight,
      scrollHeight: options.scrollHeight,
      scrollTop: Number(options.scrollTop.toFixed(2)),
      selectedText: selected?.textContent?.trim() || "",
      selectedCenterDelta: Number(centerDelta.toFixed(2)),
      pageScrollBefore: pageBefore,
      pageScrollAfter: appScroll?.scrollTop ?? 0,
    };
    close(root);
    select.value = originalValue;
    sync(select);
    return result;
  }

  async function keyboardBounds(select) {
    const root = select._appSelectRoot || select.parentElement?.querySelector(":scope > .native-select-menu");
    const trigger = root.querySelector("[data-app-select-trigger]");
    const menu = root.querySelector("[data-app-select-menu]");
    const options = root.querySelector("[data-app-select-options]");
    close(root);
    trigger.focus({ preventScroll: true });
    trigger.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    await frame();
    document.activeElement?.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true }));
    await frame();
    const maximum = Math.max(0, options.scrollHeight - options.clientHeight);
    const result = {
      menuHeight: Number(menu.getBoundingClientRect().height.toFixed(2)),
      scrollTop: Number(options.scrollTop.toFixed(2)),
      maximum: Number(maximum.toFixed(2)),
      inBounds: options.scrollTop >= -1 && options.scrollTop <= maximum + 1,
    };
    close(root);
    return result;
  }

  const hero = document.querySelector("[data-facade-hero]");
  const tier = document.querySelector('[data-facade-rank="tier"]');
  const heroValue = hero?.options[Math.floor(hero.options.length / 2)]?.value;
  const heroResult = await measure(hero, heroValue);
  const masterResult = await measure(tier, "MASTER");
  const challengerResult = await measure(tier, "CHALLENGER");
  const heroKeyboard = await keyboardBounds(hero);
  const tierKeyboard = await keyboardBounds(tier);
  const failures = [];
  if (heroResult.optionCount <= 20) failures.push("hero optionCount must be > 20");
  if (heroResult.scrollHeight <= heroResult.clientHeight) failures.push("hero options must overflow and scroll");
  if (heroResult.menuHeight > 320.5 || masterResult.menuHeight > 320.5 || challengerResult.menuHeight > 320.5) failures.push("menu height exceeded 320px");
  if (heroResult.selectedCenterDelta > Math.max(24, heroResult.clientHeight * 0.2)) failures.push("selected hero is not near the center");
  if (!masterResult.selectedText.includes("大师")) failures.push("MASTER is not visible after opening");
  if (!challengerResult.selectedText.includes("最强王者")) failures.push("CHALLENGER is not visible after opening");
  if (heroResult.pageScrollBefore !== heroResult.pageScrollAfter) failures.push("opening hero select changed .app-scroll.scrollTop");
  if (!heroKeyboard.inBounds || !tierKeyboard.inBounds) failures.push("keyboard navigation overflowed its options viewport");
  const report = { hero: heroResult, master: masterResult, challenger: challengerResult, keyboard: { hero: heroKeyboard, tier: tierKeyboard }, failures };
  if (failures.length) throw new Error(`R65 select probe failed: ${JSON.stringify(report)}`);
  return report;
})();
