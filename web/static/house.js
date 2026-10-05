// House page: expandable rows, table filters, and completion note popovers.
// Loaded from inside .house-page; morph swaps keep the script element, so it
// runs once per page load, and the guard keeps listeners bound once anyway.

// Toggle detail row visibility.
function houseToggleDetail(row) {
    var detail = row.nextElementSibling;
    if (!detail || !detail.classList.contains("house-detail-row")) return;
    var hidden = detail.classList.toggle("house-detail-hidden");
    var toggle = row.querySelector(".house-row-toggle");
    if (toggle) toggle.setAttribute("aria-expanded", String(!hidden));
}

// A click anywhere on a row toggles it, except on the row's other controls.
function houseRowClick(e, row) {
    row = row || e.currentTarget;
    if (!e.target.closest(".house-row-toggle") && e.target.closest("a, button, input, select, textarea, label, form")) return;
    houseToggleDetail(row);
}

// House table filters.
function houseFilter(filter) {
    var btns = document.querySelectorAll(".house-filters .filter-tag");
    for (var i = 0; i < btns.length; i++) {
        btns[i].classList.toggle("active", btns[i].getAttribute("data-filter") === filter);
    }

    var rows = document.querySelectorAll("tr.house-row");
    for (var j = 0; j < rows.length; j++) {
        var row = rows[j];
        var detail = row.nextElementSibling;
        var type = row.getAttribute("data-type");
        var status = row.getAttribute("data-status");
        var days = parseInt(row.getAttribute("data-days") || "9999", 10);
        var show = false;

        switch (filter) {
            case "all":
                show = true;
                break;
            case "overdue":
                show = (type === "maintenance" && status === "overdue");
                break;
            case "due-week":
                show = (type === "maintenance" && days <= 7);
                break;
            case "due-month":
                show = (type === "maintenance" && days <= 30);
                break;
            case "todo":
                show = (type === "project" && status === "todo");
                break;
            case "active":
                show = (type === "project" && status === "active");
                break;
        }

        row.style.display = show ? "" : "none";
        if (detail && detail.classList.contains("house-detail-row")) {
            if (!show) {
                detail.style.display = "none";
            } else {
                detail.style.display = "";
            }
        }
    }
}

// Click actions for the delegated dispatcher in tracker.js. This script runs
// before tracker.js on the first load (it sits in the page content, ahead of
// the layout's scripts), so it creates the shared table if needed.
window.clickActions = window.clickActions || {};
window.clickActions["house-row"] = function(el, evt) { houseRowClick(evt, el); };
window.clickActions["house-filter"] = function(el) { houseFilter(el.getAttribute("data-filter")); };
window.clickActions["toggle-note-popover"] = function(el) {
    var pop = el.closest(".house-note-popover");
    if (pop) pop.classList.toggle("open");
};

// Close house note popovers when clicking outside. Clicks inside a
// data-stop-click region (row actions, edit sections, delete forms) leave
// them alone, as they did when those regions stopped propagation inline.
if (!window.houseListenersBound) {
    window.houseListenersBound = true;
    // A logged completion closes its note popover.
    document.addEventListener("htmx:afterRequest", function(e) {
        var pop = e.detail.successful && e.detail.elt && e.detail.elt.closest && e.detail.elt.closest(".house-note-popover");
        if (pop) pop.classList.remove("open");
    });
    document.addEventListener("click", function(e) {
        if (e.target.closest && e.target.closest("[data-stop-click]")) return;
        var pops = document.querySelectorAll(".house-note-popover.open");
        for (var i = 0; i < pops.length; i++) {
            if (!pops[i].contains(e.target)) {
                pops[i].classList.remove("open");
            }
        }
    });
}
