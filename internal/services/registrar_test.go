package services

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/internal/mockproto"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCRegistrar(t *testing.T) {
	ctrl := gomock.NewController(t)
	req := require.New(t)
	logcfg := zap.NewDevelopmentConfig()
	logcfg.DisableStacktrace = true
	logger, err := logcfg.Build()
	req.NoError(err, "should be able to create logger")

	// Creation of registrar
	mockMuxClient := mockproto.NewMockMultiplexerServiceClient(ctrl)
	registrar, err := NewGRPCRegistrar(logger, mockMuxClient)
	req.NoError(err, "registrar creation shouldn't fail")

	// Registration without any matchers
	ctx := context.Background()
	_, _, err = registrar.Register(ctx, MatcherGroup{})
	req.ErrorIs(err, ErrNoMatchers, "shouldn't be able to register with no matchers")

	// Unrecoverable registration request fail
	mockMuxClient.EXPECT().RegisterHandler(gomock.Any(), gomock.Any()).Return(nil, status.Error(codes.Unauthenticated, "fake unauthenticated error"))
	updatech, errorch, err := registrar.Register(ctx, MatcherGroup{regexp.MustCompile("/command")})
	req.NoError(err, "registration shouldn't fail")
	req.Eventually(func() bool {
		select {
		case e, ok := <-errorch:
			req.True(ok, "error channel should not be empty")
			req.Error(e, "should receive proper error on channel")
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*50)
	req.Eventually(func() bool {
		select {
		case _, ok := <-updatech:
			req.False(ok, "update channel should close without update")
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*50)

	// Two failed registration requests followed by a successful registration with single update
	var tgUpdate tgbotapi.Update
	tgUpdate.Message = &tgbotapi.Message{
		Text: "message text",
	}
	tgUpdateBytes, err := json.Marshal(tgUpdate)
	req.NoError(err, "should be able to marshal telegram update")

	ctx, cancel := context.WithCancel(ctx)
	mockUpdateStream := mockproto.NewMockMultiplexerService_RegisterHandlerClient(ctrl)
	mockMuxClient.EXPECT().RegisterHandler(ctx, &genproto.RegisterRequest{
		Matchers: []string{"/command"},
	}).Return(nil, status.Error(codes.Unavailable, "fake network error 1"))
	mockMuxClient.EXPECT().RegisterHandler(ctx, &genproto.RegisterRequest{
		Matchers: []string{"/command"},
	}).Return(nil, status.Error(codes.Unavailable, "fake network error 2"))
	mockMuxClient.EXPECT().RegisterHandler(ctx, &genproto.RegisterRequest{
		Matchers: []string{"/command"},
	}).Return(mockUpdateStream, nil)
	mockUpdateStream.EXPECT().Recv().Return(&genproto.Update{Json: tgUpdateBytes}, nil)
	mockUpdateStream.EXPECT().Recv().Return(nil, status.FromContextError(context.Canceled).Err())

	updatech, errorch, err = registrar.Register(ctx, MatcherGroup{regexp.MustCompile("/command")})
	req.NoError(err, "registration shouldn't fail")

	var update tgbotapi.Update
	req.Eventually(func() bool {
		select {
		case update = <-updatech:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*50, "expected update on channel")
	req.Equal(tgUpdate, update, "received incorrect update")
	cancel()
	req.Eventually(func() bool {
		select {
		case update, ok := <-updatech:
			if ok {
				req.Fail("received unexpected update on channel", update)
			}
		default:
			return false
		}

		select {
		case err, ok := <-errorch:
			if ok {
				req.Fail("received unexpected error on channel", `errs: "%v" "%v"`, err, errors.Unwrap(err))
			}
		default:
			return false
		}
		return true
	}, time.Second, time.Millisecond*50, "expected updater goroutine to shut down and close channels")
}
