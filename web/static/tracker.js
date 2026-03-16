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

    document.querySelectorAll('.tracker-item').forEach(function(el) {
        if (!activeFilterType) {
            el.style.display = '';
        } else if (activeFilterType === 'category') {
            var tags = (el.getAttribute('data-tags') || '').trim().split(/\s+/);
            el.style.display = tags.indexOf(activeFilterValue) >= 0 ? '' : 'none';
        } else {
            var attr = el.getAttribute('data-' + activeFilterType);
            el.style.display = (attr === activeFilterValue) ? '' : 'none';
        }
    });
}

function toggleItem(btn) {
    if (!btn) return;
    var item = btn.closest('.tracker-item');
    item.classList.toggle('minimised');
    btn.textContent = item.classList.contains('minimised') ? '\u25B8' : '\u25BE';
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
    });
    // Update the toggle-all button label.
    var toggleBtn = document.querySelector('.filter-toggle');
    if (toggleBtn) toggleBtn.textContent = shouldMinimise ? 'expand' : 'collapse';
}

function clearTrackerFilter() {
    var prefix = filterKeyPrefix();
    localStorage.removeItem(prefix + '_filterType');
    localStorage.removeItem(prefix + '_filterValue');
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
    el.scrollIntoView({block: 'nearest'});
})();

// Re-apply filter after HTMX SSE swap.
document.addEventListener('htmx:afterSwap', function() {
    if (activeFilterType) {
        applyFilter();
    }
});
