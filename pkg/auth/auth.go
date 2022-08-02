package auth

import (
	"context"

	"github.com/bbralion/CTFloodBot/pkg/services"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const tokenKey = "authorization"

type GRPCClientInterceptor string

// NewGRPCClientInterceptor creates a new gRPC client interceptor which uses the given token
func NewGRPCClientInterceptor(token string) GRPCClientInterceptor {
	return GRPCClientInterceptor(token)
}

func (t GRPCClientInterceptor) attach(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, tokenKey, string(t))
}

// Unary returns a unary gRPC client interceptor with authentication using the setup token
func (t GRPCClientInterceptor) Unary() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		return invoker(t.attach(ctx), method, req, reply, cc, opts...)
	}
}

// Stream returns a stream gRPC client interceptor with authentication using the setup token
func (t GRPCClientInterceptor) Stream() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(t.attach(ctx), desc, cc, method, opts...)
	}
}

// GRPCServerInterceptor is a Unary and Stream interceptor provider which
// uses an underlying AuthProvider for authentication of clients
type GRPCServerInterceptor struct {
	logger   logr.Logger
	provider services.Authenticator
}

// NewGRPCServerInterceptor returns a new gRPC server interceptor
// which authenticates clients using the specified provider.
func NewGRPCServerInterceptor(logger logr.Logger, provider services.Authenticator) *GRPCServerInterceptor {
	return &GRPCServerInterceptor{logger, provider}
}

func (i *GRPCServerInterceptor) authorize(ctx context.Context, method string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md[tokenKey]) != 1 {
		return status.Error(codes.Unauthenticated, "must contain metadata with single auth token")
	}

	client, err := i.provider.Authenticate(md[tokenKey][0])
	if err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}

	i.logger.Info("gRPC request from authenticated client", "client", client, "method", method)
	return nil
}

// Unary returns a unary gRPC server interceptor for authentication
func (i *GRPCServerInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if err := i.authorize(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream returns a stream gRPC server interceptor for authentication
func (i *GRPCServerInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if err := i.authorize(stream.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, stream)
	}
}
