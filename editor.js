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
        viewportMargin: 10,
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
        closeInboxModal();
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

        if (match && files['media/'] && files['media/'][match[1]]) {
            return files['media/'][match[1]].imageUrl;
        }

        if (match && files['img/'] && files['img/'][match[1]]) {
            return files['img/'][match[1]].imageUrl;
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

        path += '.md';

        if (getMemFile(path) !== null) {
            openFile(path, true, 'editor2-textarea')
            return;
        }

        // Try to find filename is any folder
        let filename = toFilename(path);
        walk(files, (path, isFile) => {
            if (!isFile) {
                return;
            }

            if (toFilename(path) === filename) {
                openFile(path, true, 'editor2-textarea');
                return false;
            }
        });
    };

    newEditor.on('inputRead', async function (cm, change) {
        if (change.text.length === 1 && change.text[0] === '[') {
            cm.showHint({
                completeSingle: false, updateOnCursorActivity: true,
            })
        }
    })

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
                        cm.replaceRange('\n', {line: 1, ch: 0});
                        cm.setCursor({line: 1, ch: 0});
                        return;
                    }
                } else {
                    // No selection, just move cursor to next line
                    cm.setCursor({line: 1, ch: 0});
                    cm.replaceSelection('\n');
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