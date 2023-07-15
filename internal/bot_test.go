package internal

import (
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/sched/worker"
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

func TestAddTaskToToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpd(-1, "New task"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
}

func TestAddMultilineTaskToToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpd(-1, "New task\nContent"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.Equal(true, tasks[0].IsMultiline)

	content, err := bot.fs.Content("today", "New task.md")
	r.NoError(err)
	r.Equal("Content", content)
}

func TestAddTaskWithSpecCharsToToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := fake.NewTG()

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpd(-1, "New task\nUrl! http://g.com (Also_text] ##header\n-item1\n-item2\n1+1=2"))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.Equal(true, tasks[0].IsMultiline)

	content, err := bot.fs.Content("today", "New task.md")
	r.NoError(err)
	r.Equal("Url! http://g.com (Also\\_text] ##header\n-item1\n-item2\n1+1=2", content)
}

func TestAddTaskToLater(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(fsys.CreateUserDirs())

	err = fsys.Put("today", "First task.md", "")
	r.NoError(err)

	tgram := fake.NewTG()

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("mv", []string{"today", "0824149b387", "later"})))
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

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = fsys.Put("today", "First task.md", "")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("comp", []string{"today", "0824149b387"})))
	r.NoError(err)

	todayTasks, err := bot.fs.FilesAndDirs("today")
	r.NoError(err)
	r.Len(todayTasks, 0)

	completedTasks, err := bot.fs.FilesAndDirs("_archive_")
	r.NoError(err)
	r.Len(completedTasks, 1)
	r.Equal("First task.md", completedTasks[0].Name)
}

func TestToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.Put("today", "First task.md", "")
	r.NoError(err)
	err = fsys.Put("today", "Second task", "")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil)))
	r.NoError(err)

	r.Equal("<b>2</b> left", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("comp", []string{"today", "0824149b387"})),
		tg.NewBtn("Second task", tg.NewCmd("comp", []string{"today", "2940ad40402"})),
		tg.NewBtn("⏳ Later", tg.NewCmd("later", []string{"later"}))},
	), tgram.SentKeyboard)
}

func TestLater(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.Put("later", "First task.md", "")
	r.NoError(err)
	err = fsys.Put("later", "Second task", "")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("later", nil)))
	r.NoError(err)

	r.Equal("⏳ Your tasks for later:", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("comp", []string{"later", "0824149b387"})),
		tg.NewBtn("Second task", tg.NewCmd("comp", []string{"later", "2940ad40402"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", []string{"today"}))},
	), tgram.SentKeyboard)

}

func TestTodayWithMultilineTasks(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.Put("today", "First task.md", "content")
	r.NoError(err)
	err = fsys.Put("today", "Second task", "")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	upd := fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil))
	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(upd)
	r.NoError(err)

	r.Equal("<b>2</b> left", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("👀 First task", tg.NewCmd("task", []string{"today", "0824149b387"})),
		tg.NewBtn("Second task", tg.NewCmd("comp", []string{"today", "2940ad40402"})),
		tg.NewBtn("⏳ Later", tg.NewCmd("later", []string{"later"}))},
	), tgram.SentKeyboard)
}

func TestDocs(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.CreateUserDirs()
	r.NoError(err)
	err = fsys.Put("", "Doc1.md", "")
	r.NoError(err)
	err = fsys.Put("", "Doc2.md", "")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("docs", nil)))
	r.NoError(err)

	r.Equal("📝 Your docs:", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Doc1", tg.NewCmd("doc", []string{"c1253521ac7"})),
		tg.NewBtn("Doc2", tg.NewCmd("doc", []string{"64572c3093f"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil))},
	), tgram.SentKeyboard)
}

func TestChecklists(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.MakeDir("-checklist1-")
	r.NoError(err)
	err = fsys.MakeDir("-checklist2-")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("checklists", nil)))
	r.NoError(err)

	r.Equal("☑️ Checklists", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Checklist1", tg.NewCmd("checklist", []string{"8d2335b5ff3"})),
		tg.NewBtn("Checklist2", tg.NewCmd("checklist", []string{"8d3625e2e84"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil))},
	), tgram.SentKeyboard)
}

func TestAddSingleItemToChecklist(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.MakeDir("-checklist1-")
	r.NoError(err)
	err = fsys.Put("today", "Item.md", "")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_chk", []string{"7b72407ca70", "-checklist1-"})))
	r.NoError(err)

	files, err := fsys.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("Item.md", files[0].Name)

	files, err = fsys.FilesAndDirs("today")
	r.NoError(err)
	r.Len(files, 0)
}

func TestAddMultipleItemsToChecklist(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = fsys.MakeDir("-checklist1-")
	r.NoError(err)
	err = fsys.Put("today", "Item.md", "item2\nitem3\n\n")
	r.NoError(err)

	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_chk", []string{"7b72407ca70", "-checklist1-"})))
	r.NoError(err)

	files, err := fsys.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Len(files, 3)
	r.ElementsMatch([]string{"Item.md", "Item2.md", "Item3.md"}, []string{files[0].Name, files[1].Name, files[2].Name})
}

func TestBot_togglePomodoro(t *testing.T) {
	r := require.New(t)
	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()
	b2 := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	b := b2

	pomodoroIn := func(dirName string) bool {
		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.FilePomodoro)
		r.NoError(err)
		return hasPomodoroInDir
	}
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))

	// Add pomodoro	to today
	r.Nil(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
	// and remove pomodoro from today
	r.Nil(b.togglePomodoro(nil))
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))

	// Add pomodoro	to today
	r.Nil(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
	// complete it
	r.Nil(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
	// and remove pomodoro from trash
	r.Nil(b.togglePomodoro(nil))
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))
}

// Check that pomodoro is returned back to today when it's due
func TestBot_pomodoroCompletion1(t *testing.T) {
	r := require.New(t)
	fsBackend := afero.NewMemMapFs()
	t.Setenv("ADMIN_USER_ID", "-1")
	fsys, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = fsys.CreateUserDirs()
	r.NoError(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()
	b := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)

	currentBackend := fs.DefaultBackend
	fs.DefaultBackend = fsBackend
	defer func() {
		fs.DefaultBackend = currentBackend
	}()

	pomodoroIn := func(dirName string) bool {
		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.FilePomodoro)
		r.NoError(err)
		return hasPomodoroInDir
	}
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))

	// Add pomodoro	to today
	r.Nil(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
	// set pomodoro duration to 1us
	r.NoError(b.conf.SetPomodoroDuration(time.Nanosecond))
	// complete it
	r.NoError(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
	// "wait" until it gets back to today
	r.NoError(worker.MoveDueTasksToToday("", "conf", fsBackend))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
}

// Check that pomodoro is not returned back to today until it's due
func TestBot_pomodoroCompletion2(t *testing.T) {
	r := require.New(t)
	fsBackend := afero.NewMemMapFs()
	t.Setenv("ADMIN_USER_ID", "-1")
	fsys, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()
	b := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)

	currentBackend := fs.DefaultBackend
	fs.DefaultBackend = fsBackend
	defer func() {
		fs.DefaultBackend = currentBackend
	}()

	pomodoroIn := func(dirName string) bool {
		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.FilePomodoro)
		r.NoError(err)
		return hasPomodoroInDir
	}
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))

	r.NoError(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
	r.NoError(b.conf.SetPomodoroDuration(2 * time.Second))
	r.NoError(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
	// trigger due tasks processing
	r.NoError(worker.MoveDueTasksToToday("", "conf", fsBackend))
	// pomodoro is not returned back to today
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
}

func TestBot_todayLabelIcons(t *testing.T) {
	r := require.New(t)
	t.Setenv("ADMIN_USER_ID", "-1")
	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()
	b := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)

	// Pomodoro is the only task in today
	r.Nil(b.togglePomodoro(nil))
	label, err := b.todayLabel()
	r.NoError(err)
	r.Contains(label, "🌴")
	r.Contains(label, "🍅")

	// Pomodoro and another task in today
	r.Nil(b.fs.Put(fs.DirToday, "Item.md", ""))
	label, err = b.todayLabel()
	r.NoError(err)
	r.NotContains(label, "🌴")
	r.Contains(label, "🍅")

	// No pomodoro, but there is another task in today
	r.Nil(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	label, err = b.todayLabel()
	r.NoError(err)
	r.NotContains(label, "🌴")
	r.NotContains(label, "🍅")

	// No pomodoro, no other tasks in today
	r.Nil(b.complete([]string{fs.DirToday, "Item.md"}))
	label, err = b.todayLabel()
	r.NoError(err)
	r.Contains(label, "🌴")
	r.NotContains(label, "🍅")
}

func makeBot(t *testing.T, conf *userconfig.Config) (*Bot, *fake.TG, *require.Assertions) {
	r := require.New(t)
	fsys, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	redis, err := miniredis.Run()
	r.NoError(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), conf)
	return bot, tgram, r
}
func TestSettingsMainPanel(t *testing.T) {
	var bot, tgram, r = makeBot(t, &userconfig.DefaultConfig)
	var err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("settings", nil)))
	r.NoError(err)
	r.Equal("Settings: ", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("🎛 Quick Panel", tg.NewCmd("configure_panel", nil)),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil))},
	), tgram.SentKeyboard)

}

// Quick Panel Data-driven tests

var btn_documents_del = tg.NewBtn("📝 Documents ➖", tg.NewCmd("panel_del", []string{"doc"}))
var btn_checklists_del = tg.NewBtn("☑️ Checklists ➖", tg.NewCmd("panel_del", []string{"checklists"}))
var btn_postpone_del = tg.NewBtn("🦥 Postpone ➖", tg.NewCmd("panel_del", []string{"postpone"}))

var delimiter = tg.NewBtn("---", tg.NewCmd("", nil))
var backBtn = tg.NewBtn("⬅️ Back", tg.NewCmd("settings", nil))

var btn_documents_add = tg.NewBtn("📝 Documents ➕", tg.NewCmd("panel_add", []string{"doc"}))
var btn_checklists_add = tg.NewBtn("☑️ Checklists ➕", tg.NewCmd("panel_add", []string{"checklists"}))
var btn_postpone_add = tg.NewBtn("🦥 Postpone ➕", tg.NewCmd("panel_add", []string{"postpone"}))

func TestConfigureQP_Empty_Default(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("configure_panel", nil)),
		[]tg.Row{
			delimiter,
			btn_documents_add,
			btn_checklists_add,
			btn_postpone_add,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_Empty_AddDoc(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{""},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"doc"})),
		[]tg.Row{
			btn_documents_del,
			delimiter,
			btn_checklists_add,
			btn_postpone_add,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_Doc_AddCheckList(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"doc"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"checklists"})),
		[]tg.Row{
			btn_documents_del,
			btn_checklists_del,
			delimiter,
			btn_postpone_add,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklists_AddPostpone(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"doc", "checklists"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_add", []string{"postpone"})),
		[]tg.Row{
			btn_documents_del,
			btn_checklists_del,
			btn_postpone_del,
			delimiter,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklistsPostpone_Show(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"doc", "checklists", "postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("configure_panel", nil)),
		[]tg.Row{
			btn_documents_del,
			btn_checklists_del,
			btn_postpone_del,
			delimiter,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklistsPostpone_DelChecklists(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"doc", "checklists", "postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"checklists"})),
		[]tg.Row{
			btn_documents_del,
			btn_postpone_del,
			delimiter,
			btn_checklists_add,
			backBtn,
		},
	}, t)
}

func TestConfigureQP_DocPostpone_DelDoc(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"doc", "postpone"},
		fake.NewUpdCmdFake(-1, tg.NewCmd("panel_del", []string{"doc"})),
		[]tg.Row{
			btn_postpone_del,
			delimiter,
			btn_documents_add,
			btn_checklists_add,
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
			btn_documents_add,
			btn_checklists_add,
			btn_postpone_add,
			backBtn,
		},
	}, t)
}

func RunQuickPanelTc(tc PrefTableTestCase, t *testing.T) {
	var cnf = &userconfig.DefaultConfig
	for _, opt := range tc.initial_opts {
		cnf.AddPanelButton(opt)
	}

	var bot, tgram, r = makeBot(t, cnf)

	var err = bot.Reply(tc.cmd_to_execute)
	r.NoError(err)
	r.Equal("Configure quick panel (➕ = add to panel, ➖ = to remove): ", tgram.SentText)
	r.Equal(tg.NewKeyboard(tc.buttons), tgram.SentKeyboard)
}

type PrefTableTestCase struct {
	initial_opts   []string
	cmd_to_execute *fake.Upd
	buttons        []tg.Row
}
