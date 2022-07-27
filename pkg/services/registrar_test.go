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
	"google.golang.org/grpc/status"
)

func TestGRPCRegistrarRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	req := require.New(t)

	// Creation of registrar
	mockMuxClient := mockproto.NewMockMultiplexerServiceClient(ctrl)
	registrar, err := NewGRPCRegistrar(mockMuxClient)
	req.NoError(err, "registrar creation shouldn't fail")

	// Registration without any matchers
	ctx, name := context.Background(), "fake"
	_, _, err = registrar.Register(ctx, name, []regexp.Regexp{})
	req.Error(err, "shouldn't be able to register with no matchers")

	// Failed registration request
	mockMuxClient.EXPECT().RegisterHandler(ctx, &genproto.RegisterRequest{
		Name:     name,
		Matchers: []string{"/command"},
	}).Return(nil, errors.New("fake register error"))
	_, _, err = registrar.Register(ctx, name, []regexp.Regexp{*regexp.MustCompile("/command")})
	req.Error(err, "shouldn't be able to continue if register request fails")

	// Successful registration with single update
	var tgUpdate tgbotapi.Update
	tgUpdate.Message = &tgbotapi.Message{
		Text: "message text",
	}
	tgUpdateBytes, err := json.Marshal(tgUpdate)
	req.NoError(err, "should be able to marshal telegram update")

	ctx, cancel := context.WithCancel(ctx)
	mockUpdateStream := mockproto.NewMockMultiplexerService_RegisterHandlerClient(ctrl)
	mockMuxClient.EXPECT().RegisterHandler(ctx, &genproto.RegisterRequest{
		Name:     name,
		Matchers: []string{"/command"},
	}).Return(mockUpdateStream, nil)
	mockUpdateStream.EXPECT().Recv().Return(&genproto.Update{Json: tgUpdateBytes}, nil)
	mockUpdateStream.EXPECT().Recv().Return(nil, status.FromContextError(context.Canceled).Err())

	updatech, errorch, err := registrar.Register(ctx, name, []regexp.Regexp{*regexp.MustCompile("/command")})
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
