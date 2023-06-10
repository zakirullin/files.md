// Package userconfig stores user's configuration in file.
// It stores such settings for users as: language, home, quick buttons, schedule and so on.
package userconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

var DefaultConfig = Config{
	config: config{
		Language:         "en",
		HomeCmd:          "today",
		MoveToButtons:    []string{"tomorrow", "later", "day", "note", "checklist", "doc", "recent", "journal"},
		PomodoroDuration: strDuration(25 * time.Minute),
	},
}

var TasksOnlyConfig = Config{
	config: config{
		HomeCmd:       "today",
		MoveToButtons: []string{"tomorrow", "later", "day"},
	},
}

var NotesOnlyConfig = Config{
	config: config{
		HomeCmd:       "notes",
		MoveToButtons: []string{"##NOTE_DIRS##"},
	},
}

type Config struct {
	config
}

type config struct {
	Language         string      `json:"language"`
	HomeCmd          string      `json:"homeCmd"`
	MoveToButtons    []string    `json:"moveToButtons"`
	PomodoroDuration strDuration `json:"pomodoroDuration"`
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &c.config)
}

// TODO add file creation
func (c *Config) LoadOrCreate(path string) error {
	configFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config.LoadOrCreate: %w", err)
	}
	defer configFile.Close()

	bytes, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("config.LoadOrCreate: %w", err)
	}

	err = json.Unmarshal(bytes, c)
	if err != nil {
		return fmt.Errorf("config.LoadOrCreate: can't unmarshal: %w", err)
	}

	return nil
}

func (c *Config) MoveToButtons() {

}

func (c *Config) Schedule() {

}

func (c *Config) Merge(config Config) {

}

func (c *Config) Save(path string) {

}

func mapConfigButtonNamesToRealNames(configNames []string) []string {
	configToReal := map[string]string{
		"tomorrow":  "🌚 For tmrw",
		"later":     "⏳ For later",
		"day":       "📆 For a day",
		"note":      "📌 To Note",
		"checklist": "☑️ To Checklist",
		"doc":       "📝 To Doc",
	}

	var realNames []string
	for _, configName := range configNames {
		realName, ok := configToReal[configName]
		if !ok {
			continue
		}

		realNames = append(realNames, realName)
	}

	return realNames
}

type strDuration time.Duration

func (d *strDuration) UnmarshalJSON(b []byte) error {
	dd := (*time.Duration)(d)
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*dd = time.Duration(value)
		return nil
	case string:
		var err error
		*dd, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}
func (c *Config) SetPomodoroDuration(value time.Duration) {
	c.config.PomodoroDuration = strDuration(value)
}

func (c *Config) PomodoroDuration() time.Duration {
	return time.Duration(c.config.PomodoroDuration)
}
