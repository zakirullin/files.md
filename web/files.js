// TODO Do sync not as often
const saverInterval = 1000; // ms, how often to save currently open file
const loaderInterval = 3000; // ms, how often to load current file from local file system

let hasUnsavedChanges = false;
let isSaving = false;
let isSyncing = false

// Files structure:
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
const supportedFileTypes = ['md', 'txt', 'png', 'jpg', 'jpeg', 'webp', 'gif',];
const systemDirs = ["img", "archive", "_read_", "_watch_", "_shop_", "today", "later", "journal", "habits", "triggers", "places"];

let filesMetadata = {files: {}, timestamps: {}, mediaTimestamp: 0};
const SYNC_STORAGE_KEY = 'files';

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
    while (hasUnsavedChanges) {
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

                if (dir === 'img') {
                    getImageUrl(entry).then(imageUrl => {
                        newFiles[dir][filename].imageUrl = imageUrl;
                    });
                }
            }
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
    const savedMetadata = localStorage.getItem(SYNC_STORAGE_KEY);
    if (savedMetadata) {
        filesMetadata = JSON.parse(savedMetadata);
    }

    return newFiles;
}

async function syncAllWithServer() {
    return;
    if (isSyncing) return;
    isSyncing = true;

    const startTime = performance.now();
    console.log("Starting sync with server...");

    // Send locally modified files and timestamps of last seen dirs from the server
    let server = {};
    let filesToSend = await collectLocallyModifiedTextFiles();
    try {
        let response = await fetch('https://api.files.md/syncTexts', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
            body: JSON.stringify({
                // TODO rem
                files: filesToSend,
                timestamps: filesMetadata['timestamps'] || [],
            })
        });
        if (!response.ok) {
            console.log(`Server responded with ${response.status}`);
            return;
        }

        server = await response.json();
    } catch (error) {
        console.error("Network error occurred:", error.message);
        isSyncing = false;
        return;
    }

    // TODO more fine-graned try-catch?
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

            // todo try-catch?
            await saveTextFile(path, content)
            setMetadata(path, content, lastModified);
            saveMetadata();
        }
        filesMetadata['timestamps'] = server.timestamps;
        saveMetadata();
    } catch(error) {
        console.error("Can't sync: ", error.message)
    }

    console.log("Sync completed in " + (performance.now() - startTime) + "ms");

    isSyncing = false;
}

async function syncFileWithServer(dir, filename) {
    const path = `${dir}/${filename}`;
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
                path: `${dir}/${filename}`,
                lastModified: serverTimestamp,
                content: content,
            })
        });
        if (!response.ok) {
            console.log(`Server responded with ${response.status}`);
            return;
        }
        let json = await response.json();
        if (["not_modified", "updated_on_server"].includes(json.status)) {
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
    await showFile(dir, filename);
    console.log("File synced with server");
}

async function syncMediaFilesFromServer() {
    // TODO skip if already syncing

    console.log(`Starting media sync from img folder...`);
    const startTime = performance.now();

    const mediaTimestamp = filesMetadata['mediaTimestamp'] || 0;
    try {
        const response = await fetch('https://api.files.md/syncMedias', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': localStorage.getItem('token')
            },
            body: JSON.stringify({
                folder: 'img',
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
            const { path, lastModified} = fileInfo;
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
                await saveMediaFile(`img/${path}`, blob, lastModified);
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
        if (lastModified > filesMetadata['mediaTimestamp']) {
            filesMetadata['mediaTimestamp'] = lastModified;
            saveMetadata();
        }

        // Load file handle into files
        // TODO split path by dir, filename? Not to have this logic
        const parts = path.split('/');
        let filename = parts.pop();
        files['img'][filename] = {handle: fileHandle};
        fileHandle.getFile().then(file => {
            files['img'][filename].lastModified = file.lastModified;
        });
        getImageUrl(fileHandle).then(imageUrl => {
            files['img'][filename].imageUrl = imageUrl;
        });
    } catch (error) {
        console.error(`Error writing media file ${path}:`, error);
        throw error;
    }
}

async function collectLocallyModifiedTextFiles() {
    const filesToSend = [];
    const promises = [];
    for (const dir in files) {
        if (dir === 'img') continue; // Skip image directory

        for (const filename in files[dir]) {
            if (dir === editor.currentDir && filename === editor.currentFile) {
                console.log("Skip sending current file");
                continue;
            }

            const promise = getFileIfChanged(dir, filename)
                .then(result => {
                    if (result) filesToSend.push(result);
                });
            promises.push(promise);
        }
    }

    await Promise.all(promises);
    return filesToSend;
}

async function getFileIfChanged(dir, filename) {
    let content;
    try {
        const fileData = files[dir][filename];
        if (!fileData?.handle) return null;

        const file = await fileData.handle.getFile();
        content = await file.text();
    } catch (error) {
        console.error(`Error processing ${dir}/${filename}:`, error);
        return null;
    }


    const path = filesMetadata?.files?.[dir]?.[filename]?.path;
    if (!path) {
        console.log(`File ${dir}/${filename} not found on server, skipping...`);
        return null;
    }

    const serverHash = filesMetadata?.files?.[dir]?.[filename]?.hash;
    const serverTime = filesMetadata?.files?.[dir]?.[filename]?.lastModified;
    if (serverHash !== hash(content)) {
        return {
            content,
            path,
            lastModified: serverTime,
        };
    }

    return null;
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
        console.log("Malformed name, skipping file...");
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
        // Log from async without console.log
        setTimeout(() => {
            console.log("Content differs:", diff.join('\n'));
        }, 0);

        return false;
    } else {
        return true;
    }
}

async function saveTextFile(path, content) {
    let fileHandle = await getFileHandle(path);
    if (fileHandle === null) {
        // TODO fix once Chromium fixes the bug
        console.log("Malformed name, skipping file...");
        return;
    }

    if (!await isContentEqual(path, content)) {
        console.log("Hashes do not match, writing file...");
        // TODO rem
        const writable = await fileHandle.createWritable();
        await writable.write(content);
        await writable.close();
    } else {
        console.log("Hashes match, no need to write file.");
    }
}

function getMetadata(path) {
    const parts = path.split('/');
    const filename = parts.pop();
    const dir = parts.join('/');

    if (filesMetadata['files']?.[dir]?.[filename]) {
        return filesMetadata['files'][dir][filename];
    } else {
        return null;
    }
}

function setMetadata(path, content, lastModified) {
    const parts = path.split('/');
    const filename = parts.pop();
    const dir = parts.join('/');

    filesMetadata['files'] = filesMetadata['files'] ?? {};
    filesMetadata['files'][dir] = filesMetadata['files'][dir] ?? {};
    filesMetadata['files'][dir][filename] = {
        hash: hash(content),
        lastModified: lastModified,
        path: path
    };
}

function saveMetadata() {
    localStorage.setItem(SYNC_STORAGE_KEY, JSON.stringify(filesMetadata));
}

// 0) Read content from local fs
// 1) Save current content to local filesystem
// 2) Sync it with the server
// TODO add hash of last read file comparison, merge on conflict (in which scenarious in can happen tho?)
async function syncCurrentFile() {
    // Wait until not saving
    while (isSaving) {
        await new Promise(r => setTimeout(r, 50));
    }

    const path = `${editor.currentDir}/${editor.currentFile}`;
    const contentWasModifiedLocally = !await isContentEqual(path, getCurrentContent());

    // TODO better way would be this:
    // Read file from fs with it's timestamp
    // If in our memory we have actual TS, just write file back
    // If fs has fresher change, merge.
    // Sync with server.

    if (contentWasModifiedLocally && !hasUnsavedChanges) {
        // Changes only from local system
        await showFile(editor.currentDir, editor.currentFile);
    } else if (hasUnsavedChanges) {
        isSaving = true;
        try {
            const dir = editor.currentDir;
            const filename = editor.currentFile;
            const fileData = files[dir][filename];
            if (fileData && fileData.handle) {
                let content = getCurrentContent();
                if (hasUnsavedChanges && contentWasModifiedLocally) {
                    // Changes from both sides: editor and local fs, need merging

                }
                // We need to atomically reset the flag once we captured a snapshot of particular version of the content.
                // This flag can be changed in the event loop, as a result of user making changes to the text in the middle
                // of our saving process. The new unsaved changes would be then handled by a subsequent saveCurrentFile() call.
                // Initially, this flag assignment was erroneously placed at the end of the function, resulting in a race condition.
                // If we override flag in the end, we would lose any changes that occurred during the 3 await calls.
                hasUnsavedChanges = false

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
            hasUnsavedChanges = true;
            return;
        }
        isSaving = false;
    }

    await syncFileWithServer(editor.currentDir, editor.currentFile);
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
    await syncAllWithServer();
    await syncMediaFilesFromServer();
}

window.addEventListener('beforeunload', function () {
    // clearInterval(window.loader);
    clearInterval(window.saver);
});


// Worker to process the saving queue
window.saver = setInterval(syncCurrentFile, saverInterval);