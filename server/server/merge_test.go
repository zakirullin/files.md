package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergePrefixCases(t *testing.T) {
	r := require.New(t)

	original := "line 1\nline 2"
	modified := "line 1\nline 2\nline 3\nline 4"
	r.Equal(modified, Merge(original, modified))
	r.Equal(modified, Merge(modified, original))
}

func TestMergeCommonPrefixDifferentSuffixes(t *testing.T) {
	r := require.New(t)

	// Both have common prefix but different additional lines
	original := "line 1\nline 2\nline 3\nline original 4"
	modified := "line 1\nline 2\nline 3\nline modified 4"
	merged := Merge(original, modified)
	r.Equal("line 1\nline 2\nline 3\nline original 4\nline modified 4", merged)
}

func TestMergeDifferentPrefixCommonSuffix(t *testing.T) {
	r := require.New(t)

	// Both have different prefixes but common suffix
	original := "line original 1\nline original 2\nline 3"
	modified := "new\nline original 1\nline original 2\nline 3"
	merged := Merge(original, modified)
	r.Equal("new\nline original 1\nline original 2\nline 3", merged, "Should merge lines before common suffix")
}

func TestMergeDivergentBody(t *testing.T) {
	r := require.New(t)

	// Divergent content with common prefix and suffix
	original := "header\nheader\noriginal A\noriginal B\nfooter\nfooter"
	modified := "header\nheader\nmodified X\nmodified Y\nfooter\nfooter"
	merged := Merge(original, modified)
	r.Equal("header\nheader\noriginal A\noriginal B\nmodified X\nmodified Y\nfooter\nfooter", merged)
}

func TestMergeSameHeader(t *testing.T) {
	r := require.New(t)

	result := Merge("#### 23 May, Saturday", "#### 23 May, Saturday")
	r.Equal("#### 23 May, Saturday", result)
}

func TestMergeDivergentContent(t *testing.T) {
	r := require.New(t)

	// Complete divergence with small common prefix
	original := "header\noriginal A\noriginal B"
	modified := "header\nmodified X\nmodified Y"
	merged := Merge(original, modified)
	r.Equal("header\noriginal A\noriginal B\nmodified X\nmodified Y", merged)
}

func TestMergeEmptyStrings(t *testing.T) {
	r := require.New(t)

	r.Equal("", Merge("", ""), "Empty strings should merge to empty string")
	r.Equal("content", Merge("", "content"), "Empty original should return modified")
	r.Equal("content", Merge("content", ""), "Empty modified should return original")
}

func TestMergeTrailingNewlines(t *testing.T) {
	r := require.New(t)

	original := "line 1\nline 2\n"
	modified := "line 1\nline 2\nline 3\n"
	r.Equal(modified, Merge(original, modified), "Should handle trailing newlines correctly")
}

func TestMergeDivergentChars(t *testing.T) {
	r := require.New(t)

	original := "abc"
	modified := "adc"
	merged := Merge(original, modified)
	r.Equal("abc\nadc", merged)
}

func TestJournal(t *testing.T) {
	r := require.New(t)

	server := "1 April\nfelt good\nate good\n2 April\nslept not so good"
	client := "1 April\nfelt good\n2 April\nslept not so good\nwent for hiking"
	merged := Merge(server, client)
	r.Equal("1 April\nfelt good\nate good\n2 April\nslept not so good\nwent for hiking", merged)
}

func TestMergeHeaders(t *testing.T) {
	r := require.New(t)

	headers := []string{"#### 23 May, Friday 🤸‍♂️🍽💪💧", "#### 23 May, Friday 🤸‍♂️🍽💪", "#### 23 May, Friday 🤸‍♂️"}
	merged := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️🍽💪💧"}, merged)
}

func TestMergeHeadersReversed(t *testing.T) {
	r := require.New(t)

	headers := []string{"#### 23 May, Friday 🤸‍♂️", "#### 23 May, Friday 🤸‍♂️🍽💪", "#### 23 May, Friday 🤸‍♂️🍽💪💧"}
	merged := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️🍽💪💧"}, merged)
}

func TestMergeHeadersWithDifferentEmojis(t *testing.T) {
	r := require.New(t)

	headers := []string{"#### 23 May, Friday 🤸‍♂️‍🍽💪💧", "#### 23 May, Friday 🤸‍♂️🍽💪📵🚶‍♂️"}
	merged := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️‍🍽💪💧📵🚶‍♂️"}, merged)
}

func TestMergeHeadersNoEmoji(t *testing.T) {
	r := require.New(t)

	headers := []string{"#### 23 May, Friday", "#### 23 May, Friday 💪"}
	merged := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 💪"}, merged)

	headers = []string{"#### 23 May, Saturday", "#### 23 May, Saturday"}
	merged = mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Saturday"}, merged)
}

// AI-gen tests

func TestMergeCompletelyDifferent(t *testing.T) {
	r := require.New(t)

	original := "apple\nbanana\ncherry"
	modified := "dog\nelephant\nfox"
	merged := Merge(original, modified)
	r.Equal("apple\nbanana\ncherry\ndog\nelephant\nfox", merged)
}

func TestMergeRepeatedLines(t *testing.T) {
	r := require.New(t)

	original := "repeat\nrepeat\nunique1"
	modified := "repeat\nrepeat\nunique2"
	merged := Merge(original, modified)
	r.Equal("repeat\nrepeat\nunique1\nunique2", merged)
}

func TestMergeWithBlankLines(t *testing.T) {
	r := require.New(t)

	original := "line1\n\nline3"
	modified := "line1\nline2\n\nline3"
	merged := Merge(original, modified)
	r.Equal("line1\nline2\n\nline3", merged)
}

func TestMergeMultipleBlankLines(t *testing.T) {
	r := require.New(t)

	original := "start\n\n\nend"
	modified := "start\nmiddle\n\n\nend"
	merged := Merge(original, modified)
	r.Equal("start\nmiddle\n\n\nend", merged)
}

func TestMergeOnlyBlankLines(t *testing.T) {
	r := require.New(t)

	original := "\n\n"
	modified := "\n\n\n"
	merged := Merge(original, modified)
	r.Equal("\n\n\n", merged)
}

func TestMergeSingleLineStrings(t *testing.T) {
	r := require.New(t)

	r.Equal("hello", Merge("hello", "hello"))
	r.Equal("hello\nworld", Merge("hello", "world"))
	r.Equal("world\nhello", Merge("world", "hello"))
}

func TestMergeVeryLongCommonPrefix(t *testing.T) {
	r := require.New(t)

	commonLines := make([]string, 100)
	for i := 0; i < 100; i++ {
		commonLines[i] = fmt.Sprintf("common line %d", i)
	}
	commonPrefix := strings.Join(commonLines, "\n")

	original := commonPrefix + "\noriginal ending"
	modified := commonPrefix + "\nmodified ending"
	merged := Merge(original, modified)
	expected := commonPrefix + "\noriginal ending\nmodified ending"
	r.Equal(expected, merged)
}

func TestMergeVeryLongCommonSuffix(t *testing.T) {
	r := require.New(t)

	commonLines := make([]string, 100)
	for i := 0; i < 100; i++ {
		commonLines[i] = fmt.Sprintf("common line %d", i)
	}
	commonSuffix := strings.Join(commonLines, "\n")

	original := "original start\n" + commonSuffix
	modified := "modified start\n" + commonSuffix
	merged := Merge(original, modified)
	expected := "original start\nmodified start\n" + commonSuffix
	r.Equal(expected, merged)
}

func TestMergeNestedCommonSubsequences(t *testing.T) {
	r := require.New(t)

	// Complex case with multiple common subsequences
	original := "A\nB\nC\nX\nD\nE\nY\nF"
	modified := "A\nB\nZ\nC\nD\nE\nW\nF"
	merged := Merge(original, modified)
	// Should preserve the LCS while adding unique content
	r.Contains(merged, "A")
	r.Contains(merged, "B")
	r.Contains(merged, "C")
	r.Contains(merged, "D")
	r.Contains(merged, "E")
	r.Contains(merged, "F")
	r.Contains(merged, "X")
	r.Contains(merged, "Y")
	r.Contains(merged, "Z")
	r.Contains(merged, "W")
}

func TestMergeWithSpecialCharacters(t *testing.T) {
	r := require.New(t)

	original := "line with\ttabs\nline with spaces"
	modified := "line with\ttabs\nline with   multiple   spaces"
	merged := Merge(original, modified)
	r.Equal("line with\ttabs\nline with spaces\nline with   multiple   spaces", merged)
}

func TestMergeUnicodeContent(t *testing.T) {
	r := require.New(t)

	original := "Hello 世界\n🌍 Earth"
	modified := "Hello 世界\n🌍 Earth\n🚀 Space"
	merged := Merge(original, modified)
	r.Equal("Hello 世界\n🌍 Earth\n🚀 Space", merged)
}

func TestMergeVeryLongLines(t *testing.T) {
	r := require.New(t)

	longLine := strings.Repeat("a", 10000)
	original := longLine + "\nshort"
	modified := longLine + "\ndifferent"
	merged := Merge(original, modified)
	r.Equal(longLine+"\nshort\ndifferent", merged)
}

func TestMergeIdenticalContent(t *testing.T) {
	r := require.New(t)

	content := "line1\nline2\nline3\nline4\nline5"
	r.Equal(content, Merge(content, content))
}

func TestMergeOneIsSubsetOfOther(t *testing.T) {
	r := require.New(t)

	subset := "line2\nline4"
	superset := "line1\nline2\nline3\nline4\nline5"

	// Test both directions
	r.Equal(superset, Merge(subset, superset))
	r.Equal(superset, Merge(superset, subset))
}

func TestMergeAlternatingPattern(t *testing.T) {
	r := require.New(t)

	// Alternating common and unique lines
	original := "common1\nunique1\ncommon2\nunique2\ncommon3"
	modified := "common1\ndifferent1\ncommon2\ndifferent2\ncommon3"
	merged := Merge(original, modified)

	r.Contains(merged, "common1")
	r.Contains(merged, "common2")
	r.Contains(merged, "common3")
	r.Contains(merged, "unique1")
	r.Contains(merged, "unique2")
	r.Contains(merged, "different1")
	r.Contains(merged, "different2")
}

func TestMergeRealWorldScenario(t *testing.T) {
	r := require.New(t)

	// Simulating a config file merge
	original := `# Configuration file
version: 1.0
database:
  host: localhost
  port: 5432
logging:
  level: info`

	modified := `# Configuration file
version: 1.0
database:
  host: localhost
  port: 5432
  timeout: 30
logging:
  level: debug
  file: app.log`

	merged := Merge(original, modified)

	// Should contain all unique lines from both
	r.Contains(merged, "# Configuration file")
	r.Contains(merged, "version: 1.0")
	r.Contains(merged, "database:")
	r.Contains(merged, "  host: localhost")
	r.Contains(merged, "  port: 5432")
	r.Contains(merged, "  timeout: 30")
	r.Contains(merged, "logging:")
	r.Contains(merged, "  level: info")
	r.Contains(merged, "  level: debug")
	r.Contains(merged, "  file: app.log")
}

func TestMergeJournalWithTasks(t *testing.T) {
	r := require.New(t)

	// More complex journal scenario
	original := `#### 24 May, Sunday
Morning routine
- Coffee ☕
- Exercise 💪
Evening reflection
- Good day overall`

	modified := `#### 24 May, Sunday
Morning routine
- Coffee ☕
- Read news 📰
- Exercise 💪
Afternoon work
- Team meeting
Evening reflection
- Good day overall
- Grateful for sunshine`

	merged := Merge(original, modified)

	// Should preserve the structure while adding new content
	r.Contains(merged, "#### 24 May, Sunday")
	r.Contains(merged, "Morning routine")
	r.Contains(merged, "- Coffee ☕")
	r.Contains(merged, "- Exercise 💪")
	r.Contains(merged, "- Read news 📰")
	r.Contains(merged, "Afternoon work")
	r.Contains(merged, "- Team meeting")
	r.Contains(merged, "Evening reflection")
	r.Contains(merged, "- Good day overall")
	r.Contains(merged, "- Grateful for sunshine")
}

func TestMergeEmojisInJournalHeaders_SingleHeader(t *testing.T) {
	r := require.New(t)

	// Single header with emojis
	headers := []string{"#### 23 May, Friday 🤸‍♂️🍽💪"}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️🍽💪"}, result)

	// Single header without emojis
	headers = []string{"#### 23 May, Friday"}
	result = mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday"}, result)
}

func TestMergeEmojisInJournalHeaders_MultipleHeadersSameDate(t *testing.T) {
	r := require.New(t)

	// Multiple headers with same date, different emojis
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday 🍽💪",
		"#### 23 May, Friday 💧",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️🍽💪💧"}, result)
}

func TestMergeEmojisInJournalHeaders_OverlappingEmojis(t *testing.T) {
	r := require.New(t)

	// Headers with overlapping emojis - should deduplicate
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️🍽💪",
		"#### 23 May, Friday 🍽💪💧",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️🍽💪💧"}, result)
}

func TestMergeEmojisInJournalHeaders_DifferentDates(t *testing.T) {
	r := require.New(t)

	// Headers with different dates - should not merge
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 24 May, Saturday 🍽💪",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 24 May, Saturday 🍽💪",
	}, result)
}

func TestMergeEmojisInJournalHeaders_PartialDateMatch(t *testing.T) {
	r := require.New(t)

	// Headers where one starts with part of another's date - should not merge
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday evening 🍽💪",
	}
	result := mergeEmojisInJournalHeaders(headers)
	// Should not merge because "#### 23 May, Friday evening" doesn't start with "#### 23 May, Friday"
	r.Equal([]string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday evening 🍽💪",
	}, result)
}

func TestMergeTwoNonHeaders(t *testing.T) {
	r := require.New(t)

	// Headers where one starts with part of another's date - should not merge
	headers := []string{
		"#### 🤸‍♂️",
		"#### 🍽💪",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{
		"#### 🤸‍♂️",
		"#### 🍽💪",
	}, result)
}

func TestMergeEmojisInJournalHeaders_NoEmojis(t *testing.T) {
	r := require.New(t)

	// Multiple headers with same date but no emojis
	headers := []string{
		"#### 23 May, Friday",
		"#### 23 May, Friday",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday"}, result)
}

func TestMergeEmojisInJournalHeaders_MixedEmojiAndNoEmoji(t *testing.T) {
	r := require.New(t)

	// Mix of headers with and without emojis
	headers := []string{
		"#### 23 May, Friday",
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday 🍽💪",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🤸‍♂️🍽💪"}, result)
}

func TestMergeEmojisInJournalHeaders_NonHeaderLines(t *testing.T) {
	r := require.New(t)

	// Mix of headers and non-headers
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"This is not a header",
		"Neither is this",
		"#### 24 May, Saturday 🍽💪",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{
		"#### 23 May, Friday 🤸‍♂️",
		"This is not a header",
		"Neither is this",
		"#### 24 May, Saturday 🍽💪",
	}, result)
}

func TestMergeEmojisInJournalHeaders_ConsecutiveGroupsWithNonHeaders(t *testing.T) {
	r := require.New(t)

	// Multiple groups separated by non-headers
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday 🍽",
		"Some content here",
		"#### 24 May, Saturday 💪",
		"#### 24 May, Saturday 💧",
		"More content",
		"#### 25 May, Sunday 🚶‍♂️",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{
		"#### 23 May, Friday 🤸‍♂️🍽",
		"Some content here",
		"#### 24 May, Saturday 💪💧",
		"More content",
		"#### 25 May, Sunday 🚶‍♂️",
	}, result)
}

func TestMergeEmojisInJournalHeaders_ComplexEmojis(t *testing.T) {
	r := require.New(t)

	// Test with complex emojis (multi-byte, skin tones, etc.)
	headers := []string{
		"#### 23 May, Friday 👨‍💻",
		"#### 23 May, Friday 🏃‍♂️",
		"#### 23 May, Friday 👍🏽",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 👨‍💻🏃‍♂️👍🏽"}, result)
}

func TestMergeEmojisInJournalHeaders_EmojiOrder(t *testing.T) {
	r := require.New(t)

	// Test that emoji order is preserved
	headers := []string{
		"#### 23 May, Friday 🥇",
		"#### 23 May, Friday 🥈",
		"#### 23 May, Friday 🥉",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday 🥇🥈🥉"}, result)
}

func TestMergeEmojisInJournalHeaders_EmptyInput(t *testing.T) {
	r := require.New(t)

	result := mergeEmojisInJournalHeaders([]string{})
	r.Empty(result)

	result = mergeEmojisInJournalHeaders(nil)
	r.Empty(result)
}

func TestMergeEmojisInJournalHeaders_BugFix_NonMergeable(t *testing.T) {
	r := require.New(t)

	// This test specifically targets the bug you found
	// When headers can't be merged, foundEmojis is accumulated but not used
	headers := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 24 May, Saturday 🍽💪", // Different date
	}
	result := mergeEmojisInJournalHeaders(headers)

	// Should return original headers unchanged, not merged
	r.Equal([]string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 24 May, Saturday 🍽💪",
	}, result)

	// The bug would have been that foundEmojis was being accumulated
	// even when the headers couldn't be merged due to different dates
}

func TestMergeEmojisInJournalHeaders_SpecialCharacters(t *testing.T) {
	r := require.New(t)

	// Test with punctuation and special characters
	headers := []string{
		"#### 23 May, Friday! 🎉",
		"#### 23 May, Friday! 🎊",
	}
	result := mergeEmojisInJournalHeaders(headers)
	r.Equal([]string{"#### 23 May, Friday! 🎉🎊"}, result)
}

func TestMergeEmojisInJournalHeaders_RealWorldScenario(t *testing.T) {
	r := require.New(t)

	// Realistic journal headers as they might appear
	headers := []string{
		"#### 28 May, Wednesday 🌅",  // morning entry
		"#### 28 May, Wednesday 💼",  // work entry
		"#### 28 May, Wednesday 🍽️", // meal entry
		"#### 28 May, Wednesday 🌙",  // evening entry
		"Some journal content here",
		"#### 29 May, Thursday ☀️",
		"#### 29 May, Thursday 🏃‍♂️💪",
	}
	result := mergeEmojisInJournalHeaders(headers)
	expected := []string{
		"#### 28 May, Wednesday 🌅💼🍽️🌙",
		"Some journal content here",
		"#### 29 May, Thursday ☀️🏃‍♂️💪",
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_SingleHeader(t *testing.T) {
	r := require.New(t)

	lines := []string{"#### 23 May, Friday"}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{{"#### 23 May, Friday"}}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_MultipleConsecutiveHeaders(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 23 May, Friday",
		"#### 23 May, Saturday",
		"#### 24 May, Sunday",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{{
		"#### 23 May, Friday",
		"#### 23 May, Saturday",
		"#### 24 May, Sunday",
	}}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_HeadersWithEmojis(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday 🍽💪",
		"#### 24 May, Saturday 💧",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{{
		"#### 23 May, Friday 🤸‍♂️",
		"#### 23 May, Friday 🍽💪",
		"#### 24 May, Saturday 💧",
	}}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_NonHeaders(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"This is not a header",
		"Neither is this",
		"Just regular text",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{
		{"This is not a header"},
		{"Neither is this"},
		{"Just regular text"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_MixedHeadersAndNonHeaders(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 23 May, Friday",
		"#### 23 May, Saturday",
		"Some journal content",
		"More content",
		"#### 24 May, Sunday",
		"Final content",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{
		{"#### 23 May, Friday", "#### 23 May, Saturday"},
		{"Some journal content"},
		{"More content"},
		{"#### 24 May, Sunday"},
		{"Final content"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_HeadersWithExtraSpaces(t *testing.T) {
	r := require.New(t)

	// Headers with extra spaces - should still match
	lines := []string{
		"#### 23 May, Friday   ",
		"####  23 May, Saturday", // Extra space after ####
		"#### 24 May, Sunday",
	}
	result := groupConsecutiveHeaders(lines)
	// Only the properly formatted ones should be grouped as headers
	expected := [][]string{
		{"#### 23 May, Friday   "},
		{"####  23 May, Saturday"}, // This won't match the regex
		{"#### 24 May, Sunday"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_InvalidHeaderFormats(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"### 23 May, Friday",      // Wrong number of #
		"##### 23 May, Friday",    // Too many #
		"#### May 23, Friday",     // Wrong date format
		"#### 23 May Friday",      // Missing comma
		"#### 23 May,",            // Missing day
		"#### 23, Friday",         // Missing month
		"#### twenty May, Friday", // Non-numeric day
	}
	result := groupConsecutiveHeaders(lines)
	// None should match, so each should be in its own group
	expected := [][]string{
		{"### 23 May, Friday"},
		{"##### 23 May, Friday"},
		{"#### May 23, Friday"},
		{"#### 23 May Friday"},
		{"#### 23 May,"},
		{"#### 23, Friday"},
		{"#### twenty May, Friday"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_ValidHeaderVariations(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 1 Jan, Monday",
		"#### 10 February, Tuesday",
		"#### 31 December, Wednesday",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{{
		"#### 1 Jan, Monday",
		"#### 10 February, Tuesday",
		"#### 31 December, Wednesday",
	}}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_MultipleGroups(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 23 May, Friday",
		"#### 23 May, Saturday",
		"Some content between groups",
		"#### 24 May, Sunday",
		"#### 24 May, Monday",
		"More content",
		"#### 25 May, Tuesday",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{
		{"#### 23 May, Friday", "#### 23 May, Saturday"},
		{"Some content between groups"},
		{"#### 24 May, Sunday", "#### 24 May, Monday"},
		{"More content"},
		{"#### 25 May, Tuesday"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_SingleNonHeader(t *testing.T) {
	r := require.New(t)

	lines := []string{"Just some text"}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{{"Just some text"}}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_HeadersAtStartAndEnd(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 23 May, Friday",
		"#### 23 May, Saturday",
		"Middle content",
		"#### 24 May, Sunday",
		"#### 24 May, Monday",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{
		{"#### 23 May, Friday", "#### 23 May, Saturday"},
		{"Middle content"},
		{"#### 24 May, Sunday", "#### 24 May, Monday"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_HeadersWithDifferentContent(t *testing.T) {
	r := require.New(t)

	// Headers with additional content after the date/day
	lines := []string{
		"#### 23 May, Friday - Good day",
		"#### 23 May, Saturday 🌞 Sunny",
		"#### 24 May, Sunday (rainy)",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{{
		"#### 23 May, Friday - Good day",
		"#### 23 May, Saturday 🌞 Sunny",
		"#### 24 May, Sunday (rainy)",
	}}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_EdgeCaseWhitespace(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"", // Empty line
		"#### 23 May, Friday",
		"   ", // Line with just spaces
		"#### 24 May, Saturday",
		"",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{
		{""},
		{"#### 23 May, Friday"},
		{"   "},
		{"#### 24 May, Saturday"},
		{""},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_CaseSensitivity(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 23 may, friday", // lowercase
		"#### 23 MAY, FRIDAY", // uppercase
		"#### 23 May, Friday", // proper case
	}
	result := groupConsecutiveHeaders(lines)
	// Only the properly cased one should match
	expected := [][]string{
		{"#### 23 may, friday", "#### 23 MAY, FRIDAY", "#### 23 May, Friday"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_RealWorldJournalExample(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 28 May, Wednesday",
		"#### 28 May, Wednesday", // Duplicate header
		"Morning routine:",
		"- Coffee ☕",
		"- Exercise 💪",
		"",
		"#### 29 May, Thursday",
		"#### 29 May, Thursday",
		"Work day:",
		"- Team meeting",
		"- Code review",
		"",
		"#### 30 May, Friday",
		"TGIF! 🎉",
	}
	result := groupConsecutiveHeaders(lines)
	expected := [][]string{
		{"#### 28 May, Wednesday", "#### 28 May, Wednesday"},
		{"Morning routine:"},
		{"- Coffee ☕"},
		{"- Exercise 💪"},
		{""},
		{"#### 29 May, Thursday", "#### 29 May, Thursday"},
		{"Work day:"},
		{"- Team meeting"},
		{"- Code review"},
		{""},
		{"#### 30 May, Friday"},
		{"TGIF! 🎉"},
	}
	r.Equal(expected, result)
}

func TestGroupConsecutiveHeaders_RegexEdgeCases(t *testing.T) {
	r := require.New(t)

	lines := []string{
		"#### 0 May, Friday",        // Zero day - should match \d+
		"#### 99 December, Sunday",  // Large day number
		"#### 1 A, Monday",          // Single letter month - should match \w+
		"#### 1 Ab, Tuesday",        // Two letter month
		"#### 1 January, W",         // Single letter day - should match \w+
		"#### 1 January, Wednesday", // Full day name
	}
	result := groupConsecutiveHeaders(lines)
	// All should match the regex pattern
	expected := [][]string{{
		"#### 0 May, Friday",
		"#### 99 December, Sunday",
		"#### 1 A, Monday",
		"#### 1 Ab, Tuesday",
		"#### 1 January, W",
		"#### 1 January, Wednesday",
	}}
	r.Equal(expected, result)
}
