// Keyboard shortcuts and search (ES5).
//
// Search and shortcut help are native <dialog>s opened with showModal(), so
// the browser traps focus, closes them on Escape and returns focus. Search
// is a combobox: focus stays in the input and aria-activedescendant points
// at the highlighted result.

var searchOverlay = document.getElementById('search-overlay');
var searchInput = document.getElementById('search-input');
var searchResults = document.getElementById('search-results');
var shortcutHelp = document.getElementById('shortcut-help');
var searchDebounce = null;
var searchActiveIdx = -1;
var gPending = false;
var searchReturnFocus = null;

function isInputFocused() {
    var el = document.activeElement;
    if (!el) return false;
    var tag = el.tagName.toLowerCase();
    return tag === 'input' || tag === 'textarea' || tag === 'select' || el.isContentEditable;
}

function anyDialogOpen() {
    return !!document.querySelector('dialog[open]');
}

function openSearch() {
    if (searchOverlay.open) return;
    searchReturnFocus = document.activeElement;
    clearResults();
    searchInput.value = '';
    searchOverlay.showModal();
    searchInput.focus();
}

function closeSearch() {
    if (searchOverlay.open) searchOverlay.close();
}

function clearResults() {
    searchResults.innerHTML = '';
    setActiveResult(-1);
    searchInput.setAttribute('aria-expanded', 'false');
}

function doSearch(query) {
    if (!query) {
        clearResults();
        return;
    }
    var xhr = new XMLHttpRequest();
    xhr.open('GET', '/search?q=' + encodeURIComponent(query), true);
    xhr.onreadystatechange = function() {
        if (xhr.readyState === 4 && xhr.status === 200 && searchOverlay.open) {
            searchResults.innerHTML = xhr.responseText;
            setActiveResult(-1);
            searchInput.setAttribute('aria-expanded', String(getSearchLinks().length > 0));
        }
    };
    xhr.send();
}

function getSearchLinks() {
    return searchResults.querySelectorAll('.search-result');
}

function setActiveResult(idx) {
    var links = getSearchLinks();
    for (var i = 0; i < links.length; i++) {
        links[i].classList.remove('search-result-active');
        links[i].setAttribute('aria-selected', 'false');
    }
    searchActiveIdx = idx;
    if (idx >= 0 && idx < links.length) {
        links[idx].classList.add('search-result-active');
        links[idx].setAttribute('aria-selected', 'true');
        links[idx].scrollIntoView({ block: 'nearest' });
        searchInput.setAttribute('aria-activedescendant', links[idx].id);
    } else {
        searchInput.removeAttribute('aria-activedescendant');
    }
}

if (searchOverlay) {
    searchOverlay.addEventListener('close', function() {
        searchInput.value = '';
        clearResults();
        var back = searchReturnFocus;
        searchReturnFocus = null;
        if (back && back !== searchInput && document.body.contains(back)) {
            // preventScroll: a same-page result link has just set the hash.
            back.focus({ preventScroll: true });
        }
    });
    searchOverlay.addEventListener('click', function(e) {
        // The backdrop belongs to the dialog element itself; a result link
        // navigates, so the dialog closes behind it.
        if (e.target === searchOverlay || e.target.closest('.search-result')) closeSearch();
    });

    searchInput.addEventListener('input', function() {
        var val = searchInput.value;
        if (searchDebounce) clearTimeout(searchDebounce);
        searchDebounce = setTimeout(function() {
            doSearch(val);
        }, 200);
    });

    searchInput.addEventListener('keydown', function(e) {
        var links = getSearchLinks();
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            if (links.length) setActiveResult(searchActiveIdx + 1 >= links.length ? 0 : searchActiveIdx + 1);
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            if (links.length) setActiveResult(searchActiveIdx - 1 < 0 ? links.length - 1 : searchActiveIdx - 1);
        } else if (e.key === 'Enter') {
            e.preventDefault();
            if (searchActiveIdx >= 0 && searchActiveIdx < links.length) {
                var href = links[searchActiveIdx].getAttribute('href');
                closeSearch();
                window.location.href = href;
            }
        }
    });
}

function openShortcutHelp() {
    if (!shortcutHelp.open) shortcutHelp.showModal();
}

function closeShortcutHelp() {
    if (shortcutHelp.open) shortcutHelp.close();
}

if (shortcutHelp) {
    shortcutHelp.addEventListener('click', function(e) {
        if (e.target === shortcutHelp || e.target.closest('[data-close-dialog]')) closeShortcutHelp();
    });
}

document.addEventListener('keydown', function(e) {
    // Ctrl+K / Cmd+K toggles search, even from a text field.
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        if (searchOverlay.open) {
            closeSearch();
        } else if (!anyDialogOpen()) {
            openSearch();
        }
        return;
    }

    // An open dialog handles its own keys, Escape included.
    if (anyDialogOpen()) return;

    // Escape closes the innermost open thing: select mode, the "more" menu,
    // then the mobile nav.
    if (e.key === 'Escape') {
        if (typeof bulkSelectActive !== 'undefined' && bulkSelectActive) {
            exitSelectMode();
            return;
        }
        if (moreMenu && !moreMenu.hidden) {
            setMoreOpen(false);
            moreBtn.focus();
            return;
        }
        var navLinks = document.getElementById('nav-links');
        if (navLinks && navLinks.classList.contains('nav-links-open')) {
            navLinks.classList.remove('nav-links-open');
            var hamburger = document.getElementById('nav-hamburger');
            if (hamburger) {
                hamburger.setAttribute('aria-expanded', 'false');
                hamburger.textContent = '\u2630';
                hamburger.focus();
            }
        }
        return;
    }

    // All remaining shortcuts require no input focus and no modifier.
    if (isInputFocused() || e.ctrlKey || e.metaKey || e.altKey) return;

    if (e.key === '/') {
        e.preventDefault();
        openSearch();
        return;
    }

    if (e.key === '?') {
        e.preventDefault();
        openShortcutHelp();
        return;
    }

    // "u" undoes the change the toast offers to undo.
    if (e.key === 'u' && !gPending && typeof toastUndoAvailable === 'function' && toastUndoAvailable()) {
        e.preventDefault();
        runToastUndo();
        return;
    }

    // "j" and "k" move focus to the next or previous row on list pages
    // and the homepage plan.
    if ((e.key === 'j' || e.key === 'k') && !gPending && moveRowFocus(e.key === 'j' ? 1 : -1)) {
        e.preventDefault();
        return;
    }

    // "g" prefix for go-to shortcuts.
    if (e.key === 'g' && !gPending) {
        gPending = true;
        setTimeout(function() { gPending = false; }, 1000);
        return;
    }

    if (gPending) {
        gPending = false;
        var link = goTargets()[e.key];
        if (link) window.location.href = link;
    }
});

// moveRowFocus focuses the next (step 1) or previous (-1) visible row's focus
// target, starting from the row holding focus or, with none, the first row.
// It reports whether there were rows to move through.
function moveRowFocus(step) {
    var all = document.querySelectorAll('main [data-row] [data-row-focus]');
    var targets = [];
    for (var i = 0; i < all.length; i++) {
        if (all[i].getClientRects().length > 0) targets.push(all[i]);
    }
    if (targets.length === 0) return false;
    var current = document.activeElement && document.activeElement.closest ? document.activeElement.closest('[data-row]') : null;
    var index = -1;
    for (var j = 0; j < targets.length; j++) {
        if (targets[j].closest('[data-row]') === current) { index = j; break; }
    }
    var next = index === -1 ? (step > 0 ? 0 : targets.length - 1) : Math.min(Math.max(index + step, 0), targets.length - 1);
    targets[next].focus();
    if (targets[next].scrollIntoView) targets[next].scrollIntoView({ block: 'nearest' });
    return true;
}

// goTargets maps each "g" shortcut key to its nav link, read from the
// data-shortcut attributes the server renders from the module registry.
function goTargets() {
    var map = {};
    var links = document.querySelectorAll('#nav-links a[data-shortcut]');
    for (var i = 0; i < links.length; i++) {
        map[links[i].getAttribute('data-shortcut')] = links[i].getAttribute('href');
    }
    return map;
}

// "more" nav disclosure.
var moreBtn = document.getElementById('nav-more-btn');
var moreMenu = document.getElementById('nav-more-menu');

function setMoreOpen(open) {
    moreMenu.hidden = !open;
    moreBtn.setAttribute('aria-expanded', String(open));
}

if (moreBtn && moreMenu) {
    moreBtn.addEventListener('click', function() {
        setMoreOpen(moreMenu.hidden);
    });
    document.addEventListener('click', function(e) {
        if (!moreMenu.hidden && !moreBtn.contains(e.target) && !moreMenu.contains(e.target)) {
            setMoreOpen(false);
        }
    });
}
