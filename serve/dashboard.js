/* Iris dashboard widgets: dependency-free vanilla JavaScript. */
(function (global) {
  "use strict";

  var DATE_FIELDS = ["due", "deadline", "date", "target"];
  var STATUS_SYMBOLS = {
    "open": "○",
    "in-progress": "◐",
    "in_progress": "◐",
    "in progress": "◐",
    "done": "●",
    "complete": "●"
  };

  function values(entry, field) {
    var value = entry && entry[field];
    if (Array.isArray(value)) return value.map(String);
    if (value === undefined || value === null) return [];
    return [String(value)];
  }

  function textValue(entry, field) {
    return values(entry, field).join(", ");
  }

  function globMatch(actual, wanted) {
    if (wanted.indexOf("*") < 0) return actual === wanted;
    var escaped = wanted.split("*").map(function (part) {
      return part.replace(/[\\^$+?.()|{}[\]]/g, "\\$&");
    });
    return new RegExp("^" + escaped.join(".*") + "$", "i").test(actual);
  }

  // Comma-separated field:value clauses are case-insensitive AND expressions.
  // Arrays match when any element matches. Asterisks provide simple glob /
  // contains matching (area:*garden*), while values without them are exact.
  function matches(entry, expression) {
    if (!expression || !expression.trim()) return true;
    return expression.split(",").every(function (clause) {
      clause = clause.trim();
      if (!clause) return true;
      var separator = clause.indexOf(":");
      if (separator < 0) return false;
      var field = clause.slice(0, separator).trim();
      var wanted = clause.slice(separator + 1).trim().toLowerCase();
      return values(entry, field).some(function (actual) {
        return globMatch(actual.toLowerCase(), wanted);
      });
    });
  }

  function statusRank(entry) {
    var status = textValue(entry, "status").trim().toLowerCase();
    if (status === "done") return 2;
    if (status === "complete") return 1;
    return 0;
  }

  function parseDate(value) {
    var match;
    if ((match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value))) {
      return validUTC(+match[1], +match[2], +match[3]);
    }
    if ((match = /^(\d{4})\/(\d{2})\/(\d{2})$/.exec(value))) {
      return validUTC(+match[1], +match[2], +match[3]);
    }
    if ((match = /^(\d{2})\.(\d{2})\.(\d{4})$/.exec(value))) {
      return validUTC(+match[3], +match[2], +match[1]);
    }
    // Date.parse handles RFC3339-like values, including timezone offsets.
    if (/^\d{4}-\d{2}-\d{2}T/.test(value) || /Z$/.test(value) || /[+-]\d{2}:?\d{2}$/.test(value)) {
      var parsed = Date.parse(value);
      return isNaN(parsed) ? null : parsed;
    }
    return null;
  }

  function validUTC(year, month, day) {
    var date = new Date(Date.UTC(year, month - 1, day));
    if (date.getUTCFullYear() !== year || date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return null;
    return date.getTime();
  }

  function candidate(entry) {
    for (var i = 0; i < DATE_FIELDS.length; i++) {
      var field = DATE_FIELDS[i];
      var raw = textValue(entry, field).trim();
      if (raw) return { raw: raw, date: parseDate(raw) };
    }
    return { raw: "", date: null };
  }

  function numberValue(value) {
    if (!/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$/.test(value.trim())) return null;
    var number = Number(value);
    return isFinite(number) ? number : null;
  }

  function compareEntries(left, right) {
    var statusResult = statusRank(left) - statusRank(right);
    if (statusResult) return statusResult;

    var a = candidate(left);
    var b = candidate(right);
    if (a.date !== null && b.date === null) return -1;
    if (a.date === null && b.date !== null) return 1;
    if (a.date !== null && b.date !== null && a.date !== b.date) return a.date - b.date;

    var aNumber = numberValue(a.raw);
    var bNumber = numberValue(b.raw);
    if (aNumber !== null && bNumber === null) return -1;
    if (aNumber === null && bNumber !== null) return 1;
    if (aNumber !== null && bNumber !== null && aNumber !== bNumber) return aNumber - bNumber;

    var aString = a.raw.toLowerCase();
    var bString = b.raw.toLowerCase();
    if (aString < bString) return -1;
    if (aString > bString) return 1;

    var aTitle = textValue(left, "title").toLowerCase();
    var bTitle = textValue(right, "title").toLowerCase();
    return aTitle < bTitle ? -1 : aTitle > bTitle ? 1 : 0;
  }

  function filteredAndSorted(entries, expression) {
    return entries.filter(function (entry) { return matches(entry, expression); }).sort(compareEntries);
  }

  function limitedEntries(entries, expression, limit) {
    var sorted = filteredAndSorted(entries, expression);
    return { entries: sorted.slice(0, limit), omitted: Math.max(0, sorted.length - limit) };
  }

  function limitFor(widget) {
    var raw = widget.getAttribute("data-limit");
    if (raw === null || raw.trim() === "") return 10;
    var limit = Number(raw);
    return isFinite(limit) && limit >= 0 ? Math.floor(limit) : 10;
  }

  function node(tag, content, className) {
    var result = document.createElement(tag);
    if (className) result.className = className;
    if (content !== undefined) result.textContent = content;
    return result;
  }

  function safeLink(entry, title) {
    var url = entry && typeof entry.url === "string" ? entry.url : "";
    if (!/^\/(?!\/)/.test(url)) return node("span", title, "dashboard-title");
    var link = document.createElement("a");
    link.href = url;
    link.className = "dashboard-title";
    link.textContent = title;
    return link;
  }

  function statusIndicator(entry) {
    var status = textValue(entry, "status").trim().toLowerCase();
    var symbol = STATUS_SYMBOLS[status] || "•";
    var className = "dashboard-status dashboard-status--" + (status || "unknown").replace(/[^a-z0-9_-]/g, "-");
    var indicator = node("span", symbol, className);
    indicator.setAttribute("aria-label", status ? "status: " + status : "status: unknown");
    return indicator;
  }

  function card(entry) {
    var article = node("article", undefined, "dashboard-card");
    var line = node("div", undefined, "dashboard-card-line");
    var title = textValue(entry, "title").trim() || "(untitled)";
    line.appendChild(statusIndicator(entry));
    line.appendChild(safeLink(entry, title));
    article.appendChild(line);

    var description = textValue(entry, "description").trim();
    if (description) article.appendChild(node("p", description, "dashboard-description"));

    var metadata = node("div", undefined, "dashboard-metadata");
    ["status", "priority", "deadline"].forEach(function (field) {
      var value = textValue(entry, field).trim();
      if (value) metadata.appendChild(node("span", field + ": " + value, "dashboard-field dashboard-field--" + field));
    });
    if (metadata.childNodes.length) article.appendChild(metadata);
    return article;
  }

  function renderCards(widget, entries, omitted) {
    var list = node("div", undefined, "dashboard-cards");
    entries.forEach(function (entry) { list.appendChild(card(entry)); });
    if (omitted > 0) list.appendChild(node("p", "+" + omitted + " others", "dashboard-others"));
    widget.replaceChildren(list);
  }

  function render(widget, entries) {
    var view = (widget.getAttribute("data-view") || "cards").trim().toLowerCase();
    if (view !== "cards") {
      widget.replaceChildren(node("p", "Unsupported dashboard view: " + view, "dashboard-error"));
      return;
    }
    var limit = limitFor(widget);
    var result = limitedEntries(entries, widget.getAttribute("data-filter") || "", limit);
    var visible = result.entries;
    if (!visible.length && result.omitted === 0) {
      widget.replaceChildren(node("p", "No matching entries.", "dashboard-empty"));
      return;
    }
    renderCards(widget, visible, result.omitted);
  }

  function ensureStyles() {
    if (document.getElementById("iris-dashboard-style")) return;
    var style = document.createElement("style");
    style.id = "iris-dashboard-style";
    style.textContent = ".dashboard-card-line{display:flex;gap:.45em;align-items:baseline}.dashboard-status{font-size:1.1em;color:#777}.dashboard-status--open{color:#668}.dashboard-status--in-progress,.dashboard-status--in_progress{color:#a76}.dashboard-status--done,.dashboard-status--complete{color:#686}.dashboard-metadata{display:flex;gap:.75em;color:#777;font-size:.9em}.dashboard-others{color:#777;font-style:italic}";
    document.head.appendChild(style);
  }

  function loadAll(widgets) {
    ensureStyles();
    widgets.forEach(function (widget) {
      widget.setAttribute("aria-busy", "true");
      widget.replaceChildren(node("p", "Loading dashboard…", "dashboard-loading"));
    });
    fetch("./..~meta~?cmd=get-frontmatter-array", { headers: { "Accept": "application/json" } })
      .then(function (response) {
        if (!response.ok) throw new Error("metadata request failed (" + response.status + ")");
        return response.json();
      })
      .then(function (entries) {
        entries = Array.isArray(entries) ? entries : [];
        widgets.forEach(function (widget) { render(widget, entries); });
      })
      .catch(function (error) {
        widgets.forEach(function (widget) {
          widget.replaceChildren(node("p", "Dashboard unavailable: " + error.message, "dashboard-error"));
        });
      })
      .finally(function () {
        widgets.forEach(function (widget) { widget.removeAttribute("aria-busy"); });
      });
  }

  var api = {
    matches: matches,
    compareEntries: compareEntries,
    filteredAndSorted: filteredAndSorted,
    limitedEntries: limitedEntries,
    parseDate: parseDate
  };
  if (global) global.IrisDashboard = api;

  function start() {
    var widgets = Array.prototype.slice.call(document.querySelectorAll(".dashboard-widget"));
    if (!widgets.length) return;
    loadAll(widgets);
  }

  if (typeof document !== "undefined") {
    if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
    else start();
  }
}(typeof window !== "undefined" ? window : (typeof globalThis !== "undefined" ? globalThis : null)));
