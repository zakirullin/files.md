class SearchModal {
    static RECENT_RESULTS = 8;

    constructor() {
        this.messageIndex = null;
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
                closeMove();
            }
        });

        // Close on outside click
        document.addEventListener('click', (event) => {
            const searchModal = document.getElementById('search');
            if (searchModal.style.display === 'block' && !searchModal.contains(event.target)) {
                this.close();
            }
        });
    }

    search() {
        let search = document.getElementById('search-input').value.toLowerCase();

        const list = document.getElementById('search-results');
        list.innerHTML = '';

        if (search.endsWith('/')) {
            const folderName = search.slice(0, -1);

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
        const lowPriorityDirs = ['archive', '_read_', '_watch_', '_shop_', 'habits', 'triggers', 'today', 'later'];

        const searchDirs = search.includes('/') && search.split('/').length === 2
            ? [search.split('/')[0]]
            : Object.keys(excludeDirs(SYSTEM_DIRS));

        search = search.includes('/')
            ? search.split('/')[1].toLowerCase()
            : search;

        // Similarity matching, check for direct file matches across directories.
        for (const dir of searchDirs) {
            if (!files[dir] || dir === 'media') continue;
            for (const filename in files[dir]) {
                const potentialMatch = filename.replace(/\.md$/, '');
                let similarityScore = similarity(search, potentialMatch);

                if (similarityScore >= 70) {
                    if (lowPriorityDirs.includes(dir)) {
                        similarityScore -= 30;
                    }
                    results.push({
                        filename: filename, dir: dir, score: similarityScore
                    });
                }
            }
        }

        // If search is equal to directory
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
        for (const dir in files) {
            // If dir is not in search dirs, skip
            if (dir === 'media') {
                continue;
            }


            for (const filename in files[dir]) {
                const potentialMatch = filename.replace(/\.md$/, '');
                const isSubstringMatch = potentialMatch.toLowerCase().includes(search.toLowerCase());

                if (!isSubstringMatch) {
                    continue; // Skip this filename if it doesn't match
                }

                let matchedPercent = (search.length / potentialMatch.length) * 100;

                results.push({
                    filename: filename, dir: dir, score: Math.round(matchedPercent)
                });
            }
        }

        const uniqueResultsMap = new Map();
        for (let i = 0; i < results.length; i++) {
            const item = results[i];
            const key = `${item.filename}-${item.dir}`;

            if (!uniqueResultsMap.has(key) || uniqueResultsMap.get(key).score < item.score) {
                uniqueResultsMap.set(key, item);
            }
        }
        results = Array.from(uniqueResultsMap.values()).sort((a, b) => b.score - a.score);
        searchModal.showResults(results);
    }

    open(text = '', messageIndex = null, buttonElement  = null) {
        this.messageIndex = messageIndex;

        let modal = document.getElementById('search');
        modal.style.display = 'block';

        const inputField = document.getElementById('search-input');
        inputField.value = text;
        inputField.focus();

        this.focusedIndex = 0;
        const goToFileResults = document.getElementById('search-results');
        goToFileResults.innerHTML = '';

        if (text === '' && this.messageIndex === null) {
            this.showRecentFiles();
        } else if (text === '') {
            this.showRootFiles();
        } else {
            this.search();
        }

        // Position the modal
        if (buttonElement && this.messageIndex !== null) {
            // Position below the button, keep x centered
            const rect = buttonElement.getBoundingClientRect();
            modal.style.position = 'fixed';
            modal.style.top = `${rect.bottom + 5}px`;
            modal.style.right = '20px';
            modal.style.left = '';
            modal.style.transform = '';
            modal.style.width = '320px';
            // modal.style.transform = 'translateX(-50%)';
        } else {
            // Default center position
            modal.style.position = 'fixed';
            modal.style.top = '30%';
            modal.style.left = '50%';
            modal.style.right = '';
            modal.style.transform = 'translate(-50%, 0)';
            modal.style.width = '';
        }
    }

    close() {
        document.getElementById('search').style.display = 'none';
        this.messageIndex = null;
    }

    showResults(results) {
        const list = document.getElementById('search-results');
        list.innerHTML = '';

        console.log(results);
        results.forEach(({dir, filename}, index) => {
            if (filename === CONFIG_FILENAME) {
                return;
            }

            const listItem = document.createElement('li');
            let title = filename.replace(/\.md$/, '').replace(/\.txt$/, '')
            if (dir !== '') {
                listItem.textContent = `${dir}/${title}`;
            } else {
                listItem.textContent = title;
            }
            listItem.setAttribute('data-path', `${dir}/${filename}`);
            listItem.setAttribute('data-index', index);

            listItem.onclick = () => this.handleItemClick(dir, filename);

            listItem.onmouseenter = () => {
                document.querySelectorAll('#search-results li').forEach(li => li.classList.remove('focused'));
                listItem.classList.add('focused');
                this.focusedIndex = index;
            };
            list.appendChild(listItem);
        });
        console.log(list);

        this.focusedIndex = 0;
        this.updateFocusedItem();
    }

    handleItemClick(dir, filename) {
        console.log('handling');
        if (this.messageIndex !== null) {
            sendCmd('mf', [filename, this.messageIndex.toString()]);
            this.close();
        } else {
            // Default behavior
            openEditor(!isChat);
            openFile(dir, filename);
            this.close();
        }
    }

    handleEnterKey() {
        const resultsList = document.getElementById('search-results').querySelectorAll('li');
        if (resultsList[this.focusedIndex]) {
            const [dir, filename] = resultsList[this.focusedIndex].getAttribute('data-path').split('/');
            this.handleItemClick(dir, filename);
        }
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
        for (const dir of Object.keys(excludeDirs(SYSTEM_DIRS))) {
            for (const filename of Object.keys(files[dir])) {
                results.push({
                    dir, filename, lastModified: files[dir][filename].lastModified,
                });
            }
        }

        results = results
            .sort((a, b) => b.lastModified - a.lastModified)
            .slice(0, SearchModal.RECENT_RESULTS);

        this.showResults(results);
    }

    showRootFiles() {
        let results = [];
        for (const filename of Object.keys(files[''])) {
            if (filename === CONFIG_FILENAME) {
                continue;
            }
            results.push({
                dir: '', filename, lastModified: files[''][filename].lastModified,
            });
        }

        results = results
            .sort((a, b) => b.lastModified - a.lastModified)
            .slice(0, SearchModal.RECENT_RESULTS);

        this.showResults(results);
    }
}

const searchModal = new SearchModal();