async function getOPFSDirHandle() {
    const root = await navigator.storage.getDirectory();
    const entries = [];
    for await (const entry of root.values()) {
        entries.push(entry);
    }

    if (entries.length === 0) {
        async function createFiles(obj, dirHandle) {
            for (const [name, data] of Object.entries(obj)) {
                if (data.isFile) {
                    const fileHandle = await dirHandle.getFileHandle(name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(data.content);
                    await writable.close();
                } else {
                    const subDirHandle = await dirHandle.getDirectoryHandle(removeTrailingSlash(name), { create: true });
                    await createFiles(data, subDirHandle);
                }
            }
        }
        await createFiles(DEFAULT_FILES, root);
    }

    return root;
}

async function migrateFromOPFSToLocal() {
    try {
        log('Starting migration from OPFS to Local FS...');
        const opfsRoot = await navigator.storage.getDirectory();
        const localRoot = await getSavedRootDirHandle();

        // Copy files to local directory
        let copiedCount = 0;
        const operations = [];

        walk(files, (path, isFile) => {
            if (isFile) {
                const operation = (async () => {
                    try {
                        // Read from OPFS
                        const file = await (await getOPFSFileHandle(opfsRoot, path)).getFile();
                        const content = await file.text();

                        // Write to local FS
                        const fileHandle = await getFileHandle(path, true);
                        const writable = await fileHandle.createWritable();
                        await writable.write(content);
                        await writable.close();
                        copiedCount++;
                        log(`✓ Copied: ${path}`);
                    } catch (error) {
                        console.error(`✗ Failed to copy ${path}:`, error);
                    }
                })();
                operations.push(operation);
            } else {
                // Create directory in local FS
                const operation = createDirectory(localRoot, path);
                operations.push(operation);
            }
        });

        // Wait for all operations to complete
        await Promise.all(operations);

        return { success: true, copiedCount };
    } catch (error) {
        console.error('Migration failed:', error);
        return { success: false, error };
    }
}

async function getOPFSFileHandle(rootHandle, path) {
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
    for (const dirName of dirs) {
        if (dirName) {
            try {
                rootHandle = await rootHandle.getDirectoryHandle(dirName);
            } catch (error) {
                throw error;
            }
        }
    }

    let fileHandle;
    try {
        fileHandle = await rootHandle.getFileHandle(filename);
    } catch (error) {
        throw error;
    }

    return fileHandle;
}

async function createDirectory(rootHandle, dirPath) {
    const pathParts = dirPath.split('/').filter(p => p);

    let currentHandle = rootHandle;
    for (const dirName of pathParts) {
        currentHandle = await currentHandle.getDirectoryHandle(dirName, { create: true });
    }
}

const DEFAULT_FILES = {
    "brain/": {
        "We think that we understand, but in reality we just know.md": {
            "content": "Reading and rereading can easily fool us into believing that we understand a text. Rereading is especially dangerous because of the mere-exposure effect: The moment we become familiar with something, we start believing we also understand it. On top of that, we also tend to like it it more.\n\n[Brain is the most complex object in known universe]",
            isFile: true,
        },
        "Brain is the most complex object in known universe.md": {
            "content": "Nothing will make you appreciate human intelligence like learning about how unbelievably challenging it is to try to create a computer as smart as we are. Building skyscrapers, putting humans in space, figuring out the details of how the Big Bang went down—all far easier than understanding our own brain or how to make something as cool as it\n\n[We think that we understand, but in reality we just know]",
            isFile: true,
        },
        "Change your environment instead of using willpower.md": {
            "content": "When scientists analyze people who appear to have tremendous self-control, it turns out those individuals aren’t all that different from those who are struggling. Instead, “disciplined” people are better at structuring their lives in a way that does not require heroic willpower and self-control.\n",
            isFile: true,
        },
    },
    "happiness/": {
        "Meditation.md": {
            "content": "Once you are relaxed, picture yourself living in an abundant world. In this abundant world, there are no restraints or limitations. Good things flow past you continuously. Imagine every abundant thing you have ever desired–car, home, friends, love, joy, wealth, success, peace of mind, challenge. Visualize yourself living your life surrounded by this abundance. Repeat this visualization several times a day until it begins to feel real to you. Open your arms, your heart, and your mind. Get out of the way, and let it happen.\n\n[Boredom is just an emotion]",
            isFile: true,
        },
        "Boredom is just an emotion.md": {
            "content": "It's not an indicator that you're doing something wrong in your life\n\nBefore we had phones and technologies we would just sit around the fire and we would talk and we wouldn't call that boring that was just life\n\nAnd bow we have that endless need for entertainment, anything when nothing is happening we think it's wrong and we need to fix it\n\nNon eventfulness is just a part of our life and you can embrace it as\npeace or you can frantically try to create more chaos\n\n[Meditation]",
            isFile: true,
        },
    },
    "🪴 Welcome.md": {
        "content": "Only essential features. No distractions.\n\n" +
            "You don't need fancy tools to take notes...\n\n"
            + "[Markdown Guide]\n[Hotkeys]\n[Links]",
        isFile: true,
    },
    "Links.md": {
        "content": "Links are important\n" +
            "\n" +
            "Relations among ideas are far more important than the ideas themselves.\n" +
            "Learning is making meaningful connections.\n\n[Markdown Guide]",
        isFile: true,
    },
    "Markdown Guide.md": {
        "content":
            "Use `#` for headers. More `#` symbols create smaller headers.\n" +
            "\n" +
            "#### Text Formatting\n" +
            "- **Bold text** using `**bold**` or `__bold__` (Cmd/Ctrl + B)\n" +
            "- *Italic text* using `*italic*` or `_italic_` (Cmd/Ctrl + I)\n" +
            "- ***Bold and italic*** using `***text***`\n" +
            "- ~~Strikethrough~~ using `~~text~~`\n" +
            "- `Inline code` using backticks\n" +
            "\n" +
            "#### Lists\n" +
            "- First item\n" +
            "- Second item\n" +
            "  - Third item\n\n" +
            "1. First item\n" +
            "2. Second item\n" +
            "   1. Third item\n" +
            "\n" +
            "#### Checklist\n" +
            "- [x] Completed task\n" +
            "- [ ] Incomplete task\n" +
            "- [ ] Another incomplete task\n\n" +
            "Format:\n`- [ ] Item`\n" +
            "\n" +
            "#### Blockquotes\n" +
            ">This is a blockquote. It can span multiple lines and is great for highlighting important information or quotes from other sources.\n" +
            "\nFormat:\n`> This is a blockquote`\n" +
            "\n" +
            "#### Code Blocks\n" +
            "```\n" +
            "Here is some code.\n" +
            "```\n" +
            "\n" +
            "#### Images\n" +
            "![Why taking notes](https://app.files.md/lib/notes.jpg)\n" +
            "\n" +
            "*You can paste your own images via `Cmd/Ctrl + V`*\n\n" +
            "#### Links\n" +
            "You can insert your own links by typing `[`.\n\n" +
            "[Links]\n" +
            "[My project]",
        isFile: true,
    },
    "Hotkeys.md": {
        "content":
            "| Hotkey | Action |\n" +
            "| -------- |-------- |\n" +
            "| `Cmd+K` / `Ctrl+K` | Open file search modal |\n" +
            "| `Cmd+N` / `Ctrl+N` | New file |\n" +
            "| `Cmd+M` / `Ctrl+M` | Move file |\n" +
            "| `Cmd+D` / `Ctrl+D` | Delete file |\n" +
            "| `Cmd+Enter` / `Ctrl+Enter` | Toggle chat mode |\n" +
            "| `Cmd+[` / `Ctrl+[`  | Go to previous file   |\n" +
            "| `Cmd+]` / `Ctrl+]`  | Go to next file  |\n" +
            "| `Cmd+\\` / `Ctrl+\\`  | Toggle sidebar |\n" +
            "\n" +
            "#### Text Formatting\n" +
            "\n" +
            "| Hotkey | Action |\n" +
            "| -------- | -------- |\n" +
            "| `Cmd+B` / `Ctrl+B` | Toggle **bold** formatting |\n" +
            "| `Cmd+I` / `Ctrl+I` | Toggle *italic* formatting |\n" +
            "| `Cmd+Y` / `Ctrl+Y`| Insert ✅ checkbox at line start |\n" +
            "| `Cmd` / `Ctrl`+`Click`| Copy text from `inline` element |\n" +
            "| `Cmd` / `Ctrl`+`Click`| To open a link like https//files.md |\n" +
            "| `Ctrl` + `Cmd` + `Space`| Insert emoji (MacOS) |\n" +
            "\n" +
            "#### Editor Functions\n" +
            "\n" +
            "| Hotkey | Action |\n" +
            "| -------- | -------- |\n" +
            "| `[` | Trigger file link autocomplete |\n" +
            "\n" +
            "[Markdown Guide]",
        isFile: true,
    },
    "My project.md": {
        "content": "You can dump project related thoughts here.",
        isFile: true,
    }
}

