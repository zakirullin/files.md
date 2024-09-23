package plugins

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"zakirullin/stuffbot/pkg/tg"

	"golang.org/x/exp/slog"
)

type Chat interface {
	Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error)
}

const (
	timeFormat = "02.01.2006 15:04:05"
	dateFormat = "02.01.2006"
)

var (
	locationNames = []string{"UTC", "MSK", "CY", "ME"}
	locations     = map[string]*time.Location{
		"UTC": loadLocation("UTC"),
		"MSK": loadLocation("Europe/Moscow"),
		"CY":  loadLocation("Asia/Nicosia"),
		"ME":  loadLocation("Europe/Podgorica"),
	}
	locationIcons = map[string]string{
		"UTC": "🕰",
		"MSK": "🔺",
		"CY":  "🏝",
		"ME":  "🏝",
	}
)

type WorldClockPlugin struct {
	userID int64
	tg     Chat
}

func NewWorldClockPlugin(userID int64, tg Chat) *WorldClockPlugin {
	return &WorldClockPlugin{userID, tg}
}

// Handle checks if the message is a date, time or timestamp and sends the current time in different timezones
func (p *WorldClockPlugin) Handle(msgText string) (bool, error) {
	var message string
	var err error

	// Try to parse date
	t, err := p.parseDate(msgText)
	if err == nil {
		message = p.buildMessage(t, p.fmtTimestamp)
		_, err = p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		if err != nil {
			return true, fmt.Errorf("world clock: %w", err)
		}

		return true, nil
	}

	// Try to parse time
	t, err = p.parseTime(msgText)
	if err == nil {
		message = p.buildMessage(t, p.fmtTimestamp)
		_, err = p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		if err != nil {
			return true, fmt.Errorf("world clock: %w", err)
		}

		return true, nil
	}

	// Try to parse timestamp
	t, err = p.parseTimestamp(msgText)
	if err == nil {
		message = p.buildMessage(t, p.fmtTime)
		_, err = p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		if err != nil {
			return true, fmt.Errorf("world clock: %w", err)
		}
		return true, nil
	}

	return false, nil
}

func (p *WorldClockPlugin) parseTimestamp(message string) (time.Time, error) {
	timestamp, err := strconv.ParseInt(message, 10, 64)
	if err == nil && timestamp > 999999 {
		return time.Unix(timestamp, 0).UTC(), nil
	}
	return time.Time{}, errors.New("invalid timestamp")
}

func (p *WorldClockPlugin) parseTime(message string) (time.Time, error) {
	parsedTime, err := time.Parse(timeFormat, message)
	if err == nil {
		return parsedTime.UTC(), nil
	}
	return time.Time{}, errors.New("invalid time")
}

func (p *WorldClockPlugin) parseDate(message string) (time.Time, error) {
	parsedDate, err := time.Parse(dateFormat, message)
	if err == nil {
		return parsedDate.UTC(), nil
	}
	return time.Time{}, errors.New("invalid date")
}

func (p *WorldClockPlugin) buildMessage(t time.Time, formatter func(time.Time) string) string {
	messageParts := make([]string, len(locations))

	for i, locName := range locationNames {
		timeInLocation := t.In(locations[locName])
		formattedTime := formatter(timeInLocation)
		messageParts[i] = fmt.Sprintf("%v %v %v", locationIcons[locName], formattedTime, locName)
	}
	return strings.Join(messageParts, "\n")
}

func (p *WorldClockPlugin) fmtTime(t time.Time) string {
	return t.Format(timeFormat)
}

func (p *WorldClockPlugin) fmtTimestamp(t time.Time) string {
	_, offset := t.Zone()
	timestampInLoc := t.Add(time.Duration(offset) * time.Second).Unix()
	return strconv.FormatInt(timestampInLoc, 10)
}

func loadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		slog.Warn("Error loading location", err)
		return nil
	}
	return location
}
