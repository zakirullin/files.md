package fake

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"zakirullin/stuffbot/pkg/tg"
)

type Upd struct {
	id  int64
	cmd tg.Cmd
	msg string
}

func NewUpd(id int64, msg string) *Upd {
	return &Upd{id: id, msg: msg}
}

func NewUpdCmdFake(id int64, cmd tg.Cmd) *Upd {
	return &Upd{id: id, cmd: cmd}
}

func (m *Upd) MsgText() string {
	return m.msg
}

func (m *Upd) UserID() int64 {
	return m.id
}

func (m *Upd) Cmd() *tg.Cmd {
	if m.cmd.Name == "" {
		return nil
	}

	return &m.cmd
}

func (m *Upd) MsgEntities() []tgbotapi.MessageEntity {
	return nil
}

func (m *Upd) CallbackQueryID() (string, bool) {
	return "", true
}

func (m *Upd) InlineQueryID() (string, bool) {
	return "", false
}

func (m *Upd) InlineQuery() (string, bool) {
	return "", false
}

func (m *Upd) IsForwarded() bool {
	return false
}

type TG struct {
	SentText       string
	EditedText     string
	SentKeyboard   *tg.Keyboard
	EditedKeyboard *tg.Keyboard
}

func NewTG() *TG {
	return &TG{}
}

func (f *TG) Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error) {
	f.SentText = text
	f.SentKeyboard = kb

	return -2, nil
}

func (f *TG) Edit(userID int64, msgID int, text string, kb *tg.Keyboard, markup string) error {
	f.EditedText = text
	f.EditedKeyboard = kb

	return nil
}

func (f *TG) Del(userID int64, msgID int) error {
	return nil
}

func (f *TG) AnswerCallbackQuery(queryID string, text string) error {
	return nil
}

func (f *TG) AnswerInlineQuery(queryID string, results []interface{}, cacheTime int, offset string) error {
	return nil
}
