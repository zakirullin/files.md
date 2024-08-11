package internal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal/constants"
	"zakirullin/stuffbot/internal/fs"
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
	botPlugins        []BotPluginInterface
	errUnknownCommand = errors.New("unknown command")
)

const (
	maxTitleLength         = 100
	inlineResultsCacheTime = 15 // seconds
	btnsPerRow             = 3
	quickBtnsPerRow        = 4
	maxBtns                = 50
	maxInlineResults       = 50
	maxMsgLength           = 4096 // UTF-8 characters
	maxMsgsToSendAtOnce    = 5
	wideSpacer             = "<code>            ⁠</code>" // On mobile phones, buttons shrink to msg width
)

// UpdInterface represents incoming user updates
type UpdInterface interface {
	MsgText() string
	UserID() int64
	Cmd() *tg.Cmd
	MsgEntities() []tgbotapi.MessageEntity
	CaptionEntities() []tgbotapi.MessageEntity
	IsForwarded() bool
	CallbackQueryID() (string, bool)
	InlineQueryID() (string, bool)
	InlineQuery() (string, bool)
	InlineQueryOffset() int
	IsSentViaBot() bool
	ReplyToMsgID() (int, bool)
	PhotoOrImageID() (string, bool)
	Caption() string
}

// TGInterface provides a simple interface to telegram API
type TGInterface interface {
	Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error)
	Edit(userID int64, msgID int, text string, kb *tg.Keyboard, markup string) error
	Del(userID int64, msgID int) error
	AnswerCallbackQuery(queryID string, text string) error
	AnswerInlineQuery(queryID string, results []interface{}, cacheTime int, offset string) error
	DownloadFile(fileID string, outFile io.Writer) (string, error)
}

type DBInterface interface {
	LastKeyboardMsgID(userID int64) (int, bool)
	SetLastKeyboardMsgID(userID int64, ID int)
	DelLastKeyboardMsgID(userID int64)
	InputExpectation(userID int64) *tg.Cmd
	SetInputExpectation(userID int64, cmd tg.Cmd)
	DelInputExpectation(userID int64)
	FilenameByMsgID(userID int64, msgID int) string
	SetFilenameByMsgID(userID int64, msgID int, filename string)
	DirByMsgID(userID int64, msgID int) string
	SetDirByMsgID(userID int64, msgID int, filename string)
}

// Bot provides commands that can be invoked by a user so to query
// server files and database. A user can also send all sort of things
// to bot (texts, photos) - in that case we'd save everything.
type Bot struct {
	userID int64
	tg     TGInterface
	fs     *fs.FS
	db     DBInterface
	conf   *userconfig.Config
}

type BotPluginInterface interface {
	ExecutePlugin(string) bool
}

var now = time.Now

func NewBot(userID int64, tg TGInterface, fs *fs.FS, db DBInterface, conf *userconfig.Config) *Bot {
	botPlugins = append(botPlugins,
		plugins.NewWorldClockPlugin(userID, tg),
	)

	return &Bot{userID, tg, fs, db, conf}
}

// Answer to incoming text message or command (inline queries aren't supported yet)
func (b *Bot) Answer(u UpdInterface) error {
	// Handle inline queries
	if _, ok := u.InlineQueryID(); ok {
		return b.search(u)
	}

	for _, plugin := range botPlugins {
		if plugin.ExecutePlugin(u.MsgText()) {
			return b.ShowTodayTasks(nil)
		}
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
			return fmt.Errorf("no such command %s: %w", cmd.Name, errUnknownCommand)
		}
		slog.Debug("Command is called", "command", cmd.Name, "params", cmd.Params)
		err = handler(cmd.Params)
		if err != nil {
			return err
		}

		if callbackQueryID, ok := u.CallbackQueryID(); ok {
			// We can tolerate an error here, that won't affect UX
			_ = b.tg.AnswerCallbackQuery(callbackQueryID, "")
		}

		return nil
	}

	// Handle inline query file requests
	// TODO write tests for all sorts of tricky input with ../
	if u.IsSentViaBot() {
		dirAndFilename := strings.Split(u.MsgText(), "/")
		var dir, filename string
		if len(dirAndFilename) == 1 {
			dir = fs.DirRoot
			filename = dirAndFilename[0]
		} else if len(dirAndFilename) == 2 {
			dir = dirAndFilename[0]
			filename = dirAndFilename[1]
		} else {
			return nil
		}

		b.delAllKeyboards()

		return b.showFile([]string{dir, filename})
	}

	// Handle forwards
	if u.IsForwarded() {
		return b.saveFromForward(u)
	}

	// Handle photos
	if _, hasPhoto := u.PhotoOrImageID(); hasPhoto {
		return b.saveFromPhoto(u)
	}

	// Handle regular text messages
	return b.saveFromRegularMsg(u)
}

// Commands and their handlers.
// Every handler accepts []string params
func (b *Bot) handlers() map[string]func([]string) error {
	return map[string]func([]string) error{
		// Direct user commands
		constants.CmdShowStart:          b.showStart,
		constants.CmdShowToday:          b.ShowTodayTasks,
		constants.CmdShowLater:          b.showLaterTasks,
		constants.CmdShowFiles:          b.showFiles,
		constants.CmdShowChecklists:     b.showChecklists,
		constants.CmdShowPostpone:       b.showPostpone,
		constants.CmdShowRename:         b.showRename,
		constants.CmdShowStats:          b.showStats,
		constants.CmdShowReadChecklist:  b.showRead,
		constants.CmdShowWatchChecklist: b.showWatch,
		constants.CmdShowShopChecklist:  b.showShop,
		// Button's commands (callbacks)
		constants.CmdRenameFile:         b.showRenameFile,
		constants.CmdShowMultilineTask:  b.showMultilineTask,
		constants.CmdShowFile:           b.showFile,
		constants.CmdShowChecklist:      b.showChecklist,
		constants.CmdCompleteChecklist:  b.completeChecklist,
		constants.CmdShowChooseDay:      b.showChooseDay,
		constants.CmdShowToFile:         b.showToFile,
		constants.CmdShowToChecklist:    b.showToChecklist,
		constants.CmdMove:               b.move,
		constants.CmdMoveToNewDir:       b.moveToNewDir,
		constants.CmdMoveToFile:         b.moveToExistingFile,
		constants.CmdMoveToNewFile:      b.moveToNewFile,
		constants.CmdMoveToChecklist:    b.moveToChecklist,
		constants.CmdMoveToNewChecklist: b.moveToNewChecklist,
		constants.CmdMoveJournal:        b.moveToJournal,
		constants.CmdSchedule:           b.schedule,
		constants.CmdComplete:           b.complete,
		constants.CmdPostpone:           b.postpone,
		constants.CmdPomodoro:           b.togglePomodoro,
		constants.CmdShowRecurringKB:    b.showRecurringKeyBoard,
		constants.CmdShowSettings:       b.showSettings,
		constants.CmdConfigurePanel:     b.showConfigureQuickPanel,
		constants.CmdAddToPanel:         b.addToPanel,
		constants.CmdDelFromPonel:       b.delFromPanel,
		// Used for button-like separators
		constants.CmdDoNothing: func(s []string) error { return nil },
	}
}

func (b *Bot) extractCmd(u UpdInterface) (*tg.Cmd, error) {
	cmd := u.Cmd()
	if cmd != nil {
		b.db.DelInputExpectation(b.userID)

		return cmd, nil
	}

	// Input expectation is mostly used for renaming things
	cmd = b.db.InputExpectation(b.userID)
	if cmd != nil {
		slog.Debug("Got command from input expectation", "command", cmd.Name)
		b.db.DelInputExpectation(b.userID)

		for i, param := range cmd.Params {
			if param == "%s" {
				cmd.Params[i] = u.MsgText()
			}
		}

		return cmd, nil
	}

	return nil, nil
}

func (b *Bot) allowedTextCmds() []string {
	return []string{
		constants.CmdShowStart,
		constants.CmdShowToday,
		constants.CmdShowLater,
		constants.CmdShowPostpone,
		constants.CmdShowFiles,
		constants.CmdShowRename,
		constants.CmdShowChecklists,
		constants.CmdShowStats,
		//"help" TODO,
		//"err" TODO,
	}
}

func (b *Bot) saveFromRegularMsg(u UpdInterface) error {
	content := extractPlainText(u)
	title, err := b.extractTitle(content)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	// Adding to an existing file
	if replyMsgID, ok := u.ReplyToMsgID(); ok {
		return b.addToRepliedFile(replyMsgID, content)
	}

	sanitizedTitle := fs.SanitizeFilename(title)

	// If title is the same as content, we don't need to save it
	if sanitizedTitle == content {
		content = ""
	}

	filename := fs.Filename(sanitizedTitle)
	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return b.showMoveTo([]string{fs.Hash(filename)})
}

func (b *Bot) saveFromPhoto(u UpdInterface) error {
	photoID, _ := u.PhotoOrImageID()

	var buf bytes.Buffer
	extension, err := b.tg.DownloadFile(photoID, &buf)
	if err != nil {
		return fmt.Errorf("can't download file: %w", err)
	}

	imgFilename := fmt.Sprintf("tg_%s%s", photoID, extension)
	err = b.fs.Write(fs.DirImg, imgFilename, buf.String())
	if err != nil {
		return fmt.Errorf("can't save photo: %w", err)
	}

	imgPath := fmt.Sprintf("../%s/%s", fs.DirImg, imgFilename)
	content := fmt.Sprintf("![[%s|center|400]]", imgPath)
	if u.Caption() != "" {
		caption := txt.EntitiesToMarkdown(u.Caption(), u.CaptionEntities())
		caption = strings.TrimSpace(txt.NormNewLines(caption))
		content = fmt.Sprintf("%s\n%s", content, txt.Ucfirst(caption))
	}

	// Adding to an existing file
	if replyMsgID, ok := u.ReplyToMsgID(); ok {
		return b.addToRepliedFile(replyMsgID, content)
	}

	// Creating a new file
	title := strings.TrimSpace(u.Caption())
	if len(title) > maxTitleLength {
		title = txt.Substr(title, 0, maxTitleLength) + "..."
	}
	if title == "" {
		title = fmt.Sprintf("Img %s", now().Format("02.01.06 15:04"))
	}
	sanitizedTitle := fs.SanitizeFilename(title)

	filename := fs.Filename(sanitizedTitle)
	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return b.showMoveTo([]string{fs.Hash(filename)})
}

func (b *Bot) saveFromForward(u UpdInterface) error {
	content := extractPlainText(u)
	title, err := b.extractTitle(content)
	if err != nil {
		return fmt.Errorf("save forward: %w", err)
	}
	// TODO what if sanitized content different same in
	// case of regular save, we should save it in the body
	title = fs.SanitizeFilename(title)
	filename := fs.Filename(title)

	// When a user forwards message + title we receive 2 updates from TG.
	// First we receive title, then the message itself. We must add our
	// forwarded message to previously saved task (by title).
	// We do sleep here because previous file might not be saved.
	// We may consider locks here, but the updates can come out of order
	time.Sleep(300 * time.Millisecond)
	files, err := b.fs.FilesAndDirs(fs.DirToday)
	if err != nil {
		return fmt.Errorf("save forward: %w", err)
	}

	files = fs.SortByCtimeDesc(fs.OnlyFiles(files))
	// TODO do we need that reverse?
	slices.Reverse(files)
	if len(files) > 0 {
		file := files[len(files)-1]
		fileWasCreatedRecently := (now().Unix() - file.Ctime) <= 2
		if fileWasCreatedRecently {
			filename = file.Name
		}
	}

	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save forward: %w", err)
	}

	return b.showMoveTo([]string{fs.Hash(filename)})
}

func (b *Bot) addToRepliedFile(replyToMsgID int, newContent string) error {
	dir := b.db.DirByMsgID(b.userID, replyToMsgID)
	filename := b.db.FilenameByMsgID(b.userID, replyToMsgID)
	existingContent, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("add: can't read: %w", err)
	}

	header := fmt.Sprintf("### %s", now().Format("02.01.2006 Monday"))
	content := txt.InsertTextAfterHeader(existingContent, header, newContent)
	err = b.fs.Write(dir, filename, content)
	if err != nil {
		return fmt.Errorf("add: can't write: %w", err)
	}

	b.delAllKeyboards()

	return b.ShowTodayTasks(nil)
}

func (b *Bot) search(u UpdInterface) error {
	query, ok := u.InlineQuery()
	if !ok {
		return nil
	}

	matchedNotes, err := b.fs.SearchNotes(query)
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
	err = b.tg.AnswerInlineQuery(queryID, results, inlineResultsCacheTime, nextOffset)
	// TG library has a bug of unmarshalling sent result, we'll mute that temporarely
	if err != nil && !strings.HasSuffix(err.Error(), "Go value of type tgbotapi.Message") {
		return fmt.Errorf("inline reply: %w", err)
	}

	return nil
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

		content = fmt.Sprintf("%s\n%s", strings.TrimSpace(existingContent), content)
	}

	if err := b.fs.Write(fs.DirToday, filename, content); err != nil {
		return fmt.Errorf("create: %w", err)
	}

	return nil
}

func (b *Bot) extractTitle(msg string) (string, error) {
	if len(msg) == 0 {
		return "", fmt.Errorf("extract title: empty msg")
	}

	parts := strings.SplitN(msg, "\n", 2)
	title := txt.Ucfirst(strings.TrimSpace(parts[0]))

	if len(title) > maxTitleLength {
		title = txt.Substr(title, 0, maxTitleLength) + "..."
	}

	return title, nil
}

func (b *Bot) tr(str string, args ...any) string {
	str = i18n.Tr(str)

	return fmt.Sprintf(str, args...)
}

// Replace last message + keyboard with the new ones
// Or show the new one (in case of photo)
func (b *Bot) show(text string, kb *tg.Keyboard, markup string) error {
	mid, hasLastKeyboard := b.db.LastKeyboardMsgID(b.userID)
	textChunks := txt.SplitTextIntoChunks(text, maxMsgLength)
	if !hasLastKeyboard || len(textChunks) > 1 {
		b.delAllKeyboards()

		// If our msg is too long, we send a few messages.
		// Keyboard is attached to the last one
		textChunks = textChunks[max(0, len(textChunks)-maxMsgsToSendAtOnce):]
		lastText, textChunks := textChunks[len(textChunks)-1], textChunks[:len(textChunks)-1]
		for _, textChunk := range textChunks {
			_, _ = b.tg.Send(b.userID, textChunk, nil, markup)
		}

		mid, err := b.tg.Send(b.userID, lastText, kb, markup)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}

		b.db.SetLastKeyboardMsgID(b.userID, mid)

		return nil
	}

	return b.tg.Edit(b.userID, mid, text, kb, markup)
}

func (b *Bot) showMoveTo(params []string) error {
	filenameHash := params[0]

	btns := []tg.Btn{
		tg.NewBtn(i18n.StrForTomorrow, tg.NewCmd(constants.CmdSchedule, []string{filenameHash, txt.I64(sched.Tomorrow()), ""})),
		tg.NewBtn(i18n.StrForLater, tg.NewCmd(constants.CmdMove, []string{fs.DirToday, filenameHash, "later"})),
		tg.NewBtn(i18n.StrForDay, tg.NewCmd(constants.CmdShowChooseDay, []string{filenameHash})),
		tg.NewBtn(i18n.StrToFile, tg.NewCmd(constants.CmdShowToFile, []string{filenameHash})),
		tg.NewBtn(i18n.StrToJournal, tg.NewCmd(constants.CmdMoveJournal, []string{fs.DirToday, filenameHash})),
		tg.NewBtn(i18n.StrToChecklist, tg.NewCmd(constants.CmdShowToChecklist, []string{filenameHash})),
	}

	var kb tg.Keyboard
	userBtnsByRows := slice.Chunk(btns, btnsPerRow)
	for _, row := range userBtnsByRows {
		kb.AddRow(row)
	}
	kb.AddRow(tg.NewBtn(i18n.StrBtnGoToToday, tg.NewCmd(constants.CmdShowToday, nil)))

	b.delAllKeyboards()

	err := b.show(b.tr("Task added for <b>today</b>!"), &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}

	return nil
}

func (b *Bot) quickPanelBtns() []tg.Btn {
	quickPanelRow := tg.NewRow()
	// We iterate through hardcoded panel to preserve order of buttons in UI
	for _, cmd := range b.conf.QuickPanelCmds() {
		for _, btn := range userconfig.QuickPanelAvailableBtns {
			if btn.Cmd == cmd {
				params := []string{}
				if btn.Cmd == constants.CmdWebAppHabits {
					habitsUrl := fmt.Sprintf("%s/habits_v2/%d", config.Config.Host, b.userID)
					params = []string{habitsUrl}
				}

				button := tg.NewBtn(btn.Emoji, tg.NewCustomCmd(btn.Cmd, params, btn.CmdType))
				quickPanelRow = append(quickPanelRow, button)
				break
			}
		}
	}

	return quickPanelRow
}

func (b *Bot) ShowTodayTasks(params []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirToday)
	if err != nil {
		return fmt.Errorf("show list: can't get files in %s dir: %w", fs.DirToday, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		if file.IsMultiline {
			cmd := tg.NewCmd(constants.CmdShowMultilineTask, []string{fs.DirToday, fs.Hash(file.Name)})
			btn = tg.NewBtn(txt.Emoji("👀", fs.UnsanitizeFilename(file.Title)), cmd)
		} else {
			cmd := tg.NewCmd(constants.CmdComplete, []string{fs.DirToday, fs.Hash(file.Name)})
			btn = tg.NewBtn(i18n.Emojify(fs.UnsanitizeFilename(file.Title)), cmd)
		}

		kb.AddRow(btn)
	}

	quickPanelBtns := b.quickPanelBtns()
	if len(quickPanelBtns) > 0 {
		quickPanelBtnsByRows := slice.Chunk(quickPanelBtns, quickBtnsPerRow)
		for _, row := range quickPanelBtnsByRows {
			kb.AddRow(row)
		}
	}

	msg := b.todayLabel()
	err = b.show(msg, &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show list: %w", err)
	}

	return nil
}

func (b *Bot) showLaterTasks(params []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirLater)
	if err != nil {
		return fmt.Errorf("show list: can't get files in %s dir: %w", fs.DirLater, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		if file.IsMultiline {
			cmd := tg.NewCmd(constants.CmdShowMultilineTask, []string{fs.DirLater, fs.Hash(file.Name)})
			btn = tg.NewBtn(txt.Emoji("👀", fs.UnsanitizeFilename(file.Title)), cmd)
		} else {
			cmd := tg.NewCmd(constants.CmdComplete, []string{fs.DirLater, fs.Hash(file.Name)})
			btn = tg.NewBtn(i18n.Emojify(fs.UnsanitizeFilename(file.Title)), cmd)
		}

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(i18n.StrBtnToday, tg.NewCmd(constants.CmdShowToday, nil)))

	msg := b.tr("⏳ Your tasks for later:")
	err = b.show(msg, &kb, tg.MarkupHTML)
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
	todayTasks := fs.ExcludePomodoro(fs.OnlyFiles(filesAndDirs))
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

func (b *Bot) showFiles(params []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("show files: can't get dirs: %w", err)
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
	shouldAddSeparator := len(dirs) > 0 && len(files) > 0
	if shouldAddSeparator {
		kb.AddRow(tg.NewBtn("-", tg.NewCmd(constants.CmdDoNothing, nil)))
	}

	files = fs.ExcludeConfig(fs.OnlyFiles(files))
	var fileBtns []tg.Btn
	for _, file := range files {
		cmd := tg.NewCmd(constants.CmdShowFile, []string{fs.DirRoot, fs.Hash(file.Name)})
		btn := tg.NewBtn(fmt.Sprintf("📄 %s", fs.UnsanitizeFilename(file.Title)), cmd)
		fileBtns = append(fileBtns, btn)
	}
	fileBtnsByRows := slice.Chunk(fileBtns, btnsPerRow)
	for _, row := range fileBtnsByRows {
		kb.AddRow(row)
	}

	kb.AddRow(tg.NewBtn(i18n.StrBtnToday, tg.NewCmd(constants.CmdShowToday, nil)))

	err = b.show(b.tr("📄 Your files:")+wideSpacer, &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show files: %w", err)
	}

	return nil
}

func (b *Bot) showChecklists(params []string) error {
	checklists, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}
	checklists = fs.OnlyChecklists(checklists)

	var kb tg.Keyboard
	for _, checklist := range checklists {
		cmd := tg.NewCmd(constants.CmdShowChecklist, []string{fs.Hash(checklist.Name)})
		btn := tg.NewBtn(checklist.Title, cmd)

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(b.tr("🏠 Today"), tg.NewCmd(constants.CmdShowToday, nil)))

	err = b.show(b.tr("☑️ Checklists"), &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}

	return nil
}

func (b *Bot) showPostpone(params []string) error {
	files, err := b.fs.FilesAndDirs(fs.DirToday)
	if err != nil {
		return fmt.Errorf("show postpone: can't get files in '%s' dir: %w", fs.DirToday, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		cmd := tg.NewCmd(constants.CmdPostpone, []string{fs.Hash(file.Name)})
		kb.AddRow(tg.NewBtn(file.Title, cmd))
	}

	kb.AddRow(tg.NewRow(
		tg.NewBtn(b.tr(constants.CmdShowRename), tg.NewCmd(constants.CmdShowRename, []string{})),
		tg.NewBtn(b.tr("OK"), tg.NewCmd(constants.CmdShowToday, []string{})),
	))

	err = b.show(b.tr("🦥 Select a task to postpone:"), &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show postpone: %w", err)
	}

	return nil
}

func (b *Bot) postpone(params []string) error {
	// TODO Remove input expectations if dir is not list
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

func (b *Bot) showRename(params []string) error {
	dir := fs.DirToday
	if len(params) > 0 {
		dir = params[0]
	}
	otherDir := fs.DirLater
	if dir == fs.DirLater {
		otherDir = fs.DirToday
	}

	files, err := b.fs.FilesAndDirs(dir)
	if err != nil {
		return fmt.Errorf("rename: can't get files in %s dir: %w", dir, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		cmd := tg.NewCmd(constants.CmdRenameFile, []string{dir, fs.Hash(file.Name)})
		btn = tg.NewBtn(txt.Emoji("👀", file.Title), cmd)

		kb.AddRow(btn)
	}

	kb.AddRow(tg.NewBtn(otherDir, tg.NewCmd(otherDir, []string{otherDir})))

	err = b.show(b.todayLabel(), &kb, tg.MarkupHTML)
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

	content, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("show rename: can't get content for %s: %w", filename, err)
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(dir, []string{dir}))),
	})

	cmd := tg.NewCmd(constants.CmdMove, []string{dir, filename, dir, "%s"})
	b.db.SetInputExpectation(b.userID, cmd)

	err = b.show(fmt.Sprintf("%s\n%s", fs.Title(filename), content), kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show rename: %w", err)
	}

	return nil
}

func (b *Bot) showStats(params []string) error {
	report, err := stats.TodayReport(b.fs, b.db, b.userID)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	err = b.show(report, nil, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show stats: %w", err)
	}

	return nil
}

func (b *Bot) showRead(params []string) error {
	return b.showChecklist([]string{fs.Hash(fs.DirRead)})
}

func (b *Bot) showWatch(params []string) error {
	return b.showChecklist([]string{fs.Hash(fs.DirWatch)})
}

func (b *Bot) showShop(params []string) error {
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
	content = txt.MarkdownToHtml(content)

	var moveToBtn tg.Btn
	btnLabel := i18n.StrBtnMoveToLater
	toDir := fs.DirLater
	if dir == fs.DirLater {
		btnLabel = i18n.StrBtnMoveToToday
		toDir = fs.DirToday
	}
	moveToBtn = tg.NewBtn(btnLabel, tg.NewCmd(constants.CmdMove, []string{dir, filenameHash, toDir}))

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(moveToBtn),
		tg.NewRow(
			tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(dir, []string{dir})),
			tg.NewBtn(i18n.StrBtnComplete, tg.NewCmd(constants.CmdComplete, []string{dir, filenameHash})),
		),
	})

	err = b.show(content, kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID(b.userID)
	if hasLastKeyboard {
		b.db.SetFilenameByMsgID(b.userID, msgID, filename)
		b.db.SetDirByMsgID(b.userID, msgID, dir)
	}

	return nil
}

func (b *Bot) showFile(params []string) error {
	dirHash := params[0]
	filenameHash := params[1]

	dir, err := b.fs.Unhash(fs.DirRoot, dirHash)
	if err != nil {
		return fmt.Errorf("show file: %w", err)
	}

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("show file: %w", err)
	}

	content, err := b.fs.Read(dir, filename)
	if err != nil {
		return fmt.Errorf("show file: : %w", err)
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBtnToday, tg.NewCmd(constants.CmdShowToday, nil))),
	})

	err = b.show(fmt.Sprintf("%s\n%s", fs.Title(filename), content), kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show file: %w", err)
	}

	msgID, hasLastKeyboard := b.db.LastKeyboardMsgID(b.userID)
	if hasLastKeyboard {
		b.db.SetFilenameByMsgID(b.userID, msgID, filename)
		b.db.SetDirByMsgID(b.userID, msgID, dir)
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
	items = items[max(0, len(items)-maxBtns):]

	kb := tg.NewKeyboard(nil)
	for _, item := range items {
		kb.AddRow(tg.NewBtn(item.Title, tg.NewCmd(constants.CmdCompleteChecklist, []string{dirHash, item.Hash})))
	}
	kb.AddRow(tg.NewBtn(i18n.StrBtnToday, tg.NewCmd(constants.CmdShowToday, nil)))

	err = b.show(fs.Title(checklist)+wideSpacer, kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	return nil
}

func (b *Bot) send(msg string) error {
	_, err := b.tg.Send(b.userID, msg, nil, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("start: can't send: %w", err)
	}
	return nil
}

func (b *Bot) showStart(params []string) error {
	return b.send("Welcome!")
}

func (b *Bot) move(params []string) error {
	// TODO Remove input expectations if dir is not list
	oldDirHash := params[0]
	oldFilenameHash := params[1]
	newDirHash := params[2]

	oldDir, err := b.fs.Unhash(fs.DirRoot, oldDirHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash old dir: %w", err)
	}

	filename, err := b.fs.Unhash(oldDir, oldFilenameHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash old filename: %w", err)
	}
	newFilename := filename
	if len(params) > 3 {
		newFilename = params[3]
	}

	newDir, err := b.fs.Unhash(fs.DirRoot, newDirHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash new dir %s: %w", newDir, err)
	}

	// TODO touch
	// TODO multiline
	err = b.fs.Rename(oldDir, filename, newDir, newFilename)
	if err != nil {
		return fmt.Errorf("move: can't move: %w", err)
	}

	return b.ShowTodayTasks(nil)
}

func (b *Bot) moveToNewDir(params []string) error {
	filenameHash := params[0]
	dir := params[1]

	err := b.fs.MakeDir(dir)
	if err != nil {
		return fmt.Errorf("move to new dir: %w", err)
	}

	return b.move([]string{fs.DirInbox, filenameHash, dir})
}

func (b *Bot) moveToExistingFile(params []string) error {
	// TODO Remove input expectations if dir is not list
	filenameHash := params[0]
	existingFilenameHash := params[1]

	if filenameHash == existingFilenameHash {
		return b.ShowTodayTasks(nil)
	}

	filename, err := b.fs.Unhash(fs.DirRoot, filenameHash)
	if err != nil {
		return fmt.Errorf("move to file: can't unhash new filename '%s': %w", filenameHash, err)
	}

	existingFilename, err := b.fs.Unhash(fs.DirRoot, existingFilenameHash)
	if err != nil {
		return fmt.Errorf("move to file: can't unhash doc '%s' in today: %w", filenameHash, err)
	}

	fileContent, err := b.fs.Read(fs.DirRoot, filename)
	if err != nil {
		return fmt.Errorf("move to file: can't read content of '%s': %w", filename, err)
	}
	fileContent = strings.TrimSpace(fileContent)
	if len(fileContent) == 0 {
		fileContent = fs.Title(filename)
	}

	existingContent, err := b.fs.Read(fs.DirRoot, existingFilename)
	if err != nil {
		return fmt.Errorf("move to file: can't get doc content of '%s': %w", existingFilename, err)
	}

	// We can tolerate this
	_ = b.fs.Del(fs.DirRoot, filename)

	header := fmt.Sprintf("### %s", now().Format("02.01.2006 Monday"))
	content := txt.InsertTextAfterHeader(existingContent, header, fileContent)

	err = b.fs.Write(fs.DirRoot, existingFilename, content)
	if err != nil {
		return fmt.Errorf("move to file: can't save file: %w", err)
	}

	return b.ShowTodayTasks(nil)
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

	if isMultiline && config.ShouldSplitChecklist(checklist) {
		content, err := b.fs.Read(fs.DirToday, filename)
		if err != nil {
			return fmt.Errorf("move to checklist: %w", err)
		}

		content = strings.TrimSpace(txt.NormNewLines(content))
		lines := strings.Split(content, "\n")
		for _, line := range lines {
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

	return b.ShowTodayTasks(nil)
}

func (b *Bot) moveToNewFile(params []string) error {
	filenameHash := params[0]
	filename := params[1]

	err := b.fs.Write(fs.DirRoot, txt.Ucfirst(filename), "")
	if err != nil {
		return fmt.Errorf("move to new file: can't create empty doc: %w", err)
	}

	return b.moveToExistingFile([]string{filenameHash, fs.Hash(filename)})
}

func (b *Bot) moveToNewChecklist(params []string) error {
	filenameHash := params[0]
	checklist := params[1]

	err := b.fs.Write(fs.DirRoot, txt.Ucfirst(checklist), "")
	if err != nil {
		return fmt.Errorf("move to new checklist: can't create empty doc: %w", err)
	}

	return b.moveToExistingFile([]string{filenameHash, fs.Hash(checklist)})
}

func (b *Bot) moveToJournal(params []string) error {
	dir := params[0]
	filenameHash := params[1]
	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't unhash filename: %w", err)
	}
	err = journal.AddRecord(b.fs, filename)
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't add note: %w", err)
	}

	err = b.fs.Del(dir, filename)
	if err != nil {
		return fmt.Errorf("failed to move to journal: can't delete note: %w", err)
	}
	return b.ShowTodayTasks(nil)
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

	// TODO multiline
	err = b.fs.Rename(dir, filename, fs.DirArchive, filename)
	if err != nil {
		return fmt.Errorf("complete: can't complete %s: %w", filename, err)
	}

	if dir == fs.DirToday && filename == fs.FilePomodoro {
		b.conf.AddToSchedule(filename, time.Now().Unix()+int64(b.conf.PomodoroDuration().Seconds()), "")
	}

	err = b.ShowTodayTasks(nil)
	if err != nil {
		return fmt.Errorf("copmlete: %w", err)
	}

	return nil
}

func (b *Bot) completeChecklist(params []string) error {
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

	err = b.fs.Rename(dir, filename, fs.DirArchive, filename)
	if err != nil {
		return fmt.Errorf("complete: can't complete %s: %w", filename, err)
	}

	return b.showChecklist([]string{dirHash})
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

	b.conf.AddToSchedule(filename, scheduleTime, cron)

	err = b.fs.Rename(fs.DirToday, filename, fs.DirLater, filename)
	if err != nil {
		return fmt.Errorf("schedule: can't rename file %s: %w", filename, err)
	}

	return b.ShowTodayTasks(nil)
}

func (b *Bot) delAllKeyboards() {
	var msgIDs []int
	mid, hasLastKeyboard := b.db.LastKeyboardMsgID(b.userID)
	if hasLastKeyboard {
		b.db.DelLastKeyboardMsgID(b.userID)
		msgIDs = append(msgIDs, mid)
	}

	// No worries if we fail - it will be cleaned up by the worker
	for _, msgID := range msgIDs {
		// If we fail to del - user would get a bunch
		// of keyboards in one chat, which is messy but not critical
		b.tg.Del(b.userID, msgID)
	}
}

// User-namespaced redis key
func (b *Bot) key(key string) string {
	return fmt.Sprintf("%s:%d", key, b.userID)
}

func (b *Bot) showChooseDay(params []string) error {
	filenameHash := params[0]

	kb, err := b.forADayKeyboard(filenameHash)
	if err != nil {
		return fmt.Errorf("choose day: %w", err)
	}

	err = b.show("choose your destiny", kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("choose day: %w", err)
	}

	return nil
}

func (b *Bot) forADayKeyboard(filenameHash string) (*tg.Keyboard, error) {
	newBtn := func(name, cron string) tg.Btn {
		return tg.NewBtn(name, tg.NewCmd(constants.CmdSchedule, []string{filenameHash, txt.I64(sched.Next(cron)), ""}))
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBtnRepeat, tg.NewCmd(constants.CmdShowRecurringKB, []string{filenameHash}))),
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

	for _, iAndj := range [][]int{{1, 7}, {9, 16}, {17, 24}, {25, 31}} {
		row := tg.NewRow()
		for i := iAndj[0]; i <= iAndj[1]; i++ {
			cron := fmt.Sprintf("0 0 %d * *", i)
			row = append(row, newBtn(txt.I64(int64(i)), cron))
		}
		kb.AddRow(row)
	}

	return kb, nil
}

func (b *Bot) showToFile(params []string) error {
	filenameHash := params[0]

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}

	err = b.fs.Rename(fs.DirToday, filename, fs.DirRoot, filename)
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}

	kb := tg.NewKeyboard(nil)
	dirBtns, err := b.toDirKeyboardButtons(filenameHash)
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}
	dirBtnsByRows := slice.Chunk(dirBtns, btnsPerRow)
	for _, row := range dirBtnsByRows {
		kb.AddRow(row)
	}

	fileBtns, err := b.toFileKeyboardButtons(filenameHash)
	if err != nil {
		return fmt.Errorf("to file dialog: %w", err)
	}
	shouldAddSeparator := len(dirBtns) > 0 && len(fileBtns) > 0
	if shouldAddSeparator {
		kb.AddRow(tg.NewBtn("Or choose a file:", tg.NewCmd(constants.CmdDoNothing, nil)))
	}
	fileBtnsByRows := slice.Chunk(fileBtns, btnsPerRow)
	for _, row := range fileBtnsByRows {
		kb.AddRow(row)
	}

	b.db.SetInputExpectation(b.userID, tg.NewCmd(constants.CmdMoveToNewDir, []string{filenameHash, "%s"}))

	err = b.show("🗂 Choose a dir or name a new one:", kb, tg.MarkupHTML)
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

	b.db.SetInputExpectation(b.userID, tg.NewCmd(constants.CmdMoveToNewChecklist, []string{filenameHash, "%s"}))

	err = b.show("choose your checklist", kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show to checklist: %w", err)
	}

	return nil
}

func (b *Bot) toFileKeyboardButtons(filenameHash string) ([]tg.Btn, error) {
	files, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return nil, fmt.Errorf("to doc keyboard: %w", err)
	}
	files = fs.OnlyFiles(files)
	if len(files) == 0 {
		return nil, nil
	}

	var buttons []tg.Btn
	newBtn := func(title, fileHash string) tg.Btn {
		title = fmt.Sprintf("%s %s", i18n.Emoji("file"), title)
		return tg.NewBtn(title, tg.NewCmd(constants.CmdMoveToFile, []string{filenameHash, fileHash}))
	}
	for _, file := range files {
		buttons = append(buttons, newBtn(file.Title, file.Hash))
	}

	return buttons, nil
}

func (b *Bot) toDirKeyboardButtons(filenameHash string) ([]tg.Btn, error) {
	newBtn := func(dir string) tg.Btn {
		emojifiedDir := fmt.Sprintf("%s %s", i18n.Emoji("dir"), dir)
		return tg.NewBtn(emojifiedDir, tg.NewCmd(constants.CmdMove, []string{fs.DirRoot, filenameHash, dir}))
	}

	dirs, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return nil, fmt.Errorf("to note keyboard: %w", err)
	}
	dirs = fs.OnlyNoteDirs(fs.OnlyDirs(dirs))

	var buttons []tg.Btn
	for _, dir := range dirs {
		buttons = append(buttons, newBtn(dir.Name))
	}

	return buttons, nil
}

func (b *Bot) toChecklistKeyboard(filenameHash string) (*tg.Keyboard, error) {
	newBtn := func(dir, title string) tg.Btn {
		return tg.NewBtn(title, tg.NewCmd(constants.CmdMoveToChecklist, []string{filenameHash, dir}))
	}

	dirs, err := b.fs.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return nil, fmt.Errorf("to checklist keyboard: %w", err)
	}
	// TODO handle case with zero folders (inline_keyboard is null), for all similar cases
	dirs = fs.OnlyChecklists(fs.OnlyDirs(dirs))

	kb := tg.NewKeyboard(nil)
	for _, dir := range dirs {
		kb.AddRow(newBtn(dir.Name, dir.Title))
	}

	return kb, nil
}

func (b *Bot) togglePomodoro(params []string) error {
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
		err := b.send(fmt.Sprintf("Pomodoro is stopped: no new \"%v\" tasks will appear automatially", fs.FilePomodoro))
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
		}
		return b.ShowTodayTasks(nil)
	}

	// Create Pomodoro task
	err = b.fs.Touch(fs.DirToday, fs.FilePomodoro)
	if err != nil {
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
		}
	}

	err = b.send(fmt.Sprintf("Pomodoro is run: you can see \"%v\" task in your %v folder. Once are ready to focus on something and start working, just complete this task."+
		" It will get back in %v to let you know that you worked enough and deserved a break. To stop it just use /%v comand again",
		fs.FilePomodoro, fs.DirToday, b.conf.PomodoroDuration(), constants.CmdPomodoro))
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
	}

	return b.ShowTodayTasks(nil)
}

func (b *Bot) showSettings(params []string) error {
	var kb tg.Keyboard
	kb.AddRow(tg.NewBtn(i18n.StrBtnQuickPanel, tg.NewCmd(constants.CmdConfigurePanel, nil)))
	kb.AddRow(tg.NewBtn(i18n.StrBtnToday, tg.NewCmd(constants.CmdShowToday, nil)))

	err := b.show("Settings:", &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("showSettings : %w", err)
	}

	return nil
}

func (b *Bot) showConfigureQuickPanel(params []string) error {
	var kb tg.Keyboard

	// Step 1. Append all buttons that are already chosen by user
	var usedBtns []string

	// We iterate through hardcoded panel to preserve order of buttons in UI
	for _, cmd := range b.conf.QuickPanelCmds() {
		for _, btn := range userconfig.QuickPanelAvailableBtns {
			if btn.Cmd != cmd {
				continue
			}

			name := fmt.Sprintf("%s %s %s", btn.Emoji, btn.Description, userconfig.QuickPanelDelButton)
			enabledCmd := tg.NewCmd(constants.CmdDelFromPonel, []string{btn.Cmd})
			kb.AddRow(tg.NewBtn(name, enabledCmd))
			usedBtns = append(usedBtns, cmd)
			break
		}
	}

	kb.AddRow(tg.NewBtn("-", tg.NewCmd(constants.CmdDoNothing, nil)))

	// Step 2. now, let's fill buttons that are not disabled...
	for _, btn := range userconfig.QuickPanelAvailableBtns {
		// Check if command is enabled
		btnUsed := false
		for _, usedBtn := range usedBtns {
			if btn.Cmd == usedBtn {
				btnUsed = true
			}
		}
		if btnUsed {
			continue
		}
		// Command is not enabled, so add it to disabled list
		name := fmt.Sprintf("%s %s %s", btn.Emoji, btn.Description, userconfig.QuickPanelAddButton)
		disabledCmd := tg.NewCmd(constants.CmdAddToPanel, []string{btn.Cmd})
		kb.AddRow(tg.NewBtn(name, disabledCmd))
	}

	kb.AddRow(tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(constants.CmdShowSettings, nil)))

	addBtn := userconfig.QuickPanelAddButton
	delBtn := userconfig.QuickPanelDelButton
	text := fmt.Sprintf("Configure quick panel (%s = add to panel, %s = to remove):", addBtn, delBtn)
	err := b.show(text, &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("configureQuickPanel : %w", err)
	}

	return nil
}

func (b *Bot) addToPanel(params []string) error {
	if len(params) == 0 {
		return fmt.Errorf("no params suplied to addToPanel")
	}

	// Search whether a command is valid
	found := false
	for _, btn := range userconfig.QuickPanelAvailableBtns {
		if btn.Cmd == params[0] {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("unknown command: %s", params[0])
	}

	if !b.conf.AddPanelButton(params[0]) {
		return fmt.Errorf("button already exists in user's prefs: %s", params[0])
	}
	b.showConfigureQuickPanel([]string{})

	return nil
}

func (b *Bot) delFromPanel(params []string) error {
	if len(params) == 0 {
		return fmt.Errorf("no params suplied to delFromPanel")
	}
	if !b.conf.DelPanelButton(params[0]) {
		return fmt.Errorf("button doesn't exist in user's prefs: %s", params[0])
	}

	b.showConfigureQuickPanel([]string{})
	return nil
}

func (b *Bot) showRecurringKeyBoard(params []string) error {
	filenameHash := params[0]

	newBtn := func(name, cron string) tg.Btn {
		return tg.NewBtn(name, tg.NewCmd(constants.CmdSchedule, []string{filenameHash, txt.I64(sched.Next(cron)), cron}))
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

	err := b.show("Configure schedule", kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("showRecuringKeyBoard : %w", err)
	}

	return nil
}

func extractPlainText(u UpdInterface) string {
	content := txt.EntitiesToMarkdown(u.MsgText(), u.MsgEntities())
	content = strings.TrimSpace(txt.NormNewLines(content))

	return txt.Ucfirst(content)
}

// func (b *Bot) getAngerEmoji(file fs.File) string {
// 	anger := string[]{"","🙄", "😕","😢","😭","🤬️"}
// 	index =

// }
