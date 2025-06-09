// HyperMD/Codemirror editor
let editor;
let focusedItemIndex = -1;
let debug = false;
// let debug = {dir: "", file: "Sim.md", loaded: false};

async function init(el) {
    initEditor(el);

    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
        document.getElementById('welcome').style.display = 'block';
        files = defaultFiles;
        buildSidebar();
        await openFile("", "Welcome.md");
        return;
    }

    const permission = await savedDirHandle.queryPermission({mode: 'read'});
    if (permission !== 'granted') {
        document.getElementById('welcome').style.display = 'block';
    }

    await initFiles();
    buildSidebar();
    await showRandomFile();
}

function initEditor(el) {
    editor = HyperMD.fromTextArea(el, {
        dragDrop: false,
        mode: "hypermd", lineNumbers: false, extraKeys: {
            // "Shift-Space": "autocomplete",
            'Cmd-[': false, 'Cmd-]': false,
        },
        hintOptions: {
            hint: CompleteEmoji.createHintFunc(),
            closeCharacters: /$^/,
            closeOnUnfocus: false,
            completeSingle: false,
            alignWithWord: false
        },
        hmdFoldEmoji: {
            myEmoji: createAutocompleteDict
        }
    });
    editor.setSize(null, "100%");

    editor.hmdResolveURL = function (path) {
        if (typeof path === 'undefined') {
            return path
        }

        path = path.replace(/%20/g, " ");

        if (/^(?!http|https|\[).+\.md$/.test(path)) {
            let parts = path.split('/');
            if (parts.length === 1) {
                openFile("", path);
                return;
            }
            openFile(parts[0], parts[1]);
            return path;
        }

        const match = path.match(/^img\/(.+\.(png|jpg|jpeg|gif|webp))$/i);

        if (match && files['img'] && files['img'][match[1]]) {
            return files['img'][match[1]].imageUrl;
        }

        return path;
    };

    editor.hmdReadLink = async function (path) {
        path = path.replace(/\|.*]$/, '');
        path = path.replace('[', '').replace(']', '');
        let parts = path.split('/');
        if (parts.length === 1) {
            await openFile("", path + '.md');
            return;
        }

        await openFile(parts[0], parts[1] + '.md');
    };

    editor.on("inputRead", async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '[') {
            editor.showHint({
                completeSingle: false, updateOnCursorActivity: true,
            })
        }
    })

    editor.setOption("viewportMargin", Infinity);
    initAutoscroll(editor);

    // TODO Image uploading
    editor.on("paste", async (_, event) => {
        const items = (event.clipboardData || event.originalEvent.clipboardData).items;
        for (const item of items) {
            if (item.kind === "file" && item.type.startsWith("image/")) {
                const file = item.getAsFile();
                const fileName = `${new Date().toISOString().replace(/[:.]/g, '-')}.png`;
                // await saveImageToDirectory(file, fileName);

                const markdownImageSyntax = `![](img/${fileName})`;
                editor.replaceSelection(markdownImageSyntax);
                // if (fileHandle) {
                //     // Insert the Markdown image syntax into the editor
                //     const markdownImageSyntax = `![[${fileName}]]`;
                //     editor.replaceSelection(markdownImageSyntax);
                //     console.log(`Image saved as: ${fileName}`);
                // } else {
                //     console.error("Failed to save the image.");
                // }
            }
        }
    });

    // Editor keybindings
    editor.addKeyMap({
        'Cmd-Y': function (cm) {
            var cursor = cm.getCursor();
            var lineStart = {line: cursor.line, ch: 0};
            cm.replaceRange('✅ ', lineStart);
            cm.focus();
        },
        'Ctrl-Y': function (cm) {
            var cursor = cm.getCursor();
            var lineStart = {line: cursor.line, ch: 0};
            cm.replaceRange('✅ ', lineStart);
            cm.focus();
        },
        'Cmd-B': function (cm) {
            let selection = cm.getSelection();
            let trimmedSelection = selection.trim();
            let prefix = selection.slice(0, selection.indexOf(trimmedSelection));
            let suffix = selection.slice(selection.indexOf(trimmedSelection) + trimmedSelection.length);

            const isBold = trimmedSelection.startsWith("**") && trimmedSelection.endsWith("**");

            let start = cm.getCursor("start");
            let end = cm.getCursor("end");

            if (isBold) {
                cm.replaceSelection(prefix + trimmedSelection.slice(2, -2) + suffix);
                cm.setSelection(
                    { line: start.line, ch: start.ch + prefix.length },
                    { line: end.line, ch: end.ch - suffix.length - 4 }
                );
            } else {
                cm.replaceSelection(prefix + `**${trimmedSelection}**` + suffix);
                cm.setSelection(
                    { line: start.line, ch: start.ch + prefix.length },
                    { line: end.line, ch: end.ch - suffix.length + 4 }
                );
            }
            cm.focus();
        },
        'Cmd-I': function (cm) {
            let selection = cm.getSelection();
            let trimmedSelection = selection.trim();
            let prefix = selection.slice(0, selection.indexOf(trimmedSelection));
            let suffix = selection.slice(selection.indexOf(trimmedSelection) + trimmedSelection.length);

            const isItalic = trimmedSelection.startsWith("*") && trimmedSelection.endsWith("*");

            let start = cm.getCursor("start");
            let end = cm.getCursor("end");

            if (isItalic) {
                cm.replaceSelection(prefix + trimmedSelection.slice(1, -1) + suffix);
                cm.setSelection(
                    { line: start.line, ch: start.ch + prefix.length },
                    { line: end.line, ch: end.ch - suffix.length - 2 }
                );
            } else {
                cm.replaceSelection(prefix + `*${trimmedSelection}*` + suffix);
                cm.setSelection(
                    { line: start.line, ch: start.ch + prefix.length },
                    { line: end.line, ch: end.ch - suffix.length + 2 }
                );
            }
            cm.focus();
        }
    });
}

function createAutocompleteDict() {
    const dict = {};

    Object.keys(excludeDirs(systemDirs)).forEach(dir => {
        Object.keys(files[dir]).forEach(filename => {
            const key = `${filename.replace(/\.md$/, "")}`;
            const filePath = `${filename.replace(/\.md$/, "")}](${dir}/${filename})`;
            dict[key] = filePath;
        });
    });

    return dict;
}

function buildSidebar() {
    let root = new TreeNode("files");
    for (const dir in files) {
        if (dir === '' || dir === 'img') {
            continue;
        }

        let dirNode = new TreeNode(dir, {expanded: false});
        for (let file in files[dir]) {
            let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await openFile(node.parent.toString(), node.toString() + ".md");
            });
            dirNode.addChild(fileNode);
        }
        root.addChild(dirNode);
    }

    if (files['']) {
        // Adding root files after dirs
        for (let file in files[""]) {
            let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await openFile("", node.toString() + ".md");
            });
            root.addChild(fileNode)
        }
    }

    new TreeView(root, "#sidebar", {
        show_root: false,
    });
}

async function showRandomFile() {
    if (debug) {
        await openFile(debug.dir, debug.file);
        return;
    }

    const allFiles = [];
    for (let dir in excludeDirs(systemDirs)) {
        for (let file in files[dir]) {
            allFiles.push({dir, file});
        }
    }

    if (allFiles.length === 0) {
        console.error("No files found to open.");
        return;
    }

    const randomFile = allFiles[Math.floor(Math.random() * allFiles.length)];

    try {
        await openFile(randomFile.dir, randomFile.file);
    } catch (error) {
        console.error("Failed to open random file:", error);
    }
}

async function openFile(dir, filename, saveToHistory = true) {
    filename = filename.normalize("NFC");
    const fileData = files[dir][filename];

    // Check if we're loading the same file and save cursor position
    let cursorPos = null;
    if (editor.currentDir === dir && editor.currentFile === filename) {
        console.log('saving cursor');
        cursorPos = editor.getCursor();
    }

    const header = filename.replace(/\.md$/, "").replace(/^\w/, (c) => c.toUpperCase());
    let content = "";
    if (fileData.handle !== undefined) {
        const file = await fileData.handle.getFile();
        content = await file.text();
    } else {
        // When do we go here?
        content = fileData.content;
    }

    content = `# ${header}\n${content}`;

    editor.currentDir = dir;
    editor.currentFile = filename;
    // TODO disable when syncing?
    if (saveToHistory) {
        const state = {dir: dir, file: filename};
        history.pushState(state, '');
    }

    editor.off();
    const wrapper = editor.getWrapperElement();
    if (wrapper && wrapper.parentNode) {
        wrapper.parentNode.removeChild(wrapper);
    }

    initEditor(document.getElementById('editor'));
    editor.currentDir = dir;
    editor.currentFile = filename;

    editor.getDoc().setValue(content);
    editor.clearHistory();
    editor.markClean();
    editor.focus();

    if (cursorPos !== null) {
        console.log('cursor not null');
        editor.setCursor(cursorPos);
        editor.scrollIntoView(cursorPos, 500);
        // TODO only focus if there's no quick dialogue
        editor.focus();
    } else {
        // Focus last line if we have short content
        const cmScroller = document.querySelector('.CodeMirror-scroll')
        const hasVerticalScroll = cmScroller.scrollHeight > cmScroller.clientHeight
        if (hasVerticalScroll) {
            focusLastLine();
            return;
        }
        focusLastLine();
    }
}

// Focus last line before the links.
function focusLastLine() {
    const lastLine = editor.lastLine();
    let targetLine = lastLine;
    for (let i = lastLine; i >= 0; i--) {
        const lineContent = editor.getLine(i).trim();
        if (!lineContent.startsWith("[") && (!lineContent.endsWith("]") || !lineContent.endsWith(")"))) {
            targetLine = i;
            console.log('found links');
            break;
        }
    }
    const targetChar = editor.getLine(targetLine).length;
    editor.setCursor({line: targetLine, ch: targetChar});
    // Cursor at the end, but scroll the doc to top
    editor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
    console.log('focusing last line');
    editor.focus();
}

function updateFocusedItem(resultsList) {
    document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
    resultsList.forEach((item, index) => {
        if (index === focusedItemIndex) {
            item.classList.add('focused');
            item.scrollIntoView({block: "nearest"});
        } else {
            item.classList.remove('focused');
        }
    });
}

function openSearchModal() {
    document.getElementById('search').style.display = 'block';
    const inputField = document.getElementById('search-input');
    inputField.focus();

    focusedItemIndex = -1;
    const goToFileResults = document.getElementById('search-results');
    goToFileResults.innerHTML = '';
    loadRecentFiles();
}

function openMoveModal() {
    document.getElementById('move').style.display = 'block';
    const inputField = document.getElementById('move-input');
    inputField.focus();

    focusedItemIndex = -1;
    const goToFileResults = document.getElementById('move-results');
    goToFileResults.innerHTML = '';
    showMoveResults(getMoveDestinations());
}

function isModifierKey(event) {
    return event.metaKey || event.ctrlKey || event.altKey;
}

window.addEventListener('keydown', (event) => {
    if (isModifierKey(event) && event.key === 'p') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('search-input').value = ''
        openSearchModal();
    }

    if (isModifierKey(event) && event.key === 'm') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('move-input').value = ''
        openMoveModal();
    }

    if (isModifierKey(event) && event.key === 'k') {
        event.preventDefault();
        document.getElementById('search-input').value = ''
        openSearchModal();
    }
});

window.addEventListener('focus', () => {
    console.log('got focus');
});

function closeSearchModal() {
    document.getElementById('search').style.display = 'none';
}

function loadRecentFiles() {
    let results = [];
    for (const dir of Object.keys(excludeDirs(systemDirs))) {
        for (const filename of Object.keys(files[dir])) {
            results.push({
                dir, filename, lastModified: files[dir][filename].lastModified,
            });
        }
    }

    results = results
        .sort((a, b) => b.lastModified - a.lastModified)
        .slice(0, 8);

    showSearchResults(results);
}

function getMoveDestinations() {
    let dirs = ["/"];
    for (const dir of Object.keys(files)) {
        if (dir === '' || dir === 'img') {
            continue;
        }
        dirs.push(dir);
    }

    // Place _read_ etc in the end
    dirs.sort((a, b) => {
        return a.includes('_') - b.includes('_') || a.localeCompare(b);
    });

    return dirs;
}

function search() {
    const search = document.getElementById('search-input').value.toLowerCase();
    if (search.trim() === '') {
        loadRecentFiles();
        return;
    }

    const list = document.getElementById('search-results');
    list.innerHTML = '';


    let results = [];
    const lowPriorityDirs = ["archive", "_read_", "_watch_", "_shop_", "habits", "triggers", "today", "later"];

    // Levenshtein distance
    for (const dir in excludeDirs(systemDirs)) {
        for (const filename in files[dir]) {
            const potentialMatch = filename.replace(/\.md$/, "");
            let similarityScore = similarity(search, potentialMatch);

            if (similarityScore >= 70) {
                if (lowPriorityDirs.includes(dir)) {
                    similarityScore -= 30;
                }
                results.push({
                    filename: filename, dir: dir, score: similarityScore
                });
            }
        }
    }

    // Substring
    for (const dir in files) {
        for (const filename in files[dir]) {
            const potentialMatch = filename.replace(/\.md$/, "");
            const isSubstringMatch = potentialMatch.toLowerCase().includes(search.toLowerCase());

            if (!isSubstringMatch) {
                continue; // Skip this filename if it doesn't match
            }

            let matchedPercent = (search.length / potentialMatch.length) * 100;

            results.push({
                filename: filename, dir: dir, score: Math.round(matchedPercent)
            });
        }
    }

    const uniqueResultsMap = new Map();
    for (let i = 0; i < results.length; i++) {
        const item = results[i];
        const key = `${item.filename}-${item.dir}`;

        if (!uniqueResultsMap.has(key) || uniqueResultsMap.get(key).score < item.score) {
            uniqueResultsMap.set(key, item);
        }
    }
    results = Array.from(uniqueResultsMap.values()).sort((a, b) => b.score - a.score);
    showSearchResults(results);
}

function suggestMove() {
    const search = document.getElementById('move-input').value.toLowerCase();
    if (search.trim() === '') {
        showMoveResults(getMoveDestinations());
        return;
    }

    let dirs = getMoveDestinations();
    dirs = dirs.filter(dir => dir.toLowerCase().startsWith(search));

    showMoveResults(dirs);
}

function showSearchResults(results) {
    const list = document.getElementById('search-results');
    results.forEach(({dir, filename}, index) => {
        const listItem = document.createElement('li');
        let title = filename.replace(/\.md$/, "")
        if (dir !== '') {
            listItem.textContent = `${dir}/${title}`;
        } else {
            listItem.textContent = title;
        }
        listItem.setAttribute('data-path', `${dir}/${filename}`);
        listItem.setAttribute('data-index', index);
        listItem.onclick = () => {
            openFile(dir, filename);
            closeSearchModal();
        };
        listItem.onmouseenter = () => {
            document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
            listItem.classList.add('focused');
            focusedItemIndex = index;
        };
        list.appendChild(listItem);
    });

    focusedItemIndex = 0;
    updateFocusedItem(list.querySelectorAll('li'));
}

function showMoveResults(dirs) {
    const list = document.getElementById('move-results');
    list.innerHTML = '';
    dirs.forEach((dir, index) => {
        const listItem = document.createElement('li');
        listItem.textContent = dir;
        listItem.setAttribute('data-path', `$dir`);
        listItem.setAttribute('data-index', index);
        listItem.onclick = () => {
            // showFile(dir, filename);
            closeSearchModal();
        };
        listItem.onmouseenter = () => {
            document.querySelectorAll('#move-results li').forEach(li => li.classList.remove('focused'));
            listItem.classList.add('focused');
            focusedItemIndex = index;
        };
        list.appendChild(listItem);
    });

    focusedItemIndex = 0;
    updateFocusedItem(list.querySelectorAll('li'));
}

function closeSearch() {
    document.getElementById('search').style.display = 'none';
}

function closeMove() {
    document.getElementById('move').style.display = 'none';
}

document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
        closeSearch();
        closeMove();
    }
});

// Toggle focus mode
document.addEventListener('keydown', function (event) {
    if (isModifierKey(event) && event.key === 'Enter') {
        event.preventDefault();
        const sidebar = document.getElementById('sidebar');
        if (sidebar.style.display === 'none') {
            sidebar.style.display = 'block';
        } else {
            sidebar.style.display = 'none'; // Hide the sidebar
        }
    }
});

window.addEventListener('popstate', (event) => {
    const state = event.state;
    if (state) {
        openFile(state['dir'], state['file'], false);
    }
});

document.getElementById('search').addEventListener('keydown', (event) => {
    const resultsList = document.getElementById('search-results').querySelectorAll('li');

    if (event.key === 'Enter') {
        event.preventDefault();
        if (resultsList[focusedItemIndex]) {
            const [dir, filename] = resultsList[focusedItemIndex].getAttribute('data-path').split('/');
            openFile(dir, filename);
            closeSearchModal();
        }
    }

    if (event.key === 'ArrowDown') {
        event.preventDefault();
        focusedItemIndex = (focusedItemIndex + 1) % resultsList.length;
        updateFocusedItem(resultsList);
    } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        focusedItemIndex = (focusedItemIndex - 1 + resultsList.length) % resultsList.length;
        updateFocusedItem(resultsList);
    }
});

function excludeDirs(excludedDirs) {
    const filteredFiles = {};

    for (const dir in files) {
        if (!excludedDirs.includes(dir)) {
            filteredFiles[dir] = files[dir];
        }
    }

    return filteredFiles;
}

async function openDir() {
    let dirHandle = await window.showDirectoryPicker();
    document.getElementById('welcome').style.display = 'none';
    await saveDirectoryHandle(dirHandle);
    files = await loadLocalFiles(dirHandle)
    console.log(files);
    buildSidebar();
    await showRandomFile();
}

function getCurrentContent() {
    let content = editor.getValue();
    const header = editor.currentFile.replace(/\.md$/, '').replace(/^\w/, (c) => c.toUpperCase());
    if (content.startsWith(`# ${header}`)) {
        content = content.slice(`# ${header}\n`.length);
    }

    return content;
}

async function getImageUrl(fileHandle) {
    const file = await fileHandle.getFile();
    return URL.createObjectURL(file);
}

// Normalize text to use only \n as line endings
function normNewLines(text) {
    return text.replace(/\r\n|\r/g, "\n");
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

async function getRootDirHandle() {
    const db = await initDB();
    return new Promise((resolve, reject) => {
        const transaction = db.transaction('handles', 'readonly');
        const store = transaction.objectStore('handles');
        const request = store.get('savedDirectoryHandle');
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

document.addEventListener('mousedown', (event) => {
    const goToFile = document.getElementById('search');
    if (goToFile.style.display === 'block' &&
        !goToFile.contains(event.target)) {
        closeSearchModal();
    }
});

// Reload files once the app gains focus
window.addEventListener("focus", async () => {
    // Sync media first, so that new images for current file would be loaded
    await syncMediaFilesFromServer();
    await syncCurrentFile();

    const savedDirectoryHandle = await getRootDirHandle();

    // Benchmark time took
    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle);
    const end = performance.now();
    console.log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncTextsWithServer()
    console.log("Sync completed");
});