package internal

import (
	"github.com/alicebob/miniredis/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
	"zakirullin/dumpbot/internal/sched/worker"
	"zakirullin/dumpbot/internal/userconfig"

	"zakirullin/dumpbot/internal/db"
	"zakirullin/dumpbot/internal/fs"
	"zakirullin/dumpbot/pkg/tg"
	"zakirullin/dumpbot/pkg/tg/fake"
)

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}
}

func TestAddTaskToToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpd(-1, "New task"))
	r.Nil(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.Nil(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
}

func TestAddMultilineTaskToToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)

	tgram := fake.NewTG()

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpd(-1, "New task\nContent"))
	r.Nil(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.Nil(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.Equal(true, tasks[0].IsMultiline)

	content, err := bot.fs.Content("today", "New task.md")
	r.Nil(err)
	r.Equal("Content", content)
}

func TestAddTaskWithSpecCharsToToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)

	tgram := fake.NewTG()

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpd(-1, "New task\nUrl! http://g.com (Also_text] ##header\n-item1\n-item2\n1+1=2"))
	r.Nil(err)

	tasks, err := bot.fs.FilesAndDirs("today")
	r.Nil(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.Equal(true, tasks[0].IsMultiline)

	content, err := bot.fs.Content("today", "New task.md")
	r.Nil(err)
	r.Equal("Url! http://g.com (Also\\_text] ##header\n-item1\n-item2\n1+1=2", content)
}

func TestAddTaskToLater(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)

	err = fsys.Put("today", "First task.md", "")
	r.Nil(err)

	tgram := fake.NewTG()

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("mv", []string{"today", "0824149b387", "later"})))
	r.Nil(err)

	todayTasks, err := bot.fs.FilesAndDirs("today")
	r.Nil(err)
	r.Len(todayTasks, 0)

	laterTasks, err := bot.fs.FilesAndDirs("later")
	r.Nil(err)
	r.Len(laterTasks, 1)
	r.Equal("First task.md", laterTasks[0].Name)
}

func TestCompleteTask(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)

	err = fsys.Put("today", "First task.md", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("comp", []string{"today", "0824149b387"})))
	r.Nil(err)

	todayTasks, err := bot.fs.FilesAndDirs("today")
	r.Nil(err)
	r.Len(todayTasks, 0)

	completedTasks, err := bot.fs.FilesAndDirs("_trash_")
	r.Nil(err)
	r.Len(completedTasks, 1)
	r.Equal("First task.md", completedTasks[0].Name)
}

func TestToday(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	err = fsys.Put("today", "First task.md", "")
	r.Nil(err)
	err = fsys.Put("today", "Second task", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil)))
	r.Nil(err)

	r.Equal("<b>2</b> left", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("comp", []string{"today", "0824149b387"})),
		tg.NewBtn("Second task", tg.NewCmd("comp", []string{"today", "2940ad40402"})),
		tg.NewBtn("⏳ Later", tg.NewCmd("later", []string{"later"}))},
	), tgram.SentKeyboard)
}

func TestLater(t *testing.T) {
	r := require.New(t)

	fsys, _ := fs.NewFS(-1, afero.NewMemMapFs())
	err := fsys.Put("later", "First task.md", "")
	r.Nil(err)
	err = fsys.Put("later", "Second task", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("later", nil)))
	r.Nil(err)

	r.Equal("⏳ Your tasks for later:", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("comp", []string{"later", "0824149b387"})),
		tg.NewBtn("Second task", tg.NewCmd("comp", []string{"later", "2940ad40402"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", []string{"today"}))},
	), tgram.SentKeyboard)

}

func TestTodayWithMultilineTasks(t *testing.T) {
	r := require.New(t)

	fsys, _ := fs.NewFS(-1, afero.NewMemMapFs())
	err := fsys.Put("today", "First task.md", "content")
	r.Nil(err)
	err = fsys.Put("today", "Second task", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	upd := fake.NewUpdCmdFake(-1, tg.NewCmd("today", nil))
	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(upd)
	r.Nil(err)

	r.Equal("<b>2</b> left", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("👀 First task", tg.NewCmd("task", []string{"today", "0824149b387"})),
		tg.NewBtn("Second task", tg.NewCmd("comp", []string{"today", "2940ad40402"})),
		tg.NewBtn("⏳ Later", tg.NewCmd("later", []string{"later"}))},
	), tgram.SentKeyboard)
}

func TestDocs(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	err = fsys.Put("", "Doc1.md", "")
	r.Nil(err)
	err = fsys.Put("", "Doc2.md", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("docs", nil)))
	r.Nil(err)

	r.Equal("📝 Your docs:", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Doc1", tg.NewCmd("doc", []string{"c1253521ac7"})),
		tg.NewBtn("Doc2", tg.NewCmd("doc", []string{"64572c3093f"})),
		tg.NewBtn("Back to docs", tg.NewCmd("docs", nil))},
	), tgram.SentKeyboard)
}

func TestChecklists(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	err = fsys.MakeDir("-checklist1-")
	r.Nil(err)
	err = fsys.MakeDir("-checklist2-")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()

	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("checklists", nil)))
	r.Nil(err)

	r.Equal("☑️ Checklists", tgram.SentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Checklist1", tg.NewCmd("checklist", []string{"8d2335b5ff3"})),
		tg.NewBtn("Checklist2", tg.NewCmd("checklist", []string{"8d3625e2e84"})),
		tg.NewBtn("🏠 Today", tg.NewCmd("today", nil))},
	), tgram.SentKeyboard)
}

func TestAddSingleItemToChecklist(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	err = fsys.MakeDir("-checklist1-")
	r.Nil(err)
	err = fsys.Put("today", "Item.md", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_chk", []string{"7b72407ca70", "-checklist1-"})))
	r.Nil(err)

	files, err := fsys.FilesAndDirs("-checklist1-")
	r.Nil(err)
	r.Len(files, 1)
	r.Equal("Item.md", files[0].Name)

	files, err = fsys.FilesAndDirs("today")
	r.Nil(err)
	r.Len(files, 0)
}

func TestAddMultipleItemsToChecklist(t *testing.T) {
	r := require.New(t)

	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	err = fsys.MakeDir("-checklist1-")
	r.Nil(err)
	err = fsys.Put("today", "Item.md", "item2\nitem3\n\n")
	r.Nil(err)

	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()

	tgram := fake.NewTG()
	bot := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	err = bot.Reply(fake.NewUpdCmdFake(-1, tg.NewCmd("mv_to_chk", []string{"7b72407ca70", "-checklist1-"})))
	r.Nil(err)

	files, err := fsys.FilesAndDirs("-checklist1-")
	r.Nil(err)
	r.Len(files, 3)
	r.ElementsMatch([]string{"Item.md", "Item2.md", "Item3.md"}, []string{files[0].Name, files[1].Name, files[2].Name})
}

func TestBot_togglePomodoro(t *testing.T) {
	r := require.New(t)
	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()
	b2 := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)
	b := b2

	pomodoroIn := func(dirName string) bool {
		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.FilePomodoro)
		r.Nil(err)
		return hasPomodoroInDir
	}
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirTrash))

	// Add pomodoro	to today
	r.Nil(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirTrash))
	// and remove pomodoro from today
	r.Nil(b.togglePomodoro(nil))
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirTrash))

	// Add pomodoro	to today
	r.Nil(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirTrash))
	// complete it
	r.Nil(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirTrash))
	// and remove pomodoro from trash
	r.Nil(b.togglePomodoro(nil))
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirTrash))
}

func TestBot_pomodoroCompletion(t *testing.T) {
	r := require.New(t)
	fsBackend := afero.NewMemMapFs()
	t.Setenv("ADMIN_USER_ID", "-1")
	fsys, err := fs.NewFS(-1, fsBackend)
	r.Nil(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()
	b := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)

	pomodoroIn := func(dirName string) bool {
		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.FilePomodoro)
		r.Nil(err)
		return hasPomodoroInDir
	}
	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirTrash))

	// Add pomodoro	to today
	r.Nil(b.togglePomodoro(nil))
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirTrash))
	// set pomodoro duration to 100ms
	b.conf.PomodoroDuration = "1s"
	// complete it
	r.Nil(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirTrash))

	// wait less than pomodoro duration
	time.Sleep(100 * time.Millisecond)
	err = worker.MoveDueTasksToToday(redis, fsBackend)
	r.Nil(err)
	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirTrash))

	// wait until it gets back to today
	time.Sleep(900 * time.Millisecond)
	err = worker.MoveDueTasksToToday(redis, fsBackend)
	r.Nil(err)
	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirTrash))
}

func TestBot_todayLabelIcons(t *testing.T) {
	r := require.New(t)
	t.Setenv("ADMIN_USER_ID", "-1")
	fsys, err := fs.NewFS(-1, afero.NewMemMapFs())
	r.Nil(err)
	tgram := fake.NewTG()
	redis, err := miniredis.Run()
	r.Nil(err)
	defer redis.Close()
	b := NewBot(-1, tgram, fsys, db.NewDB(redis), &userconfig.DefaultConfig)

	// Pomodoro is the only task in today
	r.Nil(b.togglePomodoro(nil))
	label, err := b.todayLabel()
	r.Nil(err)
	r.Contains(label, "🌴")
	r.Contains(label, "🍅")

	// Pomodoro and another task in today
	r.Nil(b.fs.Put(fs.DirToday, "Item.md", ""))
	label, err = b.todayLabel()
	r.Nil(err)
	r.NotContains(label, "🌴")
	r.Contains(label, "🍅")

	// No pomodoro, but there is another task in today
	r.Nil(b.complete([]string{fs.DirToday, fs.FilePomodoro}))
	label, err = b.todayLabel()
	r.Nil(err)
	r.NotContains(label, "🌴")
	r.NotContains(label, "🍅")

	// No pomodoro, no other tasks in today
	r.Nil(b.complete([]string{fs.DirToday, "Item.md"}))
	label, err = b.todayLabel()
	r.Nil(err)
	r.Contains(label, "🌴")
	r.NotContains(label, "🍅")
}
