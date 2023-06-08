package tg

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var errNoUserID = errors.New("update should always have at least one userID")

// Upd is a simple wrapper over Telegram Update object
type Upd struct {
	raw tgbotapi.Update
}

func NewUpd(u tgbotapi.Update) *Upd {
	return &Upd{raw: u}
}

func (u *Upd) MsgText() string {
	if u.raw.Message != nil {
		return u.raw.Message.Text
	}

	return ""
}

func (u *Upd) UserID() int64 {
	switch {
	case u.raw.Message != nil:
		return u.raw.Message.Chat.ID
	case u.raw.CallbackQuery != nil && u.raw.CallbackQuery.Message != nil && u.raw.CallbackQuery.Message.Chat != nil:
		return u.raw.CallbackQuery.Message.Chat.ID
	case u.raw.InlineQuery != nil && u.raw.InlineQuery.From != nil:
		return u.raw.InlineQuery.From.ID
	default:
		panic(errNoUserID)
	}
}

func (u *Upd) Cmd() *Cmd {
	if u.raw.CallbackQuery != nil {
		cmd := Cmd{}
		_ = json.Unmarshal([]byte(u.raw.CallbackQuery.Data), &cmd)

		return &cmd
	}

	for _, entity := range u.raw.Message.Entities {
		if entity.IsCommand() {
			slashedCommand := getTextByOffset(u.raw.Message.Text, entity.Offset, entity.Length)
			cmd := NewCmd(strings.TrimPrefix(slashedCommand, "/"), nil)
			return &cmd
		}
	}

	return nil
}

func (u *Upd) MsgEntities() []tgbotapi.MessageEntity {
	if u.raw.Message != nil {
		return u.raw.Message.Entities
	}

	return nil
}

func (u *Upd) CallbackQueryID() (string, bool) {
	if u.raw.CallbackQuery == nil {
		return "", false
	}

	return u.raw.CallbackQuery.ID, true
}

func (u *Upd) InlineQueryID() (string, bool) {
	if u.raw.InlineQuery == nil {
		return "", false
	}

	return u.raw.InlineQuery.ID, true
}

func (u *Upd) InlineQuery() (string, bool) {
	if u.raw.InlineQuery == nil {
		return "", false
	}

	return u.raw.InlineQuery.Query, true
}

func (u *Upd) IsForwarded() bool {
	message := u.raw.Message
	if message == nil {
		return false
	}

	if message.ForwardFromMessageID != 0 {
		return true
	}

	if message.ForwardFrom != nil {
		return true
	}

	if message.ForwardSenderName != "" {
		return true
	}

	return false
}

// Takes into account Telegram's UTF-16 encoding
// First we encode runes [128078 127997] into UTF-16 representation
// We get string [55357 56398 55356 57341]
// Then we decode this string back to runes
//
// A Unicode code point (rune) is the numerical value assigned to each character in the Unicode standard.
// Think of runes like this: just as each letter in the alphabet has a unique position or index,
// Unicode assigns a unique number to every character it includes, regardless of the writing system
// or language it belongs to. This number is called the rune (code point).
func getTextByOffset(text string, offset, length int) string {
	utfEncodedString := utf16.Encode([]rune(text))
	runeString := utf16.Decode(utfEncodedString[offset:length])

	return string(runeString)
}
