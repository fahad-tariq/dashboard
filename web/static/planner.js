/* planner.js -- client-side picker filter + drag-and-drop (ES5 compatible) */
/* All DnD events are delegated from document so they survive SSE outerHTML swaps. */

/* global window, document, fetch, htmx */

window.planDragInProgress = false;
window.planDetailExpanded = false;

function planItemClick(e) {
    var item = e.currentTarget;
    if (!item || !item.classList.contains('plan-item')) return;
    var wasMinimised = item.classList.contains('minimised');
    item.classList.toggle('minimised');
    var chevron = item.querySelector('.plan-item-toggle');
    if (chevron) chevron.innerHTML = wasMinimised ? '\u25BE' : '\u25B8';
    item.setAttribute('aria-expanded', String(wasMinimised));
    // Disable drag on expanded item to prevent accidental drag.
    if (wasMinimised) {
        item.setAttribute('draggable', 'false');
    } else if (!item.classList.contains('plan-item-done')) {
        item.setAttribute('draggable', 'true');
    }
    // Update global flag: true if any item is expanded.
    var expanded = document.querySelectorAll('.plan-item:not(.minimised)');
    window.planDetailExpanded = expanded.length > 0;
}

function plannerFilter(query) {
    var items = document.querySelectorAll('.plan-pick-item');
    var q = query.toLowerCase();
    for (var i = 0; i < items.length; i++) {
        var title = (items[i].getAttribute('data-title') || '').toLowerCase();
        var tags = (items[i].getAttribute('data-tags') || '').toLowerCase();
        if (!q || title.indexOf(q) !== -1 || tags.indexOf(q) !== -1) {
            items[i].style.display = '';
        } else {
            items[i].style.display = 'none';
        }
    }
}

// --- Homepage plan reorder (drag within .plan-today-tasks) ---

var draggedEl = null;

function clearDropIndicators() {
    var all = document.querySelectorAll('.plan-drop-above, .plan-drop-below');
    for (var i = 0; i < all.length; i++) {
        all[i].classList.remove('plan-drop-above', 'plan-drop-below');
    }
}

function collectSlugs(list) {
    var container = document.querySelector('.plan-today-tasks');
    if (!container) return [];
    var items = container.querySelectorAll('.plan-item[data-list="' + list + '"]');
    var slugs = [];
    for (var i = 0; i < items.length; i++) {
        var slug = items[i].getAttribute('data-slug');
        if (slug) slugs.push(slug);
    }
    return slugs;
}

function postReorder(list) {
    var slugs = collectSlugs(list);
    if (slugs.length === 0) return;
    var body = 'slugs=' + encodeURIComponent(slugs.join(', ')) + '&list=' + encodeURIComponent(list);
    fetch('/plan/reorder', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'HX-Request': 'true' },
        credentials: 'same-origin',
        body: body
    });
}

// All DnD delegated from document so listeners survive SSE DOM replacement.
document.addEventListener('dragstart', function(e) {
    // Homepage plan reorder.
    var item = e.target.closest('.plan-today-tasks .plan-item');
    if (item && !item.classList.contains('plan-item-done')) {
        draggedEl = item;
        item.classList.add('plan-item-dragging');
        window.planDragInProgress = true;
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', item.getAttribute('data-slug'));
        return;
    }

    // Calendar week view day move.
    var task = e.target.closest('.calendar-grid-week .calendar-task');
    if (task) {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', task.getAttribute('data-slug'));
        e.dataTransfer.setData('application/x-list', task.getAttribute('data-list'));
        task.classList.add('plan-item-dragging');
        window.planDragInProgress = true;
    }
});

document.addEventListener('dragover', function(e) {
    // Homepage reorder.
    if (draggedEl) {
        var target = e.target.closest('.plan-today-tasks .plan-item');
        if (!target || target === draggedEl) {
            clearDropIndicators();
            // Still need preventDefault if over the container to allow drop.
            if (e.target.closest('.plan-today-tasks')) e.preventDefault();
            return;
        }
        if (target.getAttribute('data-list') !== draggedEl.getAttribute('data-list')) {
            clearDropIndicators();
            e.preventDefault();
            return;
        }
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        clearDropIndicators();
        var rect = target.getBoundingClientRect();
        var mid = rect.top + rect.height / 2;
        if (e.clientY < mid) {
            target.classList.add('plan-drop-above');
        } else {
            target.classList.add('plan-drop-below');
        }
        return;
    }

    // Calendar day move.
    var cell = e.target.closest('.calendar-grid-week .calendar-cell');
    if (cell && cell.getAttribute('data-date')) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        var grid = cell.closest('.calendar-grid-week');
        var cells = grid.querySelectorAll('.calendar-cell-drop-target');
        for (var i = 0; i < cells.length; i++) {
            if (cells[i] !== cell) cells[i].classList.remove('calendar-cell-drop-target');
        }
        cell.classList.add('calendar-cell-drop-target');
    }
});

document.addEventListener('drop', function(e) {
    // Homepage reorder.
    if (draggedEl) {
        e.preventDefault();
        var target = e.target.closest('.plan-today-tasks .plan-item');
        if (!target || target === draggedEl) return;
        if (target.getAttribute('data-list') !== draggedEl.getAttribute('data-list')) return;

        var rect = target.getBoundingClientRect();
        var mid = rect.top + rect.height / 2;
        if (e.clientY < mid) {
            target.parentNode.insertBefore(draggedEl, target);
        } else {
            target.parentNode.insertBefore(draggedEl, target.nextSibling);
        }
        postReorder(draggedEl.getAttribute('data-list'));
        return;
    }

    // Calendar day move.
    var cell = e.target.closest('.calendar-grid-week .calendar-cell');
    if (!cell) return;
    var date = cell.getAttribute('data-date');
    if (!date) return;
    e.preventDefault();

    var slug = e.dataTransfer.getData('text/plain');
    var list = e.dataTransfer.getData('application/x-list');
    if (!slug || !list) return;

    if (list === 'personal') list = 'todos';

    var body = 'slug=' + encodeURIComponent(slug) + '&list=' + encodeURIComponent(list) + '&date=' + encodeURIComponent(date);
    fetch('/plan/set', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'HX-Request': 'true' },
        credentials: 'same-origin',
        body: body
    }).then(function() {
        window.location.reload();
    });
});

document.addEventListener('dragend', function() {
    if (draggedEl) {
        draggedEl.classList.remove('plan-item-dragging');
    }
    draggedEl = null;
    clearDropIndicators();
    window.planDragInProgress = false;

    // Clean up calendar highlights.
    var cells = document.querySelectorAll('.calendar-cell-drop-target');
    for (var i = 0; i < cells.length; i++) {
        cells[i].classList.remove('calendar-cell-drop-target');
    }
    var tasks = document.querySelectorAll('.plan-item-dragging');
    for (var j = 0; j < tasks.length; j++) {
        tasks[j].classList.remove('plan-item-dragging');
    }
});

document.addEventListener('dragleave', function(e) {
    var cell = e.target.closest('.calendar-cell');
    if (cell) cell.classList.remove('calendar-cell-drop-target');
});

// --- Mobile fallback: up/down arrow buttons ---

function planMoveUp(btn) {
    var item = btn.closest('.plan-item');
    if (!item) return;
    var prev = item.previousElementSibling;
    while (prev && !prev.classList.contains('plan-item')) {
        prev = prev.previousElementSibling;
    }
    if (!prev) return;
    if (prev.getAttribute('data-list') !== item.getAttribute('data-list')) return;
    item.parentNode.insertBefore(item, prev);
    postReorder(item.getAttribute('data-list'));
}

function planMoveDown(btn) {
    var item = btn.closest('.plan-item');
    if (!item) return;
    var next = item.nextElementSibling;
    while (next && !next.classList.contains('plan-item')) {
        next = next.nextElementSibling;
    }
    if (!next) return;
    if (next.getAttribute('data-list') !== item.getAttribute('data-list')) return;
    item.parentNode.insertBefore(item, next.nextSibling);
    postReorder(item.getAttribute('data-list'));
}
