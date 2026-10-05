/* live.js -- live refresh, morph swaps, fragment responses, focus and
   submission feedback (ES5 compatible).

   Each page has one live container ([data-live]) that refreshes on its
   modules' SSE events and receives the responses of its forms. Both arrive
   as idiomorph swaps, which keep elements (and so focus and listeners) in
   place. Markup declares the state the client owns, and the morph keeps it:

     data-keep-class="a b"   these classes keep their current state
     data-keep-attr="a b"    these attributes keep their current value
     data-morph-skip         the element and its contents are left alone
                             (client-built content, such as upload.js's
                             image area or loaded commentary). It needs an
                             id: idiomorph pairs an element without one with
                             any new element of the same tag, whose content
                             the skip would then swallow.
     data-client-*           attributes set by scripts are never removed
     edited text fields and changed checkboxes keep what the user entered

   Rows ([data-row]) sit in lists ([data-row-list], with data-row-heading
   naming the element to fall back to). When a row's form takes the row out
   of its list, focus moves to the next row, then the previous, then the
   heading. */

/* global window, document, htmx, Idiomorph, showToast */

(function() {
    // --- Refresh gate: refreshes that must wait are queued, never dropped ---

    var holds = {};
    var queued = [];

    function held() {
        for (var k in holds) {
            if (holds.hasOwnProperty(k)) return true;
        }
        return false;
    }

    function hold(reason) {
        holds[reason] = true;
    }

    // release lifts one hold; once none remain, each queued container
    // refreshes once.
    function release(reason) {
        delete holds[reason];
        if (held() || typeof htmx === 'undefined') return;
        var pending = queued;
        queued = [];
        for (var i = 0; i < pending.length; i++) {
            if (document.body.contains(pending[i].elt)) htmx.trigger(pending[i].elt, pending[i].type);
        }
    }

    function enqueue(elt, type) {
        for (var i = 0; i < queued.length; i++) {
            if (queued[i].elt === elt) return;
        }
        queued.push({ elt: elt, type: type });
    }

    // keepValue marks a field whose value a script set, so a morph keeps it
    // like a field the user typed in.
    function keepValue(el) {
        el.liveEdited = true;
    }

    window.liveRefresh = { hold: hold, release: release, keepValue: keepValue };

    // --- Revisions: skip the SSE echo of the tab's own writes ---

    var seenRevision = {};

    function noteRevisions(header) {
        if (!header) return;
        var parts = header.split(' ');
        for (var i = 0; i < parts.length; i++) {
            var kv = parts[i].split('=');
            var rev = parseInt(kv[1], 10);
            if (kv[0] && !isNaN(rev) && !(seenRevision[kv[0]] >= rev)) seenRevision[kv[0]] = rev;
        }
    }

    // isEcho reports whether an SSE event ("<id> <revision>") describes a
    // change this tab already has. Events without a revision (external
    // edits) always refresh.
    function isEcho(evt) {
        var data = evt && evt.detail && evt.detail.data;
        if (typeof data !== 'string') return false;
        var parts = data.split(' ');
        var rev = parseInt(parts[1], 10);
        return !isNaN(rev) && seenRevision[parts[0]] >= rev;
    }

    // Revisions count from zero in each server process, so a reconnect
    // (usually a restart) forgets what this tab has seen.
    document.addEventListener('htmx:sseOpen', function() {
        seenRevision = {};
    });

    document.addEventListener('htmx:confirm', function(evt) {
        if (evt.detail.elt && evt.detail.elt.liveHold) {
            evt.preventDefault();
            return;
        }
        var trigger = evt.detail.triggeringEvent;
        if (!trigger || typeof trigger.type !== 'string' || trigger.type.indexOf('sse:') !== 0) return;
        if (isEcho(trigger)) {
            evt.preventDefault();
            return;
        }
        if (held()) {
            evt.preventDefault();
            enqueue(evt.detail.elt, trigger.type);
        }
    });

    // --- Morph: keep the state the client owns ---

    function words(s) {
        return s ? s.split(/\s+/).filter(Boolean) : [];
    }

    function keepClientState(oldNode, newNode) {
        if (oldNode.nodeType !== 1 || newNode.nodeType !== 1) return true;
        if (oldNode.hasAttribute('data-morph-skip')) return false;
        words(newNode.getAttribute('data-keep-class')).forEach(function(c) {
            newNode.classList.toggle(c, oldNode.classList.contains(c));
        });
        words(newNode.getAttribute('data-keep-attr')).forEach(function(a) {
            if (oldNode.hasAttribute(a)) {
                newNode.setAttribute(a, oldNode.getAttribute(a));
            } else {
                newNode.removeAttribute(a);
            }
        });
        if (oldNode.tagName === 'INPUT' && (oldNode.type === 'checkbox' || oldNode.type === 'radio')) {
            if (oldNode.checked !== oldNode.defaultChecked) newNode.checked = oldNode.checked;
        }
        return true;
    }

    // Text the user typed is kept by refusing the morph's value update. This
    // is the hook idiomorph consults both when the server's field has a value
    // and when it has none (which would otherwise clear the field).
    function keepAttribute(name, node, mutation) {
        if (name === 'value' && node.liveEdited) return false;
        if (mutation === 'remove' && name.indexOf('data-client-') === 0) return false;
        return true;
    }

    document.addEventListener('input', function(evt) {
        var el = evt.target;
        if (el && (el.tagName === 'TEXTAREA' || (el.tagName === 'INPUT' && el.type !== 'checkbox' && el.type !== 'radio'))) {
            el.liveEdited = true;
        }
    });

    // A reset form holds the server's values again. Hidden fields are left:
    // reset does not change them, and only scripts write to them.
    document.addEventListener('reset', function(evt) {
        var fields = evt.target.elements || [];
        for (var i = 0; i < fields.length; i++) {
            if (fields[i].type !== 'hidden') fields[i].liveEdited = false;
        }
    });

    document.addEventListener('DOMContentLoaded', function() {
        if (typeof Idiomorph === 'undefined') return;
        Idiomorph.defaults.ignoreActiveValue = true;
        Idiomorph.defaults.callbacks.beforeNodeMorphed = keepClientState;
        Idiomorph.defaults.callbacks.beforeAttributeUpdated = keepAttribute;
        Idiomorph.defaults.callbacks.beforeNodeRemoved = function(node) {
            return !(node.nodeType === 1 && node.hasAttribute('data-morph-skip'));
        };
        Idiomorph.defaults.callbacks.afterNodeAdded = function(node) {
            if (node.nodeType === 1) document.dispatchEvent(new CustomEvent('live:added', { detail: node }));
        };
    });

    // A form's own successful submission clears what was typed, so the
    // server's values replace it instead of being kept as edits. The morph
    // leaves the focused field alone (ignoreActiveValue), so whatever has
    // focus in the submitted form is blurred first and refocused after the
    // swap: a focused field would keep showing its old value, and idiomorph
    // also pairs nodes differently around the focused element and its
    // parents, which built a row moved between lists wrongly when its
    // button kept focus (second Phase 8 CI run).
    var refocusAfterSwap = null;

    // An error flash (say an invalid cadence) means the form was not
    // accepted, so what was typed stays for the user to correct.
    function flashIsError(xhr) {
        var raw = xhr.getResponseHeader('HX-Trigger');
        if (!raw || raw.charAt(0) !== '{') return false;
        try {
            var flash = JSON.parse(raw)['dash:flash'];
            return !!(flash && flash.error);
        } catch (e) {
            return false;
        }
    }

    document.addEventListener('htmx:beforeSwap', function(evt) {
        var elt = evt.detail.requestConfig && evt.detail.requestConfig.elt;
        if (!elt || elt.tagName !== 'FORM' || evt.detail.xhr.status >= 300 || flashIsError(evt.detail.xhr)) return;
        var active = document.activeElement;
        if (active && active !== elt && elt.contains(active)) {
            refocusAfterSwap = active;
            active.blur();
        }
        elt.reset();
        elt.dispatchEvent(new CustomEvent('live:form-accepted', { bubbles: true }));
    });

    // --- Submission feedback ---

    // Buttons are marked aria-disabled rather than disabled: disabling the
    // focused button would drop focus to <body>. A second submit while the
    // first is in flight is refused in htmx:confirm below.
    function setBusy(form, busy) {
        var buttons = form.querySelectorAll('button');
        for (var i = 0; i < buttons.length; i++) {
            if (busy) {
                buttons[i].setAttribute('aria-disabled', 'true');
            } else {
                buttons[i].removeAttribute('aria-disabled');
            }
        }
        var row = rowOf(form);
        if (!row) return;
        if (busy) {
            row.setAttribute('aria-busy', 'true');
        } else {
            row.removeAttribute('aria-busy');
        }
    }

    // --- Focus after change ---

    var FOCUS_TARGET = '[data-row-focus]';
    var pendingFocus = null;

    function listOf(row) {
        return row.parentNode ? row.parentNode.closest('[data-row-list]') : null;
    }

    function rowsIn(list) {
        var all = list.querySelectorAll('[data-row]');
        var rows = [];
        for (var i = 0; i < all.length; i++) {
            if (listOf(all[i]) === list) rows.push(all[i]);
        }
        return rows;
    }

    function visible(el) {
        return !!el && document.body.contains(el) && el.getClientRects().length > 0;
    }

    // rowOf finds the row a form belongs to. A form can also sit outside its
    // row, in an element naming the row with data-row-of (house detail rows).
    function rowOf(form) {
        var row = form.closest('[data-row]');
        if (row) return row;
        var owner = form.closest('[data-row-of]');
        return owner ? document.getElementById(owner.getAttribute('data-row-of')) : null;
    }

    function rememberRow(form) {
        var row = rowOf(form);
        var list = row && listOf(row);
        if (!row || !row.id || !list) {
            pendingFocus = null;
            return;
        }
        var rows = rowsIn(list);
        var idx = rows.indexOf(row);
        var next = [];
        for (var i = idx + 1; i < rows.length; i++) next.push(rows[i].id);
        for (var j = idx - 1; j >= 0; j--) next.push(rows[j].id);
        var active = document.activeElement;
        pendingFocus = {
            form: form,
            row: row.id,
            list: list.id,
            next: next,
            heading: list.getAttribute('data-row-heading'),
            active: active && active.id
        };
    }

    function focusRow(row) {
        var target = row.querySelector(FOCUS_TARGET) || row;
        target.focus();
    }

    function focusLost() {
        var active = document.activeElement;
        return !active || active === document.body || !visible(active);
    }

    // restoreFocus runs after a form's response settles. If the row stayed
    // in its list, it only repairs lost focus. If the row left its list,
    // focus that is lost or still inside the moved row goes to the next row:
    // a morph can keep the focused button alive inside the moved row (now
    // likely a different action), so "lost" alone is not enough.
    function restoreFocus() {
        var p = pendingFocus;
        pendingFocus = null;
        if (!p) return;
        var row = document.getElementById(p.row);
        var list = document.getElementById(p.list);
        var stayed = row && list && listOf(row) === list && visible(row);
        var active = document.activeElement;
        if (stayed && !focusLost()) return;
        if (!stayed && !focusLost() && !(row && row.contains(active))) return;
        if (stayed) {
            var before = p.active && document.getElementById(p.active);
            if (visible(before)) {
                before.focus();
            } else {
                focusRow(row);
            }
            return;
        }
        for (var i = 0; i < p.next.length; i++) {
            var other = document.getElementById(p.next[i]);
            if (other && listOf(other) === list && visible(other)) {
                focusRow(other);
                return;
            }
        }
        var heading = (p.heading && document.getElementById(p.heading)) || document.querySelector('[data-live] h1');
        if (heading) heading.focus();
    }

    // Refreshes wait while a form request is in flight: its response is a
    // fresh render, and a refresh fetched alongside it could arrive later
    // with the state from before.
    var requestHolds = 0;

    document.addEventListener('htmx:beforeRequest', function(evt) {
        var elt = evt.detail.elt;
        if (!elt || elt.tagName !== 'FORM') return;
        elt.liveHold = 'request-' + (++requestHolds);
        hold(elt.liveHold);
        rememberRow(elt);
        setBusy(elt, true);
    });

    document.addEventListener('htmx:afterRequest', function(evt) {
        var elt = evt.detail.elt;
        if (evt.detail.successful) noteRevisions(evt.detail.xhr && evt.detail.xhr.getResponseHeader('Dash-Revisions'));
        if (elt && elt.liveHold) {
            release(elt.liveHold);
            elt.liveHold = null;
        }
        if (elt && elt.tagName === 'FORM' && document.body.contains(elt)) setBusy(elt, false);
        if (!evt.detail.successful && pendingFocus) {
            // Nothing moved; put focus back where the user left it.
            var active = pendingFocus.active && document.getElementById(pendingFocus.active);
            pendingFocus = null;
            if (focusLost() && visible(active)) active.focus();
        }
    });

    document.addEventListener('htmx:afterSettle', function(evt) {
        var back = refocusAfterSwap;
        refocusAfterSwap = null;
        if (visible(back) && focusLost()) back.focus();
        var config = evt.detail.requestConfig;
        if (pendingFocus && config && config.elt === pendingFocus.form) restoreFocus();
    });

    // --- Flash, undo and errors ---

    function liveContainer() {
        return document.querySelector('[data-live]');
    }

    function undo(path) {
        var live = liveContainer();
        if (!live || typeof htmx === 'undefined') return;
        htmx.ajax('POST', path, { source: live, target: live, swap: 'morph', select: '[data-live]' });
    }

    document.addEventListener('dash:flash', function(evt) {
        var f = evt.detail || {};
        if (!f.message) return;
        showToast(f.message, {
            error: !!f.error,
            undo: f.undo ? function() { undo(f.undo); } : null
        });
    });

    document.addEventListener('htmx:responseError', function(evt) {
        var text = (evt.detail.xhr.responseText || '').trim();
        if (text.length > 200 || text.indexOf('<') !== -1) text = '';
        showToast(text || 'Something went wrong. Nothing was changed.', { error: true });
    });

    document.addEventListener('htmx:sendError', function() {
        showToast('Could not reach the server. Nothing was changed.', { error: true });
    });
})();
