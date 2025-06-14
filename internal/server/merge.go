package server

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

const (
	header = `^#### \d+ \w+, \w+`
)

// Merge combines two strings (s1 and s2) by identifying longest sequences of common lines.
//
// The algorithm:
// 1) Splits both inputs into lines
// 2) Uses dynamic programming to find the longest common subsequence (LCS) between every two lines
// 3) Constructs a merged result that preserves all unique content from both strings
// 4) Maintains the original order of content from both strings
// TODO add support for json merging
func Merge(s1, s2 string) string {
	if len(s1) == 0 {
		return s2
	}
	if len(s2) == 0 {
		return s1
	}
	lines1 := strings.Split(s1, "\n")
	lines2 := strings.Split(s2, "\n")

	// Dynamical programming table containing the longest common prefix for each pair.
	lcsLength := make([][]int, len(lines1)+1)
	for i := range lcsLength {
		lcsLength[i] = make([]int, len(lines2)+1)
	}

	// Fill the lcsLength table.
	for i := 1; i <= len(lines1); i++ {
		for j := 1; j <= len(lines2); j++ {
			if lines1[i-1] == lines2[j-1] {
				lcsLength[i][j] = lcsLength[i-1][j-1] + 1
			} else {
				lcsLength[i][j] = max(lcsLength[i-1][j], lcsLength[i][j-1])
			}
		}
	}

	// Build the merged result.
	result := backtrack(lines1, lines2, lcsLength, len(lines1), len(lines2))
	result = mergeEmojisInJournalHeaders(result)

	return strings.Join(result, "\n")
}

// backtrack performs backtracking through the dynamic programming table lcsLength
// to construct the merged result based on the longest common subsequence (LCS).
func backtrack(lines1, lines2 []string, lcsLength [][]int, i, j int) []string {
	if i == 0 && j == 0 {
		return []string{}
	}

	if i == 0 {
		return append(backtrack(lines1, lines2, lcsLength, i, j-1), lines2[j-1])
	}

	if j == 0 {
		return append(backtrack(lines1, lines2, lcsLength, i-1, j), lines1[i-1])
	}

	// If the current lines are the same, include it only once.
	if lines1[i-1] == lines2[j-1] {
		return append(backtrack(lines1, lines2, lcsLength, i-1, j-1), lines1[i-1])
	}

	// Choose the direction with the longer common subsequence.
	if lcsLength[i-1][j] > lcsLength[i][j-1] {
		return append(backtrack(lines1, lines2, lcsLength, i-1, j), lines1[i-1])
	} else {
		return append(backtrack(lines1, lines2, lcsLength, i, j-1), lines2[j-1])
	}
}

// Headers like this should be merged:
// #### 23 May, Friday
// #### 23 May, Friday 🤸‍
// #### 23 May, Friday 🤸‍🍽
// #### 23 May, Friday 🤸‍🍽💪
// #### 23 May, Friday 🤸‍🍽💪💧
// #### 23 May, Friday 🤸‍🍽💪💧🚶‍♂️
func mergeEmojisInJournalHeaders(lines []string) []string {
	var mergedLines []string
	groups := groupConsecutiveHeaders(lines)
	for _, group := range groups {
		if len(group) == 1 {
			mergedLines = append(mergedLines, group[0])
			continue
		}

		possibleEmojis := regexp.MustCompile(` [^\w\s\p{P}]+$`)
		date := possibleEmojis.ReplaceAllString(group[0], "")
		prefixIsSame := true
		for _, line := range group {
			emojis := possibleEmojis.FindString(line)
			if date+emojis != line {
				prefixIsSame = false
			}
		}
		// If at least one line from group doesn't start with the same date, we can't merge them.
		if !prefixIsSame {
			mergedLines = append(mergedLines, group...)
			continue
		}

		foundEmojis := ""
		for _, line := range group {
			foundEmojis += strings.TrimSpace(possibleEmojis.FindString(line))
		}
		if foundEmojis != "" {
			foundEmojis = " " + unique(foundEmojis)
		}
		mergedLines = append(mergedLines, date+foundEmojis)
	}

	return mergedLines
}

func groupConsecutiveHeaders(lines []string) [][]string {
	re := regexp.MustCompile(header)
	var groups [][]string
	i := 0
	for i < len(lines) {
		if re.MatchString(lines[i]) {
			var group []string
			for i < len(lines) && re.MatchString(lines[i]) {
				group = append(group, lines[i])
				i++
			}
			groups = append(groups, group)
		} else {
			groups = append(groups, []string{lines[i]})
			i++
		}
	}

	return groups
}

// unique returns a string containing unique unicode graphemes from both input strings.
// The order is preserved.
func unique(s string) string {
	var uniq string
	graphemes := uniseg.NewGraphemes(s)
	for graphemes.Next() {
		g := graphemes.Str()
		if !strings.Contains(uniq, g) {
			uniq += g
		}
	}

	return uniq
}
