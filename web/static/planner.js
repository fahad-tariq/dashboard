/* planner.js -- client-side picker filter + drag-and-drop (ES5 compatible) */

/* global window, document, fetch, FormData, htmx */

window.planDragInProgress = false;

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

(function() {
    var container = document.querySelector('.plan-today-tasks');
    if (!container) return;

    container.addEventListener('dragstart', function(e) {
        var item = e.target.closest('.plan-item');
        if (!item || item.classList.contains('plan-item-done')) {
            e.preventDefault();
            return;
        }
        draggedEl = item;
        item.classList.add('plan-item-dragging');
        window.planDragInProgress = true;
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', item.getAttribute('data-slug'));
    });

    container.addEventListener('dragover', function(e) {
        if (!draggedEl) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';

        var target = e.target.closest('.plan-item');
        if (!target || target === draggedEl) {
            clearDropIndicators();
            return;
        }
        if (target.getAttribute('data-list') !== draggedEl.getAttribute('data-list')) {
            clearDropIndicators();
            return;
        }

        clearDropIndicators();
        var rect = target.getBoundingClientRect();
        var mid = rect.top + rect.height / 2;
        if (e.clientY < mid) {
            target.classList.add('plan-drop-above');
        } else {
            target.classList.add('plan-drop-below');
        }
    });

    container.addEventListener('drop', function(e) {
        e.preventDefault();
        if (!draggedEl) return;

        var target = e.target.closest('.plan-item');
        if (!target || target === draggedEl) return;
        if (target.getAttribute('data-list') !== draggedEl.getAttribute('data-list')) return;

        var rect = target.getBoundingClientRect();
        var mid = rect.top + rect.height / 2;
        if (e.clientY < mid) {
            target.parentNode.insertBefore(draggedEl, target);
        } else {
            target.parentNode.insertBefore(draggedEl, target.nextSibling);
        }

        var list = draggedEl.getAttribute('data-list');
        postReorder(list);
    });

    container.addEventListener('dragend', function() {
        if (draggedEl) {
            draggedEl.classList.remove('plan-item-dragging');
        }
        draggedEl = null;
        clearDropIndicators();
        window.planDragInProgress = false;
        // No manual SSE trigger needed: postReorder writes the file,
        // the file watcher sends an SSE event, and planDragInProgress
        // is already false so the swap proceeds normally.
    });
})();

// --- Calendar week view: drag tasks between days ---

(function() {
    var grid = document.querySelector('.calendar-grid-week');
    if (!grid) return;

    grid.addEventListener('dragstart', function(e) {
        var task = e.target.closest('.calendar-task');
        if (!task) return;
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', task.getAttribute('data-slug'));
        e.dataTransfer.setData('application/x-list', task.getAttribute('data-list'));
        task.classList.add('plan-item-dragging');
        window.planDragInProgress = true;
    });

    grid.addEventListener('dragover', function(e) {
        var cell = e.target.closest('.calendar-cell');
        if (!cell || !cell.getAttribute('data-date')) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        // Clear other highlights.
        var cells = grid.querySelectorAll('.calendar-cell-drop-target');
        for (var i = 0; i < cells.length; i++) {
            if (cells[i] !== cell) cells[i].classList.remove('calendar-cell-drop-target');
        }
        cell.classList.add('calendar-cell-drop-target');
    });

    grid.addEventListener('dragleave', function(e) {
        var cell = e.target.closest('.calendar-cell');
        if (cell) cell.classList.remove('calendar-cell-drop-target');
    });

    grid.addEventListener('drop', function(e) {
        e.preventDefault();
        var cell = e.target.closest('.calendar-cell');
        if (!cell) return;
        var date = cell.getAttribute('data-date');
        if (!date) return;

        var slug = e.dataTransfer.getData('text/plain');
        var list = e.dataTransfer.getData('application/x-list');
        if (!slug || !list) return;

        // Map list aliases.
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

    grid.addEventListener('dragend', function() {
        window.planDragInProgress = false;
        var cells = grid.querySelectorAll('.calendar-cell-drop-target');
        for (var i = 0; i < cells.length; i++) {
            cells[i].classList.remove('calendar-cell-drop-target');
        }
        var tasks = grid.querySelectorAll('.plan-item-dragging');
        for (var j = 0; j < tasks.length; j++) {
            tasks[j].classList.remove('plan-item-dragging');
        }
    });
})();

// --- Mobile fallback: up/down arrow buttons ---

function planMoveUp(btn) {
    var item = btn.closest('.plan-item');
    if (!item) return;
    var prev = item.previousElementSibling;
    // Skip non-plan-item siblings (like list labels).
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
    // Skip non-plan-item siblings (like list labels).
    while (next && !next.classList.contains('plan-item')) {
        next = next.nextElementSibling;
    }
    if (!next) return;
    if (next.getAttribute('data-list') !== item.getAttribute('data-list')) return;
    item.parentNode.insertBefore(item, next.nextSibling);
    postReorder(item.getAttribute('data-list'));
}
