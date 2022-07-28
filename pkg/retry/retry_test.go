package retry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func assertNumCallsFunc(req *require.Assertions, n int, tmpErr, finalErr error) func() (error, any) {
	ctr := 0
	return func() (error, any) {
		req.Less(ctr, n, "should be called %d times at most", n)
		ctr++
		if ctr == n {
			return finalErr, nil
		}
		return tmpErr, nil
	}
}

func testStrategy(req *require.Assertions, n int, strategy func(func() (error, any), ...ErrTransformer) (any, error)) {
	_, err := strategy(assertNumCallsFunc(req, n, errors.New("fake recoverable error"), nil))
	req.NoError(err)
	e := errors.New("fake unrecoverable error")
	_, err = strategy(assertNumCallsFunc(req, n, errors.New("fake recoverable error"), Unrecoverable(e)))
	req.ErrorIs(e, err)
}

func TestRetry(t *testing.T) {
	req := require.New(t)

	for i := 1; i < 4; i++ {
		testStrategy(req, i, Backoff[any])
		testStrategy(req, i, Static[any])
	}
}
