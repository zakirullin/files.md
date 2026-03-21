package sched

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"zakirullin/stuffbot/bot/fs"
	"zakirullin/stuffbot/bot/userconfig"
)

var Now = func() time.Time {
	return time.Now()
}

type Cron struct {
	RunAt int64
	Cron  string
	Cmd   string // For future use
}

func NewCron(runAt int64, cron string) Cron {
	return Cron{runAt, cron, "move"}
}

func BeginningOfTheDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func Tomorrow() int64 {
	tomorrow := Now().AddDate(0, 0, 1)

	return BeginningOfTheDay(tomorrow).Unix()
}

// NextExcludeToday returns next unix time for cron expression
func NextExcludeToday(crn string) int64 {
	sched, err := cron.ParseStandard(crn)
	// TODO release, we should not panic when a user provided bad config
	if err != nil {
		// It's a logical error in code, we don't obtain cron expressions from user input
		panic(fmt.Errorf("invalid cron expression %s: %w", crn, err))
	}

	now := Now()
	endOfDay := now.Truncate(24 * time.Hour).Add(24*time.Hour - time.Nanosecond)

	// TODO release take into account user timezone
	return sched.Next(endOfDay).Unix()
}

func ScheduleReport(scheduledTasks []userconfig.Schedule) string {
	schedule := make(map[string][]string)
	order := []string{}

	addToSchedule := func(day string, task string) {
		// Only add to order slice if the key is new
		if _, exists := schedule[day]; !exists {
			order = append(order, day)
		}
		schedule[day] = append(schedule[day], task)
	}
	for _, task := range scheduledTasks {
		addToSchedule(formatTaskDate(task.ScheduledAt), fs.DisplayName(task.Filename))
	}

	var report string
	for _, day := range order {
		report += fmt.Sprintf("<b>%s</b>\n", day)
		for _, task := range schedule[day] {
			report += fmt.Sprintf("- %s\n", task)
		}
		report += "\n"
	}

	return strings.TrimSpace(report)
}

// FilenamesAndSchedules returns filenames and schedules:
// Filename.md => Tomorrow
func FilenamesAndSchedules(scheduledTasks []userconfig.Schedule) map[string]string {
	formatted := make(map[string]string)
	for _, task := range scheduledTasks {
		formatted[task.Filename] = formatTaskDate(task.ScheduledAt)
	}

	return formatted
}

func formatTaskDate(scheduledAt int64) string {
	today := Now().Truncate(24 * time.Hour)
	taskDate := time.Unix(scheduledAt, 0).In(Now().Location()).Truncate(24 * time.Hour)

	diffDays := int(taskDate.Sub(today).Hours() / 24)

	switch {
	case diffDays == 0:
		return "Today"
	case diffDays == 1:
		return "Tomorrow"
	case diffDays > 1 && diffDays <= 6: // Nearest day
		return taskDate.Format("Monday 02")
	case diffDays >= 7 && diffDays <= 13:
		return "Next " + taskDate.Format("Monday 02")
	default:
		return taskDate.Format("02 January, Monday")
	}
}
