const {test, expect} = require('@playwright/test');

test.beforeEach(async ({page}, testInfo) => {
    // Capture browser console output and pageerrors for the whole test,
    // so when something flakes we can read what the app was actually
    // doing. The afterEach below attaches the buffer to the Playwright
    // report; failures end up with a `page-console.log` artifact next to
    // the screenshot/video.
    const logs = [];
    page.on('console', (msg) => {
        logs.push(`[${msg.type()}] ${msg.text()}`);
    });
    page.on('pageerror', (err) => {
        logs.push(`[pageerror] ${err.message}`);
    });
    testInfo.__pageLogs = logs;

    // Playwright doesn't isolate OPFS per parallel tests, it seems.
    await page.goto('/robots.txt');
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        for await (const entry of root.values()) {
            try { await root.removeEntry(entry.name, { recursive: true }); } catch (_) {}
        }
    });

    await page.goto('/index.html');
    await page.waitForSelector('#tree', {timeout: 5000});
});

test.afterEach(async ({}, testInfo) => {
    const logs = testInfo.__pageLogs;
    if (!logs || logs.length === 0) return;
    if (testInfo.status !== testInfo.expectedStatus) {
        // Print to stdout so failures show the browser console inline -
        // attachments end up hashed under playwright-report/data/ and are
        // a pain to find in CI.
        console.log(`\n--- browser console for "${testInfo.title}" ---\n${logs.join('\n')}\n--- end ---`);
    }
    await testInfo.attach('page-console.log', {
        body: logs.join('\n'),
        contentType: 'text/plain',
    });
});

async function setMarkdownAndCursor(page, cursor = {line: 1, ch: 10}) {
    await page.evaluate((cursorPos) => {
        editor.setValue('# Heading\n**bold** text');
        editor.setCursor(cursorPos);
        editor.focus();
        editor.refresh();
    }, cursor);
}

async function getBoldTokenState(page) {
    return await page.evaluate(() => {
        const token = document.querySelector('#editor-container .cm-formatting-strong.hmd-hidden-token');
        const style = token ? getComputedStyle(token) : null;

        return {
            hasRawLine: !!document.querySelector('#editor-container .hmd-raw-editing-line'),
            hasHiddenStrongToken: !!token,
            fontSize: style ? style.fontSize : null,
            color: style ? style.color : null,
        };
    });
}

test('should load the Files.md editor', async ({page}) => {
    await expect(page).toHaveTitle('Files.md');

    await expect(page.locator('#sidebar')).toBeVisible();
    await expect(page.locator('#open-folder')).toBeVisible();
});

test('raw editing line preference is off by default', async ({page}) => {
    await setMarkdownAndCursor(page);

    await expect.poll(async () => {
        const state = await getBoldTokenState(page);
        return state.hasHiddenStrongToken;
    }).toBe(true);

    const state = await getBoldTokenState(page);
    expect(state.hasRawLine).toBe(false);
    expect(parseFloat(state.fontSize)).toBeLessThanOrEqual(1);
    expect(await page.locator('#raw-editing-line-toggle').getAttribute('aria-pressed')).toBe('false');
    expect(await page.evaluate(() => localStorage.getItem('rawEditingLine'))).toBe(null);
    expect(await page.evaluate(() => editor.getOption('hmdRawEditingLine'))).toBe(false);
});

test('raw editing line preference toggles live and applies to both editors', async ({page}) => {
    await setMarkdownAndCursor(page);
    await page.click('#raw-editing-line-toggle');

    await expect(page.locator('#raw-editing-line-toggle')).toHaveAttribute('aria-pressed', 'true');
    await expect.poll(async () => {
        return await page.evaluate(() => ({
            stored: localStorage.getItem('rawEditingLine'),
            editor: editor.getOption('hmdRawEditingLine'),
            editor2: editor2.getOption('hmdRawEditingLine'),
        }));
    }).toEqual({stored: 'true', editor: true, editor2: true});

    await expect.poll(async () => {
        const state = await getBoldTokenState(page);
        return state.hasRawLine;
    }).toBe(true);

    let state = await getBoldTokenState(page);
    expect(state.hasHiddenStrongToken).toBe(true);
    expect(parseFloat(state.fontSize)).toBeGreaterThan(5);
    expect(state.color).not.toBe('rgba(0, 0, 0, 0)');

    await page.evaluate(() => {
        showEditor2();
        editor2.setValue('# Split\n**bold** text');
        editor2.setCursor({line: 1, ch: 10});
        editor2.focus();
        editor2.refresh();
    });

    await expect.poll(async () => {
        return await page.evaluate(() => !!document.querySelector('#editor2-container .hmd-raw-editing-line'));
    }).toBe(true);

    await page.click('#raw-editing-line-toggle');
    await expect(page.locator('#raw-editing-line-toggle')).toHaveAttribute('aria-pressed', 'false');
    await expect.poll(async () => {
        const offState = await getBoldTokenState(page);
        return {
            stored: await page.evaluate(() => localStorage.getItem('rawEditingLine')),
            option: await page.evaluate(() => editor.getOption('hmdRawEditingLine')),
            hasRawLine: offState.hasRawLine,
        };
    }).toEqual({stored: 'false', option: false, hasRawLine: false});
});

test('raw editing line preference persists after reload', async ({page}) => {
    await page.click('#raw-editing-line-toggle');
    await expect(page.locator('#raw-editing-line-toggle')).toHaveAttribute('aria-pressed', 'true');

    await page.reload();
    await page.waitForSelector('#tree', {timeout: 5000});
    await setMarkdownAndCursor(page);

    await expect.poll(async () => {
        return await page.evaluate(() => ({
            stored: localStorage.getItem('rawEditingLine'),
            option: editor.getOption('hmdRawEditingLine'),
            hasRawLine: !!document.querySelector('#editor-container .hmd-raw-editing-line'),
        }));
    }).toEqual({stored: 'true', option: true, hasRawLine: true});
});

test('should open markdown file via quick panel and see bold text formatting', async ({page}) => {
    const isMac = process.platform === 'darwin';
    const modifier = isMac ? 'Meta' : 'Control';
    await page.keyboard.press(`${modifier}+k`);

    await page.waitForSelector('#search', {timeout: 3000});
    await page.locator('#search-input').fill('Markdown');
    await page.keyboard.press('Enter');

    // Read the main editor's value directly. Two CodeMirror instances live
    // in the DOM (editor + editor2), so a `.CodeMirror` selector hits a
    // strict-mode violation, and `.first()` is racy because either editor
    // may finish initialising sooner than the other under load.
    await expect.poll(async () => {
        return await page.evaluate(() => window.editor && window.editor.getValue && window.editor.getValue());
    }).toContain('**Bold text**');

    const codeMirrorContent = await page.evaluate(() => window.editor.getValue());
    expect(codeMirrorContent).toContain('**Bold text**');
    expect(codeMirrorContent).toContain('**bold**');
    expect(codeMirrorContent).toContain('using');
});

test('insert link', async ({page}) => {
    await page.click('#sidebar >> text=Welcome');

    await page.click('.CodeMirror');
    await page.keyboard.press('Meta+a');
    await page.keyboard.press('Delete');
    await page.keyboard.type('[markdown');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(500);
    const content = await page.locator('.CodeMirror-code').textContent();

    console.log('Content:', content);
    expect(content).toContain('[Markdown Guide](/Markdown%20Guide.md)');
});

test('should handle text selection correctly', async ({page}) => {
    test.skip(!process.env.RUN_SELECTION, 'pixel-dependent; run with RUN_SELECTION=1');
    // Add some test content with various markdown elements
    await page.click('#sidebar >> text=Welcome');
    await page.waitForTimeout(500);
    await page.keyboard.press('Control+a');
    await page.keyboard.press('Delete');

    const testContent = `# Heading
**Bold text** and normal text
\`inline code\` with more text
[Link text](url)`;

    await page.keyboard.type(testContent);
    await page.waitForTimeout(500);

    // Test 1: Select all text
    await page.keyboard.press('Control+a');
    await page.waitForTimeout(500);

    // Check if selection div is created with proper positioning
    const allSelections = page.locator('.CodeMirror-selected');
    let count = await allSelections.count();
    expect(count).toEqual(4);

    const expectedSelections = [
        { left: 2, width: 139, right: 141 },
        { left: 2, width: 95, right: 97 },
        { left: 2, width: 188, right: 190 },
        { left: 2, width: 223, right: 225 },
    ];

    for (let i = 0; i < count; i++) {
        const selection = allSelections.nth(i);

        const selectionData = await selection.evaluate(el => {
            const style = window.getComputedStyle(el);
            const left = parseInt(style.left);
            const width = parseInt(style.width);
            return {
                left: left,
                width: width,
                right: left + width
            };
        });

        expect(selectionData.left).toBe(expectedSelections[i].left);
        expect(selectionData.width).toBe(expectedSelections[i].width);
        expect(selectionData.right).toBe(expectedSelections[i].right);
    }
});

test('should handle text selection for word-wrap content', async ({page}) => {
    test.skip(!process.env.RUN_SELECTION, 'pixel-dependent; run with RUN_SELECTION=1');
    // Add some test content with various markdown elements
    await page.click('#sidebar >> text=Welcome');
    await page.waitForSelector('.CodeMirror');
    await page.keyboard.press('Meta+a');
    await page.keyboard.press('Delete');
    await page.waitForTimeout(200);

    const testContent = `Lorem ipsum dolor\nLorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`;

    await page.keyboard.type(testContent);
    await page.waitForTimeout(500);

    // Test 1: Select all text
    await page.keyboard.press('Control+a');
    await page.waitForTimeout(200);

    // Check if selection div is created with proper positioning
    const allSelections = page.locator('.CodeMirror-selected');
    let count = await allSelections.count();
    expect(count).toEqual(10);

    const expectedSelections = [
        { left: 2, width: 138, right: 140 },
        { left: 2, width: 742, right: 744 },
        { left: 2, width: 740, right: 742 },
        { left: 2, width: 753, right: 755 },
        { left: 2, width: 738, right: 740 },
        { left: 2, width: 718, right: 720 },
        { left: 2, width: 746, right: 748 },
        { left: 2, width: 702, right: 704 },
        { left: 2, width: 691, right: 693 },
        { left: 2, width: 503, right: 505 },
    ];

    for (let i = 0; i < count; i++) {
        const selection = allSelections.nth(i);

        const selectionData = await selection.evaluate(el => {
            const style = window.getComputedStyle(el);
            const left = parseInt(style.left);
            const width = parseInt(style.width);
            return {
                left: left,
                width: width,
                right: left + width
            };
        });

        expect(selectionData.left).toBe(expectedSelections[i].left);
        expect(selectionData.width).toBe(expectedSelections[i].width);
        expect(selectionData.right).toBe(expectedSelections[i].right);
    }
});

test('opening link in editor2 should not clobber main editor when stale editor2 has out-of-sync content', async ({page}) => {
    await page.evaluate(async () => {
        // Seed OPFS once, so external modifications aren't clobbered by repeated setup.
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

    // 1) Open Recipes in the main editor
    await expand('life');
    await page.click(nodeSel('Recipes'));
    await page.waitForTimeout(300);

    // 2) Click Pilaf link — opens Pilaf in editor2
    await page.evaluate(() => editor.hmdReadLink('Pilaf'));
    await page.waitForTimeout(500);

    // 3) Press Escape — editor2 is hidden but editor2.path stays = life/Pilaf.md
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);

    // 4) Modify Pilaf on disk from outside the editor (simulates server sync)
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const lifeDir = await root.getDirectoryHandle('life');
        const handle = await lifeDir.getFileHandle('Pilaf.md');
        const writable = await handle.createWritable();
        await writable.write('Pilaf recipe UPDATED externally');
        await writable.close();
    });
    await page.waitForTimeout(200);

    // 5) Open Dream in main editor
    await expand('hap');
    await page.click(nodeSel('Dream'));
    await page.waitForTimeout(300);

    // 6) Click Awareness link — should open in editor2
    await page.evaluate(() => editor.hmdReadLink('Awareness'));
    await page.waitForTimeout(1000);

    // Main editor must still hold Dream, not be poisoned with Pilaf content.
    const state = await page.evaluate(() => ({
        editorPath: editor.path,
        editorContent: editor.getValue(),
        editor2Path: editor2.path,
        editor2Content: editor2.getValue(),
    }));
    expect(state.editorPath).toBe('/hap/Dream.md');
    expect(state.editorContent).toBe('# Dream\nDream body [Awareness](Awareness.md)');
    expect(state.editor2Path).toBe('/hap/Awareness.md');
    expect(state.editor2Content).toBe('# Awareness\nAwareness body');
});

test('reopen link in editor2 after escape + switch shows target content, not empty', async ({page}) => {
    // Bug: open note1, click link to note2 (editor2 opens). Press Esc.
    // Open note3 in editor1. Open note1 in editor1. Click link to note2 -
    // editor2 used to come up empty instead of showing note2.
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const write = async (name, content) => {
            const handle = await root.getFileHandle(name, {create: true});
            const writable = await handle.createWritable();
            await writable.write(content);
            await writable.close();
        };
        await write('Note1.md', 'Note1 body [Note2](Note2.md)');
        await write('Note2.md', 'Note2 body');
        await write('Note3.md', 'Note3 body');

        window.getTemporaryStorageDirHandle = async function () {
            return await navigator.storage.getDirectory();
        };
    });

    await page.evaluate(() => {
        init(document.getElementById('editor'));
    });
    await page.waitForTimeout(500);

    const nodeSel = (name) => `#tree .tree-item:text-is('${name}')`;
    // Click the rendered Note2 link inside the main editor's content
    // (not editor2, where #editor2-container also has cm-link spans).
    const note2LinkInEditor1 = page.locator('#editor-container .cm-link:has-text("Note2")').first();

    // 1) Open Note1 in the main editor
    await page.click(nodeSel('Note1'));
    await page.waitForTimeout(300);

    // 2) Click Note2 link - opens Note2 in editor2
    await note2LinkInEditor1.click();
    await page.waitForTimeout(500);

    // 3) Press Escape - editor2 hidden, editor2.path stays = /Note2.md
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);

    // 4) Open Note3 in the main editor (sidebar click)
    await page.click(nodeSel('Note3'));
    await page.waitForTimeout(300);

    // 5) Re-open Note1 in main editor (sidebar click)
    await page.click(nodeSel('Note1'));
    await page.waitForTimeout(300);

    // 6) Click Note2 link again - editor2 should show Note2 body, not empty.
    await note2LinkInEditor1.click();
    await page.waitForTimeout(1000);

    // Assert on the rendered DOM of editor2's container, not editor2.getValue().
    // getValue() returns the logical buffer of the (possibly detached) editor
    // instance and can pass even when the wrapper was nuked from the DOM by an
    // earlier editor1 re-init - which is exactly the bug.
    const state = await page.evaluate(() => ({
        editorPath: editor.path,
        editor2Path: editor2.path,
        editor2Content: editor2.getValue(),
        editor2HasWrapper: !!document.querySelector('#editor2-container .CodeMirror'),
        editor2DomText: (document.querySelector('#editor2-container .CodeMirror-code')?.innerText || '').trim(),
    }));
    expect(state.editorPath).toMatch(/Note1\.md$/);
    expect(state.editor2Path).toMatch(/Note2\.md$/);
    expect(state.editor2HasWrapper).toBe(true);
    expect(state.editor2DomText).toContain('Note2 body');
    expect(state.editor2Content).toBe('# Note2\nNote2 body');
});

test('should handle partical text selection for word-wrap content', async ({page}) => {
    test.skip(!process.env.RUN_SELECTION, 'pixel-dependent; run with RUN_SELECTION=1');
    await page.click('#sidebar >> text=Welcome');
    await page.waitForTimeout(500);
    await page.keyboard.press('Meta+a');
    await page.keyboard.press('Delete');

    const testContent = `\`1400–1500\` Рассвет эпохи возрождения (особенно Флоренция, Рим, Венеция). Человек в центре. Развитие гуманизма: акцент на личность, разум, творчество человека. Наука и открытия расцвет астрономии, анатомии, математики (Коперник, Галилей, Леонардо да Винчи). Искусство – новые методы перспективы, реализма, анатомической точности. Великие художники: Леонардо, Микеланджело, Рафаэль, Боттичелли.`;

    await page.keyboard.type(testContent);
    await page.waitForTimeout(500);

    await page.evaluate(() => {
        editor.setSelection(
            { line: 1, ch: 84 },
            { line: 1, ch: 184 }
        );
    });
    await page.waitForTimeout(800);

    const allSelections = page.locator('.CodeMirror-selected');
    let count = await allSelections.count();
    expect(count).toEqual(2);

    const expectedSelections = [
        { left: 697, width: 62, right: 759 },
        { left: 2, width: 752, right: 754 },
    ];

    for (let i = 0; i < count; i++) {
        const selection = allSelections.nth(i);

        const selectionData = await selection.evaluate(el => {
            const style = window.getComputedStyle(el);
            const left = parseInt(style.left);
            const width = parseInt(style.width);
            return {
                left: left,
                width: width,
                right: left + width
            };
        });

        expect(selectionData.left).toBe(expectedSelections[i].left);
        expect(selectionData.width).toBe(expectedSelections[i].width);
        expect(selectionData.right).toBe(expectedSelections[i].right);
    }
});
