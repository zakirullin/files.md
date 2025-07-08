// HyperMD/Codemirror editor
// let editor;
// let editor2;
let tree;
let isChat = false;
let isWelcome = false;
let debug = false;
// let debug = {dir: '', file: 'Sim.md', loaded: false};

const sidebar = document.getElementById('sidebar');
const sidebarContainer = document.getElementById('sidebar-container');
const content = document.getElementById('content')

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

    const savedDirHandle = await getRootDirHandle();
    const hasSavedDir = savedDirHandle instanceof FileSystemDirectoryHandle;
    if (!hasSavedDir) {
        document.getElementById('open-folder').style.display = 'inline';
        document.getElementById('new-file').style.display = 'none';
        document.getElementById('new-folder').style.display = 'none';
        document.getElementById('open-chat').style.display = 'none';
        document.getElementById('open-chat-modal').style.display = 'none';
        files = DEFAULT_FILES;
        isWelcome = true;
        renderSidebar();
        await openFile('', 'Welcome.md');
        return;
    } else {
        isWelcome = false;
        document.getElementById('open-folder').style.display = 'none';
        document.getElementById('new-file').style.display = 'inline';
        document.getElementById('new-folder').style.display = 'inline';
        document.getElementById('open-chat').style.display = 'inline';
        document.getElementById('open-chat-modal').style.display = 'inline';
    }

    const permission = await savedDirHandle.queryPermission({mode: 'readwrite'});
    console.log('PERMISSION', permission);
    if (permission !== 'granted') {
        document.getElementById('open-folder').style.display = 'inline';
        document.getElementById('new-file').style.display = 'none';
        document.getElementById('new-folder').style.display = 'none';
        document.getElementById('open-chat').style.display = 'none';
        document.getElementById('open-chat-modal').style.display = 'none';
        isWelcome = true;
    }

    const rootDirHandle = await getRootDirHandle();

    let perf = performance.now();
    files = await loadLocalFiles(rootDirHandle);
    console.log(`Files loaded in ${performance.now() - perf}ms`);

    initChat();
    initWasm();

    perf = performance.now();
    renderSidebar();
    console.log(`Sidebar built in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    // perf = performance.now();
    openChat();
    // showRandomFile();
    // console.log(`Random file opened in: ${(performance.now() - perf).toFixed(3)} milliseconds`);

    perf = performance.now();
    await syncTextsWithServer();
    await renderSidebar();
    await syncMedia();
    console.log(`Files initialized in: ${(performance.now() - perf).toFixed(3)} milliseconds`);
}

function initEditor(el) {
    if (window.editor !== undefined && el.id === 'editor-textarea' ) {
        editor.off();
        const wrapper = editor.getWrapperElement();
        if (wrapper && wrapper.parentNode) {
            wrapper.parentNode.removeChild(wrapper);
        }

        editor2.off();
        const wrapper2 = editor2.getWrapperElement();
        if (wrapper2 && wrapper2.parentNode) {
            wrapper2.parentNode.removeChild(wrapper2);
        }
    } else if (window.editor2 !== undefined && el.id === 'editor2-textarea') {
        editor2.off();
        const wrapper = editor2.getWrapperElement();
        if (wrapper && wrapper.parentNode) {
            wrapper.parentNode.removeChild(wrapper);
        }
    }

    let newEditor = HyperMD.fromTextArea(el, {
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
    newEditor.setSize(null, '100%');
    newEditor.on('focus', function() {
        currentEditor = newEditor;
        currentEditor.refresh(); // Cursor & hide tokens conflict if we don't call it
        closeChatModal();
        console.log('Focused to:', newEditor.currentFile);
    });

    newEditor.hmdResolveURL = function (path) {
        if (typeof path === 'undefined') {
            return path
        }

        path = path.replace(/%20/g, ' ');

        // TODO really dirty fix for links like:
        // ../media/image.png, remove
        if (path.startsWith('../')) {
            path = path.replace('../', '');
        }

        if (/^(?!http|https|\[).+\.md$/.test(path)) {
            let parts = path.split('/');
            if (parts.length === 1) {
                openFile('', path, true, 'editor2-textarea');
                return;
            }
            openFile(parts[0], parts[1], true, 'editor2-textarea');
            return path;
        }

        // TODO support other than media and img folders
        const match = path.match(/\/(.+\.(png|jpg|jpeg|gif|webp))$/i);

        if (match && files['media'] && files['media'][match[1]]) {
            return files['media'][match[1]].imageUrl;
        }

        if (match && files['img'] && files['img'][match[1]]) {
            return files['img'][match[1]].imageUrl;
        }

        return path;
    };

    newEditor.hmdReadLink = async function (path) {
        path = path.replace(/\|.*]$/, '');
        path = path.replace('[', '').replace(']', '');

        // If it is a web link open window blank
        if (/^(http|https):\/\//.test(path)) {
            window.open(path, '_blank');
            return;
        }

        let parts = path.split('/');
        if (parts.length === 1) {
            path += '.md';
            // Does file exist in root dir?
            if (files[''] && files[''][path]) {
                openFile('', path, true, 'editor2-textarea');
                return;
            }

            // Does file exist in current dir?
            if (files[editor.currentDir] && files[editor.currentDir][path]) {
                openFile(editor.currentDir, path, true, 'editor2-textarea');
                return;
            }

            // Loop through all 1st level dirs to find
            for (const dir in files) {
                if (files[dir][path]) {
                    openFile(dir, path, true, 'editor2-textarea');
                    return;
                }
            }

            return;
        }

        await openFile(parts[0], parts[1] + '.md', true, 'editor2-textarea');
    };

    newEditor.on('inputRead', async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '[') {
            cm.showHint({
                completeSingle: false, updateOnCursorActivity: true,
            })
        }
    })

    // Force '# ' to remain at first line.
    newEditor.on('change', function (cm, change) {
        if (change.from.line === 0) {
            const line = cm.getLine(0);
            if (!line.startsWith('# ')) {
                const content = line.replace(/^#*\s*/, '');
                cm.replaceRange('# ' + content, {line: 0, ch: 0}, {line: 0, ch: line.length});
            }
        }
    });

    // Image upload
    newEditor.on('paste', async (_, event) => {
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
                        currentEditor.replaceSelection(markdownImageSyntax);
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
    newEditor.addKeyMap({
        'Cmd-A': function (cm) {
            const cursor = cm.getCursor();

            // If cursor is on the first line, select all text in that line
            if (cursor.line === 0) {
                const lineLength = cm.getLine(0).length;
                cm.setSelection(
                    {line: 0, ch: 0},
                    {line: 0, ch: lineLength}
                );
                return;
            }

            // Otherwise, use default Cmd-A behavior (select all except first line)
            const lastLine = cm.lastLine();
            const lastLineLength = cm.getLine(lastLine).length;

            cm.setSelection(
                {line: 1, ch: 0},
                {line: lastLine, ch: lastLineLength},
                {scroll: false}
            );
        },
        'Ctrl-A': function (cm) {
            const cursor = cm.getCursor();

            // If cursor is on the first line, select all text in that line
            if (cursor.line === 0) {
                const lineLength = cm.getLine(0).length;
                cm.setSelection(
                    {line: 0, ch: 0},
                    {line: 0, ch: lineLength}
                );
                return;
            }

            // Otherwise, use default Cmd-A behavior (select all except first line)
            const lastLine = cm.lastLine();
            const lastLineLength = cm.getLine(lastLine).length;

            cm.setSelection(
                {line: 1, ch: 0},
                {line: lastLine, ch: lastLineLength},
                {scroll: false}
            );
        },
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

    newEditor.getWrapperElement().addEventListener('mousedown', function (e) {
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
            background: var(--col-bg-alt); color: var(--col-tx); padding: 8px 16px; border-radius: 5px;
            border: 1px solid var(--col-border);
            z-index: 9999; font-size: 14px;
        `;
        document.body.appendChild(toast);
        setTimeout(() => document.body.removeChild(toast), 1000);
    }, true);

    initAutoscroll(newEditor);

    return newEditor;
}

async function initWasm() {
    console.log('INIT CHAT');
    const go = new Go();
    const wasmFile = await fetch(`chat.wasm${window.COMMIT_HASH}`);
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

// Logic for click-handling is in click.js => isWikiLink
function createAutocompleteDict() {
    const entries = [];

    // Collect all files with their metadata
    Object.keys(excludeDirs(SYSTEM_DIRS)).forEach(dir => {
        Object.keys(files[dir]).forEach(filename => {
            if (filename === CONFIG_FILENAME || filename === CHAT_FILENAME) {
                return;
            }
            const key = `${filename.replace(/\.md$/, '')}`;
            const url = `${dir}/${filename}`.replace(/ /g, '%20');
            const filePath = `${filename.replace(/\.md$/, '')}](${url})`;

            entries.push({
                key,
                filePath,
                lastModified: files[dir][filename].lastModified
            });
        });
    });

    // Sort by last modified (most recent first)
    entries.sort((a, b) => b.lastModified - a.lastModified);
    const dict = {};
    entries.forEach(entry => {
        dict[entry.key] = entry.filePath;
    });

    let lowPriorityEntries = [];
    ['_read_', '_watch_', '_shop_', 'today', 'later', 'journal'].forEach(dir => {
        Object.keys(files[dir]).forEach(filename => {
            if (filename === CONFIG_FILENAME || filename === CHAT_FILENAME) {
                return;
            }
            const key = `${filename.replace(/\.md$/, '')}`;
            const url = `${dir}/${filename}`.replace(/ /g, '%20');
            const filePath = `${filename.replace(/\.md$/, '')}](${url})`;

            lowPriorityEntries.push({
                key,
                filePath,
                lastModified: files[dir][filename].lastModified
            });
        });
    });

    lowPriorityEntries.sort((a, b) => b.lastModified - a.lastModified);
    lowPriorityEntries.forEach(entry => {
        dict[entry.key] = entry.filePath;
    });

    return dict;
}

function renderSidebar(focusDir = '') {
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

    const groups = [
        ['_read_', '_watch_', '_shop_'],
        ['today', 'later'],
        ['journal', 'habits', 'insights', 'archive'],
    ];

    for (let i = 0; i < groups.length; i++) {
        const dirList = groups[i];
        const existingDirs = dirList.filter(dir => files[dir]);
        if (existingDirs.length === 0) continue;

        existingDirs.forEach((dir, index) => {
            const dirNode = root.getChildren().find(child => child.toString() === dir);
            if (dirNode) {
                root.removeChild(dirNode);
                if (index === existingDirs.length - 1) {
                    dirNode.isGroupEnd = true;
                }
                root.addChild(dirNode);
            }
        });
    }

    const groupedDirs = new Set(['_read_', '_watch_', '_shop_', 'journal', 'habits', 'insights', 'archive', 'today', 'later']);
    for (const dir in files) {
        if (dir === '' || dir === 'media' || groupedDirs.has(dir)) continue;

        const dirNode = root.getChildren().find(child => child.toString() === dir);
        if (dirNode) {
            root.removeChild(dirNode);
            root.addChild(dirNode);
        }
    }

    // Process root-level files
    if (files['']) {
        for (let file in files['']) {
            if (file === CONFIG_FILENAME || file === CHAT_FILENAME) {
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

    await renderSidebar();
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

    await renderSidebar(finalFolderName);
}

// Focus last line before the links.
function focusLastLine() {
    let lastLine = currentEditor.lastLine();
    let targetLine = lastLine;

    // Eat all empty lines before first links.
    while (lastLine >= 0) {
        const lineContent = currentEditor.getLine(lastLine).trim();
        if (lineContent === '') {
            lastLine--;
            continue;
        }

        lastLine = Math.min(lastLine + 1, currentEditor.lastLine());
        break;
    }
    for (let i = lastLine; i >= 0; i--) {
        const lineContent = currentEditor.getLine(i).trim();

        if (!lineContent.startsWith('[') && (!lineContent.endsWith(']') || !lineContent.endsWith(')'))) {
            targetLine = i;
            break;
        }
    }
    const targetChar = currentEditor.getLine(targetLine).length;
    currentEditor.setCursor({line: targetLine, ch: targetChar});
    // Cursor at the end, but scroll the doc to top
    currentEditor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
    currentEditor.focus();
}

function isMetaKey(event) {
    return event.metaKey || event.ctrlKey || event.altKey;
}

// Hotkeys
window.addEventListener('keydown', async (event) => {
    if (isMetaKey(event) && event.key == 'w') {
        hideEditor2();
    }

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
        moveModal.open();
    }

    if (isMetaKey(event) && event.key === 'd') {
        event.preventDefault();
        event.stopPropagation();

        let dir = editor.currentDir;
        let filename = editor.currentFile;
        if (filename === CHAT_FILENAME) {
            return;
        }

        const nextFile = findNextFile(dir, filename);

        let oldPath = toPath(dir, filename);
        let newPath = toPath('archive', filename);

        await moveFile(oldPath, newPath);

        await renderSidebar();
        if (nextFile) {
            await openFile(nextFile.dir, nextFile.filename);
        } else {
            showRandomFile();
        }
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

document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
        if (chatContainer.style.display !== 'none') {
            const selectedMessages = chat.querySelectorAll('.message.selected');
            if (selectedMessages.length > 0) {
                selectedMessages.forEach(message => message.classList.remove('selected'));
                event.preventDefault();
                event.stopPropagation();
                return;
            }

            closeChatModal();
            editor.focus();
            return;
        }

        hideEditor2();
        editor.focus();

        const allMessages = chat.querySelectorAll('.message');
        allMessages.forEach(message => message.classList.remove('selected'));
        // If in chat, focus chat input
        if (isChat) {
            chatInput.focus();
        }
    }
});

function openBot() {
    if (isChat) {
        return;
    }

    sidebarContainer.style.display = 'none';
    content.style.display = 'none';
    chat.style.display = 'flex';
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
    if (event.shiftKey && isMetaKey(event) && event.key === 'Enter') {
        event.preventDefault();
        if (isChat) {
            history.back();
        } else {
            openChat();
        }
        return;
    }
    if (isMetaKey(event) && event.key === '\\') {
        toggleSidebar();
    }
    if (isMetaKey(event) && event.key === 'Enter') {
        event.preventDefault();
        toggleChat();
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
        renderSidebar();
        return;
    }

    isWelcome = false;
    renderSidebar();
    await openChat();
}

function getCurrentContent() {
    let content = currentEditor.getValue();
    const header = toHeader(currentEditor.currentFile).toLowerCase();
    // Remove header if it exists.
    if (content.toLowerCase().startsWith(header)) {
        content = content.slice(`${header}\n`.length);
    } else if (content.toLowerCase().startsWith('# ')) {
        // Skip header placeholder.
        // What is the case when starts with # '? Empty filename? Header not equal to original header?
        // TODO but do we always have \n?
        content = content.slice(`# \n`.length);
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
    await renderSidebar();
    console.log('Sync completed');
});

// Sync files on chat focus lose.
window.addEventListener('blur', async function () {
    console.log('Window lost focus');
    editor.refresh();
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
    await renderSidebar();
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
    if (document.getElementById('search').style.display !== 'none' ||
        document.getElementById('move').style.display !== 'none') {
        return;
    }

    if (isChat) {
        return;
    }
}, true);

function toggleSidebar() {
    const sidebar = document.getElementById('sidebar');
    const openZone = document.getElementById('open-sidebar');

    if (sidebar.style.display === 'none') {
        sidebar.style.display = 'block';
        openZone.style.display = 'none';
    } else {
        sidebar.style.display = 'none';
        openZone.style.display = 'block';
        if (isChat) {
            chatInput.focus();
        } else {
            currentEditor.focus();
        }
    }
}

function getCurrentVersion() {
    return window.COMMIT_HASH ? window.COMMIT_HASH.replace('?v=', '') : '';
}

function findNextFile(currentDir, currentFilename) {
    const allFiles = [];

    // Collect all files except system files
    for (let dir in excludeDirs(SYSTEM_DIRS)) {
        for (let file in files[dir]) {
            if (file === CONFIG_FILENAME || file === CHAT_FILENAME) {
                continue;
            }
            allFiles.push({dir, filename: file});
        }
    }

    if (allFiles.length <= 1) {
        return null; // No other files available
    }

    // Find current file index
    const currentIndex = allFiles.findIndex(f =>
        f.dir === currentDir && f.filename === currentFilename
    );

    if (currentIndex === -1) {
        return allFiles[0]; // Fallback to first file
    }

    // Return next file, or first file if we're at the end
    const nextIndex = (currentIndex + 1) % allFiles.length;
    return allFiles[nextIndex];
}

function showEditor2() {
    rememberEditorPos();
    const editor2Container = document.getElementById('editor2-container');

    editor2Container.style.display = 'flex';
    editor2Container.offsetHeight; // Force reflow
    editor2Container.classList.add('show');

    editor.refresh();
    editor2.focus();
    restoreEditorPos();
}

function hideEditor2() {
    const editor2Container = document.getElementById('editor2-container');

    editor2Container.classList.remove('show');
    restoreEditorPos();

    setTimeout(() => {
        editor2Container.style.display = 'none';
        editor.refresh(); // IT seems we have to refresh once size changes.
    }, 300);
}

let topLineNumber;
function rememberEditorPos() {
    const scrollInfo = editor.getScrollInfo();
    const topCoords = editor.coordsChar({left: 0, top: scrollInfo.top}, "local");
    topLineNumber = topCoords.line;
}

function restoreEditorPos() {
    if (topLineNumber === undefined) {
        return;
    }
    editor.refresh();
    const newTopLineY = editor.charCoords({line: topLineNumber, ch: 0}, "local").top;
    editor.scrollTo(null, newTopLineY);
}