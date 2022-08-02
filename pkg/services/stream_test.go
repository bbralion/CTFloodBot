package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/stretchr/testify/require"
)

// streamContains validates that a stream contains the wanted values in any order
func streamContains[T any](req *require.Assertions, s Stream[T], want []T) {
	got := make([]bool, len(want))
	req.Eventually(func() bool {
		select {
		case val, ok := <-s:
			if !ok {
				for _, v := range got {
					req.True(v, "not all wanted values found on stream")
				}
				return true
			}

			var foundUnused bool
			for i, v := range got {
				if !v && reflect.DeepEqual(want[i], val) {
					got[i], foundUnused = true, true
					break
				}
			}
			req.True(foundUnused, "redundant value found on stream: %q", val)
		default:
		}
		return false
	}, time.Hour*5, time.Millisecond*50)
}

func TestRawStream_AsTgBotApi(t *testing.T) {
	tests := []struct {
		name    string
		updates []Maybe[RawUpdate]
		want    []Maybe[tgbotapi.Update]
	}{
		{
			name:    "no values",
			updates: nil,
			want:    nil,
		},
		{
			name: "values and error",
			updates: []Maybe[RawUpdate]{
				{Value: RawUpdate(`{"update_id":1,"inline_query":{"query":"inline-query-test"}}`)},
				{Value: RawUpdate(`{"update_id":2,"message":{"text":"message-test"}}`)},
				{Error: errors.New("connection error")},
			},
			want: []Maybe[tgbotapi.Update]{
				{Value: tgbotapi.Update{UpdateID: 1, InlineQuery: &tgbotapi.InlineQuery{Query: "inline-query-test"}}},
				{Value: tgbotapi.Update{UpdateID: 2, Message: &tgbotapi.Message{Text: "message-test"}}},
				{Error: errors.New("connection error")},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := require.New(t)

			stream := make(chan Maybe[RawUpdate], DefaultCapacity)
			go func() {
				defer close(stream)
				for _, u := range tt.updates {
					stream <- u
				}
			}()

			streamContains(req, RawStream(stream).AsTgBotAPI(), tt.want)
		})
	}
}

const (
	longPollTestMessagesTotal       = 10000
	longPollTestMessagesPerResponse = 100
)

func encodeAPIResponse(data any) ([]byte, error) {
	response := struct {
		Ok     bool
		Result any
	}{
		Ok:     true,
		Result: data,
	}

	b, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshaling api response: %w", err)
	}
	return b, nil
}

type longPollTestRoundTripper struct {
	data [][]byte
	mu   sync.Mutex
	i    int
}

func (rt *longPollTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		return nil, fmt.Errorf("context done: %w", req.Context().Err())
	default:
	}
	response := func(b []byte) *http.Response {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: int64(len(b)),
			Body:          io.NopCloser(bytes.NewBuffer(b)),
		}
	}

	if strings.HasSuffix(req.URL.Path, "getMe") {
		b, err := encodeAPIResponse(tgbotapi.User{ID: 1})
		if err != nil {
			return nil, err
		}
		return response(b), nil
	}

	var cur int
	rt.mu.Lock()
	cur, rt.i = rt.i, (rt.i+1)%len(rt.data)
	rt.mu.Unlock()
	return response(rt.data[cur]), nil
}

func newLongPollTestClient(req *require.Assertions, n int) *http.Client {
	data := make([][]byte, n*longPollTestMessagesTotal/longPollTestMessagesPerResponse)
	for i := range data {
		updates := make([]tgbotapi.Update, longPollTestMessagesPerResponse)
		for j := range updates {
			updates[j] = tgbotapi.Update{UpdateID: i*(longPollTestMessagesPerResponse) + j}
		}

		raw, err := encodeAPIResponse(updates)
		req.NoError(err)
		data[i] = raw
	}
	return &http.Client{Transport: &longPollTestRoundTripper{data: data}}
}

func longPollTestValidate(b *testing.B, s Stream[Maybe[tgbotapi.Update]]) {
	b.Helper()
	cnt, end := 0, longPollTestMessagesTotal*b.N
	for range s {
		cnt++
		if cnt == end {
			break
		}
	}
}

// Benchmark of simple long polling using a single goroutine receiving messages and decoding them
func BenchmarkNaiveLongPoll(b *testing.B) {
	req := require.New(b)
	b.StopTimer()
	b.Logf("Decoding %d messages %d times", longPollTestMessagesTotal, b.N)
	api, err := tgbotapi.NewBotAPIWithClient("aboba", tgbotapi.APIEndpoint, newLongPollTestClient(req, b.N))
	req.NoError(err)

	b.StartTimer()
	updateCh, _ := api.GetUpdatesChan(tgbotapi.UpdateConfig{})
	stream := make(chan Maybe[tgbotapi.Update])
	go func() {
		defer close(stream)
		for {
			v, ok := <-updateCh
			if !ok {
				return
			}

			stream <- Maybe[tgbotapi.Update]{Value: v}
		}
	}()
	longPollTestValidate(b, stream)
	api.StopReceivingUpdates()
}

// Benchmark of long poll using parallelized json decoding
func BenchmarkOptimizedLongPoll(b *testing.B) {
	req := require.New(b)
	b.StopTimer()
	b.Logf("Decoding %d messages %d times", longPollTestMessagesTotal, b.N)
	streamer, err := NewLongPollStreamer("https://api.telegram.org", "aboba", LongPollOptions{Client: newLongPollTestClient(req, b.N)})
	req.NoError(err)
	ctx, cancel := context.WithCancel(context.Background())

	b.StartTimer()
	stream := streamer.Stream(ctx).AsTgBotAPI()
	longPollTestValidate(b, stream)
	cancel()
	stream.Drain()
}
