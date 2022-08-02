package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/internal/models"
	"github.com/bbralion/CTFloodBot/pkg/retry"
	"github.com/go-logr/logr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// Registrar allows registration of command handlers for subsequent receival of updates
type Registrar interface {
	// Register registers a new command handler with the given matchers.
	// The context should span the lifetime of the registered handler and canceled when it dies.
	Register(ctx context.Context, matchers models.MatcherGroup) (models.UpdateChan, error)
}

// gRPCRegistrar is an implementation of Registrar using grpc with retries
type gRPCRegistrar struct {
	logger logr.Logger
	client genproto.MultiplexerServiceClient
}

func (r *gRPCRegistrar) tryRegister(ctx context.Context, request *genproto.RegisterRequest, updateCh chan models.PossibleUpdate) error {
	stream, err := retry.Backoff(func() (genproto.MultiplexerService_RegisterHandlerClient, error) {
		stream, err := r.client.RegisterHandler(ctx, request)
		if err == nil {
			return stream, nil
		}
		if retry.IsGRPCUnavailable(err) {
			r.logger.Error(err, "gRPC registrar retrying connection to server")
			return nil, retry.Recoverable()
		}
		return nil, retry.Unrecoverable(err)
	})
	if err != nil {
		return fmt.Errorf("registering handler: %w", err)
	}

	for {
		updatePB, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("receiving update: %w", err)
		}

		var update tgbotapi.Update
		if err := json.Unmarshal([]byte(updatePB.Json), &update); err != nil {
			return fmt.Errorf("unmarshaling update json: %w", err)
		}

		select {
		case updateCh <- models.PossibleUpdate{Update: update}:
		case <-ctx.Done():
			return nil
		}
	}
}

func (r *gRPCRegistrar) Register(ctx context.Context, matchers models.MatcherGroup) (models.UpdateChan, error) {
	if len(matchers) < 1 {
		return nil, errors.New("cannot register with zero matchers")
	}

	request := &genproto.RegisterRequest{
		Matchers: make([]string, len(matchers)),
	}
	for i, m := range matchers {
		request.Matchers[i] = m.String()
	}

	updateCh := make(chan models.PossibleUpdate)
	go func() {
		defer close(updateCh)

		_, err := retry.Static(func() (any, error) {
			err := r.tryRegister(ctx, request, updateCh)
			if uw := errors.Unwrap(err); uw == nil || retry.IsGRPCCanceled(uw) {
				return nil, nil
			} else if retry.IsGRPCUnavailable(uw) {
				r.logger.Error(err, "gRPC registrar reconnecting stream")
				return nil, retry.Recoverable()
			}
			return nil, retry.Unrecoverable(err)
		})
		if err != nil {
			updateCh <- models.PossibleUpdate{Error: err}
		}
	}()
	return updateCh, nil
}

// NewGRPCRegistrar creates a Registrar based on the gRPC API client with preconfigured retries
func NewGRPCRegistrar(logger logr.Logger, client genproto.MultiplexerServiceClient) Registrar {
	if logger == (logr.Logger{}) {
		logger = logr.Discard()
	}
	return &gRPCRegistrar{logger.WithName("registrar"), client}
}
