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
                try {
                    await subdir.getFileHandle(file.name);
                } catch (error) {
                    const fileHandle = await subdir.getFileHandle(file.name, { create: true });
                    const writable = await fileHandle.createWritable();
                    await writable.write(file.content);
                    await writable.close();
                }
            }

            await root.getFileHandle('Saved.md', { create: true });

            return root;
        };
    })

    await page.evaluate(() => {
        init(document.getElementById('editor'));
    });

    await page.waitForSelector('.CodeMirror', {timeout: 10000});
    await page.waitForSelector('#sidebar-tree', {timeout: 5000});
}

test('sync new files from server', async ({ page }) => {
    await createFileOnServer('file.md', 'test content');
    await createFileOnServer('another.md', '*italic*');

    await setup(page);

    // Check that existing files are not removed
    await clickAndExpectContent(page, 'subdir/Notes', '# Notes\nSome Text');
    await clickAndExpectContent(page, 'subdir/README', '# README\nHello world');

    // Check that new files are added
    await clickAndExpectContent(page, 'file', '# File\ntest content');
    await clickAndExpectContent(page, 'another', '# Another\n*italic*');
});

test('sync new files from client', async ({ page }) => {
    await setup(page);

    await clickAndExpectContent(page, 'Saved', '# Saved\n');

    await page.click('#new-file');
    await page.waitForTimeout(100);
    await page.keyboard.type('New file');
    await page.waitForTimeout(100);
    await page.keyboard.press('Enter');
    await page.keyboard.type('content');
    await page.waitForTimeout(3000);
    await page.pause();

    await expectFileOnServer(page, 'New file.md', 'content\n');
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

async function createFileOnServer(filepath, content) {
    const p = path.join(getServerDir(), filepath);
    await fs.writeFile(p, content, 'utf8');
}

async function expectFileOnServer(page, filepath, expectedContent) {
    const p = path.join(getServerDir(), filepath);
    console.log(p);
    const actualContent = await fs.readFile(p, 'utf8');

    expect(actualContent).toBe(expectedContent);
}

function saltToken(token, salt = '') {
    return crypto.createHash('sha256')
        .update(token + salt)
        .digest('hex');
}

async function clickAndExpectContent(page, filePath, expectedContent) {
    const parts = filePath.split('/');
    const dirs = parts.slice(0, -1);
    const file = parts[parts.length - 1];

    for (const dir of dirs) {
        const isSelected = await page.locator(`#sidebar-tree .tj_description:has-text('${dir}')`).evaluate(el => el.classList.contains('expanded'));
        if (!isSelected) {
            await page.click(`#sidebar-tree .tj_description:has-text('${dir}')`);
            await page.waitForTimeout(100);
        }
    }

    await page.click(`#sidebar-tree .tj_description:has-text('${file}')`);
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