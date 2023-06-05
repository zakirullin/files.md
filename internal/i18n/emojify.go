package i18n

import (
	"fmt"
	"strings"
)

func Emojify(str string) string {
	strLower := strings.ToLower(str)
	aliases := []string{strLower, strLower + "s", strings.TrimSuffix(strLower, "s")}
	for _, alias := range aliases {
		icon, _ := emojisByKeyword[alias]
		if icon != "" {
			return fmt.Sprintf("%s %s", icon, str)
		}
	}

	return str
}
