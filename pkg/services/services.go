package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"time"

	"github.com/go-redis/redis/v8"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.uber.org/zap"
)

// HandlerRegisterer allows registration of command handlers for subsequent receival of updates
type HandlerRegisterer interface {
	// RegisterHandler registers a new command handler with the given name and matchers.
	// The context should span the lifetime of the registered handler and canceled when it dies.
	RegisterHandler(ctx context.Context, name string, matchers []regexp.Regexp) (tgbotapi.UpdatesChannel, error)
}

// Proxy is a HandlerRegisterer which also handles bot API initialization
type Proxy interface {
	HandlerRegisterer
	InitBotAPI() (*tgbotapi.BotAPI, error)
}

// renewalCoef defines how soon a handler's registration is
// renewed compared to the actual deadline
const renewalCoef = 2

// tgAPIError is an internal error wrapper for errors which
// occurred during registration, updates, etc
type tgAPIError struct {
	wrapped   error
	message   string
	operation string
}

func (e *tgAPIError) Unwrap() error {
	return e.wrapped
}

func (e *tgAPIError) Error() string {
	return e.message
}

func logTgAPIError(logger *zap.Logger, e *tgAPIError) {
	logger.Warn(e.message, zap.String("operation", e.operation), zap.Error(e.wrapped))
}

// redisHTTPProxy implements registration using http and update receival using redis
type redisHTTPProxy struct {
	r        *redis.Client
	h        *http.Client
	l        *zap.Logger
	endpoint *url.URL
}

// internalHTTPTransport is a roundtripper for the http proxy
// with added authorization and error handling
type internalHTTPTransport struct {
	http.RoundTripper
	logger *zap.Logger
	token  string
}

func (t *internalHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", t.token)
	resp, err := t.RoundTripper.RoundTrip(r)
	if err != nil {
		err := &tgAPIError{
			wrapped:   err,
			message:   "roundtripping request to bot proxy failed",
			operation: "roundtrip",
		}
		logTgAPIError(t.logger, err)
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("request to bot proxy failed: unauthorized")
	}
	return resp, nil
}

// RedisHTTPConfig specifies the configuration of the redis-http-based proxy.
// All fields are expected to be set unless specified otherwise.
type RedisHTTPConfig struct {
	Logger *zap.Logger
	Redis  *redis.Client
	// RoundTripper can be nil, in which case http.DefaultTransport will be used
	RoundTripper http.RoundTripper
	// Token is the authorization token for the http API
	Token string
	// Endpoint of the http API
	Endpoint *url.URL
}

// NewRedisHTTPProxy constructs a new redis-http-based proxy
func NewRedisHTTPProxy(config *RedisHTTPConfig) (Proxy, error) {
	if config.Logger == nil || config.Redis == nil {
		return nil, errors.New("unable to create registerer without required components")
	} else if config.Token == "" || config.Endpoint == nil {
		return nil, errors.New("token and endpoint of http API must be set")
	}

	transport := config.RoundTripper
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &redisHTTPProxy{
		r: config.Redis,
		h: &http.Client{
			Transport: &internalHTTPTransport{
				RoundTripper: transport,
				logger:       config.Logger,
				token:        config.Token,
			},
		},
		l:        config.Logger,
		endpoint: config.Endpoint,
	}, nil
}

func urlJoin(base *url.URL, relative ...string) string {
	cp := *base
	cp.RawPath = ""
	cp.Path = path.Join(append([]string{cp.Path}, relative...)...)
	return cp.String()
}

func (p *redisHTTPProxy) updateRegistration(ctx context.Context, request *RegisterHandlerRequest) (*RegisterHandlerResponse, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshaling register request failed: %w", err)
	}
	buffer := bytes.NewBuffer(b)

	// Do registration request
	httpreq, err := http.NewRequestWithContext(ctx, "POST", urlJoin(p.endpoint, "internal", "register"), buffer)
	if err != nil {
		return nil, fmt.Errorf("http request construction failed: %w", err)
	}
	httpreq.Header.Set("Content-Type", "application/json")

	httpresp, err := p.h.Do(httpreq)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}

	// Ensure that registration was successful
	body, err := io.ReadAll(httpresp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response failed: %w", err)
	}

	if httpresp.StatusCode != http.StatusOK && httpresp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("proxy responded with bad status code (%d): %s", httpresp.StatusCode, body)
	}

	var resp RegisterHandlerResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body %s: %w", body, err)
	}
	return &resp, nil
}

func (p *redisHTTPProxy) RegisterHandler(ctx context.Context, name string, matchers []regexp.Regexp) (tgbotapi.UpdatesChannel, error) {
	var apiError *tgAPIError
	defer func() {
		if apiError != nil {
			logTgAPIError(p.l, apiError)
		}
	}()

	// Construct registration to be used for all future registrations
	request := RegisterHandlerRequest{
		Name:     name,
		Matchers: make([]string, len(matchers)),
	}
	for i, m := range matchers {
		request.Matchers[i] = m.String()
	}

	response, err := p.updateRegistration(ctx, &request)
	if err != nil {
		apiError = &tgAPIError{
			wrapped:   err,
			message:   "failed to register handler",
			operation: "registration",
		}
		return nil, apiError
	}
	getRenewalCh := func() <-chan time.Time {
		return time.After(time.Until(response.Deadline) / renewalCoef)
	}

	updates := make(chan tgbotapi.Update)
	go func() {
		defer close(updates)

		// Subscribe to updates
		subscriber := p.r.Subscribe(ctx, response.Channel)

		// Wait until we have to renew or are killed
		renewalCh := getRenewalCh()
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-subscriber.Channel():
				var update tgbotapi.Update
				if err := json.Unmarshal([]byte(message.Payload), &update); err != nil {
					logTgAPIError(p.l, &tgAPIError{
						wrapped:   err,
						message:   fmt.Sprintf("failed to unmarshal update message (%s)", message.Payload),
						operation: "update",
					})
					return
				}

				updates <- update
			case <-renewalCh:
				// renew our registration and setup the renewal channel again
				if response, err = p.updateRegistration(ctx, &request); err != nil {
					logTgAPIError(p.l, &tgAPIError{
						wrapped:   err,
						message:   "failed to renew handler",
						operation: "renewal",
					})
					return
				}
				renewalCh = getRenewalCh()
			}
		}
	}()

	return updates, nil
}

func (p *redisHTTPProxy) InitBotAPI() (*tgbotapi.BotAPI, error) {
	// construct bot api format with meaningless token
	bot, err := tgbotapi.NewBotAPIWithClient("fake-token", urlJoin(p.endpoint, "proxy%s", "%s"), p.h)
	if err != nil {
		err := &tgAPIError{
			wrapped:   err,
			message:   "failed to initialize bot API",
			operation: "init",
		}
		logTgAPIError(p.l, err)
		return nil, err
	}
	return bot, nil
}
