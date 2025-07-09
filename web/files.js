// TODO Do sync not as often
// TODO migrate to unversal file id = filepath, instead of two components
const API_HOST = window.API_HOST || 'https://api.files.md';
// TODO that's quite often. Maybe on edit + focus?
const CURRENT_FILE_SYNC_INTERVAL = 1000; // ms, how often to save currently open file
const LOAD_INTERVAL = 3000; // ms, how often to load current file from local file system

let isSaving = false;
let isSyncing = false;
let isSyncingMedia = false;
let isSyncingCurrentFile = false;
let isLoadingLocalFiles = false;

// The types of files we have:
// - serverFile, file on server
// - localFile, file on local fs
// - memFile, in-memory representation of local file
// The latter is needed for quick access to file's handle and metadata.

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
// TODO multidir rename to memFiles
let files = {}; // In-memory representation of files
let serverFiles = {files: {}, media: {}, timestamps: {}, mediaTimestamp: 0};
const SERVER_STORAGE_KEY = 'files';
const SUPPORTED_EXTENSIONS = ['md', 'txt', 'png', 'jpg', 'jpeg', 'webp', 'gif',];
const SYSTEM_DIRS = ['media', 'archive', '_read_', '_watch_', '_shop_', 'today', 'later', 'journal', 'habits', 'triggers', 'places', 'insights'];
const CONFIG_PATH = '/config.json';

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
    async function loadDir(dirHandle, path = '/', depth = 3) {
        const entries = [];
        for await (const entry of dirHandle.values()) {
            entries.push(entry);
        }
        entries.sort((a, b) => a.name.localeCompare(b.name));

        const dirPromises = [];
        for (const entry of entries) {
            const filename = entry.name.normalize('NFC');

            let isSupportedExtension = SUPPORTED_EXTENSIONS.includes(filename.split('.').pop());
            let isConfig = filename === CONFIG_PATH;
            if (entry.kind === 'directory') {
                if (filename.startsWith('.') || depth >= 5) continue;

                const dir = `${path}${filename}/`;
                newFiles[filename + '/'] = {};
                dirPromises.push({handle: entry, dir, depth: depth + 1});
            } else if (entry.kind === 'file' && (isSupportedExtension || isConfig)) {
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

                currentDir[filename] = {path: `${path}${filename}`, isFile: true, handle: entry};
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

    // Load server files
    const savedServerFiles = localStorage.getItem(SERVER_STORAGE_KEY);
    if (savedServerFiles) {
        serverFiles = JSON.parse(savedServerFiles);
    }

    isLoadingLocalFiles = false;
    console.log(newFiles);

    return newFiles;
}

// TODO add support for config.json
async function syncTextsWithServer() {
    if (files === undefined || Object.keys(files).length === 0) {
        return;
    }
    // TODO multidir rem
    // if (localStorage.getItem('token') === null) {
    //     return;
    // }
    if (debug) {
        return;
    }

    if (isSyncing) return;
    isSyncing = true;

    const startTime = performance.now();
    console.log('Starting sync with server...');

    // Send locally modified files and timestamps of last seen dirs from the server
    const {modified, deleted} = await collectModifiedAndDeletedFiles();
    const server = await post('syncTexts', {
        modified: modified,
        deleted: deleted,
        timestamps: serverFiles['timestamps'] || [],
    });
    if (server === null) {
        isSyncing = false;
        return;
    }

    // Remove info about server files on client
    for (const path of deleted) {
        removeServerFile(path);
    }

    try {
        // Write files received from the server
        let failedAtLeastOnce = false;
        for (const fileInfo of server.files) {
            const {path, content, lastModified} = fileInfo;
            // If it is current file, skip, because we sync it separately
            // TODO if we skip current, don't take it's timestamp? We had a bug when sync was broken for 1 file
            // TODO fix missing / for root files
            // TODO multidir add /path to server
            if (path === editor.path || path === editor2.path) {
                console.log('Skip current received from server' + path);
                continue;
            }

            try {
                await saveTextFile(path, content)
                addMemFile(path, {
                    isFile: true,
                    content: content,
                    lastModified: lastModified,
                    path: path,
                    handle: await getFileHandle(path),
                });

                console.log('SYNC texsts: write file: ', path);
                addServerFile(path, content, lastModified);
                // Unfortunately rename is not working, so we have to delete the old file
                const shouldRemoveOldFile = path in server.renames;
                if (shouldRemoveOldFile) {
                    const oldPath = server.renames[path];
                    try {
                        await removeFile(oldPath);
                    } catch (err) {
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

// TODO multidir all callers
async function syncLocalFileWithServer(path) {
    return;
    // TODO multidir
    if (localStorage.getItem('token') === null) {
        return;
    }

    let file = await (await getFileHandle(path)).getFile();
    // TODO we might only need to send content when modifying
    let content = await file.text();
    let serverTimestamp = getServerFile(path)?.lastModified || 0;

    let serverFile = {};
    try {
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
                clientLastModified: file.lastModified,
                // TODO multidir
                clientLastSynced: getServerFile(path)?.lastSynced || 0,
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
            // TODO maybe RC here? When file was updated, but during this time we already changed it
            addServerFile(path, content, json.lastModified, getServerFile(path)?.lastSynced);
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

    const lastSynced = await saveTextFile(path, serverFile.content);
    addServerFile(path, serverFile.content, serverFile.lastModified, lastSynced);
    console.log(`Saved server file for ${path} with timestamp ${serverFile.lastModified}`);
    saveServerFiles();
    console.log('Opening file after sync');
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
                        'Authorization': localStorage.getItem('token'),
                        'Version': getCurrentVersion()
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

// TODO rename textFiles?
async function collectModifiedAndDeletedFiles() {
    const modifiedFiles = [];
    const existingFiles = {};
    const promises = [];
    // TODO multidir walk
    // for (const dir in files) {
    //     if (dir === 'media') continue; // Skip image directory
    //
    //     // TODO multidir walk
    //     for (const filename in files[dir]) {
    //         // TODO write tests for that?
    //         if ((dir === editor.currentDir && filename === editor.currentFile)
    //             || (dir === editor2.currentDir && filename === editor2.currentFile)) {
    //             console.log('Skip sending current file: ' + dir + '/' + filename);
    //             continue;
    //         }
    //
    //         // TODO multidir path
    //         const promise = getFileStatus(path)
    //             .then(result => {
    //                 if (result.status === 'modified' || result.status === 'new') {
    //                     modifiedFiles.push(result);
    //                 }
    //
    //                 if (result.status !== 'error') {
    //                     existingFiles[result.path] = true;
    //                 }
    //             });
    //         promises.push(promise);
    //     }
    // }
    //
    walk(files, (path, entry, isFile) => {
        if (!isFile) {
            return;
        }

        if (path.startsWith('/media/')) {
            return;
        }

        // TODO write tests for that?
        if (path === editor.path || path === editor2.path) {
            console.log('Skip sending current file: ' + path);
            return;
        }

        console.log('walked to', path);
        const promise = getFileStatus(path)
            .then(result => {
                if (result.status === 'modified' || result.status === 'new') {
                    modifiedFiles.push(result);
                }

                if (result.status !== 'error') {
                    existingFiles[result.path] = true;
                }
            });
        promises.push(promise);
    });

    await Promise.all(promises);

    // Find deleted files that are in server files but not in existing files.
    let deleted = [];
    // TODO multidir walk
    for (const dir in serverFiles.files) {
        for (const filename in serverFiles.files[dir]) {
            if (/[<>:'|?*\\/\x00-\x1F\x7F]/.test(filename)) {
                continue;
            }
            // Skip current files.
            if ((dir === editor.currentDir && filename === editor.currentFile)
                || (dir === editor2.currentDir && filename === editor2.currentFile)) {
                continue;
            }
            if (!existingFiles[toPath(dir, filename)]) {
                console.log('DELETED ' + toPath(dir, filename));
                deleted.push(toPath(dir, filename));
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

    return `/${dir}/${file}`;
}

async function getFileStatus(path) {
    let content;
    console.log('checking' , path);
    try {
        const memFile = getMemFile(path);
        if (!memFile?.handle) {
            return {
                status: 'error',
            }
        }

        const file = await memFile.handle.getFile();
        content = await file.text();
    } catch (error) {
        console.error('Error processing', path, error);
        return {
            status: 'error',
        }
    }

    // TODO why path is stored at all?
    // const path = serverFiles?.files?.[dir]?.[filename]?.path;
    let serverFile = getServerFile(path);
    console.log('STATUS', path, serverFile);
    if (serverFile === null) {
        console.log('NEW FILE ' + path);
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

        // console.log(diff);

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

    const fileExists = !await exists([path]);
    if (fileExists || !await isContentEqual(path, content)) {
        // TODO what if we're syncing first time and already have changes?
        console.log('Hashes do not match, writing file...', path);
        const writable = await fileHandle.createWritable();
        await writable.write(content);
        await writable.close();
    } else {
        console.log('Hashes match, no need to write file.');
    }

    const file = await fileHandle.getFile();
    return file.lastModified;
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

    // TODO multidir
    // const parts = path.split('/');
    // const filename = parts.pop();
    // const dir = parts.join('/');
    // if (files[dir] && files[dir][filename]) {
    //     delete files[dir][filename];
    // }
}

// TODO can we reuse moveFile?
async function moveCurrentFile(toDir) {
    isSyncingCurrentFile = true;

    // TODO add prevent syncing?
    const oldPath = currentEditor.path;
    const newPath = toPath(toDir, toFilename(currentEditor.path));

    try {
        let content = getCurrentContent();
        await saveTextFile(newPath, content);
        // TODO move to saveTextFile?
        removeMemFile(path);
        // delete files[editor.currentDir][editor.currentFile];
        console.log('MOVING to DIR:', toDir);

        addMemFile(path, {
            isFile: true,
            content: content,
            lastModified: 0,
            handle: await getFileHandle(newPath),
        });
        // files[toDir][editor.currentFile] = {
        //     content: content,
        //     lastModified: 0,
        //     handle: await getFileHandle(newPath),
        // }
        editor.path = newPath;
        addServerFile(newPath, content, 0);
        saveServerFiles();

        await removeFile(oldPath);
        await renderSidebar();
    } catch (error) {
        console.error('Error moving file:', error);
    }

    isSyncingCurrentFile = false;
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

    // const oldParts = oldPath.split('/');
    // const oldFilename = oldParts.pop();
    // const oldDir = oldParts.join('/');
    //
    // const newParts = newPath.split('/');
    // const newFilename = newParts.pop();
    // const newDir = newParts.join('/');

    try {
        let file = await (await getFileHandle(oldPath)).getFile();
        let content = await file.text();
        await saveTextFile(newPath, content);

        console.log('saving ' + newPath);
        addMemFile(newPath, {
            isFile: true,
            content: content,
            lastModified: 0,
            path: newPath,
            handle: await getFileHandle(newPath),
        });
        addServerFile(newPath, content, 0);
        saveServerFiles();

        // Server file will be removed here.
        await removeFile(oldPath);
        removeMemFile(oldPath);
        // delete files[oldDir][oldFilename];
        await renderSidebar();

        console.log(`Moved ${oldPath} to ${newPath}`);
    } catch (error) {
        console.error('Error moving file:', error);
    }
}

// Returns server file or null if not found.
function getServerFile(path) {
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = serverFiles['files'];
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            return null;
        }
        currentDir = currentDir[dir];
    }

    return currentDir[filename] || null;
}

function addServerFile(path, content, lastModifiedAt, clientLastSynced = null) {
    // const parts = path.split('/');
    // const filename = parts.pop();
    // const dir = parts.join('/');
    //
    // serverFiles['files'] = serverFiles['files'] ?? {};
    // serverFiles['files'][dir] = serverFiles['files'][dir] ?? {};
    // serverFiles['files'][dir][filename] = {
    //     hash: hash(content),
    //     lastModified: lastModifiedAt,
    //     lastSynced: clientLastSynced,
    //     path: path,
    // };
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = serverFiles['files'];
    for (let dir of dirs) {
        dir += '/';
        if (!currentDir[dir]) {
            return null;
        }
        currentDir = currentDir[dir];
    }

    currentDir[filename] = {
        isFile: true,
        hash: hash(content),
        lastModified: lastModifiedAt,
        lastSynced: clientLastSynced,
        path: path,
    }
}

function removeServerFile(path) {
    // console.log('removing info about server file', path);
    // const parts = path.split('/');
    // const filename = parts.pop();
    // const dir = parts.join('/');
    //
    // if (serverFiles.files?.[dir]?.[filename]) {
    //     delete serverFiles.files[dir][filename];
    // }
    let dirs = path.split('/');
    dirs = dirs.filter(d => d !== '');
    const filename = dirs.pop();

    let currentDir = serverFiles['files'];
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
    localStorage.setItem(SERVER_STORAGE_KEY, JSON.stringify(serverFiles));
}

// TODO save old file
async function openFile(path, saveToHistory = true, el = 'editor-textarea') {
    // Why we do normalize here as well?
    path = path.normalize('NFC');

    if (el === 'editor-textarea') {
        currentEditor = editor;
    } else if (el === 'editor2-textarea') {
        currentEditor = editor2;
    }

    await syncCurrentFile(false);

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
    chatButton.classList.remove('hidden');
    chatContainer.style.display = 'none';
    closeChatModal();

    const start = performance.now();

    const memFile = getMemFile(path);
    console.log(memFile);

    // Check if we're loading the same file and save cursor position
    let cursorPos = null;
    if (currentEditor.path === path) {
        console.log('saving cursor');
        cursorPos = editor.getCursor();
    }


    let filename = toFilename(path);
    const header = filename.replace(/\.md$/, '').replace(/^\w/, (c) => c.toUpperCase());
    let content = '';
    if (memFile.handle !== undefined) {
        const file = await memFile.handle.getFile();
        content = await file.text();
        content = `# ${header}\n${content}`;
    } else {
        // We use welcome's files
        content = memFile.content;
    }

    // TODO disable when syncing?
    if (saveToHistory) {
        const state = {path: path};
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
        console.log('cursor not null');
        currentEditor.setCursor(cursorPos);
        currentEditor.scrollTo(null, 0);
        // const editorScrollHeight = currentEditor.getScrollInfo().clientHeight;
        // Only scroll if editor'sheight more than current screen height
        // const contentFitsTheScreen = editorScrollHeight <= window.innerHeight;
        // console.log('FITS', contentFitsTheScreen);
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
    console.log(`File opened in: ${(end - start).toFixed(3)} milliseconds`);
    // Get the editor instance
}

// 0) Read content from local fs
// 1) Save current content to local filesystem
// 2) Sync it with the server
// TODO add hash of last read file comparison, merge on conflict (in which scenarious in can happen tho?)
async function syncCurrentFile(syncWithServer = true) {
    if (files === undefined || isWelcome || debug || currentEditor.path === undefined) {
        return;
    }

    /// TODO detect welcome mode separately
    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
        return;
    }

    if (isSaving) {
        return;
    }

    if (isSyncingCurrentFile) {
        return;
    }
    isSyncingCurrentFile = true;

    const path = currentEditor.path;
    let isCurrentEditorSame = () => {
        return path === window.currentEditor.path;
    }

    // Track in-editor renaming.
    if (path !== CHAT_PATH) {
        const filename = toFilename(path);
        try {
            // TODO track if no first line?
            const firstLine = currentEditor.getValue().split('\n')[0];
            let newFilename = ucfirst(fromHeaderToFilename(firstLine));
            // If filename is empty, generate an available "Untitled" name
            // TODO check for forbidden filename chars
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
                console.log('Filename has changed from ', filename, 'to', newFilename);
                // Change the file immediately, because on further await calls it can be synced by syncTexts.
                currentEditor.path = toDir(path) + '/' + newFilename;

                // 1. Remove file with old filename
                // 2. Create file with new filename

                let content = getCurrentContent();
                // TODO every await means we can can have RC due to editor content change
                await removeFile(path);
                removeMemFile(path);
                console.log('Removed', path);

                // Get fresher content after await.
                // if (isCurrentEditorSame()) {
                //     content = getCurrentContent();
                // }

                // if (isCurrentEditorSame()) {
                //     content = getCurrentContent();
                //     // Change current file if the editor is unchanged.
                // }
                const newPath = toDir(path) + '/' + newFilename;
                addMemFile(newPath, {
                    isFile: true,
                    content: content,
                    lastModified: 0,
                    path: newPath,
                    handle: await getFileHandle(newPath, true),
                });
                await saveTextFile(newPath, getCurrentContent());
                addServerFile(newPath, content, 0);
                saveServerFiles();
                console.log('Created', newPath);

                await renderSidebar();

                // Used further for syncing.
                // filename = newFilename;

                // Let's call it a day?
                isSyncingCurrentFile = false;
                return;
            }
        } catch (error) {
            console.error('Error during filename change:', error);
            isSyncingCurrentFile = false;
            return;
        }
    }

    if (path === CHAT_PATH) {
        // Try to load local changes.
        if (chatIsClean) {
            try {
                let inMemoryLastModified = getMemFile(path)?.lastModified;
                let file = await ((await getFileHandle(CHAT_PATH)).getFile());

                // Update last modified in memory.
                let memFile = getMemFile(path);
                memFile.lastModified = file.lastModified;
                addMemFile(path, memFile);

                let localLastModified = file.lastModified;
                // TODO inmemory lastmodified should be reloaded
                // files[dir][filename].lastModified = localLastModified;
                if (inMemoryLastModified !== localLastModified) {
                    console.log('Chat was modified locally', CHAT_PATH);
                    await openFile(CHAT_PATH);
                    isSyncingCurrentFile = false;
                    return;
                }
            } catch (error) {
                console.error('Error opening file:', error);
                isSyncingCurrentFile = false;
                return;
            }
        }

        if (syncWithServer) {
            try {
                await syncLocalFileWithServer(CHAT_PATH);
            } catch (error) {
                console.error('Error during sync with server:', error);
            }
        }

        isSyncingCurrentFile = false;
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
        isSyncingCurrentFile = false;
        return;
    }

    // I believe that after each await we should check that user hasn't changed the editor.
    if (!isCurrentEditorSame()) {
        isSyncingCurrentFile = false;
        return;
    }

    // Handling editor changes.
    if (contentWasModifiedLocally && currentEditor.isClean()) {
        console.log('WAS MODIFIED LOCALLY, and the editor is clean', path);

        // Changes only from local system
        try {
            await openFile(path);
        } catch (error) {
            console.error('Error opening file:', error);
            isSyncingCurrentFile = false;
            return;
        }
    } else if (!currentEditor.isClean()) {
        isSaving = true;
        try {
            // const file = files[dir][filename];
            console.log('Getting', path);
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
            isSyncingCurrentFile = false;
            return;
        }
        isSaving = false;
    }

    if (syncWithServer) {
        try {
            await syncLocalFileWithServer(path);
        } catch (error) {
            console.error('Error during sync with server:', error);
        }
    }

    isSyncingCurrentFile = false;
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
function walk(obj, callback, path = '/') {
    if (obj.isFile) {
        callback(path, obj, true);
        return;
    }
    for (const key in obj) {
        const item = obj[key];
        const fullPath = path + key;

        if (item.isFile) {
            callback(fullPath, item, true);
        } else {
            callback(fullPath, item, false);
            walk(item, callback, fullPath);
        }
    }
}

function toFilename(path) {
    const {filename} = toDirAndFilename(path);

    return filename;
}

// Dir with no slash at the end.
// Empty for root.
function toDir(path) {
    const {dirPath} = toDirAndFilename(path);

    if (dirPath === '/') {
        return '';
    }

    return dirPath;
}

// Dir with no slash at the end.
function toDirAndFilename(path) {
    let parts = path.split('/');
    parts = parts.filter(p => p !== '');

    const filename = parts.pop();
    let dirPath = '/' + parts.join('/');
    return {dirPath, filename};
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


window.addEventListener('beforeunload', function () {
    // clearInterval(window.loader);
    clearInterval(window.saver);
});


// Worker to process the saving queue
window.saver = setInterval(syncCurrentFile, CURRENT_FILE_SYNC_INTERVAL);