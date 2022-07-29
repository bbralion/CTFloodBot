package services

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bbralion/CTFloodBot/internal/mocks"
	"github.com/go-logr/logr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestPollingUpdateProvider(t *testing.T) {
	ctrl, req := gomock.NewController(t), require.New(t)

	mockClient := mocks.NewMockTGBotAPIHTTPClient(ctrl)
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{Body: io.NopCloser(bytes.NewBuffer([]byte(`{"ok":true,"result":{}}`)))}, nil)
	api, err := tgbotapi.NewBotAPIWithClient("fake-token", "http://endpoint/fake%s/%s", mockClient)
	req.NoError(err)

	up := NewPollingUpdateProvider(logr.Discard(), api)
	ctx, cancel := context.WithCancel(context.Background())

	mockClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{Body: io.NopCloser(bytes.NewBuffer([]byte(`{"ok":true,"result":[{"update_id":1}]}`)))}, nil
	}).AnyTimes()
	updates := up.Updates(ctx)
	<-ctx.Done()

	// Update received
	req.Eventually(func() bool {
		select {
		case update, ok := <-updates:
			req.True(ok, "one update must be received")
			req.Equal(1, update.UpdateID)
			return true
		default:
		}
		return false
	}, time.Second, time.Millisecond*50)

	// Channel closed
	req.Eventually(func() bool {
		select {
		case _, ok := <-updates:
			req.False(ok, "channel must be closed")
			return true
		default:
		}
		return false
	}, time.Second, time.Millisecond*50)
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
