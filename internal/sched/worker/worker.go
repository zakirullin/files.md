package worker

import (
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/txt"
)

const (
	daysInAdvanceForLater = 7 * 24 * time.Hour
)

var now = time.Now

// MoveDueTasks moves due tasks from archive to later or today, or from later to today
func MoveDueTasks(
	storagePath,
	configFilename string,
	fsBackend afero.Fs,
	telegram internal.Chat,
) error {
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
			slog.Error("schedule worker: can't parse user ID", "err", err)
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
			shouldScheduleForLater := secondsLeft > 0 && secondsLeft <= int64(daysInAdvanceForLater.Seconds())
			shouldNotSchedule := !shouldScheduleForToday && !shouldScheduleForLater
			if shouldNotSchedule {
				continue
			}

			if shouldScheduleForLater {
				err = moveTaskToLater(schedule.Filename, userFS)
				if err != nil {
					slog.Error("schedule worker: can't move to later", "err", err)
				}
				// We don't need to save the schedule, since we didn't modify it
				continue
			}

			err = moveTaskToToday(schedule.Filename, userFS)
			if err != nil {
				slog.Error("schedule worker: can't move to today", "err", err)
				continue
			}
			slog.Debug("scheduled task moved to today", schedule.Filename, "filename")

			bot := internal.NewBot(userID, telegram, userFS, db.NewDB(), userconf)
			_ = bot.ShowToday(nil)

			// Schedule a recurring task if cron is not empty
			if len(schedule.Cron) != 0 {
				scheduledAt := sched.NextExcludeToday(schedule.Cron)
				err = userconf.AddToSchedule(schedule.Filename, scheduledAt, schedule.Cron)
				if err != nil {
					slog.Error("schedule worker: can't add to schedule", "err", err)
					continue
				}
				slog.Debug("task was rescheduled", "filename", schedule.Filename, "schedule", schedule.Cron, "scheduledAt", scheduledAt)
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

func moveTaskToLater(filename string, userFS *fs.FS) error {
	filenames, err := userFS.FilesAndDirs(fs.DirArchive)
	if err != nil {
		return fmt.Errorf("moveTaskToLater: %w", err)
	}

	for _, f := range filenames {
		if f.Name == filename {
			err = userFS.Rename(fs.DirArchive, filename, fs.DirLater, filename)
			if err != nil {
				return fmt.Errorf("moveTaskToLater: can't rename: %w", err)
			}
		}
	}

	return nil
}

func moveTaskToToday(filename string, userFS *fs.FS) error {
	dirsToLookFor := []string{fs.DirLater, fs.DirArchive}
	for _, dir := range dirsToLookFor {
		exists, err := userFS.Exists(dir, filename)
		if err != nil {
			return fmt.Errorf("moveTaskForToday: can't check for existence: %w", err)
		}
		if !exists {
			continue
		}

		err = userFS.Rename(dir, filename, fs.DirToday, filename)
		if err != nil {
			return fmt.Errorf("moveTaskForToday: can't rename: %w", err)
		}
	}

	return nil
}
