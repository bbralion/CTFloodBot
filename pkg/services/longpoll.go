package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

// DefaultLongPollTimeout is the default timeout used for long polling
const DefaultLongPollTimeout = time.Second * 60

// LongPollOptions specifies various options to use inside the long poll streamer.
// Offset, Limit and Timeout specify the values to send to Telegram's getUpdates method.
// By default a Timeout of DefaultLongPollTimeout is used, and DefaultCapacity will be used as the default Limit.
type LongPollOptions struct {
	Offset  int
	Limit   int
	Timeout time.Duration
	Client  *http.Client
}

type longPollStreamer struct {
	opts        LongPollOptions
	endpointURL *url.URL
	params      url.Values
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
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad response code while polling: %s", resp.Status)
	}
	return resp, nil
}

// readRespones tries to read the response in the fastest way possible. That is, if ContentLength
// is set, then we can use it in order to allocate a buffer wit the correct size from the get-go.
func (s *longPollStreamer) readResponse(resp *http.Response) (buf []byte, err error) {
	defer resp.Body.Close()

	if resp.ContentLength != -1 {
		buf = make([]byte, resp.ContentLength)
		_, err = io.ReadFull(resp.Body, buf)
	} else {
		buf, err = io.ReadAll(resp.Body)
	}
	return
}

func (s *longPollStreamer) parseUpdates(ctx context.Context, stream chan<- Maybe[RawUpdate], resp *http.Response) error {
	buf, err := s.readResponse(resp)
	if err != nil {
		return fmt.Errorf("reading getUpdates response: %w", err)
	}

	// Parse API response wrapper. The updates here are parsed as RawUpdate's, which simply set
	// their value to the correct portion of the prepared buffer, bypassing an extra copy.
	var apiResp struct {
		Ok          bool
		Description string
		Result      []RawUpdate
	}
	if err := json.Unmarshal(buf, &apiResp); err != nil {
		return fmt.Errorf("parsing getUpdates response: %w", err)
	}
	if !apiResp.Ok {
		return fmt.Errorf("getUpdates response.Ok is false: %s", apiResp.Description)
	}

	for _, u := range apiResp.Result {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		stream <- Maybe[RawUpdate]{Value: u}
	}
	if len(apiResp.Result) > 0 {
		var updateWrapper struct {
			UpdateID int `json:"update_id"`
		}
		if err := json.Unmarshal(apiResp.Result[len(apiResp.Result)-1], &updateWrapper); err != nil {
			return fmt.Errorf("retrieving update_id: %w", err)
		}
		s.params.Set("offset", strconv.Itoa(updateWrapper.UpdateID+1))
	}
	return nil
}

func (s *longPollStreamer) Stream(ctx context.Context) RawStream {
	stream := make(chan Maybe[RawUpdate], s.opts.Limit)
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

			if err := s.parseUpdates(ctx, stream, resp); err != nil {
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
	if opts.Limit == 0 {
		opts.Limit = DefaultCapacity
	}
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}

	// Timeout is always set to avoid short polling, other opts are used only if set
	endpointURL.Path = path.Join(endpointURL.Path, "bot"+token, "getUpdates")
	params := make(url.Values)
	params.Set("timeout", strconv.Itoa(int(opts.Timeout.Seconds())))
	params.Set("limit", strconv.Itoa(opts.Limit))
	params.Set("offset", strconv.Itoa(opts.Offset))
	return &longPollStreamer{opts, endpointURL, params}, nil
}
