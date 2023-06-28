package stats

import (
	"fmt"
	"os"
	"testing"
	"time"

	dbpkg "zakirullin/stuffbot/internal/db"
	fs2 "zakirullin/stuffbot/internal/fs"

	"github.com/alicebob/miniredis/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestDoneToday(t *testing.T) {
	r := require.New(t)

	saved := fs2.Ctime
	defer func() {
		fs2.Ctime = saved
	}()
	fs2.Ctime = func(fi os.FileInfo) int64 {
		return 1
	}

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Unix(0, 0)
	}

	fs, _ := fs2.NewFS("", afero.NewMemMapFs())
	err := fs.Put("_archive_", "a.md", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	if err != nil {
		panic(fmt.Sprintf("Can't create Redis: %s\n", err))
	}
	defer func() {
		redis.Close()
	}()

	db := dbpkg.NewDB(redis)
	r.Nil(err)

	tasks, err := DoneToday(fs, db, -1)
	r.Nil(err)

	r.Equal([]string{"A"}, tasks)
}

func TestDoneTodayExcludeScheduled(t *testing.T) {
	r := require.New(t)

	saved := fs2.Ctime
	defer func() {
		fs2.Ctime = saved
	}()
	fs2.Ctime = func(fi os.FileInfo) int64 {
		return 1
	}

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Unix(0, 0)
	}

	fs, _ := fs2.NewFS("", afero.NewMemMapFs())
	err := fs.Put("_archive_", "a.md", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	if err != nil {
		panic(fmt.Sprintf("Can't create Redis: %s\n", err))
	}
	defer func() {
		redis.Close()
	}()

	db := dbpkg.NewDB(redis)
	err = db.AddToSchedule(-1, "a.md", 1, "cron")
	r.Nil(err)

	tasks, err := DoneToday(fs, db, -1)
	r.Nil(err)

	r.Empty(tasks)
}

func TestDoneTodayScheduled(t *testing.T) {
	r := require.New(t)

	saved := fs2.Ctime
	defer func() {
		fs2.Ctime = saved
	}()
	fs2.Ctime = func(fi os.FileInfo) int64 {
		return 1
	}

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Unix(0, 0)
	}

	fs, _ := fs2.NewFS("", afero.NewMemMapFs())
	err := fs.Put("_archive_", "a.md", "")
	r.Nil(err)
	err = fs.Put("_archive_", "b.md", "")
	r.Nil(err)

	redis, err := miniredis.Run()
	if err != nil {
		panic(fmt.Sprintf("Can't create Redis: %s\n", err))
	}
	defer func() {
		redis.Close()
	}()

	db := dbpkg.NewDB(redis)
	err = db.AddToSchedule(-1, "a.md", 1, "cron")
	r.Nil(err)

	tasks, err := DoneTodayScheduled(fs, db, -1)
	r.Nil(err)
	r.Equal([]string{"A"}, tasks)
}
