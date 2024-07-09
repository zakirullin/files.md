// Package userconfig stores user's configuration in file.
// It stores such settings for users as: language, home, quick buttons, schedule and so on.
package userconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal/constants"
	"zakirullin/stuffbot/internal/fs"
)

var DefaultConfig = Config{ // TODO apply default config if some fields are missing
	raw: raw{
		Language:               "en",
		HomeCmd:                "today",
		MoveToCommands:         []string{constants.CmdTomorrow, constants.CmdLater, "day", "note", "checklist", "doc", "recent", "journal"}, // TODO replace with constants
		PomodoroDurationMinute: 25,
		Schedules:              []Schedule{},
		JournalFilenameFormat:  "January 2006.md",
		JournalHeaderFormat:    "02, Monday",
		QuickPanelCommands:     []string{},
	},
}

var TasksOnlyConfig = Config{
	raw: raw{
		HomeCmd:        "today",
		MoveToCommands: []string{"tomorrow", "later", "day"},
	},
}

var NotesOnlyConfig = Config{
	raw: raw{
		HomeCmd:        "notes",
		MoveToCommands: []string{"##NOTE_DIRS##"},
	},
}

type Config struct {
	raw
}

type Schedule struct {
	Filename   string
	ScheduleAt int64
	Cron       string
	Cmd        string // For future use
}

type raw struct {
	Language               string     `json:"language"`
	HomeCmd                string     `json:"homeCmd"`
	MoveToCommands         []string   `json:"moveToCommands"`
	PomodoroDurationMinute float64    `json:"pomodoroDurationMinute"`
	JournalFilenameFormat  string     `json:"journalFilename"`
	JournalHeaderFormat    string     `json:"journalHeaderFormat"`
	Schedules              []Schedule `json:"schedules"`
	QuickPanelCommands     []string   `json:"quickPanelCommands"`
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) LoadOrCreate(path string) error {
	exists, err := fs.Exists(path)
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}

	if !exists {
		c.raw = DefaultConfig.raw
		return nil
	}

	configFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}
	defer configFile.Close()

	bytes, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}

	err = json.Unmarshal(bytes, c)
	if err != nil {
		return fmt.Errorf("config load: can't unmarshal: %w", err)
	}

	return nil
}

func (c *Config) Save(path string) error { // TODO add lazy saving, save only if config was changed
	bytes, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return fmt.Errorf("config save: can't marshal config: %w", err)
	}

	err = fs.WriteFile(path, bytes, 0644)
	if err != nil {
		return fmt.Errorf("config save: can't write config file: %w", err)
	}

	return nil
}

func (c *Config) MoveToCmds() []string {
	configToReal := map[string]string{
		"tomorrow":  i18n.StrForTomorrow,
		"later":     i18n.StrForLater,
		"day":       i18n.StrForDay,
		"note":      i18n.StrToNote,
		"checklist": i18n.StrToChecklist,
		"doc":       i18n.StrToFile,
		"journal":   i18n.StrToJournal,
	}

	var realCmds []string
	for _, configName := range c.raw.MoveToCommands {
		realName, ok := configToReal[configName]
		if !ok {
			continue
		}

		realCmds = append(realCmds, realName)
	}

	return realCmds
}

func (c *Config) SetPomodoroDuration(value time.Duration) error {
	if value <= 0 || value > 24*time.Hour {
		return fmt.Errorf("raw.SetPomodoroDuration: value is invalid: %v", value)
	}
	c.raw.PomodoroDurationMinute = value.Minutes()
	return nil
}

func (c *Config) PomodoroDuration() time.Duration {
	minutes := c.raw.PomodoroDurationMinute
	if minutes <= 0 {
		slog.Error("Pomodoro duration is invalid. Using default value", "duration",
			c.raw.PomodoroDurationMinute, "default", DefaultConfig.raw.PomodoroDurationMinute)
		//I don't use DefaultConfig.PomodoroDuration() because it may cause infinite recursion
		minutes = DefaultConfig.raw.PomodoroDurationMinute
	}
	return time.Duration(minutes * float64(time.Minute))
}
func (c *Config) Schedules() []Schedule {
	return c.raw.Schedules
}

// AddToSchedule task from _archive_ or later at scheduleAt (Unix timestamp, sec). Tasks appear in today folder.
// If cron is provided this task will be repeated. Other wise, it will be executed once.
func (c *Config) AddToSchedule(filename string, scheduleAt int64, cron string) {
	c.raw.Schedules = append(c.raw.Schedules, Schedule{filename, scheduleAt, cron, ""})
}

func (c *Config) DelFromSchedule(filename string) {
	var newSchedules []Schedule
	for _, schedule := range c.raw.Schedules {
		if schedule.Filename != filename {
			newSchedules = append(newSchedules, schedule)
		}
	}
}

func (c *Config) JournalFilenameFormat() string {
	if c.raw.JournalFilenameFormat == "" {
		return DefaultConfig.raw.JournalFilenameFormat
	}
	return c.raw.JournalFilenameFormat
}

func (c *Config) SetJournalFilenameFormat(path string) {
	c.raw.JournalFilenameFormat = path
}

func (c *Config) JournalHeaderFormat() string {
	if c.raw.JournalHeaderFormat == "" {
		return DefaultConfig.raw.JournalHeaderFormat
	}
	return c.raw.JournalHeaderFormat
}

func (c *Config) SetJournalHeaderFormat(format string) {
	c.raw.JournalHeaderFormat = format
}

func (c *Config) AddPanelButton(button string) bool {
	// Does this button already exist?
	for _, curBtn := range c.raw.QuickPanelCommands {
		if curBtn == button {
			return false
		}
	}
	c.raw.QuickPanelCommands = append(c.raw.QuickPanelCommands, button)
	return true
}

func (c *Config) HasQuickPanelCmd(cmd string) bool {
	for _, pref := range c.raw.QuickPanelCommands {
		if cmd == pref {
			return true
		}
	}
	return false
}

func (c *Config) DelPanelButton(toDelete string) bool {
	var newButtons []string
	var found = false // Was the target
	for _, curBtn := range c.raw.QuickPanelCommands {
		if curBtn == toDelete {
			found = true
		} else {
			newButtons = append(newButtons, curBtn)
		}
	}
	c.raw.QuickPanelCommands = newButtons
	return found
}
