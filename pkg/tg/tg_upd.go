package tg

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"zakirullin/stuffbot/pkg/txt"
)

var (
	errNoUserID    = errors.New("update should always have at least one userID")
	imageMimeTypes = []string{
		"image/gif",
		"image/jpeg",
		"image/pjpeg",
		"image/png",
		"image/tiff",
		"image/webp",
	}
)

// TGUpd is a simple wrapper over Telegram Update object
type TGUpd struct {
	raw tgbotapi.Update
}

func NewTGUpd(u tgbotapi.Update) *TGUpd {
	return &TGUpd{raw: u}
}

func (u *TGUpd) MsgText() string {
	if u.raw.Message != nil {
		return u.raw.Message.Text
	}

	return ""
}

func (u *TGUpd) UserID() int64 {
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

func (u *TGUpd) Cmd() *Cmd {
	if u.raw.CallbackQuery != nil {
		cmd := Cmd{}
		_ = json.Unmarshal([]byte(u.raw.CallbackQuery.Data), &cmd)

		return &cmd
	}

	for _, entity := range u.raw.Message.Entities {
		if entity.IsCommand() {
			slashedCommand := getTextByOffset(u.raw.Message.Text, entity.Offset, entity.Length)
			cmd := NewCmd(strings.TrimPrefix(slashedCommand, "/"), nil)

			text := strings.Replace(u.raw.Message.Text, slashedCommand, "", 1)
			text = txt.Ucfirst(strings.TrimSpace(text))
			cmd.Params = []string{text}

			return &cmd
		}
	}

	return nil
}

func (u *TGUpd) MsgEntities() []tgbotapi.MessageEntity {
	if u.raw.Message != nil {
		return u.raw.Message.Entities
	}

	return nil
}

func (u *TGUpd) CaptionEntities() []tgbotapi.MessageEntity {
	if u.raw.Message != nil {
		if (u.raw.Message.CaptionEntities) != nil {
			return u.raw.Message.CaptionEntities
		}
	}

	return nil
}

func (u *TGUpd) CallbackQueryID() (string, bool) {
	if u.raw.CallbackQuery == nil {
		return "", false
	}

	return u.raw.CallbackQuery.ID, true
}

func (u *TGUpd) InlineQueryID() (string, bool) {
	if u.raw.InlineQuery == nil {
		return "", false
	}

	return u.raw.InlineQuery.ID, true
}

func (u *TGUpd) InlineQuery() (string, bool) {
	if u.raw.InlineQuery == nil {
		return "", false
	}

	return u.raw.InlineQuery.Query, true
}

func (u *TGUpd) InlineQueryOffset() int {
	if u.raw.InlineQuery == nil {
		return 0
	}

	offset, _ := strconv.Atoi(u.raw.InlineQuery.Offset)

	return offset
}

func (u *TGUpd) IsForwarded() bool {
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

func (u *TGUpd) IsSentViaBot() bool {
	message := u.raw.Message
	if message == nil {
		return false
	}

	return message.ViaBot != nil
}

func (u *TGUpd) ReplyToMsgID() (int, bool) {
	message := u.raw.Message
	if message == nil {
		return 0, false
	}

	if message.ReplyToMessage == nil {
		return 0, false
	}

	return message.ReplyToMessage.MessageID, true
}

func (u *TGUpd) PhotoOrImageID() (string, bool) {
	photoID, found := u.photoID()
	if found {
		return photoID, true
	}

	imageID, found := u.imageID()
	if found {
		return imageID, true
	}

	return "", false
}

// Caption returns the caption for the animation, audio,
// document, paid media, photo, video or voice
func (u *TGUpd) Caption() string {
	message := u.raw.Message
	if message == nil {
		return ""
	}

	return message.Caption
}

func (u *TGUpd) photoID() (string, bool) {
	message := u.raw.Message
	if message == nil {
		return "", false
	}

	if message.Photo == nil {
		return "", false
	}

	// Pick the photo with the maximum size, as FakeTG
	// makes some small crops
	photoSize := 0
	photoID := ""
	found := false
	for _, photo := range message.Photo {
		if photo.FileSize > photoSize {
			photoSize = photo.FileSize
			photoID = photo.FileID
			found = true
		}
	}

	return photoID, found
}

func (u *TGUpd) imageID() (string, bool) {
	message := u.raw.Message
	if message == nil {
		return "", false
	}

	if message.Document == nil {
		return "", false
	}

	if slices.Contains(imageMimeTypes, message.Document.MimeType) {
		return message.Document.FileID, true
	}

	return "", false
}

func (u *TGUpd) MsgID() (int, bool) {
	if u.raw.Message != nil {
		return u.raw.Message.MessageID, true
	}

	return 0, false
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

	runeString := utf16.Decode(utfEncodedString[offset : offset+length])

	return string(runeString)
}
