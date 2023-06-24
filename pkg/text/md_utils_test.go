package text

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_AddDailyNote(t *testing.T) {
	r := require.New(t)
	now = func() time.Time {
		return time.Date(2023, 05, 30, 10, 04, 36, 0, time.UTC)
	}

	type testcase struct {
		name string
		md   string
		note string
		want string
	}

	tests := []testcase{
		{
			name: "Empty MD",
			note: "note 1",
			want: "#### 30, Tuesday\n* note 1\n",
		},
		{
			"New daily note",
			"#### 29, Tuesday\n* note 1",
			"note 2",
			"#### 29, Tuesday\n* note 1\n\n#### 30, Tuesday\n* note 2\n",
		},
		{
			"Append daily note",
			"#### 29, Tuesday\n* note 1\n\n#### 30, Tuesday\n* note 2",
			"note 3",
			"#### 29, Tuesday\n* note 1\n\n#### 30, Tuesday\n* note 2\n* note 3\n",
		},

		{
			"Append daily note",
			"#### 29, Tuesday\n* note 1\n\n#### 30, Tuesday\nsome text\n* note 2",
			"note 3",
			"#### 29, Tuesday\n* note 1\n\n#### 30, Tuesday\n* note 3\n\nsome text\n* note 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddDailyNote(tt.md, tt.note)
			r.Equal(tt.want, got)
		})
	}
}
