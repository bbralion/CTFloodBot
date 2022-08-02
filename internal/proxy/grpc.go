package proxy

import (
	"context"
	"errors"

	"github.com/bbralion/CTFloodBot/internal/genproto"
	"github.com/bbralion/CTFloodBot/pkg/auth"
	"github.com/bbralion/CTFloodBot/pkg/services"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

type GRPC struct {
	genproto.UnimplementedMultiplexerServiceServer
	AdvertisedHTTPEndpoint string
	Addr                   string
	Logger                 logr.Logger
	AuthProvider           services.Authenticator
}

func (p *GRPC) ListenAndServe() error {
	if p.AuthProvider == nil || p.AdvertisedHTTPEndpoint == "" {
		return errors.New("logger, auth provider and the advertised http endpoint must be set")
	}

	return nil
}

func (p *GRPC) GetConfig(context.Context, *genproto.ConfigRequest) (*genproto.ConfigResponse, error) {
	return &genproto.ConfigResponse{
		Config: &genproto.Config{
			ProxyEndpoint: p.AdvertisedHTTPEndpoint,
		},
	}, nil
}

func (p *GRPC) RegisterHandler(*genproto.RegisterRequest, genproto.MultiplexerService_RegisterHandlerServer) error {
	return nil
}

func (p *GRPC) setupGRPC() *grpc.Server {
	interceptor := auth.NewGRPCServerInterceptor(p.Logger, p.AuthProvider)
	server := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.Unary()),
		grpc.StreamInterceptor(interceptor.Stream()),
	)

	genproto.RegisterMultiplexerServiceServer(server, p)
	return server
}
