package sched

import (
	"testing"
	"time"

	"zakirullin/stuffbot/pkg/txt"

	"github.com/stretchr/testify/require"
)

func TestUcfirst(t *testing.T) {
	r := require.New(t)

	res := txt.Ucfirst("abc")

	r.Equal("Abc", res)
}

func TestUcfirstRu(t *testing.T) {
	r := require.New(t)

	res := txt.Ucfirst("абв")

	r.Equal("Абв", res)
}

func TestTomorrow(t *testing.T) {
	r := require.New(t)

	savedNow := Now
	defer func() {
		Now = savedNow
	}()
	Now = func() time.Time {
		return time.Date(1970, 1, 1, 10, 45, 10, 0, time.UTC)
	}

	tomorrow := Tomorrow()
	r.Equal(time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC).Unix(), tomorrow)
}

func TestFormatTaskDate(t *testing.T) {
	r := require.New(t)

	savedNow := Now
	defer func() {
		Now = savedNow
	}()
	Now = func() time.Time {
		return time.Date(1970, 1, 1, 10, 45, 10, 0, time.UTC)
	}

	r.Equal("Today", formatTaskDate(0))
	r.Equal("Today", formatTaskDate(10))         // seconds are ignored
	r.Equal("Today", formatTaskDate(60*60*2))    // hours are ignored
	r.Equal("Today", formatTaskDate(60*60*24-1)) // hours are ignored
	r.Equal("Tomorrow", formatTaskDate(60*60*24))
	r.Equal("Tomorrow", formatTaskDate(60*60*24))
	r.Equal("Saturday 03", formatTaskDate(60*60*24*2))
	r.Equal("Wednesday 07", formatTaskDate(60*60*24*6))
	r.Equal("Next Thursday 08", formatTaskDate(60*60*24*7))
}

func TestNextExcludeTodayClosestMonday(t *testing.T) {
	// Set Now function to a fixed time for testing
	savedNow := Now
	Now = func() time.Time {
		return time.Date(2024, time.September, 22, 15, 30, 0, 0, time.UTC) // Sunday, 3:30PM UTC
	}
	defer func() {
		Now = savedNow
	}()

	// Test cron expression for next Monday
	cronExpr := "0 0 * * 1"
	nextUnix := NextExcludeToday(cronExpr)

	expectedNext := time.Date(2024, time.September, 23, 0, 0, 0, 0, time.UTC) // Monday 9:00AM UTC
	require.Equal(t, expectedNext.Unix(), nextUnix, "next scheduled time should be Monday 9:00 AM")

	// Test invalid cron expression, expecting a panic
	invalidCronExpr := "invalid cron expression"
	require.Panics(t, func() { NextExcludeToday(invalidCronExpr) }, "should panic for invalid cron expression")
}

func TestNextExcludeToday(t *testing.T) {
	savedNow := Now
	defer func() {
		Now = savedNow
	}()
	Now = func() time.Time {
		return time.Date(2024, time.September, 23, 10, 0, 0, 0, time.UTC)
	}

	// Test cron for Monday (0 0 * * 1)
	mondayCron := "0 0 * * 1"
	expectedMonday := time.Date(2024, time.September, 30, 0, 0, 0, 0, time.UTC).Unix()

	require.Equal(t, expectedMonday, NextExcludeToday(mondayCron))

	// Test cron for Wednesday (0 0 * * 3)
	wednesdayCron := "0 0 * * 3"
	expectedWednesday := time.Date(2024, time.September, 25, 0, 0, 0, 0, time.UTC).Unix()

	require.Equal(t, expectedWednesday, NextExcludeToday(wednesdayCron))

	// Test cron for Friday (0 0 * * 5)
	fridayCron := "0 0 * * 5"
	expectedFriday := time.Date(2024, time.September, 27, 0, 0, 0, 0, time.UTC).Unix()

	require.Equal(t, expectedFriday, NextExcludeToday(fridayCron))

	// Test cron for Sunday (0 0 * * 0)
	sundayCron := "0 0 * * 0"
	expectedSunday := time.Date(2024, time.September, 29, 0, 0, 0, 0, time.UTC).Unix()
	require.Equal(t, expectedSunday, NextExcludeToday(sundayCron))

	// Test cron for 1st of the month (0 0 1 * *)
	firstDayCron := "0 0 1 * *"
	expectedFirstDay := time.Date(2024, time.October, 1, 0, 0, 0, 0, time.UTC).Unix()
	require.Equal(t, expectedFirstDay, NextExcludeToday(firstDayCron))

	// Test cron for 15th of the month (0 0 15 * *)
	fifteenthDayCron := "0 0 15 * *"
	expectedFifteenthDay := time.Date(2024, time.October, 15, 0, 0, 0, 0, time.UTC).Unix()
	require.Equal(t, expectedFifteenthDay, NextExcludeToday(fifteenthDayCron))

	// Test cron for 31st of the month (0 0 31 * *)
	thirtyFirstDayCron := "0 0 31 * *"
	expectedThirtyFirstDay := time.Date(2024, time.October, 31, 0, 0, 0, 0, time.UTC).Unix()
	require.Equal(t, expectedThirtyFirstDay, NextExcludeToday(thirtyFirstDayCron))
}
