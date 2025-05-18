const saverInterval = 500; // ms, how often to save currently open file
const loaderInterval = 3000; // ms, how often to load current file from local file system

let hasUnsavedChanges = false;
let isSaving = false;

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

let filesMetadata = {files: {}, timestamps: {}};
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
    const savedStates = localStorage.getItem(SYNC_STORAGE_KEY);
    if (savedStates) {
        filesMetadata = JSON.parse(savedStates);
    }

    return newFiles;
}

async function syncAllWithServer() {
    const startTime = performance.now();
    console.log("Starting sync with server...");

    // Send locally modified files and timestamps of last seen dirs from the server
    let server = {};
    let filesToSend = await collectLocallyModifiedFiles();
    try {
        let response = await fetch('https://habits.files.md/sync', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'Authorization': localStorage.getItem('token')},
            body: JSON.stringify({
                // TODO rem
                files: [],
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
        return;
    }

    // Write files received from the server
    for (const fileInfo of server.files) {
        const {path, content, lastModified} = fileInfo;
        // If it is current file, skip, because we sync it separately
        if (path === `${editor.currentDir}/${editor.currentFile}`) {
            console.log("Skip current " + path);
            continue;
        }

        console.log("Syncing " + path);
        await write(path, content)
        setMetadata(path, content, lastModified);
    }
    filesMetadata['timestamps'] = server.timestamps;
    saveMetadata();
    console.log("Sync completed in " + (performance.now() - startTime) + "ms");
}

async function syncFileWithServer(dir, filename) {
    const path = `${dir}/${filename}`;
    let file = await (await getFileHandle(path)).getFile();
    // TODO we might only need to send content when modifying
    let content = await file.text();
    let serverTimestamp = getMetadata(path)?.lastModified || 0;

    let serverFile = {};
    try {
        let response = await fetch('https://habits.files.md/syncFile', {
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
            saveLastModified(path, json.lastModified);
            console.log(json.status);
            return;
        }
        serverFile = json
    } catch (error) {
        console.error("Network error occurred:", error.message);
        return;
    }
    console.log(serverFile);
    await write(path, serverFile.content);
    console.log('showing file sync one');
    await showFile(dir, filename);
    console.log("File synced with server");
}

async function collectLocallyModifiedFiles() {
    const filesToSend = [];
    const promises = [];
    for (const dir in files) {
        if (dir === 'img') continue; // Skip image directory

        for (const filename in files[dir]) {
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

async function write(path, content) {
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

function saveLastModified(path, lastModified) {
    const parts = path.split('/');
    const filename = parts.pop();
    const dir = parts.join('/');

    // TODO what if missing?
    filesMetadata['files'][dir][filename][lastModified] = lastModified;
    saveMetadata();
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

// 1) Save current content to local filesystem
// 2) Sync it with the server
async function saveCurrentFile() {
    if (!hasUnsavedChanges) return;

    // Wait until not saving
    while (isSaving) {
        await new Promise(r => setTimeout(r, 50));
    }

    // TODO better way would be this:
    // Read file from fs with it's timestamp
    // If in our memory we have actual TS, just write file back
    // If fs has fresher change, merge.
    // Sync with server.
    isSaving = true;
    try {
        const dir = editor.currentDir;
        const filename = editor.currentFile;
        const fileData = files[dir][filename];
        if (fileData && fileData.handle) {
            let content = getCurrentContent();
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
        hasUnsavedChanges = false;
        return;
    }

    await syncFileWithServer(editor.currentDir, editor.currentFile);

    isSaving = false;
    hasUnsavedChanges = false;
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

    // Refresh current file
    // window.loader = setInterval(async function () {
    //     // Check if current file has been modified
    //     let path = `${editor.currentDir}/${editor.currentFile}`;
    //     let contentWithNormalizedLinks = getCurrentContent().replace(/\[\[(.+?)\|.*?\]\]/g, '[[$1]]');
    //     if (!hasUnsavedChanges && !await isContentEqual(path, contentWithNormalizedLinks)) {
    //         console.log('showing file');
    //         await showFile(editor.currentDir, editor.currentFile, false);
    //     }
    // }, loaderInterval)
}

window.addEventListener('beforeunload', function () {
    // clearInterval(window.loader);
    clearInterval(window.saver);
});


// Worker to process the saving queue
window.saver = setInterval(saveCurrentFile, saverInterval);