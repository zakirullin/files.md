package worker

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/server/fs"
	"zakirullin/stuffbot/server/userconfig"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}
}

//func TestBot_togglePomodoro(t *testing.T) {
//	r := require.New(t)
//	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
//	r.NoError(err)
//	tgram := tg.NewFakeTG()
//	redis, err := miniredis.Run()
//	r.NoError(err)
//	defer redis.Close()
//	b2 := internal.NewBot(-1, tgram, userFS,db.NewDB(), &userconfig.DefaultConfig)
//	b := b2
//
//	pomodoroIn := func(dirName string) bool {
//		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.PomodoroTask)
//		r.NoError(err)
//		return hasPomodoroInDir
//	}
//	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))
//
//	// Add pomodoro	to today
//	r.Nil(b.togglePomodoro(nil))
//	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
//	// and remove pomodoro from today
//	r.Nil(b.togglePomodoro(nil))
//	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))
//
//	// Add pomodoro	to today
//	r.Nil(b.togglePomodoro(nil))
//	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
//	// complete it
//	r.Nil(b.complete([]string{fs.DirToday, fs.PomodoroTask}))
//	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
//	// and remove pomodoro from trash
//	r.Nil(b.togglePomodoro(nil))
//	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))
//}
//
// func TestWorkerReturnsPomodoroBackToToday(t *testing.T) {
// 	r := require.New(t)

// 	fsBackend := afero.NewMemMapFs()
// 	userFS, err := fs.NewFS("/-1", fsBackend)
// 	r.NoError(err)
// 	err = userFS.CreateUserDirs()
// 	r.NoError(err)

// 	tgram := tg.NewFakeTG()
// 	redis, err := miniredis.Run()
// 	r.NoError(err)
// 	defer redis.Close()

// 	b := NewBot(-1, tgram, userFS,db.NewDB(), &userconfig.DefaultConfig)

// 	currentBackend := fs.DefaultBackend
// 	fs.DefaultBackend = fsBackend
// 	defer func() {
// 		fs.DefaultBackend = currentBackend
// 	}()

// 	pomodoroIn := func(dirName string) bool {
// 		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.PomodoroTask)
// 		r.NoError(err)
// 		return hasPomodoroInDir
// 	}
// 	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))

// 	// Add pomodoro	to today
// 	r.Nil(b.togglePomodoro(nil))
// 	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
// 	// set pomodoro duration to 1us
// 	r.NoError(b.conf.SetPomodoroDuration(time.Nanosecond))
// 	// complete it
// 	r.NoError(b.complete([]string{fs.DirToday, fs.PomodoroTask}))
// 	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
// 	// "wait" until it gets back to today
// 	r.NoError(worker.MoveDueTasksToToday("", "conf", fsBackend))
// 	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
// }

//func TestWorkerPomodoroIsNotReturnedUntilItIsDue(t *testing.T) {
//	r := require.New(t)
//	fsBackend := afero.NewMemMapFs()
//	userFS, err := fs.NewFS("/-1", fsBackend)
//	r.NoError(err)
//	tgram := tg.NewFakeTG()
//	redis, err := miniredis.Run()
//	r.NoError(err)
//	defer redis.Close()
//	b := NewBot(-1, tgram, userFS,db.NewDB(), &userconfig.DefaultConfig)
//
//	currentBackend := fs.DefaultBackend
//	fs.DefaultBackend = fsBackend
//	defer func() {
//		fs.DefaultBackend = currentBackend
//	}()
//
//	pomodoroIn := func(dirName string) bool {
//		hasPomodoroInDir, err := b.fs.Exists(dirName, fs.PomodoroTask)
//		r.NoError(err)
//		return hasPomodoroInDir
//	}
//	r.False(pomodoroIn(fs.DirToday) || pomodoroIn(fs.DirArchive))
//
//	r.NoError(b.togglePomodoro(nil))
//	r.True(pomodoroIn(fs.DirToday) && !pomodoroIn(fs.DirArchive))
//	r.NoError(b.conf.SetPomodoroDuration(2 * time.Second))
//	r.NoError(b.complete([]string{fs.DirToday, fs.PomodoroTask}))
//	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
//	// trigger due tasks processing
//	r.NoError(worker.MoveDueTasksToToday("", "conf", fsBackend))
//	// pomodoro is not returned back to today
//	r.True(!pomodoroIn(fs.DirToday) && pomodoroIn(fs.DirArchive))
//}

func TestMoveDueTasksFromArchive(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirArchive, fs.DoneFilename, "- [ ] due task")

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	_ = cfg.CreateDefaultIfNotExists()
	_ = cfg.AddToSchedule("due task", 0, "")
	r.NoError(err)

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Equal("due task", sc[0].Filename)
	r.Equal(int64(0), sc[0].ScheduledAt)
	r.Equal("", sc[0].Cmd)
	r.Equal("", sc[0].Cron)

	tgram := tg.NewFakeTG()
	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
	r.NoError(err)

	todayMD, err := userFS.Read(fs.DirRoot, fs.TodayFilename)
	r.NoError(err)

	items, _ := txt.ChecklistItems(todayMD)
	r.Contains(items, "due task")

	sc, err = cfg.Schedules()
	r.NoError(err)
	r.Empty(sc)
}

func TestMoveDueTasksFromLater(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirRoot, fs.LaterFilename, "- [ ] due task")

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	_ = cfg.CreateDefaultIfNotExists()
	_ = cfg.AddToSchedule("due task", 0, "")
	r.NoError(err)

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Equal("due task", sc[0].Filename)
	r.Equal(int64(0), sc[0].ScheduledAt)
	r.Equal("", sc[0].Cmd)
	r.Equal("", sc[0].Cron)

	tgram := tg.NewFakeTG()
	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
	r.NoError(err)

	todayMD, err := userFS.Read(fs.DirRoot, fs.TodayFilename)
	r.NoError(err)

	items, _ := txt.ChecklistItems(todayMD)
	r.Contains(items, "due task")

	sc, err = cfg.Schedules()
	r.NoError(err)
	r.Empty(sc)
}

//func TestMoveDueTasksMovesToLater(t *testing.T) {
//	r := require.New(t)
//
//	savedNow := now
//	defer func() {
//		now = savedNow
//	}()
//	now = func() time.Time {
//		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
//	}
//
//	fsBackend := afero.NewMemMapFs()
//	userFS, err := fs.NewFS("/-1", fsBackend)
//	r.NoError(err)
//	err = userFS.CreateDirsIfNotExist()
//	r.NoError(err)
//	_ = userFS.Write("archive", "due task.md", "")
//
//	cfg := userconfig.NewConfig(userFS, -1, "config.json")
//	_ = cfg.CreateDefaultIfNotExists()
//	_ = cfg.AddToSchedule("due task.md", 7*24*int64(time.Hour.Seconds()), "")
//	r.NoError(err)
//
//	sc, err := cfg.Schedules()
//	r.NoError(err)
//	r.Equal("due task.md", sc[0].Filename)
//	r.Equal(int64(604800), sc[0].ScheduledAt)
//	r.Equal("", sc[0].Cmd)
//	r.Equal("", sc[0].Cron)
//
//	tgram := tg.NewFakeTG()
//	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
//	r.NoError(err)
//
//	exists, err := userFS.Exists("later", "due task.md")
//	r.NoError(err)
//	r.True(exists)
//
//	sc, err = cfg.Schedules()
//	r.NoError(err)
//	r.Len(sc, 1)
//}

func TestMoveDueTasksDoesntMove(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	err = userFS.Write("archive", "due task.md", "")
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)
	err = cfg.AddToSchedule("due task.md", 7*24*int64(time.Hour.Seconds())+1, "")
	r.NoError(err)

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Equal("due task.md", sc[0].Filename)
	r.Equal(7*24*int64(time.Hour.Seconds())+1, sc[0].ScheduledAt)
	r.Equal("", sc[0].Cmd)
	r.Equal("", sc[0].Cron)

	tgram := tg.NewFakeTG()
	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
	r.NoError(err)

	exists, err := userFS.Exists("archive", "due task.md")
	r.NoError(err)
	r.True(exists)

	sc, err = cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
}
