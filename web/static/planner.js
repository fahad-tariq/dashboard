/* planner.js -- client-side picker filter + SSE guard (ES5 compatible) */

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
