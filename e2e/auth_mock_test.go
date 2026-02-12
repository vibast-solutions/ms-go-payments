//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	authpb "github.com/vibast-solutions/ms-go-auth/app/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultPaymentsCallerAPIKey   = "payments-caller-key"
	defaultPaymentsNoAccessAPIKey = "payments-no-access-key"
	defaultPaymentsAppAPIKey      = "payments-app-api-key"
	paymentsAuthMockAddr          = "0.0.0.0:38084"
)

func paymentsCallerAPIKey() string {
	if value := strings.TrimSpace(os.Getenv("PAYMENTS_CALLER_API_KEY")); value != "" {
		return value
	}
	return defaultPaymentsCallerAPIKey
}

func paymentsNoAccessAPIKey() string {
	if value := strings.TrimSpace(os.Getenv("PAYMENTS_NO_ACCESS_API_KEY")); value != "" {
		return value
	}
	return defaultPaymentsNoAccessAPIKey
}

func paymentsAppAPIKey() string {
	if value := strings.TrimSpace(os.Getenv("PAYMENTS_APP_API_KEY")); value != "" {
		return value
	}
	return defaultPaymentsAppAPIKey
}

type paymentsAuthGRPCServer struct {
	authpb.UnimplementedAuthServiceServer
}

func (s *paymentsAuthGRPCServer) ValidateInternalAccess(ctx context.Context, req *authpb.ValidateInternalAccessRequest) (*authpb.ValidateInternalAccessResponse, error) {
	if incomingPaymentsAPIKey(ctx) != paymentsAppAPIKey() {
		return nil, status.Error(codes.Unauthenticated, "unauthorized caller")
	}

	apiKey := strings.TrimSpace(req.GetApiKey())
	switch apiKey {
	case paymentsCallerAPIKey():
		return &authpb.ValidateInternalAccessResponse{
			ServiceName:   "payments-gateway",
			AllowedAccess: []string{"payments-service", "subscriptions-service", "notifications-service", "profile-service"},
		}, nil
	case paymentsNoAccessAPIKey():
		return &authpb.ValidateInternalAccessResponse{
			ServiceName:   "payments-gateway",
			AllowedAccess: []string{"subscriptions-service"},
		}, nil
	default:
		return nil, status.Error(codes.Unauthenticated, "invalid api key")
	}
}

func incomingPaymentsAPIKey(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-api-key")
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func TestMain(m *testing.M) {
	if os.Getenv("PAYMENTS_CALLER_API_KEY") == "" {
		_ = os.Setenv("PAYMENTS_CALLER_API_KEY", defaultPaymentsCallerAPIKey)
	}
	if os.Getenv("PAYMENTS_NO_ACCESS_API_KEY") == "" {
		_ = os.Setenv("PAYMENTS_NO_ACCESS_API_KEY", defaultPaymentsNoAccessAPIKey)
	}
	if os.Getenv("PAYMENTS_APP_API_KEY") == "" {
		_ = os.Setenv("PAYMENTS_APP_API_KEY", defaultPaymentsAppAPIKey)
	}

	listener, err := net.Listen("tcp", paymentsAuthMockAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start payments auth grpc mock: %v\n", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	authpb.RegisterAuthServiceServer(grpcServer, &paymentsAuthGRPCServer{})

	go func() {
		_ = grpcServer.Serve(listener)
	}()

	exitCode := m.Run()

	grpcServer.GracefulStop()
	_ = listener.Close()

	os.Exit(exitCode)
}
