package services

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/stretchr/testify/require"
)

func startExpectingMuxClient(wg *sync.WaitGroup, req *require.Assertions, mux Multiplexer, updates []tgbotapi.Update, matchers MatcherGroup) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := mux.Register(ctx, matchers)
	req.NoError(err, "should register without error")

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range updates {
			update, ok := <-ch
			req.True(ok, "wanted %d updates but got %d", len(updates), i)

			// Currently only test message matches, since no others are supported
			req.True(matchers.MatchString(update.Message.Text), "got non-matching update")
			req.Equal(updates[i], update, "invalid order of updates")
		}
		cancel()
		// Channel should close on next send
		req.Eventually(func() bool {
			_, ok := <-ch
			return !ok
		}, time.Second*10, time.Millisecond*50)
	}()
}

func TestMultiplexer(t *testing.T) {
	req := require.New(t)

	updates := make([]tgbotapi.Update, 0, 13)
	for _, text := range []string{"/a", "/b", "/c", "/d", "/e", "/f", "/g", "/h", "/j", "/aboba", "/sus", "/0", "/aboba"} {
		updates = append(updates, tgbotapi.Update{Message: &tgbotapi.Message{Text: text}})
	}

	var wg sync.WaitGroup
	mux := NewMultiplexer(1)
	startExpectingMuxClient(&wg, req, mux, updates[:2], MatcherGroup{regexp.MustCompile("^/[ab]$")})
	startExpectingMuxClient(&wg, req, mux, updates[:6], MatcherGroup{regexp.MustCompile("^/[a-f]$")})
	startExpectingMuxClient(&wg, req, mux, updates[:9], MatcherGroup{regexp.MustCompile("^/[a-j]$")})
	startExpectingMuxClient(&wg, req, mux, updates[:9], MatcherGroup{regexp.MustCompile(".*")})
	startExpectingMuxClient(&wg, req, mux, []tgbotapi.Update{
		{Message: &tgbotapi.Message{Text: "/aboba"}},
		{Message: &tgbotapi.Message{Text: "/sus"}},
		{Message: &tgbotapi.Message{Text: "/aboba"}},
	}, MatcherGroup{regexp.MustCompile("^/(aboba|sus)$")})

	for _, update := range updates {
		mux.Serve(update)
	}

	// Wait for all clients to finish
	time.Sleep(time.Second * 2)

	// serve one last fake update to close the channels
	mux.Serve(updates[0])
	wg.Wait()
}
