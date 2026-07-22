const {test, expect: baseExpect} = require('@playwright/test');
const fs = require('fs').promises;
const path = require('path');
const crypto = require('crypto');

const expect = baseExpect.extend({
    toBeNumberOrNull(received) {
        const pass = received === null || (typeof received === 'number' && !isNaN(received));

        if (pass) {
            return {
                message: () => `expected ${received} not to be a number or null`,
                pass: true,
            };
        } else {
            return {
                message: () => `expected ${received} to be a number or null, but got ${typeof received}`,
                pass: false,
            };
        }
    }
});


const getServerDir = (workerIndex) => `../storage/${currentWorkerIndex}`;
const getTokensDir = () => `../storage/-1`;
let currentWorkerIndex = '-1';

test.beforeEach(async ({page}, testInfo) => {
    currentWorkerIndex = testInfo.workerIndex.toString();

    await fs.rm(getServerDir(), { recursive: true, force: true });
    await fs.mkdir(getServerDir(), { recursive: true });
    await fs.mkdir(getTokensDir(), { recursive: true });

    await fs.writeFile(getTokensDir() + `/` + saltToken(currentWorkerIndex), currentWorkerIndex, 'utf8');

    // Capture page console output and attach it to the Playwright report so
    // failures show what the browser was logging. Each line ends up in the
    // per-test "stdout" attachment, visible in the HTML report.
    const logs = [];
    page.on('console', (msg) => {
        logs.push(`[${msg.type()}] ${msg.text()}`);
    });
    page.on('pageerror', (err) => {
        logs.push(`[pageerror] ${err.message}`);
    });
    testInfo.__pageLogs = logs;
});

test.afterEach(async ({}, testInfo) => {
    const logs = testInfo.__pageLogs;
    if (!logs || logs.length === 0) return;
    await testInfo.attach('page-console.log', {
        body: logs.join('\n'),
        contentType: 'text/plain',
    });
});

async function setup(page) {
    await page.addInitScript(() => {
        localStorage.setItem('apiUrl', 'http://localhost:8080');
        localStorage.setItem('lastServerOk', 1);
    });

    // Token is read from a cookie by the server; set it on the shared
    // `localhost` domain so it's sent with both the app's (localhost:3000)
    // and the API's (localhost:8080) requests.
    await page.context().addCookies([{
        name: 'token',
        value: String(currentWorkerIndex),
        domain: 'localhost',
        path: '/',
    }]);

    await page.goto('/index.html');

    await page.evaluate(()=> {
        window.getTemporaryStorageDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const subdir = await root.getDirectoryHandle('subdir', { create: true });

            const files = [
                { name: 'README.md', content: 'Hello world' },
                { name: 'Notes.md', content: 'Some Text' }
            ];

            for (const file of files) {
                const fileHandle = await root.getFileHandle(file.name, { create: true });
                const writable = await fileHandle.createWritable();
                await writable.write(file.content);
                await writable.close();
            }

            await root.getFileHandle('Chat.md', { create: true });
            const fileHandle =  await root.getFileHandle('config.json', { create: true });
            const writable = await fileHandle.createWritable()
            await writable.write('{}');
            await writable.close();

            return root;
        };
    })

    await page.evaluate(() => {
        init(document.getElementById('editor'));
    });

    await page.waitForSelector('#chat', {timeout: 10000});
    await page.waitForSelector('#tree', {timeout: 5000});
}

test('sync new files from server', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome Text');
    await page.click(`#tree .tree-item:has-text('README')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    // Check that new files are added
    await page.click(`#tree .tree-item:has-text('File')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await page.click(`#tree .tree-item:has-text('Another')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Another\n*italic*');
});

test('sync new files from client', async ({ page }) => {
    await setup(page);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Content');
    await page.waitForTimeout(3000);

    await expectFileOnServer(page, 'New file.md', 'Content');
});

test('sync new files from client, ignore current file in syncFiles', async ({ page }) => {
    await setup(page);

    const consoleMessages = [];
    page.on('console', msg => {
        consoleMessages.push({ type: msg.type(), text: msg.text() });
    });

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Content');

    // Two blurs: first primes server timestamps (syncFiles takes the
    // "NEVER SYNCED BEFORE" path on the very first call), second is the
    // one where the receive-side skip at files.js:229 actually fires —
    // server echoes the current file and client should log the skip.
    await page.evaluate(() => window.dispatchEvent(new Event('blur')));
    await page.waitForTimeout(1500);
    await page.evaluate(() => window.dispatchEvent(new Event('blur')));
    await page.waitForTimeout(2000);

    await expectFileOnServer(page, 'New file.md', 'Content');

    // log() in app.js wraps messages with %c style directives, so match
    // the substring rather than an exact text.
    const skipped = consoleMessages.some(m =>
        m.type === 'log' && m.text.includes('Skip receiving current file during bath sync /New file.md')
    );
    expect(skipped).toBe(true);
});

test('sync existing files from client', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await page.click(`#tree .tree-item:has-text('README')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome Text');
    // Trigger syncFiles, first time to get server state
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(500);

    // Trigger syncFiles, second time to send client files
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(500);

    // Check that existing files from client are synced
    await expectFileOnServer(page, 'README.md', 'Hello world');
    await expectFileOnServer(page, 'Notes.md', 'Some Text');

    // Check that existing server files are preserved
    await page.click(`#tree .tree-item:has-text('file')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await page.click(`#tree .tree-item:has-text('another')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Another\n*italic*');
});

test('get changes for current file from server', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await page.click(`#tree .tree-item:has-text('file')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await expectCurrentContent(page, '# File\ntest content');

    await createFileOnServer('File.md', 'test content\nadded');
    await createFileOnServer('File2.md', 'test content\nadded2');
    await page.waitForTimeout(2000);
    // await page.pause();
    await expectCurrentContent(page, '# File\ntest content\nadded');
});

test('send changes from current file to server', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await page.click(`#tree .tree-item:has-text('file')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await expectCurrentContent(page, '# File\ntest content');

    // Update file on server, check that changes received
    await createFileOnServer('File.md', 'test content\nadded');
    await page.waitForTimeout(2000);
    await expectCurrentContent(page, '# File\ntest content\nadded');

    // Place cursor at the end
    await page.keyboard.press('Meta+ArrowDown');
    await page.keyboard.press('Enter');

    await page.keyboard.type('added from client');
    await page.waitForTimeout(2000);
    await expectFileOnServer(page, 'file.md', 'test content\nadded\nadded from client');
});

test('changed on both client and serve, should merge', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');

    await setup(page);

    await page.click(`#tree .tree-item:has-text('file')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    // Disable sync by intercepting every API call and aborting it. Cleaner
    // than clearing the auth cookie — no "Wrong token" noise on the server,
    // and the client just gets a network error (which syncLocalFileWithServer
    // already handles silently).
    const API_HOST = 'localhost:8080';
    await page.route(`http://${API_HOST}/**`, (route) => route.abort());

    // Modify on server
    await createFileOnServer('File.md', 'test content\nadded from server');
    await page.waitForTimeout(2000);

    // Modify on client
    await page.keyboard.press('Meta+ArrowDown');
    await page.keyboard.press('Enter');
    await page.keyboard.type('addded from client');

    // Enable sync
    await page.unroute(`http://${API_HOST}/**`);

    await page.waitForTimeout(2000);
    await expectFileOnServer(page, 'File.md', 'test content\nadded from server\naddded from client');
    await expectCurrentContent(page, '# File\ntest content\nadded from server\naddded from client');
});

test("sync one new file from client doesn't conflict with syncFiles", async ({ page }) => {
    await setup(page);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('abc');
    await page.keyboard.type('def');
    await page.keyboard.type('abc');

    await page.waitForTimeout(500);

    // Trigger syncFiles
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.keyboard.type('def');

    await page.keyboard.press('Enter');
    await page.keyboard.type('def');
    await page.keyboard.press('Enter');
    await page.keyboard.type('Content');
    await page.waitForTimeout(3000);

    await expectFileOnServer(page, 'New file.md', 'abcdefabcdef\ndef\nContent');
});

test('delete files on client will propagate to server as well', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);

    await page.waitForTimeout(300);

    await page.click(`#tree .tree-item:has-text('Notes')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome Text');
    await page.click(`#tree .tree-item:has-text('README')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    await page.click(`#tree .tree-item:has-text('file')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await page.click(`#tree .tree-item:has-text('another')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Another\n*italic*');
    await page.click(`#tree .tree-item:has-text('another')`);

    await page.waitForTimeout(200);

    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Another\n*italic*');
    await page.keyboard.press('Meta+d');

    await page.waitForTimeout(1000);

    // syncFiles should propagate deletion to server
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(500);

    // await page.pause();

    expectFileOnServer(page, 'File.md', 'test content');
    expectNoFileOnServer(page, 'another.md');
});

test('files exist on both client and server, config is not removed on first sync', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);
    await page.waitForTimeout(300);

    // Check that existing files are not removed
    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome Text');
    await page.click(`#tree .tree-item:has-text('README')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    // Check that new files are added
    await page.click(`#tree .tree-item:has-text('file')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await page.click(`#tree .tree-item:has-text('another')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Another\n*italic*');
    // Trigger syncFiles
    await page.waitForTimeout(300);
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });
    await page.waitForTimeout(300);

    await expectFileOnServer(page, 'File.md', 'test content');
    await expectFileOnServer(page, 'Another.md', '*italic*');

    const configExists = await fs.access(path.join(getServerDir(), 'config.json')).then(() => true).catch(() => false);
    expect(configExists).toBe(true);
});

test('files exist on both client and server, serverFiles contains proper server files', async ({ page }) => {
    await createFileOnServer('File.md', 'test content');
    await createFileOnServer('dir/File2.md', 'test content2');
    await createFileOnServer('Another.md', '*italic*');

    await setup(page);
    await page.waitForTimeout(300);

    // Check that existing files are not removed
    await page.click(`#tree .tree-item:has-text('Notes')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Notes\nSome Text');
    await page.click(`#tree .tree-item:has-text('README')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# README\nHello world');
    // Check that new files are added
    await page.click(`#tree .tree-item:has-text('File')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File\ntest content');
    await page.click(`#tree .tree-item:has-text('Another')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# Another\n*italic*');
    // Trigger syncFiles
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });
    await page.waitForTimeout(300);

    await expectFileOnServer(page, 'File.md', 'test content');
    await expectFileOnServer(page, 'Another.md', '*italic*');

    let filesOnServer = await page.evaluate(() => {
        return server['files'];
    });
    expect(filesOnServer).toEqual({
        'Chat.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/Chat.md'
        },
        'Notes.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/Notes.md'
        },
        'README.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/README.md'
        },
        'Another.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.toBeNumberOrNull(),
            lastClientModified: expect.any(Number),
            path: '/Another.md'
        },
        'config.json': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/config.json'
        },
        'dir/': {
            'File2.md': {
                hash: expect.any(Number),
                isFile: true,
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/dir/File2.md'
            }
        },
        'File.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/File.md'
        },
        'happiness/': {
            'Boredom is just an emotion.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/happiness/Boredom is just an emotion.md'
            },
            'Abundant meditation.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/happiness/Abundant meditation.md'
            }
        },
        '🪴 Welcome.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/🪴 Welcome.md'
        },
        'Hotkeys.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/Hotkeys.md'
        },
        'Links.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/Links.md'
        },
        'My Project.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/My Project.md'
        },
        'Markdown Guide.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: expect.any(Number),
            path: '/Markdown Guide.md'
        },
        'brain/': {
            'Change your environment instead of using willpower.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/brain/Change your environment instead of using willpower.md'
            },
            'Brain is the most complex object in known universe.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/brain/Brain is the most complex object in known universe.md'
            },
            'We think that we understand, but in reality we just know.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/brain/We think that we understand, but in reality we just know.md'
            },
            'Zettelkasten.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: expect.any(Number),
                path: '/brain/Zettelkasten.md'
            }
        },
    });

    // await page.pause();
    if (!await page.locator(`#tree .tree-item:text-is('dir')`).evaluate(el => el.classList.contains("expanded"))) {
        await page.click(`#tree .tree-item:text-is('dir')`);
        await page.waitForTimeout(100);
    }
    await page.click(`#tree .tree-item:has-text('file2')`);
    await page.waitForTimeout(200);
    expect(await page.evaluate(() => document.querySelector(".CodeMirror").CodeMirror.getValue())).toBe('# File2\ntest content2');
});


test('sync changes from client, update clientLastModified & lastClientModified', async ({ page }) => {
    await setup(page);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Content');
    await page.waitForTimeout(3000);

    await expectFileOnServer(page, 'New file.md', 'Content');

    let serverFiles = await page.evaluate(() => {
        return server['files'];
    });

    let clientFileLastModified = await page.evaluate(async () => {
        let fileHandle = await getFileHandle('/New file.md');
        let file = await fileHandle.getFile();

        return file.lastModified;
    });

    expect(serverFiles['New file.md'].lastModified).not.toBeNull();
    expect(serverFiles['New file.md'].lastClientModified).toEqual(clientFileLastModified);
});

async function createFileOnServer(filepath, content) {
    const p = path.join(getServerDir(), filepath);

    // Create all intermediate directories
    await fs.mkdir(path.dirname(p), { recursive: true });

    await fs.writeFile(p, content, 'utf8');
}

async function expectFileOnServer(page, filepath, expectedContent) {
    const p = path.join(getServerDir(), filepath);
    const actualContent = await fs.readFile(p, 'utf8');

    expect(actualContent).toBe(expectedContent);
}

async function expectNoFileOnServer(page, filepath) {
    const p = path.join(getServerDir(), filepath);

    const exists = await fs.access(p).then(() => true).catch(() => false);
    expect(exists).toBe(false);
}

function saltToken(token, salt = '') {
    return crypto.createHash('sha256')
        .update(token + salt)
        .digest('hex');
}


async function expectCurrentContent(page, content) {
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe(content);
}

