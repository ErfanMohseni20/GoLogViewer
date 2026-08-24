/* GoLogViewer front-end.
   Reads its configuration from the #config script tag the server renders, so
   this file stays a static asset the browser can cache. */
(() => {
    'use strict';

    const config = JSON.parse(document.getElementById('config').textContent);

    const state = {
        files: [],
        file: '',
        page: 1,
        perPage: 50,
        levels: new Set(),
        search: '',
        totalPages: 0,
        counts: {},
        live: false,
        stream: null,
        expanded: new Set(),
    };

    const $ = (id) => document.getElementById(id);
    const els = {
        fileList: $('file-list'),
        logs: $('logs'),
        title: $('current-file'),
        summary: $('summary'),
        pageInfo: $('page-info'),
        search: $('search'),
        perPage: $('per-page'),
        levels: $('levels'),
        prev: $('prev-page'),
        next: $('next-page'),
        live: $('live-btn'),
        refresh: $('refresh-btn'),
        theme: $('theme-btn'),
        download: $('download-btn'),
        del: $('delete-btn'),
        clearAll: $('clear-all-btn'),
        toast: $('toast'),
    };

    /* ---------- helpers ---------- */

    const escapeHtml = (value) => String(value ?? '').replace(/[&<>"']/g, (c) => (
        { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
    ));

    function formatBytes(bytes) {
        if (!Number.isFinite(bytes)) return '';
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
        if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`;
        return `${(bytes / 1073741824).toFixed(2)} GB`;
    }

    const relativeFormatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
    const RELATIVE_STEPS = [
        ['second', 60], ['minute', 60], ['hour', 24], ['day', 7], ['week', 4.35], ['month', 12], ['year', Infinity],
    ];

    function relativeTime(date) {
        let delta = (date.getTime() - Date.now()) / 1000;
        for (const [unit, span] of RELATIVE_STEPS) {
            if (Math.abs(delta) < span) return relativeFormatter.format(Math.round(delta), unit);
            delta /= span;
        }
        return '';
    }

    function formatTime(value) {
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return { absolute: String(value ?? ''), relative: '' };
        return {
            absolute: date.toLocaleString(undefined, {
                year: 'numeric', month: '2-digit', day: '2-digit',
                hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
            }),
            relative: relativeTime(date),
        };
    }

    /* Highlights search hits inside already-escaped text. Escaping happens
       first, so the injected <mark> tags are the only markup that survives. */
    function highlight(escaped, term) {
        if (!term) return escaped;
        const pattern = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        return escaped.replace(new RegExp(escapeHtml(pattern), 'gi'), (hit) => `<mark>${hit}</mark>`);
    }

    let toastTimer;
    function toast(message) {
        els.toast.textContent = message;
        els.toast.classList.add('show');
        clearTimeout(toastTimer);
        toastTimer = setTimeout(() => els.toast.classList.remove('show'), 2200);
    }

    function showState(container, kind, title, detail) {
        const spinner = kind === 'loading' ? '<div class="spinner"></div>' : '';
        container.innerHTML =
            `<div class="state ${kind === 'error' ? 'error' : ''}">${spinner}` +
            `<strong>${escapeHtml(title)}</strong>` +
            (detail ? `<span>${escapeHtml(detail)}</span>` : '') +
            `</div>`;
    }

    async function api(path, options) {
        const response = await fetch(config.prefix + path, options);
        if (!response.ok) {
            let message = `${response.status} ${response.statusText}`;
            try {
                const body = await response.json();
                if (body && body.error) message = body.error;
            } catch { /* response was not JSON; keep the status text */ }
            throw new Error(message);
        }
        return response.status === 204 ? null : response.json();
    }

    /* ---------- files ---------- */

    async function loadFiles() {
        try {
            const data = await api('/api/files');
            state.files = data.files || [];
            renderFiles();

            if (state.files.length === 0) {
                state.file = '';
                els.title.textContent = 'No log files';
                showState(els.logs, 'empty', 'No log files yet',
                    `Nothing in ${data.log_dir}. Write a log entry and refresh.`);
                updateActionButtons();
                return;
            }

            if (!state.file || !state.files.some((f) => f.name === state.file)) {
                selectFile(state.files[0].name);
            }
        } catch (error) {
            els.fileList.innerHTML = `<li class="empty">${escapeHtml(error.message)}</li>`;
        }
    }

    function renderFiles() {
        if (state.files.length === 0) {
            els.fileList.innerHTML = '<li class="empty">No log files found</li>';
            return;
        }

        els.fileList.innerHTML = state.files.map((file) => {
            const { relative } = formatTime(file.modified * 1000);
            return `
                <li>
                    <button type="button" class="file-item ${file.name === state.file ? 'active' : ''}"
                            data-file="${escapeHtml(file.name)}">
                        <span class="file-row">
                            <span class="file-name">${escapeHtml(file.name)}</span>
                            <span class="file-meta">${formatBytes(file.size)}</span>
                        </span>
                        <span class="file-time">${escapeHtml(relative)}</span>
                    </button>
                </li>`;
        }).join('');
    }

    function selectFile(name) {
        state.file = name;
        state.page = 1;
        state.expanded.clear();
        els.title.textContent = name;
        renderFiles();
        updateActionButtons();
        if (state.live) startStream();
        loadEntries();
    }

    function updateActionButtons() {
        const disabled = !state.file;
        if (els.download) els.download.disabled = disabled;
        if (els.del) els.del.disabled = disabled;
        els.live.disabled = disabled;
    }

    /* ---------- entries ---------- */

    function buildParams() {
        const params = new URLSearchParams({
            file: state.file,
            page: String(state.page),
            per_page: String(state.perPage),
        });
        if (state.search) params.set('search', state.search);
        for (const level of state.levels) params.append('level', level);
        return params;
    }

    let requestToken = 0;

    async function loadEntries() {
        if (!state.file) return;

        const token = ++requestToken;
        showState(els.logs, 'loading', 'Loading entries…');

        try {
            const data = await api(`/api/entries?${buildParams()}`);
            // A slower earlier request must not overwrite a newer response.
            if (token !== requestToken) return;

            state.totalPages = data.total_pages || 0;
            state.counts = data.level_counts || {};

            renderCounts();
            renderEntries(data.entries || []);

            els.summary.textContent =
                `${data.total.toLocaleString()} ${data.total === 1 ? 'entry' : 'entries'}` +
                (data.truncated ? ' (partial scan)' : '') +
                ` · ${formatBytes(data.file_size)}`;
            els.pageInfo.textContent = `Page ${data.page} of ${Math.max(data.total_pages, 1)}`;
            els.prev.disabled = data.page <= 1;
            els.next.disabled = data.page >= Math.max(data.total_pages, 1);
        } catch (error) {
            if (token !== requestToken) return;
            showState(els.logs, 'error', 'Could not load entries', error.message);
        }
    }

    function renderCounts() {
        els.levels.querySelectorAll('.chip').forEach((chip) => {
            const level = chip.dataset.level;
            const count = state.counts[level] || 0;
            chip.querySelector('.count').textContent = count ? count.toLocaleString() : '0';
            chip.setAttribute('aria-pressed', String(state.levels.has(level)));
            chip.hidden = count === 0 && !state.levels.has(level);
        });
    }

    function renderEntries(entries) {
        if (entries.length === 0) {
            showState(els.logs, 'empty', 'No matching entries',
                state.search || state.levels.size ? 'Try relaxing the filters.' : 'This file is empty.');
            return;
        }

        const term = state.search.toLowerCase();

        els.logs.innerHTML = entries.map((entry, index) => {
            const level = entry.level || 'info';
            const time = formatTime(entry.time);
            const open = state.expanded.has(index);

            const context = entry.context && Object.keys(entry.context).length
                ? JSON.stringify(entry.context, null, 2) : '';

            const sub = [];
            if (entry.channel) sub.push(`<span class="tag">${escapeHtml(entry.channel)}</span>`);
            if (entry.caller) sub.push(`<span class="tag">${escapeHtml(entry.caller)}</span>`);
            if (time.relative) sub.push(`<span>${escapeHtml(time.relative)}</span>`);

            const hasDetail = Boolean(context || entry.exception || entry.stack);

            return `
                <article class="entry entry-${escapeHtml(level)}">
                    <button type="button" class="entry-head" data-index="${index}"
                            aria-expanded="${open}">
                        <span class="badge badge-${escapeHtml(level)}">${escapeHtml(level)}</span>
                        <span class="entry-main">
                            <p class="entry-message">${highlight(escapeHtml(entry.message || entry.raw || ''), term)}</p>
                            ${sub.length ? `<span class="entry-sub">${sub.join('')}</span>` : ''}
                        </span>
                        <span class="entry-time">${escapeHtml(time.absolute)}</span>
                    </button>
                    ${open || !hasDetail ? renderBody(entry, context, term, index) : ''}
                </article>`;
        }).join('');

        els.logs.dataset.entries = JSON.stringify(entries);
    }

    function renderBody(entry, context, term, index) {
        const hasDetail = Boolean(context || entry.exception || entry.stack);
        if (!hasDetail) return '';

        let html = '<div class="entry-body">';
        if (entry.exception) {
            html += `<h4>Exception</h4><pre class="exception">${highlight(escapeHtml(entry.exception), term)}</pre>`;
        }
        if (entry.stack) {
            html += `<h4>Stack trace</h4><pre>${escapeHtml(entry.stack)}</pre>`;
        }
        if (context) {
            html += `<h4>Context</h4><pre>${highlight(escapeHtml(context), term)}</pre>`;
        }
        html += `<div class="entry-actions">
                    <button type="button" class="btn" data-copy="${index}">Copy JSON</button>
                 </div></div>`;
        return html;
    }

    /* ---------- live tail ---------- */

    function startStream() {
        stopStream();
        if (!state.file) return;

        const source = new EventSource(`${config.prefix}/api/stream?file=${encodeURIComponent(state.file)}`);

        // New lines land at the top of page 1, so reload rather than splice —
        // it keeps filters, counts and pagination consistent for free.
        source.addEventListener('entries', () => {
            if (state.page === 1) loadEntries();
        });
        source.addEventListener('reset', () => {
            state.page = 1;
            loadEntries();
            loadFiles();
        });
        source.onerror = () => {
            // EventSource reconnects on its own; surface it only once.
            if (source.readyState === EventSource.CLOSED) {
                toast('Live tail disconnected');
                setLive(false);
            }
        };

        state.stream = source;
    }

    function stopStream() {
        if (state.stream) {
            state.stream.close();
            state.stream = null;
        }
    }

    function setLive(on) {
        state.live = on;
        els.live.classList.toggle('is-live', on);
        els.live.setAttribute('aria-pressed', String(on));
        els.live.textContent = on ? '● Live' : 'Live tail';
        if (on) {
            state.page = 1;
            startStream();
            loadEntries();
        } else {
            stopStream();
        }
    }

    /* ---------- theme ---------- */

    function applyTheme(theme) {
        if (theme) {
            document.documentElement.setAttribute('data-theme', theme);
        } else {
            document.documentElement.removeAttribute('data-theme');
        }
        els.theme.textContent = theme === 'dark' ? '☾' : theme === 'light' ? '☀' : '◐';
        els.theme.title = `Theme: ${theme || 'system'}`;
    }

    function cycleTheme() {
        const order = ['light', 'dark', ''];
        const current = document.documentElement.getAttribute('data-theme') || '';
        const next = order[(order.indexOf(current) + 1) % order.length];
        try {
            if (next) localStorage.setItem('gologviewer-theme', next);
            else localStorage.removeItem('gologviewer-theme');
        } catch { /* private mode; the choice just will not persist */ }
        applyTheme(next);
    }

    /* ---------- events ---------- */

    els.fileList.addEventListener('click', (event) => {
        const button = event.target.closest('.file-item');
        if (button) selectFile(button.dataset.file);
    });

    els.logs.addEventListener('click', (event) => {
        const copy = event.target.closest('[data-copy]');
        if (copy) {
            const entries = JSON.parse(els.logs.dataset.entries || '[]');
            const entry = entries[Number(copy.dataset.copy)];
            navigator.clipboard.writeText(JSON.stringify(entry, null, 2))
                .then(() => toast('Copied to clipboard'))
                .catch(() => toast('Clipboard unavailable'));
            return;
        }

        const head = event.target.closest('.entry-head');
        if (!head) return;

        const index = Number(head.dataset.index);
        if (state.expanded.has(index)) state.expanded.delete(index);
        else state.expanded.add(index);

        renderEntries(JSON.parse(els.logs.dataset.entries || '[]'));
    });

    els.levels.addEventListener('click', (event) => {
        const chip = event.target.closest('.chip');
        if (!chip) return;

        const level = chip.dataset.level;
        if (state.levels.has(level)) state.levels.delete(level);
        else state.levels.add(level);

        state.page = 1;
        renderCounts();
        loadEntries();
    });

    let searchTimer;
    els.search.addEventListener('input', () => {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(() => {
            state.search = els.search.value.trim();
            state.page = 1;
            loadEntries();
        }, 250);
    });

    els.perPage.addEventListener('change', () => {
        state.perPage = Number(els.perPage.value);
        state.page = 1;
        loadEntries();
    });

    els.prev.addEventListener('click', () => {
        if (state.page > 1) { state.page -= 1; state.expanded.clear(); loadEntries(); }
    });

    els.next.addEventListener('click', () => {
        if (state.page < state.totalPages) { state.page += 1; state.expanded.clear(); loadEntries(); }
    });

    els.refresh.addEventListener('click', () => { loadFiles(); loadEntries(); });
    els.live.addEventListener('click', () => setLive(!state.live));
    els.theme.addEventListener('click', cycleTheme);

    if (els.download) {
        els.download.addEventListener('click', () => {
            window.location.href = `${config.prefix}/api/download?file=${encodeURIComponent(state.file)}`;
        });
    }

    if (els.del) {
        els.del.addEventListener('click', async () => {
            if (!confirm(`Delete ${state.file}? This cannot be undone.`)) return;
            try {
                await api(`/api/files?file=${encodeURIComponent(state.file)}`, { method: 'DELETE' });
                toast(`Deleted ${state.file}`);
                state.file = '';
                await loadFiles();
            } catch (error) {
                toast(error.message);
            }
        });
    }

    if (els.clearAll) {
        els.clearAll.addEventListener('click', async () => {
            if (!confirm('Delete every log file? This cannot be undone.')) return;
            try {
                const result = await api('/api/clear', { method: 'POST' });
                toast(`Removed ${result.removed} file(s)`);
                state.file = '';
                await loadFiles();
            } catch (error) {
                toast(error.message);
            }
        });
    }

    document.addEventListener('keydown', (event) => {
        const typing = /^(INPUT|SELECT|TEXTAREA)$/.test(event.target.tagName);

        if (event.key === '/' && !typing) {
            event.preventDefault();
            els.search.focus();
            els.search.select();
            return;
        }
        if (event.key === 'Escape' && typing) {
            event.target.blur();
            return;
        }
        if (typing) return;

        if (event.key === 'r') { loadFiles(); loadEntries(); }
        if (event.key === 'l') setLive(!state.live);
        if (event.key === 't') cycleTheme();
        if (event.key === 'j' && !els.next.disabled) els.next.click();
        if (event.key === 'k' && !els.prev.disabled) els.prev.click();
    });

    // Pause the stream while the tab is hidden so a backgrounded viewer does
    // not hold an open connection and keep polling the file.
    document.addEventListener('visibilitychange', () => {
        if (!state.live) return;
        if (document.hidden) stopStream();
        else { startStream(); loadEntries(); }
    });

    window.addEventListener('beforeunload', stopStream);

    /* ---------- boot ---------- */

    let saved = null;
    try { saved = localStorage.getItem('gologviewer-theme'); } catch { /* ignore */ }
    applyTheme(saved || '');

    updateActionButtons();
    loadFiles();

    // Files are cheap to list and their sizes drive the sidebar, so refresh
    // them on a slow timer even when live tail is off.
    setInterval(loadFiles, 15000);
})();
