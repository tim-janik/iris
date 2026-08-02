/* Iris dashboard widgets: dependency-free vanilla JavaScript. */
(function (global) {
  "use strict";

  var DATE_FIELDS = ["due", "deadline", "date", "target"];
  // Presentation defaults: records without a status count as "open", records
  // without a priority count as "medium". Applied before filtering, sorting,
  // and rendering, so filters like status:open match unclassified records.
  var DEFAULT_STATUS = "open";
  var DEFAULT_PRIORITY = "medium";
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

  function withDefaults(entry) {
    var normalized = Object.assign({}, entry);
    if (!textValue(normalized, "status").trim()) normalized.status = DEFAULT_STATUS;
    if (!textValue(normalized, "priority").trim()) normalized.priority = DEFAULT_PRIORITY;
    return normalized;
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
    return entries.map(withDefaults).filter(function (entry) { return matches(entry, expression); }).sort(compareEntries);
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
    style.textContent = ".dashboard-card-line{display:flex;gap:.45em;align-items:baseline}.dashboard-status{font-size:1.1em;color:#777}.dashboard-status--open{color:#668}.dashboard-status--in-progress,.dashboard-status--in_progress{color:#a76}.dashboard-status--done,.dashboard-status--complete{color:#686}.dashboard-metadata{display:flex;gap:.75em;color:#777;font-size:.9em}.dashboard-others{color:#777;font-style:italic}" + ".dashboard-new fieldset{border:1px solid #ccc;border-radius:6px;margin:1em 0;padding:.75em 1em}.dashboard-new legend{font-weight:bold}.dashboard-new .dashboard-new-name{display:flex;align-items:baseline;gap:.4em}.dashboard-new .dashboard-new-suffix{color:#999}.dashboard-new .dashboard-new-fields{display:grid;grid-template-columns:repeat(auto-fill,minmax(13em,1fr));gap:.6em 1.2em;margin:1em 0}.dashboard-new label{display:flex;flex-direction:column;gap:.15em;font-size:.9em;color:#555}.dashboard-new input[type=text],.dashboard-new input[type=date],.dashboard-new select,.dashboard-new textarea{padding:.3em .4em;border:1px solid #ccc;border-radius:4px;font:inherit;color:#222}.dashboard-new textarea{width:100%;box-sizing:border-box}.dashboard-new button{margin-top:.6em;padding:.4em 1.1em}.dashboard-new .dashboard-new-status{color:#777;font-size:.9em;margin-left:.6em}.dashboard-new .dashboard-new-status.dashboard-error{color:#a33}";
    document.head.appendChild(style);
  }

  // --- create-file form widget (form.dashboard-new) ---
  // The form posts the file contents (generated frontmatter + markdown body)
  // to ./..~meta~?cmd=create-file and redirects to the clean HTML page on
  // success, or shows the server's error message in place.

  function stripMdSuffix(name) {
    return name.replace(/\.md$/i, "");
  }

  function validFileName(name) {
    return name.trim() !== "" && !/^\./.test(name) && !/[\\\/]/.test(name);
  }

  // Splits a leading frontmatter block (--- ... --- or ... ) from the rest
  // of a document. Returns null when the text has no complete frontmatter
  // block, matching the shared parser's behavior for incomplete blocks.
  function splitFrontmatter(text) {
    var lines = String(text || "").split("\n");
    if (!lines.length) return null;
    var first = lines[0];
    if (first.charCodeAt(0) === 0xFEFF) first = first.slice(1);
    if (first.trim() !== "---") return null;
    var end = -1;
    for (var i = 1; i < lines.length; i++) {
      var trimmed = lines[i].trim();
      if (trimmed === "---" || trimmed === "...") { end = i; break; }
    }
    if (end < 0) return null;
    return { fm: lines.slice(1, end), terminator: lines[end], rest: lines.slice(end + 1) };
  }

  // Top-level keys of a frontmatter block: simple "key: value" lines at
  // column 0, lower-cased. Indented lines (nested maps, list items) and
  // lines without a colon are ignored.
  function frontmatterKeys(fmLines) {
    var keys = {};
    fmLines.forEach(function (line) {
      var match = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:/.exec(line);
      if (match) keys[match[1].toLowerCase()] = true;
    });
    return keys;
  }

  // Builds the file contents from form fields and the markdown body. When the
  // body already starts with a complete frontmatter block, the form fields are
  // merged into it: fields whose key already exists (compared case-
  // insensitively) are left untouched so user-typed frontmatter takes
  // precedence, all other form fields are appended before the terminator, and
  // the rest of the document is preserved verbatim. Without frontmatter, a
  // block is built from the fields as before.
  function buildContents(fields, body) {
    var extraLines = [];
    for (var key in fields) {
      if (!Object.prototype.hasOwnProperty.call(fields, key)) continue;
      var value = String(fields[key]).trim();
      if (value) extraLines.push(key + ": " + value);
    }
    var parsed = splitFrontmatter(body);
    if (parsed) {
      var taken = frontmatterKeys(parsed.fm);
      var added = extraLines.filter(function (line) {
        var key = line.slice(0, line.indexOf(":")).toLowerCase();
        return !taken[key];
      });
      var fmParts = [];
      if (parsed.fm.length) fmParts.push(parsed.fm.join("\n"));
      if (added.length) fmParts.push(added.join("\n"));
      var joined = fmParts.join("\n");
      var rebuilt = "---\n" + (joined ? joined + "\n" : "") + parsed.terminator;
      var rest = parsed.rest.join("\n");
      return rest ? rebuilt + "\n" + rest : rebuilt;
    }
    if (!extraLines.length) return String(body || "");
    return "---\n" + extraLines.join("\n") + "\n---\n\n" + body;
  }

  // Collects non-empty frontmatter fields from a form. Every form control
  // with a name attribute is a candidate; the "name" and "contents" controls
  // are excluded because they are handled separately.
  function formFields(form) {
    var fields = {};
    Array.prototype.slice.call(form.elements).forEach(function (el) {
      var key = el.getAttribute("name");
      if (!key || key === "name" || key === "contents") return;
      var value = String(el.value || "").trim();
      if (value) fields[key] = value;
    });
    return fields;
  }

  function showFormStatus(status, message, isError) {
    if (!status) return;
    status.textContent = message;
    status.className = "dashboard-new-status" + (isError ? " dashboard-error" : "");
  }

  // Tomorrow's date (local time) as YYYY-MM-DD, used for the "now+1day"
  // default deadline. <input type="date"> requires a concrete date value.
  function defaultDeadline() {
    var now = new Date();
    now.setDate(now.getDate() + 1);
    var month = String(now.getMonth() + 1);
    var day = String(now.getDate());
    return now.getFullYear() + "-" + (month.length < 2 ? "0" + month : month) + "-" + (day.length < 2 ? "0" + day : day);
  }

  // Fills empty form controls that carry a data-default attribute, so freshly
  // opened forms come pre-filled with sensible values.
  function applyFormDefaults(form) {
    Array.prototype.slice.call(form.querySelectorAll("[data-default]")).forEach(function (el) {
      if (el.value) return;
      if (el.getAttribute("data-default") === "now+1day") el.value = defaultDeadline();
    });
  }

  function initCreateForm(form) {
    applyFormDefaults(form);
    var button = form.querySelector('button[type="submit"]');
    var status = form.querySelector(".dashboard-new-status");
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      var name = stripMdSuffix(String(form.elements["name"].value || "").trim());
      if (!validFileName(name)) {
        showFormStatus(status, "Invalid file name: no leading dot, no slashes.", true);
        return;
      }
      var contents = buildContents(formFields(form), form.elements["contents"].value || "");
      if (button) button.disabled = true;
      showFormStatus(status, "Creating " + name + ".md \u2026");
      fetch("./..~meta~?cmd=create-file&name=" + encodeURIComponent(name + ".md"), {
        method: "POST",
        headers: { "Accept": "application/json" },
        body: contents
      }).then(function (response) {
        if (response.ok) {
          window.location.assign("./" + encodeURIComponent(name));
          return;
        }
        return response.text().then(function (message) {
          showFormStatus(status, "Create failed: " + message, true);
        });
      }).catch(function (error) {
        showFormStatus(status, "Create failed: " + error.message, true);
      }).finally(function () {
        if (button) button.disabled = false;
      });
    });
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
    parseDate: parseDate,
    withDefaults: withDefaults,
    stripMdSuffix: stripMdSuffix,
    validFileName: validFileName,
    buildContents: buildContents,
    splitFrontmatter: splitFrontmatter,
    formFields: formFields,
    defaultDeadline: defaultDeadline
  };
  if (global) global.IrisDashboard = api;

  function start() {
    var widgets = Array.prototype.slice.call(document.querySelectorAll(".dashboard-widget"));
    if (widgets.length) loadAll(widgets);
    var forms = Array.prototype.slice.call(document.querySelectorAll("form.dashboard-new"));
    if (forms.length) {
      ensureStyles();
      forms.forEach(initCreateForm);
    }
  }

  if (typeof document !== "undefined") {
    if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
    else start();
  }
}(typeof window !== "undefined" ? window : (typeof globalThis !== "undefined" ? globalThis : null)));
