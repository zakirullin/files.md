package plugins

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"zakirullin/dumpbot/pkg/tg"

	"golang.org/x/exp/slog"
)

const (
	timeFormat = "02.01.2006 15:04:05"
)

var (
	locationNames = []string{"utc", "msk", "cyprus", "me"}
	locations     = map[string]*time.Location{
		"utc":    loadLocation("UTC"),
		"msk":    loadLocation("Europe/Moscow"),
		"cyprus": loadLocation("Asia/Nicosia"),
		"me":     loadLocation("Europe/Podgorica"),
	}
	locationIcons = map[string]string{
		"utc":    "🕰",
		"msk":    "🔺",
		"cyprus": "🏝",
		"me":     "🏝",
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
	tg     tg.TGInterface
}

func NewWorldClockPlugin(userID int64, tg tg.TGInterface) *WorldClockPlugin {
	return &WorldClockPlugin{userID, tg}
}

func (p *WorldClockPlugin) ExecutePlugin(msgText string) bool {
	var message string
	var time time.Time
	var err error

	time, err = p.parseTime(msgText)
	if err == nil {
		message = p.buildMessage(time, fmtTimestamp)
		p.tg.Send(p.userID, message, nil, tg.MarkupHTML)
		return true
	}

	time, err = p.parseTimestamp(msgText)
	if err == nil {
		message = p.buildMessage(time, fmtTime)
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
	return time.Time{}, errors.New("Not a valid timestamp")
}

func (p *WorldClockPlugin) parseTime(message string) (time.Time, error) {
	parsedTime, err := time.Parse(timeFormat, message)
	if err == nil {
		return parsedTime.UTC(), nil
	}
	return time.Time{}, errors.New("Not a valid time")
}

func (p *WorldClockPlugin) buildMessage(t time.Time, formatter func(time.Time) string) string {
	messageParts := make([]string, len(locations))

	for _, locName := range locationNames {
		timeInLocation := t.In(locations[locName])
		formattedTime := formatter(timeInLocation)
		messageParts = append(
			messageParts,
			fmt.Sprintf("%v %v %v", locationIcons[locName], formattedTime, locName),
		)
	}
	return strings.Join(messageParts, "\n")
}

func fmtTime(t time.Time) string {
	return t.Format(timeFormat)
}

func fmtTimestamp(t time.Time) string {
	_, offset := t.Zone()
	timestampInLoc := t.Add(time.Duration(offset) * time.Second).Unix()
	return strconv.FormatInt(timestampInLoc, 10)
}
