"use strict";

const fs = require("node:fs"), path = require("node:path"), vm = require("node:vm");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");
const requiredWindowsBackendNames = ["TestR204KeySaveAndClear", "TestR204KeyRuntime401AndPrivacy", "TestSplitRegistryPathSupportsNativeTencentKeys"];

function goTests(directory, project = root) {
  const result = spawnSync("go", ["run", path.join(__dirname, "ci-test-inventory.go"), directory], {
    cwd: project, encoding: "utf8", timeout: 60000, maxBuffer: 16 * 1024 * 1024,
  });
  if (result.error || result.status !== 0) throw new Error(result.error?.message || result.stderr);
  return JSON.parse(result.stdout);
}

function nodeTests(file, project = root) {
  const source = fs.readFileSync(path.resolve(project, file), "utf8");
  // Tokenize comments and quoted/template source fixtures as single tokens, so
  // a commented-out declaration or a synthetic source string cannot satisfy CI.
  // Only literal quoted test names are accepted; dynamic names fail closed.
  const tokens = [];
  for (let index = 0; index < source.length;) {
    const start = index, char = source[index];
    if (/\s/.test(char)) { index++; continue; }
    if (source.startsWith("//", index)) { index = source.indexOf("\n", index); if (index < 0) break; continue; }
    if (source.startsWith("/*", index)) { const end = source.indexOf("*/", index + 2); if (end < 0) throw new Error(`unterminated comment: ${file}`); index = end + 2; continue; }
    if (char === "/" && (!tokens.length || ["=", "(", "[", ",", ":", ";", "!", "?", "{", "return"].includes(tokens.at(-1).text))) {
      index++;
      let bracket = false;
      while (index < source.length) {
        if (source[index] === "\\") { index += 2; continue; }
        if (source[index] === "[") bracket = true;
        if (source[index] === "]") bracket = false;
        if (source[index] === "/" && !bracket) break;
        index++;
      }
      if (index >= source.length) throw new Error(`unterminated regexp: ${file}`);
      index++;
      while (/[a-z]/i.test(source[index] || "0")) index++;
      tokens.push({ text: "regexp" });
      continue;
    }
    if (['"', "'", "`"].includes(char)) {
      index++;
      while (index < source.length && source[index] !== char) { if (source[index] === "\\") index++; index++; }
      if (index >= source.length) throw new Error(`unterminated string: ${file}`);
      index++;
      tokens.push({ text: source.slice(start, index), literal: char !== "`" });
    } else if (/[\w$]/.test(char)) {
      while (index < source.length && /[\w$]/.test(source[index])) index++;
      tokens.push({ text: source.slice(start, index) });
    } else { tokens.push({ text: char }); index++; }
  }
  const declarations = [];
  for (let index = 0; index < tokens.length; index++) {
    if (!["test", "it"].includes(tokens[index].text)) continue;
    let next = index + 1;
    if (tokens[next]?.text === "." && ["only", "skip", "todo"].includes(tokens[next + 1]?.text)) next += 2;
    if (tokens[next]?.text !== "(" || !tokens[next + 1]?.literal) continue;
    declarations.push({ name: vm.runInNewContext(tokens[next + 1].text, Object.create(null), { timeout: 100, codeGeneration: { strings: false, wasm: false } }), file });
  }
  return declarations;
}

function selectTests(pattern, declarations, kind) {
  // CI filters are simple alternations. Check EVERY arm, rather than merely
  // proving that the whole regexp matches one surviving test.
  let alternatives = pattern;
  if (kind === "go" && alternatives.startsWith("^(") && alternatives.endsWith(")$")) alternatives = alternatives.slice(2, -2);
  const arms = alternatives.split("|");
  for (const arm of arms) {
    if (!arm) throw new Error(`empty ${kind} filter arm: ${pattern}`);
    const regexp = new RegExp(kind === "go" ? `^(?:${arm})$` : arm);
    if (!declarations.some(test => regexp.test(test.name))) throw new Error(`unmatched ${kind} test filter: ${arm}`);
  }
  const regexp = new RegExp(pattern);
  const selected = declarations.filter(test => regexp.test(test.name));
  if (!selected.length) throw new Error(`empty ${kind} test selection: ${pattern}`);
  return selected;
}

function workflowFilters(source) {
  const filters = [];
  let directory = ".";
  for (const original of source.split(/\r?\n/)) {
    const line = original.trim();
    if (line.startsWith("#")) continue;
    if (/^- (?:name:|uses:)/.test(line) || /^\w[\w-]*:$/.test(line)) directory = ".";
    const location = line.match(/^(?:working-directory:|Push-Location)\s+([\w./-]+)$/);
    if (location) directory = location[1];
    if (/^Pop-Location\b|finally \{ Pop-Location/.test(line)) directory = ".";
    if (/\bgo test\b/.test(line) && /(?:^|\s)-run(?:[=\s])/.test(line)) {
      const filter = line.match(/-run(?:=|\s+)(?:'([^']+)'|"([^"]+)"|([^\s]+))/);
      const target = line.match(/\bgo test(?:\s+-[^\s]+)*\s+(\.\/[\w./-]+|\.)(?=\s|$)/);
      if (!filter || !target) throw new Error(`cannot parse Go filter/package: ${line}`);
      const recursive = target[1].endsWith("...");
      const packageDirectory = path.posix.join(directory, target[1].replace(/\/\.\.\.$/, ""));
      filters.push({ kind: "go", pattern: filter[1] || filter[2] || filter[3], directory: packageDirectory, recursive });
    }
    if (line.includes("--test-name-pattern") || line.includes("--expected-tests")) {
      const filter = line.match(/--test-name-pattern(?:=|\s+)(?:'([^']+)'|"([^"]+)"|([^\s]+))/);
      const files = [...line.matchAll(/(?:^|\s)([\w./-]+\.test\.cjs)(?=\s|$)/g)].map(match => match[1]);
      if ((line.includes("--test-name-pattern") && !filter) || files.length !== 1) throw new Error(`cannot parse Node filter: ${line}`);
      const expected = line.match(/--expected-tests=(\d+)/);
      if (line.includes("--expected-tests") && !expected) throw new Error(`cannot parse Node expected count: ${line}`);
      filters.push({ kind: "node", pattern: filter ? filter[1] || filter[2] || filter[3] : undefined, file: files[0], expected: expected ? Number(expected[1]) : undefined });
    }
  }
  return filters;
}

function verifyWorkflow(source, project = root) {
  if (source.includes("scripts/run-ci-tests.cjs go backend")) {
    const names = new Set(goTests("backend", project).map(test => test.name));
    for (const name of requiredWindowsBackendNames) if (!names.has(name)) throw new Error(`missing required Windows backend declaration: ${name}`);
  }
  const filters = workflowFilters(source);
  for (const filter of filters) {
    let declarations = filter.kind === "go" ? goTests(filter.directory, project) : nodeTests(filter.file, project);
    if (filter.kind === "go" && !filter.recursive) declarations = declarations.filter(test => path.dirname(test.file) === filter.directory);
    const selected = filter.pattern === undefined ? declarations : selectTests(filter.pattern, declarations, filter.kind);
    if (filter.expected !== undefined && selected.length !== filter.expected) throw new Error(`Node filter count ${selected.length} != expected ${filter.expected}: ${filter.pattern || filter.file}`);
    filter.names = selected.map(test => test.name);
  }
  if (!filters.length) throw new Error("CI has no verifiable test filters");
  return filters;
}

module.exports = { goTests, nodeTests, selectTests, workflowFilters, verifyWorkflow, requiredWindowsBackendNames };
if (require.main === module) {
  try {
    const filename = process.argv[2] || path.join(root, ".github/workflows/ci.yml");
    console.log(JSON.stringify(verifyWorkflow(fs.readFileSync(filename, "utf8")), null, 2));
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
