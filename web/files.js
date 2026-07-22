// files.js - connects three worlds:
// - the disk (File System Access API), an
// - in-memory mirror (`files`)
// - and a snapshot of what the server has (`server`, persisted to localStorage).
//
// Sync model: each batch round-trip sends modified
// files, locally-deleted paths, per-dir timestamp cursors, and a global fslog
// "commit offset" (`server.serverTime`); receives files to pull, renames, and
// server-side deletes from the fslog newer than the watermark.

// User can set his own server apiUrl through localstorage.
const API_URL = localStorage.getItem('apiUrl') || 'https://api.files.md';
const CURRENT_FILE_SYNC_INTERVAL = 1000; // ms, how often to save currently open file
// Matches server's MaxMediaSize (server/sync/sync.go). Server caps the JSON
// request body, which holds base64 (~33% inflation), so the effective raw
// file limit is roughly 3/4 of this. Files above MAX_MEDIA_SIZE are rejected
// outright; files between 3/4 and 1 of MAX_MEDIA_SIZE may still be refused
// by the server when base64 pushes the body past the cap.
const MAX_MEDIA_SIZE = 65 * 1024 * 1024;

let isSaving = false;
let isSyncingFiles = false;
let isSyncingMedia = false;
let isMessingWithCurrentEditor = false;
let isSyncingFileWithServer = {}; // path -> bool, prevents concurrent server syncs for the same file
let needsResyncWithServer = {}; // path -> bool, flags that another sync was requested while one was in flight
let isLoadingLocalFiles = false;

// We should know if we had at least one successful
// communication with the server (/token), so that
// we run sync periodically. We won't run if the app
// is not linked to the server. Unfortunately we can't just
// check "token" cookie, because it is HttpOnly.
const LAST_SERVER_OK_KEY = 'lastServerOk';
const MAX_DIR_NESTING_LEVEL = 10;

function markServerOk() {
    localStorage.setItem(LAST_SERVER_OK_KEY, Date.now().toString());
}

// Sync indicator for the current file: a quiet dot that turns orange while
// the server hasn't yet acked what's in the editor - either because you just
// typed (clears on the next sync tick) or because the server is unreachable.
// Hidden for local-only setups.
let lastSyncOkAt = null;

function renderSyncStatus(state) { // 'ok' | 'edits' | 'error'
    const dot = document.getElementById('sync-status');
    if (dot === null) {
        return;
    }
    const at = lastSyncOkAt === null
        ? 'never'
        : new Date(lastSyncOkAt).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    dot.style.display = 'block';
    dot.classList.toggle('bad', state !== 'ok');
    dot.title = state === 'ok' ? `Synced at ${at}`
        : state === 'edits' ? `Unsynced changes. Last synced at ${at}`
        : `Not synced, server unreachable. Last synced at ${at}`;
}

// Called on every editor change (see initEditor).
function markSyncDirty() {
    if (!hasLastServerOk()) {
        return;
    }
    renderSyncStatus('edits');
}

function hasLastServerOk() {
    return localStorage.getItem(LAST_SERVER_OK_KEY) !== null;
}

// The types of files we have:
// - serverFile, file on server
// - localFile, file on local fs
// - memFile, in-memory representation of local file
// The latter is needed for quick access to file's handle and metadata.

// We operate with absolute paths in our webapp. Server/wasm is currently operating with relative paths (better for safety checks).

// In-memory mapping of local file system:
// {
//   'dir/': [
//     {
//       'filename': [
//         {
//           content: 'File content here...',
//           lastModified: <timestamp>,
//           handle: <file handle>,
//           imageUrl: <image url if any>
//         },
//         ...
//       ]
//     },
//     ...
//   ]
// }
let files = {}; // In-memory representation of local files

// In-memory snapshot of what the server has, persisted to localStorage
// under SERVER_STORAGE_KEY and rehydrated at boot:
// {
//   files: {
//     'dir/': {
//       'filename.md': {
//         hash: '<content hash>',
//         lastModified: <server timestamp>,
//         lastClientModified: <client timestamp the server acknowledged>,
//         path: '/dir/filename.md'
//       },
//       ...
//     },
//     ...
//   },
//   media: {
//     'image.png': {
//       isFile: true,
//       lastModified: <server timestamp>
//     },
//     ...
//   },
//   timestamps: { '<path>': <ts>, ... }, // per-path cursor for incremental sync
//   mediaTimestamp: <max ts across media> // single cursor for media sync
// }
let server = {files: {}, media: {}, timestamps: {}, mediaTimestamp: 0}; // In-memory representation of server

// Reverse index for non-md files (currently just images): filename -> first.
// Lets the editor resolve `![](foo.png)` even when the
// image lives in a folder other than media.
let mediaIndex = {};

const SERVER_STORAGE_KEY = 'server'; // If scheme is migrated, I believe it's better to introduce a new key, because for now old keys aren't removed.
const SUPPORTED_EXTENSIONS = ['md', 'png', 'jpg', 'jpeg', 'webp', 'gif', 'mp4', 'webm', 'mov', 'mp3', 'ogg', 'oga', 'weba', 'wav'];

function isMediaPath(path) {
    return /\.(png|jpg|jpeg|gif|webp|mp4|webm|mov|mp3|ogg|oga|weba|wav)$/i.test(path);
}
const SYSTEM_DIRS = ['media', 'archive', 'journal', 'habits', 'triggers', 'insights'];
const CONFIG_PATH = '/config.json';

async function loadLocalFiles(rootDirHandle, slowMode = false) {
    if (isLoadingLocalFiles) {
        return files;
    }
    isLoadingLocalFiles = true;

    // TODO should we wait for editor2 as well?
    // What if "isClean" is changed mid-through? We have awaits.
    // Better check per/file right before loading?
    while (!editor.isClean()) {
        await new Promise(r => setTimeout(r, 50));
    }

    let newFiles = {};
    // Rebuild the filename->path image index from scratch on every load so
    // renamed/deleted images don't linger in the lookup.
    mediaIndex = {};

    // Loads files recursively
    async function loadDir(dirHandle, path = '/', depth = 0) {
        const entries = [];
        for await (const entry of dirHandle.values()) {
            entries.push(entry);
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));

        const dirPromises = [];
        for (let i = 0; i < entries.length; i++) {
            const entry = entries[i];
            const filename = entry.name.normalize('NFC');

            let isSupportedExtension = SUPPORTED_EXTENSIONS.includes(filename.split('.').pop().toLowerCase());
            let isConfig = filename === toFilename(CONFIG_PATH);

            let dirs = path.split('/');
            dirs = dirs.filter(d => d !== '');
            let currentDir = newFiles;
            for (let dir of dirs) {
                dir += '/';
                if (!currentDir[dir]) {
                    currentDir[dir] = {};
                }
                currentDir = currentDir[dir]; // Move reference deeper
            }

            if (entry.kind === 'directory') {
                if (filename.startsWith('.') || depth >= MAX_DIR_NESTING_LEVEL) continue;

                currentDir[filename + '/'] = {};
                const dir = `${path}${filename}/`;
                dirPromises.push({handle: entry, path: dir, depth: depth + 1});
            } else if (entry.kind === 'file' && (isSupportedExtension || isConfig)) {
                // Reuse existing file handle if it exists
                let existingDir = files;
                for (let dir of dirs) {
                    dir += '/';
                    if (existingDir === undefined || existingDir[dir] === undefined) {
                        existingDir = undefined;
                        break;
                    }
                    existingDir = existingDir[dir];
                }

                const fileWasPreviouslyLoaded = existingDir && existingDir[filename] !== undefined
                if (fileWasPreviouslyLoaded) {
                    currentDir[filename] = existingDir[filename];
                } else {
                    currentDir[filename] = {path: `${path}${filename}`, isFile: true, handle: entry};
                    entry.getFile().then(file => {
                        currentDir[filename].lastModified = file.lastModified;
                    });
                }

                if (!isMediaPath(filename)) {
                    continue
                }

                if (!currentDir[filename].imageUrl) {
                    getImageUrl(entry).then(imageUrl => {
                        currentDir[filename].imageUrl = imageUrl;
                    });
                }
                // Index every image by its bare filename. First write wins.
                if (!mediaIndex[filename]) {
                    mediaIndex[filename] = currentDir[filename];
                }
            }
            if (slowMode && i % 50 === 0) {
                await new Promise(r => setTimeout(r, 0));
            }
        }

        if (debug) {
            if (!debug.loaded) {
                debug.loaded = true
                await loadDir(rootDirHandle, debug.dir, 1);
            }
            return;
        }

        if (!slowMode) {
            await Promise.all(dirPromises.map(({handle, path, depth}) =>
                loadDir(handle, path, depth)
            ));
            return;
        }

        const batchSize = 6;
        for (let i = 0; i < dirPromises.length; i += batchSize) {
            const batch = dirPromises.slice(i, i + batchSize);
            await Promise.all(batch.map(({handle, path, depth}) =>
                loadDir(handle, path, depth)
            ));
            await new Promise(r => setTimeout(r, 0));
        }
    }

    try {
        await loadDir(rootDirHandle);
    } catch (error) {
        log('Load Local files: ', error);
        isLoadingLocalFiles = false;
        throw error;
    }

    // Load server files
    const savedServerFiles = localStorage.getItem(SERVER_STORAGE_KEY);
    if (savedServerFiles) {
        server = JSON.parse(savedServerFiles);
    }

    isLoadingLocalFiles = false;

    return newFiles;
}

async function syncFilesWithServer() {
    // We should have at least one 200 response from service.
    // The first 200 response we get from /token, meaning that
    // our application is linked to the server for sync.
    if (!hasLastServerOk()) {
        return;
    }
    if (files === undefined || Object.keys(files).length === 0) {
        return;
    }
    if (debug) {
        return;
    }

    if (isSyncingFiles) return;
    isSyncingFiles = true;

    const startTime = performance.now();
    log('Starting sync with server...');

    // Send locally modified files and timestamps of last seen dirs from the server
    // TODO check if we fully synced at least once (timestamps exists)

    let modified = [];
    let deleted = [];
    // TODO is it possible that the server has zero files? I think at least '.' is sent
    let hasFullySyncedFilesAtLeastOnce = server['timestamps'] !== undefined && Object.keys(server['timestamps']).length > 0;
    ;
    if (hasFullySyncedFilesAtLeastOnce) {
        log('SYNCED AT LEAST ONCE, collecting local files', server['timestamps']);
        ({modified, deleted} = await collectModifiedAndDeletedFiles());
    } else {
        log('NEVER SYNCED BEFORE');
    }
    const { json: response, error } = await post('syncFilenames', {
        modified: modified,
        deleted: deleted,
        timestamps: server['timestamps'] || [],
        serverTime: server['serverTime'] || 0,
    });
    if (error) {
        logError('syncFilenames failed:', error);
        isSyncingFiles = false;
        return;
    }

    // Remove info about server files on client
    for (const path of deleted) {
        removeServerFile(path);
    }

    try {
        // Write files received from the server
        let failedAtLeastOnce = false;
        for (const fileInfo of response.files) {
            let {path, content, lastModified} = fileInfo;
            // We get relative paths from server, and in our app we use absolute paths
            const relPath = path;
            path = joinPath('/', relPath);

            // If it is current file, skip, because we sync it separately
            // TODO if we skip current, don't take it's timestamp? We had a bug when sync was broken for 1 file
            // TODO fix missing / for root files
            if (path === editor.path || path === editor2.path) {
                log('Skip receiving current file during bath sync', path);
                continue;
            }

            try {
                const lastClientModified = await writeIfContentIsDifferent(path, content)
                addMemFile(path, {
                    isFile: true,
                    content: content,
                    lastModified: lastModified,
                    lastClientModified: lastClientModified,
                    path: path,
                    handle: await getFileHandle(path),
                });

                log('SYNC texts: write file: ', path);
                setServerFile(path, content, lastModified, lastClientModified);
                // Unfortunately rename is not working, so we have to delete the old file
                const shouldRemoveOldFile = response.renames !== null && relPath in response.renames;
                // TODO write e2e for renames
                if (shouldRemoveOldFile) {
                    const oldPath = joinPath('/', response.renames[relPath]);
                    try {
                        log('DELETED due to renaming', oldPath);
                        await remove(oldPath);
                    } catch (err) {
                        log('RENAME: cant remove file: ', err, path);
                    }
                }
                saveServerFiles();
            } catch (error) {
                logError(`Error saving file ${path}:`, error);
                if (!error.message.includes('Name is not allowed')) {
                    failedAtLeastOnce = true;
                }
            }
        }
        // Apply server-side deletions: drop any local file that was deleted on
        // server. Local copies older than the recorded deletedAt are deleted.
        // If local change is newer than deletedAt - we skip deletion.
        if (response.deleted) {
            const serverTime = server['serverTime'] || 0;
            for (const [relPath, deletedAt] of Object.entries(response.deleted)) {
                const path = joinPath('/', relPath);
                const local = getMemFile(path);
                if (!local) continue;
                if (local.lastModified > deletedAt) continue;
                try {
                    log('SYNC: deleting locally due to server fslog:', path);
                    // await remove(path);
                    // removeServerFile(path);
                } catch (err) {
                    logError('SYNC: cant delete locally:', err, path);
                }
            }
            server['serverTime'] = serverTime;
            saveServerFiles();
        }

        // Only move timestamp pointers when we were able to sync all the files.
        // Otherwise we can have situation when we synced files only partially,
        // let's say serverFiles is having only half files from server, then they
        // will be sent by subsequent syncTexts call, because collectLocalFiles
        // would report them as new.
        if (!failedAtLeastOnce) {
            log('BATCH sync ok, moving timestamps');
            server['timestamps'] = response.timestamps;
            saveServerFiles();
        } else {
            log("BATCH sync error, timestamps aren't moved");
        }
    } catch (error) {
        logError("Can't sync:", error.message)
    }

    log('Sync completed in ' + (performance.now() - startTime) + 'ms');

    isSyncingFiles = false;
}

async function syncLocalFileWithServer(path) {
    // We should have at least one 200 response from service.
    // The first 200 response we get from /token, meaning that
    // our application is linked to the server for sync.
    if (!hasLastServerOk()) {
        return
    }

    if (isSyncingFileWithServer[path]) {
        needsResyncWithServer[path] = true;
        return;
    }

    isSyncingFileWithServer[path] = true;
    try {
        let file = await (await getFileHandle(path)).getFile();
        // TODO we might only need to send content when modifying
        let content = await file.text();
        let serverTimestamp = getServerFile(path)?.lastModified || 0;

        let serverFile = {};
        const clientLastModified = file.lastModified;
        const { json, error } = await post('syncFile', {
            path: path,
            lastModified: serverTimestamp,
            clientLastModified: clientLastModified,
            // We take the last client timestamp known to the server. Server can
            // decide whether the file was modified on client or not.
            clientLastSynced: getServerFile(path)?.lastClientModified || 0,
            content: content,
        });
        if (error) {
            logError(`syncText ${path} failed:`, error);
            if (window.currentEditor?.path === path) {
                renderSyncStatus('error');
            }
            return;
        }
        // The server acked `content` - but only report "synced" if the source
        // of truth still holds exactly that; edits made mid-flight stay orange
        // until the next tick confirms them. In chat mode messages are written
        // straight to the file (not through the editor), so compare against a
        // fresh read of the file; for the editor, getCurrentContent() (not
        // getValue()) because the `# Filename` header line is stripped from
        // what is written and synced.
        if (window.currentEditor?.path === path) {
            const truth = (typeof isChat !== 'undefined' && isChat && path === CHAT_PATH)
                ? await (await (await getFileHandle(path)).getFile()).text()
                : getCurrentContent();
            if (truth === content) {
                lastSyncOkAt = Date.now();
                renderSyncStatus('ok');
            }
        }

        // For the cases when server was updated only on server, we move lastSyncedAt pointer,
        // meaning that there are no "dirty" changes on client.
        if (json.status === 'notModified') {
            setServerFileLastClientModified(path, clientLastModified);
            return;
        }
        if (json.status === 'updatedOnServer') {
            // TODO maybe RC here? When file was updated, but during this time we already changed it
            setServerFile(path, content, json.lastModified, clientLastModified);
            log(`Moved lastModified for ${path} with timestamp ${json.lastModified}`, json);
            saveServerFiles();
            return;
        }
        // if status is "merged" or "ok", it means it means we have changes to write.
        serverFile = json;

        // We have either of these:
        // 1) New file from server
        // 2) Modified only on server
        // 3) Merged on server
        const lastClientModified = await writeIfContentIsDifferent(path, serverFile.content);
        setServerFile(path, serverFile.content, serverFile.lastModified, lastClientModified);
        log(`Saved server file for ${path} with timestamp ${serverFile.lastModified}`);
        saveServerFiles();
        if (path === editor.path) {
            log('Opening file after sync');
            await openFile(path);
        }
        if (path === editor2.path) {
            log('Opening file after sync in editor2');
            await openFile(path, true, 'editor2-textarea');
        }
        log('File synced with server');
    } finally {
        isSyncingFileWithServer[path] = false;
        if (needsResyncWithServer[path]) {
            needsResyncWithServer[path] = false;
            await syncLocalFileWithServer(path);
        }
    }
}

async function syncMediaFiles() {
    // We should have at least one 200 response from service.
    // The first 200 response we get from /token, meaning that
    // our application is linked to the server for sync.
    if (!hasLastServerOk()) {
        return;
    }
    if (files === undefined) {
        return;
    }
    if (isSyncingMedia) {
        return;
    }
    if (debug) {
        return;
    }

    isSyncingMedia = true;

    const startTime = performance.now();

    const mediaTimestamp = server['mediaTimestamp'] || 0;
    if (mediaTimestamp !== 0) {
        // Send new files from client to server
        let newMedias = await collectNewMediaFiles();
        for (const mediaFilename of newMedias) {
            try {
                // TODO improve that hardcode :D
                let fileHandle = await getFileHandle('media/' + mediaFilename)
                let file = await fileHandle.getFile();
                if (file.size > MAX_MEDIA_SIZE) {
                    logError(`Skipping ${mediaFilename}: ${(file.size / 1024 / 1024).toFixed(1)} MB exceeds ${(MAX_MEDIA_SIZE / 1024 / 1024).toFixed(0)} MB limit`);
                    continue;
                }
                const arrayBuffer = await file.arrayBuffer();
                const uint8Array = new Uint8Array(arrayBuffer);
                let binaryString = '';
                for (let i = 0; i < uint8Array.length; i++) {
                    binaryString += String.fromCharCode(uint8Array[i]);
                }
                const base64String = btoa(binaryString);

                // Raw fetch: the upload reply is empty (not JSON), so the
                // JSON-based post() helper doesn't apply here.
                const response = await fetch(`${API_URL}/syncMediaFile`, {
                    method: 'POST',
                    credentials: 'include',
                    headers: {
                        'Content-Type': 'application/json',
                        'Version': getCurrentVersion(),
                    },
                    body: JSON.stringify({
                        filename: mediaFilename,
                        data: base64String,
                    }),
                });
                if (!response.ok) {
                    let body = '';
                    try { body = await response.text(); } catch (_) {}
                    logError(`Failed to sync media file ${mediaFilename}: ${response.status} ${response.statusText}: ${body}`.trim());
                } else {
                    markServerOk();
                    server['media'][mediaFilename] = {
                        isFile: true,
                        lastModified: 0, // We don't track binary files modifications.
                    };
                    saveServerFiles();
                    log(`Successfully synced media file: ${mediaFilename}`);
                }
            } catch (error) {
                logError(`Error syncing media file ${mediaFilename}:`, error);
            }
        }
    }
    try {
        const { json: serverData, error } = await post('syncMediaFilenames', {
            timestamp: mediaTimestamp,
        });
        if (error) {
            logError('syncMediaFilenames failed:', error);
            isSyncingMedia = false;
            return;
        }

        let filesProcessed = 0;
        for (const fileInfo of serverData.files) {
            const {filename, lastModified} = fileInfo;
            log(`Downloading media file: ${filename}`);

            try {
                // Raw fetch: this endpoint streams a binary blob, not JSON,
                // so the JSON-based post() helper doesn't apply here.
                const response = await fetch(`${API_URL}/syncMediaFile`, {
                    method: 'POST',
                    credentials: 'include',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        filename: filename,
                        timestamp: mediaTimestamp
                    })
                });
                if (!response.ok) {
                    logError(`Failed to download ${filename}: ${response.status}`);
                    continue;
                }
                markServerOk();

                const blob = await response.blob();
                await saveMediaFile(`media/${filename}`, blob, lastModified);
                filesProcessed++;
            } catch (error) {
                logError(`Error processing media file ${filename}:`, error);
            }
        }

        log(`Media sync completed in ${(performance.now() - startTime).toFixed(2)}ms. Downloaded ${filesProcessed} files.`);
    } catch (error) {
        logError('Network error during media sync:', error.message);
    }

    isSyncingMedia = false;
}

// Saves media file and moves pointer
async function saveMediaFile(path, blob, lastModified) {
    const fileHandle = await getFileHandle(path, true);
    if (fileHandle === null) {
        log(`Malformed name for ${path}, skipping file...`);
        return;
    }

    // Check if file exists already
    try {
        const file = await fileHandle.getFile();
        const fileExists = file.size > 0;
        if (fileExists) {
            if (server['mediaTimestamp'] === undefined || lastModified > server['mediaTimestamp']) {
                server['mediaTimestamp'] = lastModified;
            }
            server['media'][file.name] = {
                isFile: true,
                lastModified: lastModified,
            }
            saveServerFiles();
            log(`File ${path} already exists and is up to date, skipping...`);
            return;
        }
    } catch (error) {
        log(`File ${path} doesn't exist or can't be read, will create it`);
    }

    try {
        const parts = path.split('/');
        let filename = parts.pop();

        const writable = await fileHandle.createWritable();
        await writable.write(blob);
        await writable.close();
        log(`Successfully wrote media file: ${path}`);
        if (lastModified > server['mediaTimestamp']) {
            server['mediaTimestamp'] = lastModified;
        }
        server['media'][filename] = {
            isFile: true,
            lastModified: lastModified,
        }
        saveServerFiles();

        // Load file handle into files
        files['media/'][filename] = {isFile: true, handle: fileHandle};
        fileHandle.getFile().then(file => {
            files['media/'][filename].lastModified = file.lastModified;
        });
        getImageUrl(fileHandle).then(imageUrl => {
            files['media/'][filename].imageUrl = imageUrl;
        });
    } catch (error) {
        logError(`Error writing media file ${path}:`, error);
        throw error;
    }
}

// TODO rename textFiles?
async function collectModifiedAndDeletedFiles() {
    const modifiedFiles = [];
    const existingFiles = {};
    const promises = [];

    // Freeze paths to prevent RC. Current file can change during collecting.
    const editorPath = editor.path;
    const editor2Path = editor2.path;
    log('Frozen paths:', editorPath, editor2Path);
    walk(files, (path, isFile) => {
        if (!isFile) {
            return;
        }

        if (path.startsWith('/media/') || path === LOG_PATH) {
            return;
        }
        // Binary media files (images, video) anywhere in the tree must not
        // go through the text sync path - file.text() corrupts them and the
        // JSON-escaped string can balloon past MaxFilenamesSize, returning
        // 400 from syncFilenames. They sync via syncMediaFile when in /media/.
        if (isMediaPath(path)) {
            return;
        }

        // TODO write tests for that?
        if (path === editorPath || path === editor2Path) {
            log('Skip sending current file: ' + path);
            return;
        }

        const promise = getFileStatus(path)
            .then(result => {
                if (result.status === 'modified' || result.status === 'new') {
                    modifiedFiles.push(result);
                }

                if (result.status !== 'error') {
                    existingFiles[result.path] = true;
                } else {
                    console.warn(`Error getting status for file ${path}:`, result);
                }
            });
        promises.push(promise);
    });

    await Promise.all(promises);

    // Find deleted files that are in server files but not in existing files.
    let deleted = [];
    walk(server.files, (path, isFile) => {
        if (!isFile) {
            return;
        }

        // Chromium doesn't support those chars on any OS
        if (/[<>:'|?*\\/\x00-\x1F\x7F]/.test(toFilename(path))) {
            return;
        }

        // Skip current files.
        if (path === editorPath || path === editor2Path) {
            return;
        }

        if (existingFiles[path] === undefined) {
            log('DELETED because not in existing or modified files:', path);
            log('Current editors paths:', editor.path, editor2.path);
            // Log files entry
            log('Mem file:', getMemFile(path));
            deleted.push(path);
        }
    });

    // If there are too many deleted files, prob something is wrong, throw an alert
    if (deleted.length > 20) {
        alert(`Trying to delete more than 20 deleted files during sync (${deleted.length}). I won't proceed, please resolve the issue manually. Probably "files" is empty in local stroage for some reason, but there are actual files on the disk.`);
        // Show first 10 files
        alert('First 10 files: \n' + deleted.slice(0, 10).join('\n'));
        localStorage.removeItem("server");
        throw new Error('Too many deleted files during sync, aborting.');
        deleted = [];
    }

    return {
        modified: modifiedFiles,
        deleted: deleted,
    };
}

async function collectNewMediaFiles() {
    if (!files['media/']) {
        return {
            newMedia: [],
        };
    }

    const newMediaFiles = [];
    for (const filename in files['media/']) {
        if (server['media'] === undefined || !(filename in server['media'])) {
            newMediaFiles.push(filename);
        }
    }

    log('NEW FILENAMES', newMediaFiles);

    return newMediaFiles;
}

async function getFileStatus(path) {
    let content;
    try {
        const memFile = getMemFile(path);
        let fileHandle = memFile?.handle;
        // First try to get the file from memory, if not found try to open from local fs.
        if (!(fileHandle instanceof FileSystemFileHandle)) {
            fileHandle = await getFileHandle(path, false);
        }
        if (!(fileHandle instanceof FileSystemFileHandle)) {
            logError("Error while getting file handle for status check", path);
            return {
                status: 'error',
            }
        }

        const file = await memFile.handle.getFile();
        content = await file.text();
    } catch (error) {
        logError('Error while getting status for file', path, error);
        return {
            status: 'error',
        }
    }

    // TODO why path is stored at all?
    // const path = serverFiles?.files?.[dir]?.[filename]?.path;
    let serverFile = getServerFile(path);
    // log('STATUS', path, serverFile);
    if (serverFile === null) {
        log('NEW LOCAL FILE ' + path);
        return {
            status: 'new',
            content: content,
            path: path,
            lastModified: 0 // new file
        }
    }

    const serverHash = serverFile.hash;
    const serverTime = serverFile.lastModified;
    if (serverHash !== hash(content)) {
        log('NEW MODIFIED LOCAL FILE ' + path);
        return {
            status: 'modified',
            content: content,
            path: path,
            lastModified: serverTime,
        };
    }

    return {
        status: 'notModified',
        path: path,
    };
}

// TODO split into two, sometimes we need just compare
async function isContentEqual(path, content) {
    let fileHandle = await getFileHandle(path);
    if (fileHandle === null) {
        // TODO fix once Chromium fixes the bug
        console.warn('Malformed name, skipping file...');
        return false;
    }

    let file = await fileHandle.getFile()
    let clientHash = hash(normNewLines(await file.text()));
    let serverHash = hash(normNewLines(content));
    if (clientHash !== serverHash) {
        // Log string differences in content, not hash
        const clientContent = normNewLines(await file.text());
        const serverContent = normNewLines(content);
        const clientLines = clientContent.split('\n');
        const serverLines = serverContent.split('\n');
        const diff = [];
        for (let i = 0; i < Math.max(clientLines.length, serverLines.length); i++) {
            const clientLine = clientLines[i] || '';
            const serverLine = serverLines[i] || '';
            if (clientLine !== serverLine) {
                diff.push(`Line ${i + 1}: '${clientLine}' vs '${serverLine}'`);
            }
        }

        // log(diff);

        return false;
    } else {
        return true;
    }
}

function getImageExtension(mimeType) {
    const extensions = {
        'image/png': 'png',
        'image/jpeg': 'jpg',
        'image/jpg': 'jpg',
        'image/gif': 'gif',
        'image/webp': 'webp',
        'video/mp4': 'mp4',
        'video/webm': 'webm',
        'video/quicktime': 'mov',
        'audio/mpeg': 'mp3',
        'audio/mp3': 'mp3',
        'audio/ogg': 'ogg',
        'audio/wav': 'wav',
        'audio/x-wav': 'wav',
        // audio-only WebM gets the dedicated .weba extension so fold-image.js
        // routes it through the <audio> path (the video regex still owns .webm).
        'audio/webm': 'weba'
    };
    return extensions[mimeType] || 'png';
}

// TODO can we reuse moveFile?
async function moveCurrentFile(toDir) {
    const oldPath = currentEditor.path;
    const newPath = joinPath('/', toDir, toFilename(currentEditor.path));
    if (oldPath === newPath) return;

    isMessingWithCurrentEditor = true;

    try {
        let content = getCurrentContent();
        await writeIfContentIsDifferent(newPath, content);
        // TODO move to saveTextFile?
        removeMemFile(oldPath);
        // delete files[editor.currentDir][editor.currentFile];
        log('MOVING to DIR:', toDir);

        addMemFile(newPath, {
            isFile: true,
            content: content,
            lastModified: 0,
            path: newPath,
            handle: await getFileHandle(newPath),
        });

        currentEditor.path = newPath;
        setServerFile(newPath, content, 0);
        saveServerFiles();

        await remove(oldPath);
        await renderSidebar();
    } catch (error) {
        logError('Error moving file:', error);
    }

    isMessingWithCurrentEditor = false;
}

// TODO lock on files modification?
function addMemFile(path, memFile) {
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = files;
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            currentDir[dir] = {};
        }
        currentDir = currentDir[dir];
    }

    currentDir[filename] = memFile;

    // Only sort the specific directory that was modified
    // TODO multidir
    // const sortedFiles = {};
    // const sortedKeys = Object.keys(files[dir]).sort((a, b) => a.localeCompare(b));
    // for (const key of sortedKeys) {
    //     sortedFiles[key] = files[dir][key];
    // }
    // files[dir] = sortedFiles;

    // Remove the global re-sorting - it's messing up the natural order
    // The directory order should stay as established by loadLocalFiles
}

async function moveFile(oldPath, newPath) {
    if (oldPath === newPath) {
        return;
    }

    try {
        let file = await (await getFileHandle(oldPath)).getFile();
        let content = await file.text();
        await writeIfContentIsDifferent(newPath, content);

        log('saving ' + newPath);
        addMemFile(newPath, {
            isFile: true,
            content: content,
            lastModified: 0,
            path: newPath,
            handle: await getFileHandle(newPath),
        });
        // Don't preemptively setServerFile here - that would stamp the
        // server snapshot with hash(content), which makes getFileStatus
        // return 'notModified' on the next sync and the server never
        // receives newPath. Leaving serverFile null lets the sync see
        // it as 'new' and push it normally.

        // Server file will be removed here.
        await remove(oldPath);
        // delete files[oldDir][oldFilename];
        await renderSidebar();

        log(`Moved ${oldPath} to ${newPath}`);
    } catch (error) {
        logError('Error moving file:', error);
    }
}

// Returns server file or null if not found.
// {
//     hash,
//     lastModified,
//     lastClientModified,
//     path,
// }
function getServerFile(path) {
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = server['files'];
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            return null;
        }
        currentDir = currentDir[dir];
    }

    return currentDir[filename] || null;
}

function setServerFile(path, content, lastModifiedAt, lastClientModifiedAt = null) {
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = server['files'];
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            currentDir[dir] = {};
        }
        currentDir = currentDir[dir];
    }

    currentDir[filename] = {
        isFile: true,
        hash: hash(content),
        lastModified: lastModifiedAt,
        lastClientModified: lastClientModifiedAt,
        path: path,
    }
}

function setServerFileLastClientModified(path, lastClientModified) {
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = server['files'];
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            currentDir[dir] = {};
        }
        currentDir = currentDir[dir];
    }

    currentDir[filename].lastClientModified = lastClientModified;
}

function removeServerFile(path) {
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = server['files'];
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            return null;
        }
        currentDir = currentDir[dir];
    }

    if (currentDir[filename] !== undefined) {
        delete currentDir[filename];
    }
}

function saveServerFiles() {
    localStorage.setItem(SERVER_STORAGE_KEY, JSON.stringify(server));
}

async function openFile(path, saveToHistory = true, el = 'editor-textarea') {
    // There are a few awaits during syncCurrentFile, we should not change currentEditor during that.
    while (isMessingWithCurrentEditor) {
        log('Waiting isMessingWithCurrentEditor...');
        await new Promise(r => setTimeout(r, 50));
    }

    const id = opId();
    log(`Opening file: ${path} in element: ${el}, opId: ${id}`, id);

    // Why we do normalize here as well?
    path = path.normalize('NFC');
    const memFile = getMemFile(path);
    if (memFile === null) {
        return;
    }

    // Save the old file
    // TODO what if we open same file?
    if (el === 'editor-textarea') {
        currentEditor = editor;
    } else if (el === 'editor2-textarea') {
        currentEditor = editor2;
    }
    // Only sync the switch-away editor when it has unsaved changes.
    let thereIsPreviousEditorToSync =  !currentEditor.isClean() &&currentEditor.path !== undefined;
    if (thereIsPreviousEditorToSync) {
        const syncStart = performance.now();
        log('Began syncing previous file');
        await syncCurrentFile(true);
        log(`Finished syncing previous file in ${(performance.now() - syncStart).toFixed(3)} ms`);
    }

    // Lock the current editor during the operation, so we won't interrupt syncCurrentEditor in the middle.
    // By this time it is guaranteed to be free because we've just waited for "syncCurrentEditor".
    // We should do this before any awaits.
    isMessingWithCurrentEditor = true;
    try {
        if (path === CHAT_PATH) {
            openChat();
            return;
        } else {
            const codemirror = document.querySelector('.CodeMirror-wrap');
            codemirror.style.display = 'block';
            chat.style.display = 'none';
            chatInput.style.display = 'none';
            isChat = false;
        }
        // chatButton.classList.remove('hidden');
        chatContainer.style.display = 'none';
        closeChatModal();

        const start = performance.now();

        const isSameFile = currentEditor.path === path;

        let filename = toFilename(path);
        const header = toHeader(filename)
        let content = '';
        if (memFile.handle !== undefined) {
            const file = await memFile.handle.getFile();
            content = await file.text();
            content = `${header}\n${content}`;
        } else {
            // We use welcome's files
            content = memFile.content;
        }

        currentEditor.path = path;
        if (saveToHistory) {
            const state = {
                path: path,
                el: el,
            };
            history.pushState(state, '');
        }

        if (isSameFile) {
            // Same-file reload (e.g. API sync or changes from local fs). Diff old and new content and
            // replaceRange only the differing middle — cursor and scroll stay put
            // naturally when the edit doesn't span them.
            if (el === 'editor2-textarea') {
                showEditor2();
            }
            currentEditor.path = path;
            const oldContent = currentEditor.getValue();
            if (oldContent !== content) {
                let prefixEnd = 0;
                const minLen = Math.min(oldContent.length, content.length);
                while (prefixEnd < minLen && oldContent[prefixEnd] === content[prefixEnd]) {
                    prefixEnd++;
                }
                let oldEnd = oldContent.length;
                let newEnd = content.length;
                while (oldEnd > prefixEnd && newEnd > prefixEnd
                && oldContent[oldEnd - 1] === content[newEnd - 1]) {
                    oldEnd--;
                    newEnd--;
                }
                currentEditor.replaceRange(
                    content.substring(prefixEnd, newEnd),
                    currentEditor.posFromIndex(prefixEnd),
                    currentEditor.posFromIndex(oldEnd)
                );
            }
            currentEditor.markClean();
        } else {
            // New editors are initialized to avoid visual glitches and refresh plugins' state.
            if (el === 'editor-textarea') {
                editor = initEditor(document.getElementById(el));
                currentEditor = editor;
                hideEditor2();
            } else if (el === 'editor2-textarea') {
                editor2 = initEditor(document.getElementById(el));
                currentEditor = editor2;
                showEditor2();
            }

            currentEditor.path = path;
            currentEditor.getDoc().setValue(content);
            currentEditor.clearHistory();
            currentEditor.markClean();
            focusLastLine();
        }

        const end = performance.now();
        log(`File ${path} opened in: ${(end - start).toFixed(3)} milliseconds, opId: ${id}`);

        // Once we spent enough time in file, set viewportMargin to infinity to prevent artefacts.
        // Artefacts can be observed during text selection (cmd+a).
        // Also cmd+f (native find) doesn't work on lazy-loaded documents =(
        setTimeout(() => {
            currentEditor.setOption('viewportMargin', Infinity);
        }, 200);

        selectSidebarItem(path);
    } catch (err) {
        logError('openFile:', err);
        throw err;
    } finally {
        isMessingWithCurrentEditor = false;
    }
}

// 0) Read content from local fs
// 1) Save current editor content to local filesystem if there's diff
// 2) Sync it with the server
// TODO add hash of last read file comparison, merge on conflict (in which scenarios in can happen though?)
// TODO It should be atomic.
// If currentEditor is changed during the execution of this function, we'll have RC.
// So, wherever we change currentEditor reference, we should lock via isMessingWithCurrentEditor.
async function syncCurrentFile(switchAwayEditor = false) {
    if (files === undefined || debug || currentEditor.path === undefined) {
        return;
    }

    // Skip sync if we don't have a saved dir
    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir && !isMemFS) {
        return;
    }

    if (isSaving) {
        return;
    }

    // TODO replace with queues?
    if (isMessingWithCurrentEditor) {
        return;
    }
    isMessingWithCurrentEditor = true;

    const path = currentEditor.path;
    let isCurrentEditorSame = () => {
        return path === window.currentEditor.path;
    }

    if (path === CHAT_PATH) {
        // Try to load local changes.
        if (chatIsClean) {
            try {
                let inMemoryLastModified = getMemFile(path)?.lastModified;
                let file = await ((await getFileHandle(CHAT_PATH)).getFile());

                // Update last modified in memory.
                let memFile = getMemFile(path);
                if (memFile !== null) {
                    memFile.lastModified = file.lastModified;
                    addMemFile(path, memFile);
                }

                let localLastModified = file.lastModified;
                // TODO inmemory lastmodified should be reloaded
                if (inMemoryLastModified !== localLastModified) {
                    isMessingWithCurrentEditor = false;
                    if (!switchAwayEditor) {
                        await openFile(CHAT_PATH);
                    }
                    return;
                }
            } catch (e) {
                logError('Error opening file:', e);
                isMessingWithCurrentEditor = false;
                return;
            }
        }

        isMessingWithCurrentEditor = false;

        if (!switchAwayEditor) {
            try {
                await syncLocalFileWithServer(CHAT_PATH);
            } catch (error) {
                logError('Error during sync with server:', error);
            }
        }

        return;
    }

    // Track in-editor renaming based on header.
    const filename = toFilename(path);
    try {
        // TODO track if no first line?
        const firstLine = currentEditor.getValue().split('\n')[0];
        const rawFromHeader = ucfirst(fromHeaderToFilename(firstLine));
        const badHeaderChar = findForbiddenChar(rawFromHeader);
        if (badHeaderChar !== null) {
            showToast(`Filename cannot contain "${badHeaderChar}"`);
        }
        let newFilename = sanitizeFilename(rawFromHeader);
        // If filename is empty, generate an available "Untitled" name
        // TODO We don't handle txt renaming here
        let hasEmptyName = newFilename.trim() === '.md';
        if (hasEmptyName) {
            let hasOldName = !filename.startsWith('Untitled');
            if (hasOldName) {
                newFilename = 'Untitled.md';
                let counter = 1;
                // TODO multidir
                // while (files[dir][newFilename]) {
                //     newFilename = `Untitled ${counter}.md`;
                //     counter++;
                // }
            } else {
                // TODO add tests
                // Already renamed to untitled
                newFilename = filename;
            }
        }

        const hasFilenameChanged = newFilename.toLowerCase() !== filename.toLowerCase();
        if (hasFilenameChanged) {
            log('Filename has changed from ', filename, 'to', newFilename);

            const newPath = joinPath(toDirPath(path), newFilename);


            // Do not delete existing files with same name. Compare names
            // case-insensitively, as most local filesystems do.
            let dirFiles = files;
            for (const dir of toDirPath(path).split('/').filter(s => s !== '')) {
                dirFiles = dirFiles && dirFiles[dir + '/'];
            }
            const nameTaken = dirFiles && Object.keys(dirFiles).some(
                name => !name.endsWith('/') && name.toLowerCase() === newFilename.toLowerCase());
            if (nameTaken) {
                showToast(`File "${trimPostfix(newFilename, '.md')}" already exists`);
                isMessingWithCurrentEditor = false;
                return;
            }

            let content = getCurrentContent();

            // Probe the new path before deleting the old file. Sanitization should
            // catch the common cases, but if the filesystem still rejects the name
            // for any reason (reserved Windows names, length limits, …), we'd
            // otherwise lose the file entirely.
            let newHandle;
            try {
                newHandle = await getFileHandle(newPath, true);
            } catch (error) {
                logError('Cannot rename, filesystem rejected new name:', newPath, error);
                alert(`Cannot rename file to "${newFilename}": ${error.message || error.name}`);
                isMessingWithCurrentEditor = false;
                return;
            }

            // Change the file immediately, because on further await calls it can be synced by syncTexts.
            currentEditor.path = newPath;

            // 1. Remove file with old filename
            // 2. Create file with new filename

            // TODO every await means we can can have RC due to editor content change
            await remove(path);
            log('Removed due to filename change', path);

            addMemFile(newPath, {
                isFile: true,
                content: content,
                lastModified: 0,
                path: newPath,
                handle: newHandle,
            });
            await writeIfContentIsDifferent(newPath, getCurrentContent());
            setServerFile(newPath, content, 0);
            saveServerFiles();
            log('Created', newPath);

            await renderSidebar();

            // Used further for syncing.
            // filename = newFilename;

            // Let's call it a day?
            isMessingWithCurrentEditor = false;
            return;
        }
    } catch (error) {
        logError('Error during filename change:', error);
        isMessingWithCurrentEditor = false;
        return;
    }

    // TODO better way would be this:
    // Read file from fs with it's timestamp
    // If in our memory we have actual TS, just write file back
    // If fs has fresher change, merge.
    // Sync with server.
    const content = getCurrentContent();
    let contentWasModifiedLocally = false;
    try {
        contentWasModifiedLocally = !await isContentEqual(path, content);
    } catch (error) {
        logError('Error checking content equality:', error);
        isMessingWithCurrentEditor = false;
        return;
    }

    // I believe that after each await we should check that user hasn't changed the editor.
    if (!isCurrentEditorSame()) {
        isMessingWithCurrentEditor = false;
        return;
    }

    // Handling editor changes.
    if (contentWasModifiedLocally && currentEditor.isClean()) {
        log('Was modified locally, and the editor is clean', path);

        // Changes only from local system
        try {
            if (!switchAwayEditor) {
                isMessingWithCurrentEditor = false;
                const el = currentEditor === editor2 ? 'editor2-textarea' : 'editor-textarea';
                await openFile(path, false, el);
            }
        } catch (error) {
            logError('Error opening file:', error);
            isMessingWithCurrentEditor = false;
            return;
        }
    } else if (!currentEditor.isClean()) {
        log('Editor is not clean', path);

        isSaving = true;
        try {
            // const file = files[dir][filename];
            const file = getMemFile(path);
            if (file && file.handle) {
                const freshContent = getCurrentContent();
                if (!currentEditor.isClean() && contentWasModifiedLocally) {
                    // Changes from both sides: editor and local fs, need merging

                }
                // We need to atomically reset the flag once we captured a snapshot of particular version of the content.
                // This flag can be changed in the event loop, as a result of user making changes to the text in the middle
                // of our saving process. The new unsaved changes would be then handled by a subsequent saveCurrentFile() call.
                // Initially, this flag assignment was erroneously placed at the end of the function, resulting in a race condition.
                // If we override flag in the end, we would lose any changes that occurred during the 3 await calls.
                currentEditor.markClean();

                const writable = await file.handle.createWritable();
                await writable.write(freshContent);
                // Buffer is flushed on disk at this moment. It could be interrupted
                // by the event loop, so we use isSaving guard.
                await writable.close();
            } else {
                // When could that happen?
                if (file.handle) {
                    logError(`Cannot save ${path}. No file handle found.`);
                }
            }
        } catch (error) {
            logError('Error during save:', error);
            isSaving = false;
            if (isCurrentEditorSame()) {
                // Revert doc back to dirty state
                editor.replaceRange(' ', editor.getCursor());
                editor.undo();
            }
            isMessingWithCurrentEditor = false;
            return;
        }
        isSaving = false;
    }

    isMessingWithCurrentEditor = false;

    if (!switchAwayEditor) {
        try {
            await syncLocalFileWithServer(path);
        } catch (error) {
            logError('Error during sync with server:', error);
        }
    }
}

function hash(str) {
    let hash = 0;
    for (let i = 0, len = str.length; i < len; i++) {
        let chr = str.charCodeAt(i);
        hash = (hash << 5) - hash + chr;
        hash |= 0;
    }

    return hash;
}

// If there are files without isFile flag - we would have recursion.
// Because walk would try to iterate over js object keys.
// Return false from callback to stop walking.
function walk(obj, callback, path = '/') {
    // Chromium's callstack limit is 11K, so we iterate.
    const stack = [{obj, path}];

    const maxAllowedIterations = 100000;
    let iterations = 0;
    while (stack.length > 0) {
        const {obj: currentObj, path: currentPath} = stack.pop();

        // Normally that would never happen.
        // But in case of an error, a watchdog like that can prevent freezing.
        // It once happened when I forgot isFile flag for media. And walk travelled
        // through fileHandle's keys, which were cyclic.
        iterations++;
        if (iterations > maxAllowedIterations) {
            log(currentPath);
            alert("An infinite loop during files walk");
            return;
        }

        if (currentObj.isFile) {
            if (callback(currentPath, true) === false) {
                return;
            }
            continue;
        }

        const isDir = path.endsWith('/');
        if (!isDir) {
            return;
        }

        const keys = Object.keys(currentObj);
        const files = [];
        const dirs = [];

        for (const key of keys) {
            if (currentObj[key].isFile) {
                files.push(key);
            } else {
                dirs.push(key);
            }
        }

        // Process files first
        for (const key of files) {
            const fullPath = currentPath + key;
            if (callback(fullPath, true) === false) {
                return;
            }
        }

        // Fire dir callbacks in forward order so consumers see ABC, not CBA.
        for (const key of dirs) {
            const fullPath = currentPath + key;
            if (callback(fullPath, false) === false) {
                return;
            }
        }

        // Push to stack in reverse so DFS pops them in forward order.
        for (let i = dirs.length - 1; i >= 0; i--) {
            const key = dirs[i];
            const item = currentObj[key];
            const fullPath = currentPath + key;
            stack.push({obj: item, path: fullPath});
        }
    }
}

function walkFilesExcludingSystemDirs(callback) {
    walk(files, (path, isFile) => {
        if (!isFile) {
            return;
        }

        const rootDir = toRootDirName(path);
        if (SYSTEM_DIRS.includes(rootDir) && toRootDirName !== '/') {
            return;
        }

        callback(path);
    });
}

function toFilename(path) {
    if (path === '/') {
        return '/';
    }

    const {filename} = toDirPathAndFilename(path);

    return filename;
}

// Percent-encode a path for use inside a markdown link's `](...)`. Spaces and
// an unescaped `)` both close the link early, breaking any link/image whose
// file name contains them. hmdResolveURL / hmdReadLink decode these back.
function encodeLinkPath(path) {
    return path.replace(/ /g, '%20').replace(/\(/g, '%28').replace(/\)/g, '%29');
}

// Backlink: after a link from `sourcePath` to `targetPath` is inserted, make
// the target point back - append a link to the source at the bottom of the
// target, unless the target already links to it.
async function addBacklink(sourcePath, targetPath) {
    if (!sourcePath || !targetPath) return;
    if (!sourcePath.startsWith('/')) sourcePath = '/' + sourcePath;
    if (!targetPath.startsWith('/')) targetPath = '/' + targetPath;
    if (sourcePath === targetPath) return; // never self-link

    const url = encodeLinkPath(sourcePath);
    const name = toFilename(sourcePath).replace(/\.md$/, '');
    const backlink = `[${name}](${url})`;

    // Separator before the appended backlink: tight (single newline) when the
    // file already ends with a link line, so stacked backlinks don't grow blank
    // lines; a blank line only separates the first backlink from prose.
    const separatorFor = (text) => {
        const body = text.replace(/\s+$/, '');
        if (!body) return '';
        const lastLine = body.slice(body.lastIndexOf('\n') + 1);
        return /^\s*\[/.test(lastLine) ? '\n' : '\n\n';
    };

    // If the target is open in an editor, append through it so the edit goes
    // via the normal save path and isn't clobbered by a stale editor save.
    const open = (editor && editor.path === targetPath && editor)
        || (editor2 && editor2.path === targetPath && editor2) || null;
    if (open) {
        const value = open.getValue();
        if (value.includes(`](${url})`)) return; // already links back
        const doc = open.getDoc();
        const body = value.replace(/\s+$/, '');
        const from = open.posFromIndex(body.length);
        const to = {line: doc.lastLine(), ch: doc.getLine(doc.lastLine()).length};
        doc.replaceRange(separatorFor(value) + backlink, from, to);
        return;
    }

    let content;
    try {
        content = await read(targetPath);
    } catch (e) {
        logError('Backlink: cannot read target', targetPath, e);
        return;
    }
    if (content.includes(`](${url})`)) return; // already links back

    const body = content.replace(/\s+$/, '');
    const newContent = body + separatorFor(content) + backlink + '\n';
    try {
        await write(targetPath, newContent);
    } catch (e) {
        logError('Backlink: cannot write target', targetPath, e);
        return;
    }
    const mem = getMemFile(targetPath);
    if (mem && 'content' in mem) mem.content = newContent;
    try { await syncLocalFileWithServer(targetPath); } catch (e) { /* best effort */ }
}

// Dir with no slash at the end.
// For '/' it returns '/'.
function toDirPath(path) {
    const {dirPath} = toDirPathAndFilename(path);

    return dirPath;
}

function toRootDirName(path) {
    const root = toRootPath(path);
    if (root === '/') {
        return root;
    }

    return trimPrefix(root, '/');
}

// Removes trailing slash if not the root path.
function removeTrailingSlash(path) {
    if (path === '/') {
        return '/';
    }
    if (path.endsWith('/')) {
        return path.slice(0, -1);
    }
    return path;
}

function joinPath(...parts) {
    const joined = parts.join('/');
    return joined.replace(/\/+/g, '/');  // Replace multiple slashes
}

// Dir with no slash at the end.
function toDirPathAndFilename(path) {
    let parts = path.split('/');
    parts = parts.filter(p => p !== '');

    const filename = parts.pop();
    let dirPath = '/' + parts.join('/');
    return {dirPath, filename};
}

function excludeDirs(excludedDirs) {
    const filteredDirs = ['/'];
    for (const dir in files) {
        if (files[dir].isFile === true) {
            continue;
        }

        const dirName = toRootDirName(dir)
        if (!excludedDirs.includes(dirName)) {
            filteredDirs.push(dir);
        }
    }

    return filteredDirs;
}


function toRootPath(path) {
    const parts = path.split('/').filter(p => p !== '');

    if (parts.length <= 1) {
        return '/';
    }

    return '/' + parts[0];
}

// Gets a file from memory by its path.
function getMemFile(path) {
    if (files === undefined) {
        return null;
    }
    if (path === '/') {
        return files;
    }

    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = files;
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            return null;
        }
        currentDir = currentDir[dir];
    }

    return currentDir[filename] || null;
}

function removeMemFile(path) {
    if (files === undefined) {
        return;
    }
    if (path === '/') {
        console.warn('Trying to remove /');
        return;
    }

    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = files;
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            return;
        }
        currentDir = currentDir[dir];
    }

    if (currentDir[filename] !== undefined) {
        delete currentDir[filename];
    }
}

// Returns nextPath for sibling or null
function findSiblingPath(path) {
    const allFiles = [];
    let foundDesiredPath = false;
    let nextPath = null;
    walk(files, (filePath, isFile) => {
        if (filePath === CONFIG_PATH || filePath === CHAT_PATH) {
            return;
        }

        if (filePath.startsWith('/media')) {
            return;
        }

        if (!isFile) {
            return;
        }

        // TODO we may wanna break from walk
        if (foundDesiredPath && nextPath === null) {
            log('NEXT path', filePath);
            nextPath = filePath;
            return;
        }

        if (filePath === path) {
            log('FOUND desired', filePath);
            foundDesiredPath = true;
        }
    });

    return nextPath;
}

async function removeCurrentFile() {
    const path = currentEditor.path;
    if (path === CHAT_PATH) {
        return;
    }

    const nextFilePath = findSiblingPath(path);

    let oldPath = path;
    let newPath = '/archive/' + toFilename(path);

    currentEditor.path = undefined;
    if (toDirPath(path) === '/archive') {
        log('Removing file permanently', path);
        await remove(oldPath);
    } else {
        log('Moving file to archive', path);
        await moveFile(oldPath, newPath);
    }

    await renderSidebar();
    if (nextFilePath) {
        await openFile(nextFilePath);
    } else {
        openRandomFile();
    }
}

async function openRandomFile() {
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
        logError('No files found to open.');
        return;
    }

    const randomPath = allFiles[Math.floor(Math.random() * allFiles.length)];
    try {
        await openFile(randomPath);
    } catch (error) {
        logError('Failed to open random file:', error);
    }
}