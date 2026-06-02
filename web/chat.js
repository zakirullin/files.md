let chatIsClean = true; // Are there any unsaved changes?

const chat = document.getElementById('chat');
const chatInput = document.getElementById('chat-input');
const chatContainer = document.getElementById('chat-container');
const chatInputWrapper = document.getElementById('chat-input-wrapper');

const MAX_TITLE_LENGTH = 100;
const RECENT_FILES = 1;
const CHAT_CONFIG_PATH = 'chat-config.json';

// Cache of the last Chat.md content we rendered from. renderMessages skips
// work when the file's content hasn't changed.
let lastChatText = null;

// Chat tabs state
let chatTabs = [];
let currentChatTab = null;
let pendingCloseTab = null; // Tab pending confirmation to close

// Load chat tabs configuration
async function loadChatConfig() {
    try {
        const handle = await getFileHandle(CHAT_CONFIG_PATH, true);
        const file = await handle.getFile();
        const content = await file.text();
        const config = JSON.parse(content);
        chatTabs = config.tabs || [];
        currentChatTab = config.lastActiveTab || 'Chat';
    } catch (err) {
        // Initialize default config
        chatTabs = [
            { name: 'Chat', messages: [] }
        ];
        currentChatTab = 'Chat';
        await saveChatConfig();
    }
}

// Save chat tabs configuration
async function saveChatConfig() {
    try {
        const config = {
            tabs: chatTabs,
            lastActiveTab: currentChatTab
        };
        await write(CHAT_CONFIG_PATH, JSON.stringify(config, null, 2));
    } catch (err) {
        logError('saveChatConfig error:', err);
    }
}

// Get current tab object
function getCurrentTab() {
    return chatTabs.find(t => t.name === currentChatTab) || chatTabs[0];
}

async function parseMessagesFromChat() {
    const tab = getCurrentTab();
    if (!tab) {
        log('No current tab found, returning empty messages');
        return { messages: [], text: '[]' };
    }
    const messages = tab.messages || [];
    log(`parseMessagesFromChat: tab=${tab.name}, messages=${messages.length}`);
    return { messages, text: JSON.stringify(messages) };
}

async function saveMessagesToChat(messages) {
    const tab = getCurrentTab();
    if (tab) {
        tab.messages = messages;
        await saveChatConfig();
        lastChatText = JSON.stringify(messages);
    }
}

// Add event listener for input changes
chatInput.addEventListener('input', autoResize);
// Initial resize to set proper height
autoResize();

chat.addEventListener('mouseover', function (e) {
    const message = e.target.closest('.message');
    if (!message) return;
    const shown = chat.querySelector('.message.actions-shown');
    if (shown && shown !== message) {
        shown.classList.remove('actions-shown');
    }
});

async function sendToChat() {
    const text = chatInput.value.trim();
    if (!text) return;

    if (text.toLowerCase().endsWith(' jj') || text.toLowerCase().endsWith(' жж')) {
        await addToJournal(text.slice(0, -3).trim());
        chatInput.value = '';
        chatIsClean = false;
        // Reload from disk so the journal file/dir created by addToJournal
        // shows up, then blink its row in the sidebar.
        files = await loadLocalFiles(await getRootDirHandle());
        renderSidebar('', [`/journal/${todayJournalFilename()}`]);
        return;
    }

    const now = new Date();
    const timestamp = now.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit'
    });
    
    const tab = getCurrentTab();
    if (!tab.messages) tab.messages = [];
    
    tab.messages.push({
        done: false,
        text,
        timestamp,
        date: new Date().toDateString()
    });
    
    await saveChatConfig();

    chatInput.value = '';
    chatIsClean = false;
    await renderMessages();
    const allMessages = chat.querySelectorAll('.message');
    if (allMessages.length > 0) {
        allMessages[allMessages.length - 1].classList.add('actions-shown');
    }
    scrollToBottom();
}

async function openChat() {
    closeChatModal();
    chatContainer.style.display = 'flex';

    if (currentEditor.path !== CHAT_PATH) {
        const state = {path: editor.path};
        history.pushState(state, '');
    }

    currentEditor.path = CHAT_PATH;

    const codemirror = document.querySelector('.CodeMirror-wrap');
    codemirror.style.display = 'none';
    chat.style.display = 'flex';
    chatInputWrapper.style.display = 'block';
    hideEditor2();

    const searchModal = document.getElementById('search');
    if (searchModal.style.display === 'none') {
        chatInput.focus();
    }
    isChat = true;
    
    await loadChatConfig();
    renderChatTabs();
    await renderMessages();
    scrollToBottom();
}

// Render chat tabs UI
function renderChatTabs() {
    let tabsContainer = document.getElementById('chat-tabs');
    if (!tabsContainer) {
        tabsContainer = document.createElement('div');
        tabsContainer.id = 'chat-tabs';
        // Insert at the beginning of chat-container
        chatContainer.insertBefore(tabsContainer, chatContainer.firstChild);
    }
    
    tabsContainer.innerHTML = chatTabs.map(tab => `
        <div class="chat-tab ${tab.name === currentChatTab ? 'active' : ''}" 
             data-tab-name="${escapeHtml(tab.name)}"
             draggable="true">
            <span class="chat-tab-name">${escapeHtml(tab.name)}</span>
            ${tab.name !== 'Chat' ? `
                <button class="chat-tab-close" title="Close tab">
                    <svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                    </svg>
                </button>
            ` : ''}
        </div>
    `).join('') + `
        <button id="add-chat-tab" title="New tab">
            <svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M8 3v10M3 8h10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            </svg>
        </button>
    `;
    
    attachTabEventListeners();
}

// Switch to a different tab
async function switchChatTab(tabName) {
    if (currentChatTab === tabName) return;
    
    currentChatTab = tabName;
    await saveChatConfig();
    renderChatTabs();
    await renderMessages();
    scrollToBottom();
}

// Add a new chat tab
async function addNewChatTab() {
    let counter = 1;
    let newName = `tag${counter}`;
    
    // Find next available tag name
    while (chatTabs.some(t => t.name === newName)) {
        counter++;
        newName = `tag${counter}`;
    }
    
    const newTab = {
        name: newName,
        messages: []
    };
    
    chatTabs.push(newTab);
    currentChatTab = newName;
    await saveChatConfig();
    renderChatTabs();
    await renderMessages();
    scrollToBottom();
}

// Close a chat tab (show confirmation dialog first)
async function closeChatTab(tabName) {
    if (tabName === 'Chat') return; // Can't close default tab

    const tab = chatTabs.find(t => t.name === tabName);
    if (!tab) return;

    // Store pending close tab and show confirmation dialog
    pendingCloseTab = tabName;
    const dialog = document.getElementById('confirm-close-tab');
    const confirmBtn = document.getElementById('confirm-close-btn');
    const cancelBtn = document.getElementById('cancel-close-btn');

    dialog.showModal();

    // Confirm button - actually close the tab
    const confirmHandler = async (e) => {
        e.stopPropagation(); // Prevent dialog close event from firing cancelHandler

        const tabToClose = pendingCloseTab;
        cleanup();

        const tabIndex = chatTabs.findIndex(t => t.name === tabToClose);
        if (tabIndex !== -1) {
            chatTabs.splice(tabIndex, 1);
            if (currentChatTab === tabToClose) {
                currentChatTab = 'Chat';
            }
            await saveChatConfig();
            renderChatTabs();
            await renderMessages();
            scrollToBottom();
        }
        dialog.close();
    };

    // Cancel button - just close dialog
    const cancelHandler = () => {
        cleanup();
        dialog.close();
    };

    const cleanup = () => {
        pendingCloseTab = null;
        confirmBtn.removeEventListener('click', confirmHandler);
        cancelBtn.removeEventListener('click', cancelHandler);
    };

    confirmBtn.addEventListener('click', confirmHandler);
    cancelBtn.addEventListener('click', cancelHandler);
}

// Attach event listeners to tab elements
function attachTabEventListeners() {
    const tabs = document.querySelectorAll('.chat-tab');
    tabs.forEach(tab => {
        const tabName = tab.dataset.tabName;
        
        tab.addEventListener('click', async (e) => {
            if (e.target.closest('.chat-tab-close')) return;
            await switchChatTab(tabName);
        });
        
        // Double-click to rename
        const nameSpan = tab.querySelector('.chat-tab-name');
        nameSpan.addEventListener('dblclick', async (e) => {
            e.stopPropagation();
            const oldName = tabName;
            nameSpan.contentEditable = 'true';
            nameSpan.classList.add('editing');
            nameSpan.focus();
            
            const range = document.createRange();
            const sel = window.getSelection();
            range.selectNodeContents(nameSpan);
            sel.removeAllRanges();
            sel.addRange(range);
            
            const finishEdit = async () => {
                if (!nameSpan.classList.contains('editing')) return;
                nameSpan.contentEditable = 'false';
                nameSpan.classList.remove('editing');
                
                const newName = nameSpan.textContent.trim();
                if (newName && newName !== oldName) {
                    // Check if name already exists
                    if (chatTabs.some(t => t.name === newName && t.name !== oldName)) {
                        nameSpan.textContent = oldName;
                        return;
                    }
                    
                    // Rename tab
                    const tabObj = chatTabs.find(t => t.name === oldName);
                    if (tabObj) {
                        tabObj.name = newName;
                        if (currentChatTab === oldName) {
                            currentChatTab = newName;
                        }
                        await saveChatConfig();
                        renderChatTabs();
                    }
                } else {
                    nameSpan.textContent = oldName;
                }
            };
            
            nameSpan.addEventListener('blur', finishEdit, { once: true });
            nameSpan.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    e.stopPropagation();
                    nameSpan.blur();
                    return false;
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    nameSpan.textContent = oldName;
                    nameSpan.blur();
                }
            });
        });
        
        // Drag and drop
        tab.addEventListener('dragstart', (e) => {
            tab.classList.add('dragging');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', tabName);
        });
        
        tab.addEventListener('dragend', () => {
            tab.classList.remove('dragging');
        });
        
        tab.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            
            const dragging = document.querySelector('.chat-tab.dragging');
            if (!dragging || dragging === tab) return;
            
            const tabsContainer = document.getElementById('chat-tabs');
            const allTabs = [...tabsContainer.querySelectorAll('.chat-tab')];
            const draggingIndex = allTabs.indexOf(dragging);
            const targetIndex = allTabs.indexOf(tab);
            
            if (draggingIndex < targetIndex) {
                tab.after(dragging);
            } else {
                tab.before(dragging);
            }
        });
        
        tab.addEventListener('drop', async (e) => {
            e.preventDefault();
            const tabsContainer = document.getElementById('chat-tabs');
            const allTabs = [...tabsContainer.querySelectorAll('.chat-tab')];
            const newOrder = allTabs.map(t => t.dataset.tabName);
            
            // Reorder chatTabs array
            const reordered = [];
            newOrder.forEach(name => {
                const tabObj = chatTabs.find(t => t.name === name);
                if (tabObj) reordered.push(tabObj);
            });
            chatTabs.forEach(t => {
                if (!reordered.includes(t)) reordered.push(t);
            });
            chatTabs = reordered;
            
            await saveChatConfig();
        });
        
        // Close button
        const closeBtn = tab.querySelector('.chat-tab-close');
        if (closeBtn) {
            closeBtn.addEventListener('click', async (e) => {
                e.stopPropagation();
                await closeChatTab(tabName);
            });
        }
    });
    
    const addBtn = document.getElementById('add-chat-tab');
    if (addBtn) {
        addBtn.addEventListener('click', addNewChatTab);
    }
}

async function openChatModal() {
    chatContainer.classList.add('modal');
    chatContainer.style.display = 'flex';
    chat.style.display = 'block';
    chatInputWrapper.style.display = 'block';
    chat.style.display = 'flex';
    chatInputWrapper.style.display = 'block';

    await loadChatConfig();
    renderChatTabs();
    chatInput.focus();
    await renderMessages();
    scrollToBottom();
}

function closeChatModal() {
    chatContainer.classList.remove('modal');
    if (!isChat) {
        chatContainer.style.display = 'none';
        chat.style.display = 'none';
        chatInputWrapper.style.display = 'none';
    }
}

async function toggleChatModal() {
    if (isChat) {
        return;
    }

    let isChatModal = document.getElementById('chat-container').classList.contains('modal');
    if (isChatModal) {
        closeChatModal();
    } else {
        openChatModal();
    }
}

async function parseMessagesFromChat() {
    const tab = getCurrentTab();
    if (!tab) {
        log('No current tab found, returning empty messages');
        return { messages: [], text: '[]' };
    }
    const messages = tab.messages || [];
    log(`parseMessagesFromChat: tab=${tab.name}, messages=${messages.length}`);
    return { messages, text: JSON.stringify(messages) };
}

function initChat() {
    let isComposing = false;
    chatInput.addEventListener('compositionstart', function () { isComposing = true; });
    chatInput.addEventListener('compositionend', function () { isComposing = false; });
    chatInput.addEventListener('keydown', async function (e) {
        if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
            e.preventDefault();
            await sendToChat();
            autoResize();
        }
    });

    const submitBtn = document.getElementById('chat-submit-btn');
    if (submitBtn) {
        submitBtn.addEventListener('click', async function () {
            await sendToChat();
            autoResize();
            chatInput.focus();
        });
    }
}

async function toggleChatMessage(timestamp, text, done) {
    const tab = getCurrentTab();
    const msg = tab.messages.find(m => m.text === text && m.timestamp === timestamp);
    if (msg) {
        msg.done = done;
        await saveChatConfig();
    }
}

function scrollToBottom() {
    setTimeout(function () {
        chat.scrollTop = chat.scrollHeight;
    }, 100);
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function autoResize() {
    if (chatInput.value === '') {
        chatInput.style.height = '';
        return;
    }

    if (chatInput.value.split('\n').length <= 1) {
        return;
    }

    chatInput.style.height = 'auto';
    chatInput.style.height = Math.min(chatInput.scrollHeight, 250) + 'px';
}

function getRecentlyModifiedFiles(n) {
    if (files === undefined) return [];

    const entries = [];
    for (const filename in files) {
        const content = files[filename];
        if (filename && content && !filename.endsWith('/') &&
            ![
                toFilename(CHAT_PATH),
                toFilename(CONFIG_PATH),
                toFilename(LATER_PATH),
                toFilename(WATCH_PATH),
                toFilename(READ_PATH),
                toFilename(SHOP_PATH),
            ].includes(filename)) {
            entries.push([filename, content]);
        }
    }

    for (let i = 0; i < entries.length - 1; i++) {
        for (let j = i + 1; j < entries.length; j++) {
            const aTime = new Date(entries[i][1].lastModified || 0);
            const bTime = new Date(entries[j][1].lastModified || 0);
            if (aTime < bTime) {
                // Swap
                const temp = entries[i];
                entries[i] = entries[j];
                entries[j] = temp;
            }
        }
    }

    // Take first 3 and extract filenames
    const result = [];
    const limit = Math.min(n, entries.length);
    for (let i = 0; i < limit; i++) {
        result.push(entries[i][0]);
    }

    return result;
}

chatInput.addEventListener('paste', async (e) => {
    const items = e.clipboardData.items;

    for (const item of items) {
        if (item.kind === 'file' && item.type.startsWith('image/')) {
            e.preventDefault();
            const file = item.getAsFile();
            const fileName = generateSafeFilename(file.name);

            const saved = await writeMediaFile(fileName, file);
            if (saved) {
                const imageMarkdown = `![${fileName}](media/${fileName})\n`;

                const cursorPos = chatInput.selectionStart;
                const textBefore = chatInput.value.substring(0, cursorPos);
                const textAfter = chatInput.value.substring(chatInput.selectionEnd);

                chatInput.value = textBefore + imageMarkdown + textAfter;

                const newCursorPos = cursorPos + imageMarkdown.length;
                chatInput.setSelectionRange(newCursorPos, newCursorPos);
                chatInput.focus();
            }
            break;
        }
    }
});

function todayJournalFilename() {
    const now = new Date();
    const monthNames = [
        'January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'
    ];
    const monthIndex = parseInt(now.toLocaleDateString('en-US', {month: 'numeric',})) - 1;
    const year = parseInt(now.toLocaleDateString('en-US', {year: 'numeric'}));
    const month = (monthIndex + 1).toString().padStart(2, '0');
    return `${year}.${month} ${monthNames[monthIndex]}.md`;
}

function todayHeader(timezone) {
    const now = new Date();
    const monthNames = [
        'January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'
    ];
    const dayNames = [
        'Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'
    ];

    const day = parseInt(now.toLocaleDateString('en-US', {day: 'numeric', timeZone: timezone}));
    const monthIndex = parseInt(now.toLocaleDateString('en-US', {month: 'numeric', timeZone: timezone})) - 1;
    const year = parseInt(now.toLocaleDateString('en-US', {year: 'numeric', timeZone: timezone}));
    const dayIndex = new Date(now.toLocaleDateString('en-US', {timeZone: timezone})).getDay();

    return `#### ${day} ${monthNames[monthIndex]}, ${dayNames[dayIndex]}`;
}

async function addToJournal(text) {
    text = ucfirst(text.trim());
    const journalFilename = todayJournalFilename();
    const journalPath = `journal/${journalFilename}`;
    const journalHeader = todayHeader().replace(/^#### /, '## ');
    await addHeaderAndText(journalPath, journalHeader, text);
}

async function moveFromChat(text, callback) {
    await callback(text);

    const { messages } = await parseMessagesFromChat();
    const filteredMessages = messages.filter(msg => msg.text !== text);
    await saveMessagesToChat(filteredMessages);
}

function attachEventListeners() {
    document.addEventListener('keydown', function (e) {
        if (isMetaKey(e) && e.key === 'a') {
            const searchModal = document.getElementById('search');
            const moveModal = document.getElementById('move');
            if ((searchModal && searchModal.style.display !== 'none' && searchModal.style.display !== '') ||
                (moveModal && moveModal.style.display !== 'none' && moveModal.style.display !== '')) {
                return;
            }

            if (e.target.id !== 'chat-input') {
                e.preventDefault();
                const allMessages = chat.querySelectorAll('.message');
                allMessages.forEach(message => message.classList.add('selected'));
            }
        }
    });

    chat.addEventListener('mousedown', function (e) {
        // If clicking outside messages, prepare for multi-select
        if (!e.target.closest('.message')) {
            let allMessages = Array.from(chat.querySelectorAll('.message'));
            let startMessage = null;

            function handleMouseMove(e) {
                const currentMessage = e.target.closest('.message');
                if (currentMessage) {
                    document.getSelection().removeAllRanges(); // Prevent text selection

                    if (!startMessage) {
                        startMessage = currentMessage;
                        document.querySelectorAll('.message.selected').forEach(m => m.classList.remove('selected'));
                        currentMessage.classList.add('selected');
                    } else if (currentMessage !== startMessage) {
                        // Select range like normal message selection
                        const startIndex = allMessages.indexOf(startMessage);
                        const endIndex = allMessages.indexOf(currentMessage);
                        const minIndex = Math.min(startIndex, endIndex);
                        const maxIndex = Math.max(startIndex, endIndex);

                        document.querySelectorAll('.message.selected').forEach(m => m.classList.remove('selected'));

                        for (let i = minIndex; i <= maxIndex; i++) {
                            allMessages[i].classList.add('selected');
                        }
                    }
                }
            }

            function handleMouseUp() {
                document.removeEventListener('mousemove', handleMouseMove);
                document.removeEventListener('mouseup', handleMouseUp);
            }

            document.addEventListener('mousemove', handleMouseMove);
            document.addEventListener('mouseup', handleMouseUp);
            return;
        }

        const message = e.target.closest('.message');
        if (!message || e.target.closest('.message-actions')) {
            return;
        }

        if (isMetaKey(e)) {
            message.classList.toggle('selected');
            return;
        }

        if (e.shiftKey) {
            const selectedMessages = document.querySelectorAll('.message.selected');
            if (selectedMessages.length > 0) {
                const allMessages = Array.from(chat.querySelectorAll('.message'));
                const lastSelected = selectedMessages[selectedMessages.length - 1];
                const startIndex = allMessages.indexOf(lastSelected);
                const endIndex = allMessages.indexOf(message);
                const minIndex = Math.min(startIndex, endIndex);
                const maxIndex = Math.max(startIndex, endIndex);

                for (let i = minIndex; i <= maxIndex; i++) {
                    allMessages[i].classList.add('selected');
                }
                return;
            }
        }

        document.querySelectorAll('.message.selected').forEach(m => m.classList.remove('selected'));
        message.classList.add('selected');

        let startMessage = message;
        let allMessages = Array.from(chat.querySelectorAll('.message'));

        function handleMouseMove(e) {
            const currentMessage = e.target.closest('.message');
            if (currentMessage && currentMessage !== startMessage) {
                document.getSelection().removeAllRanges();

                const startIndex = allMessages.indexOf(startMessage);
                const endIndex = allMessages.indexOf(currentMessage);
                const minIndex = Math.min(startIndex, endIndex);
                const maxIndex = Math.max(startIndex, endIndex);

                document.querySelectorAll('.message.selected').forEach(m => m.classList.remove('selected'));

                for (let i = minIndex; i <= maxIndex; i++) {
                    allMessages[i].classList.add('selected');
                }
            }
        }

        function handleMouseUp() {
            document.removeEventListener('mousemove', handleMouseMove);
            document.removeEventListener('mouseup', handleMouseUp);
        }

        document.addEventListener('mousemove', handleMouseMove);
        document.addEventListener('mouseup', handleMouseUp);
    });

    chat.addEventListener('click', function (e) {
        // Only clear selection if clicking outside messages AND not dragging
        if (!e.target.closest('.message') && !e.detail > 1) {
            document.querySelectorAll('.message.selected').forEach(m => m.classList.remove('selected'));
        }
    });

    chat.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') {
            const selectedMessages = chat.querySelectorAll('.message.selected');
            if (selectedMessages.length > 0) {
                selectedMessages.forEach(message => message.classList.remove('selected'));
                e.preventDefault();
                e.stopPropagation();
            }
        }
    }, true);

    // Add event listeners for editing message content
    // chatContainer.querySelectorAll('.message-content[contenteditable]').forEach(element => {
    //     element.addEventListener('blur', function (e) {
    //         saveEdit(e.target.dataset.noteId, e.target.textContent);
    //         e.target.classList.remove('editing');
    //     });
    //
    //     element.addEventListener('focus', function (e) {
    //         e.target.classList.add('editing');
    //     });
    //
    //     element.addEventListener('keydown', function (e) {
    //         if (e.key === 'Enter' && !e.shiftKey) {
    //             e.preventDefault();
    //             e.target.blur();
    //         }
    //         if (e.key === 'Escape') {
    //             e.target.textContent = messages.find(n => n.id == e.target.dataset.noteId).text;
    //             e.target.blur();
    //         }
    //     });
    // });

    chat.querySelectorAll('.complete-btn').forEach(btn => {
        btn.addEventListener('mousedown', function (e) { e.stopPropagation(); });
        btn.addEventListener('click', async function (e) {
            e.stopPropagation();
            const el = btn.closest('.message');
            el.classList.toggle('completed');
            const done = el.classList.contains('completed');
            try {
                await toggleChatMessage(el.dataset.timestamp, el.dataset.text, done);
            } catch (err) {
                logError('Failed to toggle chat line:', err);
                el.classList.toggle('completed'); // revert
            }
        });
    });

    chat.querySelectorAll('.copy-btn').forEach(btn => {
        btn.addEventListener('mousedown', function (e) { e.stopPropagation(); });
        btn.addEventListener('click', async function (e) {
            e.stopPropagation();
            const message = btn.closest('.message');
            const text = message.dataset.text;
            
            try {
                await navigator.clipboard.writeText(text);
                btn.classList.add('copied');
                setTimeout(() => {
                    btn.classList.remove('copied');
                }, 1000);
            } catch (err) {
                logError('Failed to copy text:', err);
            }
        });
    });

    chat.querySelectorAll('.to-file-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const searchModalElement = document.getElementById('search');
            if (searchModalElement.style.display !== 'none' && searchModalElement.style.display !== '') {
                searchModal.close();
            } else {
                const message = btn.closest('.message');
                // Keep this message's action row visible while the picker is
                // open - mouse leaves the bubble as soon as the modal grabs
                // focus, otherwise the buttons fade out under the user.
                message.classList.add('actions-pinned');
                searchModal.open('', e.target, message);
            }
        });
    });

    chat.querySelectorAll('.to-journal-btn').forEach(btn => {
        btn.addEventListener('click', async function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');

            let msgs = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                msgs = Array.from(selectedMessages).map(msg => msg.querySelector('.message').textContent);
                messagesToRemove = selectedMessages;
            } else {
                msgs = [btn.closest('.message').querySelector('.message-content').textContent];
                messagesToRemove = [btn.closest('.message')];
            }

            (async () => {
                for (const msg of msgs) {
                    await moveFromChat(msg, addToJournal);
                }
                await renderMessages();
                // The journal file (or even the journal/ dir) may have
                // just been created on disk. addToJournal goes through
                // write(), which doesn't touch the in-memory `files` map,
                // so reload from disk before rendering or the new entry
                // won't show up in the sidebar.
                files = await loadLocalFiles(await getRootDirHandle());
                renderSidebar('', [`/journal/${todayJournalFilename()}`]);
            })();

            // TODO only remove if previous is successful
            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
        });
    });

    chat.querySelectorAll('.to-checklist-btn').forEach(btn => {
        btn.addEventListener('click', async function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let msgs = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                msgs = Array.from(selectedMessages).map(msg => msg.querySelector('.message').textContent);
                messagesToRemove = selectedMessages;
            } else {
                msgs = [btn.closest('.message').dataset.text];
                messagesToRemove = [btn.closest('.message')];
            }


            (async () => {
                for (const msg of msgs) {
                    await moveFromChat(msg, async msg => {
                        await addChecklistItem(btn.dataset.checklist, msg)
                    });
                }
                // The checklist file (Later.md / Read.md / Watch.md /
                // Shop.md) may not exist yet - addChecklistItem creates it
                // on disk via write() but doesn't touch the in-memory
                // `files` map, so reload before rendering.
                files = await loadLocalFiles(await getRootDirHandle());
                // dataset.checklist is "Later.md"/"Read.md"/etc.; sidebar
                // paths are absolute, so prepend / so the includes() match
                // fires.
                renderSidebar('', [joinPath('/', btn.dataset.checklist)]);
            })();

            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            setTimeout(() => {
                renderMessages();
            }, 500);
            chatInput.focus();
        });
    });

    chat.querySelectorAll('.to-archive-btn').forEach(btn => {
        btn.addEventListener('click', async function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let msgs = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                msgs = Array.from(selectedMessages).map(msg => msg.querySelector('.message-content').textContent);
                messagesToRemove = selectedMessages;
            } else {
                msgs = [btn.closest('.message').querySelector('.message-content').textContent];
                messagesToRemove = [btn.closest('.message')];
            }

            const destinations = [];
            (async () => {
                for (const msgText of msgs) {
                    const [header, body] = extractHeaderAndBody(msgText, MAX_TITLE_LENGTH);
                    const path = joinPath('/', btn.dataset.dir, sanitizeFilename(header)) + '.md';
                    destinations.push(path);
                    await moveFromChat(msgText, async () => {
                        await write(path, body)
                    });
                }
                await renderMessages();
                // Reload from disk - write() above creates new files (and
                // possibly the archive/ dir itself) without touching the
                // in-memory `files` map.
                files = await loadLocalFiles(await getRootDirHandle());
                renderSidebar('', destinations);
            })();

            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
        });
    });

    chat.querySelectorAll('.to-recent-btn').forEach(btn => {
        btn.addEventListener('click', async function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let msgs = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                msgs = Array.from(selectedMessages).map(msg => msg.querySelector('.message-content').textContent);
                messagesToRemove = selectedMessages;
            } else {
                msgs = [btn.closest('.message').querySelector('.message-content').textContent];
                messagesToRemove = [btn.closest('.message')];
            }

            const path = btn.dataset.filename;
            let callback = async text => await addHeaderAndText(path, todayHeader(), text, true, false);
            (async () => {
                for (const msg of msgs) {
                    await moveFromChat(msg, callback);
                }
                await renderMessages();
                // The recent-file may not exist yet (addHeaderAndText goes
                // through write() and doesn't touch the in-memory `files`
                // map), so reload before rendering. dataset.filename is
                // just "Foo.md"; the sidebar walker produces "/Foo.md" -
                // normalize so modifiedPaths.includes(path) matches.
                files = await loadLocalFiles(await getRootDirHandle());
                renderSidebar('', [joinPath('/', path)]);
            })();

            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });

            chatInput.focus();
        });
    });

    // Enable editing on double-click
    chat.querySelectorAll('.message-content').forEach(content => {
        const originalText = content.textContent;
        const message = content.closest('.message');
        const copyBtn = message.querySelector('.copy-btn');
        
        content.addEventListener('dblclick', function (e) {
            e.stopPropagation();
            this.contentEditable = 'true';
            this.classList.add('editing');
            
            // Hide action buttons and change copy button to confirm
            const actions = message.querySelector('.message-actions');
            if (actions) actions.style.display = 'none';
            if (copyBtn) {
                copyBtn.innerHTML = `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M5 13l4 4L19 7" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>`;
                copyBtn.title = 'Confirm';
                copyBtn.classList.add('confirm-btn');
            }
            
            this.focus();
            
            // Move cursor to end
            const range = document.createRange();
            const sel = window.getSelection();
            range.selectNodeContents(this);
            range.collapse(false);
            sel.removeAllRanges();
            sel.addRange(range);
        });
        
        const finishEdit = async function () {
            if (!content.classList.contains('editing')) return;
            
            content.contentEditable = 'false';
            content.classList.remove('editing');
            
            // Restore action buttons and copy button
            const actions = message.querySelector('.message-actions');
            if (actions) actions.style.display = '';
            if (copyBtn) {
                copyBtn.innerHTML = `<svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M8 4v12a2 2 0 002 2h8a2 2 0 002-2V7.242a2 2 0 00-.602-1.43L16.083 2.57A2 2 0 0014.685 2H10a2 2 0 00-2 2z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                    <path d="M16 18v2a2 2 0 01-2 2H6a2 2 0 01-2-2V9a2 2 0 012-2h2" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>`;
                copyBtn.title = 'Copy';
                copyBtn.classList.remove('confirm-btn');
            }
            
            const newText = content.textContent.trim();
            if (newText && newText !== originalText) {
                const timestamp = message.dataset.timestamp;
                const oldText = message.dataset.text;
                
                try {
                    const { messages } = await parseMessagesFromChat();
                    const msgIndex = messages.findIndex(m => m.text === oldText && m.timestamp === timestamp);
                    if (msgIndex !== -1) {
                        messages[msgIndex].text = newText;
                        await saveMessagesToChat(messages);
                        message.dataset.text = newText;
                    }
                } catch (err) {
                    logError('Failed to save edited message:', err);
                    content.textContent = originalText;
                }
            } else if (!newText) {
                content.textContent = originalText;
            }
        };
        
        content.addEventListener('keydown', async function (e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                await finishEdit();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                content.textContent = originalText;
                await finishEdit();
            }
        });
        
        content.addEventListener('blur', finishEdit);
        
        // Confirm button click
        if (copyBtn) {
            copyBtn.addEventListener('click', async function (e) {
                if (this.classList.contains('confirm-btn')) {
                    e.stopPropagation();
                    await finishEdit();
                }
            });
        }
    });
}

async function renderMessages() {
    const { messages, text } = await parseMessagesFromChat();
    if (text === lastChatText) {
        log('Chat unchanged, skipping render');
        return;
    }
    lastChatText = text;
    log(`Loaded ${messages.length} messages from tab: ${currentChatTab}`);

    if (messages.length === 0) {
        chat.innerHTML = `
            <div class="empty-state">
                <img class="empty-icon" src="img/icon.png" alt="">
                <div class="empty-title">Free your head</div>
                <div class="empty-desc">Drop whatever’s on your mind here</div>
            </div>
        `;
        return;
    }

    const recentFiles = getRecentlyModifiedFiles(RECENT_FILES);
    const recentFilesButtons = recentFiles.map(filename => `
    <div class="btn-wrapper">
       <button class="action-btn to-recent-btn" data-filename="${filename}">
           ${filename.replace(/\.md$/, '').slice(0, 10)}${filename.replace(/\.md$/, '').length > 10 ? '…' : ''}
       </button>
       <span class="btn-label">To ${filename.replace(/\.md$/, '')}</span>
    </div>
    `).join('');

    // add own class every other message
    chat.innerHTML = messages.map((message, i) => `
        <div class="message ${i % 2 === 1 ? 'own' : ''}${message.done ? ' completed' : ''}" data-text="${escapeHtml(message.text)}" data-timestamp="${message.timestamp}">
            <button class="complete-btn" title="Mark as done">
                <svg width="22" height="22" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6.5 17l6 6 13-13"/>
                </svg>
            </button>
            <button class="copy-btn" title="Copy">
                <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M8 4v12a2 2 0 002 2h8a2 2 0 002-2V7.242a2 2 0 00-.602-1.43L16.083 2.57A2 2 0 0014.685 2H10a2 2 0 00-2 2z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                    <path d="M16 18v2a2 2 0 01-2 2H6a2 2 0 01-2-2V9a2 2 0 012-2h2" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
            </button>
            <div class="message-content"
                 data-text="${escapeHtml(message.text)}"
                 spellcheck="false">${escapeHtml(message.text)}</div>
            <div class="message-footer">
                <span class="message-time">${message.timestamp}</span>
                <div class="message-actions">
                    ${recentFilesButtons}
                    <div class="btn-wrapper">
                        <button class="action-btn to-file-btn" data-text="${escapeHtml(message.text)}">
                            <?xml version="1.0" encoding="utf-8"?>
                            <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                <path d="M13 3H8.2C7.0799 3 6.51984 3 6.09202 3.21799C5.71569 3.40973 5.40973 3.71569 5.21799 4.09202C5 4.51984 5 5.0799 5 6.2V17.8C5 18.9201 5 19.4802 5.21799 19.908C5.40973 20.2843 5.71569 20.5903 6.09202 20.782C6.51984 21 7.0799 21 8.2 21H12M13 3L19 9M13 3V7.4C13 7.96005 13 8.24008 13.109 8.45399C13.2049 8.64215 13.3578 8.79513 13.546 8.89101C13.7599 9 14.0399 9 14.6 9H19M19 9V12M17 19H21M19 17V21" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
                            </svg>
                        </button>
                    <span class="btn-label">To File</span>
                    </div>
                    
                    <div class="btn-wrapper">
                        <button class="action-btn to-journal-btn" data-text="${escapeHtml(message.text)}">
                            <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                <path fill-rule="evenodd" clip-rule="evenodd" d="M12 6.00019C10.2006 3.90317 7.19377 3.2551 
                                4.93923 5.17534C2.68468 7.09558 2.36727 10.3061 4.13778 12.5772C5.60984 14.4654 10.0648 
                                18.4479 11.5249 19.7369C11.6882 19.8811 11.7699 19.9532 11.8652 19.9815C11.9483 20.0062 
                                12.0393 20.0062 12.1225 19.9815C12.2178 19.9532 12.2994 19.8811 12.4628 19.7369C13.9229 
                                18.4479 18.3778 14.4654 19.8499 12.5772C21.6204 10.3061 21.3417 7.07538 19.0484 
                                5.17534C16.7551 3.2753 13.7994 3.90317 12 6.00019Z" 
                                stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
                            </svg>
                        </button>
                        <span class="btn-label">To Journal</span>
                    </div>
 
                    <div class="btn-wrapper">
                        <button class="action-btn to-checklist-btn" data-checklist="Later.md">
                            <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" fill="none" viewBox="0 0 32 32">
                                <circle cx="16" cy="16" r="13" stroke-width="2" style="fill: none !important;"/>
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 8v8l4 4"/>
                            </svg>
                        </button>
                        <span class="btn-label">To Later</span>
                    </div>

                    <div class="btn-wrapper">
                        <button class="action-btn to-checklist-btn" data-checklist="Read.md">
                            <?xml version="1.0" encoding="utf-8"?>
                            <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                <path d="M4 19V6.2C4 5.0799 4 4.51984 4.21799 4.09202C4.40973 3.71569 4.71569 3.40973 5.09202 3.21799C5.51984 3 6.0799 3 7.2 3H16.8C17.9201 3 18.4802 3 18.908 3.21799C19.2843 3.40973 19.5903 3.71569 19.782 4.09202C20 4.51984 20 5.0799 20 6.2V17H6C4.89543 17 4 17.8954 4 19ZM4 19C4 20.1046 4.89543 21 6 21H20M9 7H15M9 11H15M19 17V21"  stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
                            </svg>
                        </button>
                        <span class="btn-label">To Read</span>
                    </div>
                    
                    <div class="btn-wrapper">
                        <button class="action-btn to-checklist-btn" data-checklist="Shop.md">
                            <?xml version="1.0" encoding="utf-8"?>
                            <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                <path clip-rule="evenodd" d="M2 1C1.44772 1 1 1.44772 1 2C1 2.55228 1.44772 3 2 3H3.21922L6.78345 17.2569C5.73276 17.7236 5 18.7762 5 20C5 21.6569 6.34315 23 8 23C9.65685 23 11 21.6569 11 20C11 19.6494 10.9398 19.3128 10.8293 19H15.1707C15.0602 19.3128 15 19.6494 15 20C15 21.6569 16.3431 23 18 23C19.6569 23 21 21.6569 21 20C21 18.3431 19.6569 17 18 17H8.78078L8.28078 15H18C20.0642 15 21.3019 13.6959 21.9887 12.2559C22.6599 10.8487 22.8935 9.16692 22.975 7.94368C23.0884 6.24014 21.6803 5 20.1211 5H5.78078L5.15951 2.51493C4.93692 1.62459 4.13696 1 3.21922 1H2ZM18 13H7.78078L6.28078 7H20.1211C20.6742 7 21.0063 7.40675 20.9794 7.81078C20.9034 8.9522 20.6906 10.3318 20.1836 11.3949C19.6922 12.4251 19.0201 13 18 13ZM18 20.9938C17.4511 20.9938 17.0062 20.5489 17.0062 20C17.0062 19.4511 17.4511 19.0062 18 19.0062C18.5489 19.0062 18.9938 19.4511 18.9938 20C18.9938 20.5489 18.5489 20.9938 18 20.9938ZM7.00617 20C7.00617 20.5489 7.45112 20.9938 8 20.9938C8.54888 20.9938 8.99383 20.5489 8.99383 20C8.99383 19.4511 8.54888 19.0062 8 19.0062C7.45112 19.0062 7.00617 19.4511 7.00617 20Z" stroke="none"/>
                            </svg>
                        </button>
                    <span class="btn-label">To Shop</span>
                    </div>
                    
                    <div class="btn-wrapper">
                    <button class="action-btn to-checklist-btn" data-index="${message.index}" data-checklist="Watch.md">
                        <?xml version="1.0" encoding="utf-8"?>
                        <svg fill="var(--col-link)" stroke="none" width="32px" height="32px" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M18,6H14.41l2.3-2.29a1,1,0,1,0-1.42-1.42L12,5.54l-1.17-2a1,1,0,1,0-1.74,1L10,6H6A3,3,0,0,0,3,9v8a3,3,0,0,0,3,3v1a1,1,0,0,0,2,0V20h8v1a1,1,0,0,0,2,0V20a3,3,0,0,0,3-3V9A3,3,0,0,0,18,6Zm1,11a1,1,0,0,1-1,1H6a1,1,0,0,1-1-1V9A1,1,0,0,1,6,8H18a1,1,0,0,1,1,1Z" stroke="none"/></svg>
                    </button>                    
                        <span class="btn-label">To Watch</span>
                    </div>
                   
                    <div class="btn-wrapper">
                        <button class="action-btn to-archive-btn" data-dir="archive">
                            <?xml version="1.0" encoding="utf-8"?>
                                <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                                <path d="M20.5001 7H3.5" stroke-width="1.5" stroke-linecap="round" fill="none"/>
                                <path d="M18.8332 8.5L18.3732 15.3991C18.1962 18.054 18.1077 19.3815 17.2427 20.1907C16.3777 21 15.0473 21 12.3865 21H11.6132C8.95235 21 7.62195 21 6.75694 20.1907C5.89194 19.3815 5.80344 18.054 5.62644 15.3991L5.1665 8.5" stroke-width="1.5" stroke-linecap="round" fill="none"/>
                                <path d="M6.5 6C6.55588 6 6.58382 6 6.60915 5.99936C7.43259 5.97849 8.15902 5.45491 8.43922 4.68032C8.44784 4.65649 8.45667 4.62999 8.47434 4.57697L8.57143 4.28571C8.65431 4.03708 8.69575 3.91276 8.75071 3.8072C8.97001 3.38607 9.37574 3.09364 9.84461 3.01877C9.96213 3 10.0932 3 10.3553 3H13.6447C13.9068 3 14.0379 3 14.1554 3.01877C14.6243 3.09364 15.03 3.38607 15.2493 3.8072C15.3043 3.91276 15.3457 4.03708 15.4286 4.28571L15.5257 4.57697C15.5433 4.62992 15.5522 4.65651 15.5608 4.68032C15.841 5.45491 16.5674 5.97849 17.3909 5.99936C17.4162 6 17.4441 6 17.5 6" stroke-width="1.5" fill="none"/>
                            </svg>
                        </button>
                        <span class="btn-label">To Archive</span>
                    </div>
                </div>
            </div>
        </div>
    `).join('');

    attachEventListeners();
}