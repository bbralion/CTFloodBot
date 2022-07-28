package services

import "go.uber.org/zap"

// error is the shared wrapper to be used for errors returned by services
type svcError struct {
	Wrapped error
	Info    string
	Message string
}

func (e *svcError) Unwrap() error {
	return e.Wrapped
}

func (e *svcError) Error() string {
	return e.Message
}

func (e *svcError) ZapFields() []zap.Field {
	return []zap.Field{zap.Error(e.Unwrap()), zap.String("info", e.Info), zap.String("message", e.Message)}
}

func wrap(e error, p, m string) *svcError {
	return &svcError{
		Wrapped: e,
		Info:    p,
		Message: m,
	}
}
