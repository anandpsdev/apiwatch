package types

import "time"

type Filter struct {
	Method      string
	Path        string
	Status      int
	StatusClass string
	Search      string
	Slow        *bool
	MinDuration time.Duration
	MaxDuration time.Duration
	From        time.Time
	To          time.Time
	Limit       int
	Offset      int
}

func (f Filter) Normalize() Filter {
	if f.Limit <= 0 {
		f.Limit = 100
	}

	if f.Limit > 1000 {
		f.Limit = 1000
	}

	if f.Offset < 0 {
		f.Offset = 0
	}

	return f
}
