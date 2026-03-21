package userconfig

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/fs"
)

const defaultTestCfgWithNewMoveCmd = `{
    "language": "en",
    "timezone": "UTC",
    "moveToCommands": [
        "new_move_cmd"
    ],
    "pomodoroDurationInMinutes": 50,
    "schedules": [],
    "quickCommands": [],
    "twoEmojisEnabled": false,
    "mode": "full",
    "quickHabitsEnabled": false,
    "channels": []
}`

func TestAddAndDelMoveCmd(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	cfg := NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	err = cfg.AddMoveToCmd("new_move_cmd")
	r.NoError(err)

	cmds, err := cfg.MoveToCmds()
	r.NoError(err)
	r.Equal([]string{"new_move_cmd"}, cmds)

	c, err := userFS.Read("", "config.json")
	r.NoError(err)
	r.Equal(defaultTestCfgWithNewMoveCmd, c)

	err = cfg.DelMoveToCmd("new_move_cmd")
	r.NoError(err)

	cmds, err = cfg.MoveToCmds()
	r.NoError(err)
	r.Empty(cmds)

	c, err = userFS.Read("", "config.json")
	r.NoError(err)
	r.Equal(defaultTestCfg, c)
}
