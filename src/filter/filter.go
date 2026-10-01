package filter

import (
	"strings"

	"github.com/anandpsdev/apiwatch/src/types"
)

func StatusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return ""
	}
}

func Matches(entry types.Entry, filter types.Filter) bool {
	if filter.Method != "" &&
		!strings.EqualFold(entry.Method, filter.Method) {
		return false
	}

	if filter.Path != "" &&
		!strings.Contains(
			strings.ToLower(entry.Path),
			strings.ToLower(filter.Path),
		) {
		return false
	}

	if filter.Status != 0 &&
		entry.StatusCode != filter.Status {
		return false
	}

	if filter.StatusClass != "" &&
		!strings.EqualFold(StatusClass(entry.StatusCode), filter.StatusClass) {
		return false
	}

	if filter.Slow != nil && entry.Slow != *filter.Slow {
		return false
	}

	if filter.MinDuration > 0 && entry.Duration < filter.MinDuration {
		return false
	}

	if filter.MaxDuration > 0 && entry.Duration > filter.MaxDuration {
		return false
	}

	if !filter.From.IsZero() &&
		entry.Timestamp.Before(filter.From) {
		return false
	}

	if !filter.To.IsZero() &&
		entry.Timestamp.After(filter.To) {
		return false
	}

	if filter.Search != "" {
		search := strings.ToLower(filter.Search)

		if !strings.Contains(strings.ToLower(entry.Path), search) &&
			!strings.Contains(strings.ToLower(entry.Method), search) &&
			!strings.Contains(strings.ToLower(entry.URL), search) &&
			!strings.Contains(strings.ToLower(entry.Route), search) {
			return false
		}
	}

	return true
}
