package userconfig

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func Test_strDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		expect  time.Duration
		value   string
		wantErr bool
	}{
		{10 * time.Minute, `"10m"`, false},
		{2 * time.Second, `"2s"`, false},
		{2 * time.Second, `2000000000`, false},
		{0, `x`, true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			r := require.New(t)
			c := &Config{}
			data := []byte(fmt.Sprintf(`{"pomodoroDuration": %v}`, tt.value))
			err := json.Unmarshal(data, c)
			if tt.wantErr {
				r.Error(err)
			} else {
				r.NoError(err)
				r.Equal(tt.expect, c.PomodoroDuration())
			}
		})
	}
}
