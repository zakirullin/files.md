package habits

// TODO one known bug - it won't correctly work
// if our week falls into 2 different years

import (
	_ "embed"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/bot/fs"
)

//go:embed testdata/month_habits.md
var monthMD string

//go:embed testdata/last_month_habits.md
var lastMonthMD string

//go:embed testdata/two_months_habits.md
var twoMonthsMD string

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}
}

func TestHabits(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirInsights, "1970 Habits.md", monthMD)

	habits, err := Habits(userFS, 1970)
	r.NoError(err)

	r.Len(habits, 5)
	year, ok := habits["Went to gym"]
	r.True(ok)

	r.Len(year, 31)

	r.EqualValues(Year{1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 1, 7: 0, 8: 0, 9: 0, 10: 0, 11: 1, 12: 0, 13: 0, 14: 1, 15: 0, 16: 0, 17: 0, 18: 1, 19: 0, 20: 1, 21: 0, 22: 0, 23: 1, 24: 0, 25: 1, 26: 0, 27: 1, 28: 0, 29: 1, 30: 0, 31: 1}, year)
}

func TestHabitsForTwoMonths(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirInsights, "1970 Habits.md", twoMonthsMD)

	habits, err := Habits(userFS, 1970)
	r.NoError(err)

	r.Len(habits, 2)
	year, ok := habits["Habit"]
	r.True(ok)

	r.Len(year, 61)

	r.EqualValues(Year{244: 0, 245: 0, 246: 0, 247: 0, 248: 0, 249: 1, 250: 0, 251: 1, 252: 0, 253: 0, 254: 0, 255: 0, 256: 1, 257: 0, 258: 0, 259: 0, 260: 1, 261: 0, 262: 0, 263: 1, 264: 1, 265: 1, 266: 0, 267: 1, 268: 1, 269: 0, 270: 1, 271: 0, 272: 1, 273: 0, 274: 0, 275: 0, 276: 1, 277: 0, 278: 0, 279: 1, 280: 1, 281: 0, 282: 1, 283: 1, 284: 0, 285: 0, 286: 0, 287: 0, 288: 0, 289: 0, 290: 0, 291: 0, 292: 1, 293: 1, 294: 0, 295: 0, 296: 1, 297: 0, 298: 1, 299: 1, 300: 1, 301: 1, 302: 1, 303: 0, 304: 1}, year)
}

func TestLastMonthHabits(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirInsights, "1970 Habits.md", lastMonthMD)

	habits, err := Habits(userFS, 1970)
	r.NoError(err)

	r.Len(habits, 1)
	year, ok := habits["Habit"]
	r.True(ok)

	r.Len(year, 31)

	completed, ok := year[335]
	r.True(ok)
	r.Equal(0, completed)

	completed, ok = year[365]
	r.True(ok)
	r.Equal(1, completed)
}

func TestLastWeekHabitsWhenWeekFallsIntoTwoMonths(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirInsights, "1970 Habits.md", twoMonthsMD)
	_ = userFS.Write(fs.DirHabits, "Habit.md", "")

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, time.September, 30, 0, 0, 0, 0, time.Local)
	}

	habits, err := LastWeekHabits(userFS, time.UTC)
	r.NoError(err)
	r.Len(habits, 2)
	r.Len(habits["Habit"], 7)
	r.EqualValues(Year{271: 0, 272: 1, 273: 0, 274: 0, 275: 0, 276: 1, 277: 0}, habits["Habit"])
	r.EqualValues(Year{271: 5, 272: 2, 273: 5, 274: 0, 275: 5, 276: 4, 277: 0}, habits["Mood"])
}

func TestLastMonthHabitsMoods(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write(fs.DirInsights, "1970 Habits.md", monthMD)

	habits, err := Habits(userFS, 1970)
	r.NoError(err)

	year, ok := habits["Mood"]
	r.True(ok)

	r.Len(year, 31)

	r.EqualValues(Year{1: 5, 2: 0, 3: 3, 4: 1, 5: 0, 6: 5, 7: 5, 8: 0, 9: 0, 10: 0, 11: 5, 12: 0, 13: 5, 14: 2, 15: 4, 16: 1, 17: 0, 18: 5, 19: 0, 20: 4, 21: 0, 22: 5, 23: 0, 24: 5, 25: 4, 26: 0, 27: 5, 28: 4, 29: 0, 30: 5, 31: 0}, year)
}

func TestWrite(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write("insights", "2024 Habits.md", monthMD)

	habits, err := Habits(userFS, 2024)
	r.NoError(err)

	err = Write(userFS, 2024, habits)
	r.NoError(err)

	updatedMonthMD, err := userFS.Read("insights", "2024 Habits.md")
	r.NoError(err)

	r.Equal(monthMD, updatedMonthMD)
}

func TestWritePreserveMoodsForPreviousMonth(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist()
	r.NoError(err)
	_ = userFS.Write("insights", "1970 Habits.md", twoMonthsMD)

	habits, err := Habits(userFS, 1970)
	r.NoError(err)

	err = Write(userFS, 1970, habits)
	r.NoError(err)

	updatedMD, err := userFS.Read("insights", "1970 Habits.md")
	r.NoError(err)

	r.Equal(twoMonthsMD, updatedMD)
}
