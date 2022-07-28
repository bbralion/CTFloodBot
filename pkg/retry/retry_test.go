package retry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func assertNumCallsFunc(req *require.Assertions, n int, err error) RecoverFunc {
	ctr := 0
	return func() error {
		req.Less(ctr, n, "should be called %d times at most", n)
		ctr++
		if ctr == n {
			return err
		}
		return errors.New("fake recoverable error")
	}
}

func testStrategy(req *require.Assertions, n int, strategy func(RecoverFunc) error) {
	req.NoError(strategy(assertNumCallsFunc(req, n, nil)))
	e := errors.New("fake unrecoverable error")
	req.ErrorIs(e, strategy(assertNumCallsFunc(req, n, Unrecoverable(e))))
}

func TestRetry(t *testing.T) {
	req := require.New(t)

	for i := 1; i < 4; i++ {
		testStrategy(req, i, Backoff)
		testStrategy(req, i, Static)
	}
}
