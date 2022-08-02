package services

import (
	"context"
	"encoding/json"
	"runtime"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// DefaultCapacity is the default capacity which should be passed into Map* and As* functions.
// 100 is selected because Telegram's long polling API limits number of updates received to 100.
const DefaultCapacity = 100

// DefaultDecodeBufferSize specifies the default buffer size used for decoding responses
// in various streamers. Can be changed to, well, change the default buffer size.
var DefaultDecodeBufferSize = 256 << 10

// Maybe defines a type that either contains a value or an error.
type Maybe[T any] struct {
	Value T
	Error error
}

// Stream is a readonly channel of some type.
type Stream[T any] <-chan T

// MappedStream maps a stream to a stream in parallel using the given mapper. runtime.NumCPU() goroutines
// are used for mapping, and the returned stream will have the same capacity as the input stream.
// The output order after processing is not synchronized or defined.
func MappedStream[T, K any](in Stream[Maybe[T]], mapper func(T) (K, error)) Stream[Maybe[K]] {
	var wg sync.WaitGroup
	n := runtime.NumCPU()
	out := make(chan Maybe[K], cap(in))

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for {
				job, ok := <-in
				if !ok {
					return
				}

				result := Maybe[K]{
					Error: job.Error,
				}
				if result.Error == nil {
					result.Value, result.Error = mapper(job.Value)
				}
				out <- result
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// RawUpdate is the raw JSON representation of an Update message.
type RawUpdate []byte

// UnmarshalJSON is like json.RawMessage's UnmarshalJSON, however instead of
// copying the data it simply assigns it. Specifically, this means that
// the data used during decoding should not be reused elsewhere afterwards (i.e. no sync.Pool)
func (r *RawUpdate) UnmarshalJSON(m []byte) error {
	*r = m
	return nil
}

// RawStream is a stream of raw updates.
type RawStream Stream[Maybe[RawUpdate]]

// AsTgBotAPI converts a RawStream into a stream of tgbotapi-style updates.
func (s RawStream) AsTgBotAPI() Stream[Maybe[tgbotapi.Update]] {
	return MappedStream(Stream[Maybe[RawUpdate]](s), func(u RawUpdate) (tu tgbotapi.Update, err error) {
		err = json.Unmarshal([]byte(u), &tu)
		return
	})
}

// RawStreamer is a provider of RawUpdate's updates via an unbuffered stream.
type RawStreamer interface {
	// Stream launches a single instance of the streamer. In general, it isn't safe to use this concurrently.
	// On context cancelation/deadline the streamer must stop streaming and close the stream.
	Stream(ctx context.Context) RawStream
}
