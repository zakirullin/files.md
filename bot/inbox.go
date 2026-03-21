package internal

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"zakirullin/stuffbot/bot/fs"
	"zakirullin/stuffbot/pkg/txt"
)

var (
	Now       = time.Now
	mu        sync.Mutex
	userLocks map[string]*sync.Mutex
)

func (b *Bot) saveToInbox(content string, timezone *time.Location) (int, error) {
	exists, err := b.fs.Exists(fs.DirRoot, fs.InboxFilename)
	if err != nil {
		return 0, fmt.Errorf("saveToChat: %w", err)
	}

	content = strings.TrimSpace(content)

	var md string
	if exists {
		md, err = b.fs.Read(fs.DirRoot, fs.InboxFilename)
		if err != nil {
			return 0, fmt.Errorf("saveToChat: %w", err)
		}
		md = txt.NormNewLines(md)
		md = strings.TrimSpace(md)
		if len(md) != 0 {
			md += "\n"
		}
	}

	blocks := readBlocks(md)
	headerRegex := regexp.MustCompile(`^#### `)
	recordCount := 0
	for _, block := range blocks {
		if !headerRegex.MatchString(block) {
			recordCount++
		}
	}

	// Add today's header if it doesn't exist
	if !strings.Contains(md, todayHeader(timezone)) {
		md += todayHeader(timezone) + "\n"
	}

	// Format timestamp with timezone
	// TODO should we use timezone here?
	timestamp := now().In(timezone).Format("`15:04`")

	// Handle images similar to journal
	//if txt.HasImage(content) {
	//	// If there's an image - place timestamp under the image
	//	re := regexp.MustCompile(txt.ImgPattern)
	//	imgLink := re.FindString(content)
	//	content = strings.TrimSpace(strings.Replace(content, imgLink, "", 1))
	//	content = fmt.Sprintf("%s\n%s %s\n", imgLink, timestamp, strings.TrimSpace(content))
	//} else {
	content = fmt.Sprintf("%s %s\n", timestamp, content)
	//}

	md += content

	if err := b.fs.Write(fs.DirRoot, fs.InboxFilename, md); err != nil {
		return 0, fmt.Errorf("saveToChat: %w", err)
	}

	return recordCount, nil
}

// moveFromInbox passes messages at given indices to a specified callback function.
// On callback success, it removes those messages from the chat file.
// msgIndices are 0-based and refer to the messages only blocks (not headers).
// On collapse=false callback would be called on every message.
func (b *Bot) moveFromInbox(
	callback func(content string, timestamp time.Time) error,
	collapse bool,
	msgIndices ...int,
) error {
	key, err := b.fs.SafePath(fs.DirRoot, "")
	if err != nil {
		return fmt.Errorf("failed to get safe path: %w", err)
	}

	lock := userLock(key)
	lock.Lock()
	defer lock.Unlock()

	content, err := b.fs.Read(fs.DirRoot, fs.InboxFilename)
	if err != nil {
		return err
	}

	blocks := readBlocks(content)

	var msgIndicesToBlockIndices []int
	headerRegex := regexp.MustCompile(`^#### `)
	for i, block := range blocks {
		if !headerRegex.MatchString(block) {
			msgIndicesToBlockIndices = append(msgIndicesToBlockIndices, i)
		}
	}
	if len(msgIndicesToBlockIndices) == 0 {
		return fmt.Errorf("no messages found")
	}
	for _, index := range msgIndices {
		if index < 0 || index >= len(msgIndicesToBlockIndices) {
			return fmt.Errorf("msgIndex %d out of bounds: use 0-%d", index, len(msgIndicesToBlockIndices)-1)
		}
	}

	// Sort msgIndices in ascending order for processing
	sortedMsgIndices := make([]int, len(msgIndices))
	copy(sortedMsgIndices, msgIndices)
	sort.Ints(sortedMsgIndices)

	// Collect specified messages from inbox.
	var msgs []struct {
		content   string
		timestamp time.Time
		index     int
	}
	for _, msgIndex := range sortedMsgIndices {
		blockIndex := msgIndicesToBlockIndices[msgIndex]
		block := blocks[blockIndex]

		// Find closest header above target msg for date context
		var headerDate string
		for i := blockIndex - 1; i >= 0; i-- {
			if headerRegex.MatchString(blocks[i]) {
				headerDate = blocks[i]
				break
			}
		}

		// Extract time and get full content
		timestampRegex := regexp.MustCompile(`^` + "`" + `(\d{2}:\d{2})` + "`" + ` `)
		if !timestampRegex.MatchString(block) {
			return fmt.Errorf("failed to parse msg timestamp for msgIndex %d", msgIndex)
		}

		// Extract timestamp
		timeMatch := regexp.MustCompile(`^` + "`" + `(\d{2}:\d{2})` + "`").FindStringSubmatch(block)
		if len(timeMatch) < 2 {
			return fmt.Errorf("failed to extract timestamp for msgIndex %d", msgIndex)
		}

		timeStr := timeMatch[1]
		// Remove timestamp prefix to get full content (including newlines)
		timestampPrefix := "`" + timeStr + "` "
		recordContent := strings.TrimPrefix(block, timestampPrefix)

		// Parse full timestamp from header date + time
		dateRegex := regexp.MustCompile(`^#### (\d{1,2}) ([A-Za-z]+), [A-Za-z]+`)
		dateMatches := dateRegex.FindStringSubmatch(headerDate)
		if len(dateMatches) < 3 {
			return fmt.Errorf("failed to parse header date for msgIndex %d", msgIndex)
		}

		// Build full timestamp
		dateTimeStr := fmt.Sprintf("%s %s %s", dateMatches[1], dateMatches[2], timeStr)
		timestamp, err := time.Parse("2 January 15:04", dateTimeStr)
		if err != nil {
			return fmt.Errorf("failed to parse timestamp for msgIndex %d: %w", msgIndex, err)
		}

		msgs = append(msgs, struct {
			content   string
			timestamp time.Time
			index     int
		}{
			content:   recordContent,
			timestamp: timestamp,
			index:     blockIndex,
		})
	}

	// First we save all the messages to files, only then we remove them from the inbox.
	if collapse {
		content := strings.Builder{}
		for _, msg := range msgs {
			content.WriteString(msg.content)
			content.WriteString("\n")
		}
		err = callback(strings.TrimSpace(content.String()), msgs[0].timestamp)
		if err != nil {
			return fmt.Errorf("callback failed: %w", err)
		}
	} else {
		for _, msg := range msgs {
			if err := callback(msg.content, msg.timestamp); err != nil {
				return fmt.Errorf("callback failed: %w", err)
			}
		}
	}

	blocksToRemove := make(map[int]bool)
	for _, msg := range msgs {
		blocksToRemove[msg.index] = true
	}
	newBlocks := make([]string, 0)
	for i, block := range blocks {
		if blocksToRemove[i] {
			continue
		}
		newBlocks = append(newBlocks, block)
	}
	modifiedContent := strings.TrimSpace(strings.Join(newBlocks, "\n"))

	return b.fs.Write(fs.DirRoot, fs.InboxFilename, modifiedContent)
}

// readBlocks parses content into logical blocks
// Returns slice where each element is either a header or a complete record
func readBlocks(content string) []string {
	content = txt.NormNewLines(content)
	lines := strings.Split(content, "\n")

	headerRegex := regexp.MustCompile(`^#### `)
	timestampRegex := regexp.MustCompile(`^` + "`" + `\d{2}:\d{2}` + "`" + ` `)

	var blocks []string
	var currentBlock strings.Builder

	for _, line := range lines {
		isHeader := headerRegex.MatchString(line)
		isTimestamp := timestampRegex.MatchString(line)

		if isHeader {
			// Save previous block if exists
			if currentBlock.Len() > 0 {
				blocks = append(blocks, strings.TrimSpace(currentBlock.String()))
				currentBlock.Reset()
			}
			// DisplayName is always its own block
			blocks = append(blocks, line)
		} else if isTimestamp {
			// Save previous block if exists
			if currentBlock.Len() > 0 {
				blocks = append(blocks, strings.TrimSpace(currentBlock.String()))
				currentBlock.Reset()
			}
			// Start new block with timestamp
			currentBlock.WriteString(line)
		} else {
			// Continue current block or start new block
			if currentBlock.Len() > 0 {
				currentBlock.WriteString("\n")
				currentBlock.WriteString(line)
			} else {
				currentBlock.WriteString(line)
			}
		}
	}

	// Add final block
	if currentBlock.Len() > 0 {
		blocks = append(blocks, strings.TrimSpace(currentBlock.String()))
	}

	return blocks
}

func todayHeader(timezone *time.Location) string {
	nowTZ := now().In(timezone)
	return fmt.Sprintf("#### %d %s, %s", nowTZ.Day(), nowTZ.Format("January"), nowTZ.Weekday())
}

func userLock(rootPath string) *sync.Mutex {
	mu.Lock()
	defer mu.Unlock()

	if userLocks == nil {
		userLocks = make(map[string]*sync.Mutex)
	}
	if lock, exists := userLocks[rootPath]; exists {
		return lock
	}

	newLock := &sync.Mutex{}
	userLocks[rootPath] = newLock

	return newLock
}
