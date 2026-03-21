package worker

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/server"
	"zakirullin/stuffbot/server/db"
	"zakirullin/stuffbot/server/fs"
	"zakirullin/stuffbot/server/journal"
	"zakirullin/stuffbot/server/sched"
	"zakirullin/stuffbot/server/userconfig"
	"zakirullin/stuffbot/pkg/txt"
)

const (
	daysInAdvanceForLater = 7 * 24 * time.Hour
)

var now = time.Now

// Map to store that we've already removed completed items from today/later.
// The key is userID#day.
var alreadyRemoved = make(map[string]bool)

// MoveDueTasks moves due tasks from archive to later or today, or from later to today
func MoveDueTasks(
	storagePath,
	configFilename string,
	fsBackend afero.Fs,
	telegram internal.Chat,
) error {
	infolog := slog.New(slog.NewTextHandler(os.Stdout, nil))

	rootFS, err := fs.NewFS(storagePath, fsBackend)
	if err != nil {
		return fmt.Errorf("schedule worker: can't create FS: %s", err)
	}

	userDirs, err := rootFS.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("schedule worker: %w", err)
	}
	// TODO release
	// userDirs = fs.OnlyUserDirs(userDirs)

	for _, userDir := range userDirs {
		userID, err := strconv.ParseInt(userDir.Name, 10, 64)
		if err != nil {
			slog.Error("schedule worker: can't parse user ID", "dir", userDir.Name, "err", err)
			continue
		}
		userPath := path.Join(storagePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, fsBackend)
		if err != nil {
			slog.Error("schedule worker: can't create user FS", "err", err)
			continue
		}

		userconf := userconfig.NewConfig(userFS, userID, configFilename)

		schedules, err := userconf.Schedules()
		if err != nil {
			slog.Error("schedule worker: can't get schedules", "err", err)
			continue
		}
		for _, schedule := range schedules {
			secondsLeft := schedule.ScheduledAt - now().Unix()
			shouldScheduleForToday := secondsLeft <= 0
			if !shouldScheduleForToday {
				continue
			}

			moved, err := moveTaskToToday(schedule.Filename, userFS)
			if err != nil {
				slog.Error("schedule worker: can't move to today", "err", err)
				continue
			}
			if !moved {
				continue
			}

			infolog.Info("scheduled task moved to today", schedule.Filename, "filename")

			bot := internal.NewBot(userID, telegram, userFS, db.NewDB(userID), userconf)
			_ = bot.ShowToday(nil)

			// Schedule a recurring task if cron is not empty
			if len(schedule.Cron) != 0 {
				nextScheduledAt := sched.NextExcludeToday(schedule.Cron)
				err = userconf.AddToSchedule(schedule.Filename, nextScheduledAt, schedule.Cron)
				if err != nil {
					slog.Error("schedule worker: can't add to schedule", "err", err)
					continue
				}
				infolog.Info("task was rescheduled", "filename", schedule.Filename, "schedule", schedule.Cron, "scheduledAt", nextScheduledAt)
				continue
			}

			// We must only delete when it's a non-repeated task
			err = userconf.DelFromSchedule(schedule.Filename)
			if err != nil {
				slog.Error("schedule worker: can't delete from schedule", "err", err)
				continue
			}
		}
	}

	return nil
}

func RemoveCompletedChecklistItems(
	storagePath,
	configFilename string,
	fsBackend afero.Fs,
) error {
	rootFS, err := fs.NewFS(storagePath, fsBackend)
	if err != nil {
		return fmt.Errorf("schedule worker: can't create FS: %s", err)
	}

	userDirs, err := rootFS.FilesAndDirs(fs.DirRoot)
	if err != nil {
		return fmt.Errorf("schedule worker: %w", err)
	}

	for _, userDir := range userDirs {
		userID, err := strconv.ParseInt(userDir.Name, 10, 64)
		if err != nil {
			slog.Error("schedule worker: can't parse user ID", "dir", userDir.Name, "err", err)
			continue
		}
		userPath := path.Join(storagePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, fsBackend)
		if err != nil {
			slog.Error("schedule worker: can't create user FS", "err", err)
			continue
		}

		if alreadyRemoved[txt.I64(userID)+"#"+now().Format("2006-01-02")] {
			continue
		}

		userconf := userconfig.NewConfig(userFS, userID, configFilename)
		tz := userconf.Timezone()
		// Only remove completed items if it's 23:50 in the user's timezone.
		if now().In(tz).Hour() != 23 || now().In(tz).Minute() < 50 {
			continue
		}

		for _, checklist := range []string{fs.TodayFilename, fs.LaterFilename} {
			md, err := userFS.Read(fs.DirRoot, checklist)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				slog.Error("schedule worker: can't read today file", "err", err)
				continue
			}

			reducedMD, removedMD := txt.RemoveCompletedChecklistItems(md)
			if removedMD == "" {
				continue
			}

			err = userFS.Write(fs.DirRoot, checklist, reducedMD)
			if err != nil {
				slog.Error("schedule worker: can't write today file", "err", err)
				continue
			}

			doneMD, err := userFS.Read(fs.DirArchive, fs.DoneFilename)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Error("schedule worker: can't read done file", "err", err)
				continue
			}
			header := fmt.Sprintf("#### %d %s %d, %s", now().Day(), now().Format("January"), now().Year(), now().Weekday())
			doneMD = txt.AddHeaderAndText(doneMD, header, removedMD)

			err = userFS.Write(fs.DirArchive, fs.DoneFilename, doneMD)
			if err != nil {
				slog.Error("schedule worker: can't write done file", "err", err)
			}

			tasks, _ := txt.ChecklistItems(removedMD)
			for _, task := range tasks {
				_ = journal.AddRecord(userFS, fmt.Sprintf("✅ %s", task), userconf.Timezone())
			}
		}

		alreadyRemoved[txt.I64(userID)+"#"+now().Format("2006-01-02")] = true
	}

	return nil
}

func moveTaskToToday(task string, userFS *fs.FS) (bool, error) {
	// Move the task to today first, then try to delete it from either Later.txt or Done.txt.
	todayMD, err := userFS.Read(fs.DirRoot, fs.TodayFilename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = userFS.Write(fs.DirRoot, fs.TodayFilename, "")
			if err != nil {
				return false, fmt.Errorf("moveTaskToToday: can't create today file: %w", err)
			}
		} else {
			return false, fmt.Errorf("moveTaskToToday: can't read today file: %w", err)
		}
	}
	err = userFS.Write(fs.DirRoot, fs.TodayFilename, txt.AddChecklistItem(todayMD, task, false))
	if err != nil {
		return false, fmt.Errorf("moveTaskToToday: can't write to today file: %w", err)
	}

	// Try to remove task from Done.txt
	doneMD, _ := userFS.Read(fs.DirArchive, fs.DoneFilename)
	reducedDoneMD, _ := txt.RemoveChecklistItem(doneMD, task)
	itemWasRemoved := doneMD != reducedDoneMD
	if itemWasRemoved {
		err = userFS.Write(fs.DirArchive, fs.DoneFilename, reducedDoneMD)
		if err != nil {
			return true, fmt.Errorf("moveTaskToToday: can't write to done file: %w", err)
		}

		return true, nil
	}

	// Try to remove task from Later.txt
	laterMD, _ := userFS.Read(fs.DirRoot, fs.LaterFilename)
	reducedLaterMD, _ := txt.RemoveChecklistItem(laterMD, task)
	itemWasRemoved = laterMD != reducedLaterMD
	if itemWasRemoved {
		err = userFS.Write(fs.DirRoot, fs.LaterFilename, reducedLaterMD)
		if err != nil {
			return true, fmt.Errorf("moveTaskToToday: can't write to done file: %w", err)
		}

		return true, nil
	}

	return true, nil
}
