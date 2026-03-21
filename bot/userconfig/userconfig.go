// Package userconfig stores user's configuration in file.
// It stores such settings for users as: language, home, quick buttons, schedule and so on.
// We read every userconfig value from the config file on every access to prevent data race.
package userconfig

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"zakirullin/stuffbot/bot/fs"
)

const (
	ModeChat    = "chat"
	ModeFull    = "full"
	ModeTasks   = "tasks"
	ModeNotes   = "notes"
	ModeJournal = "journal"
)

var DefaultConfig = config{
	Language:                  "en",
	Timezone:                  "UTC",
	MoveToCmds:                []string{},
	PomodoroDurationInMinutes: 50,
	Schedules:                 []Schedule{},
	QuickCmds:                 []string{},
	TwoEmojisEnabled:          false,
	Mode:                      "full",
	QuickHabitsEnabled:        false,
	Channels:                  []int64{},
}

var (
	mu        sync.Mutex
	userLocks map[int64]*sync.Mutex
)

type Config struct {
	userFS   *fs.FS
	userID   int64
	filename string
}

type Schedule struct {
	Filename    string
	ScheduledAt int64
	Cron        string
	Cmd         string // For future use
}

type config struct {
	Language                  string     `json:"language"`
	Timezone                  string     `json:"timezone"`
	MoveToCmds                []string   `json:"moveToCommands"`
	PomodoroDurationInMinutes int64      `json:"pomodoroDurationInMinutes"`
	Schedules                 []Schedule `json:"schedules"`
	QuickCmds                 []string   `json:"quickCommands"`
	TwoEmojisEnabled          bool       `json:"twoEmojisEnabled"`
	Mode                      string     `json:"mode"`
	QuickHabitsEnabled        bool       `json:"quickHabitsEnabled"`
	Channels                  []int64    `json:"channels"`
}

func NewConfig(userFS *fs.FS, userID int64, filename string) *Config {
	return &Config{userFS: userFS, userID: userID, filename: filename}
}

func (c *Config) CreateDefaultIfNotExists() error {
	exists, err := c.userFS.Exists(fs.DirRoot, c.filename)
	if err != nil {
		return fmt.Errorf("can't check whether config exists: %w", err)
	}
	if exists {
		return nil
	}

	err = c.write(DefaultConfig)
	if err != nil {
		return fmt.Errorf("can't write default config file: %w", err)
	}

	return nil
}

func (c *Config) Timezone() *time.Location {
	cfg, _ := c.read(c.filename)

	if cfg.Timezone == "" {
		return time.UTC
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return time.UTC
	}

	return location
}

func (c *Config) SetTimezone(tz string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("set timezone: can't read config: %w", err)
	}
	cfg.Timezone = tz
	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("set timezone: can't write config: %w", err)
	}

	return nil
}

func (c *Config) ChatOnlyMode() bool {
	cfg, _ := c.read(c.filename)

	return cfg.Mode == ModeChat
}

func (c *Config) TasksOnlyMode() bool {
	cfg, _ := c.read(c.filename)

	return cfg.Mode == ModeTasks
}

// TODO release test everything in this mode
func (c *Config) NotesOnlyMode() bool {
	cfg, _ := c.read(c.filename)

	return cfg.Mode == ModeNotes
}

func (c *Config) JournalOnlyMode() bool {
	cfg, _ := c.read(c.filename)

	return cfg.Mode == ModeJournal
}

func (c *Config) SetMode(mode string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, _ := c.read(c.filename)
	cfg.Mode = mode
	err := c.write(cfg)
	if err != nil {
		return fmt.Errorf("set mode: can't write config: %w", err)
	}

	return nil
}

func (c *Config) PomodoroDuration() time.Duration {
	cfg, _ := c.read(c.filename)

	return time.Duration(cfg.PomodoroDurationInMinutes * int64(time.Minute))
}

func (c *Config) SetPomodoroDuration(duration time.Duration) error {
	if duration <= 0 || duration > 24*time.Hour {
		return fmt.Errorf("set pomodoro duration: duration is invalid: %v", duration)
	}

	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("set pomodoro duration: can't read config: %w", err)
	}
	cfg.PomodoroDurationInMinutes = int64(duration.Minutes())
	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("set pomodoro duration: can't write config: %w", err)
	}

	return nil
}

func (c *Config) Schedules() ([]Schedule, error) {
	cfg, err := c.read(c.filename)
	if err != nil {
		return nil, fmt.Errorf("can't get schedules: can't read config: %w", err)
	}

	schedules := cfg.Schedules
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].ScheduledAt > schedules[j].ScheduledAt
	})
	slices.Reverse(schedules)

	return schedules, nil
}

// AddToSchedule replaces existing schedule with the same filename
func (c *Config) AddToSchedule(filename string, scheduleAt int64, cron string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("can't add to schedule: can't read config: %w", err)
	}

	found := false
	for i := range cfg.Schedules {
		if cfg.Schedules[i].Filename == filename {
			cfg.Schedules[i].ScheduledAt = scheduleAt
			cfg.Schedules[i].Cron = cron
			found = true
			break
		}
	}
	if !found {
		cfg.Schedules = append(cfg.Schedules, Schedule{filename, scheduleAt, cron, ""})
	}

	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("can't add to schedule: can't write config: %w", err)
	}

	return nil
}

func (c *Config) DelFromSchedule(filename string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("can't del from schedule: can't read config: %w", err)
	}

	var newSchedules []Schedule
	for _, schedule := range cfg.Schedules {
		if schedule.Filename == filename {
			continue
		}
		newSchedules = append(newSchedules, schedule)
	}
	cfg.Schedules = newSchedules

	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("can't del from schedule: can't write config: %w", err)
	}

	return nil
}

func (c *Config) ShouldSplitChecklist(checklist string) bool {
	for _, unsplittableChecklist := range []string{fs.DirRead, fs.DirWatch} {
		if checklist == unsplittableChecklist {
			return false
		}
	}
	return true
}

func (c *Config) TwoEmojisPerButtonEnabled() bool {
	cfg, _ := c.read(c.filename)

	return cfg.TwoEmojisEnabled
}

func (c *Config) QuickHabitsEnabled() bool {
	cfg, _ := c.read(c.filename)

	return cfg.QuickHabitsEnabled
}

func (c *Config) Channels() []int64 {
	cfg, _ := c.read(c.filename)

	return cfg.Channels
}

func (c *Config) read(path string) (config, error) {
	exists, err := c.userFS.Exists(fs.DirRoot, path)
	if err != nil {
		return DefaultConfig, fmt.Errorf("config load: %w", err)
	}

	if !exists {
		return DefaultConfig, nil
	}

	content, err := c.userFS.Read(fs.DirRoot, c.filename)
	if err != nil {
		return DefaultConfig, fmt.Errorf("config load: %w", err)
	}

	cfg := config{}
	err = json.Unmarshal([]byte(content), &cfg)
	if err != nil {
		return DefaultConfig, fmt.Errorf("config load: can't unmarshal: %w", err)
	}

	return cfg, nil
}

func (c *Config) write(cfg config) error {
	bytes, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("config save: can't marshal config: %w", err)
	}

	err = c.userFS.Write(fs.DirRoot, c.filename, string(bytes))
	if err != nil {
		return fmt.Errorf("config save: can't write config file: %w", err)
	}

	return nil
}

func (c *Config) userLock() *sync.Mutex {
	mu.Lock()
	defer mu.Unlock()

	if userLocks == nil {
		userLocks = make(map[int64]*sync.Mutex)
	}
	if lock, exists := userLocks[c.userID]; exists {
		return lock
	}

	newLock := &sync.Mutex{}
	userLocks[c.userID] = newLock

	return newLock
}
