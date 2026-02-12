package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRequestIDFromMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(requestIDHeader, "grpc-abc"))
	if got := requestIDFromMetadata(ctx); got != "grpc-abc" {
		t.Fatalf("expected grpc-abc, got %q", got)
	}
}

func TestRequestIDInterceptorRequiresHeader(t *testing.T) {
	interceptor := RequestIDInterceptor()

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for missing x-request-id, got %v", err)
	}
}

func TestRequestIDInterceptorUsesIncomingHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(requestIDHeader, "grpc-fixed"))
	interceptor := RequestIDInterceptor()

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		if got := RequestIDFromContext(ctx); got != "grpc-fixed" {
			t.Fatalf("expected grpc-fixed, got %q", got)
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecoveryInterceptorConvertsPanicToInternal(t *testing.T) {
	interceptor := RecoveryInterceptor()
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/payments.PaymentsService/CreatePayment"}, func(context.Context, interface{}) (interface{}, error) {
		panic("boom")
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected codes.Internal, got %v", err)
	}
}

func TestLoggingInterceptorPassThrough(t *testing.T) {
	interceptor := LoggingInterceptor()
	resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/payments.PaymentsService/GetPayment"}, func(context.Context, interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "ok" {
		t.Fatalf("unexpected response: %v", resp)
	}
}
