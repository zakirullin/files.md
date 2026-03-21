package userconfig

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/server/fs"
)

const defaultTestCfg = `{
    "language": "en",
    "timezone": "UTC",
    "moveToCommands": [],
    "pomodoroDurationInMinutes": 50,
    "schedules": [],
    "quickCommands": [],
    "twoEmojisEnabled": false,
    "mode": "full",
    "quickHabitsEnabled": false,
    "channels": []
}`

const timezoneTestCfg = `{
    "language": "en",
    "timezone": "Europe/Nicosia",
    "moveToCommands": [
        "sc_tmrw",
        "later",
        "sc_day",
        "to_file",
        "mv_to_journal"
    ],
    "pomodoroDurationInMinutes": 50,
    "schedules": [],
    "quickCommands": [],
    "allowTwoEmojisInButton": false,
    "mode": "tasks"
}`

const invalidTimezoneTestCfg = `{
    "language": "en",
    "timezone": "invalid/timezone",
    "moveToCommands": [
        "sc_tmrw",
        "later",
        "sc_day",
        "to_file",
        "mv_to_journal"
    ],
    "pomodoroDurationInMinutes": 50,
    "schedules": [],
    "quickCommands": [],
    "allowTwoEmojisInButton": false
    "mode": "tasks"
}`

func TestCreateDefaultIfNotExists(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	cfg := NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	c, err := userFS.Read("", "config.json")
	r.NoError(err)

	r.Equal(defaultTestCfg, c)
}

func TestCreateDefaultIfNotExistsExistingConfig(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "config.json", "invalid json")
	r.NoError(err)

	cfg := NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	c, err := userFS.Read("", "config.json")
	r.NoError(err)

	r.Equal("invalid json", c)
}

func TestTimezone(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "config.json", timezoneTestCfg)
	r.NoError(err)

	cfg := NewConfig(userFS, -1, "config.json")
	r.NoError(err)

	tz := cfg.Timezone()
	r.Equal("Europe/Nicosia", tz.String())
}

func TestTimezoneInvalid(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "config.json", invalidTimezoneTestCfg)
	r.NoError(err)

	cfg := NewConfig(userFS, -1, "config.json")
	r.NoError(err)

	tz := cfg.Timezone()
	r.Equal("UTC", tz.String())
}
