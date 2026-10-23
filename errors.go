package dynoutbox

import "errors"

// ErrInvalidEvent is matched, through errors.Is, by every event validation error.
var ErrInvalidEvent = errors.New("dynoutbox: invalid event")

// ValidationError reports which event field failed validation and which rule it
// broke. It never contains the rejected value, which may hold personal data.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return ErrInvalidEvent.Error() + ": " + e.Field + ": " + e.Reason
}

// Unwrap makes errors.Is(err, ErrInvalidEvent) true.
func (e *ValidationError) Unwrap() error { return ErrInvalidEvent }

func invalid(field, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}
