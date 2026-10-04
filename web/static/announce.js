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

var toastTimer = null;

// showToast displays msg as text only and announces it. The toast itself is
// not a live region, so the message is read once.
function showToast(msg) {
    var el = document.getElementById('toast');
    if (!el) return;
    el.querySelector('.toast-text').textContent = msg;
    el.hidden = false;
    window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(function() { el.hidden = true; }, 6000);
    announce(msg);
}

window.announce = announce;
window.showToast = showToast;
