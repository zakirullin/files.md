// TODO Do sync not as often
// TODO migrate to unversal file id = filepath, instead of two components
const API_HOST = window.API_HOST || 'https://api.files.md';
// TODO that's quite often. Maybe on edit + focus?
const CURRENT_FILE_SYNC_INTERVAL = 1000; // ms, how often to save currently open file
const LOAD_INTERVAL = 3000; // ms, how often to load current file from local file system

let isSaving = false;
let isSyncingTexts = false;
let isSyncingMedia = false;
let isMessingWithCurrentEditor = false;
let isSyncingFileWithServer = {}; // path -> bool, prevents concurrent server syncs for the same file
let needsResyncWithServer = {}; // path -> bool, flags that another sync was requested while one was in flight
let isLoadingLocalFiles = false;

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
// TODO multidir rename to memFiles?
let files = {}; // In-memory representation of local files
let server = { files: {}, media: {}, timestamps: {}, mediaTimestamp: 0 }; // In-memory representation of server

const SERVER_STORAGE_KEY = 'server'; // If scheme is migrated, I believe it's better to introduce a new key, because for now old keys aren't removed.
const SUPPORTED_EXTENSIONS = ['md', 'txt', 'png', 'jpg', 'jpeg', 'webp', 'gif',];
const SYSTEM_DIRS = ['media', 'archive', '_read_', '_watch_', '_shop_', 'today', 'later', 'journal', 'habits', 'triggers', 'places', 'insights'];
const CONFIG_PATH = '/config.json';

async function loadLocalFiles(rootDirHandle, slowMode = false) {
    if (isLoadingLocalFiles) {
        return;
    }

    while (!editor.isClean()) {
        await new Promise(r => setTimeout(r, 50));
    }

    let newFiles = {};
    isLoadingLocalFiles = true;
    // Loads files recursively
    async function loadDir(dirHandle, path = '/', depth = 3) {
        const entries = [];
        for await (const entry of dirHandle.values()) {
            entries.push(entry);
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));

        const dirPromises = [];
        for (let i = 0; i < entries.length; i++) {
            const entry = entries[i];
            const filename = entry.name.normalize('NFC');

            let isSupportedExtension = SUPPORTED_EXTENSIONS.includes(filename.split('.').pop());
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
                if (filename.startsWith('.') || depth >= 5) continue;

                currentDir[filename + '/'] = {};
                const dir = `${path}${filename}/`;
                dirPromises.push({ handle: entry, path: dir, depth: depth + 1 });
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
                if (existingDir && existingDir[filename] !== undefined) {
                    currentDir[filename] = existingDir[filename];
                    continue;
                }

                currentDir[filename] = { path: `${path}${filename}`, isFile: true, handle: entry };
                entry.getFile().then(file => {
                    currentDir[filename].lastModified = file.lastModified;
                });

                // TODO support any dirs
                if (dirs[0] === 'media' || dirs[0] === 'img') {
                    getImageUrl(entry).then(imageUrl => {
                        currentDir[filename].path = `${path}${filename}`;
                        currentDir[filename].isFile = true;
                        currentDir[filename].imageUrl = imageUrl;
                    });
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
            await Promise.all(dirPromises.map(({ handle, path, depth }) =>
                loadDir(handle, path, depth)
            ));
            return;
        }

        const batchSize = 6;
        for (let i = 0; i < dirPromises.length; i += batchSize) {
            const batch = dirPromises.slice(i, i + batchSize);
            await Promise.all(batch.map(({ handle, path, depth }) =>
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

// config.json is currently only synced from server, no local changes are propogated.
async function syncTextsWithServer() {
    if (files === undefined || Object.keys(files).length === 0) {
        return;
    }
    if (localStorage.getItem('token') === null) {
        return;
    }
    if (debug) {
        return;
    }

    if (isSyncingTexts) return;
    isSyncingTexts = true;

    const startTime = performance.now();
    log('Starting sync with server...');

    // Send locally modified files and timestamps of last seen dirs from the server
    // TODO check if we fully synced at least once (timestamps exists)

    let modified = [];
    let deleted = [];
    // TODO is it possible that the server has zero files? I think at least '.' is sent
    let hasFullySyncedFilesAtLeastOnce = server['timestamps'] !== undefined && Object.keys(server['timestamps']).length > 0;;
    if (hasFullySyncedFilesAtLeastOnce) {
        log('SYNCED AT LEAST ONCE, collecting local files', server['timestamps']);
        ({ modified, deleted } = await collectModifiedAndDeletedFiles());
    } else {
        log('NEVER SYNCED BEFORE');
    }
    const response = await post('syncTexts', {
        modified: modified,
        deleted: deleted,
        timestamps: server['timestamps'] || [],
    });
    if (response === null) {
        isSyncingTexts = false;
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
            let { path, content, lastModified } = fileInfo;
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
                const shouldRemoveOldFile = relPath in response.renames;
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
                console.warn(`Error saving file ${path}:`, error);
                // Don't treat malformed filenames as sync error.
                log(error);
                if (!error.message.includes('Name is not allowed')) {
                    failedAtLeastOnce = true;
                }
            }
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
        console.error("Can't sync:", error.message)
    }

    log('Sync completed in ' + (performance.now() - startTime) + 'ms');

    isSyncingTexts = false;
}

async function syncLocalFileWithServer(path) {
    if (localStorage.getItem('token') === null) {
        return;
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
        try {
            const clientLastModified = file.lastModified;
            let response = await fetch(`${API_HOST}/syncText`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': localStorage.getItem('token'),
                    'Version': getCurrentVersion()
                },
                body: JSON.stringify({
                    path: path,
                    lastModified: serverTimestamp,
                    clientLastModified: clientLastModified,
                    // We take the last client timestamp known to the server. Server can
                    // decide whether the file was modified on client or not.
                    clientLastSynced: getServerFile(path)?.lastClientModified || 0,
                    content: content,
                })
            });
            if (!response.ok) {
                log(`Server responded with ${response.status}`);
                return;
            }
            let json = await response.json();

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
            serverFile = json
        } catch (error) {
            console.error('Network error occurred:', error.message);
            return;
        }

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
        log('File synced with server');
    } finally {
        isSyncingFileWithServer[path] = false;
        if (needsResyncWithServer[path]) {
            needsResyncWithServer[path] = false;
            await syncLocalFileWithServer(path);
        }
    }
}

async function syncMedia() {
    if (files === undefined) {
        return;
    }
    if (isSyncingMedia) {
        return;
    }
    if (localStorage.getItem('token') === null) {
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
                const arrayBuffer = await file.arrayBuffer();
                const uint8Array = new Uint8Array(arrayBuffer);
                let binaryString = '';
                for (let i = 0; i < uint8Array.length; i++) {
                    binaryString += String.fromCharCode(uint8Array[i]);
                }
                const base64String = btoa(binaryString);

                const response = await fetch(`${API_HOST}/syncMedia`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': localStorage.getItem('token'),
                        'Version': getCurrentVersion()
                    },
                    body: JSON.stringify({
                        filename: mediaFilename,
                        data: base64String,
                    })
                });

                if (response.ok) {
                    server['media'][mediaFilename] = {
                        isFile: true,
                        lastModified: 0, // We don't track binary files modifications.
                    };
                    saveServerFiles();
                    log(`Successfully synced media file: ${mediaFilename}`);
                } else {
                    console.error(`Failed to sync media file ${mediaFilename}:`, response.statusText, response, await response.text());
                }
            } catch (error) {
                console.error(`Error syncing media file ${mediaFilename}:`, error);
            }
        }
    }
    try {
        const response = await fetch(`${API_HOST}/syncMedias`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': localStorage.getItem('token'),
                'Version': getCurrentVersion()
            },
            body: JSON.stringify({
                timestamp: mediaTimestamp
            })
        });
        if (!response.ok) {
            console.error(`Server responded with ${response.status}`);
        }

        const serverData = await response.json();

        let filesProcessed = 0;
        for (const fileInfo of serverData.files) {
            const { filename, lastModified } = fileInfo;
            log(`Downloading media file: ${filename}`);

            try {
                // Fetch binary file
                const response = await fetch(`${API_HOST}/syncMedia`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': localStorage.getItem('token')
                    },
                    body: JSON.stringify({
                        filename: filename,
                        timestamp: mediaTimestamp
                    })
                });
                if (!response.ok) {
                    console.error(`Failed to download ${filename}: ${response.status}`);
                    continue;
                }

                const blob = await response.blob();
                await saveMediaFile(`media/${filename}`, blob, lastModified);
                filesProcessed++;
            } catch (error) {
                console.error(`Error processing media file ${filename}:`, error);
            }
        }

        log(`Media sync completed in ${(performance.now() - startTime).toFixed(2)}ms. Downloaded ${filesProcessed} files.`);
    } catch (error) {
        console.error('Network error during media sync:', error.message);
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
        files['media/'][filename] = { isFile: true, handle: fileHandle };
        fileHandle.getFile().then(file => {
            files['media/'][filename].lastModified = file.lastModified;
        });
        getImageUrl(fileHandle).then(imageUrl => {
            files['media/'][filename].imageUrl = imageUrl;
        });
    } catch (error) {
        console.error(`Error writing media file ${path}:`, error);
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
        // let dirHandle = getSavedRootDirHandle();
        // alert(`Saved dir handle: ${dirHandle}.`);
        // console.log(dirHandle);
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
            console.error("Error while getting file handle for status check", path);
            return {
                status: 'error',
            }
        }

        const file = await memFile.handle.getFile();
        content = await file.text();
    } catch (error) {
        console.error('Error while getting status for file', path, error);
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
        'image/webp': 'webp'
    };
    return extensions[mimeType] || 'png';
}

// TODO can we reuse moveFile?
async function moveCurrentFile(toDir) {
    isMessingWithCurrentEditor = true;

    // TODO add prevent syncing?
    const oldPath = currentEditor.path;
    const newPath = joinPath('/', toDir, toFilename(currentEditor.path));

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
        // files[toDir][editor.currentFile] = {
        //     content: content,
        //     lastModified: 0,
        //     handle: await getFileHandle(newPath),
        // }
        currentEditor.path = newPath;
        setServerFile(newPath, content, 0);
        saveServerFiles();

        await remove(oldPath);
        await renderSidebar();
    } catch (error) {
        console.error('Error moving file:', error);
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
        setServerFile(newPath, content, 0);
        saveServerFiles();

        // Server file will be removed here.
        await remove(oldPath);
        // delete files[oldDir][oldFilename];
        await renderSidebar();

        log(`Moved ${oldPath} to ${newPath}`);
    } catch (error) {
        console.error('Error moving file:', error);
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
    if (currentEditor.path !== undefined && currentEditor.path !== path) {
        log('Began syncing previous file');
        await syncCurrentEditor(false);
        log('Finished syncing previous file');
    }

    // Lock the current editor during the operation, so we won't interrupt syncCurrentEditor in the middle.
    // By this time it is guaranteed to be free because we've just waited for "syncCurrentEditor".
    // We should do this before any awaits.
    isMessingWithCurrentEditor = true;

    if (path === INBOX_PATH) {
        openInbox();
        isMessingWithCurrentEditor = false;
        return;
    } else {
        const codemirror = document.querySelector('.CodeMirror-wrap');
        codemirror.style.display = 'block';
        inbox.style.display = 'none';
        chatInput.style.display = 'none';
        isInbox = false;
    }
    // chatButton.classList.remove('hidden');
    chatContainer.style.display = 'none';
    closeInboxModal();

    const start = performance.now();

    // Check if we're loading the same file and save cursor position
    let cursorPos = null;
    if (currentEditor.path === path) {
        log('saving cursor');
        cursorPos = editor.getCursor();
    }

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

    if (cursorPos !== null) {
        log('cursor not null');
        currentEditor.setCursor(cursorPos);
        currentEditor.scrollTo(null, 0);
        // const editorScrollHeight = currentEditor.getScrollInfo().clientHeight;
        // Only scroll if editor'sheight more than current screen height
        // const contentFitsTheScreen = editorScrollHeight <= window.innerHeight;
        // log('FITS', contentFitsTheScreen);
        // if (contentFitsTheScreen) {
        // let margin = 500;
        // currentEditor.scrollIntoView(cursorPos, margin);
        // }
        // TODO only focus if there's no quick dialogue
        currentEditor.focus();
    } else {
        focusLastLine();
    }

    const end = performance.now();
    log(`File ${path} opened in: ${(end - start).toFixed(3)} milliseconds, opId: ${id}`);

    // Once we spent enough time in file, set viewportMargin to infinity to prevent artefacts.
    // Artefacts can be observed during text selection (cmd+a).
    setTimeout(() => {
        currentEditor.setOption('viewportMargin', Infinity);
    }, 100);

    isMessingWithCurrentEditor = false;
}

// 0) Read content from local fs
// 1) Save current content to local filesystem
// 2) Sync it with the server
// TODO add hash of last read file comparison, merge on conflict (in which scenarious in can happen tho?)
// TODO It should be atomic.
// If currentEditor is changed during the execution of this function, we'll have RC.
// So, wherever we change currentEditor reference, we should lock via isSyncingCurrentEditor.
async function syncCurrentEditor(syncWithServer = true) {
    if (files === undefined || isWelcome || debug || currentEditor.path === undefined) {
        return;
    }

    // Skip sync if we're in in-memory mode.
    /// TODO detect welcome mode separately
    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
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

    if (path === INBOX_PATH) {
        // Try to load local changes.
        if (chatIsClean) {
            try {
                let inMemoryLastModified = getMemFile(path)?.lastModified;
                let file = await ((await getFileHandle(INBOX_PATH)).getFile());

                // Update last modified in memory.
                let memFile = getMemFile(path);
                if (memFile !== null) {
                    memFile.lastModified = file.lastModified;
                    addMemFile(path, memFile);
                }

                let localLastModified = file.lastModified;
                // TODO inmemory lastmodified should be reloaded
                if (inMemoryLastModified !== localLastModified) {
                    log(files);
                    isMessingWithCurrentEditor = false;
                    await openFile(INBOX_PATH);
                    return;
                }
            } catch (e) {
                logError('Error opening file:', e);
                isMessingWithCurrentEditor = false;
                return;
            }
        }

        isMessingWithCurrentEditor = false;

        if (syncWithServer) {
            try {
                await syncLocalFileWithServer(INBOX_PATH);
            } catch (error) {
                console.error('Error during sync with server:', error);
            }
        }

        return;
    }

    // Track in-editor renaming based on header.
    const filename = toFilename(path);
    try {
        // TODO track if no first line?
        const firstLine = currentEditor.getValue().split('\n')[0];
        let newFilename = ucfirst(fromHeaderToFilename(firstLine));
        // If filename is empty, generate an available "Untitled" name
        // TODO check for forbidden filename chars
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
            // Change the file immediately, because on further await calls it can be synced by syncTexts.
            currentEditor.path = joinPath(toDirPath(path), newFilename);

            // 1. Remove file with old filename
            // 2. Create file with new filename

            let content = getCurrentContent();
            // TODO every await means we can can have RC due to editor content change
            await remove(path);
            log('Removed due to filename change', path);

            const newPath = joinPath(toDirPath(path), newFilename);
            addMemFile(newPath, {
                isFile: true,
                content: content,
                lastModified: 0,
                path: newPath,
                handle: await getFileHandle(newPath, true),
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
        console.error('Error during filename change:', error);
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
        // const path = `${dir}/${filename}`;
        contentWasModifiedLocally = !await isContentEqual(path, content);
    } catch (error) {
        console.error('Error checking content equality:', error);
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
        log('WAS MODIFIED LOCALLY, and the editor is clean', path);

        // Changes only from local system
        try {
            isMessingWithCurrentEditor = false;
            await openFile(path, false);
        } catch (error) {
            console.error('Error opening file:', error);
            isMessingWithCurrentEditor = false;
            return;
        }
    } else if (!currentEditor.isClean()) {
        isSaving = true;
        try {
            // const file = files[dir][filename];
            log('Getting', path);
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
                    console.error(`Cannot save ${path}. No file handle found.`);
                }
            }
        } catch (error) {
            console.error('Error during save:', error);
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

    if (syncWithServer) {
        try {
            await syncLocalFileWithServer(path);
        } catch (error) {
            console.error('Error during sync with server:', error);
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

function getDirs() {
    if (files === undefined) {
        return [];
    }

    let dirs = Object.keys(files).filter(dir => !SYSTEM_DIRS.includes(dir));
    dirs.push('habits');
    // replace '' with /
    dirs = dirs.map(dir => dir === '' ? '/' : dir);

    return dirs;
}

// Returns json response or null on error.
async function post(endpoint, data) {
    try {
        let response = await fetch(`${API_HOST}/${endpoint}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': localStorage.getItem('token'),
                'Version': getCurrentVersion()
            },
            body: JSON.stringify(data)
        });
        if (!response.ok) {
            return null;
        }

        const json = await response.json();

        // Handle special commands from server;
        if (json.status === 'reload') {
            const url = new URL(window.location);
            url.searchParams.set('t', Date.now());
            window.location.href = url.toString();
        } else if (json.status === 'close') {
            window.location.href = "about:blank"
        }

        return json;
    } catch (error) {
        console.error('Network error occurred:', error.message);
        return null;
    }
}

// If there are files without isFile flag - we would have recursion.
// Because walk would try to iterate over js object keys.
// Return false from callback to stop walking.
function walk(obj, callback, path = '/') {
    // Chromium's callstack limit is 11K, so we iterate.
    const stack = [{ obj, path }];

    const maxAllowedIterations = 100000;
    let iterations = 0;
    while (stack.length > 0) {
        const { obj: currentObj, path: currentPath } = stack.pop();

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

        // Add directories to stack (in reverse order to maintain order)
        for (let i = dirs.length - 1; i >= 0; i--) {
            const key = dirs[i];
            const item = currentObj[key];
            const fullPath = currentPath + key;

            if (callback(fullPath, false) === false) {
                return;
            }
            stack.push({ obj: item, path: fullPath });
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

    const { filename } = toDirPathAndFilename(path);

    return filename;
}

// Dir with no slash at the end.
// For '/' it returns '/'.
function toDirPath(path) {
    const { dirPath } = toDirPathAndFilename(path);

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
    return { dirPath, filename };
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
        if (filePath === CONFIG_PATH || filePath === INBOX_PATH) {
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
    if (path === INBOX_PATH) {
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
        showRandomFile();
    }
}

window.addEventListener('beforeunload', function() {
    // clearInterval(window.loader);
    clearInterval(window.saver);
});


// Worker to process the saving queue
window.saver = setInterval(() => {
    if (document.hasFocus()) {
        syncCurrentEditor();
    }
}, CURRENT_FILE_SYNC_INTERVAL);
