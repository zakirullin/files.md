package tg

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Upd struct {
	userID           int64
	cmd              Cmd
	Msg              string
	PhotoID          string
	PhotoCaption     string
	ReplyToMessageID int
	IsSentViaBotVal  bool
	InlineQueryVal   string
	IsInlineQueryVal bool
}

func NewUpd(userID int64, msg string) *Upd {
	return &Upd{
		userID:           userID,
		Msg:              msg,
		ReplyToMessageID: -1,
		IsSentViaBotVal:  false,
		InlineQueryVal:   "",
		IsInlineQueryVal: false,
	}
}

func NewFakeUpdCmd(id int64, cmd Cmd) *Upd {
	return &Upd{userID: id, cmd: cmd}
}

func (u *Upd) MsgText() string {
	return u.Msg
}

func (u *Upd) UserID() int64 {
	return u.userID
}

func (u *Upd) Cmd() *Cmd {
	if u.cmd.Name == "" {
		return nil
	}

	return &u.cmd
}

func (u *Upd) MsgEntities() []tgbotapi.MessageEntity {
	return nil
}

func (u *Upd) CaptionEntities() []tgbotapi.MessageEntity {
	return nil
}

func (u *Upd) CallbackQueryID() (string, bool) {
	return "", true
}

func (u *Upd) InlineQueryID() (string, bool) {
	return "", false
}

func (u *Upd) InlineQuery() (string, bool) {
	return u.InlineQueryVal, u.IsInlineQueryVal
}

func (u *Upd) InlineQueryOffset() int {
	return 0
}

func (u *Upd) IsForwarded() bool {
	return false
}

func (u *Upd) IsSentViaBot() bool {
	return u.IsSentViaBotVal
}

func (u *Upd) ReplyToMsgID() (int, bool) {
	return u.ReplyToMessageID, u.ReplyToMessageID != -1
}

func (u *Upd) PhotoOrImageID() (string, bool) {
	if u.PhotoID != "" {
		return u.PhotoID, true
	}

	return "", false
}

func (u *Upd) Caption() string {
	return u.PhotoCaption
}

func (u *Upd) MsgID() (int, bool) {
	return 0, false
}
