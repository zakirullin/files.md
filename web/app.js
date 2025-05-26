// HyperMD/Codemirror editor
let editor;
let focusedItemIndex = -1;

async function init(el) {
    initEditor(el);

    const savedDirectoryHandle = await getRootDirHandle();
    const userHasOpenedDirectory = savedDirectoryHandle instanceof FileSystemDirectoryHandle;
    if (!userHasOpenedDirectory) {
        document.getElementById('welcome').style.display = 'block';
        files = defaultFiles;
        buildSidebar();
        await showFile("", "Welcome.md");
        return;
    }

    const permission = await savedDirectoryHandle.queryPermission({mode: 'read'});
    if (permission !== 'granted') {
        document.getElementById('welcome').style.display = 'block';
    }

    await initFiles();
    buildSidebar();
    await showRandomFile();
}

function initEditor(el) {
    editor = HyperMD.fromTextArea(el, {
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
                showFile("", path);
                return;
            }
            showFile(parts[0], parts[1]);
            return path;
        }

        const match = path.match(/^img\/(.+\.(png|jpg|jpeg|gif|webp))$/i);

        if (match && files['img'] && files['img'][match[1]]) {
            return files['img'][match[1]].imageUrl;
        }

        return path;
    };

    editor.hmdReadLink = async function (path) {
        path = path.replace('[', '').replace(']', '');
        let parts = path.split('/');
        if (parts.length === 1) {
            await showFile("", path + '.md');
            return;
        }

        await showFile(parts[0], parts[1] + '.md');
    };

    editor.on("inputRead", async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '[') {
            editor.showHint({
                completeSingle: false, updateOnCursorActivity: true,
            })
        }
    })

    editor.on("change", async function (cm, changeObj) {
        // Save on user input only
        if (changeObj.origin && changeObj.origin !== "setValue") {
            hasUnsavedChanges = true
        }
    });

    editor.setOption("viewportMargin", Infinity);
    editor.setOption("cursorScrollMargin", 50);

    let scrollInterval;
    let isSelecting = false;
    const resistance = 100; // the more it is, the slower scrolling is

    function startAutoScroll(direction) {
        if (scrollInterval) return; // Already scrolling

        scrollInterval = setInterval(() => {
            const scrollInfo = editor.getScrollInfo();
            const lineHeight = editor.defaultTextHeight();

            if (direction === 'up') {
                editor.scrollTo(null, Math.max(0, scrollInfo.top - lineHeight));
            } else if (direction === 'down') {
                const maxScroll = scrollInfo.height - scrollInfo.clientHeight;
                editor.scrollTo(null, Math.min(maxScroll, scrollInfo.top + lineHeight));
            }
        }, resistance);
    }
    function stopAutoScroll() {
        if (scrollInterval) {
            clearInterval(scrollInterval);
            scrollInterval = null;
        }
    }

    function checkAutoScroll(e) {
        if (!isSelecting) return;

        const editorRect = editor.getWrapperElement().getBoundingClientRect();
        const margin = 50;
        const mouseY = e.clientY;

        // Check if mouse is near top or bottom of editor
        if (mouseY < editorRect.top + margin) {
            startAutoScroll('up');
        } else if (mouseY > editorRect.bottom - margin) {
            startAutoScroll('down');
        } else {
            stopAutoScroll();
        }
    }

// Track mouse down on editor
    editor.getWrapperElement().addEventListener("mousedown", function(e) {
        if (e.target.closest('.CodeMirror')) {
            isSelecting = true;
            // Check immediately on mousedown in case we start at the edge
            setTimeout(() => checkAutoScroll(e), 0);
        }
    });

// Track mouse up globally
    document.addEventListener("mouseup", function() {
        isSelecting = false;
        stopAutoScroll();
    });

// Track mouse movement on editor
    editor.getWrapperElement().addEventListener("mousemove", checkAutoScroll);

// Stop scrolling when mouse leaves editor
    editor.getWrapperElement().addEventListener("mouseleave", function() {
        stopAutoScroll();
    });

// Additional: Check for auto-scroll during selection changes
// This catches cases where the selection extends to edges programmatically
    editor.on('beforeSelectionChange', function(cm, obj) {
        if (isSelecting) {
            // Small delay to let the selection update, then check mouse position
            setTimeout(() => {
                const mouseEvent = window.lastMouseEvent;
                if (mouseEvent) {
                    checkAutoScroll(mouseEvent);
                }
            }, 0);
        }
    });

// Track the last mouse event for reference
    document.addEventListener('mousemove', function(e) {
        window.lastMouseEvent = e;
    });
    // TODO Image uploading
    editor.on("paste", async (_, event) => {
        const items = (event.clipboardData || event.originalEvent.clipboardData).items;
        for (const item of items) {
            if (item.kind === "file" && item.type.startsWith("image/")) {
                const file = item.getAsFile();
                const fileName = `${new Date().toISOString().replace(/[:.]/g, '-')}.png`;
                await saveImageToDirectory(file, fileName);

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

    editor.addKeyMap({
        'Cmd-Y': function (cm) {
            cm.replaceSelection('✅ ');
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
                await showFile(node.parent.toString(), node.toString() + ".md");
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
                await showFile("", node.toString() + ".md");
            });
            root.addChild(fileNode)
        }
    }

    new TreeView(root, "#sidebar", {
        show_root: false,
    });
}

async function showRandomFile() {
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
        await showFile(randomFile.dir, randomFile.file);
    } catch (error) {
        console.error("Failed to open random file:", error);
    }
}

async function showFile(dir, filename, saveToHistory = true) {
    filename = filename.normalize("NFC");
    const fileData = files[dir][filename];

    // Check if we're loading the same file and save cursor position
    let cursorPos = null;
    if (editor.currentDir === dir && editor.currentFile === filename) {
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
    // Replace extended links with just link
    content = content.replace(/\[\[(.+?)\|.*?\]\]/g, '[[$1]]');

    editor.currentDir = dir;
    editor.currentFile = filename;
    if (saveToHistory) {
        const state = {dir: dir, file: filename};
        history.pushState(state, '');
    }

    editor.getDoc().setValue(content);
    editor.clearHistory();


    if (cursorPos !== null) {
        setTimeout(() => {
            editor.setCursor(cursorPos);
            editor.scrollIntoView(cursorPos, 500);
            // TODO only focus if there's no quick dialogue
            editor.focus();
        }, 300);
    } else {
        // Set cursor at the end of the page.
        // We need to execute this code after some rendering loop. If we don't do that,
        // Images and other heavy stuff won't be loaded
        // P.S. Is it try after we set infinite loading?
        setTimeout(() => {
            focusLastLine();
        }, 300);
    }
}

function focusLastLine() {
    const lastLine = editor.lastLine();
    let targetLine = lastLine;
    for (let i = lastLine; i >= 0; i--) {
        const lineContent = editor.getLine(i).trim();
        if (!lineContent.startsWith("[") && (!lineContent.endsWith("]") || !lineContent.endsWith(")"))) {
            targetLine = i;
            break;
        }
    }
    const targetChar = editor.getLine(targetLine).length;
    editor.setCursor({line: targetLine, ch: targetChar});
    // Why doing scroll to 0 line?
    editor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
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

document.addEventListener('keydown', (event) => {
    if (event.metaKey && event.key === 'p') {
        event.preventDefault();
        document.getElementById('search-input').value = ''
        openSearchModal();
    }

    if (event.metaKey && event.key === 'k') {
        event.preventDefault();
        document.getElementById('search-input').value = ''
        openSearchModal();
    }
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
            showFile(dir, filename);
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

function closeGoToFile() {
    document.getElementById('search').style.display = 'none';
}

document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
        closeGoToFile();
    }
});

// Toggle focus mode
document.addEventListener('keydown', function (event) {
    if ((event.altKey || event.metaKey) && event.key === 'Enter') {
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
        showFile(state['dir'], state['file'], false);
    }
});

document.getElementById('search').addEventListener('keydown', (event) => {
    const resultsList = document.getElementById('search-results').querySelectorAll('li');

    if (event.key === 'Enter') {
        event.preventDefault();
        if (resultsList[focusedItemIndex]) {
            const [dir, filename] = resultsList[focusedItemIndex].getAttribute('data-path').split('/');
            showFile(dir, filename);
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
    files = await loadLocalFiles(savedDirectoryHandle);
    console.log("Files loaded");
    await syncAllWithServer()
    console.log("Sync completed");
});