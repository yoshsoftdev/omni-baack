package consumererrors

import "errors"

var (
	ErrUnknownChannel = errors.New("unknown phone_number_id")
	ErrMalformedEvent = errors.New("malformed event payload")
)

// TerminalError indicates a failure that should route to DLQ without retry.
type TerminalError struct {
	Cause    error
	Category string
}

func (e *TerminalError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "terminal consumer error"
}

func (e *TerminalError) Unwrap() error {
	return e.Cause
}

func TerminalDLQ(category string, err error) error {
	return &TerminalError{Cause: err, Category: category}
}

func IsTerminal(err error) (*TerminalError, bool) {
	var te *TerminalError
	if errors.As(err, &te) {
		return te, true
	}
	return nil, false
}
