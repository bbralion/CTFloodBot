package retry

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	DelayScheduler func() time.Duration
	ErrTransformer func(error) error
)

type recoverError struct {
	wrapped     error
	recoverable bool
}

func (e recoverError) Error() string {
	s := "recoverable error: %s"
	if !e.recoverable {
		s = "un" + s
	}
	return fmt.Sprintf(s, e.wrapped.Error())
}

func (e recoverError) Unwrap() error {
	return e.wrapped
}

// Recoverable is used to explicitly mark an error as recoverable
func Recoverable(err error) error {
	return recoverError{err, true}
}

// Unrecoverable wraps an error to indicate that it is not recoverable from,
// after which retries will be stopped and it will be returned
func Unrecoverable(err error) error {
	return recoverError{err, false}
}

// Recover runs the function using a custom delay scheduler
func Recover[T any](f func() (error, T), s DelayScheduler, et ...ErrTransformer) (T, error) {
	for {
		err, ret := f()
		for _, t := range et {
			err = t(err)
		}

		var re recoverError
		if err == nil {
			return ret, nil
		} else if errors.As(err, &re) && !re.recoverable {
			return ret, re.Unwrap()
		}

		time.Sleep(s())
	}
}

const (
	DefaultBackoffMinDelay = time.Millisecond * 50
	DefaultBackoffMaxDelay = time.Minute * 10
	DefaultBackoffFactor   = 2
)

// Backoff runs the function using the backoff retry algorithm
func Backoff[T any](f func() (error, T), et ...ErrTransformer) (T, error) {
	delay, next := time.Duration(0), DefaultBackoffMinDelay
	return Recover(f, func() time.Duration {
		delay, next = next, next*DefaultBackoffFactor
		if next > DefaultBackoffMaxDelay {
			next = DefaultBackoffMaxDelay
		}
		return delay
	}, et...)
}

const DefaultStaticDelay = time.Second

// Static runs the function using a static retry delay
func Static[T any](f func() (error, T), et ...ErrTransformer) (T, error) {
	return Recover(f, func() time.Duration {
		return DefaultStaticDelay
	}, et...)
}

// IsGRPCUnavailable is a helper for testing whether the error resembles a gRPC Unavailable status
func IsGRPCUnavailable(err error) bool {
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.Unavailable
}

// IsGRPCUnavailable is a helper for testing whether the error
// resembles a gRPC Canceled or DeadlineExceeded status
func IsGRPCCanceled(err error) bool {
	s, ok := status.FromError(err)
	return ok && (s.Code() == codes.Canceled || s.Code() == codes.DeadlineExceeded)
}
