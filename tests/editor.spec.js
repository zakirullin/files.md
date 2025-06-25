const {test, expect} = require('@playwright/test');

test.describe('Files.md Text Editor Sync Tests', () => {
    test.beforeEach(async ({page}) => {
        await page.goto('/app.html');

        await page.waitForSelector('.CodeMirror', {timeout: 10000});
        await page.waitForSelector('#sidebar-tree', {timeout: 5000});
    });

    test('should load the Files.md editor', async ({page}) => {
        await expect(page).toHaveTitle('Files.md (Alpha version)');

        await expect(page.locator('#sidebar')).toBeVisible();
        await expect(page.locator('.CodeMirror')).toBeVisible();
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
        await page.waitForSelector('.CodeMirror', {timeout: 5000});

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

        await page.waitForSelector('.CodeMirror', {timeout: 5000});

        await page.click('.CodeMirror');
        await page.keyboard.press('Meta+a');
        await page.keyboard.press('Delete');
        await page.keyboard.type('[markdown');
        await page.keyboard.press('Enter');

        await page.waitForTimeout(500);
        const content = await page.locator('.CodeMirror-code').textContent();

        console.log('Content:', content);
        expect(content).toContain('[Markdown Guide](/Markdown Guide.md)');
        // await page.pause()
    });

    test('should handle text selection correctly', async ({page}) => {
        // Add some test content with various markdown elements
        await page.click('.CodeMirror');
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
            { left: 2, width: 177, right: 179 },
            { left: 2, width: 97, right: 99 },
            { left: 2, width: 196, right: 198 },
            { left: 2, width: 229, right: 231 },
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

        await page.pause();
    });

    test('should handle text selection for word-wrap content', async ({page}) => {
        // Add some test content with various markdown elements
        await page.click('.CodeMirror');
        await page.keyboard.press('Control+a');
        await page.keyboard.press('Delete');

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
            { left: 2, width: 315, right: 317 },
            { left: 2, width: 741, right: 743 },
            { left: 2, width: 728, right: 730 },
            { left: 2, width: 752, right: 754 },
            { left: 2, width: 716, right: 718 },
            { left: 2, width: 734, right: 736 },
            { left: 2, width: 734, right: 736 },
            { left: 2, width: 753, right: 755 },
            { left: 2, width: 730, right: 732 },
            { left: 2, width: 624, right: 626 },
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
});

