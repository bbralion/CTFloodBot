package proxy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/bbralion/CTFloodBot/internal/models"
	"github.com/bbralion/CTFloodBot/pkg/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const defaultTimeout = time.Second * 5

// Client is the proxy client implementation
type Client struct {
	// Handler is the telegram update handler to use
	Handler Handler
	// Matchers specify the matchers used to filter the requests which should be handled by this client
	Matchers []string
}

// RegisterAndRun registers the client using the given registrar,
// then handles the received updates by responding to them using the api
func (c *Client) RegisterAndRun(ctx context.Context, registrar services.Registrar, api *tgbotapi.BotAPI) error {
	if c.Handler == nil || len(c.Matchers) < 1 {
		return errors.New("handler and matchers must be specified for client")
	}

	matchers := make(models.MatcherGroup, len(c.Matchers))
	for i, m := range c.Matchers {
		var err error
		if matchers[i], err = regexp.Compile(m); err != nil {
			return fmt.Errorf("invalid matcher specified: %w", err)
		}
	}

	updateCh, err := registrar.Register(ctx, matchers)
	if err != nil {
		return fmt.Errorf("registering client: %w", err)
	}

	for {
		select {
		case update := <-updateCh:
			if update.Error != nil {
				return fmt.Errorf("receiving updates: %w", err)
			}
			c.Handler.Serve(api, update.Update)
		case <-ctx.Done():
			return nil
		}
	}
}

// func (c *Client) connectGRPC() (*grpc.ClientConn, error) {
// 	if c.GRPCEndpoint == "" || c.Token == "" {
// 		return nil, errors.New("endpoint and token must not be empty")
// 	}

// 	interceptor := auth.NewGRPCClientInterceptor(c.Token)
// 	conn, err := grpc.Dial(c.GRPCEndpoint,
// 		grpc.WithTransportCredentials(insecure.NewCredentials()),
// 		grpc.WithUnaryInterceptor(interceptor.Unary()),
// 		grpc.WithStreamInterceptor(interceptor.Stream()))
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to dial proxy gRPC endpoint: %w", err)
// 	}
// 	return conn, nil
// }

// func (c *Client) getConfig(ctx context.Context, gc genproto.MultiplexerServiceClient) (*genproto.Config, error) {
// 	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
// 	defer cancel()

// 	config, err := gc.GetConfig(ctx, &genproto.ConfigRequest{})
// 	if err != nil {
// 		return nil, fmt.Errorf("requesting config from gRPC server: %w", err)
// 	}
// 	return config.Config, nil
// }

// func (c *Client) connectHTTP(endpoint string) (*tgbotapi.BotAPI, error) {
// 	api, err := tgbotapi.NewBotAPIWithClient(c.Token, endpoint, &http.Client{
// 		Transport: &http.Transport{
// 			ResponseHeaderTimeout: defaultTimeout,
// 		},
// 	})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to setup telegram bot api: %w", err)
// 	}
// 	return api, nil
// }
