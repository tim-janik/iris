// Lightweight fixture/unit checks for the dependency-free dashboard helpers.
// Run with: node --test serve/dashboard_test.js
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const context = { globalThis: {}, console };
vm.runInNewContext(fs.readFileSync(__dirname + "/dashboard.js", "utf8"), context);
const dashboard = context.globalThis.IrisDashboard;

test("filters exact values, arrays, AND clauses, and contains globs", () => {
  const entry = { status: "open", area: "vegetable garden", keywords: ["repair", "outside"] };
  assert.equal(dashboard.matches(entry, "status:open,area:*garden*"), true);
  assert.equal(dashboard.matches(entry, "keywords:outside"), true);
  assert.equal(dashboard.matches(entry, "status:done"), false);
});

test("sorts active, complete, and done records with relaxed dates", () => {
  const sorted = dashboard.filteredAndSorted([
    { title: "done", status: "done", due: "2020-01-01" },
    { title: "complete", status: "complete", due: "2020-01-01" },
    { title: "later", status: "open", due: "2026/02/01" },
    { title: "soon", status: "open", due: "01.02.2026" }
  ], "");
  assert.deepEqual(sorted.map((entry) => entry.title), ["soon", "later", "complete", "done"]);
});

test("uses number/string fallbacks and reports omitted records", () => {
  const result = dashboard.limitedEntries([
    { title: "ten", due: "10" },
    { title: "two", due: "2" },
    { title: "alpha", due: "not-a-date" }
  ], "", 2);
  assert.deepEqual(result.entries.map((entry) => entry.title), ["two", "ten"]);
  assert.equal(result.omitted, 1);
});
