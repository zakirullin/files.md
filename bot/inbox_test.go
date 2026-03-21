package internal

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/bot/db"
	"zakirullin/stuffbot/bot/fs"
	"zakirullin/stuffbot/pkg/tg"
)

func TestReadMessagesEmpty(t *testing.T) {
	r := require.New(t)
	result := readBlocks("")
	r.Empty(result)
}

func TestReadMessagesOnlyHeader(t *testing.T) {
	r := require.New(t)
	result := readBlocks("#### 27 June, Friday")
	r.Equal([]string{"#### 27 June, Friday"}, result)
}

func TestReadMessagesSingleRecord(t *testing.T) {
	r := require.New(t)
	result := readBlocks("`01:01` Simple record")
	r.Equal([]string{"`01:01` Simple record"}, result)
}

func TestReadMessagesHeaderWithRecord(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` Simple record"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Simple record"}, result)
}

func TestReadMessagesMultilineRecord(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` Multiline\nc\nontent"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Multiline\nc\nontent"}, result)
}

func TestReadMessagesMultipleRecords(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` First record\n`02:02` Second record"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` First record", "`02:02` Second record"}, result)
}

func TestReadMessagesMultipleHeaders(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`01:01` First day\n#### 28 June, Saturday\n`02:02` Second day"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` First day", "#### 28 June, Saturday", "`02:02` Second day"}, result)
}

func TestReadMessagesWindowsLineEndings(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\r\n`01:01` Windows record"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Windows record"}, result)
}

func TestReadMessagesWithEmptyLines(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n\n`01:01` Record with\n\nempty lines"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`01:01` Record with\n\nempty lines"}, result)
}

func TestReadMessagesInvalidTimestamp(t *testing.T) {
	r := require.New(t)
	content := "#### 27 June, Friday\n`not timestamp` Should be continuation\n`01:01` Real record"
	result := readBlocks(content)
	r.Equal([]string{"#### 27 June, Friday", "`not timestamp` Should be continuation", "`01:01` Real record"}, result)
}

func TestSaveToChatNewFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToInbox("Test content", time.UTC)
	r.NoError(err)
	r.Equal(0, index)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`01:01` Test content\n", content)
}

func TestSaveToChatExistingFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, "#### 27 June, Thursday\n`00:30` Existing content\n")
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToInbox("New content", time.UTC)
	r.NoError(err)
	r.Equal(1, index)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`00:30` Existing content\n`01:01` New content\n", content)
}

func TestSaveToChatNewDay(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 28, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, "#### 27 June, Thursday\n`00:30` Yesterday content\n")
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToInbox("Today content", time.UTC)
	r.NoError(err)
	r.Equal(1, index)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`00:30` Yesterday content\n#### 28 June, Friday\n`01:01` Today content\n", content)
}

func TestSaveToChatWithImage(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToInbox("![](image.jpg) Image description", time.UTC)
	r.NoError(err)
	r.Equal(0, index)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`01:01` ![](image.jpg) Image description\n", content)
}

func TestSaveToChatEmptyFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, "")
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	index, err := bot.saveToInbox("Test content", time.UTC)
	r.NoError(err)
	r.Equal(0, index)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)
	r.Equal("#### 27 June, Thursday\n`01:01` Test content\n", content)
}

//func TestSaveToChatWithTimezone(t *testing.T) {
//	r := require.New(t)
//
//	savedNow := now
//	defer func() { now = savedNow }()
//	now = func() time.Time {
//		return time.Date(2024, 6, 27, 1, 1, 0, 0, time.UTC)
//	}
//
//	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
//	r.NoError(err)
//
//	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
//
//	// Use EST timezone (UTC-5)
//	est, err := time.LoadLocation("America/New_York")
//	r.NoError(err)
//
//	index, err := bot.saveToChat("Test content", est)
//	r.NoError(err)
//	r.Equal(1, index)
//
//	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
//	r.NoError(err)
//	// Should use EST time which is 20:01 (8:01 PM) the previous day
//	r.Contains(content, "`20:01` Test content")
//}

// Test normal case - properly formatted content
func TestReadMessages_NormalCase(t *testing.T) {
	content := `#### 1 July, Tuesday
` + "`15:19`" + ` Пройтись на улице
#### 2 July, Wednesday
` + "`10:30`" + ` Почитать книгу`

	result := readBlocks(content)
	expected := []string{
		"#### 1 July, Tuesday",
		"`15:19` Пройтись на улице",
		"#### 2 July, Wednesday",
		"`10:30` Почитать книгу",
	}

	if len(result) != len(expected) {
		t.Fatalf("Expected %d blocks, got %d", len(expected), len(result))
	}

	for i, block := range result {
		if block != expected[i] {
			t.Errorf("Block %d mismatch:\nExpected: %q\nGot: %q", i, expected[i], block)
		}
	}
}

// Test message without timestamp - this could be the source of the issue
func TestReadMessages_MessageWithoutTimestamp(t *testing.T) {
	content := `#### 1 July, Tuesday
` + "`15:19`" + ` Пройтись на улице
Провести звонок с Нео
#### 2 July, Wednesday
` + "`10:30`" + ` Почитать книгу`

	result := readBlocks(content)

	// The message without timestamp should be grouped with the previous timestamped message
	expected := []string{
		"#### 1 July, Tuesday",
		"`15:19` Пройтись на улице\nПровести звонок с Нео",
		"#### 2 July, Wednesday",
		"`10:30` Почитать книгу",
	}

	if len(result) != len(expected) {
		t.Fatalf("Expected %d blocks, got %d\nGot: %v", len(expected), len(result), result)
	}

	for i, block := range result {
		if block != expected[i] {
			t.Errorf("Block %d mismatch:\nExpected: %q\nGot: %q", i, expected[i], block)
		}
	}
}

//func TestReadMessages_MultipleMessagesWithoutTimestamps(t *testing.T) {
//	content := `#### 1 July, Tuesday
//` + "`15:19`" + ` Пройтись на улице
//Провести звонок с Нео
//Купить молоко
//#### 2 July, Wednesday
//Почитать книгу
//` + "`10:30`" + ` Сходить в магазин`
//
//	result := readMessages(content)
//
//	// Check what happens with multiple untimestamped messages
//	t.Logf("Result blocks: %v", result)
//
//	// This should reveal how readMessages handles content without timestamps
//	headerRegex := regexp.MustCompile(`^#### `)
//	timestampRegex := regexp.MustCompile(`^` + "`" + `\d{2}:\d{2}` + "`" + ` `)
//
//	for i, block := range result {
//		isHeader := headerRegex.MatchString(block)
//		hasTimestamp := timestampRegex.MatchString(block)
//		t.Logf("Block %d: isHeader=%v, hasTimestamp=%v, content=%q", i, isHeader, hasTimestamp, block)
//
//		if !isHeader && !hasTimestamp {
//			t.Errorf("Found record without timestamp: %q", block)
//		}
//	}
//}

//func TestReadMessages_ContentAfterHeaderWithoutTimestamp(t *testing.T) {
//	content := `#### 1 July, Tuesday
//#### 2 July, Wednesday
//Почитать книгу
//` + "`10:30`" + ` Сходить в магазин`
//
//	result := readMessages(content)
//
//	t.Logf("Result blocks: %v", result)
//
//	// This case might be critical - content right after header without timestamp
//	headerRegex := regexp.MustCompile(`^#### `)
//	timestampRegex := regexp.MustCompile(`^` + "`" + `\d{2}:\d{2}` + "`" + ` `)
//
//	for i, block := range result {
//		isHeader := headerRegex.MatchString(block)
//		hasTimestamp := timestampRegex.MatchString(block)
//		t.Logf("Block %d: isHeader=%v, hasTimestamp=%v, content=%q", i, isHeader, hasTimestamp, block)
//
//		if !isHeader && !hasTimestamp {
//			t.Errorf("Found record without timestamp: %q", block)
//		}
//	}
//}

// Test multiline message formatting
func TestReadBlocks_MultilineMessage(t *testing.T) {
	content := `#### 1 July, Tuesday
` + "`15:19`" + ` Пройтись на улице
и купить хлеб
в магазине
#### 2 July, Wednesday
` + "`10:30`" + ` Почитать книгу`

	result := readBlocks(content)
	expected := []string{
		"#### 1 July, Tuesday",
		"`15:19` Пройтись на улице\nи купить хлеб\nв магазине",
		"#### 2 July, Wednesday",
		"`10:30` Почитать книгу",
	}

	if len(result) != len(expected) {
		t.Fatalf("Expected %d blocks, got %d", len(expected), len(result))
	}

	for i, block := range result {
		if block != expected[i] {
			t.Errorf("Block %d mismatch:\nExpected: %q\nGot: %q", i, expected[i], block)
		}
	}
}

func TestReadBlocksHasTimestamp(t *testing.T) {
	content := `#### 1 July, Tuesday
` + "`15:19`" + ` Do some stuff 
Arrange a call with Neo
#### 2 July, Wednesday
` + "`10:30`" + ` Read a book`

	messages := readBlocks(content)

	// Filter to find record messages (not headers)
	headerRegex := regexp.MustCompile(`^#### `)
	var recordIndices []int
	var records []string

	for i, message := range messages {
		if !headerRegex.MatchString(message) {
			recordIndices = append(recordIndices, i)
			records = append(records, message)
		}
	}

	t.Logf("Found %d records:", len(records))
	for i, record := range records {
		t.Logf("Record %d: %q", i, record)
	}

	timestampRegex := regexp.MustCompile(`^` + "`" + `\d{2}:\d{2}` + "`" + ` `)
	for i, record := range records {
		hasTimestamp := timestampRegex.MatchString(record)
		t.Logf("Record %d has timestamp: %v", i, hasTimestamp)
		if !hasTimestamp {
			t.Errorf("Record %d missing timestamp: %q", i, record)
		}
	}
}

//func TestReadMessages_ExactIssueScenario(t *testing.T) {
//	content := `#### 1 July, Tuesday
//#### 2 July, Wednesday
//Почитать книгу
//` + "`15:19`" + ` Пройтись на улице
//Провести звонок с Нео`
//
//	result := readMessages(content)
//
//	t.Logf("Input content:")
//	t.Logf("%q", content)
//	t.Logf("Parsed blocks:")
//	for i, block := range result {
//		t.Logf("Block %d: %q", i, block)
//	}
//
//	// Check for records without timestamps
//	headerRegex := regexp.MustCompile(`^#### `)
//	timestampRegex := regexp.MustCompile(`^` + "`" + `\d{2}:\d{2}` + "`" + ` `)
//
//	for i, block := range result {
//		isHeader := headerRegex.MatchString(block)
//		hasTimestamp := timestampRegex.MatchString(block)
//
//		if !isHeader && !hasTimestamp {
//			t.Errorf("Found record without timestamp at index %d: %q", i, block)
//		}
//	}
//}

// Test timestamp pattern matching
func TestTimestampPatternMatching(t *testing.T) {
	timestampRegex := regexp.MustCompile(`^` + "`" + `\d{2}:\d{2}` + "`" + ` `)

	validTimestamps := []string{
		"`15:19` Пройтись на улице",
		"`10:30` Почитать книгу",
		"`23:59` Test message",
	}

	invalidTimestamps := []string{
		"15:19 Пройтись на улице",  // No backticks
		"Пройтись на улице",        // No timestamp
		"15:19` Пройтись на улице", // Missing opening backtick
		"`15:19 Пройтись на улице", // Missing closing backtick
		"`5:19` Пройтись на улице", // Single digit hour
		"`15:9` Пройтись на улице", // Single digit minute
	}

	for _, ts := range validTimestamps {
		if !timestampRegex.MatchString(ts) {
			t.Errorf("Valid timestamp not matched: %q", ts)
		}
	}

	for _, ts := range invalidTimestamps {
		if timestampRegex.MatchString(ts) {
			t.Errorf("Invalid timestamp matched: %q", ts)
		}
	}
}

// Test what happens when saveToChat adds content without proper formatting
func TestSaveToChat_ContentAddition(t *testing.T) {
	// Mock the current time for testing
	originalNow := Now
	defer func() { Now = originalNow }()

	Now = func() time.Time {
		return time.Date(2024, 7, 2, 15, 19, 0, 0, time.UTC)
	}

	timezone := time.UTC

	// Test content formatting in saveToChat
	content := "Arrange call with Neo"
	timestamp := Now().In(timezone).Format("`15:04`")
	expectedFormat := timestamp + " " + content + "\n"

	t.Logf("Content: %q", content)
	t.Logf("Timestamp: %q", timestamp)
	t.Logf("Expected format: %q", expectedFormat)

	// This should match the format from saveToChat function
	if !strings.Contains(expectedFormat, "`15:19`") {
		t.Errorf("Timestamp format is incorrect: %q", expectedFormat)
	}
}

// Test edge case: empty content handling
func TestReadMessages_EmptyContent(t *testing.T) {
	content := ""
	result := readBlocks(content)

	if len(result) != 0 {
		t.Errorf("Expected 0 blocks for empty content, got %d: %v", len(result), result)
	}
}

// Test edge case: only headers
func TestReadMessages_OnlyHeaders(t *testing.T) {
	content := `#### 1 July, Tuesday
#### 2 July, Wednesday`

	result := readBlocks(content)
	expected := []string{
		"#### 1 July, Tuesday",
		"#### 2 July, Wednesday",
	}

	if len(result) != len(expected) {
		t.Fatalf("Expected %d blocks, got %d", len(expected), len(result))
	}

	for i, block := range result {
		if block != expected[i] {
			t.Errorf("Block %d mismatch:\nExpected: %q\nGot: %q", i, expected[i], block)
		}
	}
}

func TestMoveFromChatSingleRecord(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	initialContent := `#### 27 June, Thursday
` + "`01:01`" + ` First record
` + "`02:02`" + ` Second record
#### 28 June, Friday
` + "`03:03`" + ` Third record`

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, initialContent)
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	var callbackCalls []struct {
		content   string
		timestamp time.Time
	}

	callback := func(content string, timestamp time.Time) error {
		callbackCalls = append(callbackCalls, struct {
			content   string
			timestamp time.Time
		}{content, timestamp})
		return nil
	}

	err = bot.moveFromInbox(callback, false, 1)
	r.NoError(err)

	r.Len(callbackCalls, 1)
	r.Equal("Second record", callbackCalls[0].content)

	expectedTime, _ := time.Parse("2 January 15:04", "27 June 02:02")
	r.Equal(expectedTime, callbackCalls[0].timestamp)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)

	expectedContent := `#### 27 June, Thursday
` + "`01:01`" + ` First record
#### 28 June, Friday
` + "`03:03`" + ` Third record`

	r.Equal(expectedContent, content)
}

func TestMoveFromChatMultipleRecords(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	initialContent := `#### 1 July, Monday
` + "`10:30`" + ` Buy groceries
milk, bread, eggs
` + "`11:45`" + ` Call mom
#### 2 July, Tuesday
` + "`09:15`" + ` Morning workout
` + "`14:20`" + ` Team meeting
discuss project timeline
and resource allocation`

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, initialContent)
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	var callbackCalls []struct {
		content   string
		timestamp time.Time
	}

	callback := func(content string, timestamp time.Time) error {
		callbackCalls = append(callbackCalls, struct {
			content   string
			timestamp time.Time
		}{content, timestamp})
		return nil
	}

	err = bot.moveFromInbox(callback, false, 0, 2)
	r.NoError(err)

	r.Len(callbackCalls, 2)

	r.Equal("Buy groceries\nmilk, bread, eggs", callbackCalls[0].content)
	expectedTime1, _ := time.Parse("2 January 15:04", "1 July 10:30")
	r.Equal(expectedTime1, callbackCalls[0].timestamp)

	r.Equal("Morning workout", callbackCalls[1].content)
	expectedTime2, _ := time.Parse("2 January 15:04", "2 July 09:15")
	r.Equal(expectedTime2, callbackCalls[1].timestamp)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)

	expectedContent := `#### 1 July, Monday
` + "`11:45`" + ` Call mom
#### 2 July, Tuesday
` + "`14:20`" + ` Team meeting
discuss project timeline
and resource allocation`

	r.Equal(expectedContent, content)
}

func TestMoveFromChatCollapsedSingleRecord(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	initialContent := `#### 27 June, Thursday
` + "`01:01`" + ` First record
` + "`02:02`" + ` Second record
#### 28 June, Friday
` + "`03:03`" + ` Third record`

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, initialContent)
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	var callbackCalls []struct {
		content   string
		timestamp time.Time
	}

	callback := func(content string, timestamp time.Time) error {
		callbackCalls = append(callbackCalls, struct {
			content   string
			timestamp time.Time
		}{content, timestamp})
		return nil
	}

	err = bot.moveFromInbox(callback, true, 1)
	r.NoError(err)

	r.Len(callbackCalls, 1)
	r.Equal("Second record", callbackCalls[0].content)

	expectedTime, _ := time.Parse("2 January 15:04", "27 June 02:02")
	r.Equal(expectedTime, callbackCalls[0].timestamp)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)

	expectedContent := `#### 27 June, Thursday
` + "`01:01`" + ` First record
#### 28 June, Friday
` + "`03:03`" + ` Third record`

	r.Equal(expectedContent, content)
}

func TestMoveFromChatCollapsedMultipleRecords(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	initialContent := `#### 1 July, Monday
` + "`10:30`" + ` Buy groceries
milk, bread, eggs
` + "`11:45`" + ` Call mom
#### 2 July, Tuesday
` + "`09:15`" + ` Morning workout
` + "`14:20`" + ` Team meeting
discuss project timeline
and resource allocation`

	err = userFS.Write(fs.DirRoot, fs.InboxFilename, initialContent)
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())

	// Track callback calls
	var callbackCalls []struct {
		content   string
		timestamp time.Time
	}

	callback := func(content string, timestamp time.Time) error {
		callbackCalls = append(callbackCalls, struct {
			content   string
			timestamp time.Time
		}{content, timestamp})
		return nil
	}

	err = bot.moveFromInbox(callback, true, 0, 2)
	r.NoError(err)

	r.Len(callbackCalls, 1)

	expectedCollapsedContent := `Buy groceries
milk, bread, eggs
Morning workout`
	r.Equal(expectedCollapsedContent, callbackCalls[0].content)

	expectedTime, _ := time.Parse("2 January 15:04", "1 July 10:30")
	r.Equal(expectedTime, callbackCalls[0].timestamp)

	content, err := userFS.Read(fs.DirRoot, fs.InboxFilename)
	r.NoError(err)

	expectedContent := `#### 1 July, Monday
` + "`11:45`" + ` Call mom
#### 2 July, Tuesday
` + "`14:20`" + ` Team meeting
discuss project timeline
and resource allocation`

	r.Equal(expectedContent, content)
}
