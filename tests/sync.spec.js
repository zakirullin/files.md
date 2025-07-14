const {test, expect} = require('@playwright/test');
const fs = require('fs').promises;
const path = require('path');
const crypto = require('crypto');

const getServerDir = (workerIndex) => `../storage/${currentWorkerIndex}`;
const getTokensDir = () => `../storage/-1`;
let currentWorkerIndex = '-1';

test.beforeEach(async ({page}, testInfo) => {
    currentWorkerIndex = testInfo.workerIndex.toString();

    await fs.rm(getServerDir(), { recursive: true, force: true });
    await fs.mkdir(getServerDir(), { recursive: true });
    await fs.mkdir(getTokensDir(), { recursive: true });

    await fs.writeFile(getTokensDir() + `/` + saltToken(currentWorkerIndex), currentWorkerIndex, 'utf8');
});

async function setup(page) {
    await page.addInitScript((workerIndex) => {
        window.API_HOST = 'http://localhost:8080';
        localStorage.setItem('token', workerIndex);
    }, currentWorkerIndex);

    await page.goto('/app.html');

    await page.evaluate(()=> {
        window.getRootDirHandle = async function() {
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

            await root.getFileHandle('Chat.txt', { create: true });
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
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'Notes', '# Notes\nSome Text');
    await clickAndExpectContent(page, 'README', '# README\nHello world');

    // Check that new files are added
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');
});

test('sync new files from client', async ({ page }) => {
    await setup(page);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Content');
    await page.waitForTimeout(3000);

    await expectFileOnServer(page, 'New file.md', 'Content');
});

test('sync new files from client, ignore current file in syncTexts', async ({ page }) => {
    await setup(page);

    const consoleMessages = [];
    page.on('console', msg => {
        consoleMessages.push({
            type: msg.type(),
            text: msg.text()
        });
    });

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('Content');

    // Trigger syncTexts
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(3000);

    await expectFileOnServer(page, 'New file.md', 'Content');

    expect(consoleMessages).toContainEqual({
        type: 'log',
        text: 'Skip receiving current file during bath sync /New file.md'
    });

});

test('sync existing files from client', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');
    await clickAndExpectContent(page, 'README', '# README\nHello world');
    await clickAndExpectContent(page, 'Notes', '# Notes\nSome Text');


    // Trigger syncTexts, first time to get server state
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(500);

    // Trigger syncTexts, second time to send client files
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(500);

    // Check that existing files from client are synced
    await expectFileOnServer(page, 'README.md', 'Hello world');
    await expectFileOnServer(page, 'Notes.md', 'Some Text');

    // Check that existing server files are preserved
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');
});

test('get changes for current file from server', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');
    // await page.waitForTimeout(1000);
    await createFileOnServer('another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await expectCurrentContent(page, '# File\ntest content');

    await createFileOnServer('file.md', 'test content\nadded');
    await createFileOnServer('file2.md', 'test content\nadded2');
    await page.waitForTimeout(2000);
    // await page.pause();
    await expectCurrentContent(page, '# File\ntest content\nadded');
});

test('send changes from current file to server', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await expectCurrentContent(page, '# File\ntest content');

    await createFileOnServer('file.md', 'test content\nadded');
    await page.waitForTimeout(2000);
    await expectCurrentContent(page, '# File\ntest content\nadded');

    // Place cursor at the end
    await page.keyboard.press('Meta+ArrowDown');
    await page.keyboard.press('Enter');

    await page.keyboard.type('addded from client');
    await page.waitForTimeout(2000);
    await expectFileOnServer(page, 'file.md', 'test content\nadded\naddded from client');
});

test('changed on both client and serve, should merge', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');

    await setup(page);

    await clickAndExpectContent(page, 'file', '# File\ntest content');

    // Disable sync
    await page.addInitScript((workerIndex) => {
        localStorage.removeItem('token')
    }, currentWorkerIndex);

    // Modify on server
    await createFileOnServer('file.md', 'test content\nadded from server');
    await page.waitForTimeout(2000);

    // Modify on client
    await page.keyboard.press('Meta+ArrowDown');
    await page.keyboard.press('Enter');
    await page.keyboard.type('addded from client');

    // Enable sync
    await page.addInitScript((workerIndex) => {
        localStorage.setItem('token', workerIndex);
    }, currentWorkerIndex);

    await page.waitForTimeout(2000);
    await expectFileOnServer(page, 'file.md', 'test content\nadded from server\naddded from client');
    await expectCurrentContent(page, '# File\ntest content\nadded from server\naddded from client');
});

test("sync one new file from client doesn't conflict with syncTexts", async ({ page }) => {
    await setup(page);

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('abc');
    await page.keyboard.type('def');
    await page.keyboard.type('abc');

    await page.waitForTimeout(500);

    // Trigger syncTexts
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
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);

    await page.waitForTimeout(300);

    await clickAndExpectContent(page, 'Notes', '# Notes\nSome Text');
    await clickAndExpectContent(page, 'README', '# README\nHello world');

    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');

    await clickAndExpectContent(page, 'another', '# Another\n*italic*');
    await page.keyboard.press('Meta+d');

    await page.waitForTimeout(1000);

    // SyncTexts should propagate deletion to server
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });

    await page.waitForTimeout(500);

    // await page.pause();

    expectFileOnServer(page, 'file.md', 'test content');
    expectNoFileOnServer(page, 'another.md');
});

test('files exist on both client and server, config is not removed on first sync', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);
    await page.waitForTimeout(300);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'Notes', '# Notes\nSome Text');
    await clickAndExpectContent(page, 'README', '# README\nHello world');

    // Check that new files are added
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');

    // Trigger syncTexts
    await page.waitForTimeout(300);
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });
    await page.waitForTimeout(300);

    await expectFileOnServer(page, 'file.md', 'test content');
    await expectFileOnServer(page, 'another.md', '*italic*');
    await expectFileOnServer(page, 'config.json', '{}');
});

test('files exist on both client and server, serverFiles contains proper server files', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('dir/file2.md', 'test content2');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);
    await page.waitForTimeout(300);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'Notes', '# Notes\nSome Text');
    await clickAndExpectContent(page, 'README', '# README\nHello world');

    // Check that new files are added
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');

    // Trigger syncTexts
    await page.evaluate(() => {
        window.dispatchEvent(new Event('focus'));
    });
    await page.waitForTimeout(300);

    await expectFileOnServer(page, 'file.md', 'test content');
    await expectFileOnServer(page, 'another.md', '*italic*');
    await expectFileOnServer(page, 'config.json', '{}');

    let filesOnServer = await page.evaluate(() => {
        return server['files'];
    });
    expect(filesOnServer).toEqual({
        'Chat.txt': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/Chat.txt'
        },
        'Notes.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/Notes.md'
        },
        'README.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/README.md'
        },
        'another.md': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/another.md'
        },
        'config.json': {
            hash: expect.any(Number),
            isFile: true,
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/config.json'
        },
        'dir/': {
            'file2.md': {
                hash: expect.any(Number),
                isFile: true,
                lastModified: expect.any(Number),
                lastClientModified: null,
                path: '/dir/file2.md'
            }
        },

        'file.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/file.md'
        },
        'happiness/': {
            'Boredom is just an emotion.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: null,
                path: '/happiness/Boredom is just an emotion.md'
            },
            'Meditation.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: null,
                path: '/happiness/Meditation.md'
            }
        },
        '🪴 Welcome.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/🪴 Welcome.md'
        },
        'Hotkeys.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/Hotkeys.md'
        },
        'Links.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/Links.md'
        },
        'My project.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/My project.md'
        },
        'Markdown Guide.md': {
            isFile: true,
            hash: expect.any(Number),
            lastModified: expect.any(Number),
            lastClientModified: null,
            path: '/Markdown Guide.md'
        },
        'brain/': {
            'Change your environment instead of using willpower.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: null,
                path: '/brain/Change your environment instead of using willpower.md'
            },
            'Brain is the most complex object in known universe.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: null,
                path: '/brain/Brain is the most complex object in known universe.md'
            },
            'We think that we understand, but in reality we just know.md': {
                isFile: true,
                hash: expect.any(Number),
                lastModified: expect.any(Number),
                lastClientModified: null,
                path: '/brain/We think that we understand, but in reality we just know.md'
            }
        },
    });

    // await page.pause();
    await clickAndExpectContent(page, 'dir/file2', '# File2\ntest content2');
});


test('sync changes from client, update clientLastModified & clientlastClientModified', async ({ page }) => {
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

    console.log(serverFiles);

    expect(serverFiles['New file.md'].lastModified).not.toBeNull();
    expect(serverFiles['New file.md'].lastClientModified).toEqual(clientFileLastModified);

    // // // // Trigger syncTexts, first time to get server state
    // // // await page.evaluate(() => {
    // // //     window.dispatchEvent(new Event('focus'));
    // // // });
    // // //
    // // // await page.waitForTimeout(500);
    // //
    // // // Trigger syncTexts, second time to send client files
    // // await page.evaluate(() => {
    // //     window.dispatchEvent(new Event('focus'));
    // // });
    //
    // await page.waitForTimeout(500);

    // // Check that existing files from client are synced
    // await expectFileOnServer(page, 'README.md', 'Hello world');
    // await expectFileOnServer(page, 'Notes.md', 'Some Text');
    //
    // // Check that existing server files are preserved
    // await clickAndExpectContent(page, 'file', '# File\ntest content');
    // await clickAndExpectContent(page, 'another', '# Another\n*italic*');
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

async function clickAndExpectContent(page, filePath, expectedContent) {
    const parts = filePath.split('/');
    const file = parts.pop();
    const dirs = parts;

    for (const dir of dirs) {
        const isSelected = await page.locator(`#tree .tj_description:text-is('${dir}')`).evaluate(el => el.classList.contains('expanded'));
        if (!isSelected) {
            await page.click(`#tree .tj_description:text-is('${dir}')`);
            await page.waitForTimeout(100);
        }
    }

    await page.click(`#tree .tj_description:has-text('${file}')`);
    await page.waitForTimeout(200);

    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe(expectedContent);
}

async function expectCurrentContent(page, content) {
    const codeMirrorContent = await page.evaluate(() => {
        const cm = document.querySelector('.CodeMirror').CodeMirror;
        return cm.getValue();
    });
    expect(codeMirrorContent).toBe(content);
}