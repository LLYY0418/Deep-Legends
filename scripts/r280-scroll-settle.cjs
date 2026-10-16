'use strict';
const { waitFor } = require('./lib/wait-progress.cjs');
const expression = "new Promise(resolve=>requestAnimationFrame(()=>{const root=document.getElementById('app-scroll');resolve({top:root.scrollTop,height:root.scrollHeight,rows:[...document.querySelectorAll('.match-entry')].slice(0,5).map(n=>({collapsed:n.hasAttribute('data-virtual-match'),height:n.getBoundingClientRect().height}))})}))";
async function waitForScrollSettlement(evaluate, options = {}) {
  let previous = '', stableFrames = 0, last = null;
  await waitFor(async () => {
    last = await evaluate(expression);
    const snapshot = JSON.stringify(last);
    stableFrames = snapshot === previous ? stableFrames + 1 : 1;
    previous = snapshot;
    return last.rows.length === 5 && last.rows.every(row => row.collapsed) && stableFrames >= 3;
  }, { idleMs: 15000, hardMs: 60000, ...options, label: 'R280 scroll virtualization settle', progress: () => last });
}
module.exports = { waitForScrollSettlement };
