package plugins

import (
	"testing"
	"time"
	"zakirullin/stuffbot/pkg/tg/fake"

	"github.com/stretchr/testify/require"
)

func TestWorldClock_ExecutePlugin_With_Time(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	result := worldClockPlugin.ExecutePlugin("15.06.2023 15:30:00")
	r.True(result)
}

func TestWorldClock_ExecutePlugin_With_Timestamp(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	result := worldClockPlugin.ExecutePlugin("1686850214")
	r.True(result)
}

func TestWorldClock_ExecutePlugin_With_BotCommand(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	result := worldClockPlugin.ExecutePlugin("cmdShowStart")
	r.False(result)
}

func TestWorldClock_parseTimestamp(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	result, err := worldClockPlugin.parseTimestamp("1686850214")
	expectedResult := time.Unix(1686850214, 0).UTC()
	r.Nil(err)
	r.Equal(expectedResult, result)
}

func TestWorldClock_parseTimestamp_When_InvalidTimestamp(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	_, err := worldClockPlugin.parseTimestamp("ff6480214")
	r.EqualError(err, "Invalid timestamp")
}

func TestWorldClock_parseTime(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	result, err := worldClockPlugin.parseTime("15.06.2023 15:30:00")
	expectedResult := time.Date(2023, time.June, 15, 15, 30, 0, 0, time.UTC)
	r.Nil(err)
	r.Equal(expectedResult, result)
}

func TestWorldClock_parseTime_When_InvalidTime(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	_, err := worldClockPlugin.parseTime("15_06_2023 15:30:00")
	r.EqualError(err, "Invalid time")
}

func TestWorldClock_parseDate(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	result, err := worldClockPlugin.parseDate("15.06.2023")
	expectedResult := time.Date(2023, time.June, 15, 0, 0, 0, 0, time.UTC)
	r.Nil(err)
	r.Equal(expectedResult, result)
}

func TestWorldClock_parseDate_When_InvalidDate(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	_, err := worldClockPlugin.parseDate("41.06.2023")
	r.EqualError(err, "Invalid date")
}

func TestWorldClock_buildMessage(t *testing.T) {
	r := require.New(t)
	tg := fake.NewTG()
	worldClockPlugin := NewWorldClockPlugin(1, tg)

	time := time.Date(2023, time.June, 15, 15, 30, 0, 0, time.UTC)
	result := worldClockPlugin.buildMessage(time, worldClockPlugin.fmtTime)
	expectedResult := "🕰 15.06.2023 15:30:00 UTC\n🔺 15.06.2023 18:30:00 MSK\n🏝 15.06.2023 18:30:00 CY\n🏝 15.06.2023 17:30:00 ME"
	r.Equal(expectedResult, result)
}
