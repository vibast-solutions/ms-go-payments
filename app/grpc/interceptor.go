package grpc

import (
	"context"
	"runtime/debug"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const requestIDHeader = "x-request-id"

type requestIDContextKey struct{}

func RequestIDInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requestID := strings.TrimSpace(requestIDFromMetadata(ctx))
		if requestID == "" {
			return nil, status.Error(codes.InvalidArgument, "x-request-id header is required")
		}

		ctx = context.WithValue(ctx, requestIDContextKey{}, requestID)
		_ = grpc.SetHeader(ctx, metadata.Pairs(requestIDHeader, requestID))

		return handler(ctx, req)
	}
}

func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start)

		fields := logrus.Fields{
			"method":     info.FullMethod,
			"grpc_code":  status.Code(err).String(),
			"latency":    latency.String(),
			"latency_ns": latency.Nanoseconds(),
		}

		if requestID := RequestIDFromContext(ctx); requestID != "" {
			fields["request_id"] = requestID
		}

		entry := logrus.WithFields(fields)
		if err != nil {
			entry.WithError(err).Warn("grpc_request")
			return resp, err
		}
		entry.Info("grpc_request")
		return resp, nil
	}
}

func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ interface{}, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				entry := logrus.WithField("method", info.FullMethod).WithField("panic", rec)
				if requestID := RequestIDFromContext(ctx); requestID != "" {
					entry = entry.WithField("request_id", requestID)
				}
				entry.WithField("stack", string(debug.Stack())).Error("grpc_panic_recovered")
				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func loggerWithContext(ctx context.Context) *logrus.Entry {
	entry := logrus.NewEntry(logrus.StandardLogger())
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		entry = entry.WithField("request_id", requestID)
	}
	return entry
}

func requestIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(requestIDHeader)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
