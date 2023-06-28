package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalCmdJSON(t *testing.T) {
	r := require.New(t)

	cmd := NewCmd("test", []string{"1"})
	js, err := json.Marshal(cmd)

	r.NoError(err)
	r.Equal(`{"n":"test","p":["1"]}`, string(js))
}

func TestUnmarshalCmdJSON(t *testing.T) {
	r := require.New(t)

	var cmd Cmd
	js := `{"n":"test","p":["1"]}`
	err := json.Unmarshal([]byte(js), &cmd)

	r.NoError(err)
	r.Equal("cmd", cmd.Type)
}

func TestRowAddBtn(t *testing.T) {
	r := require.New(t)

	row := NewRow()
	row = append(row, NewBtn("test", NewCmd("cmd", nil)))
	row = append(row, NewBtn("test2", NewCmd("cmd2", nil)))

	expectedBtns := []Btn{NewBtn("test", NewCmd("cmd", nil)), NewBtn("test2", NewCmd("cmd2", nil))}
	r.Equal(expectedBtns, row)
}
