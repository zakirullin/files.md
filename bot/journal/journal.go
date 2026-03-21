package journal

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"zakirullin/stuffbot/bot/fs"
	"zakirullin/stuffbot/bot/habits"
	"zakirullin/stuffbot/pkg/txt"
)

var (
	Now       = time.Now
	mu        sync.Mutex
	userLocks map[string]*sync.Mutex
)

// AddRecord adds a record for the current day.
// Creates a file if there's no one for the current month
func AddRecord(userFS *fs.FS, record string, timezone *time.Location) error {
	key, err := userFS.SafePath(fs.DirRoot, "")
	if err != nil {
		return fmt.Errorf("failed to get safe path: %w", err)
	}

	lock := userLock(key)
	lock.Lock()
	defer lock.Unlock()

	record = strings.TrimSpace(record)
	journalFilename := todayJournalFilename(timezone)
	exists, err := userFS.Exists(fs.DirJournal, journalFilename)
	if err != nil {
		return err
	}

	var md string
	if exists {
		md, err = userFS.Read(fs.DirJournal, journalFilename)
		if err != nil {
			return err
		}
		md = txt.NormNewLines(md)
		md = strings.TrimSpace(md)
		if len(md) != 0 {
			md += "\n"
		}
	}

	if !strings.Contains(md, todayHeader(timezone)) {
		md += todayHeader(timezone) + "\n"
	}

	timestamp := Now().In(timezone).Format("`15:04`")
	if txt.HasImage(record) {
		// If there's an image - place timestamp under the image
		re := regexp.MustCompile(txt.ImgPattern)
		imgLink := re.FindString(record)
		record = strings.TrimSpace(strings.Replace(record, imgLink, "", 1))
		record = fmt.Sprintf("%s\n%s %s\n", imgLink, timestamp, strings.TrimSpace(record))
	} else {
		record = fmt.Sprintf("%s %s\n", Now().In(timezone).Format("`15:04`"), record)
	}

	md += record

	return userFS.Write(fs.DirJournal, journalFilename, md)
}

// AddEmoji adds an emoji to the current day's record
// Creates a file if there's no one for the current month
func AddEmoji(userFS *fs.FS, emoji string, timezone *time.Location) error {
	if len(emoji) == 0 {
		return nil
	}

	key, err := userFS.SafePath(fs.DirRoot, "")
	if err != nil {
		return fmt.Errorf("failed to get safe path: %w", err)
	}

	lock := userLock(key)
	lock.Lock()
	defer lock.Unlock()

	journalFilename := todayJournalFilename(timezone)
	exists, err := userFS.Exists(fs.DirJournal, journalFilename)
	if err != nil {
		return err
	}

	if !exists {
		md := fmt.Sprintf("%s %s", todayHeader(timezone), emoji)
		return userFS.Write(fs.DirJournal, journalFilename, md)
	}

	md, err := userFS.Read(fs.DirJournal, journalFilename)
	if err != nil {
		return err
	}
	md = txt.NormNewLines(md)
	md = strings.TrimSpace(md)

	todayHeaderRE := regexp.MustCompile(fmt.Sprintf(`(%s) *(.*)`, todayHeader(timezone)))
	if todayHeaderRE.MatchString(md) {
		replacement := fmt.Sprintf(`$1 ${2}%s`, emoji)
		// Prepend day's mood emoji in front of all other emojis
		if slices.Contains(habits.MoodEmojis, emoji) {
			replacement = fmt.Sprintf(`$1 %s${2}`, emoji)
		}
		md = todayHeaderRE.ReplaceAllString(md, replacement)
	} else {
		md += fmt.Sprintf("\n%s %s", todayHeader(timezone), emoji)
	}

	err = userFS.Write(fs.DirJournal, journalFilename, md)
	if err != nil {
		return fmt.Errorf("failed to write to journal: %w", err)
	}

	return nil
}

func todayJournalFilename(timezone *time.Location) string {
	return Now().In(timezone).Format("2006.01 January.md")
}

func todayHeader(timezone *time.Location) string {
	nowTZ := Now().In(timezone)
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
