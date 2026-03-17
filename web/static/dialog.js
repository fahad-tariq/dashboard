// Styled confirmation dialogs replacing browser confirm().

var confirmPendingForm = null;
var confirmPreviousFocus = null;

function confirmAction(form, message) {
    var modal = document.getElementById('confirm-modal');
    if (!modal) return true;

    confirmPendingForm = form;
    confirmPreviousFocus = document.activeElement;

    var titleEl = document.getElementById('confirm-modal-title');
    var warningEl = document.getElementById('confirm-modal-warning');
    var confirmBtn = document.getElementById('confirm-modal-ok');

    if (titleEl) titleEl.textContent = message || 'Are you sure?';

    var isDestructive = /delete|remove|drop/i.test(message || '');
    if (warningEl) {
        warningEl.style.display = isDestructive ? '' : 'none';
    }
    if (confirmBtn) {
        if (isDestructive) {
            confirmBtn.classList.add('confirm-btn-danger');
        } else {
            confirmBtn.classList.remove('confirm-btn-danger');
        }
    }

    modal.classList.add('visible');

    if (confirmBtn) confirmBtn.focus();

    return false;
}

function confirmModalCancel() {
    var modal = document.getElementById('confirm-modal');
    if (modal) modal.classList.remove('visible');
    if (confirmPreviousFocus) {
        confirmPreviousFocus.focus();
        confirmPreviousFocus = null;
    }
    confirmPendingForm = null;
}

function confirmModalOk() {
    var modal = document.getElementById('confirm-modal');
    if (modal) modal.classList.remove('visible');
    if (confirmPendingForm) {
        var form = confirmPendingForm;
        confirmPendingForm = null;
        confirmPreviousFocus = null;
        form.submit();
    }
}

// Escape key closes modal.
document.addEventListener('keydown', function(evt) {
    if (evt.key === 'Escape') {
        var modal = document.getElementById('confirm-modal');
        if (modal && modal.classList.contains('visible')) {
            confirmModalCancel();
        }
    }
});

// Focus trap within the modal.
document.addEventListener('keydown', function(evt) {
    if (evt.key !== 'Tab') return;
    var modal = document.getElementById('confirm-modal');
    if (!modal || !modal.classList.contains('visible')) return;

    var cancelBtn = document.getElementById('confirm-modal-cancel');
    var okBtn = document.getElementById('confirm-modal-ok');
    if (!cancelBtn || !okBtn) return;

    var focusable = [cancelBtn, okBtn];
    var first = focusable[0];
    var last = focusable[focusable.length - 1];

    if (evt.shiftKey) {
        if (document.activeElement === first) {
            evt.preventDefault();
            last.focus();
        }
    } else {
        if (document.activeElement === last) {
            evt.preventDefault();
            first.focus();
        }
    }
});

// Close modal on overlay click (outside the dialog).
document.addEventListener('click', function(evt) {
    var modal = document.getElementById('confirm-modal');
    if (!modal || !modal.classList.contains('visible')) return;
    if (evt.target === modal) {
        confirmModalCancel();
    }
});
