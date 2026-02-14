// Main application file.
// We use HyperMD/Codemirror as an underlying text editor.
// We read and save files using Local File System API (or in-memory FS in case of Safari).
// We sync both text and media files with the server if there's a token key in local storage.

let isInbox = false;
let isWelcome = false;
let debug = false;
// let debug = {dir: '', file: 'Sim.md', loaded: false};

const sidebar = document.getElementById('sidebar');
const sidebarContainer = document.getElementById('sidebar-container');
const content = document.getElementById('content')

const TODAY_PATH = '/Today.txt';
const LATER_PATH = '/Later.txt';
const DONE_PATH = '/archive/Done.txt';
const LOG_PATH = '/archive/Log.txt';

const OPEN_INBOX_AFTER_IDLE = 1 * 60 * 60 * 1000; // ms
let openInboxIdleTimer = null;

async function init() {
    // Authorize if we have one-time token in URL.
    const urlParams = new URLSearchParams(window.location.search);
    const oneTimeToken = urlParams.get('token');
    if (oneTimeToken) {
        try {
            // Exchange one-time token for permanent token
            const response = await fetch('https://api.files.md/token', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    oneTimeToken: oneTimeToken
                })
            });

            if (response.ok) {
                const data = await response.json();
                localStorage.setItem('token', data.token);
                const url = new URL(window.location);
                url.searchParams.delete('token');
                window.history.replaceState({}, '', url);
            } else {
                alert('The token has expired or is invalid. Please try to request a new link.');
                console.error('Token exchange failed:', response.status);
            }
        } catch (error) {
            alert('The token has expired or is invalid. Please try to request a new link.');
            console.error('Error exchanging token:', error);
        }
    }

    const savedDirHandle = await getSavedRootDirHandle();
    const hasSavedLocalDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedLocalDir) {
        document.getElementById('open-folder').style.display = 'inline';
        isWelcome = false;
        // document.getElementById('open-chat-modal').style.display = 'inline';
    } else {
        isWelcome = false;
        document.getElementById('open-folder').style.display = 'none';
        // document.getElementById('open-chat-modal').style.display = 'inline';
    }

    // Alert if there's no "Allow on every visit" check.
    if (isChrome() && hasSavedLocalDir) {
        const permission = await (await getRootDirHandle()).queryPermission({ mode: 'readwrite' });
        log('PERMISSION', permission);
        if (permission !== 'granted') {
            document.getElementById('open-folder').style.display = 'inline';
            // TODO maybe ask user to check "Allow on every visit" on left part of the sidebar
            await removeSavedRootDirHandle();
            alert('Can\'t access folder.\n\nPlease, open folder again and check "Allow on every visit" checkbox');
        }
    }

    let rootDirHandle = await getRootDirHandle();

    let perf = performance.now();
    files = await loadLocalFiles(rootDirHandle);
    log(`Files loaded in ${performance.now() - perf}ms`);

    initInbox();

    perf = performance.now();
    renderSidebar();
    log(`Sidebar built in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    // perf = performance.now();
    openInbox();
    // showRandomFile();
    // log(`Random file opened in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    perf = performance.now();
    await syncTextsWithServer();
    await renderSidebar();
    await syncMedia();
    log(`Files initialized in: ${(performance.now() - perf).toFixed(3)} milliseconds`);
}

// Logic for click-handling is in click.js => isWikiLink
function createAutocompleteDict() {
    const entries = [];

    // Collect all files with their metadata
    walkFilesExcludingSystemDirs((path) => {
        if (path === CONFIG_PATH || path === INBOX_PATH || path === TODAY_PATH || path === LATER_PATH || path === READ_PATH || path === WATCH_PATH || path === SHOP_PATH) {
            return;
        }

        const filename = toFilename(path);
        const key = `${filename.replace(/\.md$/, '')}`;
        const filePath = `${key}]`;

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
            if (filename === CONFIG_PATH || filename === INBOX_PATH) {
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

async function showRandomFile() {
    if (debug) {
        await openFile(debug.dir, debug.file);
        return;
    }

    const allFiles = [];
    walkFilesExcludingSystemDirs((path) => {
        if (path === CONFIG_PATH) {
            return;
        }

        allFiles.push(path);
    });

    if (allFiles.length === 0) {
        console.error('No files found to open.');
        return;
    }

    const randomPath = allFiles[Math.floor(Math.random() * allFiles.length)];
    try {
        await openFile(randomPath);
    } catch (error) {
        console.error('Failed to open random file:', error);
    }
}

async function newFile() {
    log('New file clicked');
    let dirPath = toDirPath(currentEditor.path);
    let selectedDirs = tree.getSelectedNodes();
    if (selectedDirs.length > 0 &&
        selectedDirs[0].getOptions &&
        typeof selectedDirs[0].getOptions === 'function' &&
        selectedDirs[0].getOptions()['dir'] === true) {
        dirPath = '/' + selectedDirs[0].toString();
    }
    // TODO don't create on disk?
    let filename = 'New file.md';

    // TODO check tests
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

// Focus last line before the links.
function focusLastLine() {
    let lastLine = currentEditor.lastLine();
    let targetLine = lastLine;

    // Eat all empty lines before first links.
    while (lastLine >= 0) {
        const lineContent = currentEditor.getLine(lastLine).trim();
        if (lineContent === '') {
            lastLine--;
            continue;
        }

        lastLine = Math.min(lastLine + 1, currentEditor.lastLine());
        break;
    }
    for (let i = lastLine; i >= 0; i--) {
        const lineContent = currentEditor.getLine(i).trim();

        if (!lineContent.startsWith('[') && (!lineContent.endsWith(']') || !lineContent.endsWith(')'))) {
            targetLine = i;
            break;
        }
    }
    const targetChar = currentEditor.getLine(targetLine).length;
    currentEditor.setCursor({ line: targetLine, ch: targetChar });
    // Cursor at the end, but scroll the doc to top
    currentEditor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
    currentEditor.focus();
}

function isMetaKey(event) {
    return event.metaKey || event.ctrlKey || event.altKey;
}

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
            const selectedMessages = inbox.querySelectorAll('.message.selected');
            if (selectedMessages.length > 0) {
                selectedMessages.forEach(message => message.classList.remove('selected'));
                event.preventDefault();
                event.stopPropagation();
                return;
            }

            closeInboxModal();
            editor.focus();
            return;
        }

        hideEditor2();
        editor.focus();

        const allMessages = inbox.querySelectorAll('.message');
        allMessages.forEach(message => message.classList.remove('selected'));
        // If in chat, focus chat input
        if (isInbox) {
            chatInput.focus();
        }
    }
});


// Toggle focus mode
document.addEventListener('keydown', function(event) {
    // Cmd+shift+enter toglle inbox modal
    if (event.shiftKey && isMetaKey(event) && event.key === 'Enter') {
        event.preventDefault();
        if (isInbox) {
            history.back();
        } else {
            event.preventDefault();
            toggleInboxModal();
        }
        return;
    }
    // Shift+Enter to toggle sidebar
    if (isMetaKey(event) && event.key === '\\') {
        toggleSidebar();
    }
    if (isMetaKey(event) && event.key === '.') {
        toggleSidebar();
    }
    if (isMetaKey(event) && event.key === 'Enter') {
        openInbox();
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

async function openDir() {
    await navigator.storage.persist();
    // // Request persistent storage for site
    // if (navigator.storage && navigator.storage.persist) {
    //     const isPersisted = await navigator.storage.persist();
    //     console.log(`Persisted storage granted: ${isPersisted}`);
    // }
    //
    let dirHandle = null;
    try {
        dirHandle = await window.showDirectoryPicker({ 'mode': 'readwrite' });
    } catch (error) {
        if (error instanceof TypeError) {
            alert('Only works in Chrome!');
        }
    }
    document.getElementById('open-folder').style.display = 'none';

    // TODO check that permissions are given?

    await saveDirectoryHandle(dirHandle);


    // Media files got corrupted because they got copied from OPFS to local fs storage.
    // It breaks binary files via .text()
    // await migrateFromOPFSToLocal();
    files = await loadLocalFiles(dirHandle)

    isWelcome = false;
    renderSidebar();
    await openInbox();
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
    } else if (filename.endsWith('.txt')) {
        header = trimPostfix(filename, '.txt');
    }
    return `# ${header}`;
}

function fromHeaderToFilename(header) {
    // Kinda tricky, but what can we do if Chromium is very slow with md files
    if (header === '# Today') {
        return toFilename(TODAY_PATH);
    }
    if (header === '# Later') {
        return toFilename(LATER_PATH);
    }
    if (header === '# Done') {
        return toFilename(DONE_PATH);
    }
    if (header === '# Shop') {
        return toFilename(SHOP_PATH);
    }
    if (header === '# Watch') {
        return toFilename(WATCH_PATH);
    }
    if (header === '# Read') {
        return toFilename(READ_PATH);
    }
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

// async function getSavedRootDirHandle() {
//     const db = await initDB();
//     return new Promise((resolve, reject) => {
//         const transaction = db.transaction('handles', 'readonly');
//         const store = transaction.objectStore('handles');
//         const request = store.get('savedDirectoryHandle');
//         request.onsuccess = () => resolve(request.result);
//         request.onerror = () => reject(request.error);
//     });
// }
//
//
//

async function getSavedRootDirHandle() {
    try {
        const db = await initDB().catch((err) => {
            console.error("[getSavedRootDirHandle] initDB failed:", err);
            throw err;
        });

        return await new Promise((resolve, reject) => {
            let transaction;

            try {
                transaction = db.transaction("handles", "readonly");
            } catch (err) {
                console.error("[getSavedRootDirHandle] db.transaction() threw:", err);
                reject(err);
                return;
            }

            transaction.onabort = () => {
                console.error("[getSavedRootDirHandle] transaction aborted:", transaction.error);
                reject(transaction.error ?? new Error("IndexedDB transaction aborted"));
            };

            transaction.onerror = () => {
                console.error("[getSavedRootDirHandle] transaction error:", transaction.error);
                reject(transaction.error ?? new Error("IndexedDB transaction error"));
            };

            const store = transaction.objectStore("handles");
            const request = store.get("savedDirectoryHandle");

            request.onsuccess = () => {
                const result = request.result;
                if (!result) {
                    console.log("[getSavedRootDirHandle] savedDirectoryHandle not found (null/undefined).");
                }
                resolve(result);
            };

            request.onerror = () => {
                console.error("[getSavedRootDirHandle] request error:", request.error);
                reject(request.error ?? new Error("IndexedDB request error"));
            };
        });
    } catch (err) {
        console.error("[getSavedRootDirHandle] failed:", err);
        throw err;
    }
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
    if (!(savedDirHandle instanceof FileSystemDirectoryHandle)) {
        return await getOPFSDirHandle();
    }

    return savedDirHandle;
}

// Reload files once the app gains focus.
window.addEventListener('focus', async () => {
    // Clear any pending inbox open timer
    if (openInboxIdleTimer) {
        clearTimeout(openInboxIdleTimer);
        openInboxIdleTimer = null;
    }

    // We don't want to do heavy stuff when chat is open.
    if (isInbox || isWelcome) {
        return false;
    }

    log('FOCUS');

    if (currentEditor.path === undefined) {
        return;
    }

    // editor.focus();
    // focus chat-input
    document.getElementById('chat-input').focus();

    const savedDirectoryHandle = await getRootDirHandle();
    // check if granted

    // Sync media first, so that new images for current file would be loaded
    await syncMedia();
    await syncCurrentEditor();

    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle, true);
    const end = performance.now();
    log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncTextsWithServer()
    await renderSidebar();
    log('Sync completed');
});

// Sync files on chat focus lose.
window.addEventListener('blur', async function() {
    log('Window lost focus');
    editor.refresh();

    // Start timer to open inbox after idle
    openInboxIdleTimer = setTimeout(() => {
        openInbox();
    }, OPEN_INBOX_AFTER_IDLE);

    // if (!isInbox) {
    //     return;
    // }
    // Why we did that?

    // Sync media first, so that new images for current file would be loaded
    // if files is not empty object
    if (Object.keys(files).length === 0) {
        return;
    }
    await syncMedia();
    await syncCurrentEditor();

    const savedDirectoryHandle = await getRootDirHandle();

    // Benchmark time took
    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle);
    const end = performance.now();
    log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncTextsWithServer()
    await renderSidebar();
    log('Sync completed');
});


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

document.addEventListener('keydown', (e) => {
    // If search or move dialog is focused - return
    if (document.getElementById('search').style.display !== 'none' ||
        document.getElementById('move').style.display !== 'none') {
        return;
    }

    if (isInbox) {
        return;
    }
}, true);

function toggleSidebar() {
    const sidebar = document.getElementById('sidebar');
    const openSidebar = document.getElementById('open-sidebar');

    if (sidebar.style.display === 'none') {
        sidebar.style.display = 'flex';
        openSidebar.style.display = 'none';
    } else {
        sidebar.style.display = 'none';
        openSidebar.style.display = 'block';
        if (isInbox) {
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
    rememberEditorPos();
    const editor2Container = document.getElementById('editor2-container');

    editor2Container.style.display = 'flex';
    editor2Container.offsetHeight; // Force reflow
    editor2Container.classList.add('show');

    editor.refresh();
    editor2.focus();
    restoreEditorPos();
}

function hideEditor2() {
    const editor2Container = document.getElementById('editor2-container');

    editor2Container.classList.remove('show');
    restoreEditorPos();

    setTimeout(() => {
        editor2Container.style.display = 'none';
        editor.refresh(); // IT seems we have to refresh once size changes.
    }, 300);
}

let topLineNumber;
function rememberEditorPos() {
    const scrollInfo = editor.getScrollInfo();
    const topCoords = editor.coordsChar({ left: 0, top: scrollInfo.top }, "local");
    topLineNumber = topCoords.line;
}

function restoreEditorPos() {
    if (topLineNumber === undefined) {
        return;
    }
    editor.refresh();
    const newTopLineY = editor.charCoords({ line: topLineNumber, ch: 0 }, "local").top;
    editor.scrollTo(null, newTopLineY);
}

function isChrome() {
    var winNav = window.navigator;
    var vendorName = winNav.vendor;

    var isChromium = window.chrome;
    var isOpera = typeof window.opr !== "undefined";
    var isFirefox = winNav.userAgent.indexOf("Firefox") > -1;
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
