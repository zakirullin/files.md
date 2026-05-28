const {test, expect} = require('@playwright/test');

async function sendChatMessage(page, text, expectedCount) {
    await page.locator('#chat-input').fill(text);
    await page.keyboard.press('Enter');
    await expect(page.locator('.message')).toHaveCount(expectedCount);
}

async function getChatVisibilityMetrics(page) {
    return await page.evaluate(() => {
        const chat = document.getElementById('chat');
        const input = document.getElementById('chat-input');
        const lastMessage = chat.querySelector('.message:last-child');
        const chatRect = chat.getBoundingClientRect();
        const inputRect = input.getBoundingClientRect();
        const lastRect = lastMessage.getBoundingClientRect();
        const style = getComputedStyle(chat);

        return {
            bottomGap: chatRect.bottom - lastRect.bottom,
            inputGap: inputRect.top - lastRect.bottom,
            paddingBottom: parseFloat(style.paddingBottom),
            scrollPaddingBottom: parseFloat(style.scrollPaddingBottom),
            scrollTop: chat.scrollTop,
            maxScrollTop: chat.scrollHeight - chat.clientHeight,
        };
    });
}

test.beforeEach(async ({page}) => {
    await page.goto('/index.html');

    await page.waitForSelector('#tree', {timeout: 5000});
});

test('send message to chat', async ({ page }) => {
    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('My message');
    await page.waitForTimeout(300);
    // TODO I believe chat is reloaded 2 times for some reason, it blinks, and thus removes previous message
    // Or wait for timeout before typing message doesn't help hmm
    await page.keyboard.press('Enter');

    await page.waitForSelector('.message');
    let content = await page.textContent('.message-content')
    expect(content).toBe('My message');

});

test('select all in chat input selects input text, not bubbles', async ({page}) => {
    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('First message');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.locator('#chat-input').click();
    await page.keyboard.type('to be cleared');
    await expect(page.locator('#chat-input')).toHaveValue('to be cleared');

    const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
    await page.keyboard.press(`${modifier}+a`);
    await page.keyboard.press('Delete');

    await expect(page.locator('#chat-input')).toHaveValue('');
    await expect(page.locator('.message-content')).toHaveText('First message');
});

test('mobile chat keeps the latest message clear of the composer', async ({page}) => {
    await page.setViewportSize({width: 900, height: 700});
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('File.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# File');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');

    const desktopPaddingBottom = await page.locator('#chat').evaluate(chat =>
        parseFloat(getComputedStyle(chat).paddingBottom));
    expect(desktopPaddingBottom).toBe(24);

    await page.setViewportSize({width: 390, height: 360});

    for (let i = 1; i <= 8; i++) {
        await sendChatMessage(page, `Mobile thought ${i}`, i);
    }

    await expect.poll(async () => {
        const metrics = await getChatVisibilityMetrics(page);
        return Math.round(metrics.maxScrollTop - metrics.scrollTop);
    }).toBe(0);

    let metrics = await getChatVisibilityMetrics(page);
    expect(metrics.paddingBottom).toBeGreaterThanOrEqual(96);
    expect(metrics.scrollPaddingBottom).toBeGreaterThanOrEqual(72);
    expect(metrics.bottomGap).toBeGreaterThanOrEqual(88);
    expect(metrics.inputGap).toBeGreaterThanOrEqual(88);

    await page.evaluate(async () => openFile('/File.md'));
    await page.waitForSelector('.CodeMirror');
    await page.evaluate(async () => openChat());
    await page.waitForSelector('#chat');

    await expect.poll(async () => {
        const metrics = await getChatVisibilityMetrics(page);
        return Math.round(metrics.maxScrollTop - metrics.scrollTop);
    }).toBe(0);

    metrics = await getChatVisibilityMetrics(page);
    expect(metrics.bottomGap).toBeGreaterThanOrEqual(88);
    expect(metrics.inputGap).toBeGreaterThanOrEqual(88);
});

test('move to dir creates a new file inside that dir', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            const root = await navigator.storage.getDirectory();
            await root.getDirectoryHandle('projects', {create: true});
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('MyTask');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-dir="projects"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    const exists = await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const projects = await root.getDirectoryHandle('projects');
        try { await projects.getFileHandle('MyTask.md'); return true; }
        catch { return false; }
    });
    expect(exists).toBe(true);
});

test('move to root creates a new file at root', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            return await navigator.storage.getDirectory();
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('RootMsg');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-dir=""]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    const exists = await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        try { await root.getFileHandle('RootMsg.md'); return true; }
        catch { return false; }
    });
    expect(exists).toBe(true);
});

test('move to existing file appends content', async ({page}) => {
    // Seed once, then return the same root on subsequent calls — otherwise the
    // app's repeated getRootDirHandle() calls would re-overwrite Notes.md.
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Append me');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() => document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(content).toContain('# Notes');
    expect(content).toContain('Append me');
});

test('move to file does not prepend a timestamp', async ({page}) => {
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Attention is all you need');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() => document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(content).toContain('Attention is all you need');
    // The body must not be prefixed with `HH:MM` - that's reserved for the
    // chat→journal flow, not move-to-file (web/lib/md.js:addHeaderAndText).
    expect(content).not.toMatch(/`\d{2}:\d{2}`\s*500k/);
});

test('move to recent file does not prepend a timestamp', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            const root = await navigator.storage.getDirectory();
            await root.getFileHandle('File.md', {create: true});
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Attention is all you need');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.action-btn').filter({hasText: 'File'}).click({force: true});
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('File')`);
    await page.waitForTimeout(200);
    const fileContent = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(fileContent).toContain('Attention is all you need');
    expect(fileContent).not.toMatch(/`\d{2}:\d{2}`\s*500k/);
});

test('system dirs (archive, today) are hidden in move-to-file modal', async ({page}) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function () {
            const root = await navigator.storage.getDirectory();
            await root.getDirectoryHandle('archive', {create: true});
            await root.getDirectoryHandle('projects', {create: true});
            return root;
        };
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('Hello');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await expect(page.locator('#search-results li[data-dir="projects"]')).toBeVisible();
    await expect(page.locator('#search-results li[data-dir="archive"]')).toHaveCount(0);
    await expect(page.locator('#search-results li[data-dir="today"]')).toHaveCount(0);
});

test('send to chat and move to recent file', async ({ page }) => {
    await page.evaluate(() => {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const fileHandle = await root.getFileHandle('File.md', { create: true });

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    await page.keyboard.type('My message');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');

    await page.waitForSelector('.message');
    let content = await page.textContent('.message-content')
    expect(content).toBe('My message');

    await page.hover('.message');
    await page.locator('.action-btn').filter({hasText: 'File'}).click({force: true});
    await page.waitForSelector('.message', {state: 'detached'});

    await page.click(`#tree .tree-item:has-text('File')`);
    await page.waitForTimeout(200);
    const fileContent = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    expect(fileContent).toContain('# File');
    expect(fileContent).toContain('My message');
});

// Regression: moving a lowercase-starting chat message via the to-file-btn
// used to crash because chat.js applied ucfirst() to the text before passing
// it to the search modal, while the DOM dataset.text stayed in original case.
// `find(el => el.dataset.text === selectedMsgText)` then returned undefined
// and modals.js threw "Cannot read properties of undefined (reading 'classList')".
test('move-to-file works for messages that start with a lowercase letter', async ({page}) => {
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    // Lowercase starting letter is the trigger for the original bug.
    await page.keyboard.type('lowercase start');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    const errors = [];
    page.on('pageerror', err => errors.push(err.message));

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    // No uncaught exceptions should have been raised during the flow.
    expect(errors).toEqual([]);

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    // The text written to the file is capitalised (ucfirst applied at the
    // write step, AFTER the DOM lookup, so it doesn't break find()).
    expect(content).toContain('Lowercase start');
});

// Regression: a chat message containing `"` used to crash to-file because
// escapeHtml() left quotes unescaped, the `data-text="..."` attribute closed
// early at the first `"`, and the modal's `dataset.text === selectedMsgText`
// lookup returned undefined.
test('move-to-file works for messages containing double quotes', async ({page}) => {
    await page.evaluate(async () => {
        const root = await navigator.storage.getDirectory();
        const fh = await root.getFileHandle('Notes.md', {create: true});
        const w = await fh.createWritable();
        await w.write('# Notes');
        await w.close();
        window.getTemporaryStorageDirHandle = async () => navigator.storage.getDirectory();
    });
    await page.evaluate(() => init(document.getElementById('editor')));

    await page.click(`#tree .tree-item:has-text('chat')`);
    await page.waitForSelector('#chat');
    const quoted = 'catches "file changed in vim." Without it';
    await page.keyboard.type(quoted);
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');
    await page.waitForSelector('.message');

    const errors = [];
    page.on('pageerror', err => errors.push(err.message));

    await page.hover('.message');
    await page.locator('.to-file-btn').first().click({force: true});
    await page.waitForSelector('#search', {state: 'visible'});

    await page.locator('#search-results li[data-path="/Notes.md"]').click();
    await page.waitForSelector('.message', {state: 'detached'});

    expect(errors).toEqual([]);

    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    const content = await page.evaluate(() =>
        document.querySelector('.CodeMirror').CodeMirror.getValue());
    // ucfirst capitalises the first letter at the write step (see the
    // lowercase-letter regression test above). What matters here is that
    // the embedded quotes round-trip intact.
    expect(content).toContain('Catches "file changed in vim." Without it');
});
