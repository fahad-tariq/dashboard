/* planner.js -- client-side picker filter + drag-and-drop (ES5 compatible) */
/* All DnD events are delegated from document, so no listener depends on
   which elements a swap kept. Live refreshes wait while a drag is in
   progress (liveRefresh in live.js) and apply when it ends. */

/* global window, document, fetch, announce */

// planItemClick toggles a plan row. The row's toggle button is the
// keyboard and screen reader control; a click anywhere else on the row (but
// not on another control) toggles too, as a mouse convenience.
function planItemClick(e) {
    var item = e.currentTarget;
    if (!item || !item.classList.contains('plan-item')) return;
    if (!e.target.closest('.plan-item-toggle') && e.target.closest('a, button, input, select, textarea, label, form')) return;
    var wasMinimised = item.classList.contains('minimised');
    item.classList.toggle('minimised');
    var toggle = item.querySelector('.plan-item-toggle');
    if (toggle) toggle.setAttribute('aria-expanded', String(wasMinimised));
    syncDraggable();
}

// syncDraggable lets open plan rows be dragged while minimised; an expanded
// row is not draggable, so text in its detail can be selected.
function syncDraggable() {
    var items = document.querySelectorAll('.plan-today-tasks .plan-item');
    for (var i = 0; i < items.length; i++) {
        var on = !items[i].classList.contains('plan-item-done') && items[i].classList.contains('minimised');
        items[i].setAttribute('draggable', String(on));
    }
}

document.addEventListener('htmx:afterSettle', syncDraggable);

function holdRefresh() {
    if (window.liveRefresh) window.liveRefresh.hold('drag');
}

function releaseRefresh() {
    if (window.liveRefresh) window.liveRefresh.release('drag');
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
// The save started by a drop; refreshes wait for it, or they could fetch
// the old order.
var dropSave = null;

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

// postReorder saves list's order and returns the request's promise.
function postReorder(list) {
    var slugs = collectSlugs(list);
    if (slugs.length === 0) return null;
    var body = 'slugs=' + encodeURIComponent(slugs.join(', ')) + '&list=' + encodeURIComponent(list);
    return fetch('/plan/reorder', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'X-Requested-With': 'XMLHttpRequest' },
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
        holdRefresh();
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', item.getAttribute('data-slug'));
        return;
    }

    // Calendar week view day move.
    // Done tasks render without draggable; a text drag inside one must not
    // move it either.
    var task = e.target.closest('.calendar-grid-week .calendar-task');
    if (task && task.getAttribute('draggable') === 'true') {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', task.getAttribute('data-slug'));
        e.dataTransfer.setData('application/x-list', task.getAttribute('data-list'));
        task.classList.add('plan-item-dragging');
        holdRefresh();
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
        dropSave = postReorder(draggedEl.getAttribute('data-list'));
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
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'X-Requested-With': 'XMLHttpRequest' },
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

    // Clean up calendar highlights.
    var cells = document.querySelectorAll('.calendar-cell-drop-target');
    for (var i = 0; i < cells.length; i++) {
        cells[i].classList.remove('calendar-cell-drop-target');
    }
    var tasks = document.querySelectorAll('.plan-item-dragging');
    for (var j = 0; j < tasks.length; j++) {
        tasks[j].classList.remove('plan-item-dragging');
    }
    var save = dropSave;
    dropSave = null;
    if (save) {
        save.then(releaseRefresh, releaseRefresh);
    } else {
        releaseRefresh();
    }
});

document.addEventListener('dragleave', function(e) {
    var cell = e.target.closest('.calendar-cell');
    if (cell) cell.classList.remove('calendar-cell-drop-target');
});

// --- Reorder buttons (keyboard, touch and screen reader alternative to drag) ---

function siblingPlanItem(item, step) {
    var el = step < 0 ? item.previousElementSibling : item.nextElementSibling;
    while (el && !el.classList.contains('plan-item')) {
        el = step < 0 ? el.previousElementSibling : el.nextElementSibling;
    }
    if (!el || el.getAttribute('data-list') !== item.getAttribute('data-list')) return null;
    return el;
}

// announceMove reports the item's new position within its list. Moving a
// focused element through the DOM drops focus, so it is restored to btn.
function announceMove(item, btn) {
    btn.focus();
    var list = item.getAttribute('data-list');
    var items = document.querySelectorAll('.plan-today-tasks .plan-item[data-list="' + list + '"]');
    var pos = Array.prototype.indexOf.call(items, item) + 1;
    var title = item.querySelector('.plan-item-title');
    announce('Moved ' + (title ? title.textContent : 'task') + ' to position ' + pos + ' of ' + items.length);
}

function planMove(btn, step) {
    var item = btn.closest('.plan-item');
    if (!item) return;
    var other = siblingPlanItem(item, step);
    if (!other) {
        var title = item.querySelector('.plan-item-title');
        announce((title ? title.textContent : 'Task') + ' is already ' + (step < 0 ? 'first' : 'last'));
        return;
    }
    item.parentNode.insertBefore(item, step < 0 ? other : other.nextSibling);
    postReorder(item.getAttribute('data-list'));
    announceMove(item, btn);
}

function planMoveUp(btn) { planMove(btn, -1); }

function planMoveDown(btn) { planMove(btn, 1); }
