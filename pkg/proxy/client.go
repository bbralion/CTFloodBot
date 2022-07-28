package proxy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/internal/services"
	"github.com/bbralion/CTFloodBot/pkg/auth"
	"github.com/bbralion/CTFloodBot/pkg/models"
	"github.com/bbralion/CTFloodBot/pkg/retry"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultTimeout = time.Second * 5

// Context provides Handler's with often needed elements
type Context interface {
	context.Context
	API() *tgbotapi.BotAPI
	Logger() *zap.Logger
}

type proxyCtx struct {
	context.Context
	api    *tgbotapi.BotAPI
	logger *zap.Logger
}

func (ctx *proxyCtx) API() *tgbotapi.BotAPI {
	return ctx.api
}

func (ctx *proxyCtx) Logger() *zap.Logger {
	return ctx.logger
}

// Handler is a proper handler of update requests received from the proxy
type Handler interface {
	Serve(ctx Context, update tgbotapi.Update)
}

// HandlerFunc is a util type for using functions as Handler's
type HandlerFunc func(ctx Context, update tgbotapi.Update)

func (f HandlerFunc) Serve(ctx Context, update tgbotapi.Update) {
	f(ctx, update)
}

// Client is the proxy client implementation, it receives updates via gRPC and answers via HTTP
type Client struct {
	Logger *zap.Logger
	// Handler is the telegram update handler used
	Handler Handler
	// Matchers specify the matchers used to filter the requests which should be handled by this client
	Matchers models.MatcherGroup
	// Token is the auth token used to authorize this client
	Token string
	// GRPCEndpoint specifies the (currently insecure) gRPC endpoint to connect to
	GRPCEndpoint string
}

// Run runs the client. It starts by connecting to the gRPC proxy, from which it receives the HTTP
// proxy endpoint, as well as all of the following updates from telegram.
func (c *Client) Run(ctx context.Context) (err error) {
	if c.Logger == nil || c.Handler == nil || len(c.Matchers) < 1 {
		return errors.New("logger, handler and matchers must be specified for client")
	}
	c.Logger = c.Logger.Named("client")
	logErr := func(msg string) {
		c.Logger.Error(msg, zap.Error(err))
	}

	// Begin by setting up a proper grpc connection
	conn, err := c.connectGRPC()
	if err != nil {
		logErr("failed to connect to specified gRPC endpoint")
		return
	}
	defer conn.Close()
	gc := genproto.NewMultiplexerServiceClient(conn)

	// And immediately test it out by requesting the HTTP endpoint
	cfg, err := c.getConfig(ctx, gc)
	if err != nil {
		logErr("failed to get config")
		return
	}

	// Connect to the telegram bot proxy
	api, err := c.connectHTTP(cfg.GetProxyEndpoint())
	if err != nil {
		logErr("failed to connect to telegram http proxy")
		return
	}

	// Configure registrar and start listening for updates
	registrar, err := services.NewGRPCRegistrar(c.Logger, gc)
	if err != nil {
		logErr("failed to setup registrar")
		return
	}

	updatech, errorch, err := registrar.Register(ctx, c.Matchers)
	if err != nil {
		logErr("failed to register client")
		return
	}

	// Make sure to cancel all requests when we die
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case err = <-errorch:
			logErr("critical error while receiving updates")
			return
		case update := <-updatech:
			c.Handler.Serve(&proxyCtx{ctx, api, c.Logger}, update)
		case <-ctx.Done():
			c.Logger.Info("shutting down")
			return
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
	config, err := retry.Backoff(func() (error, *genproto.Config) {
		ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
		defer cancel()

		resp, err := gc.GetConfig(ctx, &genproto.ConfigRequest{})
		if err == nil {
			return nil, resp.GetConfig()
		} else if retry.IsGRPCUnavailable(err) {
			c.Logger.Info("retrying connection to gRPC server", zap.Error(err))
			return retry.Recoverable(err), nil
		}
		return retry.Unrecoverable(err), nil
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
