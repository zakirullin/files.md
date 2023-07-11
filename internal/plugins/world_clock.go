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

func loadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		slog.Warn("Error loading location", err)
		return nil
	}
	return location
}

type WorldClockPlugin struct {
	userID int64
	tg     TGInterface
}

func NewWorldClockPlugin(userID int64, tg TGInterface) *WorldClockPlugin {
	return &WorldClockPlugin{userID, tg}
}

func (p *WorldClockPlugin) ExecutePlugin(msgText string) bool {
	var message string
	var time time.Time
	var err error

	time, err = p.parseDate(msgText)
	if err == nil {
		message = p.buildMessage(time, p.fmtTimestamp)
		p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		return true
	}

	time, err = p.parseTime(msgText)
	if err == nil {
		message = p.buildMessage(time, p.fmtTimestamp)
		p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		return true
	}

	time, err = p.parseTimestamp(msgText)
	if err == nil {
		message = p.buildMessage(time, p.fmtTime)
		p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		return true
	}

	return false
}

func (p *WorldClockPlugin) parseTimestamp(message string) (time.Time, error) {
	timestamp, err := strconv.ParseInt(message, 10, 64)
	if err == nil && timestamp > 999999 {
		return time.Unix(timestamp, 0).UTC(), nil
	}
	return time.Time{}, errors.New("Invalid timestamp")
}

func (p *WorldClockPlugin) parseTime(message string) (time.Time, error) {
	parsedTime, err := time.Parse(timeFormat, message)
	if err == nil {
		return parsedTime.UTC(), nil
	}
	return time.Time{}, errors.New("Invalid time")
}

func (p *WorldClockPlugin) parseDate(message string) (time.Time, error) {
	parsedDate, err := time.Parse(dateFormat, message)
	if err == nil {
		return parsedDate.UTC(), nil
	}
	return time.Time{}, errors.New("Invalid date")
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
