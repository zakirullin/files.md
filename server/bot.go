// Bot's main functionality. We accept messages from the user,
// we ask user where to save the messages. We save messages
// to plain markdown files locally.

package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/exp/slog"

	"github.com/zakirullin/files.md/server/config"
	"github.com/zakirullin/files.md/server/fs"
	"github.com/zakirullin/files.md/server/habits"
	"github.com/zakirullin/files.md/server/i18n"
	"github.com/zakirullin/files.md/server/journal"
	"github.com/zakirullin/files.md/server/pkg/slice"
	"github.com/zakirullin/files.md/server/pkg/tg"
	"github.com/zakirullin/files.md/server/pkg/txt"
	"github.com/zakirullin/files.md/server/plugins"
	"github.com/zakirullin/files.md/server/stats"
	"github.com/zakirullin/files.md/server/sync"
	"github.com/zakirullin/files.md/server/userconfig"
)

var (
	errUnknownCommand           = errors.New("unknown command")
	errInvalidRequestFromInline = errors.New("invalid request from inline query")
	errInvalidInlineQuery       = errors.New("invalid inline query")
	BotPlugins                  = []BotPlugin{plugins.NewWorldClockPlugin()}
)

const (
	btnsPerRow               = 3
	quickBtnsPerRow          = 4
	maxBtns                  = 50
	maxBtnsInChecklist       = 10 // For _read_ and _watch_ checklists, so we're less likely to be overwhelmed :)
	maxGroupedBtnsInMoveTo   = 6
	maxInlineResults         = 20
	maxMsgLength             = 4096 // In UTF-8 characters (runes), skin-tone emojis count as 2
	maxMsgsToSendAtOnce      = 5    // For lengthy messages
	maxHeaderLength          = 100
	maxHeaderLengthForMobile = 33 // Fits regular mobile screen
	inlineResultsCacheTime   = 15 // Seconds

	// On mobile phones buttons shrink to the message width, and sometimes it's too narrow, so we make the message wider
	wideSpacer = "<code>            ⁠</code>"
)

// Update represents incoming user updates.
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
	ChannelID() (int64, bool)
	ChannelName() (string, bool)
}

// Chat provides a simple interface to chat API like Telegram.
type Chat interface {
	Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error)
	SendImages(userID int64, images []string) ([]int, error)
	SendReaction(userID int64, msgID int, reaction string) error
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
	SetRecentFilenameByMsgID(msgID int, filename string)
	SetRecentDirByMsgID(msgID int, filename string)
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

var now = time.Now

// Telegram only allows 64 bytes in callback_data,
// So we have to be really short :)
const (
	CmdShowStart                       = "start"
	CmdDoNothing                       = "nothing"
	CmdShowLater                       = "later"
	CmdShowToday                       = "today"
	CmdShowFiles                       = "files"
	CmdShowDirs                        = "dirs"
	CmdShowPostpone                    = "postpone"
	CmdShowMoveFromToday               = "move"
	CmdShowMoveTo                      = "s_move"
	CmdShowRename                      = "rename"
	CmdShowRenameFile                  = "rename_file"
	CmdShowChecklists                  = "checklists"
	CmdShowStats                       = "stats"
	CmdOpenInApp                       = "app"
	CmdShowHelp                        = "help"
	CmdComplete                        = "c"
	CmdPostpone                        = "post"
	CmdShowLongItem                    = "item"
	CmdShowLongItemFromToday           = "item_t"
	CmdShowFile                        = "file"
	CmdShowChecklist                   = "checklist"
	CmdShowChecklistItem               = "check_show"
	CmdCompleteListItem                = "check_comp"
	CmdShowMoveToDirOrFile             = "to_file"
	CmdShowMoveToChecklist             = "to_checklist"
	CmdRename                          = "ren"
	CmdMoveToExistingDir               = "mv"
	CmdMoveToChecklist                 = "add_item"
	CmdCompleteChecklistItem           = "check_item"
	CmdMoveToExistingDirFromToday      = "mv_t"
	CmdRequestNewDir                   = "new_dir"
	CmdMoveToNewDir                    = "mv_to_new_dir"
	CmdMoveToExistingFile              = "mf"
	CmdMoveToExistingNote              = "mvn"
	CmdMoveToNewFile                   = "mn"
	CmdMoveToDirChecklist              = "mv_to_chk"
	CmdMoveToRead                      = "mv_to_read"
	CmdMoveToWatch                     = "mv_to_watch"
	CmdMoveToShop                      = "mv_to_shop"
	CmdMoveToNewChecklist              = "mv_to_new_chk"
	CmdMoveToJournal                   = "mv_to_journal"
	CmdMoveToLater                     = "mv_later"
	CmdShowScheduleForDay              = "sc_day"
	CmdSchedule                        = "sc"
	CmdScheduleForTmrw                 = "sc_tmrw"
	CmdPomodoro                        = "pomodoro"
	CmdShowScheduleForDayRecurring     = "sc_day_r"
	CmdLater                           = "later"
	CmdShowSettings                    = "settings"
	CmdShowQuickBtnsSettings           = "c_quick_btns"
	CmdShowMoveToBtnsSettings          = "c_move_btns"
	CmdAddToQuickBtns                  = "add_quick"
	CmdDelFromQuickBtns                = "del_quick"
	CmdAddToMoveToBtns                 = "add_move"
	CmdDelFromMoveToBtns               = "del_move"
	CmdShowTimezone                    = "timezone"
	CmdSetTimezone                     = "set_timezone"
	CmdShowReadChecklist               = "read"
	CmdShowWatchChecklist              = "watch"
	CmdShowShopChecklist               = "shop"
	CmdShowSchedule                    = "schedule"
	CmdDownload                        = "download"
	CmdTasksOnlyMode                   = "tasks_only"
	CmdNotesOnlyMode                   = "notes_only"
	CmdJournalOnlyMode                 = "journal_only"
	CmdFullMode                        = "full"
	CmdChatMode                        = "chat"
	CmdInlineQuerySearchEveryWhere     = "search"
	CmdInlineQuerySearchInDir          = "search_dir"
	CmdWebAppHabits                    = "habits"
	CmdRandomNote                      = "random_note"
	CmdAddToJournalShortcut            = "j"
	CmdAddToJournalAndContinueShortcut = "ja"
	CmdAddToRecentFileShortcut         = "+"
	CmdCompleteHabit                   = "ch"
	CmdShare                           = "share"
)

var Shortcuts = map[string][]string{
	CmdAddToJournalShortcut:            {"/ж", "jj", "жж"},
	CmdAddToJournalAndContinueShortcut: {"жд", "jd", "ja"},
	CmdAddToRecentFileShortcut:         {"++"},
}

// Bot has all the things that we need to handle a message or command from a user.
// We use tg chat to talk with the user.
// We use fs to save artefacts to the disk (.md files).
// We use db to save temporal things like recent command.
// We use cfg to configure bot behaviour (config.json).
type Bot struct {
	userID int64
	tg     Chat
	fs     *fs.FS
	db     Database
	cfg    *userconfig.Config
}

func NewBot(userID int64, tg Chat, fs *fs.FS, db Database, cfg *userconfig.Config) *Bot {
	return &Bot{userID, tg, fs, db, cfg}
}

// Reply to incoming text message, command or inline query
func (b *Bot) Reply(u Update) error {
	// Handle inline queries.
	if _, ok := u.InlineQueryID(); ok {
		return b.answerSearch(u)
	}

	// Handle messages from channels.
	_, isChannel := u.ChannelID()
	if isChannel {
		channelName, _ := u.ChannelName()
		if len(strings.TrimSpace(channelName)) == 0 {
			channelName = "UnknownChannel"
		}

		return b.addToFile(fs.DirUserRoot, fs.Filename(channelName), u.MsgText())
	}

	// Handle plugins.
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
			if cmd.Name == CmdCompleteHabit || cmd.Name == CmdComplete {
				_ = b.tg.AnswerCallbackQuery(callbackQueryID, completedMsg())
			} else if cmd.Name == CmdShare {
				_ = b.tg.AnswerCallbackQuery(callbackQueryID, "Shared 💚!")
			} else {
				_ = b.tg.AnswerCallbackQuery(callbackQueryID, "")
			}
		}

		return nil
	}

	// Handle images.
	if _, hasImage := u.PhotoOrImageID(); hasImage {
		err = b.saveFromImage(u)
	} else {
		err = b.saveFromTextMsg(u)
	}

	if errors.Is(err, fs.ErrQuotaExceeded) {
		b.tg.Send(b.userID, "Storage quota exceeded. Please delete some files.", nil, tg.MarkupHTML)
		return nil
	}

	return err
}

// Commands and their handlers.
// Every handler accepts []string params
func (b *Bot) handlers() map[string]func([]string) error {
	handlers := map[string]func([]string) error{
		// Direct user commands
		CmdShowToday:          b.ShowToday,
		CmdShowStart:          b.showStart,
		CmdShowLater:          b.showLaterTasks,
		CmdShowFiles:          b.showFiles,
		CmdShowDirs:           b.showDirs,
		CmdShowChecklists:     b.showChecklists,
		CmdShowPostpone:       b.showPostpone,
		CmdShowMoveTo:         b.showMoveTo,
		CmdShowRename:         b.showRename,
		CmdShowStats:          b.showStats,
		CmdShowReadChecklist:  b.showRead,
		CmdRandomNote:         b.randomNote,
		CmdShowWatchChecklist: b.showWatch,
		CmdShowShopChecklist:  b.showShop,
		CmdShowSchedule:       b.showSchedule,
		CmdShowMoveFromToday:  b.showMoveFromToday,
		CmdShowSettings:       b.showSettings,
		CmdShowTimezone:       b.showTimezone,
		CmdSetTimezone:        b.setTimezone,
		CmdOpenInApp:          b.openInApp,
		CmdShowHelp:           b.showHelp,
		CmdDownload:           b.download,
		// Button's commands (callbacks)
		CmdShowRenameFile:                  b.showRenameFile,
		CmdShowLongItem:                    b.showLongItem,
		CmdShowLongItemFromToday:           b.showLongItemFromInbox,
		CmdShowFile:                        b.showFile,
		CmdShowChecklist:                   b.showChecklist,
		CmdCompleteListItem:                b.completeListItem,
		CmdShowChecklistItem:               b.showChecklistItem,
		CmdShowScheduleForDay:              b.showToADay,
		CmdShowMoveToDirOrFile:             b.showMoveToFileOrDir,
		CmdShowMoveToChecklist:             b.showToChecklist,
		CmdMoveToExistingDir:               b.moveToDir,
		CmdMoveToChecklist:                 b.moveToChecklist,
		CmdCompleteChecklistItem:           b.completeChecklistItem,
		CmdMoveToExistingDirFromToday:      b.moveToDirFromToday,
		CmdRequestNewDir:                   b.requestNewDirName,
		CmdMoveToNewDir:                    b.moveToNewDir,
		CmdMoveToExistingFile:              b.moveToExistingFile,
		CmdMoveToExistingNote:              b.moveToExistingNote,
		CmdMoveToNewFile:                   b.moveToNewFile,
		CmdMoveToDirChecklist:              b.moveToDirChecklist,
		CmdMoveToRead:                      b.moveToRead,
		CmdMoveToWatch:                     b.moveToWatch,
		CmdMoveToShop:                      b.moveToShop,
		CmdMoveToNewChecklist:              b.moveToNewChecklist,
		CmdMoveToJournal:                   b.moveToJournal,
		CmdMoveToLater:                     b.moveToLater,
		CmdSchedule:                        b.schedule,
		CmdScheduleForTmrw:                 b.scheduleForTmrw,
		CmdComplete:                        b.complete,
		CmdPostpone:                        b.postpone,
		CmdPomodoro:                        b.togglePomodoro,
		CmdShowScheduleForDayRecurring:     b.showToADayRecurring,
		CmdShowQuickBtnsSettings:           b.showQuickBtnsSettings,
		CmdShowMoveToBtnsSettings:          b.showMoveToBtnsSettings,
		CmdAddToQuickBtns:                  b.addToQuickBtns,
		CmdDelFromQuickBtns:                b.delFromQuickBtns,
		CmdAddToMoveToBtns:                 b.addToMoveToBtns,
		CmdDelFromMoveToBtns:               b.delFromMoveToBtns,
		CmdAddToJournalShortcut:            b.addToJournalFromShortcut,
		CmdAddToJournalAndContinueShortcut: b.addToJournalAndContinue,
		CmdAddToRecentFileShortcut:         b.addToRecentFileOrNoteFromShortcut,
		CmdRename:                          b.rename,
		CmdTasksOnlyMode:                   b.setTasksOnlyMode,
		CmdNotesOnlyMode:                   b.setNotesOnlyMode,
		CmdJournalOnlyMode:                 b.setJournalOnlyMode,
		CmdFullMode:                        b.setFullMode,
		CmdChatMode:                        b.setChatOnlyMode,
		CmdCompleteHabit:                   b.completeHabit,
		CmdShare:                           b.shareNote,
		// Used for button-like separators
		CmdDoNothing: func(s []string) error { return nil },
	}

	for cmd, shortcuts := range Shortcuts {
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
			_, _ = b.tg.Send(b.userID, i18n.Tr("I know nothing about this command 😕"), nil, tg.MarkupHTML)
			return nil, fmt.Errorf("unknown command: %s", cmd.Name)
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

	for canonicalCMD, shortcuts := range Shortcuts {
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

func (b *Bot) saveFromTextMsg(u Update) error {
	msg := extractMarkdown(u)
	if len(msg) == 0 {
		return fmt.Errorf("save: empty message")
	}

	// Collapse a few consecutive messages into one, see bot_forwards.go
	msgTime, updateHasTime := u.Time()
	if updateHasTime {
		_, shouldCollapse := collapseToMsg(b.userID, msgTime)
		if shouldCollapse {
			// We just write at the end of our append-only chat file,
			// that would concat the current message with the previous one.
			err := b.createOrAdd(fs.DirUserRoot, fs.TodayFilename, msg)
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

	msgHash, err := b.appendToToday(msg, b.cfg.Timezone())
	if err != nil {
		return fmt.Errorf("save to chat: %w", err)
	}

	if b.cfg.ChatOnlyMode() {
		msgID, _ := u.MsgID()
		_ = b.tg.SendReaction(b.userID, msgID, "👌")
		return nil
	}

	if updateHasTime {
		setFirstMsgHash(b.userID, msgHash, msgTime)
		setFirstMsgTime(b.userID, msgTime)
	}

	if b.cfg.JournalOnlyMode() {
		return b.moveToJournal([]string{msgHash})
	}

	return b.showMoveTo([]string{msgHash})
}

// TODO test collapsing from both regular messages and images
func (b *Bot) saveFromImage(u Update) error {
	content, err := b.saveImage(u)
	if err != nil {
		return fmt.Errorf("save from image: %w", err)
	}

	// Collapse a few consecutive messages into one, see bot_forwards.go
	msgTime, updateHasTime := u.Time()
	if updateHasTime {
		_, shouldCollapse := collapseToMsg(b.userID, msgTime)
		if shouldCollapse {
			err := b.createOrAdd(fs.DirUserRoot, fs.TodayFilename, content)
			if err != nil {
				return fmt.Errorf("save collapsed: %w", err)
			}
			return nil
		}
	}

	// Adding to an existing file
	if replyMsgID, ok := u.ReplyToMsgID(); ok {
		return b.addToRepliedFile(replyMsgID, content)
	}

	msgHash, err := b.appendToToday(content, b.cfg.Timezone())
	if err != nil {
		return fmt.Errorf("save from image: %w", err)
	}

	if b.cfg.ChatOnlyMode() {
		msgID, _ := u.MsgID()
		// We can tolerate missing reaction.
		_ = b.tg.SendReaction(b.userID, msgID, "👌")
		return nil
	}

	// Track forwards.
	if updateHasTime {
		setFirstMsgHash(b.userID, msgHash, msgTime)
		setFirstMsgTime(b.userID, msgTime)
	}

	if b.cfg.JournalOnlyMode() {
		return b.moveToJournal([]string{msgHash})
	}

	return b.showMoveTo([]string{msgHash})
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
	err = b.fs.Write(fs.DirMedia, imgFilename, buf.String())
	if err != nil {
		return "", fmt.Errorf("can't save image: %w", err)
	}

	// TODO remove center
	imgPath := fmt.Sprintf("%s/%s", fs.DirMedia, imgFilename)
	content := fmt.Sprintf("![](%s)", imgPath)
	// If there's caption, place it under the image
	if u.Caption() != "" {
		caption := txt.TelegramEntitiesToMarkdown(u.Caption(), u.CaptionEntities())
		caption = strings.TrimSpace(txt.NormNewLines(caption))
		content = fmt.Sprintf("%s\n%s", content, txt.Ucfirst(caption))
	}

	return content, nil
}

// TODO add chat.md support
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
	content := txt.AddHeaderAndText(existingContent, header, newContent)
	err = b.fs.Write(dir, existingFilename, content)
	if err != nil {
		return fmt.Errorf("add: can't write: %w", err)
	}

	b.delAllKeyboards()

	b.db.SetRecentCommand(CmdMoveToExistingFile)
	b.db.SetRecentCommandParams([]string{fs.ShortHash(existingFilename)})

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
		if note.ParentDir == fs.DirUserRoot {
			path = note.Name
		}
		article := tgbotapi.NewInlineQueryResultArticleHTML(strconv.Itoa(id), note.DisplayName, path)
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
		dir = fs.DirUserRoot
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
		msgHash := c.Params[0]

		err := b.moveFromToday(func(content string, timestamp time.Time) error {
			if dir == fs.DirUserRoot {
				// We have a file
				b.db.SetRecentCommand(CmdMoveToExistingFile)
				b.db.SetRecentCommandParams([]string{fs.ShortHash(filename)})
			} else {
				// We have a note (a file placed in a subdirectory)
				b.db.SetRecentCommand(CmdMoveToExistingNote)
				b.db.SetRecentCommandParams([]string{fs.ShortHash(filename), fs.ShortHash(dir)})
			}

			err := b.addToFile(dir, filename, content)
			if err != nil {
				return fmt.Errorf("inline query: can't add to file %s: %w", filename, err)
			}

			return nil
		}, false, msgHash)
		if err != nil {
			return fmt.Errorf("inline query: can't move from chat: %w", err)
		}

		// Just an informative message
		_, _ = b.tg.Send(b.userID, fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.DisplayName(filename)), nil, tg.MarkupHTML)

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

	if err := b.fs.Write(dir, filename, content); err != nil {
		return fmt.Errorf("create: %w", err)
	}

	return nil
}

func (b *Bot) extractHeaderAndBody(msg string, maxHeaderLen int) (string, string, error) {
	if len(msg) == 0 {
		return "", "", fmt.Errorf("extract title: empty msg")
	}

	parts := strings.Split(msg, "\n")
	title := txt.Ucfirst(strings.TrimSpace(parts[0]))
	if txt.HasImage(title) {
		if len(parts) > 1 {
			title = txt.Ucfirst(strings.TrimSpace(parts[1]))
		}

		if title == "" || len(parts) == 1 {
			title = fmt.Sprintf("Img %s", now().Format("02.01.06 15:04"))
		}
	}

	if utf8.RuneCountInString(title) > maxHeaderLen {
		title = txt.Substr(title, 0, maxHeaderLen) + "..."
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

	title := fs.DisplayName(filename)
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
		dir := fs.DirUserRoot
		link = strings.TrimSpace(link)
		parts := strings.SplitN(link, "/", 2)
		if len(parts) == 2 {
			dir = parts[0]
			link = parts[1]
		}

		cmd := tg.NewCmd(CmdShowFile, []string{fs.Hash(dir), fs.Hash(link)})
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
	msgHash := params[0]

	if b.cfg.NotesOnlyMode() {
		b.delAllKeyboards()

		return b.showMoveToFileOrDir([]string{msgHash})
	}

	var kb tg.Keyboard
	userMoveToBtns := b.moveToBtns(msgHash)
	if len(userMoveToBtns) == 0 {
		b.delAllKeyboards()

		return b.ShowToday(nil)
	}

	// Add recent command if any
	recentBtn := b.recentCmdBtn(msgHash)
	if recentBtn != nil {
		userMoveToBtns = append(userMoveToBtns, *recentBtn)
	}

	// This command is "do nothing and leave an item in the inbox"
	if !b.cfg.TasksOnlyMode() {
		showTodayCmd := tg.NewCmd(CmdShowToday, []string{})
		showTodayLabel := "👌"
		userMoveToBtns = append(userMoveToBtns, tg.NewBtn(showTodayLabel, showTodayCmd))
	}

	userBtnsByRows := slice.Chunk(userMoveToBtns, btnsPerRow)
	for _, row := range userBtnsByRows {
		kb.AddRow(row)
	}

	b.delAllKeyboards()

	msg := b.tr("Saved to <b>today</b>!")
	if err := b.showHTML(msg, &kb); err != nil {
		return fmt.Errorf("move: %w", err)
	}

	return nil
}

func (b *Bot) recentCmdBtn(msgHash string) *tg.Btn {
	recentCmd, ok := b.db.RecentCommand()
	if !ok {
		return nil
	}

	args, _ := b.db.RecentCommandParams()
	args = append(args, msgHash)
	targetFilenameHash := args[0]

	var unhashedTarget string
	icon := "⭐️"
	if recentCmd == CmdMoveToExistingFile {
		var err error
		unhashedTarget, err = b.fs.Unhash(fs.DirUserRoot, targetFilenameHash)
		if err != nil {
			return nil
		}
	} else if recentCmd == CmdMoveToExistingNote {
		dir, err := b.fs.Unhash(fs.DirUserRoot, args[1])
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

	name := fmt.Sprintf("%s %s", icon, fs.DisplayName(unhashedTarget))
	btn := tg.NewBtn(name, tg.NewCmd(recentCmd, args))
	return &btn
}

func (b *Bot) ShowToday(_ []string) error {
	if b.cfg.NotesOnlyMode() {
		return b.showDirs(nil)
	}

	if b.cfg.JournalOnlyMode() || b.cfg.ChatOnlyMode() {
		_, err := b.tg.Send(b.userID, i18n.Tr("What's on your mind?"), nil, tg.MarkupHTML)
		if err != nil {
			return fmt.Errorf("show today: can't send journal message: %w", err)
		}
		return nil
	}

	var kb tg.Keyboard

	// Adding records from inbox
	content, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("show today: can't read chat file: %w", err)
	}
	blocks := readBlocks(content)
	// Inbox entry: `- [ ] body` or `- [ ] `HH:MM` body` (timestamp optional).
	// Capture group 1 holds the checkbox marker.
	inboxEntryRegex := regexp.MustCompile(`^- \[([ xX])\] (?:` + "`" + `\d{2}:\d{2}` + "` )?")
	shownCount := 0
	for _, block := range blocks {
		m := inboxEntryRegex.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		// Skip already-completed entries from the visible list. The hash is
		// the same for `[ ]` / `[x]` variants, so tapping the button after a
		// completion toggle still resolves to the right line.
		if m[1] == "x" || m[1] == "X" {
			continue
		}
		shownCount++

		msgHash := todayBlockHash(block)

		// Strip the matched prefix (optional checkbox + optional timestamp).
		block = strings.TrimSpace(block[len(m[0]):])

		// Skip image link if any.
		parts := strings.Split(block, "\n")
		title := txt.Ucfirst(strings.TrimSpace(parts[0]))
		if txt.HasImage(title) {
			if len(parts) > 1 {
				title = txt.Ucfirst(strings.TrimSpace(parts[1]))
			}

			if title == "" || len(parts) == 1 {
				title = fmt.Sprintf("Img %s", now().Format("02.01.06 15:04"))
			}
		}

		if len([]rune(title)) >= maxHeaderLengthForMobile || txt.HasImage(block) {
			cmd := tg.NewCmd(CmdShowLongItemFromToday, []string{msgHash})
			btn := tg.NewBtn(txt.Emoji(i18n.Emoji("eyes"), title), cmd)
			kb.AddRow(btn)
		} else {
			cmd := tg.NewCmd(CmdComplete, []string{msgHash})
			btn := tg.NewBtn(txt.Emoji(i18n.Emoji(title), title), cmd)
			kb.AddRow(btn)
		}
	}

	// Adding habits
	habitsRow := tg.NewRow()
	userHabits := make(map[string]habits.Year)
	if b.cfg.QuickHabitsEnabled() {
		// We can tolerate missing habits
		userHabits, _ = habits.LastWeekHabits(b.fs, b.cfg.Timezone())
		_, ok := userHabits[habits.MoodHabit]
		if ok {
			delete(userHabits, habits.MoodHabit)
		}
	}
	for habit, year := range userHabits {
		if completed, _ := year[time.Now().YearDay()]; completed == 1 {
			continue
		}

		cmd := tg.NewCmd(CmdCompleteHabit, []string{habit})
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

	msg := b.todayLabel(shownCount)
	err = b.showHTML(msg, &kb)
	if err != nil {
		return fmt.Errorf("show list: %w", err)
	}

	return nil
}

func (b *Bot) showLaterTasks(_ []string) error {
	var kb tg.Keyboard

	// Adding tasks from Later.md
	laterChecklistMD, err := b.fs.Read(fs.DirUserRoot, fs.LaterFilename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("show later: can't read later file: %w", err)
	}
	if len(laterChecklistMD) != 0 {
		tasks := txt.IncompleteChecklistItems(laterChecklistMD)
		for _, task := range tasks {
			cmd := tg.NewCmd(CmdCompleteChecklistItem, []string{fs.Hash(fs.LaterFilename), fs.Hash(task)})
			btn := tg.NewBtn(i18n.AddEmoji(task), cmd)
			kb.AddRow(btn)
		}
	}
	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil)))

	msg := b.tr("⏳ Your tasks for <b>later</b>:")
	err = b.showHTML(msg, &kb)
	if err != nil {
		return fmt.Errorf("show list: %w", err)
	}

	return nil
}

// TODO improve a bit
// msgsCount - how many messages (inbox items) were shown to a user
func (b *Bot) todayLabel(msgsCount ...int) string {
	var statusBar string

	hasPomodoroInToday := false
	todayMD, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err == nil {
		_, completed := txt.ChecklistItems(todayMD)
		checked, exists := completed[fs.PomodoroTask]
		hasPomodoroInToday = exists && !checked
	}
	if hasPomodoroInToday {
		statusBar = i18n.Emoji(fs.DisplayName(fs.PomodoroTask))
	}

	tasksCount := 0
	if len(msgsCount) > 0 && msgsCount[0] > 0 {
		tasksCount += msgsCount[0]
	}

	if tasksCount == 0 {
		statusBar += i18n.Emoji("palm")
	}

	if len(statusBar) != 0 {
		statusBar += " "
	}

	if tasksCount == 0 {
		return statusBar + i18n.Tr("You don't have any tasks!")
	}

	return statusBar + fmt.Sprintf(i18n.Tr("<b>%d</b> left%s"), tasksCount, wideSpacer)
}

func (b *Bot) randomNote(_ []string) error {
	rootEntries, err := b.fs.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return fmt.Errorf("random note: can't get root: %w", err)
	}
	type note struct {
		dir, name string
	}
	var notes []note
	for _, dir := range fs.OnlyNoteDirs(fs.OnlyDirs(rootEntries)) {
		entries, err := b.fs.FilesAndDirs(dir.Name)
		if err != nil {
			return fmt.Errorf("random note: can't get files in %s: %w", dir.Name, err)
		}
		for _, f := range fs.OnlyUserMDFiles(entries) {
			notes = append(notes, note{dir: dir.Name, name: f.Name})
		}
	}
	if len(notes) == 0 {
		return b.ShowToday(nil)
	}
	pick := notes[rand.Intn(len(notes))]
	return b.showFile([]string{fs.Hash(pick.dir), fs.Hash(pick.name)})
}

func (b *Bot) showFiles(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return fmt.Errorf("show files: can't get files: %w", err)
	}

	var kb tg.Keyboard
	mdFiles := fs.ExcludeConfig(fs.OnlyUserMDFiles(files))
	var fileBtns []tg.Btn
	for _, file := range mdFiles {
		cmd := tg.NewCmd(CmdShowFile, []string{fs.DirUserRoot, fs.Hash(file.Name)})
		btn := tg.NewBtn(fmt.Sprintf("%s", fs.UnsanitizeFilename(file.DisplayName)), cmd)
		fileBtns = append(fileBtns, btn)
	}
	fileBtnsByRows := slice.Chunk(fileBtns, btnsPerRow)
	for _, row := range fileBtnsByRows {
		kb.AddRow(row)
	}
	inlineCmd := tg.NewCustomCmd(CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)

	footer := tg.NewRow(tg.NewBtn(i18n.Tr("🔎 Search"), inlineCmd))
	if !b.cfg.NotesOnlyMode() {
		footer = append(footer, tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil)))
	}
	kb.AddRow(footer)

	err = b.showHTML(b.tr("📄 Your files:")+wideSpacer, &kb)
	if err != nil {
		return fmt.Errorf("show files: %w", err)
	}

	return nil
}

func (b *Bot) showDirs(_ []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return fmt.Errorf("show dirs: can't get dirs: %w", err)
	}

	dirs := fs.OnlyNoteDirs(fs.OnlyDirs(files))
	var dirBtns []tg.Btn
	for _, dir := range dirs {
		cmd := tg.NewCustomCmd("", []string{dir.Name}, tg.CmdTypeInlineQueryCurrentChat)
		btn := tg.NewBtn(fmt.Sprintf("%s %s", i18n.Emoji("dir"), dir.DisplayName), cmd)
		dirBtns = append(dirBtns, btn)
	}

	var kb tg.Keyboard
	dirBtnsByRows := slice.Chunk(dirBtns, btnsPerRow)
	for _, row := range dirBtnsByRows {
		kb.AddRow(row)
	}

	inlineCmd := tg.NewCustomCmd(CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)
	footer := tg.NewRow(tg.NewBtn(i18n.Tr("🔎 Search"), inlineCmd))
	if !b.cfg.NotesOnlyMode() {
		footer = append(footer, tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil)))
	}
	kb.AddRow(footer)

	err = b.showHTML(b.tr("🗂 Your dirs:")+wideSpacer, &kb)
	if err != nil {
		return fmt.Errorf("show dirs: %w", err)
	}

	return nil
}

func (b *Bot) showChecklists(_ []string) error {
	checklists, err := b.fs.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}
	checklists = fs.OnlyChecklists(checklists)

	var kb tg.Keyboard
	for _, checklist := range checklists {
		cmd := tg.NewCmd(CmdShowChecklist, []string{fs.Hash(checklist.Name)})
		btn := tg.NewBtn(i18n.AddEmoji(checklistTitle(checklist.Name)), cmd)

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(b.tr("🏠 Today"), tg.NewCmd(CmdShowToday, nil)))

	err = b.showHTML(b.tr("☑️ Checklists"), &kb)
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}

	return nil
}

func (b *Bot) showPostpone(_ []string) error {
	var kb tg.Keyboard

	// Inbox items also show in /postpone so the user can send them to Later.md.
	inboxMD, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err == nil {
		for _, block := range readBlocks(inboxMD) {
			if inboxHeaderRegex.MatchString(block) {
				continue
			}
			if strings.HasPrefix(block, "- [x] ") || strings.HasPrefix(block, "- [X] ") {
				continue
			}
			preview := strings.SplitN(stripInboxEntryPrefix(block), "\n", 2)[0]
			if len([]rune(preview)) > maxHeaderLengthForMobile {
				preview = string([]rune(preview)[:maxHeaderLengthForMobile]) + "…"
			}
			cmd := tg.NewCmd(CmdPostpone, []string{todayBlockHash(block)})
			kb.AddRow(tg.NewBtn("💬 "+preview, cmd))
		}
	}

	kb.AddRow(tg.NewRow(
		tg.NewBtn(b.tr("Rename"), tg.NewCmd(CmdShowRename, []string{})),
		tg.NewBtn(b.tr("OK"), tg.NewCmd(CmdShowToday, []string{})),
	))

	err = b.showHTML(b.tr("🦥 Select a task to postpone:"), &kb)
	if err != nil {
		return fmt.Errorf("show postpone: %w", err)
	}

	return nil
}

func (b *Bot) showMoveFromToday(_ []string) error {
	var kb tg.Keyboard

	// Show today inbox items
	inboxContent, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err == nil {
		blocks := readBlocks(inboxContent)
		for _, block := range blocks {
			if inboxHeaderRegex.MatchString(block) {
				continue
			}
			// Skip already-completed entries — they're about to be swept anyway.
			if strings.HasPrefix(block, "- [x] ") || strings.HasPrefix(block, "- [X] ") {
				continue
			}
			preview := strings.SplitN(stripInboxEntryPrefix(block), "\n", 2)[0]
			if len([]rune(preview)) > maxHeaderLengthForMobile {
				preview = string([]rune(preview)[:maxHeaderLengthForMobile]) + "…"
			}
			cmd := tg.NewCmd(CmdShowMoveTo, []string{todayBlockHash(block)})
			kb.AddRow(tg.NewBtn("💬 "+preview, cmd))
		}
	}

	kb.AddRow(tg.NewRow(
		tg.NewBtn(b.tr("Rename"), tg.NewCmd(CmdShowRename, []string{})),
		tg.NewBtn(b.tr("OK"), tg.NewCmd(CmdShowToday, []string{})),
	))

	err = b.showHTML(b.tr("🦥 Select an item to move:"), &kb)
	if err != nil {
		return fmt.Errorf("show move from today: %w", err)
	}

	return nil
}

func (b *Bot) postpone(params []string) error {
	hash := params[0]

	err := b.moveFromToday(func(content string, _ time.Time) error {
		laterMD, rerr := b.fs.Read(fs.DirUserRoot, fs.LaterFilename)
		if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return fmt.Errorf("postpone: can't read later file: %w", rerr)
		}
		return b.fs.Write(fs.DirUserRoot, fs.LaterFilename, txt.AddChecklistItem(laterMD, content, false))
	}, false, hash)
	if err != nil {
		return fmt.Errorf("postpone: can't move inbox entry to later: %w", err)
	}

	return b.showPostpone(nil)
}

func (b *Bot) showRename(_ []string) error {
	var kb tg.Keyboard

	inboxMD, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err == nil {
		for _, block := range readBlocks(inboxMD) {
			if inboxHeaderRegex.MatchString(block) {
				continue
			}
			if strings.HasPrefix(block, "- [x] ") || strings.HasPrefix(block, "- [X] ") {
				continue
			}
			preview := strings.SplitN(stripInboxEntryPrefix(block), "\n", 2)[0]
			if len([]rune(preview)) > maxHeaderLengthForMobile {
				preview = string([]rune(preview)[:maxHeaderLengthForMobile]) + "…"
			}
			cmd := tg.NewCmd(CmdShowRenameFile, []string{fs.TodayFilename, todayBlockHash(block)})
			kb.AddRow(tg.NewBtn("💬 "+preview, cmd))
		}
	}

	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil)))

	err = b.showHTML(b.todayLabel(), &kb)
	if err != nil {
		return fmt.Errorf("show rename: %w", err)
	}

	return nil
}

func (b *Bot) showRenameFile(params []string) error {
	checklist := params[0]
	itemHash := params[1]

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBack, tg.NewCmd(CmdShowToday, []string{}))),
	})

	cmd := tg.NewCmd(CmdRename, []string{checklist, itemHash, "%s"})
	b.db.SetInputExpectation(cmd)

	err := b.showHTML(i18n.Tr("OK. Send me the new name for your task"), kb)
	if err != nil {
		return fmt.Errorf("show rename: %w", err)
	}

	return nil
}

func (b *Bot) rename(params []string) error {
	checklist := params[0]
	itemHash := params[1]
	newItemNameFromUserInput := params[2]

	md, err := b.fs.Read(fs.DirUserRoot, checklist)
	if err != nil {
		return fmt.Errorf("rename: can't read checklist %s: %w", checklist, err)
	}

	if checklist == fs.TodayFilename {
		md, err = renameTodayBlock(md, itemHash, newItemNameFromUserInput)
		if err != nil {
			return fmt.Errorf("rename: %w", err)
		}
	} else {
		md, _ = txt.RemoveChecklistItem(md, itemHash)
		md = txt.AddChecklistItem(md, newItemNameFromUserInput, false)
	}

	err = b.fs.Write(fs.DirUserRoot, checklist, md)
	if err != nil {
		return fmt.Errorf("rename: can't write checklist %s: %w", checklist, err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) showStats(_ []string) error {
	report, err := stats.TodayReport(b.fs, b.db, b.userID)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil))})
	err = b.showHTML(strings.TrimSpace(report), kb)
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
	schedule := ScheduleReport(scheduledTasks)
	if len(schedule) == 0 {
		schedule = i18n.Tr("You don't have any scheduled tasks! 🌴")
	}

	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil))})
	err = b.showHTML(schedule, kb)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	return nil
}

func (b *Bot) showRead(_ []string) error {
	return b.showChecklist([]string{fs.Hash(fs.ReadFilename)})
}

func (b *Bot) showWatch(_ []string) error {
	return b.showChecklist([]string{fs.Hash(fs.WatchFilename)})
}

func (b *Bot) showShop(_ []string) error {
	return b.showChecklist([]string{fs.Hash(fs.ShopFilename)})
}

// TODO today.md move to today/later
func (b *Bot) showLongItem(params []string) error {
	checklistHash := params[0]
	itemHash := params[1]

	checklist, err := b.fs.Unhash(fs.DirUserRoot, checklistHash)
	if err != nil {
		return fmt.Errorf("complete checklist item: can't unhash checklist %s: %w", checklistHash, err)
	}

	checklistMD, err := b.fs.Read(fs.DirUserRoot, checklist)
	if err != nil {
		return fmt.Errorf("complete checklist item: can't read checklist %s: %w", checklist, err)
	}

	item := txt.ChecklistItem(checklistMD, itemHash)

	cmd := CmdShowToday
	if checklist == fs.LaterFilename {
		cmd = CmdShowLater
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn(i18n.StrBack, tg.NewCmd(cmd, []string{})),
			tg.NewBtn(i18n.StrComplete, tg.NewCmd(CmdCompleteChecklistItem, []string{checklistHash, itemHash})),
		),
	})

	err = b.showMD(item, kb)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	return nil
}

// TODO today.md move to today/later
func (b *Bot) showLongItemFromInbox(params []string) error {
	msgHash := params[0]

	inboxMD, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err != nil {
		return fmt.Errorf("show long item: can't read inbox file: %w", err)
	}

	_, block, ok := findTodayBlockByHash(inboxMD, msgHash)
	if !ok {
		return fmt.Errorf("show long item: msgHash %q not found in inbox", msgHash)
	}

	// Strip optional `- [ ]` / `- [x] ` prefix + backtick-timestamp prefix.
	prefixRegex := regexp.MustCompile(`^(?:- \[[ xX]\] )?` + "`" + `\d{2}:\d{2}` + "`" + ` `)
	if loc := prefixRegex.FindStringIndex(block); loc != nil {
		block = strings.TrimSpace(block[loc[1]:])
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn(i18n.StrBack, tg.NewCmd(CmdShowToday, []string{})),
			tg.NewBtn(i18n.AddEmoji("Move"), tg.NewCmd(CmdShowMoveTo, []string{msgHash})),
			tg.NewBtn(txt.Emoji(i18n.Emoji("Archive"), "Complete"), tg.NewCmd(CmdComplete, []string{msgHash})),
		),
	})

	if err := b.showMD(block, kb); err != nil {
		return fmt.Errorf("show long item from inbox: %w", err)
	}

	return nil
}

func (b *Bot) showFile(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirUserRoot, dirHash)
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
		inlineCmd := tg.NewCustomCmd(CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)
		row = append(row, tg.NewBtn(i18n.Tr("🔎 Search"), inlineCmd))

		hasChannelsToPrint := len(b.cfg.Channels()) > 0
		if hasChannelsToPrint {
			cmd := tg.NewCmd(CmdShare, []string{fs.Hash(dir), fs.Hash(filename)})
			row = append(row, tg.NewBtn(i18n.Tr("🖨 Share"), cmd))
		}
	}
	row = append(row, tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil)))
	kb := tg.NewKeyboard([]tg.Row{row})

	md := fmt.Sprintf("**%s**\n\n%s", fs.DisplayName(filename), content)
	err = b.showMD(md, kb)
	if err != nil {
		return fmt.Errorf("show file: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if hasLastKeyboard {
		b.db.SetRecentFilenameByMsgID(msgID, filename)
		b.db.SetRecentDirByMsgID(msgID, dir)
	}

	return nil
}

func (b *Bot) showChecklist(params []string) error {
	checklistHash := params[0]

	checklist, err := b.fs.Unhash(fs.DirUserRoot, checklistHash)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	md, err := b.fs.Read(fs.DirUserRoot, checklist)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	items := txt.IncompleteChecklistItems(md)
	// TODO check that we're showing last buttons
	maxButtons := maxBtns
	if checklist == fs.ReadFilename || checklist == fs.WatchFilename {
		maxButtons = maxBtnsInChecklist
	}
	items = items[max(0, len(items)-maxButtons):]

	kb := tg.NewKeyboard(nil)
	for _, item := range items {
		if len([]rune(item)) >= maxHeaderLengthForMobile {
			cmd := tg.NewCmd(CmdShowLongItem, []string{fs.Hash(checklist), fs.Hash(item)})
			btn := tg.NewBtn(txt.Emoji(i18n.Emoji("eyes"), item), cmd)
			kb.AddRow(btn)
		} else {
			cmd := tg.NewCmd(CmdCompleteChecklistItem, []string{checklistHash, fs.Hash(item)})
			btn := tg.NewBtn(i18n.AddEmoji(item), cmd)
			kb.AddRow(btn)
		}
	}
	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil)))

	title := checklistTitle(checklist)
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
			return b.setNotesOnlyMode(nil)
		} else if mode == "tasks" {
			return b.setTasksOnlyMode(nil)
		} else if mode == "journal" {
			return b.setJournalOnlyMode(nil)
		} else if mode == "full" {
			return b.setFullMode(nil)
		}
	}

	// Default to full mode, people don't like to choose.
	return b.setFullMode(nil)
}

func (b *Bot) moveToDirFromToday(params []string) error {
	// TODO Remove input expectations if dir is not today
	toDirHash := params[0]
	fromDirHash := params[1]
	fromFilenameHash := params[2]

	oldDir, err := b.fs.Unhash(fs.DirUserRoot, fromDirHash)
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

	toDir, err := b.fs.Unhash(fs.DirUserRoot, toDirHash)
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
		_ = journal.AddRecord(b.fs, fmt.Sprintf("📌 %s", fs.DisplayName(filename)), b.cfg.Timezone())
	}

	b.db.SetRecentCommand(CmdMoveToExistingNote)
	// Move from dir is today, because quick command
	// appears when file is in today dir
	b.db.SetRecentCommandParams([]string{fs.Hash(filename), toDirHash})

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("dir"), fmt.Sprintf(i18n.Tr("Moved to <b>%s</b>"), fs.DisplayName(toDir)))
	// Just an informative messages
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToDir(params []string) error {
	// TODO Remove input expectations if dir is not today
	toDirHash := params[0]

	msgHashes := strings.Split(params[1], ",")

	toDir, err := b.fs.Unhash(fs.DirUserRoot, toDirHash)
	canCreateMissingDir := slices.Contains([]string{fs.DirArchive, fs.DirHabits}, toDirHash)
	if err != nil {
		if canCreateMissingDir {
			// It will be created later in createOrAdd.
			toDir = toDirHash
		} else {
			return fmt.Errorf("move: can't unhash new dir %s: %w", toDir, err)
		}
	}

	err = b.moveFromToday(func(content string, timestamp time.Time) error {
		var sanitizedTitle string
		sanitizedTitle, content, err = b.extractHeaderAndBody(content, maxHeaderLength)
		if err != nil {
			return fmt.Errorf("move to dir from chat: can't extract title and content: %w", err)
		}

		filename := fs.Filename(sanitizedTitle)

		notesDir := fs.OnlyNoteDirs([]fs.File{{Name: toDir}})
		isNotesDir := len(notesDir) == 1
		if isNotesDir {
			// We can tolerate this, as this is informative logging
			_ = journal.AddRecord(b.fs, fmt.Sprintf("📌 %s", fs.DisplayName(filename)), b.cfg.Timezone())
		}

		return b.createOrAdd(toDir, filename, content)
	}, true, msgHashes...)

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("dir"), fmt.Sprintf(i18n.Tr("Moved to <b>%s</b>"), fs.DisplayName(toDir)))
	// Just an informative messages
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToChecklist(params []string) error {
	toChecklistHash := params[0]

	msgHashes := strings.Split(params[1], ",")

	for _, msgHash := range msgHashes {
		_, err := b.addToChecklist(toChecklistHash, msgHash)
		if err != nil {
			return fmt.Errorf("move to checklist: can't add to checklist: %w", err)
		}
	}

	return b.ShowToday(nil)
}

func (b *Bot) addToChecklist(checklistHash string, msgHash string) (string, error) {
	checklist, err := b.fs.Unhash(fs.DirUserRoot, checklistHash)
	// Create known checklist if it doesn't exist
	if err != nil {
		supportedChecklists := []string{
			fs.TodayFilename,
			fs.LaterFilename,
			fs.ReadFilename,
			fs.WatchFilename,
			fs.ShopFilename,
		}

		created := false
		for _, supportedChecklist := range supportedChecklists {
			if fs.Hash(supportedChecklist) == checklistHash || supportedChecklist == checklistHash {
				checklist = supportedChecklist
				err = b.fs.Write(fs.DirUserRoot, checklist, "")
				if err != nil {
					return "", fmt.Errorf("add to checklist: can't create checklist %s: %w", checklist, err)
				}
				created = true
				break
			}
		}

		if !created {
			return "", fmt.Errorf("add to checklist: can't unhash checklist %s: %w", checklistHash, err)
		}
	}

	checklistMD, err := b.fs.Read(fs.DirUserRoot, checklist)
	if err != nil {
		return "", fmt.Errorf("add to checklist: can't read checklist %s: %w", checklist, err)
	}

	var item string
	err = b.moveFromToday(func(content string, timestamp time.Time) error {
		item = content
		md := txt.AddChecklistItem(checklistMD, content, false)
		return b.fs.Write(fs.DirUserRoot, checklist, md)
	}, true, msgHash)
	if err != nil {
		return "", fmt.Errorf("move to checklist: can't move from chat: %w", err)
	}

	return item, nil
}

func (b *Bot) completeChecklistItem(params []string) error {
	checklistHash := params[0]
	itemHash := params[1]

	checklist, err := b.fs.Unhash(fs.DirUserRoot, checklistHash)
	if err != nil {
		return fmt.Errorf("complete checklist item: can't unhash checklist %s: %w", checklistHash, err)
	}

	checklistMD, err := b.fs.Read(fs.DirUserRoot, checklist)
	if err != nil {
		return fmt.Errorf("complete checklist item: can't read checklist %s: %w", checklist, err)
	}

	md, item := txt.CompleteChecklistItem(checklistMD, itemHash)
	err = b.fs.Write(fs.DirUserRoot, checklist, md)
	if err != nil {
		return fmt.Errorf("complete checklist item: can't complete item from chat: %w", err)
	}

	if item == fs.PomodoroTask {
		err = b.cfg.AddToSchedule(item, time.Now().Unix()+int64(b.cfg.PomodoroDuration().Seconds()), "")
		if err != nil {
			return fmt.Errorf("complete checklist item: can't add to schedule: %w", err)
		}
	} else {
		// We can tolerate failure of writing to journal, since that's not single source of truth
		_ = journal.AddRecord(b.fs, fmt.Sprintf("✅ %s", fs.DisplayName(item)), b.cfg.Timezone())
	}

	if checklist == fs.LaterFilename {
		return b.showLaterTasks(nil)
	} else if checklist != fs.TodayFilename {
		return b.showChecklist([]string{checklist})
	}

	return b.ShowToday(nil)
}

func (b *Bot) requestNewDirName(params []string) error {
	filenameHash := params[0]

	err := b.showHTML(i18n.Tr("OK. Send me the name for your new dir"), nil)
	if err != nil {
		return fmt.Errorf("request new dir: %w", err)
	}

	b.db.SetInputExpectation(tg.NewCmd(CmdMoveToNewDir, []string{filenameHash, "%s"}))

	return nil
}

// moveToNewDir accepts dir name as a second parameter
// which is a bit off, but the thing is sometimes it is replaced with
// inputExpectation, which only can add parameters in the end.
func (b *Bot) moveToNewDir(params []string) error {
	msgIndicesStr := params[0]
	dir := strings.ToLower(fs.SanitizeFilename(params[1]))

	exists, err := b.fs.Exists(fs.DirUserRoot, dir)
	if err != nil {
		return fmt.Errorf("move to new dir from caht: %w", err)
	}
	if !exists {
		err = b.fs.MakeDir(dir)
		if err != nil {
			return fmt.Errorf("move to new dir from chat: %w", err)
		}
	}

	return b.moveToDir([]string{dir, msgIndicesStr})
}

// TODO reuse move to existing note as more general?
func (b *Bot) moveToExistingFile(params []string) error {
	// TODO Remove input expectations if dir is not today (?)
	existingFilenameHash := params[0]

	msgHashes := strings.Split(params[1], ",")

	existingFilename, err := b.fs.Unhash(fs.DirUserRoot, existingFilenameHash)
	if err != nil {
		return fmt.Errorf("move to file: can't unhash existing file '%s': %w", existingFilenameHash, err)
	}

	err = b.moveFromToday(func(content string, timestamp time.Time) error {
		return b.addToFile(fs.DirUserRoot, existingFilename, content)
	}, true, msgHashes...)
	if err != nil {
		return fmt.Errorf("move to file: can't add to existing file '%s': %w", existingFilename, err)
	}

	b.db.SetRecentCommand(CmdMoveToExistingFile)
	b.db.SetRecentCommandParams([]string{fs.ShortHash(existingFilename)})

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("file"), fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.DisplayName(existingFilename)))
	// Just an informative messages
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToExistingNote(params []string) error {
	toFilenameHash := params[0]
	toDirHash := params[1]

	msgHashes := strings.Split(params[2], ",")

	var toDir string
	if toDirHash == "" {
		toDir = fs.DirUserRoot
	} else {
		var err error
		toDir, err = b.fs.Unhash(fs.DirUserRoot, toDirHash)
		if err != nil {
			return fmt.Errorf("move to existing note: %w", err)
		}
	}

	toFilename, err := b.fs.Unhash(toDir, toFilenameHash)
	if err != nil {
		return fmt.Errorf("move to existing note:: %w", err)
	}

	err = b.moveFromToday(func(content string, t time.Time) error {
		err = b.addToFile(toDir, toFilename, content)
		if err != nil {
			return fmt.Errorf("move to existing note: can't add to file %s: %w", toFilename, err)
		}

		b.db.SetRecentCommand(CmdMoveToExistingNote)
		b.db.SetRecentCommandParams([]string{fs.ShortHash(toFilename), fs.ShortHash(toDir)})

		return nil
	}, false, msgHashes...)
	if err != nil {
		return fmt.Errorf("move to existing note: can't read content from chat: %w", err)
	}

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("file"), fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.DisplayName(toFilename)))
	// Just an informative messages
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToDirChecklist(params []string) error {
	msgHashes := strings.Split(params[0], ",")
	checklistDirHash := params[1]

	checklistDir, err := b.fs.Unhash(fs.DirUserRoot, checklistDirHash)
	if err != nil {
		return fmt.Errorf("move to checklistDir: %w", err)
	}

	err = b.moveFromToday(func(content string, t time.Time) error {
		isMultiline := txt.IsMultiline(content)

		if isMultiline && b.cfg.ShouldSplitChecklist(checklistDir) {
			content = strings.TrimSpace(txt.NormNewLines(content))
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				line = fs.SanitizeFilename(line)
				err = b.fs.Write(checklistDir, fs.Filename(line), "")
				if err != nil {
					return fmt.Errorf("move to checklistDir: %w", err)
				}
			}
		} else {
			sanitizedTitle, content, err := b.extractHeaderAndBody(content, maxHeaderLengthForMobile)
			if err != nil {
				return fmt.Errorf("move to checklistDir: %w", err)
			}
			filename := fs.Filename(sanitizedTitle)
			return b.fs.Write(checklistDir, filename, content)
		}

		return nil
	}, false, msgHashes...)
	if err != nil {
		return fmt.Errorf("move to checklistDir: can't read content from chat: %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) moveToRead(params []string) error {
	msgIndices := params[0]

	return b.moveToChecklist([]string{fs.Hash(fs.ReadFilename), msgIndices})
}

func (b *Bot) moveToWatch(params []string) error {
	msgIndices := params[0]

	return b.moveToChecklist([]string{fs.Hash(fs.WatchFilename), msgIndices})
}

func (b *Bot) moveToShop(params []string) error {
	msgIndices := params[0]

	return b.moveToChecklist([]string{fs.Hash(fs.ShopFilename), msgIndices})
}

func (b *Bot) moveToNewFile(params []string) error {
	msgHash := params[0]
	newFilenameFromUserInput := fs.Filename(params[1])

	//filename, err := b.fs.Unhash(fs.DirUserRoot, msgIndex)
	//if err != nil {
	//	return fmt.Errorf("move to new file: can't unhash existing file '%s': %w", msgIndex, err)
	//}
	//
	//// Save existing filename to content in case the content of new file is empty (i.e. not multiline)
	//content, err := b.fs.Read(fs.DirUserRoot, filename)
	//if err != nil {
	//	return fmt.Errorf("move to new file: can't read file '%s': %w", filename, err)
	//}
	err := b.moveFromToday(func(content string, t time.Time) error {
		content = strings.TrimSpace(content)
		//if len(content) == 0 {
		//	content = fs.DisplayName(filename)
		//	err = b.fs.Write(fs.DirUserRoot, filename, content)
		//	if err != nil {
		//		return fmt.Errorf("move to new file: can't write content of '%s': %w", filename, err)
		//	}
		//}

		// TODO check for safety
		// TODO won't we lost some text here in case of multiline?
		//err = b.fs.Rename(fs.DirUserRoot, filename, fs.DirUserRoot, newFilenameFromUserInput)
		//if err != nil {
		//	return fmt.Errorf("move to new file: can't create empty file: %w", err)
		//}

		// We can tolerate this
		//_ = journal.AddRecord(b.fs, fmt.Sprintf("📄 %s", fs.DisplayName(filename)), b.cfg.Timezone())

		b.db.SetRecentCommand(CmdMoveToExistingFile)
		b.db.SetRecentCommandParams([]string{fs.ShortHash(newFilenameFromUserInput)})

		// TODO add if exists
		return b.fs.Write(fs.DirUserRoot, newFilenameFromUserInput, content)
	}, false, msgHash)
	if err != nil {
		return fmt.Errorf("move to new file: can't read content from chat: %w", err)
	}

	msg := txt.Emoji(i18n.Emoji("file"), fmt.Sprintf(i18n.Tr("Saved to <b>%s</b>"), fs.DisplayName(newFilenameFromUserInput)))
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToNewChecklist(params []string) error {
	msgHash := params[0]

	supposedName := params[1]
	supposedName = fs.SanitizeFilename(supposedName)

	dir := strings.ToLower(supposedName)
	dir = fmt.Sprintf("_%s_", dir)
	exists, err := b.fs.Exists(fs.DirUserRoot, dir)
	if err != nil {
		return fmt.Errorf("move to new checklist: %w", err)
	}
	if !exists {
		err = b.fs.MakeDir(dir)
	}

	return b.moveToDir([]string{dir, msgHash})
}

func (b *Bot) moveToJournal(params []string) error {
	msgHashes := params

	err := b.moveFromToday(func(content string, t time.Time) error {
		// TODO take into account time from chat
		return journal.AddRecord(b.fs, content, b.cfg.Timezone())
	}, false, msgHashes...)
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't add record: %w", err)
	}

	b.delAllKeyboards()
	msg := txt.Emoji(i18n.Emoji("journal"), i18n.Tr("Saved to <b>journal</b>"))
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	if b.cfg.JournalOnlyMode() {
		return nil
	}

	return b.ShowToday(nil)
}

func (b *Bot) addToJournalAndContinue(params []string) error {
	content := params[0]

	err := journal.AddRecord(b.fs, content, b.cfg.Timezone())
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't add note: %w", err)
	}

	// Don't return - continue to save to inbox as well.
	msgHash, err := b.appendToToday(content, b.cfg.Timezone())
	if err != nil {
		return fmt.Errorf("save to inbox: %w", err)
	}

	return b.showMoveTo([]string{msgHash})
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
	cmd, _ := b.db.RecentCommand()

	var existingFilename string
	if cmd == CmdMoveToExistingFile {
		var err error
		existingFilename, err = b.fs.Unhash(fs.DirUserRoot, args[0])
		if err != nil {
			return fmt.Errorf("failed to move to recent file or note: can't unhash filename: %w", err)
		}

		err = b.addToFile(fs.DirUserRoot, existingFilename, content)
		if err != nil {
			return fmt.Errorf("failed to move to recent file: can't add note: %w", err)
		}
	} else if cmd == CmdMoveToExistingNote {
		dir, err := b.fs.Unhash(fs.DirUserRoot, args[1])
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

	msg := fmt.Sprintf(i18n.Tr("Added to <b>%s</b>"), fs.DisplayName(existingFilename))
	_, _ = b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)

	return b.ShowToday(nil)
}

func (b *Bot) moveToLater(params []string) error {
	msgHash := params[0]

	return b.moveToChecklist([]string{fs.LaterFilename, msgHash})
}

// complete toggles the Markdown task marker on a single Inbox.md entry
// in place. `- [ ]` ↔ `- [x]`. Legacy entries without a prefix are upgraded to
// `- [x]`. The entry stays in the file; it is no longer archived.
func (b *Bot) complete(params []string) error {
	msgHash := params[0]

	key, err := b.fs.SafePath(fs.DirUserRoot, "")
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	lock := userLock(key)
	lock.Lock()
	defer lock.Unlock()

	content, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err != nil {
		return fmt.Errorf("complete: can't read inbox: %w", err)
	}

	blockIdx, block, ok := findTodayBlockByHash(content, msgHash)
	if !ok {
		return fmt.Errorf("complete: msgHash %q not found in inbox", msgHash)
	}

	blocks := readBlocks(content)
	switch {
	case strings.HasPrefix(block, "- [ ] "):
		blocks[blockIdx] = "- [x] " + block[6:]
	case strings.HasPrefix(block, "- [x] "), strings.HasPrefix(block, "- [X] "):
		blocks[blockIdx] = "- [ ] " + block[6:]
	default:
		// Legacy entry without a task prefix — upgrade to completed form.
		blocks[blockIdx] = "- [x] " + block
	}

	newContent := strings.TrimSpace(strings.Join(blocks, "\n"))
	if err := b.fs.Write(fs.DirUserRoot, fs.TodayFilename, newContent); err != nil {
		return fmt.Errorf("complete: can't write inbox: %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) completeListItem(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirUserRoot, dirHash)
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

	err = b.fs.Rename(dir, filename, fs.DirArchive, filename)
	if err != nil {
		return fmt.Errorf("complete: can't complete %s: %w", filename, err)
	}

	return b.showChecklist([]string{dirHash})
}

func (b *Bot) showChecklistItem(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirUserRoot, dirHash)
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
			tg.NewBtn(i18n.StrBack, tg.NewCmd(CmdShowChecklist, []string{dirHash})),
			tg.NewBtn(i18n.StrComplete, tg.NewCmd(CmdCompleteListItem, []string{dirHash, filenameHash})),
		),
	})

	err = b.showHTML(content, kb)
	if err != nil {
		return fmt.Errorf("show checklist item: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID()
	if hasLastKeyboard {
		b.db.SetRecentFilenameByMsgID(msgID, filename)
		b.db.SetRecentDirByMsgID(msgID, dir)
	}

	return nil
}

func (b *Bot) schedule(params []string) error {
	msgHash := params[0]
	timeStr := params[1]
	cron := params[2]

	scheduleTime, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("schedule: can't parse timestamp: %w", err)
	}

	item, err := b.addToChecklist(fs.LaterFilename, msgHash)
	if err != nil {
		return fmt.Errorf("schedule: can't move to later: %w", err)
	}

	err = b.cfg.AddToSchedule(item, scheduleTime, cron)
	if err != nil {
		return fmt.Errorf("schedule: can't add to schedule: %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) scheduleForTmrw(params []string) error {
	return b.schedule([]string{params[0], txt.I64(Tomorrow()), ""})
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
		return tg.NewBtn(name, tg.NewCmd(CmdSchedule, []string{filenameHash, txt.I64(NextExcludeToday(cron)), ""}))
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrRepeat, tg.NewCmd(CmdShowScheduleForDayRecurring, []string{filenameHash}))),
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
	kb.AddRow(tg.NewBtn(i18n.StrToToday, tg.NewCmd(CmdShowToday, nil)))

	return kb, nil
}

func (b *Bot) showMoveToFileOrDir(params []string) error {
	msgHash := params[0]
	maxRecentBtns := maxGroupedBtnsInMoveTo

	userWantedAllBtns := len(params) > 1
	if userWantedAllBtns {
		maxRecentBtns = maxBtns
	} else {
		//b.db.SetRecentCommand(CmdMoveToExistingFile)
		//b.db.SetRecentCommandParams([]string{fs.ShortHash(filename), fs.ShortHash(fs.DirToday)})
	}

	kb := tg.NewKeyboard(nil)
	skippedBtns := false

	//fileBtns, err := b.moveToFileBtns(fs.ShortHash(filename))
	fileBtns, err := b.moveToFileBtns(msgHash)
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

	dirBtns, err := b.moveToDirBtns(msgHash)
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
	btn := tg.NewBtn("🗂 New Dir", tg.NewCmd(CmdRequestNewDir, []string{msgHash}))
	dirBtns = append(dirBtns, btn)

	//shouldAddSeparator := len(fileBtns) > 0
	//if shouldAddSeparator {
	searchCMD := tg.NewCustomCmd(CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)
	kb.AddRow(tg.NewBtn(i18n.Tr("Search"), searchCMD))
	//}
	dirBtnsByRows := slice.Chunk(dirBtns, btnsPerRow)
	for _, row := range dirBtnsByRows {
		kb.AddRow(row)
	}

	if skippedBtns {
		kb.AddRow(tg.NewBtn(i18n.Tr("More..."), tg.NewCmd(CmdShowMoveToDirOrFile, []string{msgHash, "full"})))
	}

	b.db.SetInputExpectation(tg.NewCmd(CmdMoveToNewFile, []string{msgHash, "%s"}))

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

	b.db.SetInputExpectation(tg.NewCmd(CmdMoveToNewChecklist, []string{filenameHash, "%s"}))

	err = b.showHTML(i18n.Tr("Choose a checklist or name a new one"), kb)
	if err != nil {
		return fmt.Errorf("show to checklist: %w", err)
	}

	return nil
}

func (b *Bot) moveToFileBtns(msgHash string) ([]tg.Btn, error) {
	files, err := b.fs.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return nil, fmt.Errorf("to doc keyboard: %w", err)
	}
	files = fs.OnlyUserMDFiles(files)
	files = fs.SortByCtimeDesc(files)
	if len(files) == 0 {
		return nil, nil
	}

	var buttons []tg.Btn
	newBtn := func(title, existingFilenameHash string) tg.Btn {
		title = fmt.Sprintf("%s", title)
		params := []string{existingFilenameHash, msgHash}
		return tg.NewBtn(title, tg.NewCmd(CmdMoveToExistingFile, params))
	}
	for _, file := range files {
		buttons = append(buttons, newBtn(file.DisplayName, fs.ShortHash(file.Name)))
	}

	return buttons, nil
}

func (b *Bot) moveToDirBtns(msgHash string) ([]tg.Btn, error) {
	newBtn := func(dir string) tg.Btn {
		emojifiedDir := fmt.Sprintf("%s %s", i18n.Emoji("dir"), txt.Ucfirst(dir))
		return tg.NewBtn(emojifiedDir, tg.NewCmd(CmdMoveToExistingDir, []string{fs.ShortHash(dir), msgHash}))
	}

	dirs, err := b.fs.FilesAndDirs(fs.DirUserRoot)
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
		return tg.NewBtn(title, tg.NewCmd(CmdMoveToDirChecklist, []string{filenameHash, dir}))
	}

	dirs, err := b.fs.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return nil, fmt.Errorf("to checklist keyboard: %w", err)
	}
	// TODO handle case with zero dirs (inline_keyboard is null), for all similar cases
	dirs = fs.OnlyChecklists(fs.OnlyDirs(dirs))

	kb := tg.NewKeyboard(nil)
	for _, dir := range dirs {
		kb.AddRow(newBtn(dir.Name, dir.DisplayName))
	}

	return kb, nil
}

func (b *Bot) togglePomodoro(_ []string) error {
	// Check if Pomodoro is already running
	hasPomodoroInToday := false
	todayMD, err := b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	if err == nil {
		_, isCompleted := txt.ChecklistItems(todayMD)
		_, hasPomodoroInToday = isCompleted[fs.PomodoroTask]
	}

	hasPomodoroInArchive := false
	doneMD, err := b.fs.Read(fs.DirArchive, fs.DoneFilename)
	if err == nil {
		_, isCompleted := txt.ChecklistItems(doneMD)
		_, hasPomodoroInToday = isCompleted[fs.PomodoroTask]
	}

	if hasPomodoroInToday {
		todayMD, _ = txt.RemoveChecklistItem(todayMD, fs.PomodoroTask)
		err = b.fs.Write(fs.DirUserRoot, fs.TodayFilename, todayMD)
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to delete pomodoro file: %w", err)
		}
	}
	if hasPomodoroInArchive {
		doneMD, _ = txt.RemoveChecklistItem(doneMD, fs.PomodoroTask)
		err = b.fs.Write(fs.DirArchive, fs.DoneFilename, doneMD)
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to delete pomodoro file: %w", err)
		}
	}

	if hasPomodoroInToday || hasPomodoroInArchive {
		_, _ = b.tg.Send(b.userID, "Pomodoro is stopped", nil, tg.MarkupHTML)
		return b.ShowToday(nil)
	}

	//todayMD, err = b.fs.Read(fs.DirUserRoot, fs.TodayFilename)
	//if err != nil {
	//	return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
	//}
	// Create Pomodoro task
	err = b.fs.Write(fs.DirUserRoot, fs.TodayFilename, txt.AddChecklistItem(todayMD, fs.PomodoroTask, false))

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
		cmd := tg.NewCmd(CmdSchedule, []string{txt.Substr(filenameHash, 0, 4), txt.I64(NextExcludeToday(cron)), cron})
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
	kb.AddRow(tg.NewBtn(i18n.StrToToday, tg.NewCmd(CmdShowToday, nil)))

	err := b.showHTML(i18n.Tr("Repeat the task"), kb)
	if err != nil {
		return fmt.Errorf("showRecuringKeyboard : %w", err)
	}

	return nil
}

// addToFile adds content at the top of the file.
// Creates a file if not exists.
func (b *Bot) addToFile(dir, filename, content string) error {
	existingContent, err := b.fs.Read(dir, filename)
	// Ignore if file is missing, it would be created.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("add to file: can't read existing file: %w", err)
	}

	header := fmt.Sprintf("#### %d %s %d, %s", now().Day(), now().Format("January"), now().Year(), now().Weekday())
	newContent := txt.AddHeaderAndText(existingContent, header, content)

	err = b.fs.Write(dir, filename, newContent)
	if err != nil {
		return fmt.Errorf("add to file: can't save file: %w", err)
	}

	return nil
}

func (b *Bot) openInApp(_ []string) error {
	token := sync.GenOneTimeToken(b.userID)
	onetimeURL := fmt.Sprintf("%s?token=%s", config.ServerCfg.AppURL, token)
	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.Tr("Open in app"), tg.NewURLCmd(onetimeURL))})

	return b.showHTML(i18n.Tr("🔗 Here's your <b>one-time</b> link! <b>Desktop-only</b> for now."), kb)
}

// TODO release add help
func (b *Bot) showHelp(_ []string) error {
	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil))})

	return b.showHTML("Not yet implemented 🏗!", kb)
}

func (b *Bot) download(_ []string) error {
	kb := tg.NewKeyboard([]tg.Row{tg.NewBtn(i18n.StrToday, tg.NewCmd(CmdShowToday, nil))})

	return b.showHTML("Not yet implemented 🏗!", kb)
}

func (b *Bot) setTasksOnlyMode(_ []string) error {
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
		CmdScheduleForTmrw,
		CmdMoveToLater,
		CmdShowScheduleForDay,
	}
	for _, cmd := range moveToCmds {
		err = b.cfg.AddMoveToCmd(cmd)
		if err != nil {
			return fmt.Errorf("full mode: can't add quick command %w", err)
		}
	}

	return b.ShowToday(nil)
}

func (b *Bot) setNotesOnlyMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeNotes)
	if err != nil {
		return fmt.Errorf("notes only mode: can't set notes only mode %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) setJournalOnlyMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeJournal)
	if err != nil {
		return fmt.Errorf("journal only mode: can't set notes only mode %w", err)
	}

	return b.showHTML(i18n.Tr("What's on your mind?"), nil)
}

func (b *Bot) setFullMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeFull)
	if err != nil {
		return fmt.Errorf("full mode: can't set notes only mode %w", err)
	}

	moveToCmds := []string{
		CmdScheduleForTmrw,
		CmdMoveToLater,
		CmdShowScheduleForDay,
		CmdShowMoveToDirOrFile,
		CmdMoveToRead,
		CmdMoveToShop,
		CmdMoveToWatch,
		CmdMoveToJournal,
	}
	for _, cmd := range moveToCmds {
		err = b.cfg.AddMoveToCmd(cmd)
		if err != nil {
			return fmt.Errorf("full mode: can't add quick command %w", err)
		}
	}

	err = b.fs.CreateSystemDirs()
	if err != nil {
		return fmt.Errorf("full mode: can't create dirs: %w", err)
	}

	return b.ShowToday(nil)
}

func (b *Bot) setChatOnlyMode(_ []string) error {
	err := b.cfg.SetMode(userconfig.ModeChat)
	if err != nil {
		return fmt.Errorf("chat only mode: can't set chat only mode %w", err)
	}

	return b.showHTML(i18n.Tr("What's on your mind?"), nil)
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

	userConf := userconfig.NewConfig(b.fs, b.userID, config.ServerCfg.ConfigFilename)
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

	dir, err := b.fs.Unhash(fs.DirUserRoot, dirHash)
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
		probablyInvalidMD := fmt.Sprintf("**%s/%s**\n\n%s", fs.DisplayName(dir), fs.DisplayName(filename), content)
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
	checklist = strings.TrimSuffix(checklist, filepath.Ext(checklist))
	stripChecklistChars := regexp.MustCompile(`^_.*?_(.+)`)
	title := stripChecklistChars.ReplaceAllString(checklist, "$1")
	title = strings.TrimPrefix(strings.TrimSuffix(title, "_"), "_")

	return fs.DisplayName(title)
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
