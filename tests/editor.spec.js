const {test, expect} = require('@playwright/test');

test.beforeEach(async ({page}) => {
    await page.goto('/app.html');

    // await page.waitForSelector('.CodeMirror', {timeout: 10000});
    await page.waitForSelector('#tree', {timeout: 5000});
});

test('should load the Files.md editor', async ({page}) => {
    await expect(page).toHaveTitle('Files.md (Beta version)');

    await expect(page.locator('#sidebar')).toBeVisible();
    await expect(page.locator('#open-folder')).toBeVisible();
});

test('should open markdown file via quick panel and see bold text formatting', async ({page}) => {
    const isMac = process.platform === 'darwin';
    const modifier = isMac ? 'Meta' : 'Control';
    await page.keyboard.press(`${modifier}+k`);

    await page.waitForSelector('#search', {timeout: 3000});
    await page.locator('#search-input').fill('Markdown');
    await page.keyboard.press('Enter');

    await page.waitForTimeout(1000);

    const codeMirrorContent = await page.locator('.CodeMirror').textContent();

    expect(codeMirrorContent).toContain('**Bold text**');
    expect(codeMirrorContent).toContain('**bold**');
    expect(codeMirrorContent).toContain('__bold__');

    await expect(page.locator('.CodeMirror')).toContainText('Bold text');
    await expect(page.locator('.CodeMirror')).toContainText('**bold**');

    await expect(page.locator('.CodeMirror')).toContainText('using');
});

test('insert link', async ({page}) => {
    const isMac = process.platform === 'darwin';
    const modifier = isMac ? 'Meta' : 'Control';

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

test('should handle partical text selection for word-wrap content', async ({page}) => {
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

