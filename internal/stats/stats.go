// Package stats generates fancy reports
// containing completed tasks and habits, checked items and so on
package stats

import (
	"fmt"
	"strings"
	"time"

	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched"
)

var now = func() time.Time {
	return time.Now()
}

func TodayReport(fsys *fs.FS, db *db.DB, userID int64) (string, error) {
	files, err := DoneToday(fsys, db, userID)
	if err != nil {
		return "", fmt.Errorf("stats.TodayReport: %w", err)
	}

	var stats []string
	for _, file := range files {
		ico := icon(file)
		stats = append(stats, fmt.Sprintf("%s <b>%s</b>", ico, fs.Title(file)))
	}

	trashedFiles, err := fsys.FilesAndDirs(fs.DirArchive)
	if err != nil {
		return "", fmt.Errorf("stats.TodayReport: can't get trashed files: %w", err)
	}
	doneTotal := len(trashedFiles)
	stats = append(stats, fmt.Sprintf("📊 %d tasks done in total", doneTotal))

	return strings.Join(stats, "\n"), nil
}

func icon(filename string) string {
	if strings.HasPrefix("-read-", filename) {
		return "📚"
	}

	if strings.HasPrefix("-watch-", filename) {
		return "📺"
	}

	if strings.HasPrefix("-shop-", filename) {
		return "🛒"
	}

	if fs.IsChecklistItem(filename) {
		return "☑️"
	}

	return "✅"
}

func DoneToday(fsys *fs.FS, db *db.DB, userID int64) ([]string, error) {
	return doneToday(fsys, db, userID, false)
}

func DoneTodayScheduled(fsys *fs.FS, db *db.DB, userID int64) ([]string, error) {
	return doneToday(fsys, db, userID, true)
}

func doneToday(fsys *fs.FS, db *db.DB, userID int64, withScheduled bool) ([]string, error) {
	files, err := fsys.FilesAndDirs(fs.DirArchive)
	if err != nil {
		return nil, fmt.Errorf("stats.DoneTasks: %w", err)
	}

	var todayFiles []fs.File
	for _, task := range files {
		if task.Ctime > sched.BeginningOfTheDay(now()).Unix() {
			todayFiles = append(todayFiles, task)
		}
	}

	//sch, err := db.Schedule(userID)
	//if err != nil {
	//	return nil, fmt.Errorf("stats.DoneTasks: %w", err)
	//}

	var todayFiltered []string
	//for _, todayFile := range todayFiles {
	//	if _, scheduled := sch[todayFile.Name]; scheduled == withScheduled {
	//		todayFiltered = append(todayFiltered, todayFile.Title)
	//	}
	//}

	return todayFiltered, nil
}
