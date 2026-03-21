package habits

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rivo/uniseg"

	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/bot/fs"
	"zakirullin/stuffbot/pkg/txt"
)

// [1 => false, <year day> => 0, 1...]
type Year map[int]int

const (
	habitSkipped            = "⚪️"
	habitCompleted          = "🟢"
	habitCompletedAtWeekend = "🟡"

	MoodHabit = "Mood"
)

var (
	MoodEmojis            = []string{"⚪️", "🤕", "😔", "😐", "🙂", "😊"}
	errMalformedMonthLine = errors.New("malformed month line")
	now                   = time.Now
)

// Habits returns Habit name => [day1 => 1, day2 => 0, ..., day365 => 0]
func Habits(userFS *fs.FS, year int) (map[string]Year, error) {
	existingHabits, err := userFS.FilesAndDirs(fs.DirHabits)
	if err != nil {
		return nil, fmt.Errorf("habits: can't read existing habits: %w", err)
	}

	habits := make(map[string]Year)
	for _, existingHabit := range existingHabits {
		habits[existingHabit.DisplayName] = make(Year)
	}

	filename := fmt.Sprintf("%d Habits.md", year)
	insightsExist, err := userFS.Exists(fs.DirInsights, filename)
	if err != nil {
		return nil, fmt.Errorf("habits: can't check whether the file insightsExist: %w", err)
	}
	if !insightsExist {
		return habits, nil
	}

	habitsForYearLines, err := userFS.Read(fs.DirInsights, filename)
	if err != nil {
		return nil, fmt.Errorf("habits:read %s error: %w", filename, err)
	}

	month := time.January
	lines := strings.Split(txt.NormNewLines(habitsForYearLines), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Parsing month line
		isMonthLine := strings.HasPrefix(line, "###")
		if isMonthLine {
			parts := strings.Split(line, " ")
			if len(parts) < 2 {
				return nil, fmt.Errorf("read habits: can't parse month line '%s': %w", line, errMalformedMonthLine)
			}

			// We should extract only first month:"June, some,gibberish"
			// TODO add tests
			date, err := time.Parse("January", txt.FirstWord(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("read habits: can't parse month %s: %w", line, err)
			}
			month = date.Month()

			continue
		}

		// Tolerant reader, if we encounter gibberish,
		// we skip it. See ADRs in README.md for details for details
		// TODO preserve gibberish between parsing seesions
		daysAndHabit := strings.SplitN(line, " ", 2)
		if len(daysAndHabit) < 2 {
			continue
		}
		days, habit := daysAndHabit[0], daysAndHabit[1]

		firstDayOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		dayOfTheYear := firstDayOfMonth.YearDay()

		// Moods line
		moodsMarker := MoodHabit
		if strings.Contains(habit, moodsMarker) {
			gr := uniseg.NewGraphemes(days)
			dayOffset := 0
			if _, ok := habits[MoodHabit]; !ok {
				habits[MoodHabit] = make(Year)
			}
			for gr.Next() {
				power := slices.Index(MoodEmojis, gr.Str())
				habits[MoodHabit][dayOfTheYear+dayOffset] = power
				dayOfTheYear++
			}
			continue
		}

		// Skip gibberish
		habitsMarker := fmt.Sprintf("%s%s%s", habitSkipped, habitCompletedAtWeekend, habitCompleted)
		if !strings.ContainsAny(days, habitsMarker) {
			continue
		}

		// Habits line
		// [⚪️🟢... Habit name] i.e. completion status
		// for every day of the above found month
		habitName := strings.TrimSpace(habit)
		if _, ok := habits[habitName]; !ok {
			habits[habitName] = make(Year)
		}

		// See README.md ADRs
		gr := uniseg.NewGraphemes(days)
		dayOffset := 0
		for gr.Next() {
			habits[habitName][dayOfTheYear+dayOffset] = 0
			if gr.Str() != habitSkipped {
				habits[habitName][dayOfTheYear+dayOffset] = 1
			}
			dayOfTheYear++
		}
	}

	return habits, nil
}

// LastWeekHabits returns Habit name => [day1 => 1, day2 => 0, ..., day365 => 0]
// with the year days falling in the last week
// FIXME doesn't work when a week falls into two years
func LastWeekHabits(userFS *fs.FS, tz *time.Location) (map[string]Year, error) {
	habitsForYear, err := Habits(userFS, now().In(tz).Year())
	if err != nil {
		return nil, fmt.Errorf("last week habits: can't get habits: %w", err)
	}

	currentDay := now().In(tz)
	for currentDay.Weekday() != time.Monday {
		currentDay = currentDay.Add(-24 * time.Hour)
	}

	existingHabits, err := userFS.FilesAndDirs(fs.DirHabits)
	if err != nil {
		return nil, fmt.Errorf("last week habits: can't read existing habits: %w", err)
	}
	// Add default mood habit which is not in habits folder
	existingHabits = append(existingHabits, fs.File{Name: MoodHabit})

	habits := make(map[string]Year)
	for _, habit := range existingHabits {
		habitName := strings.TrimSuffix(habit.Name, fs.MDExt)
		habits[habitName] = make(Year)
		for offset := range 7 {
			yearDay := currentDay.Add(time.Duration(offset) * 24 * time.Hour).YearDay()
			habits[habitName][yearDay] = 0
			if status, ok := habitsForYear[habitName][yearDay]; ok {
				habits[habitName][yearDay] = status
			}
		}
	}

	return habits, nil
}

func Write(userFS *fs.FS, year int, habits map[string]Year) error {
	habitKeys := make([]string, 0)
	for k := range habits {
		if k == MoodHabit {
			continue
		}
		habitKeys = append(habitKeys, k)
	}
	sort.Strings(habitKeys)
	if _, ok := habits[MoodHabit]; ok {
		habitKeys = append(habitKeys, MoodHabit)
	}

	content := ""
	day := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	for day.Year() < year+1 {
		habitsForMonth := ""
		for _, habitName := range habitKeys {
			statuses := ""
			dayOfMonth := day
			atLeastOneCompletion := false
			for dayOfMonth.Month() == day.Month() {
				emoji := habitSkipped
				if status, ok := habits[habitName][dayOfMonth.YearDay()]; ok {
					emoji = emojiForStatus(habitName, dayOfMonth, status)
				}
				if emoji != habitSkipped {
					atLeastOneCompletion = true
				}
				statuses += emoji

				dayOfMonth = dayOfMonth.AddDate(0, 0, 1)
			}
			if atLeastOneCompletion {
				habitsForMonth += fmt.Sprintf("%s %s\n", statuses, habitName)
			}
		}

		if len(habitsForMonth) != 0 {
			if len(content) > 0 {
				content += "\n"
			}
			content += fmt.Sprintf("### %s\n%s", day.Month(), habitsForMonth)
		}

		day = day.AddDate(0, 1, 0)
	}

	filename := fmt.Sprintf("%d Habits.md", year)
	err := userFS.Write(fs.DirInsights, filename, content)
	if err != nil {
		return fmt.Errorf("can't write habits: %w", err)
	}

	return nil
}

func Emoji(userFS *fs.FS, habitName string) string {
	emoji, _ := userFS.Read(fs.DirHabits, fs.Filename(habitName))
	if emoji == "" {
		emoji = i18n.Emoji(habitName)
	}
	if emoji == "" {
		emoji = "⚡️"
	}

	return emoji
}

func emojiForStatus(habitName string, day time.Time, status int) string {
	if habitName == MoodHabit {
		if status < len(MoodEmojis) {
			return MoodEmojis[status]
		}

		return habitSkipped
	}

	if status == 1 {
		isWeekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		if isWeekend {
			return habitCompletedAtWeekend
		} else {
			return habitCompleted
		}
	}

	return habitSkipped
}
