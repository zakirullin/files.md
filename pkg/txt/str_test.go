package txt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPositiveI64ToStr(t *testing.T) {
	r := require.New(t)

	s := I64(1)

	r.Equal("1", s)
}

func TestNegativeI64ToStr(t *testing.T) {
	r := require.New(t)

	s := I64(-1)

	r.Equal("-1", s)
}

func TestZeroI64ToStr(t *testing.T) {
	r := require.New(t)

	s := I64(0)

	r.Equal("0", s)
}

func TestUcfirst(t *testing.T) {
	r := require.New(t)

	res := Ucfirst("abc")

	r.Equal("Abc", res)
}

func TestUcfirstRu(t *testing.T) {
	r := require.New(t)

	res := Ucfirst("абв")

	r.Equal("Абв", res)
}

func TestLcfirst(t *testing.T) {
	r := require.New(t)

	res := Lcfirst("ABC")

	r.Equal("aBC", res)
}

func TestLcfirstRu(t *testing.T) {
	r := require.New(t)

	res := Lcfirst("АБВ")

	r.Equal("аБВ", res)
}

func TestInsertTextAfterHeaderNoHeader(t *testing.T) {
	r := require.New(t)

	content := InsertTextAfterHeader("### header 1\nitem1\nitem2", "### header 5", "new item")

	r.Equal("### header 5\nnew item\n### header 1\nitem1\nitem2", content)
}

func TestInsertTextAfterHeader(t *testing.T) {
	r := require.New(t)

	content := InsertTextAfterHeader("### header 1\nitem1\nitem2\n### header 2", "### header 1", "new item")

	r.Equal("### header 1\nnew item\nitem1\nitem2\n### header 2", content)
}

func TestInsertTextAfterHeaderInTheMiddle(t *testing.T) {
	r := require.New(t)

	content := InsertTextAfterHeader("### header 0\n### header 1\nitem1\nitem2\n### header 2", "### header 1", "new item")

	r.Equal("### header 0\n### header 1\nnew item\nitem1\nitem2\n### header 2", content)
}

func TestInsertTextAfterHeaderInTheMiddleOnlyHeader(t *testing.T) {
	r := require.New(t)

	content := InsertTextAfterHeader("### header 0\n### header 1\n### header 2", "### header 1", "new item")

	r.Equal("### header 0\n### header 1\nnew item\n### header 2", content)
}

func TestSplitTextIntoChunks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected []string
	}{
		{
			name:     "basic split with spaces",
			input:    "This is a test to check the splitting of text",
			maxLen:   10,
			expected: []string{"This is a", "test to", "check the", "splitting", "of text"},
		},
		{
			name:     "split with newlines",
			input:    "Line one\nLine two\nLine three",
			maxLen:   15,
			expected: []string{"Line one", "Line two", "Line three"},
		},
		{
			name:     "long string without spaces",
			input:    "supercalifragilisticexpialidocious",
			maxLen:   10,
			expected: []string{"supercalif", "ragilistic", "expialidoc", "ious"},
		},
		{
			name:     "exact match",
			input:    "ExactMatch",
			maxLen:   10,
			expected: []string{"ExactMatch"},
		},
		{
			name:     "trailing and leading spaces",
			input:    "   Leading and trailing spaces   ",
			maxLen:   15,
			expected: []string{"Leading and", "trailing spaces"},
		},
		{
			name:     "no split needed",
			input:    "Short text",
			maxLen:   50,
			expected: []string{"Short text"},
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: []string{""},
		},
		{
			name:     "string with only spaces",
			input:    "                                            ",
			maxLen:   10,
			expected: []string{""},
		},
		{
			name:     "string with only spaces",
			input:    "aaa",
			maxLen:   2,
			expected: []string{"aa", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := SplitTextIntoChunks(tt.input, tt.maxLen)
			require.Equal(t, tt.expected, res)
		})
	}
}
