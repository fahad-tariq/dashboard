// Shared tracker JS for tasks and goals pages.

var activeFilterType = '';
var activeFilterValue = '';

function filterKeyPrefix() {
    return window.location.pathname.replace(/\//g, '_');
}

function trackerFilter(type, value) {
    if (activeFilterType === type && activeFilterValue === value) {
        type = '';
        value = '';
    }
    activeFilterType = type;
    activeFilterValue = value;

    var prefix = filterKeyPrefix();
    localStorage.setItem(prefix + '_filterType', type);
    localStorage.setItem(prefix + '_filterValue', value);

    applyFilter();
}

function updateFilterBadge() {
    var container = document.querySelector('.tracker-filters');
    if (!container) return;
    var existing = container.querySelector('.filter-active-badge');
    if (activeFilterType) {
        if (!existing) {
            var badge = document.createElement('span');
            badge.className = 'filter-active-badge';
            badge.textContent = 'filtered';
            container.appendChild(badge);
        }
    } else {
        if (existing) existing.parentNode.removeChild(existing);
    }
}

function applyFilter() {
    document.querySelectorAll('.filter-tag[data-action="filter"]').forEach(function(b) {
        var on = activeFilterType
            ? b.getAttribute('data-filter-type') === activeFilterType && b.getAttribute('data-value') === activeFilterValue
            : b.getAttribute('data-filter-type') === '';
        b.classList.toggle('active', on);
        b.setAttribute('aria-pressed', String(on));
    });

    var items = document.querySelectorAll('.tracker-item');
    var totalItems = items.length;
    var visibleCount = 0;

    items.forEach(function(el) {
        if (!activeFilterType) {
            el.style.display = '';
            visibleCount++;
        } else if (activeFilterType === 'category') {
            var tags = (el.getAttribute('data-tags') || '').toLowerCase().trim().split(/\s+/);
            var show = tags.indexOf(activeFilterValue) >= 0;
            el.style.display = show ? '' : 'none';
            if (show) visibleCount++;
        } else {
            var attr = el.getAttribute('data-' + activeFilterType);
            var show = attr === activeFilterValue;
            el.style.display = show ? '' : 'none';
            if (show) visibleCount++;
        }
    });

    var filterEmpty = document.querySelector('.filter-empty');
    if (filterEmpty) {
        if (activeFilterType && visibleCount === 0 && totalItems > 0) {
            filterEmpty.style.display = '';
        } else {
            filterEmpty.style.display = 'none';
        }
    }

    updateFilterBadge();
}

// itemHeaderClick toggles a row when its header is clicked. The row's
// .item-toggle button is the keyboard and screen reader control; clicks on
// other controls in the header (badges, checkbox, forms, links) are theirs.
function itemHeaderClick(e, header) {
    header = header || e.currentTarget;
    var toggle = header.querySelector('.item-toggle');
    if (!toggle) return;
    if (e.target.closest('.item-toggle') || !e.target.closest('a, button, input, select, textarea, label, form')) {
        toggleItem(toggle);
    }
}

// setExpanded opens or closes a row. The chevron follows aria-expanded in
// CSS, and morph swaps keep both (data-keep-class, data-keep-attr).
function setExpanded(item, expanded) {
    item.classList.toggle('minimised', !expanded);
    var toggle = item.querySelector('.item-toggle');
    if (toggle) toggle.setAttribute('aria-expanded', String(expanded));
    if (expanded) loadCommentary(item);
}

function toggleItem(btn) {
    if (!btn) return;
    var item = btn.closest('.tracker-item');
    setExpanded(item, item.classList.contains('minimised'));
}

function loadCommentary(item) {
    var slot = item.querySelector('.commentary-slot');
    if (!slot || slot.getAttribute('data-loaded')) return;
    var url = slot.getAttribute('data-commentary-url');
    if (!url) return;
    slot.setAttribute('data-loaded', '1');
    var xhr = new XMLHttpRequest();
    xhr.open('GET', url);
    xhr.onload = function() {
        if (xhr.status === 200 && xhr.responseText.trim()) {
            slot.innerHTML = xhr.responseText;
        }
    };
    xhr.send();
}

function trackerToggleAll() {
    var items = document.querySelectorAll('.tracker-item:not(.tracker-item-done)');
    var anyMinimised = false;
    items.forEach(function(el) {
        if (el.classList.contains('minimised')) anyMinimised = true;
    });
    var shouldMinimise = !anyMinimised;
    items.forEach(function(el) { setExpanded(el, !shouldMinimise); });
    var toggleBtn = document.querySelector('.filter-toggle');
    if (toggleBtn) toggleBtn.textContent = shouldMinimise ? 'expand' : 'collapse';
}

function clearTrackerFilter() {
    var prefix = filterKeyPrefix();
    localStorage.removeItem(prefix + '_filterType');
    localStorage.removeItem(prefix + '_filterValue');
}

// Task completion celebration animation.
function celebrateComplete(form) {
    var item = form.closest('.tracker-item') || form.closest('.plan-item');
    if (item) {
        item.classList.add(item.classList.contains('plan-item') ? 'plan-item-completing' : 'tracker-item-completing');
    }
    return true;
}

// Idea triage fades the card out while the request runs; the swap waits
// for the fade (hx-swap="morph swap:300ms" on the form).
function triageAnimate(form) {
    var item = form.closest('.tracker-item');
    if (item) item.classList.add('idea-transitioning');
    return true;
}

// On page load: restore filter, expand hash target.
(function() {
    var prefix = filterKeyPrefix();
    var savedType = localStorage.getItem(prefix + '_filterType') || '';
    var savedValue = localStorage.getItem(prefix + '_filterValue') || '';
    if (savedType) {
        activeFilterType = savedType;
        activeFilterValue = savedValue;
        applyFilter();
    }

    // Expand and scroll to the row named by the hash: /todos#item-<id>.
    var hash = window.location.hash.slice(1);
    if (!hash) return;
    var el = document.getElementById(hash);
    if (!el || !el.classList.contains('tracker-item')) return;
    setExpanded(el, true);
    el.scrollIntoView({ block: 'center' });
})();

// --- Bulk select mode ---
var bulkSelectActive = false;

// Select mode works on the list pages and on today's plan. Plan rows mix
// three lists, so they are selected as "list:id"; list rows by ID.
var selectScope = '.tracker-page, .ideas-page, .plan-section';
var selectableRows = '.tracker-item, .plan-item';

function setSelectToggle(active) {
    var btn = document.getElementById('select-toggle');
    if (!btn) return null;
    btn.classList.toggle('active', active);
    btn.textContent = active ? 'cancel' : 'select';
    btn.setAttribute('aria-pressed', String(active));
    return btn;
}

function toggleSelectMode() {
    bulkSelectActive = !bulkSelectActive;
    var page = document.querySelector(selectScope);
    if (bulkSelectActive) {
        if (page) page.classList.add('select-mode');
        setSelectToggle(true);
        if (typeof syncDraggable === 'function') syncDraggable();
    } else {
        exitSelectMode();
    }
}

function exitSelectMode() {
    bulkSelectActive = false;
    var page = document.querySelector(selectScope);
    if (page) page.classList.remove('select-mode');
    var btn = setSelectToggle(false);
    if (btn) btn.focus();
    deselectAll();
    var bar = document.getElementById('bulk-bar');
    if (bar) bar.classList.remove('visible');
    if (typeof syncDraggable === 'function') syncDraggable();
}

function bulkCheckboxChanged() {
    updateBulkBar();
}

function selectionValue(item) {
    var id = item.getAttribute('data-id');
    if (!id) return '';
    if (item.classList.contains('plan-item')) return item.getAttribute('data-list') + ':' + id;
    return id;
}

function getSelectedIDs() {
    var ids = [];
    var checkboxes = document.querySelectorAll('.bulk-checkbox:checked');
    checkboxes.forEach(function(cb) {
        var item = cb.closest(selectableRows);
        if (item) {
            var value = selectionValue(item);
            if (value) ids.push(value);
            item.classList.add('bulk-selected');
        }
    });
    // Clear unselected items.
    document.querySelectorAll('.bulk-checkbox:not(:checked)').forEach(function(cb) {
        var item = cb.closest(selectableRows);
        if (item) item.classList.remove('bulk-selected');
    });
    return ids;
}

function updateBulkBar() {
    var ids = getSelectedIDs();
    var bar = document.getElementById('bulk-bar');
    var countEl = document.getElementById('bulk-bar-count');
    if (!bar) return;
    if (ids.length > 0) {
        bar.classList.add('visible');
        if (countEl) countEl.textContent = ids.length + ' selected';
    } else {
        bar.classList.remove('visible');
        if (countEl) countEl.textContent = '0 selected';
    }
}

function selectAllVisible() {
    var items = document.querySelectorAll('.tracker-item:not(.tracker-item-done), .plan-item:not(.plan-item-done)');
    items.forEach(function(el) {
        if (el.style.display === 'none') return;
        if (!el.getAttribute('data-id')) return;
        var cb = el.querySelector('.bulk-checkbox');
        if (cb) cb.checked = true;
    });
    updateBulkBar();
}

function deselectAll() {
    document.querySelectorAll('.bulk-checkbox').forEach(function(cb) {
        cb.checked = false;
    });
    document.querySelectorAll('.tracker-item.bulk-selected, .plan-item.bulk-selected').forEach(function(el) {
        el.classList.remove('bulk-selected');
    });
    updateBulkBar();
}

// fillBulkForm writes the selection into the form's ids (list pages) or
// items (plan) field and returns how many were selected.
function fillBulkForm(form) {
    var ids = getSelectedIDs();
    if (ids.length === 0) return 0;
    var input = form.querySelector('input[name="ids"], input[name="items"]');
    if (input) input.value = ids.join(', ');
    return ids.length;
}

function submitBulkAction(formId) {
    var form = document.getElementById(formId);
    if (!form) return false;
    return fillBulkForm(form) > 0;
}

function confirmBulkDelete(form) {
    var n = fillBulkForm(form);
    if (n === 0) return false;
    return confirmAction(form, 'Move ' + n + ' items to trash?');
}

// --- Delegated event dispatch ---
// Templates carry no inline handlers (the CSP is meant to drop
// 'unsafe-inline' for scripts). Listeners live on document so they survive
// SSE outerHTML swaps.
//
//   data-action="name"   click handler from clickActions, called with
//                        (element, event); page scripts may add to it.
//   data-stop-click      clicks inside do not reach data-action ancestors
//                        (stands in for the old inline stopPropagation, e.g.
//                        controls inside a row that toggles on click).
//   data-confirm="msg"   form submit asks via the confirm modal first
//                        (dialog.js handles htmx forms through htmx:confirm).
//   data-submit="name"   form submit handler from submitActions; returning
//                        false cancels the native submit.
//   data-autosubmit      a select that submits its form on change.
// Page scripts inside the content (house.js) run before this file, so the
// table may already hold their actions.
var clickActions = window.clickActions || {};
clickActions['item-header'] = function(el, evt) { itemHeaderClick(evt, el); };
clickActions['filter'] = function(el) {
    trackerFilter(el.getAttribute('data-filter-type') || '', el.getAttribute('data-filter-value') || '');
};
clickActions['toggle-all-items'] = function() { trackerToggleAll(); };
clickActions['toggle-select-mode'] = function() { toggleSelectMode(); };
clickActions['select-all'] = function() { selectAllVisible(); };
clickActions['deselect-all'] = function() { deselectAll(); };
clickActions['bulk-checkbox'] = function() { bulkCheckboxChanged(); };
clickActions['bulk-submit'] = function(el) { return submitBulkAction(el.getAttribute('data-bulk-form')); };
// A date input's own clear control: Safari's desktop date input has none.
clickActions['clear-date'] = function(el) {
    var input = document.getElementById(el.getAttribute('data-clear'));
    if (!input) return;
    input.value = '';
    if (window.liveRefresh) window.liveRefresh.keepValue(input);
    input.focus();
};

var submitActions = {
    'celebrate': function(form) { return celebrateComplete(form); },
    'triage': function(form) { return triageAnimate(form); },
    'bulk-delete': function(form) { return confirmBulkDelete(form); },
    'clear-filter': function() { clearTrackerFilter(); return true; }
};

// findClickAction returns the nearest data-action element at or above
// target, or null when a data-stop-click element comes first.
function findClickAction(target) {
    var el = target;
    while (el && el.nodeType === 1) {
        if (el.hasAttribute('data-action')) return el;
        if (el.hasAttribute('data-stop-click')) return null;
        el = el.parentNode;
    }
    return null;
}

document.addEventListener('click', function(evt) {
    var el = findClickAction(evt.target);
    if (!el) return;
    var handler = clickActions[el.getAttribute('data-action')];
    if (handler && handler(el, evt) === false) evt.preventDefault();
});

// Form submissions from the confirm modal use form.submit(), which fires no
// submit event, so a confirmed form is not intercepted a second time. htmx
// has already cancelled the native submit of its own forms (and asks through
// htmx:confirm), so only plain forms are confirmed here.
document.addEventListener('submit', function(evt) {
    var form = evt.target;
    if (!form || !form.getAttribute) return;
    var message = form.getAttribute('data-confirm');
    if (message !== null && !evt.defaultPrevented && !confirmAction(form, message)) {
        evt.preventDefault();
        return;
    }
    var handler = submitActions[form.getAttribute('data-submit')];
    if (handler && handler(form, evt) === false) evt.preventDefault();
});

document.addEventListener('change', function(evt) {
    var el = evt.target;
    if (el && el.hasAttribute && el.hasAttribute('data-autosubmit') && el.form) {
        // requestSubmit fires the submit event, so htmx forms stay htmx.
        if (el.form.requestSubmit) {
            el.form.requestSubmit();
        } else {
            el.form.submit();
        }
    }
});

// A swap brings rows the filter has not seen and drops the "filtered" badge.
// In select mode it also resets the toggle's label and the bulk bar's count
// (the morph keeps the classes and the ticked boxes).
document.addEventListener('htmx:afterSettle', function() {
    if (activeFilterType) applyFilter();
    updateFilterBadge();
    if (bulkSelectActive) {
        setSelectToggle(true);
        updateBulkBar();
    }
});

// A failed request leaves the row in place, so undo its animation.
document.addEventListener('htmx:afterRequest', function(evt) {
    if (evt.detail.successful || !evt.detail.elt || !evt.detail.elt.closest) return;
    var item = evt.detail.elt.closest('.tracker-item, .plan-item');
    if (item) item.classList.remove('idea-transitioning', 'tracker-item-completing', 'plan-item-completing');
});
