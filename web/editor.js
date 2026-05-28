function cleanupEditor(editorInstance) {
    if (!editorInstance) return;

    if (editorInstance._tableScrollObserver) {
        editorInstance._tableScrollObserver.disconnect();
    }

    editorInstance.off();
}

function wrapRenderedTables(root) {
    if (!root || !root.querySelectorAll) return;

    root.querySelectorAll('table').forEach((table) => {
        if (table.closest('.hmd-table-scroll')) return;

        const wrapper = document.createElement('div');
        wrapper.className = 'hmd-table-scroll';
        table.parentNode.insertBefore(wrapper, table);
        wrapper.appendChild(table);
    });
}

function installTableScrollWrappers(cm) {
    const wrapper = cm.getWrapperElement();

    wrapRenderedTables(wrapper);

    const observer = new MutationObserver((mutations) => {
        mutations.forEach((mutation) => {
            mutation.addedNodes.forEach((node) => {
                if (node.nodeType !== Node.ELEMENT_NODE) return;

                if (node.matches && node.matches('table')) {
                    wrapRenderedTables(node.parentNode);
                    return;
                }

                wrapRenderedTables(node);
            });
        });
    });

    observer.observe(wrapper, {childList: true, subtree: true});
    cm._tableScrollObserver = observer;
}

function getHorizontalTableScrollbar(e) {
    if (!e.target.closest) return null;

    const scroller = e.target.closest('.hmd-table-scroll');
    if (!scroller || scroller.scrollWidth <= scroller.clientWidth) return null;

    const scrollbarHeight = scroller.offsetHeight - scroller.clientHeight;
    if (scrollbarHeight <= 0) return null;

    const rect = scroller.getBoundingClientRect();
    const withinX = e.clientX >= rect.left && e.clientX <= rect.right;
    const withinScrollbarY = e.clientY >= rect.bottom - scrollbarHeight && e.clientY <= rect.bottom;

    return withinX && withinScrollbarY ? scroller : null;
}

function initEditor(el) {
    if (window.editor !== undefined && el.id === 'editor-textarea' ) {
        cleanupEditor(editor);
        const wrapper = editor.getWrapperElement();
        if (wrapper && wrapper.parentNode) {
            wrapper.parentNode.removeChild(wrapper);
        }

        cleanupEditor(editor2);
        const wrapper2 = editor2.getWrapperElement();
        if (wrapper2 && wrapper2.parentNode) {
            wrapper2.parentNode.removeChild(wrapper2);
        }
    } else if (window.editor2 !== undefined && el.id === 'editor2-textarea') {
        cleanupEditor(editor2);
        const wrapper = editor2.getWrapperElement();
        if (wrapper && wrapper.parentNode) {
            wrapper.parentNode.removeChild(wrapper);
        }
    }

    let newEditor = HyperMD.fromTextArea(el, {
        dragDrop: false,
        viewportMargin: 10,
        mode: {
            name: 'hypermd',
            math: true,
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
        hmdFoldMath: {
            renderer: KatexRenderer,
        },
        // Enable fold-code so ```mermaid blocks get rendered via the
        // hypermd-mermaid renderer (registered as suggested:true on load).
        hmdFoldCode: { mermaid: true },
        configureMouse: () => ({addNew: false}) // disable multicursor
    });
    newEditor.setSize(null, '100%');
    newEditor.on('focus', function() {
        currentEditor = newEditor; // FIXME possible RC here? If isMessingWithCurrentEditor is hold, this would overwrite
        currentEditor.refresh(); // Cursor & hide tokens conflict if we don't call it
        closeChatModal();
        log('Focused to:', newEditor.path);
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

        // Bare domain like google.com/test - window.open treats it as a relative
        // path without a protocol, which makes the PWA navigate to a sub-path.
        // Exclude local-file extensions (md, image types) so `![](img.png)`
        // isn't mistaken for a domain.
        if (/^[a-z0-9-]+(\.[a-z0-9-]+)+(\/|$)/i.test(path)
            && !/\.(md|png|jpg|jpeg|gif|webp)$/i.test(path)) {
            return 'https://' + path;
        }

        if (/^(?!http|https|\[).+\.md$/.test(path)) {
            const isMobile = window.matchMedia('(max-width: 670px)').matches;
            const target = isMobile ? 'editor-textarea' : 'editor2-textarea';
            const fullPath = path.startsWith('/') ? path : '/' + path;
            openFile(fullPath, true, target);
            return path;
        }

        // Capture only the bare filename (no slashes) so the media/img
        // lookups by filename work even when path has a folder prefix.
        const match = path.match(/(?:^|\/)([^/]+\.(png|jpg|jpeg|gif|webp))$/i);

        if (match && files['media/'] && files['media/'][match[1]]) {
            return files['media/'][match[1]].imageUrl;
        }

        if (match && files['img/'] && files['img/'][match[1]]) {
            return files['img/'][match[1]].imageUrl;
        }

        // Filename fallback - look up bare filename in the global
        // image index built by loadLocalFiles. Resolves images stored in
        // any folder when the markdown link's path doesn't match.
        if (match) {
            const bareName = match[1].split('/').pop();
            if (mediaIndex[bareName] && mediaIndex[bareName].imageUrl) {
                return mediaIndex[bareName].imageUrl;
            }
        }

        return path;
    };

    newEditor.hmdReadLink = async function (path) {
        path = path.replace(/\|.*]$/, '');
        path = path.replace('[', '').replace(']', '');

        // Handle action links
        if (path === 'cmd:openDir') {
            openDir();
            return;
        }
        if (path === 'cmd:openChat') {
            openChat();
            return;
        }

        // If it is a web link open window blank
        if (/^(http|https):\/\//.test(path)) {
            window.open(path, '_blank');
            return;
        }

        path += '.md';

        // Phones don't have the room for the split-view editor2 -
        // route link follows into the main editor instead.
        const target = window.matchMedia('(max-width: 670px)').matches
            ? 'editor-textarea'
            : 'editor2-textarea';

        if (getMemFile(path) !== null) {
            openFile(path, true, target)
            return;
        }

        // Try to find filename is any folder
        let filename = toFilename(path);
        walk(files, (path, isFile) => {
            if (!isFile) {
                return;
            }

            if (toFilename(path) === filename) {
                openFile(path, true, target);
                return false;
            }
        });
    };

    newEditor.on('inputRead', async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '`') {
            const cursor = cm.getCursor();
            const line = cm.getLine(cursor.line);
            const before = line.slice(0, cursor.ch);
            const after = line.slice(cursor.ch);
            // Trigger only on the third backtick at the start of a line.
            if (!/^ {0,3}`{3}$/.test(before)) return;
            if (after.length > 0) return;
            // Skip when this ``` is closing an already-open fence.
            if (cursor.line > 0) {
                const prevState = cm.getStateAfter(cursor.line - 1);
                if (prevState && prevState.fencedEndRE) return;
            }
            cm.replaceRange('\n\n```', cursor);
            cm.setCursor({ line: cursor.line + 1, ch: 0 });
            return;
        }
        if (change.text.length === 1 && change.text[0] === '[') {
            const cursor = cm.getCursor();
            // Skip the link autocomplete when the [ is preceded by a
            // backslash - that's an escaped bracket, not the start of a link.
            const charBefore = cursor.ch >= 2 ? cm.getRange({line: cursor.line, ch: cursor.ch - 2}, {line: cursor.line, ch: cursor.ch - 1}) : '';
            if (charBefore === '\\') {
                return;
            }
            // Skip when the [ sits inside an inline code span (`...[...`).
            // The token at the cursor carries the `inline-code` style and we
            // don't want to insert links into code.
            const token = cm.getTokenAt(cursor);
            if (token && token.type && /\binline-code\b/.test(token.type)) {
                return;
            }
            cm.showHint({
                completeSingle: false, updateOnCursorActivity: true,
            })
        }
    })

    installTableScrollWrappers(newEditor);

    newEditor.getWrapperElement().addEventListener('mousedown', function(e) {
        const scroller = getHorizontalTableScrollbar(e);
        if (!scroller) return;

        scroller.classList.add('is-scrolling');
        const stopScrolling = () => scroller.classList.remove('is-scrolling');
        window.addEventListener('mouseup', stopScrolling, {once: true});
        window.addEventListener('blur', stopScrolling, {once: true});
        e.stopImmediatePropagation();
    }, true);

    // Auto-select/highlight title when clicking on the first line
    // TODO clear on second click
    newEditor.getWrapperElement().addEventListener('mousedown', function(e) {
        // Get the position where the mouse was clicked
        const coords = newEditor.coordsChar({left: e.clientX, top: e.clientY});

        if (coords.line === 0) {
            // Check if cursor is already on line 0
            const currentCursor = newEditor.getCursor();
            if (currentCursor.line === 0) {
                // Cursor already on line 0, don't select
                return;
            }

            // Cursor not on line 0, select the title
            setTimeout(() => {
                const lineLength = newEditor.getLine(0).length;
                newEditor.setSelection(
                    {line: 0, ch: 2},  // Start from character 2 (skip "# ")
                    {line: 0, ch: lineLength}
                );
            }, 150);
        }
    }, true);

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
                    const fileHandle = await writeMediaFile(fileName, file);
                    if (fileHandle) {
                        if (!files['media/']) {
                            files['media/'] = {};
                        }
                        files['media/'][fileName] = {
                            isFile: true,
                            handle: fileHandle,
                            lastModified: Date.now(),
                            imageUrl: URL.createObjectURL(file)
                        };

                        const markdownImageSyntax = `![](media/${fileName})\n`;
                        currentEditor.replaceSelection(markdownImageSyntax);
                        log(`Image saved as: ${fileName}`);
                    } else {
                        logError('Failed to save the image.');
                        alert('Failed to save the image. Please try again.');
                    }
                } catch (error) {
                    logError('Error saving image:', error);
                    alert('Error saving image: ' + error.message);
                }
            }
        }
    });

    // Editor keybindings
    newEditor.addKeyMap({
        'Enter': function (cm) { // If header is selected, enter should move cursor to next line
            const cursor = cm.getCursor();
            // If there's a selection on the header line, just move cursor
            if (cursor.line === 0) {
                if (cm.somethingSelected()) {
                    const selections = cm.listSelections();
                    const isHeaderSelection = selections.some(sel =>
                        sel.anchor.line === 0 || sel.head.line === 0
                    );

                    if (isHeaderSelection) {
                        // Clear selection and move cursor to start of line 1
                        cm.setCursor({line: 1, ch: 0});
                        return;
                    }
                } else {
                    // No selection, just move cursor to next line
                    cm.setCursor({line: 1, ch: 0});
                    return;
                }
            }

            // For all other lines, use default Enter behavior
            return CodeMirror.Pass;
        },
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

    function showCopiedToast() {
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
    }

    newEditor.getWrapperElement().addEventListener('mousedown', function (e) {
        if (!isMetaKey(e)) return;

        e.preventDefault();

        const code = e.target.closest('.cm-inline-code');
        if (!code) return;

        navigator.clipboard.writeText(code.textContent);
        showCopiedToast();
    }, true);

    newEditor.on('renderLine', function (cm, lineHandle, el) {
        if (el.querySelector('.code-copy-btn')) return;
        const lineNo = lineHandle.lineNo();
        if (lineNo == null) return;
        const here = cm.getStateAfter(lineNo);
        const prev = lineNo > 0 ? cm.getStateAfter(lineNo - 1) : null;
        if (!(here && here.fencedEndRE && (!prev || !prev.fencedEndRE))) return;
        if (/^\s*```\s*mermaid\b/.test(cm.getLine(lineNo))) return;
        const btn = document.createElement('button');
        btn.className = 'code-copy-btn';
        btn.title = 'Copy';
        btn.onmousedown = function (e) {
            e.preventDefault();
            e.stopPropagation();
            const begin = lineHandle.lineNo();
            const lines = [];
            for (let L = begin + 1, last = cm.lineCount(); L < last; L++) {
                const st = cm.getStateAfter(L);
                if (!st || !st.fencedEndRE) break;
                lines.push(cm.getLine(L));
            }
            navigator.clipboard.writeText(lines.join('\n'));
            showCopiedToast();
        };
        el.appendChild(btn);
    });

    initAutoscroll(newEditor);

    return newEditor;
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
    currentEditor.setCursor({ line: targetLine, ch: targetChar });
    // Cursor at the end, but scroll the doc to top
    currentEditor.scrollTo(null, 0);
    // TODO only focus if there's no quick dialogue
    currentEditor.focus();
}

let savedScrollTop;
function rememberEditorPos() {
    savedScrollTop = editor.getScrollInfo().top;
}

function restoreEditorPos() {
    if (savedScrollTop === undefined) {
        return;
    }
    editor.refresh();
    editor.scrollTo(null, savedScrollTop);
}

// KaTeX renderer for HyperMD's fold-math addon. fold-math instantiates
// this class with (container, mode) and calls startRender/clear as the
// user edits. mode === 'display' for $$...$$, anything else for $...$.
function KatexRenderer(container, mode) {
    this.container = container;
    this.mode = mode;
    this.span = document.createElement('span');
    container.appendChild(this.span);
}
KatexRenderer.prototype.startRender = function (expr) {
    try {
        window.katex.render(expr, this.span, {
            displayMode: this.mode === 'display',
            throwOnError: false,
        });
    } catch (e) {
        this.span.textContent = expr;
    }
    if (this.onChanged) this.onChanged(expr);
};
KatexRenderer.prototype.clear = function () {
    if (this.span.parentNode === this.container) {
        this.container.removeChild(this.span);
    }
};
KatexRenderer.prototype.isReady = function () {
    return true;
};
