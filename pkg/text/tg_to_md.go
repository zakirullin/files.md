package text

import (
	"fmt"
	"unicode"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var needEscape = make(map[rune]struct{})

const RUNE_NEWLINE = '\n'

func init() {
	for _, r := range []rune{'_', '*', '~', '`'} { // '[', ']', '(', ')', '>', '#',  '+', '-', '=', '|', '{', '}', '.', '!'} {
		needEscape[r] = struct{}{}
	}
}

// EntitiesToMarkdown converts plain text with Telegram entities to CommonMark Markdown.
// Telegram's formatting entities don't take the new lines into account. I.e. if we have a multiline
// bold text, it would be referred as a single bold entity, which is not what we want. This function
// inserts the necessary closing tags before the new lines and opening tags after the new lines.
// https://core.telegram.org/bots/api#messageentity
// https://commonmark.org/help/
func EntitiesToMarkdown(text string, messageEntities []tgbotapi.MessageEntity) string {
	input := []rune(NormNewLines(text))
	insertions := make(map[int]string)
	noEscape := make(map[int]*struct{})
	strct := struct{}{}
	stopEscape := func(e *tgbotapi.MessageEntity) {
		for i := e.Offset; i < e.Offset+e.Length; i++ {
			noEscape[i] = &strct
		}
	}

	for _, e := range messageEntities {
		var before, after string

		if e.IsBold() {
			before = "**"
			after = "**"
		} else if e.IsItalic() {
			before = "*"
			after = "*"
		} else if e.Type == "underline" {
			before = "__"
			after = "__"
		} else if e.Type == "strikethrough" {
			before = "~"
			after = "~"
		} else if e.IsCode() {
			before = "`"
			after = "`"
			stopEscape(&e)
		} else if e.IsPre() {
			before = "```" + e.Language
			after = "```"
			stopEscape(&e)
		} else if e.IsTextLink() {
			before = "["
			after = fmt.Sprintf(`](%s)`, e.URL)
		} else if e.IsURL() {
			stopEscape(&e)
		}
		if before == "" {
			continue
		}

		isOpen := false
		spacesToEat := 0
		for offset, c := range input[e.Offset : e.Offset+e.Length] {
			if c == RUNE_NEWLINE && isOpen {
				insertions[(e.Offset+offset)-spacesToEat] += after
				isOpen = false
				spacesToEat = 0
				continue
			}
			if unicode.IsSpace(c) {
				spacesToEat++
				continue
			}
			if !isOpen {
				insertions[e.Offset+offset] += before
				isOpen = true
			}
			spacesToEat = 0
		}
		if isOpen {
			insertions[(e.Offset+e.Length)-spacesToEat] += after
		}
	}

	var output []rune
	utf16pos := 0
	for _, c := range input {
		output = append(output, []rune(insertions[utf16pos])...)
		_, stopEscaping := noEscape[utf16pos]
		if _, shouldEscape := needEscape[c]; shouldEscape && !stopEscaping {
			output = append(output, '\\')
		}
		output = append(output, c)
		utf16pos += len(utf16.Encode([]rune{c}))
	}
	output = append(output, []rune(insertions[utf16pos])...)

	return string(output)
}
