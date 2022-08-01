package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/stretchr/testify/require"
)

func streamIs[T any](req *require.Assertions, s Stream[T], want []T) {
	got := 0
	req.Eventually(func() bool {
		select {
		case val, ok := <-s:
			if !ok {
				req.Equal(len(want), got, "less values on stream than wanted")
				return true
			}
			req.Less(got, len(want), "more updates on stream than wanted")
			req.Equal(want[got], val)
			got++
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

			stream := make(chan Maybe[RawUpdate])
			go func() {
				defer close(stream)
				for _, u := range tt.updates {
					stream <- u
				}
			}()

			streamIs(req, RawStream(stream).AsTgBotAPI(DefaultCapacity), tt.want)
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
	return json.Marshal(response)
}

type longPollTestRoundTripper struct {
	data [][]byte
	mu   sync.Mutex
	i    int
}

func (rt *longPollTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	default:
	}

	if strings.HasSuffix(req.URL.Path, "getMe") {
		b, err := encodeAPIResponse(tgbotapi.User{ID: 1})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBuffer(b))}, nil
	}

	var cur int
	rt.mu.Lock()
	cur, rt.i = rt.i, (rt.i+1)%len(rt.data)
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBuffer(rt.data[cur])),
	}, nil
}

func newLongPollTestRoundTripper(b *testing.B) *longPollTestRoundTripper {
	data := make([][]byte, b.N*longPollTestMessagesTotal/longPollTestMessagesPerResponse)
	for i := range data {
		updates := make([]tgbotapi.Update, longPollTestMessagesPerResponse)
		for j := range updates {
			updates[j] = tgbotapi.Update{UpdateID: i*(longPollTestMessagesPerResponse) + j}
		}

		raw, err := encodeAPIResponse(updates)
		if err != nil {
			b.Error(err)
			b.FailNow()
			return nil
		}
		data[i] = raw
	}
	return &longPollTestRoundTripper{data: data}
}

func longPollTestValidate(b *testing.B, s Stream[Maybe[tgbotapi.Update]]) {
	end := longPollTestMessagesTotal * b.N
	i, cnt := 0, 0
	for u := range s {
		if u.Error != nil {
			b.Error(u.Error)
			b.FailNow()
		} else if u.Value.UpdateID != i {
			b.Errorf("expected %d but got %d", i, u.Value.UpdateID)
			b.FailNow()
		}

		i = (i + 1) % (b.N * longPollTestMessagesTotal)
		cnt++
		if cnt == end {
			break
		}
	}
}

// Benchmark of simple long polling using a single goroutine receiving messages and decoding them
func BenchmarkNaiveLongPoll(b *testing.B) {
	b.StopTimer()
	b.Logf("Decoding %d messages %d times", longPollTestMessagesTotal, b.N)
	api, err := tgbotapi.NewBotAPIWithClient("aboba", tgbotapi.APIEndpoint, &http.Client{Transport: newLongPollTestRoundTripper(b)})
	if err != nil {
		b.Error(err)
		b.FailNow()
	}

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
	b.StopTimer()
	b.Logf("Decoding %d messages %d times", longPollTestMessagesTotal, b.N)
	streamer, err := NewLongPollStreamer("https://api.telegram.org", "aboba", LongPollOptions{Client: &http.Client{Transport: newLongPollTestRoundTripper(b)}})
	if err != nil {
		b.Error(err)
		b.FailNow()
	}
	ctx, cancel := context.WithCancel(context.Background())

	b.StartTimer()
	stream := streamer.Stream(ctx)
	longPollTestValidate(b, stream.AsTgBotAPI(DefaultCapacity))
	cancel()
}
