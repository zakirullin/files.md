const INBOX_PATH = '/Inbox.txt';
let messages = [];
let chatIsClean = true; // Are there any unsaved changes?

const inbox = document.getElementById('chat');
const chatInput = document.getElementById('chat-input');
const chatButton = document.getElementById('open-chat-modal');
const chatContainer = document.getElementById('chat-container');

const READ_PATH = '/Read.txt';
const SHOP_PATH = '/Shop.txt';
const WATCH_PATH = '/Watch.txt';

async function sendMsg() {
    const text = chatInput.value.trim();
    if (!text) return;

    if (wasmReply !== undefined) {
        await wasmReply(text);
    } else {
        console.log('Fallback to direct inbox writing');
        // Sometimes chat.wasm is not loaded during poor internet connection, so we fallback to direct writing.
        await saveToInbox(text);
    }
    chatInput.value = '';
    chatIsClean = false;
    await loadMessages();
    renderMessages();
    scrollToBottom();
}

async function openChat() {
    closeChatModal();
    chatContainer.style.display = 'flex';
    chatButton.classList.add('hidden');

    if (currentEditor.path !== INBOX_PATH) {
        const state = {path: editor.path};
        history.pushState(state, '');
    }

    currentEditor.path = INBOX_PATH;

    const codemirror = document.querySelector('.CodeMirror-wrap');
    codemirror.style.display = 'none';
    inbox.style.display = 'flex';
    chatInput.style.display = 'block';
    hideEditor2();

    const searchModal = document.getElementById('search');
    if (searchModal.style.display === 'none') {
        chatInput.focus();
    }
    isInbox = true;
    await loadMessages();
    renderMessages();
    scrollToBottom();
}

async function openChatModal() {
    chatContainer.classList.add('modal');
    chatContainer.style.display = 'flex';
    chatButton.classList.add('hidden');
    inbox.style.display = 'block';
    chatInput.style.display = 'block';
    inbox.style.display = 'flex';
    chatInput.style.display = 'block';

    chatInput.focus();
    await loadMessages();
    renderMessages();
    scrollToBottom();
}

// Clicking outside the modal will close the modal.
// document.addEventListener('click', (event) => {
//     let isChatModal = chatContainer.classList.contains('modal');
//     if (isChatModal && !chatContainer.contains(event.target) && !chatButton.contains(event.target)) {
//         closeChatModal();
//     }
// });

function closeChatModal() {
    chatContainer.classList.remove('modal');
    if (!isInbox) {
        chatContainer.style.display = 'none';
        inbox.style.display = 'none';
        chatInput.style.display = 'none';
        chatButton.classList.remove('hidden');
    }
}

async function toggleChat() {
    if (isInbox) {
        return;
    }

    let isChatModal = document.getElementById('chat-container').classList.contains('modal');
    if (isChatModal) {
        closeChatModal();
    } else {
        openChatModal();
    }
}

function parseFileContent(content) {
    // Normalize line endings
    content = content.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    const lines = content.split('\n');

    const headerRegex = /^#### /;
    const timestampRegex = /^`\d{2}:\d{2}` /;

    const blocks = [];
    let currentBlock = '';

    for (const line of lines) {
        const isHeader = headerRegex.test(line);
        const isTimestamp = timestampRegex.test(line);

        if (isHeader || isTimestamp) {
            // Save previous block if exists
            if (currentBlock.length > 0) {
                blocks.push(currentBlock.trim());
                currentBlock = '';
            }

            // Start new block
            currentBlock = line;
        } else {
            // Continue current block
            if (currentBlock.length > 0) {
                currentBlock += '\n' + line;
            }
        }
    }

    // Add final block
    if (currentBlock.length > 0) {
        blocks.push(currentBlock.trim());
    }

    // Parse blocks into messages
    const messages = [];
    let currentDate = null;

    // TODO write clearer way
    let numblocks = 0
    for (let i = 0; i < blocks.length; i++) {
        const block = blocks[i];

        // Check if block is a date header
        if (block.startsWith('####')) {
            currentDate = block.replace(/^#+\s*/, '').trim();
            numblocks++;
            continue;
        }

        // Check if block is a timestamped message
        const timeMatch = block.match(/^`(\d{2}:\d{2})`\s*([\s\S]*)$/);
        if (timeMatch) {
            const [, timestamp, text] = timeMatch;

            if (text.trim()) {
                messages.push({
                    index: i - numblocks,
                    text: text.trim(),
                    timestamp: timestamp,
                    date: currentDate || new Date().toDateString()
                });
            }
        }
    }

    return messages;
}

function formatFileContent(messages) {
    if (messages.length === 0) return '';

    // Group messages by date
    const messagesByDate = {};
    messages.forEach(msg => {
        const date = msg.date || new Date().toDateString();
        if (!messagesByDate[date]) {
            messagesByDate[date] = [];
        }
        messagesByDate[date].push(msg);
    });

    let content = '';
    Object.entries(messagesByDate).forEach(([date, msgs]) => {
        if (content) content += '\n';
        content += `#### ${date}\n`;
        msgs.forEach(msg => {
            content += `\`${msg.timestamp}\` ${msg.text}\n`;
        });
    });

    return content;
}

async function loadMessages() {
    try {
        const file = await ((await getFileHandle(INBOX_PATH, true)).getFile());
        const content = await file.text();

        // Parse the content and load messages
        messages = parseFileContent(content);

        console.log(`Loaded ${messages.length} messages from ${INBOX_PATH}`);
    } catch (error) {
        console.error('Error loading data:', error);
        // Initialize with empty data if file doesn't exist or can't be read
        messages = [];
    }
}

async function saveData() {

}

function initChat() {
    // chat = document.getElementById('chat');
    // chatInput = document.getElementById('chat-input');

    chatInput.addEventListener('keydown', async function (e) {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            await sendMsg();
            autoResize();
        }
    });
}

async function initWasm() {
    window.wasmReady = () => {
        console.log('WASM is ready');
    };

    console.log('Init wasm inbox');
    try {
        const go = new Go();
        const wasmResponse = await fetch(`chat.wasm${window.COMMIT_HASH}`);
        if (!wasmResponse.ok) {
            throw new Error(`Failed to fetch WASM: ${wasmResponse.status} ${wasmResponse.statusText}`);
        }

        const contentLength = wasmResponse.headers.get('content-length');
        if (contentLength) {
            console.log(`WASM file size: ${parseInt(contentLength).toLocaleString()} bytes`);
        }

        const arrayBuffer = await  wasmResponse.clone().arrayBuffer();
        console.log(`WASM file actual size: ${arrayBuffer.byteLength.toLocaleString()} bytes`);

        const wasmModule = await WebAssembly.instantiateStreaming(wasmResponse, go.importObject);
        console.log('WASM module loaded successfully');
        go.run(wasmModule.instance);

    } catch (error) {
        console.error('Error loading WASM module:', error);

        if (error instanceof TypeError && error.message.includes('fetch')) {
            console.error('Network error: Could not fetch WASM file');
        } else if (error instanceof WebAssembly.CompileError) {
            console.error('WASM compilation error:', error.message);
        } else if (error instanceof WebAssembly.LinkError) {
            console.error('WASM linking error:', error.message);
        } else if (error instanceof WebAssembly.RuntimeError) {
            console.error('WASM runtime error:', error.message);
        }
    }
}

async function logWasm(val) {
    console.log(val);
}

async function saveToInbox(content) {
    const now = new Date();
    const timestamp = now.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit'
    });

    const formattedContent = `\n\`${timestamp}\` ${content}\n`;

    await addToTextFile(INBOX_PATH, formattedContent);
}

async function receive(modifiedPaths) {
    console.log('Wasm: receiving:', modifiedPaths);
    // We only update sidebar if we're in either chat mode
    let noChatModal = !document.getElementById('chat-container').classList.contains('modal');
    let noFullScreenChat = currentEditor.path !== INBOX_PATH
    if (noChatModal && noFullScreenChat) {
        return;
    }

    await loadMessages();
    renderMessages();
    scrollToBottom();

    const fileHandle = await getFileHandle(INBOX_PATH);
    let file = await fileHandle.getFile();
    // TODO inmemory lastmodified should be reloaded
    if (currentEditor !== null && currentEditor.path === INBOX_PATH) {
        // Update in-memory lastModified
        if (getMemFile(INBOX_PATH) !== null) {
            getMemFile(INBOX_PATH).lastModified = file.lastModified;
        } else {
            addMemFile(INBOX_PATH, {
                isFile: true,
                past: INBOX_PATH,
                lastModified: file.lastModified,
                handle: fileHandle,
            })
        }
    }
    chatIsClean = true;

    for (const path of modifiedPaths) {
        const memFile = getMemFile(path);
        if (memFile !== null) {
            continue;
        }

        addMemFile(path, {
            isFile: true,
            path: path,
            lastModified: 0,
            handle: await getFileHandle(path, false),
        });
    }
    if (modifiedPaths.length !== 0) {
        renderSidebar('', modifiedPaths);
    }
}

function renderMessages() {
    if (messages.length === 0) {
        inbox.innerHTML = `
            <div class="empty-state">
                <div class="empty-title">Free your head</div>
                <div class="empty-desc">Drop whatever’s on your mind here</div>
            </div>
        `;
        return;
    }

    const recentFiles = getRecentlyModifiedFiles();
    const recentFilesButtons = recentFiles.map(filename => `
   <div class="btn-wrapper">
       <button class="action-btn to-recent-btn" data-filename="${filename}">
           ${filename.replace(/\.md$/, '').slice(0, 10)}${filename.replace(/\.md$/, '').length > 10 ? '…' : ''}
       </button>
       <span class="btn-label">To ${filename.replace(/\.md$/, '')}</span>
    </div>
    `).join('');

    // add own class every other message
    inbox.innerHTML = messages.map((message, i) => `
        <div class="message ${i % 2 === 1 ? 'own' : ''}" data-index="${message.index}">
            <div class="message-content" 
                 contenteditable="true" 
                 data-index="${message.index}"
                 spellcheck="false">${escapeHtml(message.text)}</div>
            <div class="message-hover-zone"></div>
            <div class="message-footer">
                <span class="message-time">${message.timestamp}</span>
                <div class="message-actions">
                    ${recentFilesButtons}
                    <div class="btn-wrapper">
                    <button class="action-btn to-file-btn" data-index="${message.index}">
                        
<?xml version="1.0" encoding="utf-8"?>
<svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M13 3H8.2C7.0799 3 6.51984 3 6.09202 3.21799C5.71569 3.40973 5.40973 3.71569 5.21799 4.09202C5 4.51984 5 5.0799 5 6.2V17.8C5 18.9201 5 19.4802 5.21799 19.908C5.40973 20.2843 5.71569 20.5903 6.09202 20.782C6.51984 21 7.0799 21 8.2 21H12M13 3L19 9M13 3V7.4C13 7.96005 13 8.24008 13.109 8.45399C13.2049 8.64215 13.3578 8.79513 13.546 8.89101C13.7599 9 14.0399 9 14.6 9H19M19 9V12M17 19H21M19 17V21" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>
                    </button>
                    <span class="btn-label">To File</span>
                    </div>
                     <div class="btn-wrapper"> 
                    <button class="action-btn submenu-btn to-dir-btn" data-index="${message.index}">
                        <svg width="32px" height="32px" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg" fill="none"> <path stroke-linecap="round" stroke-width="3" fill="none" d="M28 11v13a2 2 0 01-2 2H6a2 2 0 01-2-2V8a2 2 0 012-2h6c3 0 3 3 5 3h9.003C27.108 9 28 9.895 28 11z"/> </svg>
                    </button>
                        <span class="btn-label">To Dir</span>
                    </div>
                   <div class="btn-wrapper">
                    <button class="action-btn to-today-btn" data-index="${message.index}">
<?xml version="1.0" encoding="utf-8"?>
<svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" clip-rule="evenodd" d="m20.215 2.387-8.258 10.547-2.704-3.092a1 1 0 1 0-1.506 1.316l3.103 3.548a1.5 1.5 0 0 0 2.31-.063L21.79 3.62a1 1 0 1 0-1.575-1.233zM20 11a1 1 0 0 0-1 1v6.077c0 .459-.021.57-.082.684a.364.364 0 0 1-.157.157c-.113.06-.225.082-.684.082H5.923c-.459 0-.57-.022-.684-.082a.363.363 0 0 1-.157-.157c-.06-.113-.082-.225-.082-.684V5.5a.5.5 0 0 1 .5-.5l8.5.004a1 1 0 1 0 0-2L5.5 3A2.5 2.5 0 0 0 3 5.5v12.577c0 .76.082 1.185.319 1.627.224.419.558.753.977.977.442.237.866.319 1.627.319h12.154c.76 0 1.185-.082 1.627-.319.42-.224.754-.558.978-.977.236-.442.318-.866.318-1.627V12a1 1 0 0 0-1-1z" stroke="none"/></svg>   
                    </button>
                    <span class="btn-label">To Do</span>
                    </div>
                       <div class="btn-wrapper">
                    <button class="action-btn to-journal-btn" data-index="${message.index}">
                        <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                            <path fill-rule="evenodd" clip-rule="evenodd" d="M12 6.00019C10.2006 3.90317 7.19377 3.2551 
                            4.93923 5.17534C2.68468 7.09558 2.36727 10.3061 4.13778 12.5772C5.60984 14.4654 10.0648 
                            18.4479 11.5249 19.7369C11.6882 19.8811 11.7699 19.9532 11.8652 19.9815C11.9483 20.0062 
                            12.0393 20.0062 12.1225 19.9815C12.2178 19.9532 12.2994 19.8811 12.4628 19.7369C13.9229 
                            18.4479 18.3778 14.4654 19.8499 12.5772C21.6204 10.3061 21.3417 7.07538 19.0484 
                            5.17534C16.7551 3.2753 13.7994 3.90317 12 6.00019Z" 
                            stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
                        </svg>
                    </button>
                        <span class="btn-label">To Journal</span>
                    </div>
                       <div class="btn-wrapper">
                    <button class="action-btn to-checklist-btn" data-index="${message.index}" data-checklist="Read.txt">
<?xml version="1.0" encoding="utf-8"?>
<svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M4 19V6.2C4 5.0799 4 4.51984 4.21799 4.09202C4.40973 3.71569 4.71569 3.40973 5.09202 3.21799C5.51984 3 6.0799 3 7.2 3H16.8C17.9201 3 18.4802 3 18.908 3.21799C19.2843 3.40973 19.5903 3.71569 19.782 4.09202C20 4.51984 20 5.0799 20 6.2V17H6C4.89543 17 4 17.8954 4 19ZM4 19C4 20.1046 4.89543 21 6 21H20M9 7H15M9 11H15M19 17V21"  stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>
                    </button>
                        <span class="btn-label">To Read</span>
                    </div>
                       <div class="btn-wrapper">
                    <button class="action-btn to-checklist-btn" data-index="${message.index}" data-checklist="Shop.txt">
<?xml version="1.0" encoding="utf-8"?>
<svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
<path clip-rule="evenodd" d="M2 1C1.44772 1 1 1.44772 1 2C1 2.55228 1.44772 3 2 3H3.21922L6.78345 17.2569C5.73276 17.7236 5 18.7762 5 20C5 21.6569 6.34315 23 8 23C9.65685 23 11 21.6569 11 20C11 19.6494 10.9398 19.3128 10.8293 19H15.1707C15.0602 19.3128 15 19.6494 15 20C15 21.6569 16.3431 23 18 23C19.6569 23 21 21.6569 21 20C21 18.3431 19.6569 17 18 17H8.78078L8.28078 15H18C20.0642 15 21.3019 13.6959 21.9887 12.2559C22.6599 10.8487 22.8935 9.16692 22.975 7.94368C23.0884 6.24014 21.6803 5 20.1211 5H5.78078L5.15951 2.51493C4.93692 1.62459 4.13696 1 3.21922 1H2ZM18 13H7.78078L6.28078 7H20.1211C20.6742 7 21.0063 7.40675 20.9794 7.81078C20.9034 8.9522 20.6906 10.3318 20.1836 11.3949C19.6922 12.4251 19.0201 13 18 13ZM18 20.9938C17.4511 20.9938 17.0062 20.5489 17.0062 20C17.0062 19.4511 17.4511 19.0062 18 19.0062C18.5489 19.0062 18.9938 19.4511 18.9938 20C18.9938 20.5489 18.5489 20.9938 18 20.9938ZM7.00617 20C7.00617 20.5489 7.45112 20.9938 8 20.9938C8.54888 20.9938 8.99383 20.5489 8.99383 20C8.99383 19.4511 8.54888 19.0062 8 19.0062C7.45112 19.0062 7.00617 19.4511 7.00617 20Z" stroke="none"/>
</svg>
                    </button>
                        <span class="btn-label">To Shop</span>
                    </div>
                       <div class="btn-wrapper">
                    <button class="action-btn to-checklist-btn" data-index="${message.index}" data-checklist="Watch.txt">
                        <?xml version="1.0" encoding="utf-8"?>
                        <svg fill="var(--col-link)" stroke="none" width="32px" height="32px" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M18,6H14.41l2.3-2.29a1,1,0,1,0-1.42-1.42L12,5.54l-1.17-2a1,1,0,1,0-1.74,1L10,6H6A3,3,0,0,0,3,9v8a3,3,0,0,0,3,3v1a1,1,0,0,0,2,0V20h8v1a1,1,0,0,0,2,0V20a3,3,0,0,0,3-3V9A3,3,0,0,0,18,6Zm1,11a1,1,0,0,1-1,1H6a1,1,0,0,1-1-1V9A1,1,0,0,1,6,8H18a1,1,0,0,1,1,1Z" stroke="none"/></svg>
                    </button>                    
                        <span class="btn-label">To Watch</span>
                    </div>
                       <div class="btn-wrapper">
                    <button class="action-btn to-archive-btn" data-index="${message.index}">
                        <?xml version="1.0" encoding="utf-8"?>
                            <svg width="32px" height="32px" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                            <path d="M20.5001 7H3.5" stroke-width="1.5" stroke-linecap="round" fill="none"/>
<!--                            <path d="M20.5001 6H3.5" stroke-width="2.5" stroke-linecap="round" fill="none"/>-->
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
                // Select all messages
                const allMessages = inbox.querySelectorAll('.message');
                allMessages.forEach(message => message.classList.add('selected'));
            }
        }
    });

    inbox.addEventListener('mousedown', function (e) {
        // If clicking outside messages, prepare for multi-select
        if (!e.target.closest('.message')) {
            let allMessages = Array.from(inbox.querySelectorAll('.message'));
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
                const allMessages = Array.from(inbox.querySelectorAll('.message'));
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
        let allMessages = Array.from(inbox.querySelectorAll('.message'));

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

    inbox.addEventListener('click', function (e) {
        // Only clear selection if clicking outside messages AND not dragging
        if (!e.target.closest('.message') && !e.detail > 1) {
            document.querySelectorAll('.message.selected').forEach(m => m.classList.remove('selected'));
        }
    });

    inbox.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') {
            const selectedMessages = inbox.querySelectorAll('.message.selected');
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


    inbox.querySelectorAll('.to-file-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const searchModalElement = document.getElementById('search');
            if (searchModalElement.style.display !== 'none' && searchModalElement.style.display !== '') {
                searchModal.close();
            } else {
                searchModal.open('', btn.dataset.index, e.target);
            }
        });
    });

    inbox.querySelectorAll('.to-dir-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();

            const moveModalElement = document.getElementById('move');
            if (moveModalElement.style.display !== 'none' && moveModalElement.style.display !== '') {
                moveModal.close();
            } else {
                moveModal.open(btn.dataset.index, e.target);
            }
        });
    });

    inbox.querySelectorAll('.to-journal-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let indices = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                indices = Array.from(selectedMessages).map(msg => msg.dataset.index);
                messagesToRemove = selectedMessages;
            } else {
                indices = [btn.dataset.index];
                messagesToRemove = [btn.closest('.message')];
            }

            sendCmd('mv_to_journal', indices);
            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
            renderSidebar();
        });
    });

    inbox.querySelectorAll('.to-today-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let indices = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                indices = Array.from(selectedMessages).map(msg => msg.dataset.index);
                messagesToRemove = selectedMessages;
            } else {
                indices = [btn.dataset.index];
                messagesToRemove = [btn.closest('.message')];
            }

            sendCmd('add_item', [toFilename(TODAY_PATH), indices.join(',')]);
            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
            renderSidebar();
        });
    });

    inbox.querySelectorAll('.to-checklist-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let indices = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                indices = Array.from(selectedMessages).map(msg => msg.dataset.index);
                messagesToRemove = selectedMessages;
            } else {
                indices = [btn.dataset.index];
                messagesToRemove = [btn.closest('.message')];
            }

            sendCmd('add_item', [btn.dataset.checklist, indices.join(',')]);
            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
            renderSidebar();
        });
    });

    inbox.querySelectorAll('.to-archive-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let indices = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                indices = Array.from(selectedMessages).map(msg => msg.dataset.index);
                messagesToRemove = selectedMessages;
            } else {
                indices = [btn.dataset.index];
                messagesToRemove = [btn.closest('.message')];
            }

            sendCmd('mv', ['archive', indices.join(',')]);
            console.log(indices);
            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
            renderSidebar();
        });
    });

    inbox.querySelectorAll('.to-recent-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            const selectedMessages = document.querySelectorAll('.message.selected');
            let indices = [];
            let messagesToRemove = [];
            if (selectedMessages.length > 0) {
                indices = Array.from(selectedMessages).map(msg => msg.dataset.index);
                messagesToRemove = selectedMessages;
            } else {
                let message = btn.closest('.message');
                indices = [message.dataset.index];
                messagesToRemove = [message];
            }

            sendCmd('mf', [btn.dataset.filename, indices.join(',')]);
            messagesToRemove.forEach(message => {
                message.classList.add('removing');
                setTimeout(() => {
                    message.remove();
                }, 300);
            });
            chatInput.focus();
            renderSidebar();
        });
    });

    // Enable editing on double-click
    inbox.querySelectorAll('.message-content').forEach(content => {
        content.addEventListener('dblclick', function (e) {
            e.stopPropagation();
            this.style.pointerEvents = 'auto';
            this.classList.add('editing');
            this.focus();
        });
    });
}

function saveEdit(noteId, newText) {
    const note = messages.find(n => n.id == noteId);
    if (note && newText.trim() !== '') {
        note.text = newText.trim();
        saveData();
    }
}

function scrollToBottom() {
    setTimeout(function () {
        inbox.scrollTop = inbox.scrollHeight;
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

// Add event listener for input changes
chatInput.addEventListener('input', autoResize);
// Initial resize to set proper height
autoResize();


function sendCmd(cmd, params) {
    console.log('Sending CMD to wasm', cmd, params)
    let cmdObj = {
        n: cmd,
        t: "cmd",
        p: params.map(p => p.toString()),
    }
    wasmReplyCmd(JSON.stringify(cmdObj));
}

function getRecentlyModifiedFiles() {
    if (files === undefined) return [];

    const entries = [];
    for (const filename in files) {
        const content = files[filename];
        if (filename && content &&
            ![
                toFilename(INBOX_PATH),
                toFilename(CONFIG_PATH),
                toFilename(TODAY_PATH),
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
    const limit = Math.min(3, entries.length);
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
            const fileName = generateSafeFileName(file.name);

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