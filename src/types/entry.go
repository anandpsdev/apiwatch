package types

import "time"

type Entry struct {
	ID         string           `json:"id"`
	Timestamp  time.Time        `json:"timestamp"`
	Method     string           `json:"method"`
	Path       string           `json:"path"`
	URL        string           `json:"url"`
	Route      string           `json:"route,omitempty"`
	StatusCode int              `json:"status_code"`
	Duration   time.Duration    `json:"duration"`
	Slow       bool             `json:"slow"`
	Request    RequestSnapshot  `json:"request"`
	Response   ResponseSnapshot `json:"response"`
	TraceID    string           `json:"trace_id,omitempty"`
	RequestID  string           `json:"request_id,omitempty"`
}

type RequestSnapshot struct {
	Headers       map[string][]string `json:"headers,omitempty"`
	Query         map[string][]string `json:"query,omitempty"`
	Body          string              `json:"body,omitempty"`
	BodyTruncated bool                `json:"body_truncated,omitempty"`
}

type ResponseSnapshot struct {
	StatusCode    int                 `json:"status_code"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`
	BodyTruncated bool                `json:"body_truncated,omitempty"`
}
