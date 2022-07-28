package services

import "errors"

// Client is an identification of a single client of a service
type Client struct {
	Name string
}

var ErrInvalidToken = errors.New("invalid authentication token provided")

// AuthProvider represents a token-based authentication provider
type AuthProvider interface {
	Authenticate(token string) (*Client, error)
}

type staticAuthProvider struct {
	clients map[string]*Client
}

func (p *staticAuthProvider) Authenticate(token string) (*Client, error) {
	if c, ok := p.clients[token]; ok {
		return c, nil
	}
	return nil, ErrInvalidToken
}

// NewStaticAuthProvider returns an auth provider which authenticates clients
// using a static token->Client map specified at creation time
func NewStaticAuthProvider(clients map[string]*Client) AuthProvider {
	return &staticAuthProvider{clients}
}
