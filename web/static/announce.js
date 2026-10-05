/* announce.js -- screen reader announcements and the toast (ES5 compatible) */

/* global window, document */

function announce(msg) {
    var el = document.getElementById('announcer');
    if (!el) return;
    // Clearing first and setting on the next tick makes a repeated message
    // ("Moved X to position 2 of 3" twice) announce again.
    el.textContent = '';
    window.setTimeout(function() { el.textContent = msg; }, 50);
}

// The toast stays at least this long, and longer while the pointer or focus
// is on it.
var TOAST_MS = 10000;

var toastTimer = null;
var toastUndo = null;
var toastPaused = false;

function hideToast() {
    var el = document.getElementById('toast');
    window.clearTimeout(toastTimer);
    toastTimer = null;
    toastUndo = null;
    toastPaused = false;
    if (!el) return;
    var focusInside = el.contains(document.activeElement);
    el.hidden = true;
    if (focusInside) {
        var main = document.getElementById('main');
        if (main) main.focus();
    }
}

function scheduleHide() {
    window.clearTimeout(toastTimer);
    if (!toastPaused) toastTimer = window.setTimeout(hideToast, TOAST_MS);
}

// showToast displays msg as text only and announces it. The toast itself is
// not a live region, so the message is read once. opts.undo, a function,
// adds an undo button (also reachable with the "u" shortcut); opts.error
// styles the toast as an error.
function showToast(msg, opts) {
    opts = opts || {};
    var el = document.getElementById('toast');
    if (!el) return;
    el.querySelector('.toast-text').textContent = msg;
    el.classList.toggle('toast-error', !!opts.error);
    toastUndo = opts.undo || null;
    el.querySelector('.toast-undo').hidden = !toastUndo;
    el.hidden = false;
    scheduleHide();
    announce(toastUndo ? msg + ' Press U to undo.' : msg);
}

// toastUndoAvailable reports whether the visible toast offers undo.
function toastUndoAvailable() {
    var el = document.getElementById('toast');
    return !!toastUndo && !!el && !el.hidden;
}

function runToastUndo() {
    var fn = toastUndo;
    hideToast();
    if (fn) fn();
}

(function() {
    var el = document.getElementById('toast');
    if (!el) return;
    function pause() {
        toastPaused = true;
        window.clearTimeout(toastTimer);
    }
    function resume() {
        if (el.matches(':hover') || el.contains(document.activeElement)) return;
        toastPaused = false;
        if (!el.hidden) scheduleHide();
    }
    el.addEventListener('mouseenter', pause);
    el.addEventListener('focusin', pause);
    el.addEventListener('mouseleave', resume);
    el.addEventListener('focusout', function() { window.setTimeout(resume, 0); });
    el.querySelector('.toast-undo').addEventListener('click', runToastUndo);
    el.querySelector('.toast-close').addEventListener('click', hideToast);
})();

window.announce = announce;
window.showToast = showToast;
