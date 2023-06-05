package i18n

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var lang *i18n.Bundle
var emojisByKeyword map[string]string

// LoadLangFile only supports single language for now
func LoadLangFile(path string) error {
	lang = i18n.NewBundle(language.English)
	_, err := lang.LoadMessageFile(path)
	if err != nil {
		return fmt.Errorf("i18n.Load: %w", err)
	}

	return nil
}

func LoadEmojiFile(path string) error {
	emojiFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("i18n.LoadEmojiFile: %w", err)
	}
	defer emojiFile.Close()

	bytes, err := io.ReadAll(emojiFile)
	if err != nil {
		return fmt.Errorf("i18n.LoadEmojiFile: %w", err)
	}

	var emojis map[string][]string
	err = json.Unmarshal(bytes, &emojis)
	if err != nil {
		return fmt.Errorf("i18n.LoadEmojiFile: can't unmarshal: %w", err)
	}

	emojisByKeyword = make(map[string]string)
	for emoji, keywords := range emojis {
		for _, keyword := range keywords {
			emojisByKeyword[keyword] = emoji
		}
	}

	return nil
}

func Tr(str string) string {
	return str
}
