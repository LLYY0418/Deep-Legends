"use strict";
const { Script } = require("node:vm");
// Cache compiled code only: every execution uses the new window's own context.
// Final source is the key so source-overlay and in-process mutants stay distinct.
const scripts = new Map();
function runRendererScript(dom, source, filename) {
  const key = filename + "\0" + source;
  let script = scripts.get(key);
  if (!script) {
    script = new Script(source, { filename });
    if (scripts.size >= 32) scripts.delete(scripts.keys().next().value);
    scripts.set(key, script);
  }
  return script.runInContext(dom.getInternalVMContext());
}
module.exports = { runRendererScript };
