// Various string functions, ported from Golang bot.

async function addChecklistItem(path, item, checked = false) {
    let md = '';
    try {
        md = await read(path);
        md = normNewLines(md);
    } catch (err) {
        md = '';
    }

    // Remove existing item
    const lines = md.split('\n');
    const filteredLines = [];

    for (const line of lines) {
        const trimmedLine = line.trim();

        if (trimmedLine.length < 6) {
            filteredLines.push(trimmedLine);
            continue;
        }

        const itemText = trimmedLine.substring(6);
        if (hash(itemText) === item || itemText === item) {
            continue;
        }

        filteredLines.push(trimmedLine);
    }

    // Add new item
    if (checked) {
        filteredLines.push('- [x] ' + item);
    } else {
        // Find the last incomplete item and insert before it
        let insertIndex = filteredLines.length;
        for (let i = filteredLines.length - 1; i >= 0; i--) {
            const line = filteredLines[i].trim();
            if (line.startsWith('- [ ] ')) {
                insertIndex = i;
            }
        }

        // Insert the new incomplete item
        if (insertIndex === filteredLines.length) {
            filteredLines.push('- [ ] ' + item);
        } else {
            filteredLines.splice(insertIndex, 0, '- [ ] ' + item);
        }
    }

    const result = filteredLines.join('\n').trim();
    await write(path, result);
}

function extractHeaderAndBody(text, maxTitleLen) {
    if (text.length === 0) {
        throw new Error('extract title: empty msg');
    }

    const parts = text.split('\n');
    let title = ucfirst(parts[0].trim());

    if (hasImage(title)) {
        if (parts.length > 1) {
            title = ucfirst(parts[1].trim());
        }

        if (title === '' || parts.length === 1) {
            const now = new Date();
            const formatted = now.toLocaleDateString('en-GB', { day: '2-digit', month: '2-digit', year: '2-digit' })
                    .replace(/\//g, '.') + ' ' +
                now.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit' });
            title = `Img ${formatted}`;
        }
    }

    if ([...title].length > maxTitleLen) {
        title = [...title].slice(0, maxTitleLen).join('') + '...';
    }

    // TODO add sanitize
    // const sanitizedTitle = sanitizeFilename(title);
    const sanitizedTitle = title;
    let content = text;

    // If title is the same as content, we don't need to save it
    if (sanitizedTitle === content) {
        content = '';
    }

    // If title is already in the content, remove it.
    if (content.startsWith(sanitizedTitle)) {
        content = content.substring(sanitizedTitle.length).trim();
    }

    return [sanitizedTitle, content];
}


async function addHeaderAndText(path, header, text, atStart = false) {
    const now = new Date();
    const timestamp = `\`${now.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit'
    })}\``;

    let formattedContent;
    if (hasImage(text)) {
        const imgMatch = text.match(IMG_PATTERN);
        if (imgMatch) {
            const imgLink = imgMatch[0];
            const textContent = text.replace(imgLink, '').trim();
            formattedContent = `${imgLink}\n${timestamp} ${textContent}`;
        }
    } else {
        formattedContent = `${timestamp} ${text}`;
    }

    let existingContent = '';
    try {
        existingContent = await read(path);
    } catch (err) {
        existingContent = '';
    }
    existingContent = normNewLines(existingContent);
    existingContent = existingContent.trim();

    let result;
    if (!existingContent.includes(header)) {
        if (existingContent === "") {
            result = `${header}\n${formattedContent}`;
        } else if (atStart) {
            result = `${header}\n${formattedContent}\n\n${existingContent}`;
        } else {
            result = `${existingContent}\n\n${header}\n${formattedContent}`;
        }
    } else {
        const lines = existingContent.split("\n");
        let headerIndex = -1;

        // Find the header line
        for (let i = 0; i < lines.length; i++) {
            if (lines[i].startsWith(header)) {
                headerIndex = i;
                break;
            }
        }

        if (headerIndex === -1) {
            result = `${header}\n${formattedContent}\n\n${existingContent}`;
        } else {
            // Find where to insert (after the last line belonging to this header)
            let insertIndex = headerIndex + 1;

            // Look for the next header or end of content
            for (let i = headerIndex + 1; i < lines.length; i++) {
                if (lines[i].startsWith("###")) {
                    insertIndex = i;
                    break;
                }
                insertIndex = i + 1;
            }

            // Insert the new content
            const newLines = [];
            newLines.push(...lines.slice(0, insertIndex));
            newLines.push(formattedContent);

            // Add empty line after new content if there's content following and it's not empty
            if (insertIndex < lines.length && lines[insertIndex].trim() !== "") {
                newLines.push("");
            }

            newLines.push(...lines.slice(insertIndex));
            result = newLines.join("\n");
        }
    }

    await write(path, result);
}

function normNewLines(text) {
    return text.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
}

function hasImage(text) {
    return IMG_PATTERN.test(text);
}

// Define the image pattern constant
const IMG_PATTERN = /!\[.*?\]\(.*?\)/;

