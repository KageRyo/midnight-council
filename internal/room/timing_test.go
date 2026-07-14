package room

import (
	"strings"
	"testing"
	"time"
)

func TestPhaseDurationsValidate(t *testing.T) {
	tests := []struct {
		name      string
		durations PhaseDurations
		wantError string
	}{
		{
			name:      "valid",
			durations: PhaseDurations{Night: time.Second, DayDiscussion: time.Second, DayVoting: time.Second},
		},
		{
			name:      "night must be positive",
			durations: PhaseDurations{DayDiscussion: time.Second, DayVoting: time.Second},
			wantError: "night duration",
		},
		{
			name:      "discussion must be positive",
			durations: PhaseDurations{Night: time.Second, DayVoting: time.Second},
			wantError: "day discussion duration",
		},
		{
			name:      "voting must be positive",
			durations: PhaseDurations{Night: time.Second, DayDiscussion: time.Second},
			wantError: "day voting duration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.durations.Validate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("err = %v, want error containing %q", err, test.wantError)
			}
		})
	}
}
