package tg

import (
	"encoding/json"
)

type Row interface {
}

type Btn struct {
	Name string
	Cmd  Cmd
}

func NewBtn(name string, cmd Cmd) Btn {
	return Btn{name, cmd}
}

// TODO remove this unnecessary method
func NewRow(btns ...Btn) []Btn {
	return btns
}

type Cmd struct {
	Name   string   `json:"n"`
	Params []string `json:"p"`
	Type   string   `json:"-"`
}

func NewCmd(name string, params []string) Cmd {
	return Cmd{name, params, "cmd"}
}

func (c *Cmd) UnmarshalJSON(data []byte) error {
	// Unmarshal JSON to the alias
	type CmdAlias Cmd
	var ca CmdAlias

	if err := json.Unmarshal(data, &ca); err != nil {
		return err
	}

	ca.Type = "cmd"

	*c = Cmd(ca)

	return nil
}

// Keyboard is an abstraction over Telegram's inline keyboard
type Keyboard struct {
	Btns []Row
}

func NewKeyboard(rows []Row) *Keyboard {
	return &Keyboard{rows}
}

func (k *Keyboard) AddRow(r Row) {
	k.Btns = append(k.Btns, r)
}
