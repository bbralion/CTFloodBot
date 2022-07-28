package services

import (
	"context"
	"sync"
	"sync/atomic"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// Multiplexer allows multiplexing various update handlers based on matchers
type Multiplexer interface {
	// Register registers a new handler which will receive updates until the context is canceled.
	// Safe for concurrent use, so matchers can be registered from anywhere.
	Register(ctx context.Context, matchers MatcherGroup) (UpdateChan, error)
	// Serve multiplexes the update across the registered handlers.
	// Isn't safe for concurrent use, so all calls to Serve must be from a single goroutine.
	Serve(update tgbotapi.Update)
}

type (
	muxKey     uint64
	muxHandler struct {
		ctx      context.Context
		matchers MatcherGroup
		channel  chan tgbotapi.Update
	}
)

// mapMux is a default implementation of Multiplexer
type mapMux struct {
	curKey    muxKey
	store     sync.Map
	bufferLen int
}

func (m *mapMux) Register(ctx context.Context, matchers MatcherGroup) (UpdateChan, error) {
	if len(matchers) < 1 {
		return nil, ErrNoMatchers
	}

	key := muxKey(atomic.AddUint64((*uint64)(&m.curKey), 1))
	h := &muxHandler{ctx, matchers, make(chan tgbotapi.Update, m.bufferLen)}

	m.store.Store(key, h)
	return h.channel, nil
}

func (m *mapMux) delete(key muxKey, h *muxHandler) {
	m.store.Delete(key)
	close(h.channel)
}

func (m *mapMux) Serve(update tgbotapi.Update) {
	// Currently only messages are supported
	if update.Message == nil {
		return
	}

	m.store.Range(func(key, value any) bool {
		mkey, mvalue := key.(muxKey), value.(*muxHandler)

		// Fail-fast if the handler is already dead
		select {
		case <-mvalue.ctx.Done():
			m.delete(mkey, mvalue)
			return true
		default:
		}

		// Match and try to send if needed
		if mvalue.matchers.MatchString(update.Message.Text) {
			select {
			case <-mvalue.ctx.Done():
				m.delete(mkey, mvalue)
			case mvalue.channel <- update:
			}
		}
		return true
	})
}

// NewMultiplexer creates a new multiplexer with the
// specified buffer size of created update channels
func NewMultiplexer(bufferLen int) Multiplexer {
	return &mapMux{bufferLen: bufferLen}
}
