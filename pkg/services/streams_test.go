package services

import (
	"errors"
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
