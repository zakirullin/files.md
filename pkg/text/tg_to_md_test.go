package text

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/require"
)

func TestBold(t *testing.T) {
	r := require.New(t)

	text := "bold"
	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "bold", Offset: 0, Length: 4},
	}

	md := EntitiesToMarkdown(text, messageEntities)
	r.Equal("**bold**", md)
}

func TestItalic(t *testing.T) {
	r := require.New(t)

	text := "italic"
	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "italic", Offset: 0, Length: 6},
	}

	md := EntitiesToMarkdown(text, messageEntities)
	r.Equal("*italic*", md)
}

func TestBoldAndItalic(t *testing.T) {
	r := require.New(t)

	text := "BoldAndItalic"
	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "bold", Offset: 0, Length: 13},
		{Type: "italic", Offset: 0, Length: 13},
	}

	md := EntitiesToMarkdown(text, messageEntities)
	r.Equal("***BoldAndItalic***", md)
}

func TestBoldThenItalic(t *testing.T) {
	r := require.New(t)

	text := "bolditalic"
	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "bold", Offset: 0, Length: 4},
		{Type: "italic", Offset: 4, Length: 6},
	}

	md := EntitiesToMarkdown(text, messageEntities)
	r.Equal("**bold***italic*", md)
}

func TestLink(t *testing.T) {
	r := require.New(t)

	text := "l"
	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "text_link", Offset: 0, Length: 1, URL: "google.com"},
	}

	md := EntitiesToMarkdown(text, messageEntities)
	r.Equal("[l](google.com)", md)
}

func TestMultilineTextWithMarkdown(t *testing.T) {
	r := require.New(t)

	text := "header\nitalic\n\nAlso italic\n\nheader2\nitalic\ncode"
	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "bold", Offset: 0, Length: 7},
		{Type: "italic", Offset: 7, Length: 21},
		{Type: "bold", Offset: 28, Length: 8},
		{Type: "italic", Offset: 36, Length: 7},
		{Type: "code", Offset: 43, Length: 4},
	}

	markdown := EntitiesToMarkdown(text, messageEntities)
	expectedMarkdown := "**header**\n*italic*\n\n*Also italic*\n\n**header2**\n*italic*\n`code`"
	r.Equal(expectedMarkdown, markdown)
}

func TestSpacedItalic(t *testing.T) {
	r := require.New(t)
	text := "Header\nLeverage one Minute Praising instead"

	var messageEntities = []tgbotapi.MessageEntity{
		{Type: "italic", Offset: 16, Length: 20},
	}

	markdown := EntitiesToMarkdown(text, messageEntities)
	expectedMarkdown := "Header\nLeverage *one Minute Praising* instead"
	r.Equal(expectedMarkdown, markdown)
}
