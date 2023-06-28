package stats

//func TestDoneToday(t *testing.T) {
//	r := require.New(t)
//
//	saved := fs2.Ctime
//	defer func() {
//		fs2.Ctime = saved
//	}()
//	fs2.Ctime = func(fi os.FileInfo) int64 {
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
//	fs, _ := fs2.NewFS("/", afero.NewMemMapFs())
//	err := fs.Put("_archive_", "a.md", "")
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
//	r.NoError(err)
//
//	tasks, err := DoneToday(fs, db, -1)
//	r.NoError(err)
//
//	r.Equal([]string{"A"}, tasks)
//}

//func TestDoneTodayExcludeScheduled(t *testing.T) {
//	r := require.New(t)
//
//	saved := fs2.Ctime
//	defer func() {
//		fs2.Ctime = saved
//	}()
//	fs2.Ctime = func(fi os.FileInfo) int64 {
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
//	fs, _ := fs2.NewFS("/", afero.NewMemMapFs())
//	err := fs.Put("_archive_", "a.md", "")
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
//	tasks, err := DoneToday(fs, db, -1)
//	r.NoError(err)
//
//	r.Empty(tasks)
//}

//func TestDoneTodayScheduled(t *testing.T) {
//	r := require.New(t)
//
//	saved := fs2.Ctime
//	defer func() {
//		fs2.Ctime = saved
//	}()
//	fs2.Ctime = func(fi os.FileInfo) int64 {
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
//	fs, _ := fs2.NewFS("/", afero.NewMemMapFs())
//	err := fs.Put("_archive_", "a.md", "")
//	r.NoError(err)
//	err = fs.Put("_archive_", "b.md", "")
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
