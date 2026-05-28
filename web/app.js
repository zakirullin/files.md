const sidebar = document.getElementById('sidebar');
const content = document.getElementById('content')

const CHAT_PATH = '/Chat.md';
const LATER_PATH = '/Later.md';
const READ_PATH = '/Read.md';
const SHOP_PATH = '/Shop.md';
const WATCH_PATH = '/Watch.md';
const LOG_PATH = '/archive/Log.txt';
const OPEN_CHAT_AFTER_IDLE = 60 * 60 * 1000; // ms
const RAW_EDITING_LINE_STORAGE_KEY = 'rawEditingLine';

let openChatIdleTimer = null;
let isChat = false;
let isMemFS = false;
let debug = false;
// let debug = {dir: '', file: 'File.md', loaded: false};

async function init() {
    updateRawEditingLineToggle();

    // Ask the browser to mark our origin as persistent so the quota
    // manager can't evict the auth cookie + localStorage under disk
    // pressure. Chrome auto-grants for installed PWAs / high-engagement
    // sites; otherwise resolves false and we run on best-effort storage.
    // Idempotent - safe to call on every load.
    if (navigator.storage && navigator.storage.persist) {
        const persisted = await navigator.storage.persist();
        log('Storage persisted:', persisted);
    }

    // Authorize if we have one-time token in URL.
    const urlParams = new URLSearchParams(window.location.search);
    const oneTimeToken = urlParams.get('token');
    if (oneTimeToken) {
        try {
            // Exchange one-time token for permanent token
            const response = await fetch(`${API_URL}/issuePermanentToken`, {
                method: 'POST',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    oneTimeToken: oneTimeToken
                })
            });

            if (response.ok) {
                // Server sets the auth cookie via Set-Cookie on this response.
                markServerOk();
                const url = new URL(window.location);
                url.searchParams.delete('token');
                window.history.replaceState({}, '', url);
            } else {
                alert('The token has expired or is invalid. Please try to request a new link.');
                logError('Token exchange failed:', response.status);
            }
        } catch (error) {
            alert('The token has expired or is invalid. Please try to request a new link.');
            logError('Error exchanging token:', error);
        }
    }

    const savedDirHandle = await getSavedRootDirHandle();
    const hasSavedLocalDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (hasSavedLocalDir) {
        isMemFS = false;
        document.getElementById('open-folder').style.display = 'none';
    } else if (typeof window.showDirectoryPicker === 'function') {
        document.getElementById('open-folder').style.display = 'flex';
        isMemFS = true;
    } else {
        // Safari/Firefox have no File System Access API for now, hide CTA.
        document.getElementById('open-folder').style.display = 'none';
        isMemFS = true;
    }

    // Alert if there's no "Allow on every visit" check.
    if (isChrome() && hasSavedLocalDir) {
        const permission = await (await getRootDirHandle()).queryPermission({ mode: 'readwrite' });
        log('PERMISSION', permission);
        if (permission !== 'granted') {
            document.getElementById('open-folder').style.display = 'flex';
            // TODO maybe ask user to check "Allow on every visit" on left part of the sidebar
            await removeSavedRootDirHandle();
            alert('Can\'t access folder.\n\nPlease, reopen the folder again and check "Allow on every visit" checkbox');
        }
    }

    let rootDirHandle = await getRootDirHandle();

    let perf = performance.now();
    files = await loadLocalFiles(rootDirHandle);
    log(`Files loaded in ${performance.now() - perf}ms`);

    initChat();

    perf = performance.now();
    renderSidebar();
    log(`Sidebar built in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    const userHasCustomAPIUrl = localStorage.getItem('apiUrl') !== null;
    if (isMemFS && !userHasCustomAPIUrl) {
        // By the time a user has setup custom API, he doesn't need welcome file :)
        await openFile('/🪴 Welcome.md');
    } else {
        openChat();
    }

    perf = performance.now();
    await syncFilesWithServer();
    await renderSidebar();
    await syncMediaFiles();
    log(`Files initialized in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

}

function getRawEditingLinePreference() {
    return localStorage.getItem(RAW_EDITING_LINE_STORAGE_KEY) === 'true';
}

function setRawEditingLinePreference(enabled) {
    localStorage.setItem(RAW_EDITING_LINE_STORAGE_KEY, enabled ? 'true' : 'false');
    updateRawEditingLineToggle();
    applyRawEditingLinePreference();
}

function toggleRawEditingLinePreference() {
    setRawEditingLinePreference(!getRawEditingLinePreference());
}

function updateRawEditingLineToggle() {
    const button = document.getElementById('raw-editing-line-toggle');
    if (!button) {
        return;
    }

    const enabled = getRawEditingLinePreference();
    button.setAttribute('aria-pressed', enabled ? 'true' : 'false');
    button.setAttribute('data-tooltip', enabled ? 'Raw current line: on' : 'Raw current line');
}

function applyRawEditingLinePreference() {
    const enabled = getRawEditingLinePreference();
    [window.editor, window.editor2].forEach(cm => {
        if (!cm || typeof cm.setOption !== 'function') {
            return;
        }

        cm.setOption('hmdRawEditingLine', enabled);
    });
}

// Logic for click-handling is in click.js => isWikiLink
function createAutocompleteDict() {
    const entries = [];
    const currentPath = currentEditor && currentEditor.path;

    // Collect all files with their metadata
    walkFilesExcludingSystemDirs((path) => {
        if (path === CONFIG_PATH || path === CHAT_PATH || path === LATER_PATH || path === READ_PATH || path === WATCH_PATH || path === SHOP_PATH) {
            return;
        }
        if (path === currentPath) {
            return;
        }

        const filename = toFilename(path);
        const key = `${filename.replace(/\.md$/, '')}`;
        const url = path.replace(/ /g, '%20');
        const filePath = `${filename.replace(/\.md$/, '')}](${url})`;

        entries.push({
            key,
            filePath,
            lastModified: getMemFile(path).lastModified
        });

    });

    // Sort by last modified (most recent first)
    entries.sort((a, b) => b.lastModified - a.lastModified);
    const dict = {};
    entries.forEach(entry => {
        dict[entry.key] = entry.filePath;
    });

    let lowPriorityEntries = [];
    ['_read_/', '_watch_/', '_shop_/', 'today/', 'later/', 'journal/'].forEach(dir => {
        if (!files[dir]) {
            return;
        }

        Object.keys(files[dir]).forEach(filename => {
            if (filename === CONFIG_PATH || filename === CHAT_PATH) {
                return;
            }
            const key = `${filename.replace(/\.md$/, '')}`;
            const url = `${dir}/${filename}`.replace(/ /g, '%20');
            const filePath = `${filename.replace(/\.md$/, '')}](${url})`;

            lowPriorityEntries.push({
                key,
                filePath,
                lastModified: files[dir][filename].lastModified
            });
        });
    });

    lowPriorityEntries.sort((a, b) => b.lastModified - a.lastModified);
    lowPriorityEntries.forEach(entry => {
        dict[entry.key] = entry.filePath;
    });

    return dict;
}

async function newFile(parentDir) {
    log('New file clicked');
    // New files always land at the root. The `parentDir` parameter is still
    // honored (sidebar right-click → New file inside a specific folder).
    const dirPath = parentDir !== undefined
        ? (parentDir === '/' ? '/' : parentDir.replace(/\/$/, ''))
        : '/';

    let filename = 'New file.md';
    let num = 1;
    while (getMemFile(joinPath(dirPath, filename)) !== null) {
        log('file exists', joinPath(dirPath, filename));
        filename = `New file (${num}).md`;
        num++;
    }

    const path = joinPath(dirPath, filename);
    log('PATH', path);
    let handle = await getFileHandle(path, true);
    addMemFile(path, {
        isFile: true,
        content: '',
        lastModified: 0,
        handle: handle,
        path: path,
        imageUrl: null
    });

    log('Creating new file', path);
    await openFile(path);
    log('CURRENT path after new', currentEditor.path);
    editor.setCursor({ line: 1, ch: 0 });
    editor.focus();

    await renderSidebar();
}

async function newFolder() {
    let folderName = prompt('Enter folder name:', 'New Folder');
    if (folderName === null) {
        return;
    }

    folderName = folderName.trim();
    if (!folderName) {
        alert('Folder name cannot be empty');
        return;
    }

    let finalFolderName = folderName;
    let num = 1;
    while (files[finalFolderName + '/']) {
        finalFolderName = `${folderName} (${num})`;
        num++;
    }

    const rootDirHandle = await getRootDirHandle();
    await rootDirHandle.getDirectoryHandle(finalFolderName, { create: true });
    files[finalFolderName + '/'] = {};

    log('CREATED folder', finalFolderName);

    await renderSidebar(finalFolderName);
}

function isMetaKey(event) {
    return event.metaKey || event.ctrlKey || event.altKey;
}

function isSidebarToggleShortcut(event) {
    if (!isMetaKey(event)) {
        return false;
    }

    // Match the physical shortcut key across ANSI/ISO keyboard layouts.
    return event.code === 'Backquote'
        || event.code === 'IntlBackslash'
        || event.key === '`'
        || event.key === '~'
        || event.key === '§'
        || event.key === '±';
}

async function openDir() {
    let dirHandle = null;
    try {
        dirHandle = await window.showDirectoryPicker({ 'mode': 'readwrite' });
    } catch (error) {
        // User pressed Esc (AbortError) or the browser doesn't support
        // the picker (TypeError).
        if (error instanceof TypeError) {
            alert('For now only Chrome browser supports local folders :(');
        }
        return;
    }
    // TODO check that permissions are given?

    // Don't race the existing files loading.
    while (isLoadingLocalFiles) {
        await new Promise(r => setTimeout(r, 50));
    }
    isLoadingLocalFiles = true

    // Don't race with files sync.
    while (isSyncingFiles) {
        await new Promise(r => setTimeout(r, 50));
    }
    isSyncingFiles = true

    // New folder would miss files that were synced from server before,
    // into a previous folder. That would send a signal to server "client has deleted some files".
    // Which we do not want, so we clean our server files "understanding".
    server = {files: {}, media: {}, timestamps: {}, mediaTimestamp: 0};
    localStorage.removeItem("server");

    await saveDirectoryHandle(dirHandle);
    await write('/Help.md', getHelpContent());

    isLoadingLocalFiles = false
    try {
        files = await loadLocalFiles(dirHandle);
    } finally {
        isSyncingFiles = false;
    }

    isMemFS = false;
    document.getElementById('open-folder').style.display = 'none';
    renderSidebar();
    await openChat();
}

function getCurrentContent() {
    let content = currentEditor.getValue();
    const header = toHeader(toFilename(currentEditor.path)).toLowerCase();
    // Remove header if it exists.
    if (content.toLowerCase().startsWith(header)) {
        content = content.slice(`${header}\n`.length);
    } else if (content.toLowerCase().startsWith('# ')) {
        // Skip header placeholder.
        // What is the case when starts with # '? Empty filename? Header not equal to original header?
        // TODO but do we always have \n?
        content = content.slice(`# \n`.length);
    }

    return content;
}

function toHeader(filename) {
    let header = filename;
    if (filename.endsWith('.md')) {
        header = trimPostfix(filename, '.md');
    }

    return `# ${header}`;
}

function fromHeaderToFilename(header) {
    if (header.startsWith('# ')) {
        return header.slice(2).trim() + '.md';
    }
    return header.trim() + '.md';
}

function ucfirst(val) {
    return String(val).charAt(0).toUpperCase() + String(val).slice(1);
}

async function getImageUrl(fileHandle) {
    const file = await fileHandle.getFile();
    return URL.createObjectURL(file);
}

// Normalize text to use only \n as line endings
function normNewLines(text) {
    return text.replace(/\r\n|\r/g, '\n');
}

function showToast(msg, ms = 1500) {
    const toast = document.createElement('div');
    if (msg instanceof Node) {
        toast.appendChild(msg);
    } else {
        toast.textContent = msg;
    }
    // Center over the editor area (not the whole viewport) so the toast
    // sits above the content rather than drifting onto the sidebar.
    const editorContainer = document.getElementById('editor-container');
    const rect = editorContainer ? editorContainer.getBoundingClientRect() : null;
    const centerX = rect ? rect.left + rect.width / 2 : window.innerWidth / 2;
    toast.style.cssText = `
        position: fixed; top: 8px; left: ${centerX}px; transform: translateX(-50%);
        background: var(--col-bg-alt); color: var(--col-tx); padding: 8px 16px; border-radius: 5px;
        border: 1px solid var(--col-border);
        z-index: 9999; font-size: 14px;
    `;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), ms);
}

function initDB() {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open('files', 1);
        request.onerror = () => reject(request.error);
        request.onsuccess = () => resolve(request.result);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains('handles')) {
                db.createObjectStore('handles');
            }
        };
    });
}

async function saveDirectoryHandle(directoryHandle) {
    const db = await initDB();
    const transaction = db.transaction('handles', 'readwrite');
    const store = transaction.objectStore('handles');
    await store.put(directoryHandle, 'savedDirectoryHandle');
}

async function getSavedRootDirHandle() {
    const db = await initDB();
    const tx = db.transaction("handles", "readonly");
    const store = tx.objectStore("handles");

    return new Promise((resolve, reject) => {
        const req = store.get("savedDirectoryHandle");
        req.onsuccess = () => resolve(req.result ?? null);
        req.onerror = () => reject(req.error);
        tx.onabort = () => reject(tx.error || new Error("Transaction aborted"));
    });
}

async function removeSavedRootDirHandle() {
    const db = await initDB();
    return new Promise((resolve, reject) => {
        const transaction = db.transaction('handles', 'readwrite');
        const store = transaction.objectStore('handles');
        const request = store.delete('savedDirectoryHandle');
        request.onsuccess = () => resolve();
        request.onerror = () => reject(request.error);
    });
}

async function getRootDirHandle() {
    const savedDirHandle = await getSavedRootDirHandle();
    // If the saved handle is from a browser missing createWritable or
    // remove (Safari OPFS, older Chromium), fall back to the in-memory FS
    // instead of letting later writes/deletes blow up.
    if (!(savedDirHandle instanceof FileSystemDirectoryHandle) || !opfsIsFullyUsable()) {
        return await getTemporaryStorageDirHandle();
    }

    return savedDirHandle;
}

const resizeHandle = document.querySelector('.resize');
let isResizing = false;
resizeHandle.addEventListener('mousedown', initResize);
document.addEventListener('mousemove', doResize);
document.addEventListener('mouseup', stopResize);

function initResize(e) {
    isResizing = true;
    document.body.classList.add('dragging');
    e.preventDefault();
}

function doResize(e) {
    if (!isResizing) return;

    log(e);
    const width = e.clientX;
    const minWidth = 200;
    const maxWidth = 600;

    const constrainedWidth = Math.min(Math.max(width, minWidth), maxWidth);
    sidebar.style.setProperty('width', constrainedWidth + 'px', 'important');
}

function stopResize() {
    if (!isResizing) return;
    isResizing = false;
    document.body.classList.remove('dragging');
}

function toggleSidebar() {
    const sidebar = document.getElementById('sidebar');
    const openSidebar = document.getElementById('open-sidebar');

    const isHidden = sidebar.style.display === 'none'
        || getComputedStyle(sidebar).display === 'none';

    if (isHidden) {
        sidebar.style.display = 'flex';
        openSidebar.style.display = 'none';
        // Suppresses the mobile media-query that hides the sidebar.
        document.body.classList.add('sidebar-open');
    } else {
        sidebar.style.display = 'none';
        openSidebar.style.display = 'block';
        document.body.classList.remove('sidebar-open');
        if (isChat) {
            chatInput.focus();
        } else {
            currentEditor.focus();
        }
    }
}

function trimPostfix(str, postfix) {
    if (str.endsWith(postfix)) {
        return str.slice(0, -postfix.length);
    }
    return str;
}

function trimPrefix(str, prefix) {
    if (str.startsWith(prefix)) {
        return str.slice(prefix.length);
    }
    return str;
}

function getCurrentVersion() {
    return window.COMMIT_HASH ? window.COMMIT_HASH.replace('?v=', '') : '';
}

function showEditor2() {
    const editor2Container = document.getElementById('editor2-container');
    const alreadyShown = editor2Container.classList.contains('show')
        && editor2Container.style.display !== 'none';
    if (alreadyShown) {
        return;
    }

    rememberEditorPos();

    editor2Container.style.display = 'flex';
    editor2Container.offsetHeight; // Force reflow
    editor2Container.classList.add('show');

    editor.refresh();
    editor2.focus();
    restoreEditorPos();
}

function hideEditor2() {
    if (typeof editor2 === 'undefined') {
        return
    }

    const editor2Container = document.getElementById('editor2-container');

    editor2Container.classList.remove('show');
    restoreEditorPos();

    // Clear editor2's path so a subsequent openFile for the same path
    // doesn't take the isSameFile short-circuit (which skips re-init and
    // would leave the panel visually empty after editor1 re-init nuked
    // editor2's wrapper).
    editor2.path = undefined;
    currentEditor = editor;
    selectSidebarItem(editor.path);

    setTimeout(() => {
        editor2Container.style.display = 'none';
        editor.refresh(); // IT seems we have to refresh once size changes.
    }, 300);
}

function isChrome() {
    var winNav = window.navigator;
    var vendorName = winNav.vendor;

    var isChromium = window.chrome;
    var isOpera = typeof window.opr !== "undefined";
    var isIEedge = winNav.userAgent.indexOf("Edg") > -1;
    var isIOSChrome = winNav.userAgent.match("CriOS");
    var isGoogleChrome = isChromium !== null
        && typeof isChromium !== "undefined"
        && vendorName === "Google Inc."
        && isOpera === false
        && isIEedge === false
        && (typeof winNav.userAgentData === "undefined" || winNav.userAgentData.brands.some(x => x.brand === "Google Chrome"));

    if (isIOSChrome) {
        return true;
    } else if (isGoogleChrome) {
        return true;
    } else {
        return false;
    }
}

function goBack() {
    history.back();
}

function goForward() {
    history.forward();
}

// Returns { json, error }. On success, error is null. On HTTP error,
// json is null and error is a "<status> <statusText>: <body>" string.
async function post(endpoint, data) {
    let response;
    try {
        response = await fetch(`${API_URL}/${endpoint}`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json',
                'Version': getCurrentVersion()
            },
            body: JSON.stringify(data)
        });
    } catch (e) {
        return { json: null, error: `network: ${e.message}` };
    }

    if (!response.ok) {
        let body = '';
        try { body = await response.text(); } catch (_) {}
        return { json: null, error: `${response.status} ${response.statusText}: ${body}`.trim() };
    }
    markServerOk();

    // Some endpoints (e.g. /syncMedia upload) reply 200 with an empty
    // body on success - treat that as `{}` so callers don't have to
    // care about the difference.
    let json;
    try {
        json = await response.json();
    } catch (e) {
        return { json: null, error: `parse: ${e.message}` };
    }

    // Handle special commands from server.
    // We may need to force-update sometimes.
    if (json.status === 'reload') {
        const url = new URL(window.location);
        url.searchParams.set('t', Date.now());
        window.location.href = url.toString();
    } else if (json.status === 'close') {
        window.location.href = "about:blank"
    }

    return { json, error: null };
}

// Custom global log() function that display immediate values and writes to a file.
// Logging a JavaScript object to the console isn't logging that object's state, it is logging an object reference.
// We make a deep copy of the object at the moment of calling so to display its true value.
function log(...args) {
    logf('', '#4CAF50', args);
}

function logError(...args) {
    logf('Error: ', '#F44336', args);
}

async function logf(prefix, color, args) {
    // Capture real caller from stack (skip 2 levels: _logInternal and log/error)
    const stack = new Error().stack;

    // Extract 3 and 4 lines from stack trace
    const callerFull = stack.split('\n')[3].trim(); // Real caller line
    // Extract only the last path segment
    const callerMatch = callerFull.match(/([^\/\\]+:\d+:\d+)/);
    let caller = callerMatch ? callerMatch[1] : callerFull;

    // Extract 4 if exists
    const callerFull2 = stack.split('\n')[4]?.trim();
    const caller2Match = callerFull2 ? callerFull2.match(/([^\/\\]+:\d+:\d+)/) : null;
    const caller2 = caller2Match ? caller2Match[1] : null;
    if (caller2) {
        // Append second caller for better context
        caller += ` <- ${caller2}`;
    }

    // Format message
    const msg = args.map(arg =>
        typeof arg === 'object' ? JSON.stringify(arg) : String(arg)
    ).join(' ');

    // Get time for console
    const date = new Date();
    const hours = date.getHours().toString().padStart(2, '0');
    const minutes = date.getMinutes().toString().padStart(2, '0');
    const seconds = date.getSeconds().toString().padStart(2, '0');
    const time = `${hours}:${minutes}:${seconds}`;

    // Compact console output with colors
    console.log(
        `%c[${time}]%c ${msg}%c ${caller}`,
        'color: #888; font-size: 0.9em',      // Time in gray
        `color: ${color}; font-weight: bold`, // Message in specified color
        'color: #888; font-size: 0.9em'       // Stack trace in gray
    );

    // File logging with full timestamp
    const day = date.getDate().toString().padStart(2, '0');
    const month = (date.getMonth() + 1).toString().padStart(2, '0');
    const year = date.getFullYear();

    const now = `${day}.${month}.${year} ${time}`;
    const logMsg = `${now} ${prefix}[${callerFull}] ${msg}\n`;

    try {
        await writeAtEnd(LOG_PATH, logMsg);
    } catch (error) {
    }
}

let operationCounter = 0;
function opId() {
    return `${++operationCounter}`;
}

// Event listeners

// Hotkeys
window.addEventListener('keydown', async (event) => {
    if (isMetaKey(event) && event.key == 'w') {
        hideEditor2();
    }

    if (isMetaKey(event) && event.key === 'p') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('search-input').value = ''
        searchModal.open();
    }

    if (isMetaKey(event) && event.key === 'k') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('search-input').value = ''
        searchModal.open();
    }

    if (isMetaKey(event) && event.key === 'm') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('move-input').value = ''
        moveModal.open();
    }

    if (isMetaKey(event) && event.key === 'd') {
        log('cmd+d');
        event.preventDefault();
        event.stopPropagation();
        removeCurrentFile();
    }

    if (isMetaKey(event) && event.key === 'n') {
        event.preventDefault();
        event.stopPropagation();
        event.stopImmediatePropagation();

        if (event.shiftKey) {
            await newFolder();
        } else {
            await newFile();
        }
    }
}, true);

document.addEventListener('keydown', (event) => {
    // TODO cursor shouldn't jump to top once we hit "esc".
    if (event.key === 'Escape') {
        if (chatContainer.style.display !== 'none') {
            const selectedMessages = chat.querySelectorAll('.message.selected');
            if (selectedMessages.length > 0) {
                selectedMessages.forEach(message => message.classList.remove('selected'));
                event.preventDefault();
                event.stopPropagation();
                return;
            }

            closeChatModal();
            editor.focus();
            return;
        }

        hideEditor2();
        editor.focus();

        const allMessages = chat.querySelectorAll('.message');
        allMessages.forEach(message => message.classList.remove('selected'));
        // If in chat, focus chat input
        if (isChat) {
            chatInput.focus();
        }
    }
});

// Toggle focus mode
document.addEventListener('keydown', function(event) {
    // Cmd+shift+enter toggle chat modal.
    if (event.shiftKey && isMetaKey(event) && event.key === 'Enter') {
        event.preventDefault();
        if (isChat) {
            history.back();
        } else {
            event.preventDefault();
            toggleChatModal();
        }
        return;
    }
    if (isSidebarToggleShortcut(event)) {
        event.preventDefault();
        toggleSidebar();
    }
    if (isMetaKey(event) && event.key === 'Enter') {
        openChat();
    }
});

document.addEventListener('keydown', (e) => {
    if (e.metaKey || e.ctrlKey) {
        document.body.classList.add('cmd-pressed');
    }
});

document.addEventListener('keyup', (e) => {
    if (!e.metaKey && !e.ctrlKey) {
        document.body.classList.remove('cmd-pressed');
    }
});

window.addEventListener('popstate', (event) => {
    const state = event.state;
    if (state) {
        openFile(state.path, false, state.el);
    }
});

// Reload files once the app gains focus.
window.addEventListener('focus', async () => {
    // Clear any pending chat open timer.
    if (openChatIdleTimer) {
        clearTimeout(openChatIdleTimer);
        openChatIdleTimer = null;
    }

    // We don't want to do heavy stuff when chat is open.
    const userHasCustomAPIUrl = localStorage.getItem('apiUrl') !== null;
    if (isChat || (isMemFS && !userHasCustomAPIUrl)) {
        if (isChat) {
            document.getElementById('chat-input').focus();
        }
        return false;
    }

    log('FOCUS');

    if (currentEditor.path === undefined) {
        return;
    }

    document.getElementById('chat-input').focus();

    const savedDirectoryHandle = await getRootDirHandle();
    // TODO check if access granted

    // Sync media first, so that new images for current file would be loaded
    await syncMediaFiles();
    await syncCurrentFile();

    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle, true);
    const end = performance.now();
    log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncFilesWithServer()
    await renderSidebar();
    log('Sync completed');
});

// Sync files on chat focus lose.
window.addEventListener('blur', async function() {
    log('Window lost focus');
    editor.refresh();

    // Start timer to open chat after idle.
    openChatIdleTimer = setTimeout(() => {
        openChat();
    }, OPEN_CHAT_AFTER_IDLE);

    // Sync media first, so that new images for current file would be loaded
    // if files is not empty object
    if (Object.keys(files).length === 0) {
        return;
    }
    await syncMediaFiles();
    await syncCurrentFile();

    const savedDirectoryHandle = await getRootDirHandle();

    // Benchmark time took
    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle);
    const end = performance.now();
    log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncFilesWithServer()
    await renderSidebar();
    log('Sync completed');
});

document.addEventListener('keydown', (e) => {
    // If search or move dialog is focused - return
    if (document.getElementById('search').style.display !== 'none' ||
        document.getElementById('move').style.display !== 'none') {
        return;
    }

    if (isChat) {
        return;
    }
}, true);

window.addEventListener('beforeunload', function() {
    clearInterval(window.saver);
});

// Worker to process the saving queue
window.saver = setInterval(() => {
    if (document.hasFocus()) {
        syncCurrentFile();
    }
}, CURRENT_FILE_SYNC_INTERVAL);
