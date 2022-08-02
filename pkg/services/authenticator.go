package services

import "errors"

// Client is an identification of a single client of a service
type Client struct {
	Name string
}

var ErrInvalidToken = errors.New("invalid authentication token provided")

// Authenticator represents a token-based authentication provider
type Authenticator interface {
	Authenticate(token string) (Client, error)
}

type staticAuthenticator struct {
	clients map[string]Client
}

func (p *staticAuthenticator) Authenticate(token string) (Client, error) {
	if c, ok := p.clients[token]; ok {
		return c, nil
	}
	return Client{}, ErrInvalidToken
}

// NewStaticAuthenticator returns an authenticator which authenticates clients
// using a static token->Client map specified at creation time
func NewStaticAuthenticator(clients map[string]Client) Authenticator {
	a := &staticAuthenticator{make(map[string]Client, len(clients))}
	for k, v := range clients {
		a.clients[k] = v
	}
	return a
}
