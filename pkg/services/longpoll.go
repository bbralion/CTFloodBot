package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// DefaultLongPollTimeout is the default timeout used for long polling
const DefaultLongPollTimeout = time.Second * 60

// LongPollOptions specifies various options to use inside the long poll streamer.
// Offset, Limit and Timeout specify the values to send to Telegram's getUpdates method.
// By default a Timeout of DefaultLongPollTimeout is used.
type LongPollOptions struct {
	Offset  int
	Limit   int
	Timeout time.Duration
	// The HTTP client to use for requests
	Client *http.Client
}

type longPollStreamer struct {
	opts        LongPollOptions
	endpointURL *url.URL
	params      url.Values
	iterator    *jsoniter.Iterator
}

func (s *longPollStreamer) poll(ctx context.Context) (*http.Response, error) {
	s.endpointURL.RawQuery = s.params.Encode()
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", s.endpointURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("preparing request: %w", err)
	}

	resp, err := s.opts.Client.Do(req)
	if err != nil {
		// Unwrap url.Error returned from do to avoid leaking url with bot token
		return nil, fmt.Errorf("doing poll request: %w", errors.Unwrap(err))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad response code while polling: %s", resp.Status)
	}
	return resp, nil
}

func (s *longPollStreamer) parseUpdates(stream chan Maybe[RawUpdate], body io.ReadCloser) error {
	s.iterator.Reset(body)
	defer body.Close()
	defer s.iterator.Reset(nil)

	// Parse API response wrapper
	var resp struct {
		Ok          bool
		Description string
		Result      []jsoniter.RawMessage
	}
	if s.iterator.ReadVal(&resp); s.iterator.Error != nil {
		return fmt.Errorf("parsing getUpdates response: %w", s.iterator.Error)
	}
	if !resp.Ok {
		return fmt.Errorf("getUpdates response.Ok is false: %s", resp.Description)
	}

	for _, u := range resp.Result {
		stream <- Maybe[RawUpdate]{Value: RawUpdate(u)}
	}
	if len(resp.Result) > 0 {
		val := jsoniter.Get(resp.Result[len(resp.Result)-1], "update_id")
		updateID := val.ToInt()
		if err := val.LastError(); err != nil {
			return fmt.Errorf("retrieving update_id: %w", err)
		}
		s.params.Set("offset", strconv.Itoa(updateID+1))
	}
	return nil
}

func (s *longPollStreamer) Stream(ctx context.Context) RawStream {
	stream := make(chan Maybe[RawUpdate])
	go func() {
		defer close(stream)

		for {
			resp, err := s.poll(ctx)
			if err != nil {
				// If the context is finished, then the error we received is *probably* a timeout/canceled
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Global context isn't finished, which means that this is a temporary timeout
				if errors.Is(err, context.DeadlineExceeded) {
					stream <- Maybe[RawUpdate]{Error: fmt.Errorf("temporary timeout while polling: %w", err)}
					continue
				}
				stream <- Maybe[RawUpdate]{Error: err}
				return
			}

			if err := s.parseUpdates(stream, resp.Body); err != nil {
				stream <- Maybe[RawUpdate]{Error: err}
				return
			}
		}
	}()
	return stream
}

// NewLongPollStreamer starts a long polling streamer on the given endpoint in the form
// "https://api.telegram.org" using the specified token for authorization.
// Long poll requests will be made to "{endpoint}/bot{token}/getUpdates".
func NewLongPollStreamer(endpoint, token string, opts LongPollOptions) (RawStreamer, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid long poll endpoint: %w", err)
	}

	// Set proper defaults in options and client
	if opts.Timeout == 0 {
		opts.Timeout = DefaultLongPollTimeout
	}
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	// Timeout is always set to avoid short polling, other opts are used only if set
	endpointURL.Path = path.Join(endpointURL.Path, "bot"+token, "getUpdates")
	params := make(url.Values)
	params.Set("timeout", strconv.Itoa(int(opts.Timeout.Seconds())))
	if opts.Offset != 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	return &longPollStreamer{
		opts, endpointURL, params,
		jsoniter.ParseBytes(jsoniter.ConfigFastest, make([]byte, DefaultDecodeBufferSize)),
	}, nil
}
