package services

import (
	"context"
	"runtime"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	jsoniter "github.com/json-iterator/go"
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

// Drain drains the stream completely.
func (s Stream[T]) Drain() {
	for {
		_, ok := <-s
		if !ok {
			return
		}
	}
}

// TODO: place inside MappedStream once type decs inside generic functions are supported
type mapJob[T, K any] struct {
	in  Maybe[T]
	out chan Maybe[K]
}

// MappedStream maps a stream to a channel in parallel using the given mapper.
// runtime.NumCPU() goroutines are used for mapping, and the returned channel
// will have the capacity specified in arguments.
//
// Make sure to call stream.Drain if you suddenly stop reading from the returned stream.
func MappedStream[T, K any](in Stream[Maybe[T]], mapper func(T) (K, error), capacity int,
) Stream[Maybe[K]] {
	n := runtime.NumCPU()

	// These channels are closed by the parallelizing goroutine
	outOrder := make(chan Stream[Maybe[K]], n)
	jobs := make(chan mapJob[T, K], n)

	// Pool of channels, should be ok to use here since the goroutines here should be longlived
	chpool := sync.Pool{
		New: func() any {
			return make(chan Maybe[K])
		},
	}
	// Parallelize input
	go func() {
		defer close(outOrder)
		defer close(jobs)

		for {
			v, ok := <-in
			if !ok {
				return
			}

			ch := chpool.Get().(chan Maybe[K])
			jobs <- mapJob[T, K]{in: v, out: ch}
			outOrder <- ch
		}
	}()

	mapF := func() {
		for {
			job, ok := <-jobs
			if !ok {
				return
			}

			result := Maybe[K]{
				Error: job.in.Error,
			}
			if result.Error == nil {
				result.Value, result.Error = mapper(job.in.Value)
			}
			job.out <- result
		}
	}

	out := make(chan Maybe[K], capacity)
	// Synchronize output
	go func() {
		defer close(out)
		for in := range outOrder {
			out <- <-in
		}
	}()

	for i := 0; i < n; i++ {
		go mapF()
	}
	return out
}

// RawUpdate is either the raw JSON representation of an Update message.
type RawUpdate jsoniter.RawMessage

// RawStream is a stream of raw updates.
type RawStream Stream[Maybe[RawUpdate]]

// AsTgBotAPI converts a RawStream into a stream of tgbotapi-style updates.
func (s RawStream) AsTgBotAPI(capacity int) Stream[Maybe[tgbotapi.Update]] {
	return MappedStream(Stream[Maybe[RawUpdate]](s), func(u RawUpdate) (tu tgbotapi.Update, err error) {
		err = jsoniter.Unmarshal([]byte(u), &tu)
		return
	}, capacity)
}

// RawStreamer is a provider of RawUpdate's updates via an unbuffered stream.
type RawStreamer interface {
	Stream(ctx context.Context) RawStream
}
