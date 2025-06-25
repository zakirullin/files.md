// TODO Do sync not as often
const API_HOST = window.API_HOST || 'https://api.files.md';
// TODO that's quite often. Maybe on edit + focus?
const SAVE_INTERVAL = 1000; // ms, how often to save currently open file
const LOAD_INTERVAL = 3000; // ms, how often to load current file from local file system

let isSaving = false;
let isSyncing = false;
let isSyncingMedia = false;
let isSyncingCurrent = false;
let isLoadingLocalFiles = false;

// In-memory mapping of local file system:
// {
//   'dir': [
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
let files = []; // In-memory representation of files.
let serverFiles = {files: {}, media: {}, timestamps: {}, mediaTimestamp: 0};
const SERVER_STORAGE_KEY = 'files';
const SUPPORTED_EXTENSIONS = ['md', 'txt', 'png', 'jpg', 'jpeg', 'webp', 'gif',];
const SYSTEM_DIRS = ['media', 'archive', '_read_', '_watch_', '_shop_', 'today', 'later', 'journal', 'habits', 'triggers', 'places'];
const CONFIG_FILENAME = 'config.json';

// Returns files in flattened structure:
// {
//   'dir': {
//      ...
//   },
//   'dir/dir2': {
//      ...
//   },
// }
// The code is quite messy. We have to make lots of optimizations,
// otherwise it's going to be slow even with 5K files.
async function loadLocalFiles(rootDirHandle) {
    if (isLoadingLocalFiles) {
        return;
    }

    while (!editor.isClean()) {
        await new Promise(r => setTimeout(r, 50));
    }

    isLoadingLocalFiles = true;
    let newFiles = {};

    // Loads files recursively
    async function loadDir(dirHandle, path = '', depth = 1) {
        const entries = [];
        for await (const entry of dirHandle.values()) {
            entries.push(entry);
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));

        const dirPromises = [];
        for (const entry of entries) {
            const filename = entry.name.normalize('NFC');

            let isSupportedExtension = SUPPORTED_EXTENSIONS.includes(filename.split('.').pop());
            let isConfig = filename === CONFIG_FILENAME;
            if (entry.kind === 'directory') {
                if (filename.startsWith('.') || depth >= 5) continue;

                const dir = `${path}${filename}/`;
                newFiles[filename] = {};
                dirPromises.push({handle: entry, dir, depth: depth + 1});

            } else if (entry.kind === 'file' && (isSupportedExtension || isConfig)) {
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

    try {
        await loadDir(rootDirHandle);
    } catch (error) {
        console.log('Load Local files: ', error);
        isLoadingLocalFiles = false;
        throw error;
    }

    // Remove empty dirs
    // for (const dir in newFiles) {
    //     if (Object.keys(newFiles[dir]).length === 0) {
    //         delete newFiles[dir];
    //     }
    // }

    // Load metadata
    const savedMetadata = localStorage.getItem(SERVER_STORAGE_KEY);
    if (savedMetadata) {
        serverFiles = JSON.parse(savedMetadata);
    }

    isLoadingLocalFiles = false;

    return newFiles;
}

// TODO add support for config.json
async function syncTextsWithServer() {
    if (files === undefined) {
        return;
    }

    if (localStorage.getItem('token') === null) {
        return;
    }
    if (debug) {
        return;
    }

    if (isSyncing) return;
    isSyncing = true;

    const startTime = performance.now();
    console.log('Starting sync with server...');

    // Send locally modified files and timestamps of last seen dirs from the server
    let server = {};
    const {modified, deleted} = await collectModifiedAndDeletedFiles();
    try {
        let response = await fetch(`${API_HOST}/syncTexts`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
            body: JSON.stringify({
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
        console.error('Network error occurred:', error.message);
        isSyncing = false;
        return;
    }

    try {
        // Write files received from the server
        let failedAtLeastOnce = false;
        for (const fileInfo of server.files) {
            const {path, content, lastModified} = fileInfo;
            // If it is current file, skip, because we sync it separately
            // TODO if we skip current, don't take it's timestamp? We had a bug when sync was broken for 1 file
            // TODO fix missing / for root files
            if (path === `${editor.currentDir}/${editor.currentFile}` || path === editor.currentFile) {
                console.log('Skip current ' + path);
                continue;
            }

            try {
                await saveTextFile(path, content)

                // TODO get rid of this
                let dir, filename;
                if (path.includes('/')) {
                    const parts = path.split('/');
                    filename = parts.pop();
                    dir = parts.join('/');
                } else {
                    dir = '';
                    filename = path;
                }
                addFileToMemory(dir, filename, {
                    content: content,
                    lastModified: lastModified,
                    handle: await getFileHandle(path),
                });

                console.log('SYNC texsts: write file: ', path);
                setServerFile(path, content, lastModified);
                // Unfortunately rename is not working, so we have to delete the old file
                const shouldRemoveOldFile = path in server.renames;
                if (shouldRemoveOldFile) {
                    const oldPath = server.renames[path];
                    try {
                        await removeFile(oldPath);
                    } catch(err) {
                        console.log('RENAME: cant remove file: ', err, path);
                    }
                }
                saveServerFiles();
            } catch (error) {
                console.warn(`Error saving file ${path}:`, error);
                // Don't treat malformed filenames as sync error.
                console.log(error);
                if (!error.message.includes('Name is not allowed')) {
                    failedAtLeastOnce = true;
                }
            }
        }
        // Only move timestamp pointers when we were able to sync all the files.
        if (!failedAtLeastOnce) {
            console.log('BATCH sync ok, moving timestamps');
            serverFiles['timestamps'] = server.timestamps;
            saveServerFiles();
        } else {
            console.log('BATCH sync error, timestamps aren\'t moved');
        }
    } catch (error) {
        console.error('Can\'t sync: ', error.message)
    }

    console.log('Sync completed in ' + (performance.now() - startTime) + 'ms');

    isSyncing = false;
}

async function syncLocalFileWithServer(dir, filename) {
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
        let response = await fetch(`${API_HOST}/syncText`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
            body: JSON.stringify({
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
        if (json.status === 'notModified') {
            return;
        }
        if (json.status === 'updatedOnServer') {
            setServerFile(path, content, json.lastModified);
            console.log(`saved metadata for ${path} with timestamp ${json.lastModified}`, json);
            saveServerFiles();
            return;
        }
        // if status is "ok", it means it was merged?So we proceed
        serverFile = json
    } catch (error) {
        console.error('Network error occurred:', error.message);
        return;
    }
    setServerFile(path, serverFile.content, serverFile.lastModified);
    console.log(`saved metadata2 for ${path} with timestamp ${serverFile.lastModified}`);
    saveServerFiles();
    await saveTextFile(path, serverFile.content);
    console.log('showing file sync one');
    await openFile(dir, filename);
    console.log('File synced with server');
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

    const mediaTimestamp = serverFiles['mediaTimestamp'] || 0;
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
                        'Authorization': localStorage.getItem('token')
                    },
                    body: JSON.stringify({
                        filename: mediaFilename,
                        data: base64String,
                    })
                });

                if (!response.ok) {
                    console.error(`Failed to sync media file ${mediaFilename}:`, response.statusText);
                } else {
                    serverFiles['media'][mediaFilename] = {
                        lastModified: 0, // We don't track binary files modifications.
                    };
                    saveServerFiles();
                    console.log(`Successfully synced media file: ${mediaFilename}`);
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
                'Authorization': localStorage.getItem('token')
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
            const {filename, lastModified} = fileInfo;
            console.log(`Downloading media file: ${filename}`);

            try {
                // Fetch the binary file
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

        console.log(`Media sync completed in ${(performance.now() - startTime).toFixed(2)}ms. Downloaded ${filesProcessed} files.`);
    } catch (error) {
        console.error('Network error during media sync:', error.message);
    }

    isSyncingMedia = false;
}

// Saves media file and moves pointer
async function saveMediaFile(path, blob, lastModified) {
    const fileHandle = await getFileHandle(path, true);
    if (fileHandle === null) {
        console.log(`Malformed name for ${path}, skipping file...`);
        return;
    }

    // Check if file exists already
    try {
        const file = await fileHandle.getFile();
        const fileExists = file.size > 0;
        if (fileExists) {
            if (serverFiles['mediaTimestamp'] === undefined || lastModified > serverFiles['mediaTimestamp']) {
                serverFiles['mediaTimestamp'] = lastModified;
            }
            serverFiles['media'][file.name] = {
                lastModified: lastModified,
            }
            saveServerFiles();
            console.log(`File ${path} already exists and is up to date, skipping...`);
            return;
        }
    } catch (error) {
        console.log(`File ${path} doesn't exist or can't be read, will create it`);
    }

    try {
        const parts = path.split('/');
        let filename = parts.pop();

        const writable = await fileHandle.createWritable();
        await writable.write(blob);
        await writable.close();
        console.log(`Successfully wrote media file: ${path}`);
        if (lastModified > serverFiles['mediaTimestamp']) {
            serverFiles['mediaTimestamp'] = lastModified;
        }
        serverFiles['media'][filename] = {
            lastModified: lastModified,
        }
        saveServerFiles();

        // Load file handle into files
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
                console.log('Skip sending current file');
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

    // Find deleted files that are in server files but not in existing files.
    let deleted = [];
    for (const dir in serverFiles.files) {
        for (const file in serverFiles.files[dir]) {
            if (/[<>:'|?*\\/\x00-\x1F\x7F]/.test(file)) {
                continue;
            }
            if (editor.currentDir === dir && editor.currentFile === file) {
                continue;
            }
            if (!existingFiles[toPath(dir, file)]) {
                console.log('DELETED ' + toPath(dir, file));
                deleted.push(toPath(dir, file));
            }
        }
    }

    return {
        modified: modifiedFiles,
        deleted: deleted,
    };
}

async function collectNewMediaFiles() {
    if (!files['media']) {
        return {
            newMedia: [],
        };
    }

    const newMediaFiles = [];
    for (const filename in files['media']) {
        if (serverFiles['media'] === undefined || !(filename in serverFiles['media'])) {
            newMediaFiles.push(filename);
        }
    }

    console.log('NEW FILENAMES', newMediaFiles);

    return newMediaFiles;
}

function toPath(dir, file) {
    if (dir === '') {
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
        console.log('NEW FILE ' + dir + '/' + filename);
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

async function getFileHandle(path, create = false) {
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
                currentDirHandle = await currentDirHandle.getDirectoryHandle(dirName, {create: create});
            } catch (error) {
                throw error;
            }
        }
    }

    let fileHandle;
    try {
        fileHandle = await currentDirHandle.getFileHandle(filename, {create: create});
    } catch (error) {
        throw error;
    }

    return fileHandle;
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

        return false;
    } else {
        return true;
    }
}

// TODO save metadata & files
async function saveTextFile(path, content) {
    let fileHandle = await getFileHandle(path, true);
    if (fileHandle === null) {
        // TODO fix once Chromium fixes the bug
        throw new Error('Invalid file name');
    }

    const fileExists= !await exists([path]);
    if (fileExists || !await isContentEqual(path, content)) {
        // TODO what if we're syncing first time and already have changes?
        console.log('Hashes do not match, writing file...', path);
        const writable = await fileHandle.createWritable();
        await writable.write(content);
        await writable.close();
    } else {
        console.log('Hashes match, no need to write file.');
    }
}

async function saveImageFile(fileName, file) {
    try {
        const rootDirHandle = await getRootDirHandle();

        let mediaDirHandle;
        try {
            mediaDirHandle = await rootDirHandle.getDirectoryHandle('media', {create: true});
        } catch (error) {
            console.error('Error creating media directory:', error);
            throw new Error('Could not create media directory');
        }

        const fileHandle = await mediaDirHandle.getFileHandle(fileName, {create: true});
        const writable = await fileHandle.createWritable();
        await writable.write(file);
        await writable.close();

        return fileHandle;
    } catch (error) {
        console.error('Error in saveImageFile:', error);
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
        console.log('Malformed name, skipping file...');
        return;
    }
    await fileHandle.remove()
    console.log(`File ${path} removed successfully.`);
}

// TODO can we reuse moveFile?
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
        console.log('MOVING to DIR:', toDir);
        files[toDir][editor.currentFile] = {
            content: content,
            lastModified: 0,
            handle: await getFileHandle(newPath),
        }
        editor.currentDir = toDir;
        setServerFile(newPath, content, 0);
        saveServerFiles();

        await removeFile(oldPath);
        await updateSidebar();
    } catch (error) {
        console.error('Error moving file:', error);
    }

    isSyncingCurrent = false;
}

// TODO lock on files modification?
function addFileToMemory(dir, filename, fileData) {
    // Ensure directory exists
    if (!files[dir]) {
        files[dir] = {};
    }

    files[dir][filename] = fileData;

    // Only sort the specific directory that was modified
    const sortedFiles = {};
    const sortedKeys = Object.keys(files[dir]).sort((a, b) => a.localeCompare(b));
    for (const key of sortedKeys) {
        sortedFiles[key] = files[dir][key];
    }
    files[dir] = sortedFiles;

    // Remove the global re-sorting - it's messing up the natural order
    // The directory order should stay as established by loadLocalFiles
}

async function moveFile(oldPath, newPath) {
    if (oldPath === newPath) {
        return;
    }
    
    const oldParts = oldPath.split('/');
    const oldFilename = oldParts.pop();
    const oldDir = oldParts.join('/');

    const newParts = newPath.split('/');
    const newFilename = newParts.pop();
    const newDir = newParts.join('/');

    try {
        let file = await (await getFileHandle(oldPath)).getFile();
        let content = await file.text();
        await saveTextFile(newPath, content);

        console.log('saving ' + newDir + '/' + newFilename);
        addFileToMemory(newDir, newFilename,  {
            content: content,
            lastModified: 0,
            handle: await getFileHandle(newPath),
        });
        setServerFile(newPath, content, 0);
        saveServerFiles();

        // Server file will be removed here.
        await removeFile(oldPath);
        delete files[oldDir][oldFilename];
        await updateSidebar();

        console.log(`Moved ${oldPath} to ${newPath}`);
    } catch (error) {
        console.error('Error moving file:', error);
    }
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

function setServerFile(path, content, lastModified) {
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

function saveServerFiles() {
    localStorage.setItem(SERVER_STORAGE_KEY, JSON.stringify(serverFiles));
}

// 0) Read content from local fs
// 1) Save current content to local filesystem
// 2) Sync it with the server
// TODO add hash of last read file comparison, merge on conflict (in which scenarious in can happen tho?)
// TODO add lock / RC for currentFile change
async function syncCurrentFile(syncWithServer = true) {
    if (files === undefined) {
        return;
    }
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

    // Track in-editor renaming.
    try {
        // TODO track if no first line?
        const firstLine = editor.getValue().split('\n')[0];
        const hasFilenameChanged = firstLine.toLowerCase() !== toHeader(editor.currentFile).toLowerCase();
        if (hasFilenameChanged) {
            const newFilename = ucfirst(fromHeader(firstLine));
            await removeFile(`${editor.currentDir}/${editor.currentFile}`);
            delete files[editor.currentDir][editor.currentFile];
            console.log('Removed', `${editor.currentDir}/${editor.currentFile}`);
            // TODO Way to verbose, to we want to mess with it like this?
            addFileToMemory(editor.currentDir, newFilename, {
                content: getCurrentContent(),
                lastModified: 0,
                handle: await getFileHandle(toPath(editor.currentDir, newFilename), true),
            });
            editor.currentFile = newFilename;

            const path = `${editor.currentDir}/${editor.currentFile}`;
            const content = getCurrentContent();
            await saveTextFile(path, content);
            setServerFile(path, content, 0);
            saveServerFiles();
            console.log('Created', `${editor.currentDir}/${editor.currentFile}`);
            await updateSidebar();
        }
    } catch (error) {
        console.error('Error during filename change:', error);
        isSyncingCurrent = false;
        return;
    }

    let contentWasModifiedLocally = false;
    try {
        const path = `${editor.currentDir}/${editor.currentFile}`;
        contentWasModifiedLocally = !await isContentEqual(path, getCurrentContent());
    } catch (error) {
        console.error('Error checking content equality:', error);
        isSyncingCurrent = false;
        return;
    }

    // TODO better way would be this:
    // Read file from fs with it's timestamp
    // If in our memory we have actual TS, just write file back
    // If fs has fresher change, merge.
    // Sync with server.

    if (contentWasModifiedLocally && editor.isClean()) {
        console.log('WAS MODIFIED LOCALLY', editor.currentFile);
        // Changes only from local system
        try {
            await openFile(editor.currentDir, editor.currentFile);
        } catch (error) {
            console.error('Error opening file:', error);
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
            // TODO We might switch to another current file.

            console.error('Error during save:', error);
            isSaving = false;
            // Revert doc back to dirty state
            editor.replaceRange(' ', editor.getCursor());
            isSyncingCurrent = false;
            editor.undo();
            return;
        }
        isSaving = false;
    }

    if (syncWithServer) {
        try {
            await syncLocalFileWithServer(editor.currentDir, editor.currentFile);
        } catch (error) {
            console.error('Error during sync with server:', error);
        }
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

window.addEventListener('beforeunload', function () {
    // clearInterval(window.loader);
    clearInterval(window.saver);
});


// Worker to process the saving queue
window.saver = setInterval(syncCurrentFile, SAVE_INTERVAL);