/* Iris dashboard widgets: dependency-free vanilla JavaScript.
   Bundle loading only: widgets are detected and one metadata request is made
   per page. Card rendering, filtering, and sorting arrive with Phase 3. */
(function () {
  "use strict";

  function node(tag, content, className) {
    var result = document.createElement(tag);
    if (className) result.className = className;
    if (content !== undefined) result.textContent = content;
    return result;
  }

  function loadAll(widgets) {
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
        if (!Array.isArray(entries) || !entries.length) {
          // Empty-result state. Widget rendering (Phase 3) consumes the data.
          widgets.forEach(function (widget) {
            widget.replaceChildren(node("p", "No matching entries.", "dashboard-empty"));
          });
        }
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

  function start() {
    var widgets = Array.prototype.slice.call(document.querySelectorAll(".dashboard-widget"));
    if (!widgets.length) return;
    loadAll(widgets);
  }

  if (typeof document !== "undefined") {
    if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
    else start();
  }
}());
