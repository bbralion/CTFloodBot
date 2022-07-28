package retry

import (
	"errors"
	"fmt"
	"time"
)

const (
	DefaultBackoffMinDelay = time.Millisecond * 50
	DefaultBackoffMaxDelay = time.Minute * 10
	DefaultBackoffFactor   = 2
)

type RecoverFunc func() error

type unrecoverableError struct {
	wrapped error
}

func (e unrecoverableError) Error() string {
	return fmt.Sprintf("unrecoverable error: %s", e.wrapped.Error())
}

func (e unrecoverableError) Unwrap() error {
	return e.wrapped
}

// Recoverable is used to explicitly mark an error as recoverable
func Recoverable(err error) error {
	return err
}

// Unrecoverable wraps an error to indicate that it is not recoverable from,
// after which retries will be stopped and it will be returned
func Unrecoverable(err error) error {
	return unrecoverableError{err}
}

// Backoff runs the function using the backoff retry algorithm
func Backoff(f RecoverFunc) error {
	delay := DefaultBackoffMinDelay
	for {
		var ue unrecoverableError
		if err := f(); err == nil {
			return nil
		} else if errors.As(err, &ue) {
			return ue.Unwrap()
		}

		time.Sleep(delay)

		delay *= DefaultBackoffFactor
		if delay > DefaultBackoffMaxDelay {
			delay = DefaultBackoffMaxDelay
		}
	}
}

const DefaultStaticDelay = time.Second

// Static runs the function using a static retry delay
func Static(f RecoverFunc) error {
	for {
		var ue unrecoverableError
		if err := f(); err == nil {
			return nil
		} else if errors.As(err, &ue) {
			return ue.Unwrap()
		}

		time.Sleep(DefaultStaticDelay)
	}
}
