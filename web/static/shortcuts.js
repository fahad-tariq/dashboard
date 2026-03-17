// Keyboard shortcuts and search overlay (ES5).

var searchOverlay = document.getElementById('search-overlay');
var searchInput = document.getElementById('search-input');
var searchResults = document.getElementById('search-results');
var shortcutHelp = document.getElementById('shortcut-help');
var searchDebounce = null;
var searchActiveIdx = -1;
var gPending = false;

function isInputFocused() {
    var el = document.activeElement;
    if (!el) return false;
    var tag = el.tagName.toLowerCase();
    return tag === 'input' || tag === 'textarea' || tag === 'select' || el.isContentEditable;
}

function openSearch() {
    searchOverlay.classList.add('visible');
    searchInput.value = '';
    searchResults.innerHTML = '';
    searchActiveIdx = -1;
    setTimeout(function() { searchInput.focus(); }, 10);
}

function closeSearch() {
    searchOverlay.classList.remove('visible');
    searchInput.value = '';
    searchResults.innerHTML = '';
    searchActiveIdx = -1;
}

function openShortcutHelp() {
    shortcutHelp.classList.add('visible');
}

function closeShortcutHelp() {
    shortcutHelp.classList.remove('visible');
}

function doSearch(query) {
    if (!query) {
        searchResults.innerHTML = '';
        searchActiveIdx = -1;
        return;
    }
    var xhr = new XMLHttpRequest();
    xhr.open('GET', '/search?q=' + encodeURIComponent(query), true);
    xhr.onreadystatechange = function() {
        if (xhr.readyState === 4 && xhr.status === 200) {
            searchResults.innerHTML = xhr.responseText;
            searchActiveIdx = -1;
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
    }
    searchActiveIdx = idx;
    if (idx >= 0 && idx < links.length) {
        links[idx].classList.add('search-result-active');
        links[idx].scrollIntoView({ block: 'nearest' });
    }
}

if (searchInput) {
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
            var next = searchActiveIdx + 1;
            if (next >= links.length) next = 0;
            setActiveResult(next);
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            var prev = searchActiveIdx - 1;
            if (prev < 0) prev = links.length - 1;
            setActiveResult(prev);
        } else if (e.key === 'Enter') {
            e.preventDefault();
            if (searchActiveIdx >= 0 && searchActiveIdx < links.length) {
                window.location.href = links[searchActiveIdx].getAttribute('href');
                closeSearch();
            }
        } else if (e.key === 'Escape') {
            e.preventDefault();
            closeSearch();
        }
    });
}

document.addEventListener('keydown', function(e) {
    // Close any open modal on Escape.
    if (e.key === 'Escape') {
        if (searchOverlay.classList.contains('visible')) {
            closeSearch();
            return;
        }
        if (shortcutHelp.classList.contains('visible')) {
            closeShortcutHelp();
            return;
        }
    }

    // Ctrl+K / Cmd+K to open search (works even when input focused).
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        if (searchOverlay.classList.contains('visible')) {
            closeSearch();
        } else {
            openSearch();
        }
        return;
    }

    // All remaining shortcuts require no input focus.
    if (isInputFocused()) return;

    // "/" opens search.
    if (e.key === '/') {
        e.preventDefault();
        openSearch();
        return;
    }

    // "?" opens shortcut help.
    if (e.key === '?') {
        e.preventDefault();
        if (shortcutHelp.classList.contains('visible')) {
            closeShortcutHelp();
        } else {
            openShortcutHelp();
        }
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
        switch (e.key) {
            case 'h': window.location.href = '/'; break;
            case 't': window.location.href = '/todos'; break;
            case 'o': window.location.href = '/goals'; break;
            case 'i': window.location.href = '/ideas'; break;
            case 'f': window.location.href = '/family'; break;
        }
    }
});
