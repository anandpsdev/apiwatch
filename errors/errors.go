package apierrors

import "errors"

var (
	ErrNotFound      = errors.New("apiwatch: entry not found")
	ErrStoreClosed   = errors.New("apiwatch: store is closed")
	ErrInvalidConfig = errors.New("apiwatch: invalid configuration")
)
