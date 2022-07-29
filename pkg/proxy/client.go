package proxy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/internal/models"
	"github.com/bbralion/CTFloodBot/internal/services"
	"github.com/bbralion/CTFloodBot/pkg/auth"
	"github.com/bbralion/CTFloodBot/pkg/retry"
	"github.com/go-logr/logr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultTimeout = time.Second * 5

// Handler is a proper handler of update requests received from the proxy
type Handler interface {
	Serve(api *tgbotapi.BotAPI, update tgbotapi.Update)
}

// HandlerFunc is a util type for using functions as Handler's
type HandlerFunc func(api *tgbotapi.BotAPI, update tgbotapi.Update)

func (f HandlerFunc) Serve(api *tgbotapi.BotAPI, update tgbotapi.Update) {
	f(api, update)
}

// Client is the proxy client implementation, it receives updates via gRPC and answers via HTTP
type Client struct {
	Logger logr.Logger
	// Handler is the telegram update handler used
	Handler Handler
	// Matchers specify the matchers used to filter the requests which should be handled by this client
	Matchers []string
	// Token is the auth token used to authorize this client
	Token string
	// GRPCEndpoint specifies the (currently insecure) gRPC endpoint to connect to
	GRPCEndpoint string
}

// Run runs the client. It starts by connecting to the gRPC proxy, from which it receives the HTTP
// proxy endpoint, as well as all of the following updates from telegram.
func (c *Client) Run(ctx context.Context) error {
	if c.Handler == nil || len(c.Matchers) < 1 {
		return errors.New("logger, handler and matchers must be specified for client")
	}
	c.Logger = c.Logger.WithName("client")

	matchers := make(models.MatcherGroup, len(c.Matchers))
	for i, m := range c.Matchers {
		var err error
		if matchers[i], err = regexp.Compile(m); err != nil {
			return fmt.Errorf("invalid matcher specified: %w", err)
		}
	}

	// Begin by setting up a proper grpc connection
	conn, err := c.connectGRPC()
	if err != nil {
		return fmt.Errorf("connecting to gRPC server: %w", err)
	}
	defer conn.Close()
	gc := genproto.NewMultiplexerServiceClient(conn)

	// And immediately test it out by requesting the HTTP endpoint
	cfg, err := c.getConfig(ctx, gc)
	if err != nil {
		return fmt.Errorf("getting config via gRPC: %w", err)
	}

	// Connect to the telegram bot proxy
	api, err := c.connectHTTP(cfg.GetProxyEndpoint())
	if err != nil {
		return fmt.Errorf("connecting to telegram HTTP proxy: %w", err)
	}

	// Configure registrar and start listening for updates
	registrar := services.NewGRPCRegistrar(c.Logger, gc)
	updateCh, err := registrar.Register(ctx, matchers)
	if err != nil {
		return fmt.Errorf("registering client: %w", err)
	}

	// Make sure to cancel all requests when we die
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case update := <-updateCh:
			if update.Error != nil {
				return fmt.Errorf("receiving updates: %w", err)
			}
			c.Handler.Serve(api, update.Update)
		case <-ctx.Done():
			c.Logger.Info("shutting down")
			return nil
		}
	}
}

func (c *Client) connectGRPC() (*grpc.ClientConn, error) {
	if c.GRPCEndpoint == "" || c.Token == "" {
		return nil, errors.New("endpoint and token must not be empty")
	}

	interceptor := auth.NewGRPCClientInterceptor(c.Token)
	conn, err := grpc.Dial(c.GRPCEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(interceptor.Unary()),
		grpc.WithStreamInterceptor(interceptor.Stream()))
	if err != nil {
		return nil, fmt.Errorf("failed to dial proxy gRPC endpoint: %w", err)
	}
	return conn, nil
}

func (c *Client) getConfig(ctx context.Context, gc genproto.MultiplexerServiceClient) (*genproto.Config, error) {
	config, err := retry.Backoff(func() (*genproto.Config, error) {
		ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()

		resp, err := gc.GetConfig(ctx, &genproto.ConfigRequest{})
		if err == nil {
			return resp.GetConfig(), nil
		} else if retry.IsGRPCUnavailable(err) {
			c.Logger.Error(err, "retrying connection to gRPC server")
			return nil, retry.Recoverable()
		}
		return nil, retry.Unrecoverable(err)
	})
	if err != nil {
		return nil, fmt.Errorf("request to gRPC server failed: %w", err)
	}
	return config, nil
}

func (c *Client) connectHTTP(endpoint string) (*tgbotapi.BotAPI, error) {
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint(c.Token, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to setup telegram bot api: %w", err)
	}
	return api, nil
}
