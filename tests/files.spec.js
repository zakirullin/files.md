const {test, expect} = require('@playwright/test');

test.beforeEach(async ({page}) => {
    await page.goto('/index.html');

    // await page.waitForSelector('.CodeMirror', {timeout: 10000});
    await page.waitForSelector('#tree', {timeout: 1000});
});

test('should load files', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const testDir = await root.getDirectoryHandle('test-files', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await testDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await testDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return testDir;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });
});

test('create new in subfolder', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subDir = await root.getDirectoryHandle('dir', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await subDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await subDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Body content');
    await page.waitForTimeout(100);
    await page.waitForTimeout(700);

    await page.click('#sidebar >> text=dir');
    await page.waitForTimeout(100);

    await page.click('#sidebar >> text=New file');
    await page.waitForTimeout(100);
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# New file\nBody content");
});

test('create new in root', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subDir = await root.getDirectoryHandle('dir', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await root.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await root.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#sidebar >> text=README');
    await page.waitForTimeout(100);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Body content');
    await page.waitForTimeout(700);

    await page.click('#sidebar >> text=New file');
    await page.waitForTimeout(100);
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# New file\nBody content");
});

test('file is not renamed on select all and change', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subDir = await root.getDirectoryHandle('dir', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: 'Some text' }
            ];

            for (const fileData of testFiles) {
                try {
                    await root.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await root.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click(`#tree .tree-item:has-text('README')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    // click on cm-header cm-header-1

    await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        cm.setCursor(1, cm.getLine(0).length);
    });
    await page.waitForTimeout(500);
    await page.keyboard.press('Meta+a');
    await page.waitForTimeout(100);
    await page.keyboard.type('New text');
    await page.waitForTimeout(1000);

    await page.click(`#tree .tree-item:has-text('Notes')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome text');
    await page.click(`#tree .tree-item:has-text('README')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nNew text');
    // Rename with existing content
    await page.waitForTimeout(100);
    await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        cm.setCursor(0, cm.getLine(0).length);
    });
    await page.keyboard.type('2')
    await page.waitForTimeout(1000);
    await page.click(`#tree .tree-item:has-text('README2')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README2\nNew text');
});

test('rename file via header removal', async ({ page }) => {
    await setup(page);

    await page.click(`#tree .tree-item:has-text('README')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    // click on cm-header cm-header-1

    await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        cm.setCursor(0, cm.getLine(0).length);
    });
    await page.keyboard.press('Meta+a');
    await page.waitForTimeout(500);
    await page.keyboard.type('Newname');
    await page.waitForTimeout(1000);

    await page.click(`#tree .tree-item:has-text('Notes')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome text');
    await page.click(`#tree .tree-item:has-text('Newname')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Newname\nHello world');
});

test('rename to empty name saves to untitled', async ({ page }) => {
    await setup(page);

    await page.click(`#tree .tree-item:has-text('README')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        cm.setCursor(0, cm.getLine(0).length);
    });
    await page.keyboard.press('Meta+a');
    await page.keyboard.press('Backspace');
    await page.waitForTimeout(1000);

    await page.click(`#tree .tree-item:has-text('Notes')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome text');
    await page.click(`#tree .tree-item:has-text('Untitled')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Untitled\nHello world');
});

test('create file and move', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subDir = await root.getDirectoryHandle('dir', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await root.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await root.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#sidebar >> text=README');
    await page.waitForTimeout(100);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('content');
    await page.waitForTimeout(700);

    await page.click('#sidebar >> text=New file');
    await page.waitForTimeout(100);
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# New file\ncontent");
});

test('rename should not create multiply files', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subDir = await root.getDirectoryHandle('dir', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await root.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await root.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#sidebar >> text=README');
    await page.waitForTimeout(100);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Meta+a');
    await page.keyboard.type('New');
    await page.waitForTimeout(1000);
    await page.keyboard.type(' fi');
    await page.waitForTimeout(1000);
    await page.keyboard.type('le');
    await page.waitForTimeout(500);
    await page.keyboard.press('ArrowDown')
    await page.keyboard.type('content');
    await page.waitForTimeout(700);

    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.click('#sidebar >> text=New file');
    await page.waitForTimeout(100);
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# New file\ncontent");

    const clientFiles = await page.evaluate(() => {
        return Object.keys(files);
    })

    expect(clientFiles).toBeDefined();
    expect(clientFiles.length).toBe(12);
    expect(clientFiles).toContain('New file.md');
});

test('create new file, move to new dir, create new file is subdir, move to root', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();

            return root;
        };

        window.prompt = function() {
            return 'dir1';
        }
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowUp');
    await page.waitForTimeout(100);
    await page.keyboard.press('Meta+a');
    await page.keyboard.type('file1');
    // await page.waitForTimeout(500); // TODO shoudln't be rc, maybe save file on focus out or something
    await page.keyboard.press('ArrowDown');
    await page.keyboard.type('content');
    await page.waitForTimeout(300);

    await page.click('#new-folder');
    await page.waitForTimeout(100);

    await page.keyboard.press('Meta+m');
    await page.waitForTimeout(100);
    await page.click('#move-results >> text=dir1');
    await page.waitForTimeout(100);

    // Create second file in same subdir
    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowUp');
    await page.waitForTimeout(200);
    await page.keyboard.press('Meta+a');
    await page.keyboard.type('file2');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowDown');
    await page.keyboard.type('content');
    await page.waitForTimeout(300);

    await page.keyboard.press('Meta+m');
    await page.waitForTimeout(100);
    await page.click('#move-results >> text=/');
    await page.waitForTimeout(500);

    // await page.click('#tree li:has-text("dir1")');
    await page.click('#tree li:has-text("dir1") ul li:has-text("File1")')
    await page.click('#tree li:has-text("File2")');

});

test("create new in root with empty name so that it won't remove previous file", async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            // Your mock code here
            const root = await navigator.storage.getDirectory();
            const subDir = await root.getDirectoryHandle('dir', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await root.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await root.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#sidebar >> text=README');
    await page.waitForTimeout(100);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('');
    await page.waitForTimeout(700);
    await page.keyboard.type('My actual new file');
    await page.keyboard.press('Enter');
    await page.keyboard.type('content');
    await page.waitForTimeout(700);

    // Check that existing README.md is there
    await page.click('#sidebar >> text=README');
    await page.waitForTimeout(100);
    let codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# README\nHello world");

    await page.click('#sidebar >> text=New file');
    await page.waitForTimeout(100);
    codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# New file\nMy actual new file\ncontent");
});

test('create new lower case', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const testDir = await root.getDirectoryHandle('test-files', { create: true });

            const testFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: '**Bold text**' }
            ];

            for (const fileData of testFiles) {
                try {
                    await testDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await testDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return testDir;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Meta+a');
    await page.keyboard.type('another file');
    await page.waitForTimeout(100);
    await page.keyboard.press('Enter');
    await page.keyboard.type('content');
    await page.waitForTimeout(700);

    await page.click('#sidebar >> text=another file');
    await page.waitForTimeout(100);
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# Another file\ncontent");
});

test('move file between directories', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const testDir = await root.getDirectoryHandle('test-files', { create: true });

            const projectsDir = await testDir.getDirectoryHandle('projects', { create: true });
            const archiveDir = await testDir.getDirectoryHandle('archive', { create: true });

            const rootFiles = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Todo.md', content: '- Task 1\n- Task 2' }
            ];

            for (const fileData of rootFiles) {
                try {
                    await testDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await testDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            const projectFiles = [
                { name: 'Project A.md', content: 'Project A details' },
                { name: 'Project B.md', content: 'Project B details' }
            ];

            for (const fileData of projectFiles) {
                try {
                    await projectsDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await projectsDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            const archiveFiles = [
                { name: 'Old Project.md', content: 'Archived project' }
            ];

            for (const fileData of archiveFiles) {
                try {
                    await archiveDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await archiveDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return testDir;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    // Wait for initialization
    await page.waitForTimeout(500);

    // Open a file from the projects directory
    await page.click('#sidebar >> text=projects');
    await page.waitForTimeout(100);
    await page.click('#sidebar >> text=Project A');
    await page.waitForTimeout(200);

    // Verify we're in the right file
    const initialContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(initialContent).toContain('Project A details');

    // Open move modal with Cmd+M
    await page.keyboard.press('Meta+m');
    await page.waitForTimeout(100);

    // Verify move modal is open
    const moveModalVisible = await page.isVisible('#move');
    expect(moveModalVisible).toBe(true);

    // Check that move destinations are shown
    const moveResults = await page.locator('#move-results li');
    const destinations = await moveResults.allTextContents();
    expect(destinations).toContain('/');
    expect(destinations).toContain('archive');
    expect(destinations).toContain('projects');

    // Move to archive directory by clicking
    await page.click('#move-results >> text=archive');
    await page.waitForTimeout(200);

    // Verify modal is closed
    const moveModalVisibleAfter = await page.isVisible('#move');
    expect(moveModalVisibleAfter).toBe(false);

    // Verify file is now in archive directory
    // Check if the sidebar reflects the change
    await page.click('#sidebar >> text=archive');
    await page.waitForTimeout(100);

    // Should see Project A in archive now
    const archiveFiles = await page.locator('#sidebar >> text=archive').locator('..').locator('text=Project A');
    expect(await archiveFiles.count()).toBe(1);

    // Verify content is preserved
    await page.click('#sidebar >> text=archive >> .. >> text=Project A');
    await page.waitForTimeout(200);

    const finalContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(finalContent).toContain('Project A details');
});

test('move file using keyboard navigation', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const testDir = await root.getDirectoryHandle('test-files', { create: true });

            // Create directories
            const workDir = await testDir.getDirectoryHandle('work', { create: true });
            const personalDir = await testDir.getDirectoryHandle('personal', { create: true });

            // Create a file in root
            const rootFiles = [
                { name: 'Meeting Notes.md', content: 'Important meeting notes' }
            ];

            for (const fileData of rootFiles) {
                try {
                    await testDir.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await testDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return testDir;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.waitForTimeout(500);

    // Open the file from root
    await page.click('#sidebar >> text=Meeting Notes');
    await page.waitForTimeout(200);

    // Open move modal
    await page.keyboard.press('Meta+m');
    await page.waitForTimeout(100);

    // Use arrow keys to navigate
    await page.keyboard.press('ArrowDown');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowDown');
    await page.waitForTimeout(100); // move to 'work'

    // Press Enter to select
    await page.keyboard.press('Enter');
    await page.waitForTimeout(200);

    // Verify file moved to work directory
    await page.click('#sidebar >> text=work');
    await page.waitForTimeout(100);

    const workFiles = await page.locator('#sidebar >> text=work').locator('..').locator('text=Meeting Notes');
    expect(await workFiles.count()).toBe(1);
});

test('create file in selected folder', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const testDir = await root.getDirectoryHandle('files', { create: true });
            await root.getDirectoryHandle('projects', { create: true });
            const rootFiles = [
                { name: 'README.md', content: 'Hello world' }
            ];

            for (const fileData of rootFiles) {
                try {
                    await root.getFileHandle(fileData.name);
                } catch (error) {
                    const fileHandle = await testDir.getFileHandle(fileData.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(fileData.content);
                    await writable.close();
                }
            }

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.waitForTimeout(500);

    await page.click('#sidebar >> text=projects');
    await page.waitForTimeout(100);

    await page.click('#new-file');
    await page.waitForTimeout(100);

    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Meta+a');

    await page.keyboard.type('Project file');
    await page.waitForTimeout(100);
    await page.keyboard.press('ArrowDown');
    await page.keyboard.type('File created in projects folder');
    await page.waitForTimeout(200);

    // close projects dir
    await page.click('#sidebar >> text=projects');
    await page.waitForTimeout(200);


    await page.click('#sidebar >> text=files');
    await page.waitForTimeout(100);

    const projectFileAtRoot = page.locator('#sidebar >> text=Project file');
    expect(await projectFileAtRoot.count()).toBe(0);

    await page.click('#sidebar >> text=projects');
    await page.waitForTimeout(200);

    await page.click('#sidebar >> text=Project file');
    await page.waitForTimeout(500);

    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe("# Project file\nFile created in projects folder");

    const projectFiles = await page.locator('#sidebar >> text=projects').locator('..').locator('text=Project file');
    expect(await projectFiles.count()).toBe(1);
});

// Regression test for a destructive file-duplication cascade.
//
// --- Where the drift happened ---
//
// The decisive line is web/files.js:1028 in the parent openFile, but you
// have to look at what currentEditor points to when it runs. Setting the
// stage for pre-fix code:
//
// Just before the click on Awareness:
//   - editor.path = /hap/Dream.md, editor content = Dream
//   - editor2.path = /life/Pilaf.md, editor2 content = stale Pilaf
//   - disk /life/Pilaf.md was externally updated
//
// Click on Awareness fires openFile('/hap/Awareness.md', true,
// 'editor2-textarea') (call this P for parent):
//
//   1. P:977 — currentEditor = editor2. Good.
//   2. P:982 — editor2.path ≠ Awareness, so await syncCurrentEditor(false).
//      Control leaves P here.
//   3. Inside that syncCurrentEditor: path = /life/Pilaf.md. Disk differs
//      from editor2 cache. editor2 is clean. Enters the reload branch and
//      calls await openFile('/life/Pilaf.md', false) — no el argument,
//      defaults to 'editor-textarea'. Call this N for nested.
//   4. Inside N: el === 'editor-textarea' → N:976 sets currentEditor =
//      editor. Then loads disk Pilaf into editor. N:1038 reassigns
//      currentEditor = editor again after reinit. N:1047 sets editor.path
//      = /life/Pilaf.md. N returns.
//   5. Control unwinds back through syncCurrentEditor, and then to P:984
//      ("Finished syncing previous file").
//
// At this exact moment:
//   - currentEditor is editor (main) — reassigned by N. P never noticed.
//   - editor.path = /life/Pilaf.md, editor content = Pilaf UPDATED
//   - P's local variables still say path = /hap/Awareness.md, el =
//     'editor2-textarea'
//
//   6. P:1028 — currentEditor.path = path — writes editor.path =
//      /hap/Awareness.md. This is where the drift is sealed. editor's
//      content is still Pilaf; its .path is now Awareness.
//   7. P:1037–1044 — branches on el === 'editor2-textarea', so it
//      reinitializes editor2 only, not editor. Loads Awareness into
//      editor2.
//   8. P returns.
//
// State after P returns:
//   - editor — path = /hap/Awareness.md, content = Pilaf. Poisoned.
//   - editor2 — path = /hap/Awareness.md, content = Awareness. Clean.
//
// --- Why the copy happened later ---
//
// The rename-from-header block at files.js:1152–1219 triggers whenever
// syncCurrentEditor runs against the poisoned editor. That requires two
// things:
//
//   1. The periodic saver (setInterval(..., 1000ms) at files.js:1662)
//      fires.
//   2. Global currentEditor points at the poisoned editor at that moment.
//
// (2) is the trigger. editor.js:47 reassigns currentEditor on any focus
// event. In the original session, focus returned to the main editor at
// some point (the user probably clicked on it when they saw the unexpected
// Pilaf content). Next saver tick:
//   - path = editor.path = /hap/Awareness.md
//   - firstLine = '# Pilaf', filename = 'Awareness.md'
//     → hasFilenameChanged === true
//   - remove('/hap/Awareness.md') → Awareness deleted
//   - writeIfContentIsDifferent('/hap/Pilaf.md', ...) with the Pilaf
//     content → hap/Pilaf.md created
//
// That's what the server log showed:
//   Creating one clientFile: '/happiness/Плов, Pilaf.md' at 07:19:39,
//   deleting file: 'happiness/0 Осознанное расслабление.md' at 07:19:46.
//
// --- The root cause in one line ---
//
// openFile kept setting .path on the global currentEditor without
// re-verifying that currentEditor still pointed at the slot it targeted
// (el). The await at P:983 surrendered control, a nested openFile rotated
// the global, and P:1028 wrote the new path onto the wrong editor
// instance.
//
// The current switchAwayEditor fix prevents the nested openFile from ever
// running, so the rotation doesn't happen, so P:1028 ends up writing onto
// the correct editor. It blocks this specific door. It does not disarm
// the executioner (rename-from-header), nor does it fix the "P doesn't
// re-verify currentEditor" problem — any future code path that rotates
// currentEditor during P's await would poison again.
test('pilaf should not be copied to happiness when opening link in editor2 after stale editor2 drift', async ({page}) => {
    await page.evaluate(async () => {
        const seedRoot = await navigator.storage.getDirectory();
        const hapDir = await seedRoot.getDirectoryHandle('hap', {create: true});
        const lifeDir = await seedRoot.getDirectoryHandle('life', {create: true});

        const write = async (dir, name, content) => {
            const handle = await dir.getFileHandle(name, {create: true});
            const writable = await handle.createWritable();
            await writable.write(content);
            await writable.close();
        };

        await write(hapDir, 'Dream.md', 'Dream body [Awareness](Awareness.md)');
        await write(hapDir, 'Awareness.md', 'Awareness body');
        await write(lifeDir, 'Pilaf.md', 'Pilaf recipe');
        await write(lifeDir, 'Recipes.md', 'Recipes list [Pilaf](Pilaf.md)');

        window.getTemporaryStorageDirHandle = async function () {
            return await navigator.storage.getDirectory();
        };
    });

    await page.evaluate(() => {
        init(document.getElementById('editor'));
    });

    await page.waitForTimeout(500);

    const nodeSel = (name) => `#tree .tree-item:text-is('${name}')`;
    const expand = async (dir) => {
        const locator = page.locator(nodeSel(dir));
        const isExpanded = await locator.evaluate(el => el.classList.contains('expanded'));
        if (!isExpanded) {
            await locator.click();
            await page.waitForTimeout(100);
        }
    };

    await expand('life');
    await page.click(nodeSel('Recipes'));
    await page.waitForTimeout(300);

    await page.evaluate(() => editor.hmdReadLink('Pilaf'));
    await page.waitForTimeout(500);

    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);

    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const lifeDir = await root.getDirectoryHandle('life');
        const handle = await lifeDir.getFileHandle('Pilaf.md');
        const writable = await handle.createWritable();
        await writable.write('Pilaf recipe UPDATED externally');
        await writable.close();
    });
    await page.waitForTimeout(200);

    await expand('hap');
    await page.click(nodeSel('Dream'));
    await page.waitForTimeout(300);

    await page.evaluate(() => editor.hmdReadLink('Awareness'));

    // Wait past the periodic saver (CURRENT_FILE_SYNC_INTERVAL = 1000ms) so any
    // pending rename-from-header operation would have fired by now.
    await page.waitForTimeout(2000);

    const disk = await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const listDir = async (name) => {
            const dir = await root.getDirectoryHandle(name);
            const names = [];
            for await (const entry of dir.values()) {
                names.push(entry.name);
            }
            return names.sort();
        };
        return {
            hap: await listDir('hap'),
            life: await listDir('life'),
        };
    });

    expect(disk.hap).toEqual(['Awareness.md', 'Dream.md']);
    expect(disk.life).toEqual(['Pilaf.md', 'Recipes.md']);
});


async function expectCurrentContent(page, content) {
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe(content);
}

async function setup(page) {
    await page.goto('/index.html');

    await page.evaluate(()=> {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();

            const files = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: 'Some text' }
            ];

            for (const file of files) {
                try {
                    await root.getFileHandle(file.name);
                } catch (error) {
                    const fileHandle = await root.getFileHandle(file.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(file.content);
                    await writable.close();
                }
            }

            return root;
        };
    })

    await page.evaluate(() => {
        init(document.getElementById('editor'));
    });

    await page.waitForSelector('#tree', {timeout: 5000});
}