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

test("defaults missing status to open and priority to medium", () => {
  const [open] = dashboard.filteredAndSorted([{ title: "no-frontmatter" }], "status:open");
  assert.equal(open.status, "open");
  assert.equal(open.priority, "medium");
  // defaulted records match status:open and priority:medium filters
  assert.equal(dashboard.filteredAndSorted([{ title: "x" }], "status:open").length, 1);
  assert.equal(dashboard.filteredAndSorted([{ title: "x" }], "priority:medium").length, 1);
  assert.equal(dashboard.filteredAndSorted([{ title: "x" }], "priority:high").length, 0);
  // explicit values are never overridden
  const [done] = dashboard.filteredAndSorted([{ title: "y", status: "done", priority: "high" }], "");
  assert.equal(done.status, "done");
  assert.equal(done.priority, "high");
});

test("defaulted status sorts as active (before complete/done)", () => {
  const sorted = dashboard.filteredAndSorted([
    { title: "done", status: "done" },
    { title: "unclassified" },
    { title: "complete", status: "complete" }
  ], "");
  assert.deepEqual(sorted.map((entry) => entry.title), ["unclassified", "complete", "done"]);
});

test("create-file helpers build names, frontmatter, and contents", () => {
  assert.equal(dashboard.stripMdSuffix("fix-fence.md"), "fix-fence");
  assert.equal(dashboard.stripMdSuffix("fix-fence.MD"), "fix-fence");
  assert.equal(dashboard.validFileName("fix-fence"), true);
  assert.equal(dashboard.validFileName("fix fence"), true);
  assert.equal(dashboard.validFileName(".hidden"), false);
  assert.equal(dashboard.validFileName("a/b"), false);
  assert.equal(dashboard.validFileName("a\\b"), false);
  assert.equal(dashboard.validFileName(""), false);
  assert.equal(dashboard.buildContents({ area: "garten", status: "open" }, "Body"),
    "---\narea: garten\nstatus: open\n---\n\nBody");
  assert.equal(dashboard.buildContents({ area: "  " }, "Body"), "Body");
  assert.equal(dashboard.buildContents({}, "Body"), "Body");
});

test("formFields collects non-empty frontmatter fields in order", () => {
  const form = { elements: [
    { getAttribute: () => "name", value: "x" },
    { getAttribute: () => "area", value: "garten" },
    { getAttribute: () => "status", value: " " },
    { getAttribute: () => "owner", value: "tim" },
    { getAttribute: () => "contents", value: "body" }
  ] };
  assert.deepEqual(JSON.parse(JSON.stringify(dashboard.formFields(form))), { area: "garten", owner: "tim" });
});

test("buildContents merges form fields into user frontmatter, user wins on duplicates", () => {
  const merged = dashboard.buildContents(
    { area: "project", status: "open", priority: "medium", owner: "tim" },
    "---\nstatus: done\nArea: garten\nkeywords:\n  - one\n  - two\n---\n\n# Title\n\nBody\n");
  // status (user) and Area (user, case-insensitive match) win; only
  // priority and owner are appended (area is a case-insensitive duplicate)
  assert.equal(merged,
    "---\nstatus: done\nArea: garten\nkeywords:\n  - one\n  - two\npriority: medium\nowner: tim\n---\n\n# Title\n\nBody\n");
});

test("buildContents preserves the ... terminator and untouched documents", () => {
  // duplicate-only merge: nothing appended, document unchanged
  assert.equal(dashboard.buildContents({ owner: "tim" }, "---\nowner: nassim\n...\n\nBody"),
    "---\nowner: nassim\n...\n\nBody");
  // non-duplicate merge keeps the ... terminator
  assert.equal(dashboard.buildContents({ owner: "tim", area: "project" }, "---\nowner: nassim\n...\n\nBody"),
    "---\nowner: nassim\narea: project\n...\n\nBody");
  // no form fields: user frontmatter untouched
  assert.equal(dashboard.buildContents({}, "---\nstatus: open\n---\n\nBody"),
    "---\nstatus: open\n---\n\nBody");
});

test("buildContents treats incomplete frontmatter as body text", () => {
  const body = "---\nstatus: open\n\nno terminator here";
  assert.equal(dashboard.buildContents({ area: "x" }, body),
    "---\narea: x\n---\n\n" + body);
  assert.equal(dashboard.splitFrontmatter(body), null);
});

test("defaultDeadline returns a valid YYYY-MM-DD date string", () => {
  const deadline = dashboard.defaultDeadline();
  assert.match(deadline, /^\d{4}-\d{2}-\d{2}$/);
  const parsed = new Date(deadline + "T00:00:00");
  assert.ok(!Number.isNaN(parsed.getTime()));
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
