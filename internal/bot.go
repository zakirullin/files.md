// Bot's main functionality. We accept messages from the user,
// we ask user where to save the messages. We save messages
// to plain markdown files locally.

package internal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal/consts"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/habits"
	"zakirullin/stuffbot/internal/journal"
	"zakirullin/stuffbot/internal/plugins"
	"zakirullin/stuffbot/internal/sched"
	"zakirullin/stuffbot/internal/stats"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/slice"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

var (
	errUnknownCommand           = errors.New("unknown command")
	errInvalidRequestFromInline = errors.New("invalid request from inline query")
	errInvalidInlineQuery       = errors.New("invalid inline query")
	BotPlugins                  = []BotPlugin{plugins.NewWorldClockPlugin()}
)

const (
	maxTitleLength         = 100
	inlineResultsCacheTime = 15 // Seconds
	btnsPerRow             = 3
	quickBtnsPerRow        = 4
	maxBtns                = 50
	maxBtnsInChecklist     = 10 // For _read_ and _watch_ checklists, so we're less likely to be overwhelmed :)
	maxGroupedBtnsInMoveTo = 6
	maxInlineResults       = 20
	maxMsgLength           = 4096 // In UTF-8 characters (runes), skin-tone emojis count as 2
	maxMsgsToSendAtOnce    = 5    // For lengthy messages
	savedImgWidth          = 400  // We insert images into *.md files with the specified width

	// On mobile phones buttons shrink to the message width, and sometimes it's too narrow, so we make the message wider
	wideSpacer = "<code>            ⁠</code>"
)

// Update represents incoming user updates
type Update interface {
	MsgText() string
	UserID() int64
	Cmd() *tg.Cmd
	MsgEntities() []tgbotapi.MessageEntity
	CaptionEntities() []tgbotapi.MessageEntity
	CallbackQueryID() (string, bool)
	InlineQueryID() (string, bool)
	InlineQuery() (string, bool)
	InlineQueryOffset() int
	IsSentViaBot() bool
	ReplyToMsgID() (int, bool)
	PhotoOrImageID() (string, bool)
	Caption() string
	MsgID() (int, bool)
	Time() (int, bool)
}

// Chat provides a simple interface to chat API like Telegram
type Chat interface {
	Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error)
	SendImages(userID int64, images []string) ([]int, error)
	Edit(userID int64, msgID int, text string, kb *tg.Keyboard, markup string) error
	Del(userID int64, msgID int) error
	AnswerCallbackQuery(queryID string, text string) error
	AnswerInlineQuery(queryID string, results []interface{}, cacheTime int, offset string) error
	DownloadFile(fileID string, outFile io.Writer) (string, error)
}

// Database stores per user data like "last sent message id"
type Database interface {
	LastKeyboardMsgID() (int, bool)
	SetLastKeyboardMsgID(ID int)
	DelLastKeyboardMsgID()
	InputExpectation() *tg.Cmd
	SetInputExpectation(cmd tg.Cmd)
	DelInputExpectation()
	FilenameByMsgID(msgID int) (string, bool)
	DirByMsgID(msgID int) (string, bool)
	SetFilenameByMsgID(msgID int, filename string)
	SetDirByMsgID(msgID int, filename string)
	RecentCommand() (string, bool)
	SetRecentCommand(cmd string)
	RecentCommandParams() ([]string, bool)
	SetRecentCommandParams(params []string)
	AddImgMsgID(msgID int)
	ImgMsgID() ([]int, bool)
	DelImgMsgID()
}

type BotPlugin interface {
	CanHandle(string) bool
	Handle(string) (output string, error error)
}

type Bot struct {
	userID int64
	tg     Chat
	fs     *fs.FS
	db     Database
	cfg    *userconfig.Config
}

var now = time.Now

func NewBot(userID int64, tg Chat, fs *fs.FS, db Database, cfg *userconfig.Config) *Bot {
	return &Bot{userID, tg, fs, db, cfg}
}

// Answer to incoming text message, command or inline query
func (b *Bot) Answer(u Update) error {
	// Handle inline queries
	if _, ok := u.InlineQueryID(); ok {
		return b.answerSearch(u)
	}

	for _, plugin := range BotPlugins {
		if plugin.CanHandle(u.MsgText()) {
			output, err := plugin.Handle(u.MsgText())
			if err != nil {
				return fmt.Errorf("answer: plugin error: %w", err)
			}
			_, _ = b.tg.Send(b.userID, output, nil, tg.MarkupHTML)

			b.delAllKeyboards()
			err = b.ShowToday(nil)
			if err != nil {
				return fmt.Errorf("answer after plugin: %w", err)
			}

			return nil
		}
	}

	// Handle inline query file requests
	if u.IsSentViaBot() {
		return b.answerFileRequest(u.MsgText())
	}

	// Handle commands
	cmd, err := b.extractCmd(u)
	if err != nil {
		return fmt.Errorf("answer: %w", err)
	}
	if cmd != nil {
		if _, ok := u.CallbackQueryID(); !ok {
			b.delAllKeyboards()
		}

		handler, ok := b.handlers()[cmd.Name]
		if !ok {
			// It should be handled at cmd extraction step
			return fmt.Errorf("no such command %s: %w", cmd.Name, errUnknownCommand)
		}
		slog.Debug("Command is called", "command", cmd.Name, "params", cmd.Params)
		err = handler(cmd.Params)
		if err != nil {
			return err
		}

		if callbackQueryID, ok := u.CallbackQueryID(); ok {
			// We can tolerate an error here, that won't affect UX
			if cmd.Name == consts.CmdComplete || cmd.Name == consts.CmdCompleteHabit {
				_ = b.tg.AnswerCallbackQuery(callbackQueryID, completedMsg())
			} else if cmd.Name == consts.CmdShare {
				_ = b.tg.AnswerCallbackQuery(callbackQueryID, "Shared 💚!")
			} else {
				_ = b.tg.AnswerCallbackQuery(callbackQueryID, "")
			}
		}

		return nil
	}

	// Handle images
	if _, hasImage := u.PhotoOrImageID(); hasImage {
		return b.saveFromImage(u)
	}

	// Handle regular text messages
	return b.saveFromRegularMsg(u)
}

// Commands and their handlers.
// Every handler accepts []string params
func (b *Bot) handlers() map[string]func([]string) error {
	handlers := map[string]func([]string) error{
		// Direct user commands
		consts.CmdShowToday:          b.ShowToday,
		consts.CmdShowStart:          b.showStart,
		consts.CmdShowLater:          b.showLaterTasks,
		consts.CmdShowFiles:          b.showFiles,
		consts.CmdShowDirs:           b.showDirs,
		consts.CmdShowChecklists:     b.showChecklists,
		consts.CmdShowPostpone:       b.showPostpone,
		consts.CmdShowMoveFromToday:  b.showMoveFromToday,
		consts.CmdShowMoveTo:         b.showMoveTo,
		consts.CmdShowRename:         b.showRename,
		consts.CmdShowStats:          b.showStats,
		consts.CmdShowReadChecklist:  b.showRead,
		consts.CmdShowWatchChecklist: b.showWatch,
		consts.CmdShowShopChecklist:  b.showShop,
		consts.CmdShowSchedule:       b.showSchedule,
		consts.CmdShowSettings:       b.showSettings,
		consts.CmdShowHelp:           b.showHelp,
		consts.CmdDownload:           b.download,
		// Button's commands (callbacks)
		consts.CmdShowRenameFile:              b.showRenameFile,
		consts.CmdShowMultilineTask:           b.showMultilineTask,
		consts.CmdShowFile:                    b.showFile,
		consts.CmdShowChecklist:               b.showChecklist,
		consts.CmdCompleteChecklistItem:       b.completeChecklistItem,
		consts.CmdShowChecklistItem:           b.showChecklistItem,
		consts.CmdShowScheduleForDay:          b.showToADay,
		consts.CmdShowMoveToDirOrFile:         b.showMoveToFileOrDir,
		consts.CmdShowMoveToChecklist:         b.showToChecklist,
		consts.CmdMoveToExistingDir:           b.moveToDir,
		consts.CmdRequestNewDir:               b.requestNewDirName,
		consts.CmdMoveToNewDir:                b.moveToNewDir,
		consts.CmdMoveToExistingFile:          b.moveToExistingFile,
		consts.CmdMoveToExistingNote:          b.moveToExistingNote,
		consts.CmdMoveToNewFile:               b.moveToNewFile,
		consts.CmdMoveToChecklist:             b.moveToChecklist,
		consts.CmdMoveToRead:                  b.moveToRead,
		consts.CmdMoveToWatch:                 b.moveToWatch,
		consts.CmdMoveToShop:                  b.moveToShop,
		consts.CmdMoveToNewChecklist:          b.moveToNewChecklist,
		consts.CmdMoveToJournal:               b.moveToJournal,
		consts.CmdMoveToLater:                 b.moveToLater,
		consts.CmdSchedule:                    b.schedule,
		consts.CmdScheduleForTmrw:             b.scheduleForTmrw,
		consts.CmdComplete:                    b.complete,
		consts.CmdPostpone:                    b.postpone,
		consts.CmdPomodoro:                    b.togglePomodoro,
		consts.CmdShowScheduleForDayRecurring: b.showToADayRecurring,
		consts.CmdShowQuickBtnsSettings:       b.showQuickBtnsSettings,
		consts.CmdShowMoveToBtnsSettings:      b.showMoveToBtnsSettings,
		consts.CmdAddToQuickBtns:              b.addToQuickBtns,
		consts.CmdDelFromQuickBtns:            b.delFromQuickBtns,
		consts.CmdAddToMoveToBtns:             b.addToMoveToBtns,
		consts.CmdDelFromMoveToBtns:           b.delFromMoveToBtns,
		consts.CmdAddToJournalShortcut:        b.addToJournalFromShortcut,
		consts.CmdAddToRecentFileShortcut:     b.addToRecentFileOrNoteFromShortcut,
		consts.CmdRename:                      b.rename,
		consts.CmdTasksOnlyMode:               b.tasksOnlyMode,
		consts.CmdNotesOnlyMode:               b.notesOnlyMode,
		consts.CmdJournalOnlyMode:             b.journalOnlyMode,
		consts.CmdFullMode:                    b.fullMode,
		consts.CmdCompleteHabit:               b.completeHabit,
		consts.CmdShare:                       b.shareNote,
		// Used for button-like separators
		consts.CmdDoNothing: func(s []string) error { return nil },
	}

	for cmd, shortcuts := range consts.Shortcuts {
		for _, shortcut := range shortcuts {
			handlers[shortcut] = handlers[cmd]
		}
	}

	return handlers
}

func (b *Bot) extractCmd(u Update) (*tg.Cmd, error) {
	cmd := u.Cmd()
	if cmd != nil {
		// Check if the command is known
		_, ok := b.handlers()[cmd.Name]
		if !ok {
			// An informative message, we can ignore that
			_, _ = b.tg.Send(b.userID, i18n.Tr("I know nothing about this command 😕"), nil, tg.MarkupHTML)
			return nil, nil
		}

		b.db.DelInputExpectation()

		return cmd, nil
	}

	// Input expectation is mostly used for renaming things
	cmd = b.db.InputExpectation()
	if cmd != nil {
		slog.Debug("Got command from input expectation", "command", cmd.Name)
		b.db.DelInputExpectation()

		for i, param := range cmd.Params {
			if param == "%s" {
				cmd.Params[i] = u.MsgText()
			}
		}

		return cmd, nil
	}

	for canonicalCMD, shortcuts := range consts.Shortcuts {
		for _, shortcut := range shortcuts {
			escapedShortcut := regexp.QuoteMeta(shortcut)
			reText := regexp.MustCompile(fmt.Sprintf(`(?i)^%s\s+|\s+%s$`, escapedShortcut, escapedShortcut))
			// The only difference from reText is that caption can contain only shortcut, with no other text
			reCaption := regexp.MustCompile(fmt.Sprintf(`(?i)^%s\s+|\s+%s$|^\s*%s\s*$`, escapedShortcut, escapedShortcut, escapedShortcut))

			doesntMatchText := !reText.MatchString(u.MsgText())
			doesntMatchCaption := !reCaption.MatchString(u.Caption())
			if doesntMatchText && doesntMatchCaption {
				continue
			}

			text := ""
			_, hasImage := u.PhotoOrImageID()
			if hasImage {
				var errImage error
				text, errImage = b.saveImage(u)
				if errImage != nil {
					return nil, fmt.Errorf("save image: %w", errImage)
				}
				text = string(reCaption.ReplaceAll([]byte(text), []byte("")))
			} else {
				text = extractMarkdown(u)
				text = string(reText.ReplaceAll([]byte(text), []byte("")))
			}

			text = txt.Ucfirst(strings.TrimSpace(text))
			shortCmd := tg.NewCmd(canonicalCMD, []string{text})

			return &shortCmd, nil
		}
	}

	return nil, nil
}

func (b *Bot) saveFromRegularMsg(u Update) error {
	msg := extractMarkdown(u)

	// Collapse a few consecutive messages into one, see bot_forwards.go
	msgTime, updateHasTime := u.Time()
	if updateHasTime {
		filename, shouldCollapse := collapseToMsg(b.userID, msgTime)
		if shouldCollapse {
			err := b.createOrAdd(fs.DirToday, filename, msg)
			if err != nil {
				return fmt.Errorf("save collapsed: %w", err)
			}
			return nil
		}
	}

	// Adding to an existing file
	if replyMsgID, ok := u.ReplyToMsgID(); ok {
		return b.addToRepliedFile(replyMsgID, msg)
	}

	sanitizedTitle, content, err := b.extractTitleAndContent(msg)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	filename := fs.Filename(sanitizedTitle)
	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	if updateHasTime {
		setFirstMsgFilename(b.userID, filename, msgTime)
		setFirstMsgTime(b.userID, msgTime)
	}

	msgID, _ := u.MsgID()
	b.db.SetDirByMsgID(msgID, fs.DirToday)
	b.db.SetFilenameByMsgID(msgID, filename)

	if b.cfg.JournalOnlyMode() {
		return b.moveToJournal([]string{fs.Hash(filename)})
	}

	return b.showMoveTo([]string{fs.Hash(filename)})
}

func (b *Bot) saveFromImage(u Update) error {
	content, err := b.saveImage(u)
	if err != nil {
		return fmt.Errorf("save from image: %w", err)
	}

	// Adding to an existing file
	if replyMsgID, ok := u.ReplyToMsgID(); ok {
		return b.addToRepliedFile(replyMsgID, content)
	}

	// Creating a new file
	title := strings.SplitN(strings.TrimSpace(u.Caption()), "\n", 2)[0]
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > maxTitleLength {
		title = txt.Substr(title, 0, maxTitleLength) + "..."
	}
	if title == "" {
		title = fmt.Sprintf("Img %s", now().Format("02.01.06 15:04"))
	}
	sanitizedTitle := fs.SanitizeFilename(title)

	filename := fs.Filename(sanitizedTitle)
	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save from image: %w", err)
	}

	if b.cfg.JournalOnlyMode() {
		return b.moveToJournal([]string{fs.Hash(filename)})
	}

	return b.showMoveTo([]string{fs.Hash(filename)})
}

// saveImage saves an image to the filesystem and returns a markdown link to it
func (b *Bot) saveImage(u Update) (string, error) {
	imageID, _ := u.PhotoOrImageID()

	var buf bytes.Buffer
	extension, err := b.tg.DownloadFile(imageID, &buf)
	if err != nil {
		return "", fmt.Errorf("can't download file: %w", err)
	}

	imgFilename := fmt.Sprintf("tg_%s%s", imageID, extension)
	err = b.fs.Write(fs.DirImg, imgFilename, buf.String())
	if err != nil {
		return "", fmt.Errorf("can't save image: %w", err)
	}

	imgPath := fmt.Sprintf("%s/%s", fs.DirImg, imgFilename)
	content := fmt.Sprintf("![center|%d](%s)", savedImgWidth, imgPath)
	// If there's caption, place it under the image
	if u.Caption() != "" {
		caption := txt.TelegramEntitiesToMarkdown(u.Caption(), u.CaptionEntities())
		caption = strings.TrimSpace(txt.NormNewLines(caption))
		content = fmt.Sprintf("%s\n%s", content, txt.Ucfirst(caption))
	}

	return content, nil
}

func (b *Bot) addToRepliedFile(replyToMsgID int, newContent string) error {
	dir, _ := b.db.DirByMsgID(replyToMsgID)
	existingFilename, ok := b.db.FilenameByMsgID(replyToMsgID)
	if !ok {
		return fmt.Errorf("add to replied: can't find filename by msgID %d", replyToMsgID)
	}
	existingContent, err := b.fs.Read(dir, existingFilename)
	if err != nil {
		return fmt.Errorf("add: can't read: %w", err)
	}

	header := fmt.Sprintf("#### %d %s, %s", now().Day(), now().Format("January"), now().Weekday())
	content := txt.InsertTextAfterHeader(existingContent, header, newContent)
	err = b.fs.Write(dir, existingFilename, content)
	if err != nil {
		return fmt.Errorf("add: can't write: %w", err)
	}

	b.delAllKeyboards()

	b.db.SetRecentCommand(consts.CmdMoveToExistingFile)
	b.db.SetRecentCommandParams([]string{fs.ShortHash(existingFilename), fs.ShortHash(fs.DirToday)})

	return b.ShowToday(nil)
}

func (b *Bot) answerSearch(u Update) error {
	query, ok := u.InlineQuery()
	if !ok {
		return nil
	}
	query = strings.TrimSpace(query)

	if strings.Contains(query, "../") || strings.Contains(query, "/..") {
		return fmt.Errorf("insecure input '%s': %w", query, errInvalidInlineQuery)
	}

	matchedNotes, err := b.fs.SearchFiles(query)
	if err != nil {
		return fmt.Errorf("inline reply: %w", err)
	}
	if u.InlineQueryOffset() >= len(matchedNotes) {
		return nil
	}
	maxIndex := min(u.InlineQueryOffset()+maxInlineResults, len(matchedNotes))
	matchedNotes = matchedNotes[u.InlineQueryOffset():maxIndex]

	var results []interface{}
	for id, note := range matchedNotes {
		path := fmt.Sprintf("<code>%s/%s</code>", note.ParentDir, note.Name)
		if note.ParentDir == fs.DirRoot {
			path = note.Name
		}
		article := tgbotapi.NewInlineQueryResultArticleHTML(strconv.Itoa(id), note.Title, path)
		results = append(results, article)
	}

	queryID, _ := u.InlineQueryID()
	nextOffset := strconv.Itoa(u.InlineQueryOffset() + maxInlineResults)

	// First element is usually the file itself, exclude it
	if len(query) == 0 {
		results = results[1:]
	}

	err = b.tg.AnswerInlineQuery(queryID, results, inlineResultsCacheTime, nextOffset)
	// FakeTG library has a bug of unmarshalling sent result, we'll mute that temporarily
	if err != nil && !strings.HasSuffix(err.Error(), "Go value of type tgbotapi.Message") {
		return fmt.Errorf("inline reply: %w", err)
	}

	return nil
}

func (b *Bot) answerFileRequest(msg string) error {
	if strings.Contains(msg, "../") || strings.Contains(msg, "/..") {
		return fmt.Errorf("insecure input '%s': %w", msg, errInvalidRequestFromInline)
	}

	dirAndFilename := strings.Split(msg, "/")
	var dir, filename string
	if len(dirAndFilename) == 1 {
		dir = fs.DirRoot
		filename = strings.TrimSpace(dirAndFilename[0])
	} else if len(dirAndFilename) == 2 {
		dir = strings.TrimSpace(dirAndFilename[0])
		filename = strings.TrimSpace(dirAndFilename[1])
	} else {
		return fmt.Errorf("invalid inline query '%s': %w", msg, errInvalidRequestFromInline)
	}

	b.delAllKeyboards()

	// TODO add tests
	// User wants to add his text to a selected file
	c := b.db.InputExpectation()
	if c != nil {
		b.db.DelInputExpectation()
		newFilenameHash := c.Params[0]
		newFilename, err := b.fs.Unhash(fs.DirRoot, newFilenameHash)
		if err != nil {
			return fmt.Errorf("inline query: can't unhash filename %s: %w", newFilenameHash, err)
		}

		// User selects same file, no need to do anything
		if dir == fs.DirRoot && filename == newFilename {
			return b.ShowToday(nil)
		}

		content, err := b.restoreMsg(fs.DirRoot, newFilename)
		if err != nil {
			return fmt.Errorf("inline query: can't read file %s: %w", newFilename, err)
		}

		if dir == fs.DirRoot {
			// We have a file
			b.db.SetRecentCommand(consts.CmdMoveToExistingFile)
			b.db.SetRecentCommandParams([]string{fs.ShortHash(filename), fs.ShortHash(fs.DirToday)})
		} else {
			// We have a note (a file placed in a subdirectory)
			b.db.SetRecentCommand(consts.CmdMoveToExistingNote)
			b.db.SetRecentCommandParams([]string{fs.ShortHash(filename), fs.ShortHash(dir)})
		}

		err = b.addToFile(dir, filename, content)
		if err != nil {
			return fmt.Errorf("inline query: can't add to file %s: %w", filename, err)
		}

		// No worries if we can't delete - we'll have a redundant file
		_ = b.fs.Del(fs.DirRoot, newFilename)

		// Just an informative message
		_, _ = b.tg.Send(b.userID, fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.Title(filename)), nil, tg.MarkupHTML)

		return b.ShowToday(nil)
	}

	return b.showFile([]string{dir, filename})
}

func (b *Bot) createOrAdd(dir, filename, content string) error {
	exists, err := b.fs.Exists(dir, filename)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}

	if exists {
		existingContent, err := b.fs.Read(dir, filename)
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}
		existingContent = strings.TrimSpace(existingContent)

		if len(existingContent) != 0 {
			content = fmt.Sprintf("%s\n%s", strings.TrimSpace(existingContent), content)
		}
	}

	if err := b.fs.Write(fs.DirToday, filename, content); err != nil {
		return fmt.Errorf("create: %w", err)
	}

	return nil
}

func (b *Bot) extractTitleAndContent(msg string) (string, string, error) {
	if len(msg) == 0 {
		return "", "", fmt.Errorf("extract title: empty msg")
	}

	parts := strings.SplitN(msg, "\n", 2)
	title := txt.Ucfirst(strings.TrimSpace(parts[0]))

	if utf8.RuneCountInString(title) > maxTitleLength {
		title = txt.Substr(title, 0, maxTitleLength) + "..."
	}

	sanitizedTitle := fs.SanitizeFilename(title)
	content := msg
	// If title is the same as content, we don't need to save it
	if sanitizedTitle == content {
		content = ""
	}
	// If title is already in the content, remove it.
	// See bot.restoreMsg() to see how the message is restored.
	if strings.HasPrefix(content, sanitizedTitle) {
		content = strings.TrimSpace(strings.TrimPrefix(content, sanitizedTitle))
	}

	return sanitizedTitle, content, nil
}

// If content is empty, use its filename as content.
// If file has content, add filename to the beginning of the content.
// If file has content, and filename was truncated (...), no need to add filename.
// If file has image and caption underneath it, no need to add title.
// The ugliest method so far.
func (b *Bot) restoreMsg(dir, filename string) (string, error) {
	msg, err := b.fs.Read(dir, filename)
	if err != nil {
		return "", fmt.Errorf("can't restore msg for '%s': %w", filename, err)
	}

	title := fs.Title(filename)
	nonTruncatedTitle := strings.TrimRight(title, "...")
	sanitizedContent := strings.ToLower(fs.SanitizeFilename(msg))
	contentHasNoTitle := !strings.HasPrefix(sanitizedContent, strings.ToLower(nonTruncatedTitle))
	hasNoImg := !txt.HasImage(msg)
	if len(msg) == 0 {
		return title, nil
	} else if contentHasNoTitle && hasNoImg {
		return fmt.Sprintf("%s\n%s", title, msg), nil
	}

	// msg has all the information, title doesn't have anything to add
	return msg, nil
}

func (b *Bot) tr(str string, args ...any) string {
	str = i18n.Tr(str)

	return fmt.Sprintf(str, args...)
}

// Replace last message + keyboard with the new one
// Or show the new one (in case of wimagehoto).
func (b *Bot) showHTML(validHTML string, kb *tg.Keyboard) error {
	b.delAllImages()

	mid, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if !hasLastKeyboard {
		b.delAllKeyboards()

		mid, err := b.tg.Send(b.userID, validHTML, kb, tg.MarkupHTML)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}

		b.db.SetLastKeyboardMsgID(mid)

		return nil
	}

	return b.tg.Edit(b.userID, mid, validHTML, kb, tg.MarkupHTML)
}

// Replace last message + keyboard with the new ones
// Or show the new one (in case of image).
// Read "Markdown to HTML conversion" section in readme's ADRs
// Chat allows 1-4096 characters AFTER entities parsing,
// meaning we can have 4096 plain chars + any amount of tags.
func (b *Bot) showMD(probablyInvalidMD string, kb *tg.Keyboard) error {
	b.delAllImages()

	probablyInvalidMD, images, links := txt.ExtractTextImgsLinks(probablyInvalidMD)

	for label, link := range links {
		dir := fs.DirRoot
		link = strings.TrimSpace(link)
		parts := strings.SplitN(link, "/", 2)
		if len(parts) == 2 {
			dir = parts[0]
			link = parts[1]
		}

		cmd := tg.NewCmd(consts.CmdShowFile, []string{fs.Hash(dir), fs.Hash(link)})
		kb.PrependRow(tg.NewRow(tg.NewBtn(txt.Ucfirst(label), cmd)))
	}

	mid, hasLastKeyboard := b.db.LastKeyboardMsgID()
	textChunks := txt.SplitTextIntoChunks(probablyInvalidMD, maxMsgLength)
	if !hasLastKeyboard || len(textChunks) > 1 || len(images) > 0 {
		b.delAllKeyboards()

		// Sending a gallery of images if there are any
		if len(images) > 0 {
			// We tolerate errors with the image gallery for now, text is more important
			mids, imgErr := b.tg.SendImages(b.userID, images)
			if imgErr == nil {
				for _, imgMid := range mids {
					b.db.AddImgMsgID(imgMid)
				}
			} else {
				slog.Error("Can't send images", "error", imgErr)
			}
		}

		// If our msg is too long, we send maxMsgsToSendAtOnce first messages.
		// Keyboard is attached to the last one
		textChunks = textChunks[0:min(maxMsgsToSendAtOnce, len(textChunks))]
		lastChunk := textChunks[len(textChunks)-1]
		textChunks = textChunks[0 : len(textChunks)-1]
		for _, textChunk := range textChunks {
			_, _ = b.tg.Send(b.userID, txt.MarkdownToHTML(textChunk), nil, tg.MarkupHTML)
		}

		mid, err := b.tg.Send(b.userID, txt.MarkdownToHTML(lastChunk), kb, tg.MarkupHTML)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}

		b.db.SetLastKeyboardMsgID(mid)

		return nil
	}

	return b.tg.Edit(b.userID, mid, txt.MarkdownToHTML(probablyInvalidMD), kb, tg.MarkupHTML)
}

func (b *Bot) showMoveTo(params []string) error {
	filenameHash := params[0]
	if b.cfg.NotesOnlyMode() {
		b.delAllKeyboards()

		return b.showMoveToFileOrDir([]string{filenameHash})
	}

	var kb tg.Keyboard
	userMoveToBtns := b.moveToBtns(filenameHash)
	if len(userMoveToBtns) == 0 {
		b.delAllKeyboards()

		return b.ShowToday(nil)
	}

	// Add recent command if any
	recentBtn := b.recentCmdBtn(filenameHash)
	if recentBtn != nil {
		userMoveToBtns = append(userMoveToBtns, *recentBtn)
	}

	userMoveToBtns = append(userMoveToBtns, tg.NewBtn(i18n.StrGoToToday, tg.NewCmd(consts.CmdShowToday, nil)))

	userBtnsByRows := slice.Chunk(userMoveToBtns, btnsPerRow)
	for _, row := range userBtnsByRows {
		kb.AddRow(row)
	}

	b.delAllKeyboards()

	err := b.showHTML(b.tr("Task added for <b>today</b>!"), &kb)
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}

	return nil
}

func (b *Bot) recentCmdBtn(filenameHash string) *tg.Btn {
	recentCmd, ok := b.db.RecentCommand()
	if !ok {
		return nil
	}

	args, _ := b.db.RecentCommandParams()
	args = append(args, filenameHash)
	targetFilenameHash := args[0]

	var unhashedTarget string
	var icon string
	if recentCmd == consts.CmdMoveToExistingFile {
		icon = i18n.Emoji("file")
		var err error
		unhashedTarget, err = b.fs.Unhash(fs.DirRoot, targetFilenameHash)
		if err != nil {
			return nil
		}
	} else if recentCmd == consts.CmdMoveToExistingNote {
		icon = i18n.Emoji("file")
		dir, err := b.fs.Unhash(fs.DirRoot, args[1])
		if err != nil {
			return nil
		}

		unhashedTarget, err = b.fs.Unhash(dir, targetFilenameHash)
		if err != nil {
			return nil
		}
	} else {
		return nil
	}

	name := fmt.Sprintf("%s %s", icon, fs.Title(unhashedTarget))
	btn := tg.NewBtn(name, tg.NewCmd(recentCmd, args))
	return &btn
}

func (b *Bot) ShowToday(_ []string) error {
	if b.cfg.NotesOnlyMode() {
		return b.showDirs(nil)
	}

	if b.cfg.JournalOnlyMode() {
		_, err := b.tg.Send(b.userID, "What's on your mind?", nil, tg.MarkupHTML)
		if err != nil {
			return fmt.Errorf("show today: can't send journal message: %w", err)
		}
		return nil
	}

	files, err := b.fs.FilesAndDirs(fs.DirToday)
	if err != nil {
		return fmt.Errorf("show today: can't get files in %s dir: %w", fs.DirToday, err)
	}

	// Adding tasks
	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		if file.IsMultiline {
			cmd := tg.NewCmd(consts.CmdShowMultilineTask, []string{fs.DirToday, fs.Hash(file.Name)})
			btn = tg.NewBtn(txt.Emoji(i18n.Emoji("eyes"), fs.UnsanitizeFilename(file.Title)), cmd)
		} else {
			cmd := tg.NewCmd(consts.CmdComplete, []string{fs.DirToday, fs.Hash(file.Name)})

			emoji := angerEmoji(file)
			// TODO add tests for all that
			if emoji == "" {
				emoji = i18n.Emoji(file.Title)
			} else if b.cfg.TwoEmojisPerButtonEnabled() {
				emoji += i18n.Emoji(file.Title)
			}
			btn = tg.NewBtn(txt.Emoji(emoji, fs.UnsanitizeFilename(file.Title)), cmd)
		}

		kb.AddRow(btn)
	}

	// Adding habits
	habitsRow := tg.NewRow()
	userHabits := make(map[string]habits.Year)
	if b.cfg.QuickHabitsEnabled() {
		// We can tolerate missing habits
		userHabits, _ = habits.LastWeekHabits(b.fs)
		_, ok := userHabits[habits.MoodHabit]
		if ok {
			delete(userHabits, habits.MoodHabit)
		}
	}
	for habit, year := range userHabits {
		if completed, _ := year[time.Now().YearDay()]; completed == 1 {
			continue
		}

		cmd := tg.NewCmd(consts.CmdCompleteHabit, []string{habit})
		habitsRow = append(habitsRow, tg.NewBtn(habits.Emoji(b.fs, habit), cmd))
	}
	if len(habitsRow) > 0 {
		kb.AddRow(habitsRow)
	}

	// Adding quick buttons
	quickBtns := b.quickBtns()
	if len(quickBtns) > 0 {
		quickBtnsByRows := slice.Chunk(quickBtns, quickBtnsPerRow)
		for _, row := range quickBtnsByRows {
			kb.AddRow(row)
		}
	}

	msg := b.todayLabel()
	err = b.showHTML(msg, &kb)
	if err != nil {
		return fmt.Errorf("show list: %w", err)
	}

	return nil
}

func (b *Bot) showLaterTasks(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirLater)
	if err != nil {
		return fmt.Errorf("show list: can't get files in %s dir: %w", fs.DirLater, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		name := i18n.Emojify(fs.UnsanitizeFilename(file.Title))
		if file.IsMultiline {
			cmd := tg.NewCmd(consts.CmdShowMultilineTask, []string{fs.DirLater, fs.Hash(file.Name)})
			btn = tg.NewBtn(txt.Emoji(i18n.Emoji("eyes"), fs.UnsanitizeFilename(file.Title)), cmd)
		} else {
			cmd := tg.NewCmd(consts.CmdComplete, []string{fs.DirLater, fs.Hash(file.Name)})
			btn = tg.NewBtn(name, cmd)
		}

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))

	msg := b.tr("⏳ Your tasks for <b>later</b>:")
	err = b.showHTML(msg, &kb)
	if err != nil {
		return fmt.Errorf("show list: %w", err)
	}

	return nil
}

func (b *Bot) todayLabel() string {
	var statusBar string

	hasPomodoroInToday, _ := b.fs.Exists(fs.DirToday, fs.FilePomodoro)
	if hasPomodoroInToday {
		statusBar = i18n.Emoji(fs.Title(fs.FilePomodoro))
	}

	filesAndDirs, _ := b.fs.FilesAndDirs(fs.DirToday)
	todayTasks := fs.ExcludePomodoro(fs.OnlyMDFiles(filesAndDirs))
	if len(todayTasks) == 0 {
		statusBar += i18n.Emoji("palm")
	}

	if len(statusBar) != 0 {
		statusBar += " "
	}

	if len(todayTasks) == 0 {
		return statusBar + i18n.Tr("You don't have any tasks!")
	}

	return statusBar + fmt.Sprintf(i18n.Tr("<b>%d</b> left%s"), len(todayTasks), wideSpacer)
}

func (b *Bot) showFiles(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("show files: can't get files: %w", err)
	}

	var kb tg.Keyboard
	mdFiles := fs.ExcludeConfig(fs.OnlyMDFiles(files))
	var fileBtns []tg.Btn
	for _, file := range mdFiles {
		cmd := tg.NewCmd(consts.CmdShowFile, []string{fs.DirRoot, fs.Hash(file.Name)})
		btn := tg.NewBtn(fmt.Sprintf("📄 %s", fs.UnsanitizeFilename(file.Title)), cmd)
		fileBtns = append(fileBtns, btn)
	}
	fileBtnsByRows := slice.Chunk(fileBtns, btnsPerRow)
	for _, row := range fileBtnsByRows {
		kb.AddRow(row)
	}
	inlineCmd := tg.NewCustomCmd(consts.CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)

	footer := tg.NewRow(tg.NewBtn(i18n.Tr("🔎 Search"), inlineCmd))
	if !b.cfg.NotesOnlyMode() {
		footer = append(footer, tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))
	}
	kb.AddRow(footer)

	err = b.showHTML(b.tr("📄 Your files:")+wideSpacer, &kb)
	if err != nil {
		return fmt.Errorf("show files: %w", err)
	}

	return nil
}

func (b *Bot) showDirs(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("show dirs: can't get dirs: %w", err)
	}

	dirs := fs.OnlyNoteDirs(fs.OnlyDirs(files))
	var dirBtns []tg.Btn
	for _, dir := range dirs {
		cmd := tg.NewCustomCmd("", []string{dir.Name}, tg.CmdTypeInlineQueryCurrentChat)
		btn := tg.NewBtn(fmt.Sprintf("%s %s", i18n.Emoji("dir"), dir.Title), cmd)
		dirBtns = append(dirBtns, btn)
	}

	var kb tg.Keyboard
	dirBtnsByRows := slice.Chunk(dirBtns, btnsPerRow)
	for _, row := range dirBtnsByRows {
		kb.AddRow(row)
	}

	inlineCmd := tg.NewCustomCmd(consts.CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)
	footer := tg.NewRow(tg.NewBtn(i18n.Tr("🔎 Search"), inlineCmd))
	if !b.cfg.NotesOnlyMode() {
		footer = append(footer, tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))
	}
	kb.AddRow(footer)

	err = b.showHTML(b.tr("🗂 Your dirs:")+wideSpacer, &kb)
	if err != nil {
		return fmt.Errorf("show dirs: %w", err)
	}

	return nil
}

func (b *Bot) showChecklists(_ []string) error {
	checklists, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}
	checklists = fs.OnlyChecklists(checklists)

	var kb tg.Keyboard
	for _, checklist := range checklists {
		cmd := tg.NewCmd(consts.CmdShowChecklist, []string{fs.Hash(checklist.Name)})
		btn := tg.NewBtn(i18n.Emojify(checklistTitle(checklist.Name)), cmd)

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(b.tr("🏠 Today"), tg.NewCmd(consts.CmdShowToday, nil)))

	err = b.showHTML(b.tr("☑️ Checklists"), &kb)
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}

	return nil
}

func (b *Bot) showPostpone(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirToday)
	if err != nil {
		return fmt.Errorf("show postpone: can't get files in '%s' dir: %w", fs.DirToday, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		cmd := tg.NewCmd(consts.CmdPostpone, []string{fs.Hash(file.Name)})
		kb.AddRow(tg.NewBtn(file.Title, cmd))
	}

	kb.AddRow(tg.NewRow(
		tg.NewBtn(b.tr("Rename"), tg.NewCmd(consts.CmdShowRename, []string{})),
		tg.NewBtn(b.tr("OK"), tg.NewCmd(consts.CmdShowToday, []string{})),
	))

	err = b.showHTML(b.tr("🦥 Select a task to postpone:"), &kb)
	if err != nil {
		return fmt.Errorf("show postpone: %w", err)
	}

	return nil
}

func (b *Bot) showMoveFromToday(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirToday)
	if err != nil {
		return fmt.Errorf("show move from today: can't get files in '%s' dir: %w", fs.DirToday, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		cmd := tg.NewCmd(consts.CmdShowMoveTo, []string{fs.Hash(file.Name)})
		kb.AddRow(tg.NewBtn(file.Title, cmd))
	}

	kb.AddRow(tg.NewRow(
		tg.NewBtn(b.tr("Rename"), tg.NewCmd(consts.CmdShowRename, []string{})),
		tg.NewBtn(b.tr("OK"), tg.NewCmd(consts.CmdShowToday, []string{})),
	))

	err = b.showHTML(b.tr("🦥 Select a task to move:"), &kb)
	if err != nil {
		return fmt.Errorf("show move from today: %w", err)
	}

	return nil
}

func (b *Bot) postpone(params []string) error {
	// TODO Remove input expectations if dir is not today (?)
	filenameHash := params[0]

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("postpone: can't unhash old filename %s in %s: %w", fs.DirToday, filenameHash, err)
	}

	// TODO touch
	// TODO multiline
	err = b.fs.Rename(fs.DirToday, filename, fs.DirLater, filename)
	if err != nil {
		return fmt.Errorf("postpone: can't move: %w", err)
	}

	return b.showPostpone(nil)
}

// TODO add tests
// TODO add ability to rename later task?
func (b *Bot) showRename(_ []string) error {
	dir := fs.DirToday

	files, err := b.fs.FilesAndDirs(dir)
	if err != nil {
		return fmt.Errorf("rename: can't get files in %s dir: %w", dir, err)
	}
	files = fs.OnlyMDFiles(files)

	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		cmd := tg.NewCmd(consts.CmdShowRenameFile, []string{dir, fs.Hash(file.Name)})
		btn = tg.NewBtn(txt.Emoji(i18n.Emoji("eyes"), file.Title), cmd)

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))

	err = b.showHTML(b.todayLabel(), &kb)
	if err != nil {
		return fmt.Errorf("show rename: %w", err)
	}

	return nil
}

func (b *Bot) showRenameFile(params []string) error {
	dir := params[0]
	filenameHash := params[1]

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("show rename: can't unhash filename %s in %s: %w", filenameHash, dir, err)
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBack, tg.NewCmd(dir, []string{dir}))),
	})

	cmd := tg.NewCmd(consts.CmdRename, []string{dir, filename, "%s"})
	b.db.SetInputExpectation(cmd)

	err = b.showHTML(i18n.Tr("OK. Send me the new name for your task"), kb)
	if err != nil {
		return fmt.Errorf("show rename: %w", err)
	}

	return nil
}

func (b *Bot) rename(params []string) error {
	dirHash := params[0]
	fromFilenameHash := params[1]
	newFilenameFromUserInput := params[2]

	dir, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash old dir: %w", err)
	}

	filename, err := b.fs.Unhash(dir, fromFilenameHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash old filename: %w", err)
	}

	err = b.fs.Rename(dir, filename, dir, newFilenameFromUserInput)
	if err != nil {
		return fmt.Errorf("move: can't move: %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) showStats(_ []string) error {
	report, err := stats.TodayReport(b.fs, b.db, b.userID)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil))})
	err = b.showHTML(report, kb)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	return nil
}

func (b *Bot) showSchedule(_ []string) error {
	scheduledTasks, err := b.cfg.Schedules()
	if err != nil {
		return fmt.Errorf("show schedule: %w", err)
	}
	schedule := sched.ScheduleReport(scheduledTasks)
	if len(schedule) == 0 {
		schedule = i18n.Tr("You don't have any scheduled tasks! 🌴")
	}

	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil))})
	err = b.showHTML(schedule, kb)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	return nil
}

func (b *Bot) showRead(_ []string) error {
	return b.showChecklist([]string{fs.Hash(fs.DirRead)})
}

func (b *Bot) showWatch(_ []string) error {
	return b.showChecklist([]string{fs.Hash(fs.DirWatch)})
}

func (b *Bot) showShop(_ []string) error {
	return b.showChecklist([]string{fs.Hash(fs.DirShop)})
}

func (b *Bot) showMultilineTask(params []string) error {
	dir := params[0]
	filenameHash := params[1]

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	content, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	var moveToLaterBtn tg.Btn
	btnLabel := i18n.StrMoveToLaterLong
	toDir := fs.DirLater
	if dir == fs.DirLater {
		btnLabel = i18n.StrToToday
		toDir = fs.DirToday
	}
	moveToLaterBtn = tg.NewBtn(btnLabel, tg.NewCmd(consts.CmdMoveToExistingDir, []string{toDir, dir, filenameHash}))

	moveBtn := tg.NewBtn(
		txt.Emoji(i18n.Emoji("right arrow"), b.tr("Move to")),
		tg.NewCmd(consts.CmdShowMoveTo, []string{filenameHash}),
	)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(moveToLaterBtn, moveBtn),
		tg.NewRow(
			tg.NewBtn(i18n.StrBack, tg.NewCmd(dir, []string{dir})),
			tg.NewBtn(i18n.StrComplete, tg.NewCmd(consts.CmdComplete, []string{dir, filenameHash})),
		),
	})

	md := fmt.Sprintf("**%s**\n%s", fs.Title(filename), content)
	err = b.showMD(md, kb)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if hasLastKeyboard {
		b.db.SetFilenameByMsgID(msgID, filename)
		b.db.SetDirByMsgID(msgID, dir)
	}

	return nil
}

func (b *Bot) showFile(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("show file: can't find dir: %w", err)
	}

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("show file: can't find file: %w", err)
	}

	content, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("show file: %w", err)
	}

	isNotesDir := len(fs.OnlyNoteDirs([]fs.File{{Name: dir}})) > 0
	row := tg.NewRow()
	if isNotesDir {
		inlineCmd := tg.NewCustomCmd(consts.CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)
		row = append(row, tg.NewBtn(i18n.Tr("🔎 Search"), inlineCmd))

		hasChannelsToPrint := len(b.cfg.Channels()) > 0
		if hasChannelsToPrint {
			cmd := tg.NewCmd(consts.CmdShare, []string{fs.Hash(dir), fs.Hash(filename)})
			row = append(row, tg.NewBtn(i18n.Tr("🖨 Share"), cmd))
		}
	}
	row = append(row, tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))
	kb := tg.NewKeyboard([]tg.Row{row})

	md := fmt.Sprintf("**%s**\n\n%s", fs.Title(filename), content)
	err = b.showMD(md, kb)
	if err != nil {
		return fmt.Errorf("show file: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if hasLastKeyboard {
		b.db.SetFilenameByMsgID(msgID, filename)
		b.db.SetDirByMsgID(msgID, dir)
	}

	return nil
}

func (b *Bot) showChecklist(params []string) error {
	dirHash := params[0]

	checklist, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	items, err := b.fs.FilesAndDirs(checklist)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}
	items = fs.SortByCtimeDesc(items)
	slices.Reverse(items)

	// TODO check that we're showing last buttons
	maxButtons := maxBtns
	if checklist == fs.DirRead || checklist == fs.DirWatch {
		maxButtons = maxBtnsInChecklist
	}
	items = items[max(0, len(items)-maxButtons):]

	kb := tg.NewKeyboard(nil)
	for _, item := range items {
		if item.IsMultiline {
			title := txt.Emoji(i18n.Emoji("eyes"), fs.UnsanitizeFilename(item.Title))
			kb.AddRow(tg.NewBtn(title, tg.NewCmd(consts.CmdShowChecklistItem, []string{dirHash, item.Hash})))
		} else {
			title := i18n.Emojify(fs.UnsanitizeFilename(item.Title))
			kb.AddRow(tg.NewBtn(title, tg.NewCmd(consts.CmdCompleteChecklistItem, []string{dirHash, item.Hash})))
		}
	}
	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))

	title := checklistTitle(checklist)
	if checklist == fs.DirRead {
		title = i18n.Tr(i18n.Emojify("Reading List"))
	} else if checklist == fs.DirWatch {
		title = i18n.Tr(i18n.Emojify("Watchlist"))
	} else if checklist == fs.DirShop {
		title = i18n.Tr(i18n.Emojify("Shopping List"))
	}
	err = b.showHTML(title+wideSpacer, kb)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	return nil
}

func (b *Bot) showStart(params []string) error {
	if len(params) > 0 {
		mode := strings.ToLower(params[0])
		if mode == "notes" {
			return b.notesOnlyMode(nil)
		} else if mode == "tasks" {
			return b.tasksOnlyMode(nil)
		} else if mode == "journal" {
			return b.journalOnlyMode(nil)
		} else if mode == "full" {
			return b.fullMode(nil)
		}
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(txt.Emoji(i18n.Emoji("notes"), b.tr("Notes only")), tg.NewCmd(consts.CmdNotesOnlyMode, nil))),
		tg.NewRow(tg.NewBtn(txt.Emoji(i18n.Emoji("tasks"), b.tr("Tasks only")), tg.NewCmd(consts.CmdTasksOnlyMode, nil))),
		tg.NewRow(tg.NewBtn(txt.Emoji(i18n.Emoji("journal"), b.tr("Journal only")), tg.NewCmd(consts.CmdJournalOnlyMode, nil))),
		tg.NewRow(tg.NewBtn(txt.Emoji(i18n.Emoji("brain"), b.tr("Everything")), tg.NewCmd(consts.CmdFullMode, nil))),
	})

	return b.showHTML("Welcome 👋! What do you need?", kb)
}

func (b *Bot) moveToDir(params []string) error {
	// TODO Remove input expectations if dir is not today
	toDirHash := params[0]
	fromDirHash := params[1]
	fromFilenameHash := params[2]

	oldDir, err := b.fs.Unhash(fs.DirRoot, fromDirHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash old dir: %w", err)
	}

	filename, err := b.fs.Unhash(oldDir, fromFilenameHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash old filename: %w", err)
	}
	newFilename := filename
	if len(params) > 3 {
		newFilename = params[3]
	}

	toDir, err := b.fs.Unhash(fs.DirRoot, toDirHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash new dir %s: %w", toDir, err)
	}

	// TODO touch
	// TODO multiline ?
	err = b.fs.Rename(oldDir, filename, toDir, newFilename)
	if err != nil {
		return fmt.Errorf("move: can't move: %w", err)
	}

	notesDir := fs.OnlyNoteDirs([]fs.File{{Name: toDir}})
	isNotesDir := len(notesDir) == 1
	if isNotesDir {
		// We can tolerate this, as this is informative logging
		_ = journal.AddRecord(b.fs, fmt.Sprintf("📌 %s", fs.Title(filename)), b.cfg.Timezone())
	}

	if toDir != fs.DirLater {
		b.db.SetRecentCommand(consts.CmdMoveToExistingNote)
		// Move from dir is today, because quick command
		// appears when file is in today dir
		b.db.SetRecentCommandParams([]string{fs.Hash(filename), toDirHash})
	}

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("dir"), fmt.Sprintf(i18n.Tr("Moved to <b>%s</b>"), fs.Title(toDir)))
	// Just an informative messages
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) requestNewDirName(params []string) error {
	filenameHash := params[0]

	err := b.showHTML(i18n.Tr("OK. Send me the name for your new dir"), nil)
	if err != nil {
		return fmt.Errorf("request new dir: %w", err)
	}

	b.db.SetInputExpectation(tg.NewCmd(consts.CmdMoveToNewDir, []string{filenameHash, "%s"}))

	return nil
}

func (b *Bot) moveToNewDir(params []string) error {
	filenameHash := params[0]
	dir := strings.ToLower(fs.SanitizeFilename(params[1]))

	exists, err := b.fs.Exists(fs.DirRoot, dir)
	if err != nil {
		return fmt.Errorf("move to new dir: %w", err)
	}
	if exists {
		return b.moveToDir([]string{dir, fs.DirRoot, filenameHash})
	}

	err = b.fs.MakeDir(dir)
	if err != nil {
		return fmt.Errorf("move to new dir: %w", err)
	}

	return b.moveToDir([]string{dir, fs.DirRoot, filenameHash})
}

func (b *Bot) moveToExistingFile(params []string) error {
	// TODO Remove input expectations if dir is not today (?)
	existingFilenameHash := params[0]
	fromDirHash := params[1]
	fromFilenameHash := params[2]

	existingFilename, err := b.fs.Unhash(fs.DirRoot, existingFilenameHash)
	if err != nil {
		return fmt.Errorf("move to file: can't unhash existing file '%s': %w", fromFilenameHash, err)
	}

	// TODO add test for adding to same file, it seems it is broken (after we added short hash)
	if fromFilenameHash == existingFilenameHash {
		b.delAllKeyboards()
		msg := txt.Emoji(i18n.Emoji("file"), fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.Title(existingFilename)))
		// Just an informative messages
		_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)
		return b.ShowToday(nil)
	}

	fromDir, err := b.fs.Unhash(fs.DirRoot, fromDirHash)
	if err != nil {
		return fmt.Errorf("move to file: can't unhash from dir '%s': %w", fromFilenameHash, err)
	}

	fromFilename, err := b.fs.Unhash(fromDir, fromFilenameHash)
	if err != nil {
		return fmt.Errorf("move to file: can't unhash new filename '%s': %w", fromFilenameHash, err)
	}

	content, err := b.restoreMsg(fromDir, fromFilename)
	if err != nil {
		return fmt.Errorf("move to file: can't read content of '%s': %w", fromFilename, err)
	}

	// We can tolerate this
	_ = b.fs.Del(fromDir, fromFilename)

	err = b.addToFile(fs.DirRoot, existingFilename, content)
	if err != nil {
		return fmt.Errorf("move to file: can't add to existing file: %w", err)
	}

	b.db.SetRecentCommand(consts.CmdMoveToExistingFile)
	b.db.SetRecentCommandParams([]string{fs.ShortHash(existingFilename), fs.ShortHash(fs.DirToday)})

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("file"), fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.Title(existingFilename)))
	// Just an informative messages
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

// Move from today to existing note
func (b *Bot) moveToExistingNote(params []string) error {
	toFilenameHash := params[0]
	toDirHash := params[1]
	fromFilenameHash := params[2]

	toDir, err := b.fs.Unhash(fs.DirRoot, toDirHash)
	if err != nil {
		return fmt.Errorf("move to exsiting note: %w", err)
	}

	toFilename, err := b.fs.Unhash(toDir, toFilenameHash)
	if err != nil {
		return fmt.Errorf("move to exsiting note:: %w", err)
	}

	fromFilename, err := b.fs.Unhash(fs.DirToday, fromFilenameHash)
	if err != nil {
		return fmt.Errorf("move to existing note:: %w", err)
	}

	content, err := b.restoreMsg(fs.DirToday, fromFilename)
	if err != nil {
		return fmt.Errorf("move to existing note: can't read file %s: %w", fromFilename, err)
	}

	err = b.addToFile(toDir, toFilename, content)
	if err != nil {
		return fmt.Errorf("move to existing note: can't add to file %s: %w", toFilename, err)
	}

	// No worries if we can't delete - we'll have a redundant file
	_ = b.fs.Del(fs.DirToday, fromFilename)

	return b.ShowToday(nil)
}

func (b *Bot) moveToChecklist(params []string) error {
	filenameHash := params[0]
	checklistHash := params[1]

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("move to checkilst: %w", err)
	}

	checklist, err := b.fs.Unhash(fs.DirRoot, checklistHash)
	if err != nil {
		return fmt.Errorf("move to checklist: %w", err)
	}

	isMultiline, err := b.fs.IsMultiline(fs.DirToday, filename)
	if err != nil {
		return fmt.Errorf("move to checklist: %w", err)
	}

	if isMultiline && b.cfg.ShouldSplitChecklist(checklist) {
		content, err := b.fs.Read(fs.DirToday, filename)
		if err != nil {
			return fmt.Errorf("move to checklist: %w", err)
		}

		content = strings.TrimSpace(txt.NormNewLines(content))
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = fs.SanitizeFilename(line)
			err = b.fs.Write(checklist, fs.Filename(line), "")
			if err != nil {
				return fmt.Errorf("move to checklist: %w", err)
			}
		}
	} else {
		err = b.fs.Rename(fs.DirToday, filename, checklist, filename)
		if err != nil {
			return fmt.Errorf("move to checklist: %w", err)
		}
	}

	// We can tolerate this
	_ = b.fs.Del(fs.DirToday, filename)

	return b.ShowToday(nil)
}

func (b *Bot) moveToRead(params []string) error {
	filenameHash := params[0]

	return b.moveToChecklist([]string{filenameHash, fs.Hash(fs.DirRead)})
}

func (b *Bot) moveToWatch(params []string) error {
	filenameHash := params[0]

	return b.moveToChecklist([]string{filenameHash, fs.Hash(fs.DirWatch)})
}

func (b *Bot) moveToShop(params []string) error {
	filenameHash := params[0]

	return b.moveToChecklist([]string{filenameHash, fs.Hash(fs.DirShop)})
}

// TODO test
func (b *Bot) moveToNewFile(params []string) error {
	existingFilenameHash := params[0]
	newFilenameFromUserInput := fs.Filename(params[1])

	filename, err := b.fs.Unhash(fs.DirRoot, existingFilenameHash)
	if err != nil {
		return fmt.Errorf("move to new file: can't unhash existing file '%s': %w", existingFilenameHash, err)
	}

	// Save existing filename to content in case the content of new file is empty (i.e. not multiline)
	content, err := b.fs.Read(fs.DirRoot, filename)
	if err != nil {
		return fmt.Errorf("move to new file: can't read file '%s': %w", filename, err)
	}

	content = strings.TrimSpace(content)
	if len(content) == 0 {
		content = fs.Title(filename)
		err = b.fs.Write(fs.DirRoot, filename, content)
		if err != nil {
			return fmt.Errorf("move to new file: can't write content of '%s': %w", filename, err)
		}
	}

	// TODO check for safety
	// TODO won't we lost some text here in case of multiline?
	err = b.fs.Rename(fs.DirRoot, filename, fs.DirRoot, newFilenameFromUserInput)
	if err != nil {
		return fmt.Errorf("move to new file: can't create empty file: %w", err)
	}

	// We can tolerate this
	_ = journal.AddRecord(b.fs, fmt.Sprintf("📄 %s", fs.Title(filename)), b.cfg.Timezone())

	// TODO test
	b.db.SetRecentCommand(consts.CmdMoveToExistingFile)
	b.db.SetRecentCommandParams([]string{fs.ShortHash(newFilenameFromUserInput), fs.ShortHash(fs.DirToday)})

	msg := txt.Emoji(i18n.Emoji("file"), fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.Title(newFilenameFromUserInput)))
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToNewChecklist(params []string) error {
	filenameHash := params[0]
	supposedName := params[1]
	supposedName = fs.SanitizeFilename(supposedName)

	dir := strings.ToLower(supposedName)
	dir = fmt.Sprintf("_%s_", dir)
	exists, err := b.fs.Exists(fs.DirRoot, dir)
	if err != nil {
		return fmt.Errorf("move to new checklist: %w", err)
	}
	if exists {
		return b.moveToDir([]string{dir, fs.DirRoot, filenameHash})
	}

	err = b.fs.MakeDir(dir)
	if err != nil {
		return fmt.Errorf("move to new checklist: %w", err)
	}

	return b.moveToDir([]string{dir, fs.DirToday, filenameHash})
}

func (b *Bot) moveToJournal(params []string) error {
	filenameHash := params[0]
	fromFilename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("move to journal: can't unhash filename: %w", err)
	}

	content, err := b.restoreMsg(fs.DirToday, fromFilename)
	if err != nil {
		return fmt.Errorf("move to journal: can't read content of '%s': %w", fromFilename, err)
	}

	err = journal.AddRecord(b.fs, content, b.cfg.Timezone())
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't add note: %w", err)
	}

	err = b.fs.Del(fs.DirToday, fromFilename)
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't delete note: %w", err)
	}

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("journal"), fmt.Sprintf(i18n.Tr("Saved to <b>journal</b>")))
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	if b.cfg.JournalOnlyMode() {
		return nil
	}

	return b.ShowToday(nil)
}

func (b *Bot) addToJournalFromShortcut(params []string) error {
	content := params[0]

	// TODO change to pass text
	err := journal.AddRecord(b.fs, content, b.cfg.Timezone())
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't add note: %w", err)
	}

	msg := i18n.Tr("Saved to <b>Journal</b>")
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

// TODO add tests
func (b *Bot) addToRecentFileOrNoteFromShortcut(params []string) error {
	content := params[0]

	args, _ := b.db.RecentCommandParams()
	if len(args) < 2 {
		return nil
	}
	cmd, _ := b.db.RecentCommand()

	var existingFilename string
	if cmd == consts.CmdMoveToExistingFile {
		var err error
		existingFilename, err = b.fs.Unhash(fs.DirRoot, args[0])
		if err != nil {
			return fmt.Errorf("failed to move to recent file or note: can't unhash filename: %w", err)
		}

		err = b.addToFile(fs.DirRoot, existingFilename, content)
		if err != nil {
			return fmt.Errorf("failed to move to recent file: can't add note: %w", err)
		}
	} else if cmd == consts.CmdMoveToExistingNote {
		dir, err := b.fs.Unhash(fs.DirRoot, args[1])
		if err != nil {
			return fmt.Errorf("failed to move to recent note: can't unhash dir: %w", err)
		}
		existingFilename, err = b.fs.Unhash(dir, args[0])
		if err != nil {
			return fmt.Errorf("failed to move to recent note: can't unhash filename: %w", err)
		}

		err = b.addToFile(dir, existingFilename, content)
		if err != nil {
			return fmt.Errorf("failed to move to recent note: can't add note: %w", err)
		}
	} else {
		return nil
	}

	msg := fmt.Sprintf(i18n.Tr("Added to <b>%s</b>"), fs.Title(existingFilename))
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToLater(params []string) error {
	filenameHash := params[0]

	return b.moveToDir([]string{fs.DirLater, fs.DirToday, filenameHash})
}

func (b *Bot) complete(params []string) error {
	dir := params[0]
	filenameHash := params[1]

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("complete: can't unhash filename %s: %w", filename, err)
	}

	if err = b.fs.Touch(dir, filename); err != nil {
		return fmt.Errorf("complete: can't touch %s: %w", filename, err)
	}

	// TODO multiline?
	err = b.fs.Rename(dir, filename, fs.DirArchive, filename)
	if err != nil {
		return fmt.Errorf("complete: can't complete %s: %w", filename, err)
	}

	if dir == fs.DirToday && filename == fs.FilePomodoro {
		err = b.cfg.AddToSchedule(filename, time.Now().Unix()+int64(b.cfg.PomodoroDuration().Seconds()), "")
		if err != nil {
			return fmt.Errorf("complete: can't add to schedule: %w", err)
		}
	} else {
		// We can tolerate failure of writing to journal, since that's not single source of truth
		_ = journal.AddRecord(b.fs, fmt.Sprintf("✅ %s", fs.Title(filename)), b.cfg.Timezone())
	}

	if dir == fs.DirLater {
		return b.showLaterTasks(nil)
	}

	return b.ShowToday(nil)
}

func (b *Bot) completeChecklistItem(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("complete: can't unhash dir %s: %w", dir, err)
	}

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("complete: can't unhash filename %s: %w", filename, err)
	}

	if err = b.fs.Touch(dir, filename); err != nil {
		return fmt.Errorf("complete: can't touch %s: %w", filename, err)
	}

	// Informative records, we don't need to check for errors
	if dir == fs.DirRead {
		_ = journal.AddRecord(b.fs, fmt.Sprintf("📚 Read %s", filename), b.cfg.Timezone())
	} else if dir == fs.DirWatch {
		_ = journal.AddRecord(b.fs, fmt.Sprintf("📺 Watched %s", filename), b.cfg.Timezone())
	}

	err = b.fs.Rename(dir, filename, fs.DirArchive, filename)
	if err != nil {
		return fmt.Errorf("complete: can't complete %s: %w", filename, err)
	}

	return b.showChecklist([]string{dirHash})
}

func (b *Bot) showChecklistItem(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("show checklist item: can't unhash dir %s: %w", dir, err)
	}

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("show checklist item: can't unhash filename %s: %w", filename, err)
	}

	content, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("show checklist item: can't read content of %s: %w", filename, err)
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn(i18n.StrBack, tg.NewCmd(consts.CmdShowChecklist, []string{dirHash})),
			tg.NewBtn(i18n.StrComplete, tg.NewCmd(consts.CmdCompleteChecklistItem, []string{dirHash, filenameHash})),
		),
	})

	err = b.showHTML(content, kb)
	if err != nil {
		return fmt.Errorf("show checklist item: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if hasLastKeyboard {
		b.db.SetFilenameByMsgID(msgID, filename)
		b.db.SetDirByMsgID(msgID, dir)
	}

	return nil
}

func (b *Bot) schedule(params []string) error {
	filenameHash := params[0]
	timeStr := params[1]
	cron := params[2]

	scheduleTime, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("schedule: can't parse timestamp: %w", err)
	}

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("schedule: can't unhash filename %s in list: %s", filenameHash, err)
	}

	err = b.cfg.AddToSchedule(filename, scheduleTime, cron)
	if err != nil {
		return fmt.Errorf("schedule: can't add to schedule: %w", err)
	}

	err = b.fs.Rename(fs.DirToday, filename, fs.DirLater, filename)
	if err != nil {
		return fmt.Errorf("schedule: can't rename file %s: %w", filename, err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) scheduleForTmrw(params []string) error {
	filenameHash := params[0]

	return b.schedule([]string{filenameHash, txt.I64(sched.Tomorrow()), ""})
}

func (b *Bot) delAllKeyboards() {
	var msgIDs []int
	mid, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if hasLastKeyboard {
		b.db.DelLastKeyboardMsgID()
		msgIDs = append(msgIDs, mid)
	}

	// No worries if we fail - it will be cleaned up by the worker
	for _, msgID := range msgIDs {
		// If we fail to del - user would get a bunch
		// of keyboards in one chat, which is messy but not critical
		_ = b.tg.Del(b.userID, msgID)
	}
}

func (b *Bot) delAllImages() {
	mids, hasSentImages := b.db.ImgMsgID()
	if !hasSentImages {
		return
	}

	b.db.DelImgMsgID()
	for _, mid := range mids {
		// If we fail to del - user would get a bunch
		// of keyboards in one chat, which is messy but not critical
		_ = b.tg.Del(b.userID, mid)
	}
}

func (b *Bot) showToADay(params []string) error {
	filenameHash := params[0]

	kb, err := b.toADayKeyboard(filenameHash)
	if err != nil {
		return fmt.Errorf("show for a day: %w", err)
	}

	err = b.showHTML(i18n.Tr("Choose a day"), kb)
	if err != nil {
		return fmt.Errorf("show for a day: %w", err)
	}

	return nil
}

func (b *Bot) toADayKeyboard(filenameHash string) (*tg.Keyboard, error) {
	newBtn := func(name, cron string) tg.Btn {
		return tg.NewBtn(name, tg.NewCmd(consts.CmdSchedule, []string{filenameHash, txt.I64(sched.NextExcludeToday(cron)), ""}))
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrRepeat, tg.NewCmd(consts.CmdShowScheduleForDayRecurring, []string{filenameHash}))),
		tg.NewRow(
			newBtn(i18n.StrMonday, "0 0 * * 1"),
			newBtn(i18n.StrTuesday, "0 0 * * 2"),
			newBtn(i18n.StrWednesday, "0 0 * * 3"),
			newBtn(i18n.StrThursday, "0 0 * * 4"),
		),
		tg.NewRow(
			newBtn(i18n.StrFriday, "0 0 * * 5"),
			newBtn(i18n.StrSaturday, "0 0 * * 6"),
			newBtn(i18n.StrSunday, "0 0 * * 0"),
		),
	})

	for _, iAndj := range [][]int{{1, 8}, {9, 16}, {17, 24}, {25, 31}} {
		row := tg.NewRow()
		for i := iAndj[0]; i <= iAndj[1]; i++ {
			cron := fmt.Sprintf("0 0 %d * *", i)
			row = append(row, newBtn(txt.I64(int64(i)), cron))
		}
		kb.AddRow(row)
	}
	kb.AddRow(tg.NewBtn(i18n.StrToToday, tg.NewCmd(consts.CmdShowToday, nil)))

	return kb, nil
}

func (b *Bot) showMoveToFileOrDir(params []string) error {
	filenameHash := params[0]
	maxRecentBtns := maxGroupedBtnsInMoveTo

	filename := ""
	// If there's a second param that we want to show all the buttons (user clicked More...)
	userWantedAllBtns := len(params) > 1
	if userWantedAllBtns {
		maxRecentBtns = maxBtns
		var err error
		filename, err = b.fs.Unhash(fs.DirRoot, filenameHash)
		if err != nil {
			return fmt.Errorf("to file dialog: %w", err)
		}
	} else {
		// For the first time we have to move file to the root directory, as this is not a task anymore
		var err error
		filename, err = b.fs.Unhash(fs.DirToday, filenameHash)
		if err != nil {
			return fmt.Errorf("to file dialog: %w", err)
		}

		err = b.fs.Rename(fs.DirToday, filename, fs.DirRoot, filename)
		if err != nil {
			return fmt.Errorf("to file dialog: %w", err)
		}

		b.db.SetRecentCommand(consts.CmdMoveToExistingFile)
		b.db.SetRecentCommandParams([]string{fs.ShortHash(filename), fs.ShortHash(fs.DirRoot)})
	}

	kb := tg.NewKeyboard(nil)
	skippedBtns := false

	fileBtns, err := b.moveToFileBtns(fs.ShortHash(filename))
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}
	if len(fileBtns) > maxRecentBtns {
		fileBtns = fileBtns[:maxRecentBtns]
		skippedBtns = true
	}
	// Move newly created file to the end of the files list
	if len(fileBtns) > 0 {
		fileBtns = append(fileBtns[1:], fileBtns[0])
	}
	fileBtnsByRows := slice.Chunk(fileBtns, btnsPerRow)
	for _, row := range fileBtnsByRows {
		kb.AddRow(row)
	}

	dirBtns, err := b.moveToDirBtns(filenameHash)
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}
	if len(dirBtns) > maxRecentBtns {
		dirBtns = dirBtns[:maxRecentBtns]
		skippedBtns = true
	}
	// Add "New Dir" to the end of the dirs list
	// TODO add tests for all these cases
	if len(dirBtns) == maxRecentBtns {
		// Free up space for the new dir button
		dirBtns = dirBtns[:len(fileBtns)-1]
	}
	btn := tg.NewBtn("🗂 New Dir", tg.NewCmd(consts.CmdRequestNewDir, []string{filenameHash}))
	dirBtns = append(dirBtns, btn)

	shouldAddSeparator := len(fileBtns) > 0
	if shouldAddSeparator {
		searchCMD := tg.NewCustomCmd(consts.CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)
		kb.AddRow(tg.NewBtn(i18n.Tr("Search"), searchCMD))
	}
	dirBtnsByRows := slice.Chunk(dirBtns, btnsPerRow)
	for _, row := range dirBtnsByRows {
		kb.AddRow(row)
	}

	if skippedBtns {
		kb.AddRow(tg.NewBtn(i18n.Tr("More..."), tg.NewCmd(consts.CmdShowMoveToDirOrFile, []string{filenameHash, "full"})))
	}

	b.db.SetInputExpectation(tg.NewCmd(consts.CmdMoveToNewFile, []string{filenameHash, "%s"}))

	err = b.showHTML("📄 Select a file or enter a new name:", kb)
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}

	return nil
}

func (b *Bot) showToChecklist(params []string) error {
	filenameHash := params[0]

	kb, err := b.toChecklistKeyboard(filenameHash)
	if err != nil {
		return fmt.Errorf("show to checklist: can't get keyboard: %w", err)
	}

	b.db.SetInputExpectation(tg.NewCmd(consts.CmdMoveToNewChecklist, []string{filenameHash, "%s"}))

	err = b.showHTML(i18n.Tr("Choose a checklist or name a new one"), kb)
	if err != nil {
		return fmt.Errorf("show to checklist: %w", err)
	}

	return nil
}

func (b *Bot) moveToFileBtns(newFilenameShortHash string) ([]tg.Btn, error) {
	files, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return nil, fmt.Errorf("to doc keyboard: %w", err)
	}
	files = fs.OnlyMDFiles(files)
	files = fs.SortByCtimeDesc(files)
	if len(files) == 0 {
		return nil, nil
	}

	var buttons []tg.Btn
	newBtn := func(title, existingFilenameHash string) tg.Btn {
		title = fmt.Sprintf("%s %s", i18n.Emoji("file"), title)
		params := []string{existingFilenameHash, fs.DirRoot, newFilenameShortHash}
		return tg.NewBtn(title, tg.NewCmd(consts.CmdMoveToExistingFile, params))
	}
	for _, file := range files {
		buttons = append(buttons, newBtn(file.Title, fs.ShortHash(file.Name)))
	}

	return buttons, nil
}

func (b *Bot) moveToDirBtns(filenameHash string) ([]tg.Btn, error) {
	newBtn := func(dir string) tg.Btn {
		emojifiedDir := fmt.Sprintf("%s %s", i18n.Emoji("dir"), txt.Ucfirst(dir))
		return tg.NewBtn(emojifiedDir, tg.NewCmd(consts.CmdMoveToExistingDir, []string{fs.ShortHash(dir), fs.DirRoot, filenameHash}))
	}

	dirs, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return nil, fmt.Errorf("to note keyboard: %w", err)
	}
	dirs = fs.OnlyNoteDirs(fs.OnlyDirs(dirs))
	dirs = fs.SortByCtimeDesc(dirs)

	var buttons []tg.Btn
	for _, dir := range dirs {
		buttons = append(buttons, newBtn(dir.Name))
	}

	return buttons, nil
}

func (b *Bot) toChecklistKeyboard(filenameHash string) (*tg.Keyboard, error) {
	newBtn := func(dir, title string) tg.Btn {
		return tg.NewBtn(title, tg.NewCmd(consts.CmdMoveToChecklist, []string{filenameHash, dir}))
	}

	dirs, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return nil, fmt.Errorf("to checklist keyboard: %w", err)
	}
	// TODO handle case with zero dirs (inline_keyboard is null), for all similar cases
	dirs = fs.OnlyChecklists(fs.OnlyDirs(dirs))

	kb := tg.NewKeyboard(nil)
	for _, dir := range dirs {
		kb.AddRow(newBtn(dir.Name, dir.Title))
	}

	return kb, nil
}

func (b *Bot) togglePomodoro(_ []string) error {
	// Check if Pomodoro is already running
	hasPomodoroInToday, err := b.fs.Exists(fs.DirToday, fs.FilePomodoro)
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to check if pomodoro is already running %w", err)
	}
	hasPomodoroInTrash, err := b.fs.Exists(fs.DirArchive, fs.FilePomodoro)
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to check if pomodoro is already running %w", err)
	}

	if hasPomodoroInToday {
		err = b.fs.Del(fs.DirToday, fs.FilePomodoro)
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to delete pomodoro file: %w", err)
		}
	}
	if hasPomodoroInTrash {
		err = b.fs.Del(fs.DirArchive, fs.FilePomodoro)
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to delete pomodoro file: %w", err)
		}
	}

	if hasPomodoroInToday || hasPomodoroInTrash {
		// Just an informative messages
		_, _ = b.tg.Send(b.userID, "Pomodoro is stopped", nil, tg.MarkupHTML)
		return b.ShowToday(nil)
	}

	// Create Pomodoro task
	err = b.fs.Touch(fs.DirToday, fs.FilePomodoro)
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
	}

	_, err = b.tg.Send(b.userID, i18n.PomodoroStarted, nil, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) showToADayRecurring(params []string) error {
	filenameHash := params[0]

	newBtn := func(name, cron string) tg.Btn {
		// We need to shorten filehash, otherwise whole payload doesn't fit telegram's restrictions (64 bytes)
		cmd := tg.NewCmd(consts.CmdSchedule, []string{txt.Substr(filenameHash, 0, 4), txt.I64(sched.NextExcludeToday(cron)), cron})
		return tg.NewBtn(name, cmd)
	}

	kb := tg.NewKeyboard([]tg.Row{
		// Cron format: Minute Hour DayOfMonth Month DayOfWeek
		tg.NewRow(
			newBtn(i18n.StrWeekdays, "0 0 * * 1-5"),
			newBtn(i18n.StrEveryday, "0 0 * * *"),
		),
		tg.NewRow(
			newBtn(i18n.StrMonday, "0 0 * * 1"),
			newBtn(i18n.StrTuesday, "0 0 * * 2"),
			newBtn(i18n.StrWednesday, "0 0 * * 3"),
			newBtn(i18n.StrThursday, "0 0 * * 4"),
		),
		tg.NewRow(
			newBtn(i18n.StrFriday, "0 0 * * 5"),
			newBtn(i18n.StrSaturday, "0 0 * * 6"),
			newBtn(i18n.StrSunday, "0 0 * * 0"),
		),
	})

	for week := 0; week < 4; week++ {
		row := tg.NewRow()
		for day := 1; day < 8; day++ {
			i := week*7 + day
			cron := fmt.Sprintf("0 0 %d * *", i)
			row = append(row, newBtn(txt.I64(int64(i)), cron))
		}
		kb.AddRow(row)
	}
	kb.AddRow(tg.NewBtn(i18n.StrToToday, tg.NewCmd(consts.CmdShowToday, nil)))

	err := b.showHTML(i18n.Tr("Repeat the task"), kb)
	if err != nil {
		return fmt.Errorf("showRecuringKeyboard : %w", err)
	}

	return nil
}

func (b *Bot) addToFile(dir, filename, content string) error {
	existingContent, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("add to file: can't get doc content of '%s': %w", filename, err)
	}

	header := fmt.Sprintf("#### %d %s, %s", now().Day(), now().Format("January"), now().Weekday())
	newContent := txt.InsertTextAfterHeader(existingContent, header, content)

	err = b.fs.Write(dir, filename, newContent)
	if err != nil {
		return fmt.Errorf("add to file: can't save file: %w", err)
	}

	return nil
}

// TODO release add help
func (b *Bot) showHelp(_ []string) error {
	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil))})

	return b.showHTML("Not yet implemented 🏗!", kb)
}

// TODO
func (b *Bot) download(_ []string) error {
	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil))})

	return b.showHTML("Not yet implemented 🏗!", kb)
}

func (b *Bot) tasksOnlyMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeTasks)
	if err != nil {
		return fmt.Errorf("tasks only mode: can't set notes only mode %w", err)
	}

	cmds, err := b.cfg.MoveToCmds()
	if err != nil {
		return fmt.Errorf("tasks only mode: can't get quick commands %w", err)
	}

	for _, cmd := range cmds {
		err = b.cfg.DelMoveToCmd(cmd)
		if err != nil {
			return fmt.Errorf("tasks only mode: can't delete quick command %w", err)
		}
	}

	moveToCmds := []string{
		consts.CmdScheduleForTmrw,
		consts.CmdMoveToLater,
		consts.CmdShowScheduleForDay,
	}
	for _, cmd := range moveToCmds {
		err = b.cfg.AddMoveToCmd(cmd)
		if err != nil {
			return fmt.Errorf("full mode: can't add quick command %w", err)
		}
	}

	return b.ShowToday(nil)
}

func (b *Bot) notesOnlyMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeNotes)
	if err != nil {
		return fmt.Errorf("notes only mode: can't set notes only mode %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) journalOnlyMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeJournal)
	if err != nil {
		return fmt.Errorf("journal only mode: can't set notes only mode %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) fullMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeFull)
	if err != nil {
		return fmt.Errorf("full mode: can't set notes only mode %w", err)
	}

	moveToCmds := []string{
		consts.CmdScheduleForTmrw,
		consts.CmdMoveToLater,
		consts.CmdShowScheduleForDay,
		consts.CmdShowMoveToDirOrFile,
		consts.CmdMoveToRead,
		consts.CmdMoveToShop,
		consts.CmdMoveToWatch,
		consts.CmdMoveToJournal,
	}
	for _, cmd := range moveToCmds {
		err = b.cfg.AddMoveToCmd(cmd)
		if err != nil {
			return fmt.Errorf("full mode: can't add quick command %w", err)
		}
	}

	return b.ShowToday(nil)
}

func (b *Bot) completeHabit(params []string) error {
	habit := params[0]
	userHabits, err := habits.Habits(b.fs, time.Now().Year())
	if err != nil {
		return fmt.Errorf("complete habit: can't get habits: %w", err)
	}

	userHabits[habit][time.Now().YearDay()] = 1

	err = habits.Write(b.fs, time.Now().Year(), userHabits)
	if err != nil {
		return fmt.Errorf("complete habit: can't write habits: %w", err)
	}

	emoji := habits.Emoji(b.fs, habit)

	userConf := userconfig.NewConfig(b.fs, b.userID, config.BotCfg.ConfigFilename)
	err = journal.AddEmoji(b.fs, emoji, userConf.Timezone())
	if err != nil {
		return fmt.Errorf("complete habit: can't write emoji to journal: %w", err)
	}

	record := fmt.Sprintf("%s %s", emoji, habit)
	err = journal.AddRecord(b.fs, record, userConf.Timezone())
	if err != nil {
		return fmt.Errorf("complete habit: can't write record to journal: %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) shareNote(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("share note: can't find dir: %w", err)
	}

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("share note: can't find file: %w", err)
	}

	content, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("share note: %w", err)
	}

	for _, channel := range b.cfg.Channels() {
		probablyInvalidMD := fmt.Sprintf("**%s/%s**\n\n%s", fs.Title(dir), fs.Title(filename), content)
		probablyInvalidMD, images, _ := txt.ExtractTextImgsLinks(probablyInvalidMD)
		// Sending a gallery of images if there are any
		if len(images) > 0 {
			// We tolerate errors with the image gallery for now, text is more important
			mids, imgErr := b.tg.SendImages(channel, images)
			if imgErr == nil {
				for _, imgMid := range mids {
					b.db.AddImgMsgID(imgMid)
				}
			} else {
				slog.Error("Can't send images", "error", imgErr)
			}
		}

		// If our msg is too long, we send maxMsgsToSendAtOnce first messages.
		// Keyboard is attached to the last one
		textChunks := txt.SplitTextIntoChunks(probablyInvalidMD, maxMsgLength)
		textChunks = textChunks[0:min(maxMsgsToSendAtOnce, len(textChunks))]
		lastChunk := textChunks[len(textChunks)-1]
		textChunks = textChunks[0 : len(textChunks)-1]
		for _, textChunk := range textChunks {
			_, _ = b.tg.Send(b.userID, txt.MarkdownToHTML(textChunk), nil, tg.MarkupHTML)
		}

		_, err := b.tg.Send(channel, txt.MarkdownToHTML(lastChunk), nil, tg.MarkupHTML)
		if err != nil {
			return fmt.Errorf("share: %w", err)
		}
	}

	return nil
}

func extractMarkdown(u Update) string {
	content := txt.TelegramEntitiesToMarkdown(u.MsgText(), u.MsgEntities())
	content = strings.TrimSpace(txt.NormNewLines(content))

	return txt.Ucfirst(content)
}

func checklistTitle(checklist string) string {
	// Once we move our items from checklists to archive,
	// they got named like -checklist-itemName
	stripChecklistChars := regexp.MustCompile(`^_.*?_(.+)`)
	title := stripChecklistChars.ReplaceAllString(checklist, "$1")
	title = strings.TrimPrefix(strings.TrimSuffix(title, "_"), "_")

	return fs.Title(title)
}

func angerEmoji(file fs.File) string {
	anger := []string{"", "🙄", "😕", "😢", "😭", "🤬️"}

	timeDiff := now().Unix() - file.Ctime
	timeDiff = max(0, timeDiff)
	daysDiff := (int)(timeDiff / (24 * 60 * 60))
	daysDiff = min(len(anger)-1, daysDiff)

	return anger[daysDiff]
}

func completedMsg() string {
	msgs := []string{
		"Completed! 🚀",
		"Done! 🎉",
		"Awesome! 💪",
		"Great job! 🌟",
		"Good work! 🎈",
		"Nice! 🎊",
		"Fantastic! 🎇",
		"Excellent! 🎯",
		"Perfect! 🏆",
		"Bravo! 👏",
		"Superb! 🌠",
		"You did it! ✅",
		"Nicely done! 🎖",
		"Nailed it! 🎯",
	}

	return msgs[rand.Intn(len(msgs))]
}
