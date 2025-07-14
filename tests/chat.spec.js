const {test, expect} = require('@playwright/test');

test.beforeEach(async ({page}) => {
    await page.goto('/app.html');

    // await page.waitForSelector('.CodeMirror', {timeout: 10000});
    await page.waitForSelector('#tree', {timeout: 5000});
});

test('send message to chat', async ({ page }) => {
    const consoleMessages = [];
    page.on('console', msg => {
        consoleMessages.push({
            type: msg.type(),
            text: msg.text()
        });
    });
    page.on('pageerror', error => {
        consoleMessages.push({
            type: 'error',
            text: error.message,
            stack: error.stack
        });
    });

    await page.waitForSelector('#chat');
    await page.keyboard.type('My message');
    await page.waitForTimeout(300);
    // TODO I believe chat is reloaded 2 times for some reason, it blinks, and thus removes previous message
    // Or wait for timeout before typing message doesn't help hmm
    await page.keyboard.press('Enter');

    await page.pause();
    await page.waitForSelector('.message');
    let content = await page.textContent('.message-content')
    expect(content).toBe('My message');

});

test('send to chat and move to recent file', async ({ page }) => {
    await page.evaluate(() => {
        window.getRootDirHandle = async function() {
            const root = await navigator.storage.getDirectory();
            const fileHandle = await root.getFileHandle('file.md', { create: true });

            return root;
        };
    });

    await page.evaluate(() => {
        init(document.getElementById("editor"));
    });

    await page.waitForSelector('#chat');
    await page.keyboard.type('My message');
    await page.waitForTimeout(300);
    await page.keyboard.press('Enter');

    await page.waitForSelector('.message', );
    let content = await page.textContent('.message-content')
    expect(content).toBe('My message');

    await page.hover('.message-hover-zone', {force: true});
    await page.waitForTimeout(500);
    await page.waitForSelector('.message-actions', { state: 'visible' });

    await page.locator('.action-btn').filter({ hasText: 'file' }).click();
    await page.waitForTimeout(500);
    await page.waitForSelector('.message', { state: 'detached' });

    let dateString = await page.evaluate(() => {
        const date = new Date();
        const months = ['January', 'February', 'March', 'April', 'May', 'June',
            'July', 'August', 'September', 'October', 'November', 'December'];
        const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

        const day = date.getDate();
        const month = months[date.getMonth()];
        const year = date.getFullYear();
        const weekday = days[date.getDay()];

        return `${day} ${month} ${year}, ${weekday}`;
    });
    dateString = dateString.replace(/,/g, ',')

    await clickAndExpectContent(page, 'file', `# File\n#### ${dateString}\nMy message`);
});


async function clickAndExpectContent(page, filePath, expectedContent) {
    const parts = filePath.split('/');
    const dirs = parts.slice(0, -1);
    const file = parts[parts.length - 1];

    for (const dir of dirs) {
        const isSelected = await page.locator(`#tree .tj_description:has-text('${dir}')`).evaluate(el => el.classList.contains('expanded'));
        if (!isSelected) {
            await page.click(`#tree .tj_description:has-text('${dir}')`);
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
