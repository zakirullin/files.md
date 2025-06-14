// TODO Do sync not as often
const saverInterval = 1000; // ms, how often to save currently open file
const loaderInterval = 3000; // ms, how often to load current file from local file system

let isSaving = false;
let isSyncing = false;
let isSyncingCurrent = false;

// In-memory mapping of local file system:
// {
//   "dir": [
//     {
//       "filename": [
//         {
//           content: "File content here...",
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
let files = [];
let serverFiles = {files: {}, timestamps: {}, mediaTimestamp: 0};
const SERVER_FILES_STORAGE_KEY = 'files';
const supportedFileTypes = ['md', 'txt', 'png', 'jpg', 'jpeg', 'webp', 'gif',];
const systemDirs = ["media", "img", "archive", "_read_", "_watch_", "_shop_", "today", "later", "journal", "habits", "triggers", "places"];

// Returns files in flattened structure:
// {
//   "dir": {
//      ...
//   },
//   "dir/dir2": {
//      ...
//   },
// }
// The code is quite messy. We have to make lots of optimizations,
// otherwise it's going to be slow even with 5K files.
async function loadLocalFiles(rootDirHandle) {
    while (!editor.isClean()) {
        await new Promise(r => setTimeout(r, 50));
    }

    let newFiles = {};

    // Loads files recursively
    async function loadDir(dirHandle, path = "", depth = 1) {
        const entries = [];
        for await (const entry of dirHandle.values()) {
            entries.push(entry);
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));

        const dirPromises = [];
        for (const entry of entries) {
            const filename = entry.name.normalize("NFC");

            if (entry.kind === 'directory') {
                if (filename.startsWith('.') || depth >= 5) continue;

                const dir = `${path}${filename}/`;
                newFiles[filename] = {};
                dirPromises.push({handle: entry, dir, depth: depth + 1});
            } else if (entry.kind === 'file' && supportedFileTypes.includes(filename.split('.').pop())) {
                const dir = path.replace(/\/+$/, '');
                if (!newFiles[dir]) newFiles[dir] = {};

                // Reuse existing file handle if it exists
                if (files?.[dir]?.[filename] !== undefined) {
                    newFiles[dir][filename] = files[dir][filename];
                    continue;
                }
                newFiles[dir][filename] = {handle: entry};

                entry.getFile().then(file => {
                    newFiles[dir][filename].lastModified = file.lastModified;
                });

                if (dir === 'media') {
                    getImageUrl(entry).then(imageUrl => {
                        newFiles[dir][filename].imageUrl = imageUrl;
                    });
                }
            }
        }

        if (debug) {
            if (!debug.loaded) {
                debug.loaded = true
                await loadDir(rootDirHandle, debug.dir, 1);
            }
            return;
        }

        await Promise.all(dirPromises.map(({handle, dir, depth}) =>
            loadDir(handle, dir, depth)
        ));
    }

    await loadDir(rootDirHandle);

    // Remove empty dirs
    for (const dir in newFiles) {
        if (Object.keys(newFiles[dir]).length === 0) {
            delete newFiles[dir];
        }
    }

    // Load metadata
    const savedMetadata = localStorage.getItem(SERVER_FILES_STORAGE_KEY);
    if (savedMetadata) {
        serverFiles = JSON.parse(savedMetadata);
    }

    return newFiles;
}

// TODO add support for config.json
async function syncTextsWithServer() {
    if (localStorage.getItem('token') === null) {
        return;
    }
    if (debug) {
        return;
    }

    if (isSyncing) return;
    isSyncing = true;

    const startTime = performance.now();
    console.log("Starting sync with server...");

    // Send locally modified files and timestamps of last seen dirs from the server
    let server = {};
    const {modified, deleted} = await collectModifiedAndDeletedFiles();
    try {
        let response = await fetch('https://api.files.md/syncTexts', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
            body: JSON.stringify({
                userId: getUserId(),
                modified: modified,
                deleted: deleted,
                timestamps: serverFiles['timestamps'] || [],
            })
        });
        if (!response.ok) {
            console.log(`Server responded with ${response.status}`);
            return;
        }

        // Remove info about server files on client
        for (const path of deleted) {
            removeInfoAboutServerFile(path);
        }

        server = await response.json();
    } catch (error) {
        console.error("Network error occurred:", error.message);
        isSyncing = false;
        return;
    }

    try {
        // Write files received from the server
        for (const fileInfo of server.files) {
            const {path, content, lastModified} = fileInfo;
            // If it is current file, skip, because we sync it separately
            // TODO if we skip current, don't take it's timestamp? We had a bug when sync was broken for 1 file
            // TODO fix missing / for root files
            if (path === `${editor.currentDir}/${editor.currentFile}` || path === editor.currentFile) {
                console.log("Skip current " + path);
                continue;
            }

            try {
                await saveTextFile(path, content)
                setMetadata(path, content, lastModified);
                // Unfortunately rename is not working, so we have to delete the old file
                const shouldRemoveOldFile = path in server.renames;
                if (shouldRemoveOldFile) {
                    const oldPath = server.renames[path];
                    await removeFile(oldPath);
                }
                saveMetadata();
            } catch (error) {
                console.error(`Error saving file ${path}:`, error);
            }
        }
        serverFiles['timestamps'] = server.timestamps;
        saveMetadata();
    } catch (error) {
        console.error("Can't sync: ", error.message)
    }

    console.log("Sync completed in " + (performance.now() - startTime) + "ms");

    isSyncing = false;
}

async function syncFileWithServer(dir, filename) {
    if (localStorage.getItem('token') === null) {
        return;
    }

    const path = toPath(dir, filename);
    let file = await (await getFileHandle(path)).getFile();
    // TODO we might only need to send content when modifying
    let content = await file.text();
    let serverTimestamp = getMetadata(path)?.lastModified || 0;

    let serverFile = {};
    try {
        let response = await fetch('https://api.files.md/syncText', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
            body: JSON.stringify({
                userId: getUserId(),
                path: toPath(dir, filename),
                lastModified: serverTimestamp,
                content: content,
            })
        });
        if (!response.ok) {
            console.log(`Server responded with ${response.status}`);
            return;
        }
        let json = await response.json();
        if (["notModified", "updatedOnServer"].includes(json.status)) {
            setMetadata(path, content, json.lastModified);
            console.log(`saved metadata for ${path} with timestamp ${json.lastModified}`);
            saveMetadata();
            return;
        }
        serverFile = json
    } catch (error) {
        console.error("Network error occurred:", error.message);
        return;
    }
    setMetadata(path, serverFile.content, serverFile.lastModified);
    console.log(`saved metadata2 for ${path} with timestamp ${serverFile.lastModified}`);
    saveMetadata();
    console.log(serverFile);
    await saveTextFile(path, serverFile.content);
    console.log('showing file sync one');
    await openFile(dir, filename);
    console.log("File synced with server");
}

async function syncMediaFilesWithServer() {
    if (localStorage.getItem('token') === null) {
        return;
    }
    if (debug) {
        return;
    }

    // TODO skip if already syncing

    console.log(`Starting media sync from img folder...`);
    const startTime = performance.now();

    const mediaTimestamp = serverFiles['mediaTimestamp'] || 0;
    try {
        const response = await fetch('https://api.files.md/syncMedias', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': localStorage.getItem('token')
            },
            body: JSON.stringify({
                userId: getUserId(),
                folder: 'media',
                timestamp: mediaTimestamp
            })
        });

        if (!response.ok) {
            console.error(`Server responded with ${response.status}`);
        }

        const serverData = await response.json();

        // Process and save media files
        let filesProcessed = 0;
        for (const fileInfo of serverData.files) {
            const {path, lastModified} = fileInfo;
            console.log(`Downloading media file: ${path}`);

            try {
                // Fetch the binary file
                const response = await fetch('https://api.files.md/syncMedia', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': localStorage.getItem('token')
                    },
                    body: JSON.stringify({
                        userId: getUserId(),
                        path: path,
                        timestamp: mediaTimestamp
                    })
                });
                if (!response.ok) {
                    console.error(`Failed to download ${path}: ${response.status}`);
                    continue;
                }

                const blob = await response.blob();
                console.log(path, blob);
                await saveMediaFile(`media/${path}`, blob, lastModified);
                filesProcessed++;
            } catch (error) {
                console.error(`Error processing media file ${path}:`, error);
            }
        }

        console.log(`Media sync completed in ${(performance.now() - startTime).toFixed(2)}ms. Downloaded ${filesProcessed} files.`);
    } catch (error) {
        console.error("Network error during media sync:", error.message);
    }
}

async function saveMediaFile(path, blob, lastModified) {
    const fileHandle = await getFileHandle(path);
    if (fileHandle === null) {
        console.log(`Malformed name for ${path}, skipping file...`);
        return;
    }

    try {
        const file = await fileHandle.getFile();
        const fileExists = file.size > 0;
        if (fileExists) {
            // Is it ok that we save metadata here as well?
            if (serverFiles['mediaTimestamp'] === undefined || lastModified > serverFiles['mediaTimestamp']) {
                serverFiles['mediaTimestamp'] = lastModified;
                saveMetadata();
            }
            console.log(`File ${path} already exists and is up to date, skipping...`);
            return;
        }
    } catch (error) {
        console.log(`File ${path} doesn't exist or can't be read, will create it`);
    }

    try {
        const writable = await fileHandle.createWritable();
        await writable.write(blob);
        await writable.close();
        console.log(`Successfully wrote media file: ${path}`);
        // TODO we assume that we got no fails. Instead save filenames hashes, same for text
        if (lastModified > serverFiles['mediaTimestamp']) {
            serverFiles['mediaTimestamp'] = lastModified;
            saveMetadata();
        }

        // Load file handle into files
        // TODO split path by dir, filename? Not to have this logic
        const parts = path.split('/');
        let filename = parts.pop();
        files['media'][filename] = {handle: fileHandle};
        fileHandle.getFile().then(file => {
            files['media'][filename].lastModified = file.lastModified;
        });
        getImageUrl(fileHandle).then(imageUrl => {
            files['media'][filename].imageUrl = imageUrl;
        });
    } catch (error) {
        console.error(`Error writing media file ${path}:`, error);
        throw error;
    }
}

async function collectModifiedAndDeletedFiles() {
    const modifiedFiles = [];
    const existingFiles = {};
    const promises = [];
    for (const dir in files) {
        if (dir === 'media') continue; // Skip image directory

        for (const filename in files[dir]) {
            if (dir === editor.currentDir && filename === editor.currentFile) {
                console.log("Skip sending current file");
                continue;
            }

            const promise = getFileStatus(dir, filename)
                .then(result => {
                    if (result.status === 'modified' || result.status === 'new') {
                        modifiedFiles.push(result);
                    }

                    if (result.status !== 'error') {
                        existingFiles[result.path] = true;
                    }
                });
            promises.push(promise);
        }
    }

    await Promise.all(promises);

    // Find deleted files that are in files metadata but not in existing files
    let deleted = [];
    for (const dir in serverFiles.files) {
        for (const file in serverFiles.files[dir]) {
            if (/[<>:"|?*\\/\x00-\x1F\x7F]/.test(file)) {
                continue;
            }
            if (editor.currentDir === dir && editor.currentFile === file) {
                continue;
            }
            if (!existingFiles[toPath(dir, file)]) {
                console.log("DELETED " + toPath(dir, file));
                deleted.push(toPath(dir, file));
            }
        }
    }

    return {
        modified: modifiedFiles,
        deleted: deleted,
    };
}

function toPath(dir, file) {
    if (dir === "") {
        return file;
    }

    return `${dir}/${file}`;
}

async function getFileStatus(dir, filename) {
    let content;
    try {
        const fileData = files[dir][filename];
        if (!fileData?.handle) {
            return {
                status: 'error',
            }
        }

        const file = await fileData.handle.getFile();
        content = await file.text();
    } catch (error) {
        console.error(`Error processing ${dir}/${filename}:`, error);
        return {
            status: 'error',
        }
    }

    // TODO why path is stored at all?
    const path = serverFiles?.files?.[dir]?.[filename]?.path;
    if (!path) {
        console.log("NEW FILE " + dir + "/" + filename);
        return {
            status: 'new',
            content: content,
            path: toPath(dir, filename), // WHY?
            lastModified: 0 // new file
        }
    }

    const serverHash = serverFiles?.files?.[dir]?.[filename]?.hash;
    const serverTime = serverFiles?.files?.[dir]?.[filename]?.lastModified;
    if (serverHash !== hash(content)) {
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

async function getFileHandle(path) {
    let dir, filename;
    if (path.includes('/')) {
        const parts = path.split('/');
        filename = parts.pop();
        dir = parts.join('/');
    } else {
        dir = '';
        filename = path;
    }

    const dirs = dir.split('/');
    let currentDirHandle = await getRootDirHandle();
    for (const dirName of dirs) {
        if (dirName) {
            try {
                currentDirHandle = await currentDirHandle.getDirectoryHandle(dirName, {create: true});
            } catch (error) {
                console.error(`Error getting directory handle for '${dirName}':`, error);
                return null;
            }
        }
    }

    let fileHandle;
    try {
        fileHandle = await currentDirHandle.getFileHandle(filename, {create: true});
    } catch (error) {
        console.error(`Error getting file handle for '${dir}/${filename}':`, error);
        return null;
    }

    return fileHandle;
}

// TODO split into two, sometimes we need just compare
async function isContentEqual(path, content) {
    let fileHandle = await getFileHandle(path);
    if (fileHandle === null) {
        // TODO fix once Chromium fixes the bug
        console.warn("Malformed name, skipping file...");
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
                diff.push(`Line ${i + 1}: "${clientLine}" vs "${serverLine}"`);
            }
        }

        return false;
    } else {
        return true;
    }
}

// TODO save metadata & files
async function saveTextFile(path, content) {
    let fileHandle = await getFileHandle(path);
    if (fileHandle === null) {
        // TODO fix once Chromium fixes the bug
        throw new Error("Invalid file name");
    }

    if (!await isContentEqual(path, content)) {
        console.log("Hashes do not match, writing file...", path);
        // TODO rem
        const writable = await fileHandle.createWritable();
        await writable.write(content);
        await writable.close();
    } else {
        console.log("Hashes match, no need to write file.");
    }
}

async function saveImageFile(fileName, file) {
    try {
        const rootDirHandle = await getRootDirHandle();

        let mediaDirHandle;
        try {
            mediaDirHandle = await rootDirHandle.getDirectoryHandle('media', { create: true });
        } catch (error) {
            console.error("Error creating media directory:", error);
            throw new Error("Could not create media directory");
        }

        const fileHandle = await mediaDirHandle.getFileHandle(fileName, { create: true });
        const writable = await fileHandle.createWritable();
        await writable.write(file);
        await writable.close();

        return fileHandle;
    } catch (error) {
        console.error("Error in saveImageFile:", error);
        throw error;
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


// TODO del from memory?
async function removeFile(path) {
    let fileHandle = await getFileHandle(path);
    if (fileHandle === null) {
        // TODO fix once Chromium fixes the bug
        console.log("Malformed name, skipping file...");
        return;
    }
    await fileHandle.remove()
    console.log(`File ${path} removed successfully.`);
}

async function moveCurrentFile(toDir) {
    isSyncingCurrent = true;

    // TODO add prevent syncing?
    const oldPath = toPath(editor.currentDir, editor.currentFile);
    const newPath = toPath(toDir, editor.currentFile);

    try {
        let content = getCurrentContent();
        await saveTextFile(newPath, content);
        // TODO move to saveTextFile?
        delete files[editor.currentDir][editor.currentFile];
        files[toDir][editor.currentFile] = {
            content: content,
            lastModified: 0,
            handle: await getFileHandle(newPath),
        }
        editor.currentDir = toDir;
        setMetadata(newPath, content, 0);
        saveMetadata();

        await removeFile(oldPath);
        await buildSidebar();
    } catch (error) {
        console.error("Error moving file:", error);
    }

    isSyncingCurrent = false;
}

function getMetadata(path) {
    const parts = path.split('/');
    const filename = parts.pop();
    const dir = parts.join('/');

    if (serverFiles['files']?.[dir]?.[filename]) {
        return serverFiles['files'][dir][filename];
    } else {
        return null;
    }
}

function setMetadata(path, content, lastModified) {
    const parts = path.split('/');
    const filename = parts.pop();
    const dir = parts.join('/');

    serverFiles['files'] = serverFiles['files'] ?? {};
    serverFiles['files'][dir] = serverFiles['files'][dir] ?? {};
    serverFiles['files'][dir][filename] = {
        hash: hash(content),
        lastModified: lastModified,
        path: path
    };
}


function removeInfoAboutServerFile(path) {
    console.log('removing info about server file', path);
    const parts = path.split('/');
    const filename = parts.pop();
    const dir = parts.join('/');

    if (serverFiles.files?.[dir]?.[filename]) {
        delete serverFiles.files[dir][filename];
    }
}

function saveMetadata() {
    localStorage.setItem(SERVER_FILES_STORAGE_KEY, JSON.stringify(serverFiles));
}

function getUserId() {
    return parseInt(localStorage.getItem('userId'));
}

// 0) Read content from local fs
// 1) Save current content to local filesystem
// 2) Sync it with the server
// TODO add hash of last read file comparison, merge on conflict (in which scenarious in can happen tho?)
async function syncCurrentFile() {
    if (debug) {
        return;
    }

    if (editor.currentFile === undefined) {
        return
    }

    /// TODO detect welcome mode separately
    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
        return;
    }

    // Wait until not saving
    // TODO what if lots of saving calls are stuck?
    // I decided to go from loop to insta return
    if (isSaving) {
        return;
    }

    if (isSyncingCurrent) {
        return;
    }
    isSyncingCurrent = true;

    try {
        // Track if filename was changed
        // TODO track if no first line?
        const firstLine = editor.getValue().split('\n')[0];
        if (firstLine !== toHeader(editor.currentFile)) {
            console.log(firstLine, toHeader(editor.currentFile));
            const newFilename = fromHeader(firstLine);
            await removeFile(`${editor.currentDir}/${editor.currentFile}`);
            delete files[editor.currentDir][editor.currentFile];
            console.log('Removed', `${editor.currentDir}/${editor.currentFile}`);
            // TODO Way to verbose, to we want to mess with it like this?
            files[editor.currentDir][newFilename] = {
                content: getCurrentContent(),
                lastModified: 0,
                handle: await getFileHandle(toPath(editor.currentDir, newFilename)),
            }
            editor.currentFile = newFilename;

            const path = `${editor.currentDir}/${editor.currentFile}`;
            const content = getCurrentContent();
            await saveTextFile(path, content);
            setMetadata(path, content, 0);
            saveMetadata();
            console.log('Created', `${editor.currentDir}/${editor.currentFile}`);
            await buildSidebar();
        }
    } catch (error) {
        console.error("Error during filename change:", error);
        isSyncingCurrent = false;
        return;
    }

    let contentWasModifiedLocally = false;
    try {
        const path = `${editor.currentDir}/${editor.currentFile}`;
        contentWasModifiedLocally = !await isContentEqual(path, getCurrentContent());
    } catch (error) {
        console.error("Error checking content equality:", error);
        isSyncingCurrent = false;
        return;
    }

    // TODO better way would be this:
    // Read file from fs with it's timestamp
    // If in our memory we have actual TS, just write file back
    // If fs has fresher change, merge.
    // Sync with server.

    if (contentWasModifiedLocally && editor.isClean()) {
        console.log("WAS MODIFIED LOCALLY");
        // Changes only from local system
        try {
            await openFile(editor.currentDir, editor.currentFile);
        } catch (error) {
            console.error("Error opening file:", error);
            isSyncingCurrent = false;
            return;
        }
    } else if (!editor.isClean()) {
        isSaving = true;
        try {
            const dir = editor.currentDir;
            const filename = editor.currentFile;
            const fileData = files[dir][filename];
            if (fileData && fileData.handle) {
                let content = getCurrentContent();
                if (!editor.isClean() && contentWasModifiedLocally) {
                    // Changes from both sides: editor and local fs, need merging

                }
                // We need to atomically reset the flag once we captured a snapshot of particular version of the content.
                // This flag can be changed in the event loop, as a result of user making changes to the text in the middle
                // of our saving process. The new unsaved changes would be then handled by a subsequent saveCurrentFile() call.
                // Initially, this flag assignment was erroneously placed at the end of the function, resulting in a race condition.
                // If we override flag in the end, we would lose any changes that occurred during the 3 await calls.
                editor.markClean();

                const writable = await fileData.handle.createWritable();
                await writable.write(content);
                // Buffer is flushed on disk at this moment. It could be interrupted
                // by the event loop, so we use isSaving guard.
                await writable.close();
            } else {
                if (fileData.handle) {
                    alert(`Cannot save ${filename}. No file handle found.`);
                }
            }
        } catch (error) {
            console.error("Error during save:", error);
            isSaving = false;
            // Revert doc back to dirty state
            editor.replaceRange(' ', editor.getCursor());
            isSyncingCurrent = false;
            editor.undo();
            return;
        }
        isSaving = false;
    }

    try {
        await syncFileWithServer(editor.currentDir, editor.currentFile);
    } catch (error) {
        console.error("Error during sync with server:", error);
    }
    isSyncingCurrent = false;
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

async function initFiles() {
    const rootDirHandle = await getRootDirHandle();

    const startTime = performance.now();
    files = await loadLocalFiles(rootDirHandle);
    console.log(`Files loaded in ${performance.now() - startTime}ms`);
    await syncTextsWithServer();
    await syncMediaFilesWithServer();
}

window.addEventListener('beforeunload', function () {
    // clearInterval(window.loader);
    clearInterval(window.saver);
});


// Worker to process the saving queue
window.saver = setInterval(syncCurrentFile, saverInterval);