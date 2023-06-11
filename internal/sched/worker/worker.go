package worker

import (
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/dumpbot/internal/db"
	"zakirullin/dumpbot/internal/fs"
	"zakirullin/dumpbot/internal/sched"
)

func MoveDueTasksToToday(redis *miniredis.Miniredis, fsBackend afero.Fs) error {
	ids, err := fs.AllUserIDs()
	if err != nil {
		return fmt.Errorf("moveDueTasksForToday: %s\n", err)
	}

	for _, id := range ids {
		database := db.NewDB(redis)
		sch, err := database.Schedule(id)
		if err != nil {
			return fmt.Errorf("moveDueTasksForToday: can't get sch: %s", err)
		}

		fsys, err := fs.NewFS(id, fsBackend)
		if err != nil {
			return fmt.Errorf("moveDueTasksForToday: can't create FS: %s", err)
		}
		for filename, cron := range sch {
			if time.Now().Unix() >= cron.RunAt {
				err = moveTaskToToday(filename, fsys)
				if err != nil {
					slog.Error("moveDueTasksForToday: can't move", "err", err)
				}
				slog.Debug("Scheduled task moved to today", filename, "filename")
				if len(cron.Cron) != 0 {
					runAt := sched.Next(cron.Cron)
					err = database.AddToSchedule(id, filename, runAt, cron.Cron)
					if err != nil {
						slog.Error("can't reschedule task", "filename", filename, "cron", cron.Cron, "err", err)
					}
					slog.Debug("Task was rescheduled", "filename", filename, "cron", cron.Cron, "runAt", runAt)
					continue
				}

				err = database.DelFromSchedule(id, filename)
				if err != nil {
					fmt.Printf("err")
				}
			}
		}
	}
	return nil
}

func moveTaskToToday(filename string, fsys *fs.FS) error {
	dirsToLookFor := []string{fs.DirLater, fs.DirTrash}
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
