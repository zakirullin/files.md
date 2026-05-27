class SearchModal {
    static RECENT_RESULTS = 15;

    constructor() {
        this.selectedMsgElement = null;
        this.focusedIndex = 0;
        this.init();
    }

    init() {
        document.getElementById('search').addEventListener('keydown', (event) => {
            const resultsList = document.getElementById('search-results').querySelectorAll('li');

            if (event.key === 'Enter') {
                event.preventDefault();
                this.handleEnterKey();
            }

            if (event.key === 'ArrowDown') {
                event.preventDefault();
                this.focusedIndex = (this.focusedIndex + 1) % resultsList.length;
                this.updateFocusedItem();
            } else if (event.key === 'ArrowUp') {
                event.preventDefault();
                this.focusedIndex = (this.focusedIndex - 1 + resultsList.length) % resultsList.length;
                this.updateFocusedItem();
            }

            if (event.key === 'Escape') {
                this.close();
                event.preventDefault();
                event.stopPropagation();
                if (isChat) {
                    chatInput.focus();
                } else {
                    currentEditor.focus();
                }
            }
        });

        // Close on outside click
        document.addEventListener('click', (event) => {
            const searchModal = document.getElementById('search');
            if (searchModal.style.display !== 'none' && !searchModal.contains(event.target)) {
                this.close();
            }
        });
    }

    search() {
        let search = document.getElementById('search-input').value.toLowerCase();

        const list = document.getElementById('search-results');
        list.innerHTML = '';

        if (search.endsWith('/')) {
            const folderName = search;

            // Check if the folder exists in files
            if (files[folderName]) {
                const list = document.getElementById('search-results');
                list.innerHTML = '';

                // Get all files from the specified folder
                const folderResults = [];
                for (const filename in files[folderName]) {
                    folderResults.push({
                        filename: filename,
                        dir: folderName,
                        score: 100 // Give max score since it's an exact folder match
                    });
                }

                this.showResults(folderResults);
                return;
            }
        }

        let results = [];
        const lowPriorityDirs = ['archive', 'habits', 'triggers'];
        let searchDirs= excludeDirs(SYSTEM_DIRS);
        const searchHasSlash = search.includes('/') && search.split('/').length === 2;
        if (searchHasSlash) {
            searchDirs = search.split('/')[0];
            search = search.split('/')[1].toLowerCase();
        }

        // Similarity matching, check for direct file matches across directories.
        walkFilesExcludingSystemDirs((path) => {
            // Ignore if not in searched dirs
            let dirName = toRootDirName(path);
            if (!searchDirs.includes(dirName)) {
                return;
            }

            const potentialMatch = toFilename(path).replace(/\.md$/, '');
            let similarityScore = similarity(search, potentialMatch);
            if (similarityScore >= 70) {
                if (lowPriorityDirs.includes(dirName)) {
                    similarityScore -= 60;
                }
                results.push({
                    path: path, score: similarityScore
                });
            }
        });

        // If search is equal to directory
        // TODO multidir?
        if (files[search]) {
            for (const filename in files[search]) {
                results.push({
                    filename: filename,
                    dir: search,
                    score: 100
                });
            }
        }

        // Check for "dir file" pattern (space separated)
        const spaceIndex = search.indexOf(' ');
        if (spaceIndex !== -1) {
            const dirName = search.substring(0, spaceIndex);
            const fileName = search.substring(spaceIndex + 1);

            if (files[dirName]) {
                for (const filename in files[dirName]) {
                    const potentialMatch = filename.replace(/\.md$/, '');
                    if (potentialMatch.toLowerCase().includes(fileName.toLowerCase())) {
                        results.push({
                            filename: filename,
                            dir: dirName,
                            score: 95
                        });
                    }
                }
            }
        }

        // Substring matching
        // for (const dir in files) {
        //     // If dir is not in search dirs, skip
        //     if (dir === 'media') {
        //         continue;
        //     }
        //
        //
        //     for (const filename in files[dir]) {
        walk(files, (path, isFile) => {
            if (!isFile) {
                return;
            }

            const dirName = toRootDirName(path);
            if (dirName === 'media') {
                return;
            }

            const filename = toFilename(path);
            const potentialMatch = trimPostfix(filename, '.md');
            const isSubstringMatch = potentialMatch.toLowerCase().includes(search.toLowerCase());
            if (!isSubstringMatch) {
                return;
            }

            let matchedPercent = (search.length / potentialMatch.length) * 100;
            if (lowPriorityDirs.includes(dirName)) {
                matchedPercent /= 5;
            }
            results.push({
                filename: filename, path: path, score: Math.round(matchedPercent)
            });
        });

        const uniqueResultsMap = new Map();
        for (let i = 0; i < results.length; i++) {
            const item = results[i];
            const key = item.path;

            if (!uniqueResultsMap.has(key) || uniqueResultsMap.get(key).score < item.score) {
                uniqueResultsMap.set(key, item);
            }
        }
        results = Array.from(uniqueResultsMap.values()).sort((a, b) => b.score - a.score);
        searchModal.showResults(results);
    }

    open(text = '', buttonElement = null, messageElement = null) {
        moveModal.close();
        this.selectedMsgElement = messageElement;

        let modal = document.getElementById('search');
        modal.style.display = 'flex';

        const inputField = document.getElementById('search-input');
        inputField.value = text;
        inputField.focus();

        this.focusedIndex = 0;
        const goToFileResults = document.getElementById('search-results');
        goToFileResults.innerHTML = '';

        if (text === '' && this.selectedMsgElement === null) {
            this.showRecentFiles();
        } else if (text === '') {
            this.showRootFiles();
        } else {
            this.search();
        }

        if (buttonElement && this.selectedMsgElement !== null) {
            const rect = buttonElement.getBoundingClientRect();
            const modalHeight = 300;
            const viewportHeight = window.innerHeight;
            const spaceBelow = viewportHeight - rect.bottom;
            const spaceAbove = rect.top;

            // TODO move to css
            const positionAbove = spaceBelow < modalHeight && spaceAbove > spaceBelow;
            modal.style.position = 'fixed';

            modal.style.left = '50%';
            modal.style.transform = 'translateX(-50% + 150px)';
            modal.style.width = '320px';

            if (positionAbove) {
                modal.style.bottom = `${viewportHeight - rect.top + 5}px`;
                modal.style.top = '';
                // Reverse the order: results on top, input at bottom
                modal.classList.add('modal-reversed');
            } else {
                modal.style.top = `${rect.bottom + 5}px`;
                modal.style.bottom = '';
                // Normal order: input on top, results below
                modal.classList.remove('modal-reversed');
            }
        } else {
            // Default center position
            modal.style.position = 'fixed';
            modal.style.top = '30%';
            modal.style.bottom = '';
            modal.style.left = '50%';
            modal.style.right = '';
            modal.style.transform = 'translate(-50%, 0)';
            modal.style.width = '';
            modal.classList.remove('modal-reversed');
        }
    }

    close() {
        document.getElementById('search').style.display = 'none';
        document.getElementById('search').classList.remove('modal-reversed');
        // Drop the keep-actions-visible flag set by to-file-btn (today.js).
        document.querySelectorAll('.message.actions-pinned')
            .forEach(m => m.classList.remove('actions-pinned'));
        this.selectedMsgElement = null;
    }

    showResults(results) {
        const list = document.getElementById('search-results');
        list.innerHTML = '';

        const focusOnHover = (item) => {
            item.onmousemove = () => {
                document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
                item.classList.add('focused');
                this.focusedIndex = Array.from(list.children).indexOf(item);
            };
        };
        const addDirItem = (dir) => {
            const dataDir = dir === '/' ? '' : dir;
            const item = document.createElement('li');
            item.textContent = dir === '/' ? '/' : (dir + '/');
            item.setAttribute('data-dir', dataDir);
            item.onclick = () => this.moveToDir(dataDir);
            focusOnHover(item);
            list.appendChild(item);
        };
        const addFileItem = (path) => {
            const item = document.createElement('li');
            const title = trimPostfix(trimPostfix(toFilename(path), '.md'), '.txt');
            const dirName = toDirPath(path);
            item.textContent = dirName === '/' ? title : trimPrefix(`${dirName}/${title}`, '/');
            item.setAttribute('data-path', path);
            item.onclick = () => this.moveToFile(path);
            focusOnHover(item);
            list.appendChild(item);
        };

        if (this.selectedMsgElement !== null) {
            // Move-to-file order: /, then root files, then folders, then other files.
            const searchVal = (document.getElementById('search-input').value || '').toLowerCase();
            const dirs = this.getDirs().filter(d => searchVal === '' || d.toLowerCase().includes(searchVal));
            const files = results.filter(({path}) => path !== CONFIG_PATH && path !== CHAT_PATH);
            const rootFiles = files.filter(({path}) => toDirPath(path) === '/');
            const subFiles = files.filter(({path}) => toDirPath(path) !== '/');

            if (dirs.includes('/')) addDirItem('/');
            rootFiles.forEach(({path}) => addFileItem(path));
            dirs.filter(d => d !== '/').forEach(addDirItem);
            subFiles.forEach(({path}) => addFileItem(path));
        } else {
            results.forEach(({path}) => {
                if (path === CONFIG_PATH) return;
                addFileItem(path);
            });
        }

        this.focusedIndex = 0;
        this.updateFocusedItem();
    }

    getDirs() {
        let dirs = ['/'];
        for (const dir of Object.keys(files)) {
            if (!dir.endsWith('/')) continue;
            const name = trimPostfix(dir, '/');
            if (SYSTEM_DIRS.includes(name)) continue;
            dirs.push(name);
        }
        dirs.sort((a, b) => {
            return a.includes('_') - b.includes('_') || a.localeCompare(b);
        });
        return dirs;
    }

    async moveToDir(toDir) {
        if (this.selectedMsgElement === null) {
            this.close();
            return;
        }

        const selectedMessages = document.querySelectorAll('.message.selected');
        let msgs = [];
        let messagesToRemove = [];
        if (selectedMessages.length > 0) {
            msgs = Array.from(selectedMessages).map(m => m.querySelector('.message-content').textContent);
            messagesToRemove = selectedMessages;
        } else {
            msgs = [this.selectedMsgElement.querySelector('.message-content').textContent];
            messagesToRemove = [this.selectedMsgElement];
        }

        const destinations = [];
        for (const msg of msgs) {
            const [header, body] = extractHeaderAndBody(msg, MAX_TITLE_LENGTH);
            const path = joinPath('/', toDir, sanitizeFilename(header)) + '.md';
            destinations.push(path);
            await moveFromChat(msg, async () => {
                await write(path, body);
                addMemFile(path, {
                    isFile: true,
                    content: body,
                    lastModified: 0,
                    path: path,
                    handle: await getFileHandle(path),
                });
                setServerFile(path, body, 0);
                saveServerFiles();
            });
            await renderMessages();
        }

        messagesToRemove.forEach(message => {
            if (!message) return;
            message.classList.add('removing');
            setTimeout(() => message.remove(), 300);
        });
        chatInput.focus();
        renderSidebar('', destinations);
        this.close();
    }

    async moveToFile(path) {
        if (this.selectedMsgElement !== null) {
            const selectedMessages = document.querySelectorAll('.message.selected');

            let msgs = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                msgs = Array.from(selectedMessages).map(msg => msg.querySelector('.message-content').textContent);
                messagesToRemove = selectedMessages;
            } else {
                msgs = [this.selectedMsgElement.querySelector('.message-content').textContent];
                messagesToRemove = [this.selectedMsgElement];
            }

            let callback = async text => await addHeaderAndText(path, todayHeader(), ucfirst(text), true, false);
            for (const msg of msgs) {
                await moveFromChat(msg, callback);
                await renderMessages();
            }

            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
            renderSidebar('', [path]);
            this.close();
        } else {
            await openFile(path);
            this.close();
        }
    }

    handleEnterKey() {
        const resultsList = document.getElementById('search-results').querySelectorAll('li');
        const item = resultsList[this.focusedIndex];
        if (!item) return;
        const dir = item.getAttribute('data-dir');
        if (dir !== null) {
            this.moveToDir(dir);
            return;
        }
        const path = item.getAttribute('data-path');
        this.moveToFile(path);
    }

    updateFocusedItem() {
        const resultsList = document.getElementById('search-results').querySelectorAll('li');
        document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
        resultsList.forEach((item, index) => {
            if (index === this.focusedIndex) {
                item.classList.add('focused');
                item.scrollIntoView({block: 'nearest'});
            } else {
                item.classList.remove('focused');
            }
        });
    }

    showRecentFiles() {
        let results = [];
        walkFilesExcludingSystemDirs((path) => {
            results.push({
                path: path,
                lastModified: getMemFile(path).lastModified,
            })
        });

        results = results
            .sort((a, b) => b.lastModified - a.lastModified)
            .slice(0, SearchModal.RECENT_RESULTS);

        this.showResults(results);
    }

    showRootFiles() {
        let results = [];
        for (const filename of Object.keys(files)) {
            if (filename.endsWith('/')) {
                continue;
            }

            if (filename === toFilename(CONFIG_PATH)) {
                continue;
            }

            results.push({
                path: '/' + filename, lastModified: files[filename].lastModified,
            });
        }

        results = results
            .sort((a, b) => b.lastModified - a.lastModified)
            .slice(0, SearchModal.RECENT_RESULTS);

        this.showResults(results);
    }
}


class MoveModal {
    constructor() {
        this.focusedIndex = 0;
        this.init();
    }

    init() {
        document.getElementById('move-input').addEventListener('keydown', (event) => {
            const resultsList = document.getElementById('move-results').querySelectorAll('li');

            if (event.key === 'Enter') {
                event.preventDefault();
                this.handleEnterKey();
            }

            if (event.key === 'ArrowDown') {
                event.preventDefault();
                this.focusedIndex = (this.focusedIndex + 1) % resultsList.length;
                this.updateFocusedItem();
            } else if (event.key === 'ArrowUp') {
                event.preventDefault();
                this.focusedIndex = (this.focusedIndex - 1 + resultsList.length) % resultsList.length;
                this.updateFocusedItem();
            }

            if (event.key === 'Escape') {
                this.close();
                event.preventDefault();
                event.stopPropagation();
                if (isChat) {
                    chatInput.focus();
                } else {
                    currentEditor.focus();
                }
            }
        });

        document.getElementById('move-input').addEventListener('input', () => {
            this.suggestMove();
        });

        // Close on outside click
        document.addEventListener('click', (event) => {
            const moveModal = document.getElementById('move');
            if (moveModal.style.display !== 'none' && !moveModal.contains(event.target)) {
                this.close();
            }
        });
    }

    open() {
        searchModal.close();

        let modal = document.getElementById('move');
        modal.style.display = 'flex';

        const inputField = document.getElementById('move-input');
        inputField.focus();

        modal.style.position = 'fixed';
        modal.style.top = '30%';
        modal.style.left = '50%';
        modal.style.right = '';
        modal.style.transform = 'translate(-50%, 0)';
        modal.style.width = '';
        modal.classList.remove('modal-reversed');

        this.focusedIndex = 0;
        const moveResults = document.getElementById('move-results');
        moveResults.innerHTML = '';
        this.showResults(this.getMoveDestinations());
    }

    close() {
        document.getElementById('move').style.display = 'none';
        document.getElementById('move').classList.remove('modal-reversed');
    }

    getMoveDestinations() {
        let dirs = ['/'];
        for (const dir of Object.keys(files)) {
            if (!dir.endsWith('/')) {
                continue;
            }

            if (dir === 'media/') {
                continue;
            }
            dirs.push(trimPostfix(dir, '/'));
        }

        // Place _read_ etc in the end
        dirs.sort((a, b) => {
            return a.includes('_') - b.includes('_') || a.localeCompare(b);
        });

        return dirs;
    }

    suggestMove() {
        const search = document.getElementById('move-input').value.toLowerCase();
        if (search.trim() === '') {
            this.showResults(this.getMoveDestinations());
            return;
        }

        let dirs = this.getMoveDestinations();
        dirs = dirs.filter(dir => dir.toLowerCase().startsWith(search));

        this.showResults(dirs);
    }

    showResults(dirs) {
        const list = document.getElementById('move-results');
        list.innerHTML = '';

        dirs.forEach((dir, index) => {
            let dataDir = dir;
            if (dataDir === '/') {
                dataDir = '';
            }

            const listItem = document.createElement('li');
            listItem.textContent = dir;
            listItem.setAttribute('data-path', dataDir);
            listItem.onclick = () => this.moveToDir(dir);

            listItem.onmousemove = () => {
                document.querySelectorAll('#move-results li').forEach(li => li.classList.remove('focused'));
                listItem.classList.add('focused');
                this.focusedIndex = index;
            };

            list.appendChild(listItem);
        });

        this.focusedIndex = 0;
        this.updateFocusedItem();
    }

    handleEnterKey() {
        const resultsList = document.getElementById('move-results').querySelectorAll('li');
        if (resultsList[this.focusedIndex]) {
            const toDir = resultsList[this.focusedIndex].getAttribute('data-path');
            this.moveToDir(toDir);
        }
    }

    updateFocusedItem() {
        const resultsList = document.getElementById('move-results').querySelectorAll('li');
        document.querySelectorAll('#move-results li').forEach(li => li.classList.remove('focused'));
        resultsList.forEach((item, index) => {
            if (index === this.focusedIndex) {
                item.classList.add('focused');
                item.scrollIntoView({block: 'nearest'});
            } else {
                item.classList.remove('focused');
            }
        });
    }

    moveToDir(toDir) {
        log('CLICKED ON folder to move', toDir);
        moveCurrentFile(toDir).then(() => {
            this.close();
        });
    }
}

const searchModal = new SearchModal();
const moveModal = new MoveModal();