package plugins

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWorldClock_ExecutePlugin_With_Time(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	output, err := worldClockPlugin.Handle("15.06.2023 15:30:00")
	r.NoError(err)
	r.Equal("🕰 1686843000 UTC\n🔺 1686853800 MSK\n🏝 1686853800 CY\n⛰ 1686850200 ME\n", output)
}

func TestWorldClock_ExecutePlugin_With_Timestamp(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	output, err := worldClockPlugin.Handle("1686850214")
	r.NoError(err)
	r.Equal("🕰 15.06.2023 17:30:14 UTC\n🔺 15.06.2023 20:30:14 MSK\n🏝 15.06.2023 20:30:14 CY\n⛰ 15.06.2023 19:30:14 ME\n", output)
}

func TestWorldClock_ExecutePlugin_With_BotCommand(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	output, err := worldClockPlugin.Handle("cmdShowStart")
	r.NoError(err)
	r.Equal("", output)
}

func TestWorldClock_parseTimestamp(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	result, err := worldClockPlugin.parseTimestamp("1686850214")
	expectedResult := time.Unix(1686850214, 0).UTC()
	r.Nil(err)
	r.Equal(expectedResult, result)
}

func TestWorldClock_parseTimestamp_When_InvalidTimestamp(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	_, err := worldClockPlugin.parseTimestamp("ff6480214")
	r.EqualError(err, "invalid timestamp")
}

func TestWorldClock_parseTime(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	result, err := worldClockPlugin.parseTime("15.06.2023 15:30:00")
	expectedResult := time.Date(2023, time.June, 15, 15, 30, 0, 0, time.UTC)
	r.Nil(err)
	r.Equal(expectedResult, result)
}

func TestWorldClock_parseTime_When_InvalidTime(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	_, err := worldClockPlugin.parseTime("15_06_2023 15:30:00")
	r.EqualError(err, "invalid time")
}

func TestWorldClock_parseDate(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	result, err := worldClockPlugin.parseDate("15.06.2023")
	expectedResult := time.Date(2023, time.June, 15, 0, 0, 0, 0, time.UTC)
	r.Nil(err)
	r.Equal(expectedResult, result)
}

func TestWorldClock_parseDate_When_InvalidDate(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	_, err := worldClockPlugin.parseDate("41.06.2023")
	r.EqualError(err, "invalid date")
}

func TestWorldClock_buildMessage(t *testing.T) {
	r := require.New(t)
	worldClockPlugin := NewWorldClockPlugin()

	sentTime := time.Date(2023, time.June, 15, 15, 30, 0, 0, time.UTC)
	result := worldClockPlugin.buildMessage(sentTime, worldClockPlugin.fmtTime)
	expectedResult := "🕰 15.06.2023 15:30:00 UTC\n🔺 15.06.2023 18:30:00 MSK\n🏝 15.06.2023 18:30:00 CY\n⛰ 15.06.2023 17:30:00 ME\n"
	r.Equal(expectedResult, result)
}
