// HyperMD/Codemirror editor
let editor;
let tree;
let focusedMoveItemIndex = -1;
let isChat = false;
let isWelcome = false;
let debug = false;
// let debug = {dir: '', file: 'Sim.md', loaded: false};

const sidebar = document.getElementById('sidebar');
const sidebarContainer = document.getElementById('sidebar-container');
const content = document.getElementById('content')
const chat = document.getElementById('chat');
const chatInput = document.getElementById('chat-input');

async function init(el) {
    // Authorize if we have one-time token in URL.
    const urlParams = new URLSearchParams(window.location.search);
    const oneTimeToken = urlParams.get('token');
    if (oneTimeToken) {
        try {
            // Exchange one-time token for permanent token
            const response = await fetch('https://api.files.md/token', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    oneTimeToken: oneTimeToken
                })
            });

            if (response.ok) {
                const data = await response.json();
                localStorage.setItem('token', data.token);
                const url = new URL(window.location);
                url.searchParams.delete('token');
                window.history.replaceState({}, '', url);
            } else {
                alert('The token has expired or is invalid. Please try to request a new link.');
                console.error('Token exchange failed:', response.status);
            }
        } catch (error) {
            alert('The token has expired or is invalid. Please try to request a new link.');
            console.error('Error exchanging token:', error);
        }
    }

    initEditor(el);

    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
        document.getElementById('open-folder').style.display = 'inline';
        document.getElementById('new-file').style.display = 'none';
        document.getElementById('new-folder').style.display = 'none';
        document.getElementById('open-chat').style.display = 'none';
        files = DEFAULT_FILES;
        updateSidebar();
        await openFile('', 'Welcome.md');
        isWelcome = true;
        return;
    } else {
        isWelcome = false;
        document.getElementById('open-folder').style.display = 'none';
        document.getElementById('new-file').style.display = 'inline';
        document.getElementById('new-folder').style.display = 'inline';
        document.getElementById('open-chat').style.display = 'inline';
    }

    const permission = await savedDirHandle.queryPermission({mode: 'readwrite'});
    console.log('PERMISSION', permission);
    if (permission !== 'granted') {
        document.getElementById('open-folder').style.display = 'inline';
        document.getElementById('new-file').style.display = 'none';
        document.getElementById('new-folder').style.display = 'none';
        document.getElementById('open-chat').style.display = 'none';
        isWelcome = true;
    }

    const rootDirHandle = await getRootDirHandle();

    let perf = performance.now();
    files = await loadLocalFiles(rootDirHandle);
    console.log(`Files loaded in ${performance.now() - perf}ms`);

    initChat();
    initWasm();

    perf = performance.now();
    updateSidebar();
    console.log(`Sidebar built in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    // perf = performance.now();
    openChat();
    // console.log(`Random file opened in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

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
            name: 'hypermd',
            math: false, // disable $math syntax$
        },
        lineNumbers: false,
        extraKeys: {
            // 'Shift-Space': 'autocomplete',
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
    editor.setSize(null, '100%');

    editor.hmdResolveURL = function (path) {
        if (typeof path === 'undefined') {
            return path
        }

        path = path.replace(/%20/g, ' ');

        if (/^(?!http|https|\[).+\.md$/.test(path)) {
            let parts = path.split('/');
            if (parts.length === 1) {
                openFile('', path);
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
            await openFile('', path + '.md');
            return;
        }

        await openFile(parts[0], parts[1] + '.md');
    };

    editor.on('inputRead', async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '[') {
            editor.showHint({
                completeSingle: false, updateOnCursorActivity: true,
            })
        }
    })

    // Force '# ' to remain at first line.
    editor.on('change', function (cm, change) {
        if (change.from.line === 0) {
            const line = cm.getLine(0);
            if (!line.startsWith('# ')) {
                const content = line.replace(/^#*\s*/, '');
                cm.replaceRange('# ' + content, {line: 0, ch: 0}, {line: 0, ch: line.length});
            }
        }
    });

    initAutoscroll(editor);

    // Image upload
    editor.on('paste', async (_, event) => {
        const items = (event.clipboardData || event.originalEvent.clipboardData).items;
        for (const item of items) {
            if (item.kind === 'file' && item.type.startsWith('image/')) {
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
                        console.error('Failed to save the image.');
                        alert('Failed to save the image. Please try again.');
                    }
                } catch (error) {
                    console.error('Error saving image:', error);
                    alert('Error saving image: ' + error.message);
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

            const isBold = trimmedSelection.startsWith('**') && trimmedSelection.endsWith('**');

            let start = cm.getCursor('start');
            let end = cm.getCursor('end');

            if (isBold) {
                cm.replaceSelection(prefix + trimmedSelection.slice(2, -2) + suffix);
                cm.setSelection(
                    {line: start.line, ch: start.ch + prefix.length},
                    {line: end.line, ch: end.ch - suffix.length - 4}
                );
            } else {
                cm.replaceSelection(prefix + `**${trimmedSelection}**` + suffix);
                cm.setSelection(
                    {line: start.line, ch: start.ch + prefix.length},
                    {line: end.line, ch: end.ch - suffix.length + 4}
                );
            }
            cm.focus();
        },
        'Cmd-I': function (cm) {
            let selection = cm.getSelection();
            let trimmedSelection = selection.trim();
            let prefix = selection.slice(0, selection.indexOf(trimmedSelection));
            let suffix = selection.slice(selection.indexOf(trimmedSelection) + trimmedSelection.length);

            const isItalic = trimmedSelection.startsWith('*') && trimmedSelection.endsWith('*');

            let start = cm.getCursor('start');
            let end = cm.getCursor('end');

            if (isItalic) {
                cm.replaceSelection(prefix + trimmedSelection.slice(1, -1) + suffix);
                cm.setSelection(
                    {line: start.line, ch: start.ch + prefix.length},
                    {line: end.line, ch: end.ch - suffix.length - 2}
                );
            } else {
                cm.replaceSelection(prefix + `*${trimmedSelection}*` + suffix);
                cm.setSelection(
                    {line: start.line, ch: start.ch + prefix.length},
                    {line: end.line, ch: end.ch - suffix.length + 2}
                );
            }
            cm.focus();
        }
    });

    editor.getWrapperElement().addEventListener('mousedown', function (e) {
        if (!isMetaKey(e)) return;

        e.preventDefault();

        const code = e.target.closest('.cm-inline-code');
        if (!code) return;

        const text = code.textContent;
        navigator.clipboard.writeText(text);

        const toast = document.createElement('div');
        toast.textContent = 'Copied!';
        toast.style.cssText = `
            position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%);
            background: var(--color-bg-alt); color: var(--color-tx); padding: 8px 16px; border-radius: 5px;
            border: 1px solid var(--color-border);
            z-index: 9999; font-size: 14px;
        `;
        document.body.appendChild(toast);
        setTimeout(() => document.body.removeChild(toast), 1000);
    }, true);
}

async function initWasm() {
    console.log('INIT CHAT');
    const go = new Go();
    const wasmFile = await fetch('chat.wasm');
    const wasmModule = await WebAssembly.instantiateStreaming(wasmFile, go.importObject);
    go.run(wasmModule.instance);
    let cmd = {
        n: 'today',
        t: 'cmd'
    }
    replyCmd(JSON.stringify(cmd));

    window.reply = reply
    window.replyCmd = replyCmd
}

async function receive(val) {
    console.log(val);
    await loadData();
    renderMessages();
    scrollToBottom();
}

function createAutocompleteDict() {
    const dict = {};

    Object.keys(excludeDirs(SYSTEM_DIRS)).forEach(dir => {
        Object.keys(files[dir]).forEach(filename => {
            if (filename === CONFIG_FILENAME) {
                return;
            }
            const key = `${filename.replace(/\.md$/, '')}`;
            const filePath = `${filename.replace(/\.md$/, '')}](${dir}/${filename})`;
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
                await openFile(node.parent.toString(), node.toString() + '.md');
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

            let fileNode = new TreeNode(file.replace(/\.md$/, '').replace(/\.txt$/, ''), {expanded: false});
            fileNode.on('click', async function (n, node) {
                await openFile('', file);
            });
            root.addChild(fileNode);

            // Restore selected state for root-level file nodes
            if (selectedNodes.has(file.replace(/\.md$/, ''))) {
                fileNode.setSelected(true);
            }
        }
    }

    tree = new TreeView(root, '#sidebar-tree', {
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
        console.error('No files found to open.');
        return;
    }

    const randomFile = allFiles[Math.floor(Math.random() * allFiles.length)];

    try {
        await openFile(randomFile.dir, randomFile.file);
    } catch (error) {
        console.error('Failed to open random file:', error);
    }
}

async function openFile(dir, filename, saveToHistory = true) {
    await syncCurrentFile(false);

    if (dir === '' && filename === CHAT_FILENAME) {
        openChat();
        return;
    } else {
        const codemirror = document.querySelector('.CodeMirror-wrap');
        codemirror.style.display = 'block';
        chat.style.display = 'none';
        chatInput.style.display = 'none';
        isChat = false;
    }

    const start = performance.now();
    filename = filename.normalize('NFC');
    const fileData = files[dir][filename];

    // Check if we're loading the same file and save cursor position
    let cursorPos = null;
    if (editor.currentDir === dir && editor.currentFile === filename) {
        console.log('saving cursor');
        cursorPos = editor.getCursor();
    }

    const header = filename.replace(/\.md$/, '').replace(/^\w/, (c) => c.toUpperCase());
    let content = '';
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
    let selectedDirs = tree.getSelectedNodes();
    if (selectedDirs.length > 0 &&
        selectedDirs[0].getOptions &&
        typeof selectedDirs[0].getOptions === 'function' &&
        selectedDirs[0].getOptions()['dir'] === true) {
        dir = selectedDirs[0].toString();
    }
    // TODO don't create on disk?
    let filename = 'New file.md';

    let num = 1;
    while (files[dir] && files[dir][filename]) {
        filename = `New file (${num}).md`;
        num++;
    }

    let handle = await getFileHandle(toPath(dir, filename), true);
    addFileToMemory(dir, filename, {
        content: '',
        lastModified: 0,
        handle: handle,
        imageUrl: null
    });

    await openFile(dir, filename);
    editor.setCursor({line: 1, ch: 0});
    editor.focus();
    // editor.setSelection(
    //     {line: 0, ch: 2},
    //     {line: 0, ch: null}
    // );

    await updateSidebar();
}

async function newFolder() {
    let folderName = prompt('Enter folder name:', 'New Folder');
    if (folderName === null) {
        return;
    }

    folderName = folderName.trim();
    if (!folderName) {
        alert('Folder name cannot be empty');
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
        if (lineContent === '') {
            lastLine--;
            continue;
        }

        lastLine = Math.min(lastLine + 1, editor.lastLine());
        break;
    }
    for (let i = lastLine; i >= 0; i--) {
        const lineContent = editor.getLine(i).trim();
        if (!lineContent.startsWith('[') && (!lineContent.endsWith(']') || !lineContent.endsWith(')'))) {
            targetLine = i;
            break;
        }
    }
    const targetChar = editor.getLine(targetLine).length;
    editor.setCursor({line: targetLine, ch: targetChar});
    // Cursor at the end, but scroll the doc to top
    editor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
    editor.focus();
}


function updateMoveFocusedItem(resultsList) {
    document.querySelectorAll('#move-results li').forEach(li => li.classList.remove('focused'));
    resultsList.forEach((item, index) => {
        if (index === focusedMoveItemIndex) {
            item.classList.add('focused');
            item.scrollIntoView({block: 'nearest'});
        } else {
            item.classList.remove('focused');
        }
    });
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

function isMetaKey(event) {
    return event.metaKey || event.ctrlKey || event.altKey;
}

// Hotkeys
window.addEventListener('keydown', async (event) => {
    if (isMetaKey(event) && event.key === 'p') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('search-input').value = ''
        searchModal.open();
    }

    if (isMetaKey(event) && event.key === 'k') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('search-input').value = ''
        searchModal.open();
    }

    if (isMetaKey(event) && event.key === 'm') {
        event.preventDefault();
        event.stopPropagation();
        document.getElementById('move-input').value = ''
        openMoveModal();
    }

    if (isMetaKey(event) && event.key === 'd') {
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
        openChat();
        await updateSidebar();
    }

    if (isMetaKey(event) && event.key === 'n') {
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

function closeMoveModal() {
    document.getElementById('move').style.display = 'none';
}

function getMoveDestinations() {
    let dirs = ['/'];
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
            console.log('CLICKED ON folder to move', dataDir);
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

function closeMove() {
    document.getElementById('move').style.display = 'none';
}

document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
        closeMove();
    }
});

function openChat() {
    chatInput.focus();
    if (isChat) {
        return;
    }

    const codemirror = document.querySelector('.CodeMirror-wrap');
    codemirror.style.display = 'none';
    chat.style.display = 'flex';
    chatInput.style.display = 'block';

    chatInput.focus();
    isChat = true;
}

function openBot() {
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
        t: 'cmd'
    }
    replyCmd(JSON.stringify(cmd));

    window.resizeTo(520, 530);
    const left = (screen.availWidth - 500) / 2;
    const top = (screen.availHeight - 500) / 2;
    window.moveTo(left, top);
}


async function switchChat() {

}

// Toggle focus mode
document.addEventListener('keydown', function (event) {
    if (isMetaKey(event) && event.key === 'Enter') {
        event.preventDefault();
        openChat();

        // const sidebar = document.getElementById('sidebar');
        // if (sidebar.style.display === 'none') {
        //     sidebar.style.display = 'block';
        // } else {
        //     sidebar.style.display = 'none'; // Hide the sidebar
        // }
    }
});

document.addEventListener('keydown', (e) => {
    if (e.metaKey || e.ctrlKey) {
        document.body.classList.add('cmd-pressed');
    }
});

document.addEventListener('keyup', (e) => {
    if (!e.metaKey && !e.ctrlKey) {
        document.body.classList.remove('cmd-pressed');
    }
});

window.addEventListener('popstate', (event) => {
    const state = event.state;
    if (state) {
        openFile(state['dir'], state['file'], false);
    }
});

document.getElementById('move').addEventListener('keydown', (event) => {
    const resultsList = document.getElementById('move-results').querySelectorAll('li');

    if (event.key === 'Enter') {
        event.preventDefault();
        if (resultsList[focusedMoveItemIndex]) {
            const dir = resultsList[focusedMoveItemIndex].getAttribute('data-path');
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
    let dirHandle = await window.showDirectoryPicker({'mode': 'readwrite'});
    document.getElementById('open-folder').style.display = 'none';
    document.getElementById('new-file').style.display = 'inline';
    document.getElementById('new-folder').style.display = 'inline';
    document.getElementById('open-chat').style.display = 'inline';

    await saveDirectoryHandle(dirHandle);
    files = await loadLocalFiles(dirHandle)

    initWasm();

    // Create welcome markdown file if empty
    if (Object.keys(files).length === 0) {
        const hotkeysFilename = '🎹 Hotkeys.md';
        await saveTextFile(hotkeysFilename, HOTKEYS_CONTENT);
        files[''] = {};
        files[''][hotkeysFilename] = {
            lastModified: 0,
            handle: await getFileHandle(hotkeysFilename),
        }

        const welcomeFilename = '🪴 Welcome.md';
        await saveTextFile(welcomeFilename, WELCOME_CONTENT);
        files[''][welcomeFilename] = {
            lastModified: 0,
            handle: await getFileHandle(welcomeFilename),
        }
        await openFile('', welcomeFilename);
        isWelcome = false;
        updateSidebar();
        return;
    }

    isWelcome = false;
    updateSidebar();
    await openChat();
}

function getCurrentContent() {
    let content = editor.getValue();
    const header = toHeader(editor.currentFile);
    if (content.toLowerCase().startsWith(`${header}`.toLowerCase())) {
        content = content.slice(`${header}\n`.length);
    } else if (content.toLowerCase().startsWith('# ')) {
        content = content.slice(`$# `.length);
    }

    return content;
}

function toHeader(filename) {
    const title = ucfirst(filename.replace(/\.md$/, ''));
    return `# ${title}`;
}

function fromHeaderToFilename(header) {
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
    return text.replace(/\r\n|\r/g, '\n');
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

// document.addEventListener('mousedown', (event) => {
//     const goToFile = document.getElementById('search');
//     if (goToFile.style.display === 'block' &&
//         !goToFile.contains(event.target)) {
//         closeSearchModal();
//     }
// });

// Reload files once the app gains focus.
window.addEventListener('focus', async () => {
    // We don't want to do heavy stuff when chat is open.
    if (isChat || isWelcome) {
        return false;
    }

    console.log('FOCUS');

    if (editor.currentFile === undefined) {
        return;
    }

    // editor.focus();
    // focus chat-input
    document.getElementById('chat-input').focus();

    const savedDirectoryHandle = await getRootDirHandle();
    // check if granted

    // Sync media first, so that new images for current file would be loaded
    await syncMedia();
    await syncCurrentFile();

    const start = performance.now();
    files = await loadLocalFiles(savedDirectoryHandle);
    const end = performance.now();
    console.log(`Files loaded in: ${(end - start).toFixed(3)} milliseconds`);
    await syncTextsWithServer()
    await updateSidebar();
    console.log('Sync completed');
});

// Sync files on chat focus lose.
window.addEventListener('blur', async function () {
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
    console.log('Sync completed');
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

    // TODO uncomment, we won't this work only if focus on editor or sidebar, not in dialogs or input fields
    // if (isMetaKey(e) && e.key === 'a') {
    //     e.preventDefault();
    //     e.stopPropagation();
    //
    //     // Select all except the first line
    //     const lastLine = editor.lastLine();
    //     const lastLineLength = editor.getLine(lastLine).length;
    //
    //     editor.getDoc().setSelection(
    //         {line: 1, ch: 0},                    // anchor
    //         {line: lastLine, ch: lastLineLength}, // head
    //         {scroll: false}  // don't scroll to cursor
    //     );
    // }
}, true);