package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNoMatchers = errors.New("cannot register with zero matchers")

// Registrar allows registration of command handlers for subsequent receival of updates
type Registrar interface {
	// Register registers a new command handler with the given matchers.
	// The context should span the lifetime of the registered handler and canceled when it dies.
	Register(ctx context.Context, matchers MatcherGroup) (UpdateChan, ErrorChan, error)
}

// gRPCRegistrar is an implementation of Registrar using grpc
type gRPCRegistrar struct {
	client genproto.MultiplexerServiceClient
}

func (r *gRPCRegistrar) Register(ctx context.Context, matchers MatcherGroup) (UpdateChan, ErrorChan, error) {
	if len(matchers) < 1 {
		return nil, nil, ErrNoMatchers
	}

	request := &genproto.RegisterRequest{
		Matchers: make([]string, len(matchers)),
	}
	for i, m := range matchers {
		request.Matchers[i] = m.String()
	}

	stream, err := r.client.RegisterHandler(ctx, request)
	if err != nil {
		return nil, nil, wrap(err, "RegisterHandler request failed", "failed to register command handler")
	}

	updatech := make(chan tgbotapi.Update)
	errorch := make(chan error, 1)
	go func() {
		defer close(updatech)
		defer close(errorch)
		// Should only be used once as errorch has capacity of 1
		sendError := func(err error, info string) {
			wrerr := wrap(err, info, "failed to receive updates")
			errorch <- wrerr
		}

		for {
			updatePB, err := stream.Recv()
			if err != nil {
				if s, ok := status.FromError(err); ok && (s.Code() == codes.Canceled || s.Code() == codes.DeadlineExceeded) {
					// If the updates are simply stopped, then no error has happened
					return
				}
				sendError(err, "unexpected error receiving next update")
				return
			}

			var update tgbotapi.Update
			if err := json.Unmarshal(updatePB.GetJson(), &update); err != nil {
				sendError(err, "failed to unmarshal json update")
				return
			}

			select {
			case updatech <- update:
			case <-ctx.Done():
				return
			}
		}
	}()

	return updatech, errorch, nil
}

// NewGRPCRegistrar creates a Registrar based on the gRPC API client
func NewGRPCRegistrar(client genproto.MultiplexerServiceClient) (Registrar, error) {
	if client == nil {
		return nil, errors.New("gRPC client must not be nil")
	}
	return &gRPCRegistrar{client}, nil
}
