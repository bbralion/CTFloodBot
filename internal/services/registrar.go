package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/pkg/models"
	"github.com/bbralion/CTFloodBot/pkg/retry"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.uber.org/zap"
)

var ErrNoMatchers = errors.New("cannot register with zero matchers")

// Registrar allows registration of command handlers for subsequent receival of updates
type Registrar interface {
	// Register registers a new command handler with the given matchers.
	// The context should span the lifetime of the registered handler and canceled when it dies.
	Register(ctx context.Context, matchers models.MatcherGroup) (models.UpdateChan, models.ErrorChan, error)
}

// gRPCRegistrar is an implementation of Registrar using grpc with retries
type gRPCRegistrar struct {
	logger *zap.Logger
	client genproto.MultiplexerServiceClient
}

func (r *gRPCRegistrar) tryRegister(ctx context.Context, request *genproto.RegisterRequest, updatech chan tgbotapi.Update) *svcError {
	stream, err := retry.Backoff(func() (error, genproto.MultiplexerService_RegisterHandlerClient) {
		stream, err := r.client.RegisterHandler(ctx, request)
		if retry.IsGRPCUnavailable(err) {
			r.logger.Info("gRPC registrar retrying connection to server", zap.Error(err))
			return retry.Recoverable(err), nil
		}
		return retry.Unrecoverable(err), stream
	})
	if err != nil {
		if retry.IsGRPCCanceled(err) {
			// Client disconnected before we managed to register
			return nil
		}
		return wrap(err, "RegisterHandler request failed", "failed to register command handler")
	}

	wrapUpdateErr := func(err error, info string) *svcError {
		return wrap(err, info, "failed to receive updates")
	}

	for {
		updatePB, err := stream.Recv()
		if err != nil {
			if retry.IsGRPCCanceled(err) {
				// If the updates are simply stopped, then no error has happened
				return nil
			}
			return wrapUpdateErr(err, "unexpected error receiving next update")
		}

		var update tgbotapi.Update
		if err := json.Unmarshal(updatePB.GetJson(), &update); err != nil {
			return wrapUpdateErr(err, "failed to unmarshal json update")
		}

		select {
		case updatech <- update:
		case <-ctx.Done():
			return nil
		}
	}
}

func (r *gRPCRegistrar) Register(ctx context.Context, matchers models.MatcherGroup) (models.UpdateChan, models.ErrorChan, error) {
	if len(matchers) < 1 {
		return nil, nil, ErrNoMatchers
	}

	request := &genproto.RegisterRequest{
		Matchers: make([]string, len(matchers)),
	}
	for i, m := range matchers {
		request.Matchers[i] = m.String()
	}

	updatech := make(chan tgbotapi.Update)
	errorch := make(chan error, 1)
	go func() {
		defer close(updatech)
		defer close(errorch)

		_, err := retry.Static(func() (error, any) {
			err := r.tryRegister(ctx, request, updatech)
			if err == nil {
				return nil, nil
			}

			// We can retry only due to connectivity issues
			if retry.IsGRPCUnavailable(err.Unwrap()) {
				r.logger.Info("gRPC registrar reconnecting stream", err.ZapFields()...)
				return retry.Recoverable(err), nil
			}
			return retry.Unrecoverable(err), nil
		})
		if err != nil {
			// Recovery failed, log and return the error
			var serr *svcError
			errors.As(err, &serr)
			r.logger.Error("gRPC registrar unable to reconnect", serr.ZapFields()...)
			errorch <- serr
		}
	}()
	return updatech, errorch, nil
}

// NewGRPCRegistrar creates a Registrar based on the gRPC API client with preconfigured retries
func NewGRPCRegistrar(logger *zap.Logger, client genproto.MultiplexerServiceClient) (Registrar, error) {
	if client == nil {
		return nil, errors.New("gRPC client must not be nil")
	}
	return &gRPCRegistrar{logger.With(zap.Namespace("registrar")), client}, nil
}
