// Lightweight fixture/unit checks for the dependency-free dashboard helpers.
// Run with: node --test serve/dashboard_test.js
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const context = { globalThis: {}, console };
// Minimal fake DOM so card()/statusIndicator()/safeLink() are testable.
function fakeElement(tag) {
  return {
    tagName: tag,
    className: "",
    textContent: undefined,
    href: undefined,
    childNodes: [],
    setAttribute() {},
    appendChild(child) { this.childNodes.push(child); return child; },
    replaceChildren(...children) { this.childNodes = children; }
  };
}
context.document = {
  createElement: fakeElement,
  getElementById: () => null,
  querySelectorAll: () => [],
  readyState: "complete",
  head: fakeElement("head"),
  addEventListener() {}
};
vm.runInNewContext(fs.readFileSync(__dirname + "/dashboard.js", "utf8"), context);
const dashboard = context.globalThis.IrisDashboard;

function cardSpans(card) {
  const spans = [];
  (function walk(node) {
    if (node.textContent !== undefined) spans.push({ cls: node.className, text: node.textContent, href: node.href });
    (node.childNodes || []).forEach(walk);
  })(card);
  return spans;
}

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

test("sorts by status, then priority, then due date", () => {
  const sorted = dashboard.filteredAndSorted([
    { title: "done-high", status: "done", priority: "high", due: "2026-01-01" },
    { title: "low-early", status: "open", priority: "low", due: "2026-08-01" },
    { title: "high-late", status: "open", priority: "high", due: "2026-09-01" },
    { title: "medium", status: "open", priority: "medium", due: "2026-07-01" }
  ], "");
  assert.deepEqual(sorted.map((entry) => entry.title), ["high-late", "medium", "low-early", "done-high"]);
});

test("same status and priority sort by due date ascending", () => {
  const sorted = dashboard.filteredAndSorted([
    { title: "late", status: "open", priority: "high", due: "2026-08-10" },
    { title: "early", status: "open", priority: "high", due: "2026-08-02" },
    { title: "middle", status: "open", priority: "high", due: "2026-08-08" }
  ], "");
  assert.deepEqual(sorted.map((entry) => entry.title), ["early", "middle", "late"]);
});

test("unknown priorities sort after medium and before low, dates still ascending", () => {
  // Mirrors the current priorityRank ladder: max, xhigh, high, medium,
  // unknown, low, xlow, min. Unknown values rank after medium so they never
  // outrank an explicitly medium item.
  const sorted = dashboard.filteredAndSorted([
    { title: "explicit-medium", status: "open", priority: "medium", due: "2026-08-08" },
    { title: "weird", status: "open", priority: "urgent", due: "2026-08-02" },
    { title: "high", status: "open", priority: "high", due: "2026-09-01" },
    { title: "low", status: "open", priority: "low", due: "2026-01-01" }
  ], "");
  assert.deepEqual(sorted.map((entry) => entry.title), ["high", "explicit-medium", "weird", "low"]);
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

test("card renders value-only metadata with date aliasing and estimate", () => {
  const entry = dashboard.withDefaults({
    title: "Fix fence", description: "Board broke", url: "/issues/fix-fence",
    deadline: "2026-02-01", estimate: "1 day", area: "garden"
  });
  const spans = cardSpans(dashboard.card(entry));
  const chips = spans.filter((s) => s.cls.startsWith("dashboard-field")).map((s) => s.text);
  // Value-only chips; field order is a display detail that keeps changing
  // during dashboard iteration, so chips are compared order-independently.
  assert.deepEqual(chips.slice().sort(), ["2026-02-01", "Garden", "Medium", "Open", "ca 1 day"].sort());
  assert.ok(!spans.some((s) => /^(status|priority|due|estimate|area):/.test(s.text)));
  // linked title and status glyph
  assert.equal(spans.find((s) => s.cls === "dashboard-title").href, "/issues/fix-fence");
  assert.ok(spans.some((s) => s.cls.split(/\s+/).includes("dashboard-status--open") && s.text === "○"));
});

test("date aliasing: due wins over deadline, date, and target", () => {
  const base = { title: "x", url: "/x", status: "done" };
  const chipOf = (entry) => {
    const spans = cardSpans(dashboard.card(entry));
    const due = spans.find((s) => s.cls.split(/\s+/).includes("dashboard-field--due"));
    return due ? due.text : null;
  };
  assert.equal(chipOf({ ...base, due: "2026-01-01", deadline: "2026-02-01" }), "2026-01-01");
  assert.equal(chipOf({ ...base, deadline: "2026-02-01" }), "2026-02-01");
  assert.equal(chipOf({ ...base, date: "2026-03-01" }), "2026-03-01");
  assert.equal(chipOf({ ...base, target: "2026-04-01" }), "2026-04-01");
  assert.equal(chipOf(base), null);
});

test("card omits estimate when absent and keeps explicit values", () => {
  const spans = cardSpans(dashboard.card({ title: "t", url: "/t", status: "done", priority: "low" }));
  const chips = spans.filter((s) => s.cls.startsWith("dashboard-field")).map((s) => s.text);
  assert.deepEqual(chips.slice().sort(), ["Done", "Low"]);
});

test("card prefixes estimate with ca at display time only", () => {
  // stored value stays clean; display adds the prefix
  assert.equal(dashboard.estimateValue({ estimate: "1 day" }), "ca 1 day");
  assert.equal(dashboard.estimateValue({ estimate: "3h" }), "ca 3h");
  // already-prefixed values are not doubled
  assert.equal(dashboard.estimateValue({ estimate: "ca 3h" }), "ca 3h");
  assert.equal(dashboard.estimateValue({ estimate: "Ca. 2 days" }), "Ca. 2 days");
  assert.equal(dashboard.estimateValue({}), "");
  const spans = cardSpans(dashboard.card({ title: "t", url: "/t", estimate: "1 day" }));
  const chips = spans.filter((s) => s.cls.startsWith("dashboard-field")).map((s) => s.text);
  assert.deepEqual(chips, ["ca 1 day"]);
});

test("dueStatus colors overdue red and soon yellow, never for done/complete", () => {
  const iso = (offsetDays) => {
    const d = new Date();
    d.setUTCDate(d.getUTCDate() + offsetDays);
    return d.toISOString().slice(0, 10);
  };
  // never flagged for done/complete, even when overdue
  assert.equal(dashboard.dueStatus({ status: "done", deadline: iso(-7) }), "");
  assert.equal(dashboard.dueStatus({ status: "complete", deadline: iso(-7) }), "");
  // past -> overdue, within 3 days -> soon, later -> normal
  assert.equal(dashboard.dueStatus({ status: "open", deadline: iso(-1) }), "overdue");
  assert.equal(dashboard.dueStatus({ status: "open", deadline: iso(2) }), "soon");
  assert.equal(dashboard.dueStatus({ status: "open", deadline: iso(10) }), "");
  // unparseable or missing values are data, not errors
  assert.equal(dashboard.dueStatus({ status: "open", deadline: "asap" }), "");
  assert.equal(dashboard.dueStatus({ status: "open" }), "");
  // the rendered chip carries the color class
  const spans = cardSpans(dashboard.card({ title: "t", url: "/t", status: "open", due: iso(-1) }));
  const due = spans.find((s) => s.cls.split(/\s+/).includes("dashboard-field--due"));
  assert.ok(due.cls.split(/\s+/).includes("dashboard-field--due-overdue"));
  const soon = cardSpans(dashboard.card({ title: "t", url: "/t", status: "open", due: iso(2) }));
  const dueSoon = soon.find((s) => s.cls.split(/\s+/).includes("dashboard-field--due"));
  assert.ok(dueSoon.cls.split(/\s+/).includes("dashboard-field--due-soon"));
});

test("reloadAfterCreate detects the Create & Reload button", () => {
  assert.equal(dashboard.reloadAfterCreate({ classList: { contains: (c) => c === "dashboard-new-reload" } }), true);
  assert.equal(dashboard.reloadAfterCreate({ classList: { contains: () => false } }), false);
  assert.equal(dashboard.reloadAfterCreate(undefined), false);
  assert.equal(dashboard.reloadAfterCreate(null), false);
});

test("defaultDue returns a valid YYYY-MM-DD date string", () => {
  const due = dashboard.defaultDue();
  assert.match(due, /^\d{4}-\d{2}-\d{2}$/);
  const parsed = new Date(due + "T00:00:00");
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
