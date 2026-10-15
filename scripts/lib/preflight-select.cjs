"use strict";
// R279 P2: change-aware test selection for the LOCAL quick preflight.
//
// This module only decides WHICH tests are related to a set of changed files.
// It never replaces CI: the result is an accelerator for the edit/fix loop and
// must not be used as release evidence. Anything it cannot map is reported as
// "unmapped" and the caller is told to push a probe branch instead.
//
// Pure functions: all file access goes through the `files` / `read` arguments
// so the rules can be unit-tested against synthetic repositories.

const path = require("node:path");
const posix = path.posix;

const NODE_TEST_RE = /^(?:backend\/web|desktop|scripts)\/[^/]+\.test\.cjs$|^relay\/riot-worker\/[^/]+\.test\.mjs$/;
const GO_SOURCE_RE = /^backend\/[^/]+\.go$/;
const GO_TEST_RE = /^backend\/[^/]+_test\.go$/;
const FILE_TOKEN_RE = /[A-Za-z0-9_.-]+\.(?:cjs|mjs|js|css|html|json|go|py|ps1|sh|yml|yaml|md|nsi|nsh|ico|woff2)\b/g;
const ASSET_EXT_RE = /\.(?:js|css|html|json|woff2|ico|png|svg)$/;
// Real-browser guards (named exactly as in scripts/r276-browser-catalog.cjs) plus the R277 strict matrix.
// They reach the product through a running backend, so no source file name ever appears in them.
const R277_STRICT = "r277-strict";
const WINDOWS_ONLY_NOTE = "windows-upgrade（真实安装升级链，只能在 CI 的 Windows job 验证）";
const GO_STOPWORDS = new Set([
  "String", "Error", "Close", "Write", "Read", "Value", "Values", "Name", "Names", "Type", "Types", "State", "Status",
  "Handler", "Config", "Result", "Request", "Response", "Context", "Server", "Client", "Options", "Option", "Init",
  "Start", "Stop", "Reset", "Format", "Parse", "Build", "Start", "Clone", "Equal", "Empty", "Match", "Check",
]);

function normalize(file) {
  return String(file).replaceAll("\\", "/").replace(/^\.\//, "");
}

function resolveRequire(from, request, fileSet) {
  const base = posix.normalize(posix.join(posix.dirname(from), request));
  for (const candidate of [base, `${base}.cjs`, `${base}.js`, `${base}.mjs`, `${base}.json`, `${base}/index.cjs`, `${base}/index.js`]) {
    if (fileSet.has(candidate)) return candidate;
  }
  return null;
}

function fileTokens(text) {
  return new Set(text.match(FILE_TOKEN_RE) || []);
}

// Build once per run. `files` are repo-relative posix paths; `read(file)` returns text.
function buildIndex({ files, read, guards = [] }) {
  const all = files.map(normalize);
  const fileSet = new Set(all);
  const byBase = new Map();
  for (const file of all) {
    const base = posix.basename(file);
    if (!byBase.has(base)) byBase.set(base, []);
    byBase.get(base).push(file);
  }
  const textCache = new Map();
  const text = (file) => {
    if (!textCache.has(file)) {
      let value = "";
      try { value = read(file) || ""; } catch (_) { value = ""; }
      textCache.set(file, value);
    }
    return textCache.get(file);
  };

  const requireGraph = new Map();
  const requiresOf = (file) => {
    if (requireGraph.has(file)) return requireGraph.get(file);
    const found = new Set();
    requireGraph.set(file, found);
    if (/\.(?:cjs|js|mjs)$/.test(file)) {
      const source = text(file);
      for (const match of source.matchAll(/require\(\s*['"](\.{1,2}\/[^'"]+)['"]\s*\)|from\s+['"](\.{1,2}\/[^'"]+)['"]/g)) {
        const resolved = resolveRequire(file, match[1] || match[2], fileSet);
        if (resolved) found.add(resolved);
      }
    }
    return found;
  };
  // Code dependencies reachable through require(), depth-limited and cycle-safe.
  const closure = (file, depth = 3) => {
    const seen = new Set([file]);
    let frontier = [file];
    for (let level = 0; level < depth; level++) {
      const next = [];
      for (const current of frontier) {
        for (const dependency of requiresOf(current)) {
          if (!seen.has(dependency)) { seen.add(dependency); next.push(dependency); }
        }
      }
      frontier = next;
    }
    return seen;
  };

  const nodeTests = all.filter((file) => NODE_TEST_RE.test(file));
  // referencedBy: repo file -> node tests that mention it (own text) or whose required helper mentions it.
  const referencedBy = new Map();
  const note = (target, test, via) => {
    if (target === test) return;
    if (!referencedBy.has(target)) referencedBy.set(target, new Map());
    const reasons = referencedBy.get(target);
    if (!reasons.has(test)) reasons.set(test, via);
  };
  for (const test of nodeTests) {
    for (const member of closure(test)) {
      const tokens = fileTokens(text(member));
      const via = member === test ? "测试内直接引用" : `经 ${member} 间接引用`;
      for (const token of tokens) {
        for (const target of byBase.get(token) || []) note(target, test, via);
      }
      if (member !== test) note(member, test, "require 依赖");
    }
  }

  const goSources = all.filter((file) => GO_SOURCE_RE.test(file) && !GO_TEST_RE.test(file));
  const goTests = all.filter((file) => GO_TEST_RE.test(file));
  const declared = new Map();
  for (const file of goSources) declared.set(file, goDeclarations(text(file)));
  const goTestIdentifiers = new Map();
  const goTestNames = new Map();
  for (const file of goTests) {
    const source = text(file);
    goTestIdentifiers.set(file, new Set(source.match(/[A-Za-z_]\w{4,}/g) || []));
    goTestNames.set(file, [...source.matchAll(/^func (Test\w+)\(/gm)].map((m) => m[1]));
  }

  // Non-test scripts (any language) that mention a file: shown as related, not auto-run.
  const scriptFiles = all.filter((file) => (/^(?:scripts|desktop|backend\/web)\/[^/]+\.(?:cjs|js|mjs|py|ps1)$/.test(file) || /^scripts\/[^/]+\/[^/]+\.go$/.test(file)) && !NODE_TEST_RE.test(file));
  const scriptRefs = new Map();
  for (const script of scriptFiles) {
    for (const token of fileTokens(text(script))) {
      for (const target of byBase.get(token) || []) {
        if (target === script) continue;
        if (!scriptRefs.has(target)) scriptRefs.set(target, new Set());
        scriptRefs.get(target).add(script);
      }
    }
  }

  // Files that CI workflows run directly (python/node scripts, fixtures). They cannot be replayed
  // cheaply here, so a change to one is reported as "CI-only" instead of being silently unmapped.
  const workflowRefs = new Map();
  for (const workflow of all.filter((file) => /^\.github\/workflows\/[^/]+\.ya?ml$/.test(file))) {
    for (const token of fileTokens(text(workflow))) {
      for (const target of byBase.get(token) || []) {
        if (!workflowRefs.has(target)) workflowRefs.set(target, new Set());
        workflowRefs.get(target).add(posix.basename(workflow));
      }
    }
  }

  return { all, fileSet, nodeTests, referencedBy, declared, goTests, goTestIdentifiers, goTestNames, scriptRefs, workflowRefs, guards, text };
}

function goDeclarations(source) {
  const names = new Set();
  // Plain functions are matched by name. Methods are matched by their receiver type only:
  // method names such as available()/accepts() are local to the type and would match
  // unrelated test files that merely use the same word.
  for (const m of source.matchAll(/^func ([A-Za-z_]\w*)\s*[\[(]/gm)) names.add(m[1]);
  for (const m of source.matchAll(/^func \(\w+\s+\*?([A-Za-z_]\w*)(?:\[[^\]]*\])?\)\s*[A-Za-z_]\w*\s*[\[(]/gm)) names.add(m[1]);
  for (const m of source.matchAll(/^type ([A-Za-z_]\w*)\b/gm)) names.add(m[1]);
  for (const m of source.matchAll(/^(?:var|const)\s+([A-Za-z_]\w*)\b/gm)) names.add(m[1]);
  for (const block of source.matchAll(/^(?:var|const)\s*\(([\s\S]*?)^\)/gm)) {
    for (const m of block[1].matchAll(/^\t([A-Za-z_]\w*)\b/gm)) names.add(m[1]);
  }
  return [...names].filter((name) => name.length >= 5 && !GO_STOPWORDS.has(name));
}

function isProductWeb(file) {
  return file.startsWith("backend/web/") && !NODE_TEST_RE.test(file) && !/\.test\.[cm]?js$/.test(file);
}
function isDesktopProduct(file) {
  return file.startsWith("desktop/") && !NODE_TEST_RE.test(file) && !file.includes("node_modules/") && !/\.test\.[cm]?js$/.test(file);
}

// changes: [{file, status}] with status A|M|D|R (default M)
function select(index, rawChanges, options = {}) {
  const changes = rawChanges.map((c) => (typeof c === "string" ? { file: normalize(c), status: "M" } : { file: normalize(c.file), status: c.status || "M" }));
  const broadRatio = options.broadRatio ?? 0.6;
  const nodeSelected = new Map();
  const addNode = (file, reason) => {
    if (!nodeSelected.has(file)) nodeSelected.set(file, []);
    if (!nodeSelected.get(file).includes(reason)) nodeSelected.get(file).push(reason);
  };
  const guards = new Map();
  const addGuard = (name, reason) => { if (!guards.has(name)) guards.set(name, reason); };
  const goFiles = new Map();
  const addGoFile = (file, reason) => { if (!goFiles.has(file)) goFiles.set(file, reason); };
  const related = new Map();
  const windowsOnly = new Set();
  const ciOnly = new Map();
  const unmapped = [];
  const staticGates = new Set(["verify-no-docs-tracked"]);
  let webLayerTouched = false;
  let docsOnly = true;

  for (const { file, status } of changes) {
    const reasonsBefore = nodeSelected.size + goFiles.size + guards.size;
    let covered = false;
    const isDoc = /\.md$/.test(file);
    if (!isDoc) docsOnly = false;

    if (NODE_TEST_RE.test(file)) { addNode(file, "测试文件本身被改动"); covered = true; }
    for (const [test, via] of index.referencedBy.get(file) || []) { addNode(test, `引用了 ${file}（${via}）`); covered = true; }
    for (const script of index.scriptRefs.get(file) || []) {
      if (!related.has(script)) related.set(script, []);
      related.get(script).push(file);
      covered = true;
    }

    if (isProductWeb(file) || isDesktopProduct(file)) {
      webLayerTouched = true;
      covered = true;
      for (const guard of index.guards) addGuard(guard, `前端/桌面文件 ${file} 被改动：浏览器层通过运行中的后端取资源，无法靠文件名关联`);
      addGuard(R277_STRICT, `前端/桌面文件 ${file} 被改动：R277 严格几何矩阵`);
      windowsOnly.add(WINDOWS_ONLY_NOTE);
      if (isProductWeb(file) && ASSET_EXT_RE.test(file) && (status === "A" || status === "D" || status === "R")) {
        staticGates.add("go-vet-backend");
        for (const test of index.goTests) {
          if (/embed|Embedded|Asset/.test(test) && /web/.test(index.text(test))) addGoFile(test, `Web 资源 ${file} 新增/删除/改名：嵌入资产清单类 Go 测试`);
        }
      }
    }
    if (file.startsWith("installer/")) { windowsOnly.add(WINDOWS_ONLY_NOTE); covered = true; }
    if (index.workflowRefs && index.workflowRefs.has(file) && file.startsWith("scripts/") && !NODE_TEST_RE.test(file) && !index.referencedBy.has(file)) {
      ciOnly.set(file, [...index.workflowRefs.get(file)]);
      covered = true;
    }

    if (GO_TEST_RE.test(file)) { addGoFile(file, "Go 测试文件本身被改动"); covered = true; }
    if (GO_SOURCE_RE.test(file) && !GO_TEST_RE.test(file)) {
      covered = true;
      staticGates.add("go-vet-backend");
      const sibling = file.replace(/\.go$/, "_test.go");
      if (index.fileSet.has(sibling)) addGoFile(sibling, `同名测试 ${sibling}`);
      const names = index.declared.get(file) || (index.text ? goDeclarations(index.text(file)) : []);
      const wanted = new Set(names);
      for (const test of index.goTests) {
        const identifiers = index.goTestIdentifiers.get(test);
        const shared = [...wanted].find((name) => identifiers.has(name));
        if (shared) addGoFile(test, `引用了 ${file} 中定义的 ${shared}`);
      }
    }
    if (file.startsWith(".github/workflows/")) { staticGates.add("workflow-contracts"); covered = true; }
    if (/\.(?:cjs|js|mjs)$/.test(file)) { staticGates.add("node-check"); }
    if (/\.go$/.test(file)) staticGates.add("gofmt");

    const gained = nodeSelected.size + goFiles.size + guards.size > reasonsBefore;
    if (!covered && !gained) unmapped.push(file);
  }

  const goTestNames = [];
  for (const file of goFiles.keys()) goTestNames.push(...(index.goTestNames.get(file) || []));
  const totalNode = index.nodeTests.length || 1;
  const ratio = nodeSelected.size / totalNode;
  const broad = ratio > broadRatio;
  const onlyDocs = docsOnly && changes.length > 0;
  // Doc-only changes with no referencing gate are fine; do not call them "unmapped".
  const reallyUnmapped = onlyDocs ? [] : unmapped.filter((file) => !(/\.md$/.test(file)));

  const scopeWarnings=changes.some(c=>/^(backend\/web\/(app\.(js|css)|index.html)|desktop\/main.cjs)$/.test(c.file))&&changes.some(c=>/^(backend\/web\/(app.js|gameplay.js|runtime.js)|desktop\/main.cjs)$/.test(c.file))?["布局与设置代码可能同时受影响：建议独立提交，按实际diff核验；不强制拆版本"]:[];
  if(changes.some(c=>c.file.startsWith("scripts/")||c.file.startsWith(".github/workflows/")))staticGates.add("pinned-contracts");
  return {
    scopeWarnings,
    changed: changes,
    nodeTests: [...nodeSelected].map(([file, reasons]) => ({ file, reasons })).sort((a, b) => a.file.localeCompare(b.file)),
    goTestFiles: [...goFiles].map(([file, reason]) => ({ file, reason })).sort((a, b) => a.file.localeCompare(b.file)),
    goTestNames: [...new Set(goTestNames)].sort(),
    browserGuards: [...guards].map(([name, reason]) => ({ name, reason })),
    relatedScripts: [...related].map(([file, targets]) => ({ file, because: [...new Set(targets)] })).sort((a, b) => a.file.localeCompare(b.file)),
    windowsOnly: [...windowsOnly],
    ciOnly: [...ciOnly].map(([file, workflows]) => ({ file, workflows })).sort((a, b) => a.file.localeCompare(b.file)),
    staticGates: [...staticGates].sort(),
    unmapped: reallyUnmapped,
    webLayerTouched,
    broad,
    ratio: Number(ratio.toFixed(3)),
    recommendProbe: broad || reallyUnmapped.length > 0 || ciOnly.size > 0,
  };
}

module.exports = { buildIndex, select, goDeclarations, normalize, NODE_TEST_RE, GO_SOURCE_RE, GO_TEST_RE, R277_STRICT, WINDOWS_ONLY_NOTE };
