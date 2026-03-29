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
    document.querySelectorAll('.filter-tag').forEach(function(b) {
        if (!activeFilterType && b.textContent.trim() === 'all') {
            b.classList.add('active');
        } else if (activeFilterType && b.getAttribute('data-value') === activeFilterValue) {
            b.classList.add('active');
        } else {
            b.classList.remove('active');
        }
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

// Persistent set of expanded item slugs -- survives SSE swaps.
var trackerExpandedItems = {};

function toggleItem(btn) {
    if (!btn) return;
    var item = btn.closest('.tracker-item');
    item.classList.toggle('minimised');
    var minimised = item.classList.contains('minimised');
    btn.textContent = minimised ? '\u25B8' : '\u25BE';
    var header = item.querySelector('.tracker-item-header');
    if (header) header.setAttribute('aria-expanded', String(!minimised));
    var slug = item.getAttribute('data-slug');
    if (slug) {
        if (minimised) {
            delete trackerExpandedItems[slug];
        } else {
            trackerExpandedItems[slug] = true;
            loadCommentary(item);
        }
    }
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
    items.forEach(function(el) {
        if (shouldMinimise) {
            el.classList.add('minimised');
        } else {
            el.classList.remove('minimised');
        }
        var btn = el.querySelector('.item-toggle');
        if (btn) btn.textContent = shouldMinimise ? '\u25B8' : '\u25BE';
        var header = el.querySelector('.tracker-item-header');
        if (header) header.setAttribute('aria-expanded', String(!shouldMinimise));
        var slug = el.getAttribute('data-slug');
        if (slug) {
            if (shouldMinimise) {
                delete trackerExpandedItems[slug];
            } else {
                trackerExpandedItems[slug] = true;
            }
        }
    });
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

// Idea triage transition animation.
function triageAnimate(form) {
    var item = form.closest('.tracker-item');
    var action = form.getAttribute('action');
    var method = form.getAttribute('method') || 'POST';
    if (item) {
        item.classList.add('idea-transitioning');
    }
    setTimeout(function() {
        fetch(action, {
            method: method,
            body: new FormData(form),
            credentials: 'same-origin'
        }).then(function() {
            window.location.reload();
        }).catch(function() {
            window.location.reload();
        });
    }, 250);
    return false;
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

    var hash = window.location.hash.replace('#', '');
    if (!hash) return;
    var el = document.getElementById('item-' + hash);
    if (!el) return;
    el.classList.remove('minimised');
    var btn = el.querySelector('.item-toggle');
    if (btn) btn.textContent = '\u25BE';
    var header = el.querySelector('.tracker-item-header');
    if (header) header.setAttribute('aria-expanded', 'true');
    var slug = el.getAttribute('data-slug');
    if (slug) trackerExpandedItems[slug] = true;
    el.scrollIntoView({block: 'nearest'});
})();

// --- Bulk select mode ---
var bulkSelectActive = false;

function toggleSelectMode() {
    bulkSelectActive = !bulkSelectActive;
    var page = document.querySelector('.tracker-page, .ideas-page');
    var btn = document.getElementById('select-toggle');
    if (bulkSelectActive) {
        if (page) page.classList.add('select-mode');
        if (btn) { btn.classList.add('active'); btn.textContent = 'cancel'; }
    } else {
        exitSelectMode();
    }
}

function exitSelectMode() {
    bulkSelectActive = false;
    var page = document.querySelector('.tracker-page, .ideas-page');
    if (page) page.classList.remove('select-mode');
    var btn = document.getElementById('select-toggle');
    if (btn) { btn.classList.remove('active'); btn.textContent = 'select'; btn.focus(); }
    deselectAll();
    var bar = document.getElementById('bulk-bar');
    if (bar) bar.classList.remove('visible');
}

function bulkCheckboxChanged() {
    updateBulkBar();
}

function getSelectedSlugs() {
    var slugs = [];
    var checkboxes = document.querySelectorAll('.bulk-checkbox:checked');
    checkboxes.forEach(function(cb) {
        var item = cb.closest('.tracker-item');
        if (item) {
            var slug = item.getAttribute('data-slug');
            if (slug) slugs.push(slug);
            item.classList.add('bulk-selected');
        }
    });
    // Clear unselected items.
    document.querySelectorAll('.bulk-checkbox:not(:checked)').forEach(function(cb) {
        var item = cb.closest('.tracker-item');
        if (item) item.classList.remove('bulk-selected');
    });
    return slugs;
}

function updateBulkBar() {
    var slugs = getSelectedSlugs();
    var bar = document.getElementById('bulk-bar');
    var countEl = document.getElementById('bulk-bar-count');
    if (!bar) return;
    if (slugs.length > 0) {
        bar.classList.add('visible');
        if (countEl) countEl.textContent = slugs.length + ' selected';
    } else {
        bar.classList.remove('visible');
        if (countEl) countEl.textContent = '0 selected';
    }
}

function selectAllVisible() {
    var items = document.querySelectorAll('.tracker-item:not(.tracker-item-done)');
    items.forEach(function(el) {
        if (el.style.display === 'none') return;
        if (!el.getAttribute('data-slug')) return;
        var cb = el.querySelector('.bulk-checkbox');
        if (cb) cb.checked = true;
    });
    updateBulkBar();
}

function deselectAll() {
    document.querySelectorAll('.bulk-checkbox').forEach(function(cb) {
        cb.checked = false;
    });
    document.querySelectorAll('.tracker-item.bulk-selected').forEach(function(el) {
        el.classList.remove('bulk-selected');
    });
    updateBulkBar();
}

function submitBulkAction(formId) {
    var form = document.getElementById(formId);
    if (!form) return false;
    var slugs = getSelectedSlugs();
    if (slugs.length === 0) return false;
    var input = form.querySelector('input[name="slugs"]');
    if (input) input.value = slugs.join(', ');
    return true;
}

function confirmBulkDelete(form) {
    var slugs = getSelectedSlugs();
    if (slugs.length === 0) return false;
    var input = form.querySelector('input[name="slugs"]');
    if (input) input.value = slugs.join(', ');
    return confirmAction(form, 'Delete ' + slugs.length + ' items? This cannot be undone.');
}

// Re-apply filter, badge, and expand state after HTMX SSE swap.
document.addEventListener('htmx:afterSwap', function() {
    if (activeFilterType) {
        applyFilter();
    }
    updateFilterBadge();
    // Reset select mode after swap (page was replaced).
    if (bulkSelectActive) {
        exitSelectMode();
    }
    // Re-expand items from the persistent tracker.
    Object.keys(trackerExpandedItems).forEach(function(slug) {
        var el = document.getElementById('item-' + slug);
        if (el && el.classList.contains('minimised')) {
            el.classList.remove('minimised');
            var btn = el.querySelector('.item-toggle');
            if (btn) btn.textContent = '\u25BE';
            var header = el.querySelector('.tracker-item-header');
            if (header) header.setAttribute('aria-expanded', 'true');
        }
    });
    // Reset plan detail flag after swap (expanded state is ephemeral).
    window.planDetailExpanded = false;
});

// Delay SSE swap when a completion celebration is in progress so the
// green flash animation is visible before the DOM is replaced.
var pendingSwap = null;

document.addEventListener('htmx:beforeSwap', function(evt) {
    var target = evt.detail.target;
    if (!target) return;

    // Suppress SSE swaps (targeting .tracker-page) while select mode, drag,
    // plan detail, or any tracker item is expanded.
    var isSSESwap = target.classList && target.classList.contains('tracker-page');
    if (isSSESwap && (bulkSelectActive || window.planDragInProgress || window.planDetailExpanded || Object.keys(trackerExpandedItems).length > 0)) {
        evt.detail.shouldSwap = false;
        return;
    }

    var completing = target.querySelector && (target.querySelector('.tracker-item-completing') || target.querySelector('.plan-item-completing'));
    if (!completing) return;

    // Store swap details and prevent the immediate swap.
    var elt = evt.detail.elt;
    pendingSwap = {
        elt: elt,
        target: target
    };
    evt.detail.shouldSwap = false;

    // After the animation delay, re-trigger the SSE refresh.
    setTimeout(function() {
        pendingSwap = null;
        if (typeof htmx !== 'undefined' && elt) {
            htmx.trigger(elt, 'sse:file-changed');
        }
    }, 400);
});

// Auto-expand and scroll to item when navigating via hash (e.g. /todos#item-slug).
(function() {
    if (!window.location.hash) return;
    var el = document.getElementById(window.location.hash.slice(1));
    if (!el || !el.classList.contains('tracker-item')) return;
    if (el.classList.contains('minimised')) {
        var btn = el.querySelector('.item-toggle');
        if (btn) toggleItem(btn); // toggleItem already tracks in trackerExpandedItems
    }
    el.scrollIntoView({ block: 'center' });
})();

