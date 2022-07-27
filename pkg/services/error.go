package services

// error is the shared wrapper to be used for errors returned by services
type svcError struct {
	Wrapped error
	Prefix  string
	Message string
}

func (e *svcError) Unwrap() error {
	return e.Wrapped
}

func (e *svcError) Error() string {
	return e.Message
}

func wrap(e error, p, m string) *svcError {
	return &svcError{
		Wrapped: e,
		Prefix:  p,
		Message: m,
	}
}
