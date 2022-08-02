package services

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/internal/mocks"
	"github.com/bbralion/CTFloodBot/internal/models"
	"github.com/go-logr/logr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Test_gRPCRegistrar_Register(t *testing.T) {
	type args struct {
		matchers models.MatcherGroup
	}
	type streamUpdate struct {
		update *genproto.Update
		err    error
	}
	type registerResponse struct {
		stream []streamUpdate
		err    error
	}
	type possibleUpdate struct {
		update tgbotapi.Update
		err    bool
	}

	tests := []struct {
		name              string
		args              args
		registerResponses []registerResponse
		want              []possibleUpdate
		wantErr           bool
	}{
		{
			name:    "registration with nil matchers",
			args:    args{matchers: nil},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "registration with no matchers",
			args:    args{matchers: models.MatcherGroup{}},
			want:    nil,
			wantErr: true,
		},
		{
			name:              "unrecoverable registration error",
			args:              args{matchers: models.MatcherGroup{regexp.MustCompile("/command")}},
			registerResponses: []registerResponse{{err: status.Error(codes.Unauthenticated, "unauthenticated")}},
			want:              []possibleUpdate{{err: true}},
		},
		{
			name: "retries during registration",
			args: args{matchers: models.MatcherGroup{regexp.MustCompile("^/command"), regexp.MustCompile("^.*$")}},
			registerResponses: []registerResponse{
				{err: status.Error(codes.Unavailable, "temporarily unavailable")},
				{err: status.Error(codes.Unavailable, "starting")},
				{err: nil, stream: []streamUpdate{
					{update: &genproto.Update{Json: `{"update_id":1}`}},
					{update: &genproto.Update{Json: `{"update_id":2}`}},
					{err: status.FromContextError(context.Canceled).Err()},
				}},
			},
			want: []possibleUpdate{
				{update: tgbotapi.Update{UpdateID: 1}},
				{update: tgbotapi.Update{UpdateID: 2}},
			},
		},
		{
			name: "invalid json in update",
			args: args{matchers: models.MatcherGroup{regexp.MustCompile("^/command"), regexp.MustCompile("^.*$")}},
			registerResponses: []registerResponse{
				{err: nil, stream: []streamUpdate{
					{update: &genproto.Update{Json: `{bad}`}},
				}},
			},
			want: []possibleUpdate{
				{err: true},
			},
		},
		{
			name: "reconnect after stream fail",
			args: args{matchers: models.MatcherGroup{regexp.MustCompile("^/aboba$")}},
			registerResponses: []registerResponse{
				{err: nil, stream: []streamUpdate{
					{update: &genproto.Update{Json: `{"update_id":1}`}},
					{err: status.Error(codes.Unavailable, "stream broken")},
				}},
				{err: nil, stream: []streamUpdate{
					{update: &genproto.Update{Json: `{"update_id":2}`}},
					{err: status.FromContextError(context.DeadlineExceeded).Err()},
				}},
			},
			want: []possibleUpdate{
				{update: tgbotapi.Update{UpdateID: 1}},
				{update: tgbotapi.Update{UpdateID: 2}},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl, req := gomock.NewController(t), require.New(t)
			defer ctrl.Finish()

			mockMuxClient := mocks.NewMockMultiplexerServiceClient(ctrl)
			r := NewGRPCRegistrar(logr.Logger{}, mockMuxClient)

			reqMatchers := make([]string, len(tt.args.matchers))
			for i, m := range tt.args.matchers {
				reqMatchers[i] = m.String()
			}

			ctx := context.Background()
			for i := range tt.registerResponses {
				resp := tt.registerResponses[i]
				stream := mocks.NewMockMultiplexerService_RegisterHandlerClient(ctrl)
				for _, u := range resp.stream {
					stream.EXPECT().Recv().Return(u.update, u.err)
				}
				mockMuxClient.EXPECT().RegisterHandler(ctx, &genproto.RegisterRequest{Matchers: reqMatchers}).Return(stream, resp.err)
			}

			updateCh, err := r.Register(ctx, tt.args.matchers)
			req.Equal(tt.wantErr, err != nil)

			left := len(tt.want)
			req.Eventually(func() bool {
				select {
				case update, ok := <-updateCh:
					if !ok {
						req.Zero(left, "less updates on channel than wanted")
						return true
					}

					req.NotZero(left, "more updates on channel than wanted")
					want := &tt.want[len(tt.want)-left]
					req.Equal(want.err, update.Error != nil)
					req.Equal(want.update, update.Update)
					left--
				default:
				}
				return updateCh == nil
			}, time.Second*5, time.Millisecond*50)
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
