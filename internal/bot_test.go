package internal

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/userconfig"

	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/tg/fake"
)

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}
}

func TestSaveFromTextMsg(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpd(-1, "New task"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)

	content, err := bot.fs.Read("today", "New task.md")
	r.NoError(err)
	r.Empty(content)
}

func TestSaveFromLongTextMsg(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpd(-1, strings.Repeat("a", 101)))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	filename := fmt.Sprintf("A%s....md", strings.Repeat("a", 99))
	r.Len(tasks, 1)
	r.Equal(filename, tasks[0].Name)

	content, err := bot.fs.Read("today", filename)
	r.NoError(err)
	r.Equal("A"+strings.Repeat("a", 100), content)
}

func TestSaveFromTextMsgWithSanitize(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpd(-1, "New task/"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task{|}.md", tasks[0].Name)

	content, err := bot.fs.Read("today", "New task{|}.md")
	r.NoError(err)
	r.Equal("New task/", content)

	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil)))
	r.NoError(err)

	r.Equal("<b>1</b> left"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("👀 New task/", tg.NewCmd("task", []string{"today", "cd59b9e6546"})),
	},
	), tgram.SentKeyboard)
}

func TestAddMultilineTaskToToday(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpd(-1, "New task\nContent"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.True(tasks[0].IsMultiline)

	content, err := bot.fs.Read("today", "New task.md")
	r.NoError(err)
	r.Equal("New task\nContent", content)
}

func TestAddTaskWithSpecCharsToToday(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpd(-1, "New task\nUrl! http://g.com (Also_text] ##header\n-item1\n-item2\n1+1=2"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.True(tasks[0].IsMultiline)

	content, err := bot.fs.Read("today", "New task.md")
	r.NoError(err)
	r.Equal("New task\nUrl! http://g.com (Also\\_text] ##header\n-item1\n-item2\n1+1=2", content)
}

func TestSaveFromRegularReply(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "Existing file.md", "Existing content")
	r.NoError(err)

	tgram := fake.NewTG()
	database := db.NewFakeDB()
	database.SetDirByMsgID(-1, 255, "today")
	database.SetFilenameByMsgID(-1, 255, "Existing file.md")
	bot := NewBot(-1, tgram, userFS, database, &userconfig.DefaultConfig)

	upd := fake.NewUpd(-1, "Line")
	upd.ReplyToMessageID = 255
	err = bot.Answer(upd)
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)
	r.Len(files, 1)

	content, err := bot.fs.Read("today", "Existing file.md")
	r.NoError(err)
	r.Equal("### 11.08.2024 Sunday\nLine\nExisting content", content)
}

func TestSaveFromPhotoWithCaption(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	upd := fake.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Caption"
	err = bot.Answer(upd)
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(files, 1)
	r.Equal("Caption.md", files[0].Name)
	r.True(files[0].IsMultiline)

	content, err := bot.fs.Read("today", "Caption.md")
	r.NoError(err)
	r.Equal("![[../img/tg_PHOTO_ID|center|400]]\nCaption", content)
}

func TestSaveFromPhotoWithLongCaption(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	upd := fake.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = strings.Repeat("a", 101)
	err = bot.Answer(upd)
	r.NoError(err)

	content, err := bot.fs.Read("today", fmt.Sprintf("A%s....md", strings.Repeat("a", 99)))
	r.NoError(err)
	r.Equal(fmt.Sprintf("![[../img/tg_PHOTO_ID|center|400]]\nA%s", strings.Repeat("a", 100)), content)
}

func TestSaveFromPhotoWithSanitizedCaption(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	upd := fake.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Caption/"
	err = bot.Answer(upd)
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(files, 1)
	r.Equal("Caption{|}.md", files[0].Name)
	r.True(files[0].IsMultiline)

	content, err := bot.fs.Read("today", "Caption{|}.md")
	r.NoError(err)
	r.Equal("![[../img/tg_PHOTO_ID|center|400]]\nCaption/", content)
}

func TestSaveFromPhotoWithoutCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	upd := fake.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	err = bot.Answer(upd)
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(files, 1)
	r.Equal("Img 11.08.24 09꞉54.md", files[0].Name)
	r.True(files[0].IsMultiline)

	content, err := bot.fs.Read("today", "Img 11.08.24 09꞉54.md")
	r.NoError(err)
	r.Equal("![[../img/tg_PHOTO_ID|center|400]]", content)
}

func TestSaveFromReplyPhotoWithCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "Existing file.md", "Existing content")
	r.NoError(err)

	tgram := fake.NewTG()

	database := db.NewFakeDB()
	database.SetDirByMsgID(-1, 255, "today")
	database.SetFilenameByMsgID(-1, 255, "Existing file.md")
	bot := NewBot(-1, tgram, userFS, database, &userconfig.DefaultConfig)

	upd := fake.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Caption"
	upd.ReplyToMessageID = 255
	err = bot.Answer(upd)
	r.NoError(err)

	content, err := bot.fs.Read("today", "Existing file.md")
	r.NoError(err)
	r.Equal("### 11.08.2024 Sunday\n![[../img/tg_PHOTO_ID|center|400]]\nCaption\nExisting content", content)
}

func TestAddTaskToLater(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.CreateUserDirs())

	err = userFS.Write("today", "First task.md", "")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("mv", []string{"today", "0824149b387", "later"})))
	r.NoError(err)

	todayTasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)
	r.Len(todayTasks, 0)

	laterTasks, err := bot.fs.FilesAndDirs("later")
	r.NoError(err)
	r.Len(laterTasks, 1)
	r.Equal("First task.md", laterTasks[0].Name)
}

func TestCompleteTask(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write("today", "First task.md", "")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("c", []string{"today", "0824149b387"})))
	r.NoError(err)

	todayTasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)
	r.Len(todayTasks, 0)

	completedTasks, err := bot.fs.FilesAndDirs("archive")
	r.NoError(err)
	r.Len(completedTasks, 1)
	r.Equal("First task.md", completedTasks[0].Name)
}

func TestToday(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "First task.md", "")
	r.NoError(err)
	err = userFS.Write("today", "Second task", "")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil)))
	r.NoError(err)

	r.Equal("<b>2</b> left"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("c", []string{"today", "0824149b387"})),
		tg.NewBtn("🥈 Second task", tg.NewCmd("c", []string{"today", "2940ad40402"})),
	},
	), tgram.SentKeyboard)
}

func TestLater(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("later", "First task.md", "")
	r.NoError(err)
	err = userFS.Write("later", "Second task", "")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("later", nil)))
	r.NoError(err)

	r.Equal("⏳ Your tasks for later:", tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("c", []string{"later", "0824149b387"})),
		tg.NewBtn("🥈 Second task", tg.NewCmd("c", []string{"later", "2940ad40402"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil)),
	},
	), tgram.SentKeyboard)
}

func TestTodayQuickMenuFilled(t *testing.T) {
	cfg := &userconfig.Config{}
	cfg.AddPanelButton("files")
	cfg.AddPanelButton("checklists")
	cfg.AddPanelButton("postpone")
	bot, tgram, r := makeBot(t, cfg)
	err := bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil)))
	r.NoError(err)
	r.Equal("<b>1</b> left"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("c", []string{"today", "0824149b387"})),
		tg.NewRow(
			tg.NewBtn("📄", tg.NewCmd("files", []string{})),
			tg.NewBtn("☑️", tg.NewCmd("checklists", []string{})),
			tg.NewBtn("🦥", tg.NewCmd("postpone", []string{})),
		),
	},
	), tgram.SentKeyboard)
}

func TestTodayWithMultilineTasks(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "First task.md", "content")
	r.NoError(err)
	err = userFS.Write("today", "Second task", "")
	r.NoError(err)

	tgram := fake.NewTG()

	upd := fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil))
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(upd)
	r.NoError(err)

	r.Equal("<b>2</b> left"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("👀 First task", tg.NewCmd("task", []string{"today", "0824149b387"})),
		tg.NewBtn("🥈 Second task", tg.NewCmd("c", []string{"today", "2940ad40402"})),
	},
	), tgram.SentKeyboard)
}

// func TestFiles(t *testing.T) {
// 	r := require.New(t)

// 	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
// 	r.NoError(err)
// 	err = userFS.CreateUserDirs()
// 	r.NoError(err)
// 	err = userFS.Write("", "Doc1.md", "")
// 	r.NoError(err)
// 	err = userFS.Write("", "Doc2.md", "")
// 	r.NoError(err)

// 	redis, err := miniredis.Run()
// 	r.NoError(err)
// 	defer redis.Close()

// 	tgram := fake.NewTG()

// 	bot := NewBot(-1, tgram, userFS,db.NewFakeDB(), &userconfig.DefaultConfig)
// 	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("files", nil)))
// 	r.NoError(err)

// 	r.Equal("📝 Your docs:", tgram.SentText)
// 	r.Equal(tg.NewKeyboard([]tg.Row{
// 		tg.NewBtn("Doc1", tg.NewCmd("file", []string{fs.DirRoot, "c1253521ac7"})),
// 		tg.NewBtn("Doc2", tg.NewCmd("file", []string{fs.DirRoot, "64572c3093f"})),
// 		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil)),
// 	},
// 	), tgram.SentKeyboard)
// }

func TestChecklists(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)
	err = userFS.MakeDir("-checklist2-")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("checklists", nil)))
	r.NoError(err)

	r.Equal("☑️ Checklists", tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Checklist1", tg.NewCmd("checklist", []string{"8d2335b5ff3"})),
		tg.NewBtn("Checklist2", tg.NewCmd("checklist", []string{"8d3625e2e84"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil)),
	},
	), tgram.SentKeyboard)
}

func TestAddSingleItemToChecklist(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)
	err = userFS.Write("today", "Item.md", "")
	r.NoError(err)

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_chk", []string{"7b72407ca70", "-checklist1-"})))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("Item.md", files[0].Name)

	files, err = userFS.FilesAndDirs("today")
	r.NoError(err)
	r.Len(files, 0)
}

func TestAddMultipleItemsToChecklist(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)
	err = userFS.Write("today", "Item.md", "item\nitem2\nitem3\n\n")
	r.NoError(err)

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_chk", []string{"7b72407ca70", "-checklist1-"})))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Len(files, 3)
	r.ElementsMatch([]string{"Item.md", "Item2.md", "Item3.md"}, []string{files[0].Name, files[1].Name, files[2].Name})
}

func TestShowChecklist(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)
	err = userFS.Write("-checklist1-", "Item.md", "")
	r.NoError(err)

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("checklist", []string{"8d2335b5ff3"})))
	r.NoError(err)

	r.Equal("Checklist1"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Item", tg.NewCmd("cc", []string{"8d2335b5ff3", "7b72407ca70"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil)),
	},
	), tgram.SentKeyboard)
}

func TestCompleteItemInChecklist(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)
	err = userFS.Write("-checklist1-", "Item.md", "")
	r.NoError(err)

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("cc", []string{"8d2335b5ff3", "7b72407ca70"})))
	r.NoError(err)

	r.Equal("Checklist1"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil)),
	},
	), tgram.SentKeyboard)

	items, err := bot.fs.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Empty(items)

	items, err = bot.fs.FilesAndDirs("archive")
	r.NoError(err)
	r.Len(items, 1)
	r.Equal("Item.md", items[0].Name)
}

func TestBotTodayLabelIcons(t *testing.T) {
	r := require.New(t)
	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	tgram := fake.NewTG()
	b := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)

	// Pomodoro is the only task in today
	r.Nil(b.togglePomodoro(nil))
	label := b.todayLabel()
	r.Contains(label, "🌴")
	r.Contains(label, "🍅")

	// Pomodoro and another task in today
	r.Nil(b.fs.Write(fs.DirToday, "Item.md", ""))
	label = b.todayLabel()
	r.NotContains(label, "🌴")
	r.Contains(label, "🍅")

	// No pomodoro, but there is another task in today
	r.Nil(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	label = b.todayLabel()
	r.NotContains(label, "🌴")
	r.NotContains(label, "🍅")

	// No pomodoro, no other tasks in today
	r.Nil(b.complete([]string{fs.DirToday, "Item.md"}))
	label = b.todayLabel()
	r.NoError(err)
	r.Contains(label, "🌴")
	r.NotContains(label, "🍅")
}

func makeBot(t *testing.T, conf *userconfig.Config) (*Bot, *fake.TG, *require.Assertions) {
	r := require.New(t)
	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "First task.md", "")
	r.NoError(err)
	err = userFS.Write("later", "Second task", "")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), conf)
	return bot, tgram, r
}

func TestSettingsMainPanel(t *testing.T) {
	bot, tgram, r := makeBot(t, &userconfig.DefaultConfig)
	err := bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("settings", nil)))
	r.NoError(err)
	r.Equal("Settings:", tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("🎛 Quick Panel", tg.NewCmd("configure_panel", nil)),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil)),
	},
	), tgram.SentKeyboard)
}

// Quick Panel Data-driven tests

var (
	btnFilesDel      = tg.NewBtn("📄 Files ➖", tg.NewCmd("panel_del", []string{"files"}))
	btnChecklistsDel = tg.NewBtn("☑️ Checklists ➖", tg.NewCmd("panel_del", []string{"checklists"}))
	btnPostponeDel   = tg.NewBtn("🦥 Postpone ➖", tg.NewCmd("panel_del", []string{"postpone"}))
)

var (
	delimiter = tg.NewBtn("-", tg.NewCmd("nothing", nil))
	backBtn   = tg.NewBtn("⬅️ Back", tg.NewCmd("settings", nil))
)

var (
	btnLater          = tg.NewBtn("⏳ Later ➕", tg.NewCmd("panel_add", []string{"later"}))
	btnSearch         = tg.NewBtn("🔎 Search ➕", tg.NewCmd("panel_add", []string{"search"}))
	btnFilesAdd       = tg.NewBtn("📄 Files ➕", tg.NewCmd("panel_add", []string{"files"}))
	btnChecklistsAdd  = tg.NewBtn("☑️ Checklists ➕", tg.NewCmd("panel_add", []string{"checklists"}))
	btnPostponeAdd    = tg.NewBtn("🦥 Postpone ➕", tg.NewCmd("panel_add", []string{"postpone"}))
	btnReadChecklist  = tg.NewBtn("📚 Read ➕", tg.NewCmd("panel_add", []string{"read"}))
	btnWatchChecklist = tg.NewBtn("📺 Watch ➕", tg.NewCmd("panel_add", []string{"watch"}))
	btnShopChecklist  = tg.NewBtn("🛒 Shop ➕", tg.NewCmd("panel_add", []string{"shop"}))
	btnHabits         = tg.NewBtn("🌱 Habits ➕", tg.NewCmd("panel_add", []string{"habits"}))
)

func TestConfigureQP_Empty_Default(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("configure_panel", nil)),
		[]tg.Row{
			delimiter,
			btnLater,
			btnSearch,
			btnFilesAdd,
			btnChecklistsAdd,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_Empty_AddDoc(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"files"})),
		[]tg.Row{
			btnFilesDel,
			delimiter,
			btnLater,
			btnSearch,
			btnChecklistsAdd,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_Doc_AddCheckList(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"checklists"})),
		[]tg.Row{
			btnFilesDel,
			btnChecklistsDel,
			delimiter,
			btnLater,
			btnSearch,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklists_AddPostpone(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "checklists"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"postpone"})),
		[]tg.Row{
			btnFilesDel,
			btnChecklistsDel,
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklistsPostpone_Show(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "checklists", "postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("configure_panel", nil)),
		[]tg.Row{
			btnFilesDel,
			btnChecklistsDel,
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklistsPostpone_DelChecklists(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "checklists", "postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"checklists"})),
		[]tg.Row{
			btnFilesDel,
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnChecklistsAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocPostpone_DelDoc(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"files"})),
		[]tg.Row{
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnFilesAdd,
			btnChecklistsAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_Postpone_DelPostpone(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"postpone"})),
		[]tg.Row{
			delimiter,
			btnLater,
			btnSearch,
			btnFilesAdd,
			btnChecklistsAdd,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnHabits,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_Empty_DelPostpone(t *testing.T) {
	RunQuickPanelTc_Error(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"postpone"})),
		[]tg.Row{},
	}, "button doesn't exist in user's prefs: postpone", t)
}

func TestConfigureQP_Empty_DelUnknown(t *testing.T) {
	RunQuickPanelTc_Error(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"wrong"})),
		[]tg.Row{},
	}, "button doesn't exist in user's prefs: wrong", t)
}

func TestConfigureQP_Empty_AddUnknown(t *testing.T) {
	RunQuickPanelTc_Error(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"wrong"})),
		[]tg.Row{},
	}, "unknown command: wrong", t)
}

func TestConfigureQP_Empty_AddEmpty(t *testing.T) {
	RunQuickPanelTc_Error(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{})),
		[]tg.Row{},
	}, "no params suplied to addToPanel", t)
}

func TestConfigureQP_Empty_DelEmpty(t *testing.T) {
	RunQuickPanelTc_Error(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{})),
		[]tg.Row{},
	}, "no params suplied to delFromPanel", t)
}

func RunQuickPanelTc(tc PrefTableTestCase, t *testing.T) {
	cnf := &userconfig.Config{}
	for _, opt := range tc.initial_opts {
		cnf.AddPanelButton(opt)
	}

	bot, tgram, r := makeBot(t, cnf)

	err := bot.Answer(tc.cmd_to_execute)
	r.NoError(err)
	r.Equal("Configure quick panel (➕ = add to panel, ➖ = to remove):", tgram.LastSentText)
	r.Equal(tg.NewKeyboard(tc.buttons), tgram.SentKeyboard)
}

func RunQuickPanelTc_Error(tc PrefTableTestCase, expectedErr string, t *testing.T) {
	cnf := &userconfig.Config{}
	for _, opt := range tc.initial_opts {
		cnf.AddPanelButton(opt)
	}
	bot, _, r := makeBot(t, cnf)
	actualErr := bot.Answer(tc.cmd_to_execute)
	r.EqualError(actualErr, expectedErr)
}

type PrefTableTestCase struct {
	initial_opts   []string
	cmd_to_execute *fake.Upd
	buttons        []tg.Row
}

func TestShowToFileNoDirs(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "Note.md", "")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.showToFile([]string{"345fbd7ab08"})
	r.NoError(err)

	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn("📄 Note", tg.NewCmd("mv_to_file", []string{"345fbd7ab08", "345fbd7ab08"}))),
	},
	), tgram.SentKeyboard)
}

func TestShowToFile(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "Note.md", "")
	r.NoError(err)
	err = userFS.MakeDir("dir")
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.showToFile([]string{"345fbd7ab08"})
	r.NoError(err)

	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn("🗂️ dir", tg.NewCmd("mv", []string{"", "345fbd7ab08", "dir"}))),
		tg.NewBtn("Or choose a file:", tg.NewCmd("nothing", nil)),
		tg.NewRow(tg.NewBtn("📄 Note", tg.NewCmd("mv_to_file", []string{"345fbd7ab08", "345fbd7ab08"}))),
	},
	), tgram.SentKeyboard)
}

func TestShow(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.show("text", nil, tg.MarkupHTML)
	r.NoError(err)

	r.Equal("text", tgram.LastSentText)
}

func TestShowLongMessage(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.show(strings.Repeat("a", 4096)+"b", nil, tg.MarkupHTML)
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
	r.Equal("b", tgram.LastSentText)
}

// When utf8.RuneCountInString(textChunk) == 4096, tg sends the message (len(textChunk) => 7003)
// if I have 4095 chars and add 🟢, we have 4096 chars and it is ok
// if I have 4095 chars and add ⚪️, we have 4097 chars and we fail, so tg doesn't operate on glyth clusters
func TestShowLongMessageWithColoredEmojis(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.show(strings.Repeat("a", 4095)+"🟢", nil, tg.MarkupHTML)
	r.NoError(err)

	r.Len(tgram.SentTexts, 1)
}

func TestShowLongMessageWithColoredEmoji(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.show(strings.Repeat("a", 4095)+"⚪️", nil, tg.MarkupHTML)
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
}

func TestShowLongMessageSplitByNewLine(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.show(strings.Repeat("a", 4094)+"\nabc", nil, tg.MarkupHTML)
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
	r.Equal("abc", tgram.LastSentText)
}

func TestShowMultilineFile(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	userFS.Write("today", "New file.md", "New file\nContent")

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	err = bot.Answer(fake.NewUpdCmdFake(-1, tg.NewCmd("task", []string{fs.DirToday, "501ef2410e2"})))
	r.NoError(err)

	r.Equal("New file\nContent", tgram.SentTexts[0])
}

func TestMoveToExistingFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("today", "Task.md", "")
	r.NoError(err)
	err = userFS.Write("", "New file.md", "")
	r.NoError(err)
	err = userFS.Write("", "Existing file.md", "")
	r.NoError(err)

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), &userconfig.DefaultConfig)
	upd := fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_file", []string{"501ef2410e2", "1c8f819d075"}))
	err = bot.Answer(upd)
	r.NoError(err)

	content, err := userFS.Read("", "Existing file.md")
	r.NoError(err)
	r.Equal("### 11.08.2024 Sunday\nNew file", content)
}
