package txt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func I64(i int64) string {
	return strconv.FormatInt(i, 10)
}

func Ucfirst(str string) string {
	for _, v := range str {
		u := string(unicode.ToUpper(v))
		return u + str[len(u):]
	}
	return ""
}

func Lcfirst(str string) string {
	for _, v := range str {
		u := string(unicode.ToLower(v))
		return u + str[len(u):]
	}
	return ""
}

// Substr respects unicode codepoints, but not multi-unicode-codepoint aware.
// Specifying skintone or gender of an emoji will count as 2 codepoints:
// https://unicode.org/emoji/charts/full-emoji-modifiers.html
func Substr(input string, start int, length int) string {
	asRunes := []rune(input)
	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}

func Emoji(emoji, str string) string {
	if emoji == "" {
		return str
	}

	return fmt.Sprintf("%s %s", emoji, str)
}

func NormNewLines(text string) string {
	text = strings.Replace(text, "\\r\\n", "\n", -1)
	return strings.Replace(text, "\\n\\r", "\n", -1)
}

// SplitTextIntoChunks splits the text into chunks less than or equal to maxLen.
// The chunks are split at the last new line or space before maxLen.
// Spaces-like characters are trimmed out from the beginning and the end of each chunk.
func SplitTextIntoChunks(text string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{text}
	}

	var chunks []string
	runes := []rune(strings.TrimSpace(text)) // Convert the string to runes

	for len(runes) > maxLen {
		subStr := runes[:maxLen]

		// Find the last newline in the substring
		splitIndex := -1
		for i := len(subStr) - 1; i >= 0; i-- {
			if subStr[i] == '\n' {
				splitIndex = i
				break
			}
		}

		if splitIndex == -1 {
			// No newline found, find the last space
			for i := len(subStr) - 1; i >= 0; i-- {
				if subStr[i] == ' ' {
					splitIndex = i
					break
				}
			}
			if splitIndex == -1 {
				// No space found either, split at maxLen
				splitIndex = maxLen
			}
		}

		// Add the chunk to the list
		trimmedSubStr := strings.TrimSpace(string(runes[:splitIndex]))
		if len(trimmedSubStr) > 0 {
			chunks = append(chunks, trimmedSubStr)
		}
		// Move the pointer forward
		runes = runes[splitIndex:]
		// Prevent leading spaces
		runes = []rune(strings.TrimSpace(string(runes)))
	}

	// Add the remaining runes as the final chunk
	chunks = append(chunks, strings.TrimSpace(string(runes)))

	return chunks
}

func InsertTextAfterHeader(existingContent, header, newContent string) string {
	if !strings.Contains(existingContent, header) {
		return strings.TrimSpace(fmt.Sprintf("%s\n%s\n%s", header, newContent, existingContent))
	}

	headerAndContent := fmt.Sprintf("%s\n%s", header, newContent)
	content := strings.Replace(existingContent, header, headerAndContent, 1)

	return strings.TrimSpace(content)
}

// TODO add tests
func FirstWord(str string) string {
	str = strings.TrimSpace(str)
	re := regexp.MustCompile(`^[^\s\p{P}]+`)
	return re.FindString(str)
}

// TODO add tests
func EscapeHTML(str string) string {
	// HTML escaping
	var htmlEscaper = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)

	return htmlEscaper.Replace(str)
}

func StripHTMLTags(str string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(str, "")
}

// TODO add tests
func ReplaceWithPlaceholders(str, regex, placeholder string) (string, map[string]string) {
	re := regexp.MustCompile(regex)
	placeholders := make(map[string]string)
	counter := 0

	// Function to replace each match with a placeholder
	res := re.ReplaceAllStringFunc(str, func(match string) string {
		p := fmt.Sprintf("#%s%d#", placeholder, counter)
		placeholders[p] = match
		counter++
		return p
	})

	return res, placeholders
}

func RestoreFromPlaceholders(str string, placeholders map[string]string) string {
	for placeholder, original := range placeholders {
		str = strings.ReplaceAll(str, placeholder, original)
	}
	return str
}

// TODO add tests
func SplitLongLines(input string, maxRunesPerLine int) string {
	var res strings.Builder
	lines := strings.Split(input, "\n")

	for _, line := range lines {
		runeCount := utf8.RuneCountInString(line)
		if runeCount > maxRunesPerLine {
			runes := []rune(line)
			for i := 0; i < len(runes); i += maxRunesPerLine {
				end := i + maxRunesPerLine
				if end > len(runes) {
					end = len(runes)
				}
				res.WriteString(string(runes[i:end]) + "\n")
			}
		} else {
			res.WriteString(line + "\n")
		}
	}

	return res.String()
}
