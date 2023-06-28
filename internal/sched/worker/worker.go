package worker

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched"
	"zakirullin/stuffbot/internal/userconfig"
)

func MoveDueTasksToToday(conf internal.Config, fsBackend afero.Fs) error {
	rootFS, _ := fs.NewFS(conf.StoragePath, fsBackend)

	userDirs, err := rootFS.FilesAndDirs("")
	if err != nil {
		return fmt.Errorf("schedule worker: %w", err)
	}
	userDirs = fs.OnlyUserDirs(userDirs)

	for _, userDir := range userDirs {
		userID, err := strconv.ParseInt(userDir.Name, 10, 64)
		if err != nil {
			return fmt.Errorf("schedule worker: can't parse user ID: %s", err)
		}
		userPath := fs.UserPath(conf.StoragePath, userID)
		userFS, err := fs.NewFS(userPath, fsBackend)
		if err != nil {
			return fmt.Errorf("schedule worker: can't create FS: %s", err)
		}

		userconf := userconfig.NewConfig()
		userconfPath := userFS.Path("", conf.ConfigFilename)
		err = userconf.LoadOrCreate(userconfPath)
		if err != nil {
			return fmt.Errorf("schedule worker: can't load user config: %s", err)
		}

		for _, schedule := range userconf.Schedules() {
			if time.Now().Unix() >= schedule.ScheduleAt {
				err = moveTaskToToday(schedule.Filename, userFS)
				if err != nil {
					slog.Error("schedule worker: can't move", "err", err)
				}
				slog.Debug("Scheduled task moved to today", schedule.Filename, "filename")
				if len(schedule.Cron) != 0 {
					runAt := sched.Next(schedule.Cron)
					userconf.AddToSchedule(schedule.Filename, runAt, schedule.Cron)
					slog.Debug("Task was rescheduled", "filename", schedule.Filename, "schedule", schedule.Cron, "runAt", runAt)
					continue
				}

				userconf.DelFromSchedule(schedule.Filename)
			}
		}
	}

	return nil
}

func moveTaskToToday(filename string, fsys *fs.FS) error {
	dirsToLookFor := []string{fs.DirLater, fs.DirArchive}
	for _, dir := range dirsToLookFor {
		filenames, err := fsys.FilesAndDirs(dir)
		if err != nil {
			return fmt.Errorf("moveTaskForToday: %w", err)
		}

		for _, f := range filenames {
			if f.Name == filename {
				err = fsys.Rename(dir, filename, fs.DirToday, filename)
				if err != nil {
					return fmt.Errorf("moveTaskForToday: can't rename: %w", err)
				}
			}
		}
	}

	return nil
}
