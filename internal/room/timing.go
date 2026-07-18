package room

import (
	"fmt"
	"time"
)

const (
	DefaultNightDuration         = 90 * time.Second
	DefaultDayDiscussionDuration = 5 * time.Minute
	DefaultDayVotingDuration     = 60 * time.Second
	DefaultLastWordsDuration     = 30 * time.Second
)

type PhaseDurations struct {
	Night         time.Duration
	DayDiscussion time.Duration
	DayVoting     time.Duration
	LastWords     time.Duration
}

func DefaultPhaseDurations() PhaseDurations {
	return PhaseDurations{
		Night:         DefaultNightDuration,
		DayDiscussion: DefaultDayDiscussionDuration,
		DayVoting:     DefaultDayVotingDuration,
		LastWords:     DefaultLastWordsDuration,
	}
}

func (d PhaseDurations) Validate() error {
	values := []struct {
		name     string
		duration time.Duration
	}{
		{name: "night", duration: d.Night},
		{name: "day discussion", duration: d.DayDiscussion},
		{name: "day voting", duration: d.DayVoting},
		{name: "last words", duration: d.LastWords},
	}
	for _, value := range values {
		if value.duration <= 0 {
			return fmt.Errorf("%s duration must be positive", value.name)
		}
	}
	return nil
}

func (d PhaseDurations) durationFor(phase Phase) time.Duration {
	switch phase {
	case PhaseNight:
		return d.Night
	case PhaseDayDiscussion:
		return d.DayDiscussion
	case PhaseDayVoting:
		return d.DayVoting
	case PhaseLastWords:
		return d.LastWords
	default:
		return 0
	}
}
