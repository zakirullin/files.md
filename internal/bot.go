package internal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/exp/slog"

	"zakirullin/dumpbot/i18n"
	"zakirullin/dumpbot/internal/db"
	"zakirullin/dumpbot/internal/fs"
	"zakirullin/dumpbot/internal/sched"
	"zakirullin/dumpbot/internal/stats"
	"zakirullin/dumpbot/internal/userconfig"
	"zakirullin/dumpbot/pkg/slice"
	"zakirullin/dumpbot/pkg/str"
	"zakirullin/dumpbot/pkg/tg"
)

var now = func() time.Time {
	return time.Now()
}

const (
	maxTitleLength         = 100
	inlineResultsCacheTime = 15 // seconds
	btnsPerRow             = 3
)

// TGInterface provides a simple interface to telegram API
type TGInterface interface {
	Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error)
	Edit(userID int64, msgID int, text string, kb *tg.Keyboard, markup string) error
	Del(userID int64, msgID int) error
	AnswerCallbackQuery(queryID string, text string) error
	AnswerInlineQuery(queryID string, results []interface{}, cacheTime int, offset string) error
}

// UpdInterface represents incoming user updates
type UpdInterface interface {
	MsgText() string
	UserID() int64
	Cmd() *tg.Cmd
	MsgEntities() []tgbotapi.MessageEntity
	IsForwarded() bool
	CallbackQueryID() (string, bool)
	InlineQueryID() (string, bool)
	InlineQuery() (string, bool)
}

// Bot provides commands that can be invoked by a user so to query
// server files and database. A user can also send all sort of things
// to bot (texts, photos) - in that case we'd save everything.
type Bot struct {
	userID int64
	tg     TGInterface
	fs     *fs.FS
	db     *db.DB
	conf   *userconfig.Config
}

func NewBot(userID int64, tg TGInterface, fs *fs.FS, db *db.DB, conf *userconfig.Config) *Bot {
	return &Bot{userID, tg, fs, db, conf}
}

// Reply to incoming text message or command (inline queries aren't supported yet)
func (b *Bot) Reply(u UpdInterface) error {
	if _, ok := u.InlineQueryID(); ok {
		return b.replyToInlineQuery(u)
	}

	cmd, err := b.extractCmd(u)
	if err != nil {
		return fmt.Errorf("extract cmd: %w", err)
	}
	if cmd != nil {
		if _, ok := u.CallbackQueryID(); !ok {
			b.delAllKeyboards()
		}

		handler, ok := b.handlers()[cmd.Name]
		if !ok {
			// TODO create error
			return errors.New(fmt.Sprintf("no such command %s", cmd.Name))
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

	if u.IsForwarded() {
		return b.saveForward(u)
	}

	return b.save(u)
}

// Commands and their handlers.
// Every handler accepts []string params
func (b *Bot) handlers() map[string]func([]string) error {
	return map[string]func([]string) error{
		// Direct user commands
		cmdShowStart:      b.showStart,
		cmdShowToday:      b.showToday,
		cmdShowLater:      b.showLater,
		cmdShowNotes:      b.showNotes,
		cmdShowDocs:       b.showDocs,
		cmdShowChecklists: b.showChecklists,
		cmdShowPostpone:   b.showPostpone,
		cmdShowRename:     b.showRename,
		cmdShowStats:      b.showStats,
		// Button's commands (callbacks)
		cmdRenameFile:         b.showRenameFile,
		cmdShowMultilineTask:  b.showTask,
		cmdShowDoc:            b.showDoc,
		cmdShowChecklist:      b.showChecklist,
		cmdShowChooseDay:      b.showChooseDay,
		cmdShowToNote:         b.showToNote,
		cmdShowToDoc:          b.showToDoc,
		cmdShowToChecklist:    b.showToChecklist,
		cmdMove:               b.move,
		cmdMoveToNewDir:       b.moveToNewDir,
		cmdMoveToDoc:          b.moveToDoc,
		cmdMoveToNewDoc:       b.moveToNewDoc,
		cmdMoveToChecklist:    b.moveToChecklist,
		cmdMoveToNewChecklist: b.moveToNewChecklist,
		cmdSchedule:           b.schedule,
		cmdComplete:           b.complete,
		cmdPostpone:           b.postpone,
		cmdPomodoro:           b.togglePomodoro,
		cmdShowRecurringKB:    b.showRecurringKeyBoard,
	}
}

func (b *Bot) extractCmd(u UpdInterface) (*tg.Cmd, error) {
	cmd := u.Cmd()
	if cmd != nil {
		return cmd, nil
	}

	// Input expectation is mostly used for renaming things
	cmd, err := b.db.InputExpectation(b.userID)
	if err != nil {
		return nil, fmt.Errorf("extract cmd: %w", err)
	}
	if cmd != nil {
		err = b.db.DelInputExpectation(b.userID)
		if err != nil {
			return nil, fmt.Errorf("extract cmd: %w", err)
		}

		for i, param := range cmd.Params {
			if param == "%s" {
				cmd.Params[i] = u.MsgText()
			}
		}

		return cmd, nil
	}

	return nil, nil
}

func (b *Bot) cmdsOnlyNotes() map[string]func([]string) error {
	return map[string]func([]string) error{
		cmdShowStart: b.showStart,
	}
}

func (b *Bot) allowedTextCmds() []string {
	return []string{
		cmdShowStart,
		cmdShowToday,
		cmdShowLater,
		cmdShowNotes,
		cmdShowPostpone,
		cmdShowDocs,
		cmdShowRename,
		cmdShowChecklists,
		cmdShowStats,
		//"help" TODO,
		//"err" TODO,
	}
}

func (b *Bot) save(u UpdInterface) error {
	msg := str.EntitiesToMarkdown(u.MsgText(), u.MsgEntities())
	msg = strings.TrimSpace(str.NormNewLines(msg))

	title, content, err := b.extractTitleAndContent(msg)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	filename := fs.Filename(title)
	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return b.showMove([]string{fs.Hash(filename)})
}

func (b *Bot) saveForward(u UpdInterface) error {
	msg := str.EntitiesToMarkdown(u.MsgText(), u.MsgEntities())
	msg = strings.TrimSpace(str.NormNewLines(msg))

	title, content, err := b.extractTitleAndContent(msg)
	if err != nil {
		return fmt.Errorf("save forward: %w", err)
	}
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

	files = fs.SortByCtime(fs.OnlyFiles(files))
	if len(files) > 0 {
		file := files[len(files)-1]
		fileWasCreatedRecently := (now().Unix() - file.Ctime) <= 2
		if fileWasCreatedRecently {
			filename = file.Name
			content = msg
		}
	}

	err = b.createOrAdd(fs.DirToday, filename, content)
	if err != nil {
		return fmt.Errorf("save forward: %w", err)
	}

	return b.showMove([]string{fs.Hash(filename)})
}

func (b *Bot) replyToInlineQuery(u UpdInterface) error {
	query, ok := u.InlineQuery()
	if !ok {
		return nil
	}

	matchedNotes, err := b.fs.SearchNotes(query)
	if err != nil {
		return fmt.Errorf("inline reply: %w", err)
	}

	var results []interface{}
	for id, note := range matchedNotes {
		results = append(results, tgbotapi.NewInlineQueryResultArticle(strconv.Itoa(id), note.Title, note.Title))
	}

	queryID, _ := u.InlineQueryID()
	err = b.tg.AnswerInlineQuery(queryID, results, inlineResultsCacheTime, "")
	if err != nil {
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
		existingContent, err := b.fs.Content(dir, filename)
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}

		content = fmt.Sprintf("%s\n%s", strings.TrimSpace(existingContent), content)
	}

	if err := b.fs.Put(fs.DirToday, filename, content); err != nil {
		return fmt.Errorf("create: %w", err)
	}

	return nil
}

func (b *Bot) extractTitleAndContent(msg string) (string, string, error) {
	if len(msg) == 0 {
		return "", "", fmt.Errorf("extract title: empty msg")
	}

	parts := strings.SplitN(msg, "\n", 2)

	title := str.Ucfirst(strings.TrimSpace(parts[0]))
	content := ""
	if len(parts) > 1 {
		content = strings.TrimSpace(parts[1])
	}
	if len(title) > maxTitleLength {
		if len(content) == 0 {
			content = title
		} else {
			content = fmt.Sprintf("%s\n\n%s", title, content)
		}
		title = str.Substr(title, 0, 100)
	}

	return title, content, nil
}

func (b *Bot) tr(str string, args ...any) string {
	str = i18n.Tr(str)

	return fmt.Sprintf(str, args...)
}

// Replace last message + keyboard with the new ones
// Or show the new one (in case of photo)
func (b *Bot) show(text string, kb *tg.Keyboard, markup string) error {
	mid, err := b.db.LastKeyboardMsgID(b.userID)
	if err != nil {
		return fmt.Errorf("show: %w", err)
	}

	if mid == nil {
		b.delAllKeyboards()

		mid, err := b.tg.Send(b.userID, text, kb, markup)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}

		err = b.db.SetLastKeyboardMsgID(b.userID, mid)
		if err != nil {
			return fmt.Errorf("show: %w", err)
		}

		return nil
	}

	return b.tg.Edit(b.userID, *mid, text, kb, markup)
}

func (b *Bot) showMove(params []string) error {
	filenameHash := params[0]

	availableCmds := map[string]tg.Cmd{
		i18n.StrForTomorrow: tg.NewCmd(cmdSchedule, []string{filenameHash, str.I64(sched.Tomorrow()), ""}),
		i18n.StrForLater:    tg.NewCmd(cmdMove, []string{fs.DirToday, filenameHash, "later"}),
		i18n.StrForDay:      tg.NewCmd(cmdShowChooseDay, []string{filenameHash}),
		i18n.StrToNote:      tg.NewCmd(cmdShowToNote, []string{filenameHash}),
		i18n.StrToChecklist: tg.NewCmd(cmdShowToChecklist, []string{filenameHash}),
		i18n.StrToDoc:       tg.NewCmd(cmdShowToDoc, []string{filenameHash}),
	}

	var btns []tg.Btn
	userCmdNames := b.conf.MoveToCmds()
	for _, userCmdName := range userCmdNames {
		cmd, ok := availableCmds[userCmdName]
		if !ok {
			// TODO rem unsupported cmd?
			continue
		}
		btns = append(btns, tg.NewBtn(userCmdName, cmd))
	}

	var kb tg.Keyboard
	userBtnsByRows := slice.Chunk(btns, btnsPerRow)
	for _, row := range userBtnsByRows {
		kb.AddRow(row)
	}
	kb.AddRow(tg.NewBtn(i18n.StrBtnGoToToday, tg.NewCmd(cmdShowToday, nil)))

	b.delAllKeyboards()

	err := b.show(b.tr("Task added for <b>today</b>!"), &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}

	return nil
}

// TODO separate today and later list
func (b *Bot) showList(params []string) error {
	dir := fs.DirToday
	if len(params) > 0 {
		dir = params[0]
	}
	oppositeDir := fs.DirLater
	oppositeLabel := i18n.StrBtnLater
	if dir == fs.DirLater {
		oppositeDir = fs.DirToday
		oppositeLabel = i18n.StrBtnToday
	}

	files, err := b.fs.FilesAndDirs(dir)
	if err != nil {
		return fmt.Errorf("show list: can't get files in %s dir: %w", dir, err)
	}

	var kb tg.Keyboard
	for _, file := range files {
		var btn tg.Btn
		if file.IsMultiline {
			cmd := tg.NewCmd(cmdShowMultilineTask, []string{dir, fs.Hash(file.Name)})
			btn = tg.NewBtn(str.Emoji("👀", file.Title), cmd)
		} else {
			cmd := tg.NewCmd(cmdComplete, []string{dir, fs.Hash(file.Name)})
			btn = tg.NewBtn(i18n.Emojify(file.Title), cmd)
		}

		kb.AddRow(btn)
	}

	kb.AddRow(tg.NewBtn(oppositeLabel, tg.NewCmd(oppositeDir, []string{oppositeDir})))

	var msg string
	if dir == fs.DirToday {
		msg, err = b.todayLabel()
		if err != nil {
			msg = b.tr("🏠 Tasks for today:")
			return err
		}
	} else {
		msg = b.tr("⏳ Your tasks for later:")
	}

	err = b.show(msg, &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show list: %w", err)
	}

	return nil
}

func (b *Bot) showNotes(params []string) error {
	dirs, err := b.fs.FilesAndDirs("")
	if err != nil {
		return fmt.Errorf("show notes: can't get dirs: %w", err)
	}
	dirs = fs.OnlyNotes(fs.OnlyDirs(dirs))

	var kb tg.Keyboard
	for _, dir := range dirs {
		cmd := tg.NewCmd(cmdComplete, []string{dir.Name, fs.Hash(dir.Name)})
		btn := tg.NewBtn(dir.Title, cmd)

		kb.AddRow(btn)
	}

	kb.AddRow(tg.NewBtn(fs.DirToday, tg.NewCmd(fs.DirToday, nil)))

	err = b.show("notes:", &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show notes: %w", err)
	}

	return nil
}

func (b *Bot) showDocs(params []string) error {
	files, err := b.fs.FilesAndDirs("")
	if err != nil {
		return fmt.Errorf("show docs: can't get dirs: %w", err)
	}
	files = fs.OnlyFiles(files)

	var kb tg.Keyboard
	for _, file := range files {
		cmd := tg.NewCmd(cmdShowDoc, []string{fs.Hash(file.Name)})
		btn := tg.NewBtn(file.Title, cmd)

		kb.AddRow(btn)
	}

	kb.AddRow(tg.NewBtn(b.tr("Back to docs"), tg.NewCmd(cmdShowDocs, nil)))

	err = b.show(b.tr("📝 Your docs:"), &kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show docs: %w", err)
	}

	return nil
}

func (b *Bot) showChecklists(params []string) error {
	checklists, err := b.fs.FilesAndDirs("")
	if err != nil {
		return fmt.Errorf("show checklists: %w", err)
	}
	checklists = fs.OnlyChecklists(checklists)

	var kb tg.Keyboard
	for _, checklist := range checklists {
		cmd := tg.NewCmd(cmdShowChecklist, []string{fs.Hash(checklist.Name)})
		btn := tg.NewBtn(checklist.Title, cmd)

		kb.AddRow(btn)
	}
	kb.AddRow(tg.NewBtn(b.tr("🏠 Today"), tg.NewCmd(cmdShowToday, nil)))

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
		cmd := tg.NewCmd(cmdPostpone, []string{fs.Hash(file.Name)})
		kb.AddRow(tg.NewBtn(file.Title, cmd))
	}

	kb.AddRow(tg.NewRow(
		tg.NewBtn(b.tr(cmdShowRename), tg.NewCmd(cmdShowRename, []string{})),
		tg.NewBtn(b.tr("OK"), tg.NewCmd(cmdShowToday, []string{})),
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
		cmd := tg.NewCmd(cmdRenameFile, []string{dir, fs.Hash(file.Name)})
		btn = tg.NewBtn(str.Emoji("👀", file.Title), cmd)

		kb.AddRow(btn)
	}

	kb.AddRow(tg.NewBtn(otherDir, tg.NewCmd(otherDir, []string{otherDir})))

	msg, err := b.todayLabel()
	if err != nil {
		// TODO fix
		msg = b.tr("Tasks for today:")
		return err
	}

	err = b.show(msg, &kb, tg.MarkupHTML)
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

	content, err := b.fs.Content(dir, filename)
	if err != nil {
		return fmt.Errorf("show rename: can't get content for %s: %w", filename, err)
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(dir, []string{dir}))),
	})

	err = b.db.SetInputExpectation(b.userID, tg.NewCmd(cmdMove, []string{dir, filename, dir, "%s"}))
	if err != nil {
		return fmt.Errorf("show rename: can't set input expectation: %w", err)
	}

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

func (b *Bot) showTask(params []string) error {
	dir := params[0]
	filenameHash := params[1]

	filename, err := b.fs.Unhash(dir, filenameHash)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	content, err := b.fs.Content(dir, filename)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	var moveToBtn tg.Btn
	if dir == fs.DirToday {
		moveToBtn = tg.NewBtn(i18n.StrBtnMoveToLater, tg.NewCmd(cmdMove, []string{dir, filenameHash, fs.DirLater}))
	} else {
		moveToBtn = tg.NewBtn(i18n.StrBtnMoveToToday, tg.NewCmd(cmdMove, []string{dir, filenameHash, fs.DirToday}))
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(moveToBtn),
		tg.NewRow(
			tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(dir, []string{dir})),
			tg.NewBtn(i18n.StrBtnComplete, tg.NewCmd(cmdComplete, []string{dir, filenameHash})),
		),
	})

	err = b.show(fmt.Sprintf("%s\n%s", fs.Title(filename), content), kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show task: %w", err)
	}

	return nil
}

func (b *Bot) showDoc(params []string) error {
	filenameHash := params[0]

	filename, err := b.fs.Unhash("", filenameHash)
	if err != nil {
		return fmt.Errorf("show doc: %w", err)
	}

	content, err := b.fs.Content("", filename)
	if err != nil {
		return fmt.Errorf("show doc: : %w", err)
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(cmdShowDocs, nil))),
	})

	err = b.show(fmt.Sprintf("%s\n%s", fs.Title(filename), content), kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show doc: %w", err)
	}

	return nil
}

func (b *Bot) showChecklist(params []string) error {
	checklistHash := params[0]

	checklist, err := b.fs.Unhash("", checklistHash)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	items, err := b.fs.FilesAndDirs(checklist)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	kb := tg.NewKeyboard(nil)
	for _, item := range items {
		kb.AddRow(tg.NewBtn(item.Title, tg.NewCmd(cmdComplete, []string{})))
	}
	kb.AddRow(tg.NewRow(tg.NewBtn(i18n.StrBtnBack, tg.NewCmd(cmdShowDocs, nil))))

	err = b.show(fs.Title(checklist), kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show checklist: %w", err)
	}

	return nil
}

func (b *Bot) showToday(params []string) error {
	return b.showList([]string{fs.DirToday})
}

func (b *Bot) showLater(params []string) error {
	return b.showList([]string{fs.DirLater})
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

	oldDir, err := b.fs.Unhash("", oldDirHash)
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

	newDir, err := b.fs.Unhash("", newDirHash)
	if err != nil {
		return fmt.Errorf("move: can't unhash new dir %s: %w", newDir, err)
	}

	// TODO touch
	// TODO multiline
	err = b.fs.Rename(oldDir, filename, newDir, newFilename)
	if err != nil {
		return fmt.Errorf("move: can't move: %w", err)
	}

	return b.showList(nil)
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

func (b *Bot) moveToDoc(params []string) error {
	// TODO Remove input expectations if dir is not list
	filenameHash := params[0]
	docHash := params[1]

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("move to doc: can't unhash new filename '%s': %w", filenameHash, err)
	}

	doc, err := b.fs.Unhash("", docHash)
	if err != nil {
		return fmt.Errorf("move to doc: can't unhash doc '%s' in today: %w", filenameHash, err)
	}

	fileContent, err := b.fs.RestoreText(fs.DirToday, filename)
	if err != nil {
		return fmt.Errorf("move to dc: can't restore file content of '%s': %w", filename, err)
	}

	// We can tolerate this
	_ = b.fs.Del(fs.DirToday, filename)

	docContent, err := b.fs.Content("", doc)
	if err != nil {
		return fmt.Errorf("move to doc: can't get doc content of '%s': %w", doc, err)
	}
	docContent = strings.TrimSpace(docContent)
	if len(docContent) > 0 {
		docContent += "\n"
	}
	docContent += fileContent

	err = b.fs.Put("", doc, docContent)
	if err != nil {
		return fmt.Errorf("move to doc: can't save file: %w", err)
	}

	return b.showToday(nil)
}

func (b *Bot) moveToChecklist(params []string) error {
	filenameHash := params[0]
	checklistHash := params[1]

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("move to checkilst: %w", err)
	}

	checklist, err := b.fs.Unhash("", checklistHash)
	if err != nil {
		return fmt.Errorf("move to checklist: %w", err)
	}

	isMultiline, err := b.fs.IsMultiline(fs.DirToday, filename)
	if err != nil {
		return fmt.Errorf("move to checklist: %w", err)
	}

	if isMultiline && shouldSplitChecklist(checklist) {
		text, err := b.fs.RestoreText(fs.DirToday, filename)
		if err != nil {
			return fmt.Errorf("move to checklist: %w", err)
		}

		text = strings.TrimSpace(str.NormNewLines(text))
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			err = b.fs.Put(checklist, fs.Filename(line), "")
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

	return b.showToday(nil)
}

func (b *Bot) moveToNewDoc(params []string) error {
	filenameHash := params[0]
	doc := params[1]

	err := b.fs.Put("", str.Ucfirst(doc), "")
	if err != nil {
		return fmt.Errorf("move to doc: can't create empty doc: %w", err)
	}

	return b.moveToDoc([]string{filenameHash, fs.Hash(doc)})
}

func (b *Bot) moveToNewChecklist(params []string) error {
	filenameHash := params[0]
	doc := params[1]

	err := b.fs.Put("", str.Ucfirst(doc), "")
	if err != nil {
		return fmt.Errorf("move to doc: can't create empty doc: %w", err)
	}

	return b.moveToDoc([]string{filenameHash, fs.Hash(doc)})
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
	err = b.fs.Rename(dir, filename, fs.DirTrash, filename)
	if err != nil {
		return fmt.Errorf("complete: can't complete %s: %w", filename, err)
	}

	if dir == fs.DirToday && filename == fs.FilePomodoro {
		err = b.db.AddToSchedule(b.userID, filename, time.Now().Unix()+int64(b.conf.PomodoroDuration().Seconds()), "")
		if err != nil {
			return fmt.Errorf("complete: can't add pomodoro task to schedule: %w", err)
		}
	}

	err = b.showList(nil)
	if err != nil {
		return fmt.Errorf("copmlete: %w", err)
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

	err = b.db.AddToSchedule(b.userID, filename, scheduleTime, cron)
	if err != nil {
		return fmt.Errorf("schedule: can't save schedule for %s: %w", filename, err)
	}

	err = b.fs.Rename(fs.DirToday, filename, fs.DirLater, filename)
	if err != nil {
		return fmt.Errorf("schedule: can't rename file %s: %w", filename, err)
	}

	return b.showList(nil)
}

func (b *Bot) delAllKeyboards() {
	var msgIDs []int
	mid, _ := b.db.LastKeyboardMsgID(b.userID)
	if mid != nil {
		_ = b.db.DelLastKeyboardMsgID(b.userID)
		msgIDs = append(msgIDs, *mid)
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
		return tg.NewBtn(name, tg.NewCmd(cmdSchedule, []string{filenameHash, str.I64(sched.Next(cron)), ""}))
	}

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn("Repeat the task", tg.NewCmd(cmdShowRecurringKB, []string{filenameHash}))),
		tg.NewRow(
			newBtn("Mn", "0 0 * * 1"),
			newBtn("Tu", "0 0 * * 2"),
			newBtn("Wd", "0 0 * * 3"),
			newBtn("Th", "0 0 * * 4"),
		),
		tg.NewRow(
			newBtn("Fr", "0 0 * * 5"),
			newBtn("St", "0 0 * * 6"),
			newBtn("Sn", "0 0 * * 0"),
		),
	})

	for _, iAndj := range [][]int{{1, 7}, {9, 16}, {17, 24}, {25, 31}} {
		row := tg.NewRow()
		for i := iAndj[0]; i <= iAndj[1]; i++ {
			cron := fmt.Sprintf("0 0 %d * *", i)
			row = append(row, newBtn(str.I64(int64(i)), cron))
		}
		kb.AddRow(row)
	}

	return kb, nil
}

func (b *Bot) showToNote(params []string) error {
	filenameHash := params[0]

	filename, err := b.fs.Unhash(fs.DirToday, filenameHash)
	if err != nil {
		return fmt.Errorf("show to note: %w", err)
	}

	err = b.fs.Rename(fs.DirToday, filename, fs.DirInbox, filename)
	if err != nil {
		return fmt.Errorf("show to note: %w", err)
	}

	kb, err := b.toNoteKeyboard(filenameHash)
	if err != nil {
		return fmt.Errorf("show to note: %w", err)
	}

	err = b.db.SetInputExpectation(b.userID, tg.NewCmd(cmdMoveToNewDir, []string{filenameHash, "%s"}))
	if err != nil {
		return fmt.Errorf("show to note: %w", err)
	}

	err = b.show("choose your destiny", kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show to note: %w", err)
	}

	return nil
}

func (b *Bot) showToDoc(params []string) error {
	filenameHash := params[0]

	kb, err := b.toDocKeyboard(filenameHash)
	if err != nil {
		return fmt.Errorf("show to doc: can't get keyboard: %w", err)
	}

	err = b.db.SetInputExpectation(b.userID, tg.NewCmd(cmdMoveToNewDoc, []string{filenameHash, "%s"}))
	if err != nil {
		return fmt.Errorf("show to doc: can't set input expectation: %w", err)
	}

	err = b.show(b.tr("📝 Choose a doc or name a new one:"), kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show to doc: %w", err)
	}

	return nil
}

func (b *Bot) showToChecklist(params []string) error {
	filenameHash := params[0]

	kb, err := b.toChecklistKeyboard(filenameHash)
	if err != nil {
		return fmt.Errorf("show to checklist: can't get keyboard: %w", err)
	}

	err = b.db.SetInputExpectation(b.userID, tg.NewCmd(cmdMoveToNewChecklist, []string{filenameHash, "%s"}))
	if err != nil {
		return fmt.Errorf("show to checklist: %w", err)
	}

	err = b.show("choose your checklist", kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("show to checklist: %w", err)
	}

	return nil
}

func (b *Bot) toDocKeyboard(filenameHash string) (*tg.Keyboard, error) {
	files, err := b.fs.FilesAndDirs("")
	if err != nil {
		return nil, fmt.Errorf("to doc keyboard: %w", err)
	}
	files = fs.OnlyFiles(files)
	if len(files) == 0 {
		return nil, nil
	}

	newBtn := func(title, docHash string) tg.Btn {
		return tg.NewBtn(title, tg.NewCmd(cmdMoveToDoc, []string{filenameHash, docHash}))
	}
	kb := tg.NewKeyboard(nil)
	for _, file := range files {
		kb.AddRow(newBtn(file.Title, file.Hash))
	}

	return kb, nil
}

func (b *Bot) toNoteKeyboard(filenameHash string) (*tg.Keyboard, error) {
	newBtn := func(dir string) tg.Btn {
		return tg.NewBtn(dir, tg.NewCmd(cmdMove, []string{fs.DirInbox, filenameHash, dir}))
	}

	dirs, err := b.fs.FilesAndDirs("")
	if err != nil {
		return nil, fmt.Errorf("to note keyboard: %w", err)
	}
	dirs = fs.OnlyNotes(fs.OnlyDirs(dirs))

	kb := tg.NewKeyboard(nil)
	for _, dir := range dirs {
		kb.AddRow(newBtn(dir.Name))
	}

	return kb, nil
}

func (b *Bot) toChecklistKeyboard(filenameHash string) (*tg.Keyboard, error) {
	newBtn := func(dir, title string) tg.Btn {
		return tg.NewBtn(title, tg.NewCmd(cmdMoveToChecklist, []string{filenameHash, dir}))
	}

	dirs, err := b.fs.FilesAndDirs("")
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

func (b *Bot) todayLabel() (string, error) {
	tasks, err := b.fs.FilesAndDirs(cmdShowToday)
	if err != nil {
		return "", fmt.Errorf("today label: %w", err)
	}
	tasks = fs.ExcludePomodoro(tasks)
	todo := len(tasks)

	hasPomodoro, err := b.fs.Exists("today", fs.FilePomodoro)
	if err != nil {
		return "", fmt.Errorf("today label: can't get pomodoro task's dir: %w", err)
	}

	// TODO add short labels
	icons := []string{"🌴"}
	label := "You don't have any tasks!"
	if todo > 0 {
		label = b.tr("<b>%d</b> left", todo)
		icons = nil
	}

	if hasPomodoro {
		icons = append([]string{"🍅"}, icons...)
	}

	if len(icons) > 0 {
		icons = append(icons, " ")
	}

	return strings.Join(icons, "") + label, nil
}

func (b *Bot) togglePomodoro(_ []string) error {
	// Check if Pomodoro is already running
	hasPomodoroInToday, err := b.fs.Exists(fs.DirToday, fs.FilePomodoro)
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to check if pomodoro is already running %w", err)
	}
	hasPomodoroInTrash, err := b.fs.Exists(fs.DirTrash, fs.FilePomodoro)
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
		err = b.fs.Del(fs.DirTrash, fs.FilePomodoro)
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to delete pomodoro file: %w", err)
		}
	}

	if hasPomodoroInToday || hasPomodoroInTrash {
		err := b.send(fmt.Sprintf("Pomodoro is stopped: no new \"%v\" tasks will appear automatially", fs.FilePomodoro))
		if err != nil {
			return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
		}
		return b.showToday(nil)
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
		fs.FilePomodoro, fs.DirToday, b.conf.PomodoroDuration(), cmdPomodoro))
	if err != nil {
		return fmt.Errorf("toggle pomodoro: failed to show pomodoro hint message %w", err)
	}

	return b.showToday(nil)
}

func (b *Bot) showRecurringKeyBoard(params []string) error {
	filenameHash := params[0]

	newBtn := func(name, cron string) tg.Btn {
		return tg.NewBtn(name, tg.NewCmd(cmdSchedule, []string{filenameHash, str.I64(sched.Next(cron)), cron}))
	}

	kb := tg.NewKeyboard([]tg.Row{
		// Cron format: Minute Hour DayOfMonth Month DayOfWeek
		tg.NewRow(
			newBtn("🏭 Week days", "0 0 * * 1-5"),
			newBtn("☀️ Every day", "0 0 * * 1-5"),
		),
		tg.NewRow(
			newBtn("1️⃣Mn", "0 0 * * 1"),
			newBtn("2️⃣Tu", "0 0 * * 2"),
			newBtn("3️⃣Wd", "0 0 * * 3"),
			newBtn("4️⃣Th", "0 0 * * 4"),
		),
		tg.NewRow(
			newBtn("5️⃣😊Fr", "0 0 * * 5"),
			newBtn("6️⃣😃St", "0 0 * * 6"),
			newBtn("7️⃣☀️Sn", "0 0 * * 0"),
		),
	})

	for week := 0; week < 4; week++ {
		row := tg.NewRow()
		for day := 1; day < 8; day++ {
			i := week*7 + day
			cron := fmt.Sprintf("0 0 %d * *", i)
			row = append(row, newBtn(str.I64(int64(i)), cron))
		}
		kb.AddRow(row)
	}

	err := b.show("Configure schedule", kb, tg.MarkupHTML)
	if err != nil {
		return fmt.Errorf("showRecuringKeyBoard : %w", err)
	}

	return nil
}
