package stats

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
)

func TestDoneToday(t *testing.T) {
	r := require.New(t)

	saved := fs.Ctime
	defer func() {
		fs.Ctime = saved
	}()
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 1
	}

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Unix(0, 0)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("archive", "a.md", "")
	r.NoError(err)

	database := db.NewDB(-1)
	r.NoError(err)

	tasks, err := DoneToday(userFS, database, -1)
	r.NoError(err)

	r.Equal([]string{"A"}, tasks)
}

//func TestDoneTodayExcludeScheduled(t *testing.T) {
//	r := require.New(t)
//
//	saved := fs.Ctime
//	defer func() {
//		fs.Ctime = saved
//	}()
//	fs.Ctime = func(fi os.FileInfo) int64 {
//		return 1
//	}
//
//	savedNow := now
//	defer func() {
//		now = savedNow
//	}()
//	now = func() time.Time {
//		return time.Unix(0, 0)
//	}
//
//	userFS, _ := fs.NewFS("/", afero.NewMemMapFs())
//	err := userFS.Write("archive", "a.md", "")
//	r.NoError(err)
//
//	userDB := db.NewDB()
//	err = db.AddToSchedule(-1, "a.md", 1, "cron")
//	r.NoError(err)
//
//	tasks, err := DoneToday(fs, db, -1)
//	r.NoError(err)
//
//	r.Empty(tasks)
//}

//func TestDoneTodayScheduled(t *testing.T) {
//	r := require.New(t)
//
//	saved := fs.Ctime
//	defer func() {
//		fs.Ctime = saved
//	}()
//	fs.Ctime = func(fi os.FileInfo) int64 {
//		return 1
//	}
//
//	savedNow := now
//	defer func() {
//		now = savedNow
//	}()
//	now = func() time.Time {
//		return time.Unix(0, 0)
//	}
//
//	fs, _ := fs.NewFS("/", afero.NewMemMapFs())
//	err := fs.Put("archive", "a.md", "")
//	r.NoError(err)
//	err = fs.Put("archive", "b.md", "")
//	r.NoError(err)
//
//	redis, err := miniredis.Run()
//	if err != nil {
//		panic(fmt.Sprintf("Can't create Redis: %s\n", err))
//	}
//	defer func() {
//		redis.Close()
//	}()
//
//	db := dbpkg.NewDB(redis)
//	err = db.AddToSchedule(-1, "a.md", 1, "cron")
//	r.NoError(err)
//
//	tasks, err := DoneTodayScheduled(fs, db, -1)
//	r.NoError(err)
//	r.Equal([]string{"A"}, tasks)
//}
