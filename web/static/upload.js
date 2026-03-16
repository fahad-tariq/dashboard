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

        // Load existing images.
        var existing = hidden.value ? hidden.value.split(',').filter(Boolean) : [];
        existing.forEach(function(img) {
            addThumbnail(gallery, hidden, img);
        });

        // File input handler.
        fileInput.addEventListener('change', function() {
            Array.from(fileInput.files).forEach(function(file) {
                uploadFile(file, function(filename) {
                    appendImage(hidden, gallery, filename);
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
                            appendImage(hidden, gallery, filename);
                        });
                        break;
                    }
                }
            });
        });
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
            });
    }

    function appendImage(hidden, gallery, filename) {
        var images = hidden.value ? hidden.value.split(',').filter(Boolean) : [];
        images.push(filename);
        hidden.value = images.join(',');
        addThumbnail(gallery, hidden, filename);
    }

    function addThumbnail(gallery, hidden, filename) {
        var wrap = document.createElement('div');
        wrap.className = 'image-thumb-wrap';

        var img = document.createElement('img');
        img.src = '/uploads/' + filename;
        img.className = 'image-thumb';
        img.loading = 'lazy';

        var remove = document.createElement('button');
        remove.type = 'button';
        remove.className = 'image-thumb-remove';
        remove.textContent = '\u00d7';
        remove.addEventListener('click', function() {
            var images = hidden.value.split(',').filter(function(f) { return f !== filename; });
            hidden.value = images.join(',');
            wrap.remove();
        });

        wrap.appendChild(img);
        wrap.appendChild(remove);
        gallery.appendChild(wrap);
    }

    // Initialise on load and after htmx swaps.
    function init() {
        initImageUpload('.tracker-notes-form, .tracker-goal-form, .idea-form');
    }

    document.addEventListener('DOMContentLoaded', init);
    document.addEventListener('htmx:afterSwap', init);

    window.initImageUpload = initImageUpload;
})();
