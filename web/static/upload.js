(function() {
    'use strict';

    function initImageUpload(formSelector) {
        document.querySelectorAll(formSelector).forEach(function(form) {
            setupForm(form);
        });
    }

    function setupForm(form) {
        // Skip if already initialised.
        if (form.dataset.uploadInit) return;
        form.dataset.uploadInit = '1';

        // Find or create hidden input for image filenames.
        var hidden = form.querySelector('input[name="images"]');
        if (!hidden) {
            hidden = document.createElement('input');
            hidden.type = 'hidden';
            hidden.name = 'images';
            form.appendChild(hidden);
        }

        // Create upload area.
        var area = document.createElement('div');
        area.className = 'upload-area';

        var fileInput = document.createElement('input');
        fileInput.type = 'file';
        fileInput.accept = 'image/*';
        fileInput.multiple = true;
        fileInput.className = 'upload-file-input';

        var label = document.createElement('label');
        label.className = 'upload-label action-btn';
        label.textContent = 'attach image';
        label.appendChild(fileInput);

        var gallery = document.createElement('div');
        gallery.className = 'image-gallery';

        area.appendChild(label);
        area.appendChild(gallery);

        // Insert before submit button.
        var submit = form.querySelector('button[type="submit"]');
        if (submit) {
            form.insertBefore(area, submit);
        } else {
            form.appendChild(area);
        }

        // Load existing images. Entries may be "filename" or "filename|caption".
        var existing = hidden.value ? hidden.value.split(',').filter(Boolean) : [];
        var filenames = [];
        existing.forEach(function(entry) {
            var parts = splitOnFirstPipe(entry);
            filenames.push(parts[0]);
            addThumbnail(gallery, hidden, parts[0], parts[1]);
        });
        // Rewrite hidden value to filenames only; captions travel via caption-N fields.
        hidden.value = filenames.join(',');

        // File input handler.
        fileInput.addEventListener('change', function() {
            Array.from(fileInput.files).forEach(function(file) {
                uploadFile(file, function(filename) {
                    appendImage(hidden, gallery, filename, '');
                });
            });
            fileInput.value = '';
        });

        // Paste handler on textareas in the form.
        form.querySelectorAll('textarea').forEach(function(ta) {
            ta.addEventListener('paste', function(e) {
                var items = (e.clipboardData || {}).items;
                if (!items) return;
                for (var i = 0; i < items.length; i++) {
                    if (items[i].type.indexOf('image/') === 0) {
                        e.preventDefault();
                        var blob = items[i].getAsFile();
                        uploadFile(blob, function(filename) {
                            appendImage(hidden, gallery, filename, '');
                        });
                        break;
                    }
                }
            });
        });
    }

    // Split a string on the first pipe character only.
    function splitOnFirstPipe(s) {
        var idx = s.indexOf('|');
        if (idx === -1) return [s.trim(), ''];
        return [s.substring(0, idx).trim(), s.substring(idx + 1).trim()];
    }

    function uploadFile(file, onSuccess) {
        var formData = new FormData();
        formData.append('file', file);

        fetch('/upload', { method: 'POST', body: formData })
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (data.filename) {
                    onSuccess(data.filename);
                }
            })
            .catch(function(err) {
                console.error('Upload failed:', err);
                var area = document.querySelector('.upload-area');
                if (area) {
                    var msg = document.createElement('div');
                    msg.className = 'flash-msg flash-msg-error';
                    msg.textContent = 'Upload failed';
                    area.parentNode.insertBefore(msg, area);
                    setTimeout(function() { msg.remove(); }, 5000);
                }
            });
    }

    function appendImage(hidden, gallery, filename, caption) {
        // Hidden field stores filenames only (no captions).
        var filenames = hidden.value ? hidden.value.split(',').filter(Boolean) : [];
        filenames.push(filename);
        hidden.value = filenames.join(',');
        addThumbnail(gallery, hidden, filename, caption);
    }

    function addThumbnail(gallery, hidden, filename, caption) {
        var wrap = document.createElement('div');
        wrap.className = 'image-thumb-wrap';
        wrap.setAttribute('data-filename', filename);

        var img = document.createElement('img');
        img.src = '/uploads/' + filename;
        img.className = 'image-thumb';
        img.loading = 'lazy';

        var captionInput = document.createElement('input');
        captionInput.type = 'text';
        captionInput.className = 'image-caption-input';
        captionInput.placeholder = 'caption';
        captionInput.value = caption || '';

        // Assign sequential caption-N name.
        var idx = gallery.querySelectorAll('.image-thumb-wrap').length;
        captionInput.name = 'caption-' + idx;

        // Strip forbidden characters on input.
        captionInput.addEventListener('input', function() {
            captionInput.value = captionInput.value.replace(/[|,\]]/g, '');
        });

        var remove = document.createElement('button');
        remove.type = 'button';
        remove.className = 'image-thumb-remove';
        remove.textContent = '\u00d7';
        remove.addEventListener('click', function() {
            var filenames = hidden.value.split(',').filter(function(f) { return f !== filename; });
            hidden.value = filenames.join(',');
            wrap.remove();
            // Re-index all remaining caption inputs sequentially.
            reindexCaptions(gallery);
        });

        wrap.appendChild(img);
        wrap.appendChild(remove);
        wrap.appendChild(captionInput);
        gallery.appendChild(wrap);
    }

    // Walk all caption inputs and reassign name attributes sequentially.
    function reindexCaptions(gallery) {
        var inputs = gallery.querySelectorAll('.image-caption-input');
        for (var i = 0; i < inputs.length; i++) {
            inputs[i].name = 'caption-' + i;
        }
    }

    // Initialise on load and after htmx swaps.
    function init() {
        initImageUpload('.tracker-notes-form, .tracker-goal-form');
    }

    document.addEventListener('DOMContentLoaded', init);
    document.addEventListener('htmx:afterSwap', init);

    window.initImageUpload = initImageUpload;
})();
