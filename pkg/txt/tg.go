package txt

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramEntitiesToMarkdown converts plain text with Telegram entities (with UTF-16 offsets) to CommonMark Markdown.
// Telegram's formatting entities don't take the new lines into account. I.e. if we have a multiline
// bold text, it would be referred as a single bold entity, which is not what we want. This function
// inserts the necessary closing tags before the new lines and opening tags after the new lines.
// https://core.telegram.org/bots/api#messageentity
// https://commonmark.org/help/
func TelegramEntitiesToMarkdown(text string, messageEntities []tgbotapi.MessageEntity) string {
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
		eatNewlines := false // This flag will tell us whether to preserve newlines or not

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
			eatNewlines = true // For preformatted code, we will eat the newlines
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
			if c == '\n' && !eatNewlines && isOpen {
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
		output = append(output, c)
		utf16pos += len(utf16.Encode([]rune{c}))
	}
	output = append(output, []rune(insertions[utf16pos])...)

	return string(output)
}

func ExtractTextImgsLinks(text string) (txt string, images []string, links map[string]string) {
	links = make(map[string]string)

	imgRegexp := regexp.MustCompile(`!\[\[.*?tg_([^.]+)\..*?\]\]`)
	linkRegexp := regexp.MustCompile(`\[\[(.+?)\]\]`)

	// Eat bottom links
	text = NormNewLines(text)
	lines := strings.Split(text, "\n")
	var processedLines []string
	for _, line := range lines {
		// If the line contains only a link reference, ignore it for now
		trimmedLine := strings.TrimSpace(line)
		if !linkRegexp.MatchString(trimmedLine) || trimmedLine != linkRegexp.FindString(line) {
			processedLines = append(processedLines, line)
		} else {
			matches := linkRegexp.FindStringSubmatch(line)
			if len(matches) == 2 {
				content := matches[1]
				parts := strings.SplitN(content, "|", 2)
				linkPath := parts[0]
				linkLabel := filepath.Base(linkPath)
				links[linkLabel] = linkPath
			}
		}
	}
	text = strings.Join(processedLines, "\n")

	// Process images
	text = imgRegexp.ReplaceAllStringFunc(text, func(match string) string {
		matches := imgRegexp.FindStringSubmatch(match)
		if len(matches) == 2 {
			images = append(images, matches[1])
			return "🖼"
		}
		return match
	})

	// Process inline links
	text = linkRegexp.ReplaceAllStringFunc(text, func(match string) string {
		matches := linkRegexp.FindStringSubmatch(match)
		if len(matches) == 2 {
			content := matches[1]
			parts := strings.SplitN(content, "|", 2)
			linkPath := parts[0]
			linkLabel := filepath.Base(linkPath)
			links[linkLabel] = linkPath

			return "`" + linkLabel + "`"
		}
		return match
	})

	return strings.TrimSpace(text), images, links
}
