package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"sync/atomic"
	"time"

	internal "github.com/bbralion/CTFloodBot/internal/services"
	"github.com/bbralion/CTFloodBot/pkg/services"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/justinas/alice"
	"go.uber.org/zap"
)

// DefaultRequestTimeout is the default timeout to be used for making requests to the telegram API
const DefaultRequestTimeout = time.Second * 30

// 16 megabytes should be enough for most usecases
const DefaultMaxBodyBytes = 16_000_000

// HTTP is the telegram API HTTP proxy
type HTTP struct {
	http.Server
	Logger       *zap.Logger
	AuthProvider services.AuthProvider
	Allowlist    internal.Allowlist
	// Transport is the transport to use for making requests to the telegram API.
	// http.DefaultTransport will be used by default
	Transport *http.Transport
	// Telegram API token
	Token string
	// Telegram API endpoint to use, may be another proxy
	Endpoint       string
	requestCounter int64
}

var pathRe = regexp.MustCompile("^/proxy(\\w+)(/.+)$")

// Path returns the HTTP proxy path format string suitable for use with tgbotapi
func (p *HTTP) Path() string {
	return "/proxy%s/%s"
}

func (p *HTTP) ListenAndServe() error {
	if p.Logger == nil || p.AuthProvider == nil || p.Allowlist == nil {
		return errors.New(
			"logger, auth provider and allow list must be specified for the http proxy server")
	} else if p.Token == "" {
		return errors.New("telegram API token must be specified")
	}

	endpointUrl, err := url.Parse(p.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint url specified: %w", err)
	}

	p.Logger = p.Logger.Named("http")
	p.setDefaults()

	handler := httputil.ReverseProxy{
		Director: func(r *http.Request) {
			// Route requests using the telegram API token and with a limited body
			reqUrl := *endpointUrl
			reqUrl.User = nil
			reqUrl.Path = fmt.Sprintf(p.Endpoint, p.Token, r.URL.Path[1:])
			r.URL = &reqUrl
			r.Body = http.MaxBytesReader(nil, r.Body, DefaultMaxBodyBytes)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			p.Logger.Warn("request to telegram API failed", zap.Error(err), zap.Int64("request_id", requestID(r)))
			w.WriteHeader(http.StatusBadGateway)
			return
		},
		Transport: p.Transport,
	}

	p.Handler = alice.New(p.PanicMiddleware, p.RequestIDMiddleware, p.LoggingMiddleware, p.AuthMiddleware).Then(&handler)
	return p.ListenAndServe()
}

func (p *HTTP) setDefaults() {
	if p.Transport == nil {
		p.Transport = &http.Transport{}
	}
	if p.Transport.ResponseHeaderTimeout == 0 {
		p.Transport.ResponseHeaderTimeout = DefaultRequestTimeout
	}
	if p.Endpoint == "" {
		p.Endpoint = tgbotapi.APIEndpoint
	}
}

type requestIDCtxKey struct{}

func requestID(r *http.Request) int64 {
	return r.Context().Value(requestIDCtxKey{}).(int64)
}

func (p *HTTP) RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt64(&p.requestCounter, 1)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDCtxKey{}, id)))
	})
}

func (p *HTTP) PanicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				p.Logger.Error("recovered from panic",
					zap.Any("recover", v),
					zap.Int64("request_id", requestID(r)))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type clientCtxKey struct{}

func (p *HTTP) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		groups := pathRe.FindStringSubmatch(r.URL.Path)
		if len(groups) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		client, err := p.AuthProvider.Authenticate(groups[1])
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		authenticatedReq := r.WithContext(context.WithValue(r.Context(), clientCtxKey{}, client))
		authenticatedReq.URL.Path = groups[2]
		next.ServeHTTP(w, authenticatedReq)
	})
}

func (p *HTTP) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rcopy := *r

		// Always log, even on panic
		defer func() {
			latency := time.Since(start)
			p.Logger.Info("handled request",
				zap.String("uri", rcopy.RequestURI),
				zap.String("method", rcopy.Method),
				zap.Duration("latency", latency),
				zap.String("remote_addr", rcopy.RemoteAddr),
				zap.Int64("request_id", requestID(r)),
				zap.Any("client", r.Context().Value(clientCtxKey{})))
		}()

		next.ServeHTTP(w, r)
	})
}
