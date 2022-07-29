package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"sync/atomic"
	"time"

	internal "github.com/bbralion/CTFloodBot/internal/services"
	"github.com/bbralion/CTFloodBot/pkg/services"
	"github.com/go-logr/logr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/justinas/alice"
)

// DefaultRequestTimeout is the default timeout to be used for making requests to the telegram API
const DefaultRequestTimeout = time.Second * 30

// 16 megabytes should be enough for most usecases
const DefaultMaxBodyBytes = 16_000_000

// HTTP is the telegram API HTTP proxy
type HTTP struct {
	http.Server
	Logger logr.Logger
	// If set will be used to authenticate clients via a token in the Authorization header
	AuthProvider services.Authenticator
	// If set only paths in the allowlist will be allowed
	Allowlist internal.Allowlist
	// Transport is the transport to use for making requests to the telegram API.
	// http.DefaultTransport will be used by default
	Transport *http.Transport
	// Telegram API token
	Token string
	// Telegram API endpoint to use, may be another proxy
	Endpoint       string
	requestCounter int64
}

var pathRe = regexp.MustCompile(`^/proxy(\w+)(/.+)$`)

// Path returns the HTTP proxy path format string suitable for use with tgbotapi
func (p *HTTP) Path() string {
	return "/proxy%s/%s"
}

func (p *HTTP) ListenAndServe() error {
	endpointURL, err := url.Parse(p.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint url specified: %w", err)
	}

	p.Logger = p.Logger.WithName("http")
	p.setDefaults()

	// TODO: implement proper handling of special commands such as setMyCommands
	handler := httputil.ReverseProxy{
		Director: func(r *http.Request) {
			// Route requests using the telegram API token and with a limited body
			reqURL := *endpointURL
			reqURL.User = nil
			reqURL.Path = fmt.Sprintf(p.Endpoint, p.Token, r.URL.Path[1:])
			r.URL = &reqURL
			r.Body = http.MaxBytesReader(nil, r.Body, DefaultMaxBodyBytes)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			p.Logger.Error(err, "request to telegram API failed", "request_id", requestID(r))
			w.WriteHeader(http.StatusBadGateway)
		},
		Transport: p.Transport,
	}

	p.Handler = alice.New(p.PanicMiddleware, p.RequestIDMiddleware, p.LoggingMiddleware, p.AuthMiddleware, p.AllowPathMiddleware).Then(&handler)
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
				p.Logger.Info("recovered from panic", "recover", v, "request_id", requestID(r))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type clientCtxKey struct{}

func (p *HTTP) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		groups := pathRe.FindStringSubmatch(r.URL.Path)
		if groups == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if p.AuthProvider != nil {
			client, err := p.AuthProvider.Authenticate(groups[1])
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			authenticatedReq := r.WithContext(context.WithValue(r.Context(), clientCtxKey{}, client))
			authenticatedReq.URL.Path = groups[2]
			next.ServeHTTP(w, authenticatedReq)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

func (p *HTTP) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rcopy := r.Clone(context.Background())

		// Always log, even on panic
		defer func() {
			latency := time.Since(start)
			p.Logger.Info("handled request",
				"uri", rcopy.RequestURI,
				"method", rcopy.Method,
				"latency", latency,
				"remote_addr", rcopy.RemoteAddr,
				"request_id", requestID(r),
				"client", r.Context().Value(clientCtxKey{}))
		}()

		next.ServeHTTP(w, r)
	})
}

func (p *HTTP) AllowPathMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.Allowlist != nil && !p.Allowlist.Allowed(r.URL.Path) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
