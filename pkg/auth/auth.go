package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bbralion/CTFloodBot/pkg/services"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	tokenKey  = "authorization"
	clientKey = "authenticated_client"
)

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
	logger   *zap.Logger
	provider services.AuthProvider
}

// NewGRPCServerInterceptor returns a new gRPC server interceptor
// which authenticates clients using the specified provider.
func NewGRPCServerInterceptor(logger *zap.Logger, provider services.AuthProvider) *GRPCServerInterceptor {
	return &GRPCServerInterceptor{logger, provider}
}

func (i *GRPCServerInterceptor) authorize(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md[tokenKey]) != 1 {
		return nil, status.Error(codes.Unauthenticated, "must contain metadata with single auth token")
	}
	if len(md[clientKey]) != 0 {
		return nil, status.Error(codes.Unauthenticated, "illegal metadata contained in request")
	}

	client, err := i.provider.Authenticate(md[tokenKey][0])
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	data, err := json.Marshal(client)
	if err != nil {
		i.logger.Error("failed to marshal client struct for storing in gRPC metadata", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error while authenticating client")
	}
	return metadata.NewIncomingContext(ctx, metadata.Join(md, metadata.Pairs(clientKey, string(data)))), nil
}

// Unary returns a unary gRPC server interceptor for authentication
func (i *GRPCServerInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		ctx, err := i.authorize(ctx)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

type authenticatedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s authenticatedStream) Context() context.Context        { return s.ctx }
func (s authenticatedStream) RecvMsg(msg interface{}) error   { return s.ServerStream.RecvMsg(msg) }   //nolint:wrapcheck
func (s authenticatedStream) SendMsg(msg interface{}) error   { return s.ServerStream.SendMsg(msg) }   //nolint:wrapcheck
func (s authenticatedStream) SendHeader(md metadata.MD) error { return s.ServerStream.SendHeader(md) } //nolint:wrapcheck
func (s authenticatedStream) SetHeader(md metadata.MD) error  { return s.ServerStream.SetHeader(md) }  //nolint:wrapcheck
func (s authenticatedStream) SetTrailer(md metadata.MD)       { s.ServerStream.SetTrailer(md) }

// Stream returns a stream gRPC server interceptor for authentication
func (i *GRPCServerInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, err := i.authorize(stream.Context())
		if err != nil {
			return err
		}
		return handler(srv, authenticatedStream{stream, ctx})
	}
}

// ClientFromCtx returns the client from an authenticated context
func ClientFromCtx(ctx context.Context) (*services.Client, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md[clientKey]) != 1 {
		return nil, errors.New("ClientFromCtx must only be called with an authenticated context")
	}

	var client services.Client
	if err := json.Unmarshal([]byte(md[clientKey][0]), &client); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stored client: %w", err)
	}
	return &client, nil
}
