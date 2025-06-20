// HyperMD/Codemirror editor
let editor;
let tree;
let focusedSearchItemIndex = -1;
let focusedMoveItemIndex = -1;
let isChat = false;
let isWelcome = false;
let debug = false;
// let debug = {dir: "", file: "Sim.md", loaded: false};

const sidebar = document.getElementById('sidebar');
const sidebarContainer = document.getElementById('sidebar-container');
const content = document.getElementById('content')

async function init(el) {
    initEditor(el);

    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
        document.getElementById('open-folder').style.display = 'inline';
        document.getElementById('new-file').style.display = 'none';
        document.getElementById('new-folder').style.display = 'none';
        document.getElementById('chat').style.display = 'none';
        files = defaultFiles;
        updateSidebar();
        await openFile("", "Welcome.md");
        isWelcome = true;
        return;
    } else {
        isWelcome = false;
        document.getElementById('open-folder').style.display = 'none';
        document.getElementById('new-file').style.display = 'inline';
        document.getElementById('new-folder').style.display = 'inline';
        document.getElementById('chat').style.display = 'inline';
    }

    const permission = await savedDirHandle.queryPermission({mode: 'read'});
    if (permission !== 'granted') {
        document.getElementById('open-folder').style.display = 'inline';
        document.getElementById('new-file').style.display = 'none';
        document.getElementById('new-folder').style.display = 'none';
        document.getElementById('chat').style.display = 'none';
        isWelcome = true;
    }

    const rootDirHandle = await getRootDirHandle();

    let perf = performance.now();
    files = await loadLocalFiles(rootDirHandle);
    console.log(`Files loaded in ${performance.now() - perf}ms`);

    initChat();

    perf = performance.now();
    updateSidebar();
    console.log(`Sidebar built in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    perf = performance.now();
    await showRandomFile();
    console.log(`Random file opened in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    perf = performance.now();
    await syncTextsWithServer();
    await updateSidebar();
    await syncMedia();
    console.log(`Files initialized in: ${(performance.now() - perf).toFixed(3)} milliseconds`);
}

function initEditor(el) {
    if (editor !== undefined) {
        editor.off();
        const wrapper = editor.getWrapperElement();
        if (wrapper && wrapper.parentNode) {
            wrapper.parentNode.removeChild(wrapper);
        }
    }

    editor = HyperMD.fromTextArea(el, {
        dragDrop: false,
        viewportMargin: Infinity,
        mode: {
            name: "hypermd",
            math: false, // disable $math syntax$
        },
        lineNumbers: false,
        extraKeys: {
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
        },
        configureMouse: () => ({addNew: false}) // disable multicursor
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

        const match = path.match(/^media\/(.+\.(png|jpg|jpeg|gif|webp))$/i);

        if (match && files['media'] && files['media'][match[1]]) {
            return files['media'][match[1]].imageUrl;
        }

        return path;
    };

    editor.hmdReadLink = async function (path) {
        path = path.replace(/\|.*]$/, '');
        path = path.replace('[', '').replace(']', '');

        // If it is a web link open window blank
        if (/^(http|https):\/\//.test(path)) {
            window.open(path, '_blank');
            return;
        }

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

    initAutoscroll(editor);

    // Image upload
    editor.on("paste", async (_, event) => {
        const items = (event.clipboardData || event.originalEvent.clipboardData).items;
        for (const item of items) {
            if (item.kind === "file" && item.type.startsWith("image/")) {
                event.preventDefault(); // Prevent default paste behavior

                const file = item.getAsFile();
                const fileName = `${new Date().toISOString().replace(/[:.]/g, '-')}.${getImageExtension(item.type)}`;

                try {
                    const fileHandle = await saveImageFile(fileName, file);
                    if (fileHandle) {
                        if (!files['media']) {
                            files['media'] = {};
                        }
                        files['media'][fileName] = {
                            handle: fileHandle,
                            lastModified: Date.now(),
                            imageUrl: URL.createObjectURL(file)
                        };

                        const markdownImageSyntax = `![](media/${fileName})`;
                        editor.replaceSelection(markdownImageSyntax);
                        console.log(`Image saved as: ${fileName}`);
                    } else {
                        console.error("Failed to save the image.");
                        alert("Failed to save the image. Please try again.");
                    }
                } catch (error) {
                    console.error("Error saving image:", error);
                    alert("Error saving image: " + error.message);
                }
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

async function initChat() {
    const go = new Go();
    const wasmFile = await fetch("main.wasm");
    const wasmModule = await WebAssembly.instantiateStreaming(wasmFile, go.importObject);
    go.run(wasmModule.instance);
    let cmd = {
        n: 'today',
        t: "cmd"
    }
    replyCmd(JSON.stringify(cmd));

    window.reply = reply
    window.replyCmd = replyCmd
}

function createAutocompleteDict() {
    const dict = {};

    Object.keys(excludeDirs(SYSTEM_DIRS)).forEach(dir => {
        Object.keys(files[dir]).forEach(filename => {
            const key = `${filename.replace(/\.md$/, "")}`;
            const filePath = `${filename.replace(/\.md$/, "")}](${dir}/${filename})`;
            dict[key] = filePath;
        });
    });

    return dict;
}

function updateSidebar(focusDir = '') {
    let expandedDirs = new Set();
    let selectedNodes = new Set();

    if (tree) {
        // Save state for all nodes (both directories and files)
        function saveNodeState(node) {
            if (node.isExpanded()) {
                expandedDirs.add(node.toString());
            }
            if (node.isSelected()) {
                selectedNodes.add(node.toString());
            }

            // Recursively save state for child nodes
            if (node.getChildren) {
                node.getChildren().forEach(child => {
                    saveNodeState(child);
                });
            }
        }

        tree.getRoot().getChildren().forEach(child => {
            saveNodeState(child);
        });
    }

    root = new TreeNode('');

    // Process directories
    for (const dir in files) {
        if (dir === '' || dir === 'media') {
            continue;
        }

        let dirNode = new TreeNode(dir, {expanded: false, dir: true});

        // Process files in directory
        for (let file in files[dir]) {
            let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await openFile(node.parent.toString(), node.toString() + ".md");
            });
            dirNode.addChild(fileNode);

            // Restore selected state for file nodes
            if (selectedNodes.has(file.replace(/\.md$/, ''))) {
                fileNode.setSelected(true);
            }
        }

        root.addChild(dirNode);

        // Handle focus directory or restore previous state
        if (dir === focusDir) {
            dirNode.setExpanded(true);
            dirNode.setSelected(true);
        } else {
            if (expandedDirs.has(dir)) dirNode.setExpanded(true);
            if (selectedNodes.has(dir)) dirNode.setSelected(true);
        }
    }

    // Process root-level files
    if (files['']) {
        for (let file in files['']) {
            if (file === CONFIG_FILENAME) {
                continue;
            }

            let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await openFile("", file);
            });
            root.addChild(fileNode);

            // Restore selected state for root-level file nodes
            if (selectedNodes.has(file.replace(/\.md$/, ''))) {
                fileNode.setSelected(true);
            }
        }
    }

    tree = new TreeView(root, "#sidebar-tree", {
        show_root: false,
    });
}

async function showRandomFile() {
    if (debug) {
        await openFile(debug.dir, debug.file);
        return;
    }

    const allFiles = [];
    for (let dir in excludeDirs(SYSTEM_DIRS)) {
        for (let file in files[dir]) {
            if (file === CONFIG_FILENAME) {
                continue;
            }

            allFiles.push({dir, file});
        }
    }

    if (allFiles.length === 0) {
        console.error("No files found to open.");
        return;
    }

    const randomFile = allFiles[Math.floor(Math.random() * allFiles.length)];
    console.log(randomFile);

    try {
        await openFile(randomFile.dir, randomFile.file);
    } catch (error) {
        console.error("Failed to open random file:", error);
    }
}

async function openFile(dir, filename, saveToHistory = true) {
    await syncCurrentFile(false);

    const start = performance.now();
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
        content = `# ${header}\n${content}`;
    } else {
        // We use welcome's files
        content = fileData.content;
    }

    editor.currentDir = dir;
    editor.currentFile = filename;
    // TODO disable when syncing?
    if (saveToHistory) {
        const state = {dir: dir, file: filename};
        history.pushState(state, '');
    }

    initEditor(document.getElementById('editor'));
    editor.currentDir = dir;
    editor.currentFile = filename;
    editor.getDoc().setValue(content);
    editor.clearHistory();
    editor.markClean();

    if (cursorPos !== null) {
        console.log('cursor not null');
        editor.setCursor(cursorPos);
        editor.scrollIntoView(cursorPos, 500);
        // TODO only focus if there's no quick dialogue
        editor.focus();
    } else {
        focusLastLine();
    }

    const end = performance.now();
    console.log(`File opened in: ${(end - start).toFixed(3)} milliseconds`);
    // Get the editor instance
}

async function newFile() {
    let dir = editor.currentDir || '';
    // let selectedDirs = tree.getSelectedNodes();
    // if (selectedDirs.length > 0) {
    //     dir = selectedDirs[0].toString();
    // }
    // TODO don't create on disk?
    let filename = "New file.md";

    let num = 1;
    while (files[dir] && files[dir][filename]) {
        filename = `New file (${num}).md`;
        num++;
    }

    let handle = await getFileHandle(toPath(dir, filename), true);
    addFileToMemory(dir, filename, {
        content: "",
        lastModified: 0,
        handle: handle,
        imageUrl: null
    });

    await openFile(dir, filename);
    editor.setCursor({line: 0, ch: 0});
    editor.focus();
    editor.setSelection(
        {line: 0, ch: 2},
        {line: 0, ch: null}
    );

    await updateSidebar();
}

async function newFolder() {
    let folderName = prompt("Enter folder name:", "New Folder");
    if (folderName === null) {
        return;
    }

    folderName = folderName.trim();
    if (!folderName) {
        alert("Folder name cannot be empty");
        return;
    }

    let finalFolderName = folderName;
    let num = 1;
    while (files[finalFolderName]) {
        finalFolderName = `${folderName} (${num})`;
        num++;
    }

    const rootDirHandle = await getRootDirHandle();
    await rootDirHandle.getDirectoryHandle(finalFolderName, {create: true});
    files[finalFolderName] = {};

    console.log('CREATED folder', finalFolderName);

    await updateSidebar(finalFolderName);
}

// Focus last line before the links.
function focusLastLine() {
    let lastLine = editor.lastLine();
    let targetLine = lastLine;

    // Eat all empty lines before first links.
    while (lastLine >= 0) {
        const lineContent = editor.getLine(lastLine).trim();
        if (lineContent === "") {
            lastLine--;
            continue;
        }

        lastLine = Math.min(lastLine + 1, editor.lastLine());
        break;
    }
    for (let i = lastLine; i >= 0; i--) {
        const lineContent = editor.getLine(i).trim();
        if (!lineContent.startsWith("[") && (!lineContent.endsWith("]") || !lineContent.endsWith(")"))) {
            targetLine = i;
            break;
        }
    }
    console.log(targetLine);
    const targetChar = editor.getLine(targetLine).length;
    editor.setCursor({line: targetLine, ch: targetChar});
    // Cursor at the end, but scroll the doc to top
    editor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
    editor.focus();
}

function updateSearchFocusedItem(resultsList) {
    document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
    resultsList.forEach((item, index) => {
        if (index === focusedSearchItemIndex) {
            item.classList.add('focused');
            item.scrollIntoView({block: "nearest"});
        } else {
            item.classList.remove('focused');
        }
    });
}

function updateMoveFocusedItem(resultsList) {
    document.querySelectorAll('#move-results li').forEach(li => li.classList.remove('focused'));
    resultsList.forEach((item, index) => {
        if (index === focusedMoveItemIndex) {
            item.classList.add('focused');
            item.scrollIntoView({block: "nearest"});
        } else {
            item.classList.remove('focused');
        }
    });
}

function openSearchModal(text = '') {
    document.getElementById('search').style.display = 'block';
    const inputField = document.getElementById('search-input');
    inputField.value = text;
    inputField.focus();

    focusedSearchItemIndex = -1;
    const goToFileResults = document.getElementById('search-results');
    goToFileResults.innerHTML = '';
    if (text === '') {
        loadRecentFiles();
    } else {
        search();
    }
}

function openMoveModal() {
    document.getElementById('move').style.display = 'block';
    const inputField = document.getElementById('move-input');
    inputField.focus();

    focusedMoveItemIndex = -1;
    const goToFileResults = document.getElementById('move-results');
    goToFileResults.innerHTML = '';
    showMoveResults(getMoveDestinations());
}

function isModifierKey(event) {
    return event.metaKey || event.ctrlKey || event.altKey;
}

// Hotkeys
window.addEventListener('keydown', async (event) => {
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

    if (isModifierKey(event) && event.key === 'd') {
        event.preventDefault();
        event.stopPropagation();

        let path = toPath(editor.currentDir, editor.currentFile);
        let dir = editor.currentDir;
        let filename = editor.currentFile;
        editor.currentDir = undefined;
        editor.currentFile = undefined;
        await removeFile(path);
        // Remove from files object
        delete files[dir][filename];
        await showRandomFile();
        await updateSidebar();
    }

    if (isModifierKey(event) && event.key === 'k') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('search-input').value = ''
        openSearchModal();
    }

    if (isModifierKey(event) && event.key === 'n') {
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

function closeSearchModal() {
    document.getElementById('search').style.display = 'none';
}

function closeMoveModal() {
    document.getElementById('move').style.display = 'none';
}

function loadRecentFiles() {
    let results = [];
    for (const dir of Object.keys(excludeDirs(SYSTEM_DIRS))) {
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
        if (dir === '' || dir === 'media') {
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

    if (search.endsWith('/')) {
        const folderName = search.slice(0, -1); // Remove the trailing slash

        // Check if the folder exists in files
        if (files[folderName]) {
            const list = document.getElementById('search-results');
            list.innerHTML = '';

            // Get all files from the specified folder
            const folderResults = [];
            for (const filename in files[folderName]) {
                folderResults.push({
                    filename: filename,
                    dir: folderName,
                    score: 100 // Give max score since it's an exact folder match
                });
            }

            showSearchResults(folderResults);
            return;
        }
    }

    let results = [];
    const lowPriorityDirs = ["archive", "_read_", "_watch_", "_shop_", "habits", "triggers", "today", "later"];

    // Levenshtein distance
    for (const dir in excludeDirs(SYSTEM_DIRS)) {
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
        if (filename === CONFIG_FILENAME) {
            return;
        }

        const listItem = document.createElement('li');
        let title = filename.replace(/\.md$/, "")
        if (dir !== '') {
            listItem.textContent = `${dir}/${title}`;
        } else {
            listItem.textContent = title;
        }
        listItem.setAttribute('data-path', `${dir}/${filename}`);
        listItem.setAttribute('data-index', index);
        listItem.onclick = async () => {
            openEditor(!isChat);
            await openFile(dir, filename);
            closeSearchModal();
        };
        listItem.onmouseenter = () => {
            document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
            listItem.classList.add('focused');
            focusedSearchItemIndex = index;
        };
        list.appendChild(listItem);
    });

    focusedSearchItemIndex = 0;
    updateSearchFocusedItem(list.querySelectorAll('li'));
}

function showMoveResults(dirs) {
    const list = document.getElementById('move-results');
    list.innerHTML = '';
    dirs.forEach((dir, index) => {
        let dataDir = dir;
        if (dataDir === '/') {
            dataDir = '';
        }
        const listItem = document.createElement('li');
        listItem.textContent = dir;
        listItem.setAttribute('data-path', dataDir);
        listItem.setAttribute('data-index', index);
        listItem.onclick = async () => {
            console.log("CLICKED ON folder to move", dataDir);
            await moveCurrentFile(dataDir);
            closeMoveModal();
        };
        listItem.onmouseenter = () => {
            document.querySelectorAll('#move-results li').forEach(li => li.classList.remove('focused'));
            listItem.classList.add('focused');
            focusedMoveItemIndex = index;
        };
        list.appendChild(listItem);
    });

    focusedMoveItemIndex = 0;
    updateMoveFocusedItem(list.querySelectorAll('li'));
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

async function openEditor(withSidebar = true) {
    if (!isChat) {
        return true;
    }

    if (withSidebar) {
        sidebarContainer.style.display = 'block';
        window.resizeTo(screen.availWidth, screen.availHeight);
        window.moveTo(0, 0);

        setTimeout(async () => {
            const rootDirHandle = await getRootDirHandle();
            files = await loadLocalFiles(rootDirHandle);
            updateSidebar();
        }, 1);
    }
    content.style.display = 'block';
    chatContainer.style.display = 'none';
    isChat = false;
    editor.focus();
}

function openChat() {
    if (isChat) {
        return;
    }

    sidebarContainer.style.display = 'none';
    content.style.display = 'none';
    chatContainer.style.display = 'flex';
    input.focus();
    isChat = true;

    let cmd = {
        n: 'today',
        t: "cmd"
    }
    replyCmd(JSON.stringify(cmd));

    window.resizeTo(520, 530);
    const left = (screen.availWidth - 500) / 2;
    const top = (screen.availHeight - 500) / 2;
    window.moveTo(left, top);
}


async function switchChat() {
    if (isChat) {
        openEditor();
        return;
    }

    openChat();
}

// Toggle focus mode
document.addEventListener('keydown', function (event) {
    if (isModifierKey(event) && event.key === 'Enter') {
        event.preventDefault();
        switchChat();

        // const sidebar = document.getElementById('sidebar');
        // if (sidebar.style.display === 'none') {
        //     sidebar.style.display = 'block';
        // } else {
        //     sidebar.style.display = 'none'; // Hide the sidebar
        // }
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
        if (resultsList[focusedSearchItemIndex]) {
            const [dir, filename] = resultsList[focusedSearchItemIndex].getAttribute('data-path').split('/');
            openEditor(!isChat);
            openFile(dir, filename);
            closeSearchModal();
        }
    }

    if (event.key === 'ArrowDown') {
        event.preventDefault();
        focusedSearchItemIndex = (focusedSearchItemIndex + 1) % resultsList.length;
        updateSearchFocusedItem(resultsList);
    } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        focusedSearchItemIndex = (focusedSearchItemIndex - 1 + resultsList.length) % resultsList.length;
        updateSearchFocusedItem(resultsList);
    }
});

document.getElementById('move').addEventListener('keydown', (event) => {
    const resultsList = document.getElementById('move-results').querySelectorAll('li');

    if (event.key === 'Enter') {
        event.preventDefault();
        if (resultsList[focusedMoveItemIndex]) {
            const dir = resultsList[focusedMoveItemIndex].getAttribute('data-path');
            console.log("HERE, ", dir, resultsList);
            moveCurrentFile(dir);
            closeMoveModal();
        }
    }

    if (event.key === 'ArrowDown') {
        event.preventDefault();
        focusedMoveItemIndex = (focusedMoveItemIndex + 1) % resultsList.length;
        updateMoveFocusedItem(resultsList);
    } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        focusedMoveItemIndex = (focusedMoveItemIndex - 1 + resultsList.length) % resultsList.length;
        updateMoveFocusedItem(resultsList);
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
    document.getElementById('open-folder').style.display = 'none';
    document.getElementById('new-file').style.display = 'inline';
    document.getElementById('new-folder').style.display = 'inline';
    document.getElementById('chat').style.display = 'inline';
    isWelcome = false;
    await saveDirectoryHandle(dirHandle);
    files = await loadLocalFiles(dirHandle)
    updateSidebar();
    await showRandomFile();
}

function getCurrentContent() {
    let content = editor.getValue();
    const header = toHeader(editor.currentFile);
    if (content.toLowerCase().startsWith(`${header}`.toLowerCase())) {
        content = content.slice(`${header}\n`.length);
    }

    return content;
}

function toHeader(filename) {
    const title = ucfirst(filename.replace(/\.md$/, ''));
    return `# ${title}`;
}

function fromHeader(header) {
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

// Reload files once the app gains focus.
window.addEventListener('focus', async () => {
    // We don't want to do heavy stuff when chat is open.
    if (isChat || isWelcome) {
        return false;
    }

    console.log("FOCUS");

    if (editor.currentFile === undefined) {
        return;
    }

    editor.focus();

    // Sync media first, so that new images for current file would be loaded
    await syncMedia();
    await syncCurrentFile();

    const savedDirectoryHandle = await getRootDirHandle();

    // Benchmark time took
    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle);
    const end = performance.now();
    console.log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncTextsWithServer()
    await updateSidebar();
    console.log("Sync completed");
});

// Sync files on chat focus lose.
window.addEventListener('blur', async function() {
    console.log('Window lost focus');
    if (!isChat) {
        return;
    }

    // Sync media first, so that new images for current file would be loaded
    await syncMedia();
    await syncCurrentFile();

    const savedDirectoryHandle = await getRootDirHandle();

    // Benchmark time took
    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle);
    const end = performance.now();
    console.log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncTextsWithServer()
    await updateSidebar();
    console.log("Sync completed");
});


const resizeHandle = document.querySelector('.resize-handle');
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

    const width = e.clientX;
    const minWidth = 200;
    const maxWidth = 600;

    const constrainedWidth = Math.min(Math.max(width, minWidth), maxWidth);
    sidebar.style.width = constrainedWidth + 'px';
}

function stopResize() {
    if (!isResizing) return;
    isResizing = false;
    document.body.classList.remove('dragging');
}

document.addEventListener('keydown', (e) => {
    // If search or move dialog is focused - return
    if (document.getElementById('search').style.display === 'block' ||
        document.getElementById('move').style.display === 'block') {
        return;
    }

    if (isChat) {
        return;
    }
    if (isModifierKey(e) && e.key === 'a') {
        e.preventDefault();
        e.stopPropagation();
        editor.execCommand('selectAll');
    }
}, true);