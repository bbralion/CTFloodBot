package retry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func assertNumCallsFunc(req *require.Assertions, n int, tmpErr, finalErr error) func() (any, error) {
	ctr := 0
	return func() (any, error) {
		req.Less(ctr, n, "should be called %d times at most", n)
		ctr++
		if ctr == n {
			return nil, finalErr
		}
		return nil, tmpErr
	}
}

func testStrategy(req *require.Assertions, n int, strategy func(func() (any, error), ...ErrTransformer) (any, error)) {
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
