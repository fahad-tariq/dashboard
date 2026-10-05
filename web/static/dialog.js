// Confirmation dialog replacing browser confirm(). A native <dialog> opened
// with showModal() traps focus, closes on Escape and returns focus itself.
// Live refreshes wait while it is open, so the form it holds stays in place.

var confirmProceed = null;

// openConfirm asks message and calls proceed if the user confirms.
function openConfirm(message, proceed) {
    var modal = document.getElementById('confirm-modal');
    if (!modal || !modal.showModal) {
        if (window.confirm(message)) proceed();
        return;
    }
    confirmProceed = proceed;

    var titleEl = document.getElementById('confirm-modal-title');
    var warningEl = document.getElementById('confirm-modal-warning');
    var confirmBtn = document.getElementById('confirm-modal-ok');
    if (titleEl) titleEl.textContent = message || 'Are you sure?';

    // Only permanent actions warn; trash can be undone.
    var permanent = /permanent|cannot be undone/i.test(message || '');
    if (warningEl) warningEl.hidden = !permanent;
    if (confirmBtn) confirmBtn.classList.toggle('confirm-btn-danger', permanent);

    if (window.liveRefresh) window.liveRefresh.hold('confirm');
    modal.showModal();
    if (confirmBtn) confirmBtn.focus();
}

// confirmAction is the submit-time check for plain (non-htmx) forms: it
// opens the dialog and returns false so the caller cancels the submit.
function confirmAction(form, message) {
    openConfirm(message, function() { form.submit(); });
    return false;
}

function confirmModalCancel() {
    var modal = document.getElementById('confirm-modal');
    if (modal && modal.open) modal.close('cancel');
}

function confirmModalOk() {
    var modal = document.getElementById('confirm-modal');
    if (modal && modal.open) modal.close('ok');
}

(function() {
    var modal = document.getElementById('confirm-modal');
    if (!modal) return;
    modal.addEventListener('close', function() {
        var proceed = confirmProceed;
        confirmProceed = null;
        // Start the confirmed request before releasing the hold; an htmx
        // request then holds refreshes itself until its response lands.
        if (modal.returnValue === 'ok' && proceed) proceed();
        modal.returnValue = '';
        if (window.liveRefresh) window.liveRefresh.release('confirm');
    });
    // A click on the backdrop lands on the dialog element itself.
    modal.addEventListener('click', function(evt) {
        if (evt.target === modal) confirmModalCancel();
    });
    document.getElementById('confirm-modal-cancel').addEventListener('click', confirmModalCancel);
    document.getElementById('confirm-modal-ok').addEventListener('click', confirmModalOk);
})();

// htmx requests from elements with data-confirm ask first, then go ahead
// through htmx's own issueRequest.
document.addEventListener('htmx:confirm', function(evt) {
    var elt = evt.detail.elt;
    var message = elt && elt.getAttribute && elt.getAttribute('data-confirm');
    if (message === null || message === undefined) return;
    evt.preventDefault();
    openConfirm(message, function() { evt.detail.issueRequest(true); });
});
