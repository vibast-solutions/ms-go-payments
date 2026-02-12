//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultPaymentsHTTPBase = "http://localhost:48080"
	defaultPaymentsGRPCAddr = "localhost:49090"
)

type httpClient struct {
	baseURL string
	client  *http.Client
}

func newHTTPClient(baseURL string) *httpClient {
	return &httpClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *httpClient) doJSON(t *testing.T, method, path string, body any) (*http.Response, []byte) {
	return c.doJSONWithAPIKey(t, method, path, body, paymentsCallerAPIKey())
}

func (c *httpClient) doJSONWithAPIKey(t *testing.T, method, path string, body any, apiKey string) (*http.Response, []byte) {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json marshal failed: %v", err)
		}
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Request-ID", fmt.Sprintf("e2e-http-%d", time.Now().UnixNano()))
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("http request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}

	return resp, bodyBytes
}

func waitForHTTP(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
		req.Header.Set("X-Request-ID", fmt.Sprintf("wait-http-%d", time.Now().UnixNano()))
		req.Header.Set("X-API-Key", paymentsCallerAPIKey())
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("http service not ready at %s", baseURL)
}

func waitForGRPC(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("grpc service not ready at %s", addr)
}

func withGRPCHeaders(apiKey string) grpc.DialOption {
	return grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", fmt.Sprintf("e2e-grpc-%d", time.Now().UnixNano()))
		if apiKey != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", apiKey)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	})
}

func dialPaymentsGRPC(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), withGRPCHeaders(paymentsCallerAPIKey()))
	if err != nil {
		t.Fatalf("grpc dial failed: %v", err)
	}
	return conn
}

func dialPaymentsGRPCRaw(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial failed: %v", err)
	}
	return conn
}

func grpcContextWithHeaders(apiKey, requestID string) context.Context {
	ctx := context.Background()
	if requestID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", requestID)
	}
	if apiKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", apiKey)
	}
	return ctx
}

func TestPaymentsE2E(t *testing.T) {
	httpBase := os.Getenv("PAYMENTS_HTTP_URL")
	if httpBase == "" {
		httpBase = defaultPaymentsHTTPBase
	}
	grpcAddr := os.Getenv("PAYMENTS_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = defaultPaymentsGRPCAddr
	}

	if err := waitForHTTP(httpBase, 30*time.Second); err != nil {
		t.Fatalf("http not ready: %v", err)
	}
	if err := waitForGRPC(grpcAddr, 30*time.Second); err != nil {
		t.Fatalf("grpc not ready: %v", err)
	}

	client := newHTTPClient(httpBase)

	conn := dialPaymentsGRPC(t, grpcAddr)
	defer conn.Close()
	grpcClient := types.NewPaymentsServiceClient(conn)

	rawConn := dialPaymentsGRPCRaw(t, grpcAddr)
	defer rawConn.Close()
	rawGRPCClient := types.NewPaymentsServiceClient(rawConn)

	t.Run("HTTPMissingRequestID", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, httpBase+"/health", nil)
		if err != nil {
			t.Fatalf("new request failed: %v", err)
		}
		req.Header.Set("X-API-Key", paymentsCallerAPIKey())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing x-request-id, got %d", resp.StatusCode)
		}
	})

	t.Run("HTTPUnauthorizedMissingAPIKey", func(t *testing.T) {
		resp, _ := client.doJSONWithAPIKey(t, http.MethodGet, "/health", nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing x-api-key, got %d", resp.StatusCode)
		}
	})

	t.Run("HTTPForbiddenInsufficientAccess", func(t *testing.T) {
		resp, _ := client.doJSONWithAPIKey(t, http.MethodGet, "/health", nil, paymentsNoAccessAPIKey())
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for insufficient access, got %d", resp.StatusCode)
		}
	})

	t.Run("GRPCMissingRequestID", func(t *testing.T) {
		_, err := rawGRPCClient.GetPayment(context.Background(), &types.GetPaymentRequest{Id: 1})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument for missing x-request-id, got %v", err)
		}
	})

	t.Run("GRPCUnauthorizedMissingAPIKey", func(t *testing.T) {
		ctx := grpcContextWithHeaders("", fmt.Sprintf("e2e-grpc-no-auth-%d", time.Now().UnixNano()))
		_, err := rawGRPCClient.GetPayment(ctx, &types.GetPaymentRequest{Id: 1})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", err)
		}
	})

	t.Run("GRPCForbiddenInsufficientAccess", func(t *testing.T) {
		ctx := grpcContextWithHeaders(paymentsNoAccessAPIKey(), fmt.Sprintf("e2e-grpc-forbidden-%d", time.Now().UnixNano()))
		_, err := rawGRPCClient.GetPayment(ctx, &types.GetPaymentRequest{Id: 1})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("HTTPValidationCreate", func(t *testing.T) {
		resp, _ := client.doJSON(t, http.MethodPost, "/payments", map[string]any{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid create request, got %d", resp.StatusCode)
		}
	})

	t.Run("GRPCValidationCreate", func(t *testing.T) {
		_, err := grpcClient.CreatePayment(context.Background(), &types.CreatePaymentRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("HTTPListPayments", func(t *testing.T) {
		resp, body := client.doJSON(t, http.MethodGet, "/payments?limit=10&offset=0", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
		}
		var payload types.ListPaymentsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal list payments failed: %v body=%s", err, string(body))
		}
	})

	t.Run("HTTPCancelNotFound", func(t *testing.T) {
		resp, body := client.doJSON(t, http.MethodPost, "/payments/999999/cancel", map[string]any{"reason": "e2e"})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("HTTPCallbackValidation", func(t *testing.T) {
		path := "/webhooks/providers/stripe/test-hash"
		resp, body := client.doJSON(t, http.MethodPost, path, map[string]any{"payload": "{}"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("GRPCGetNotFound", func(t *testing.T) {
		_, err := grpcClient.GetPayment(context.Background(), &types.GetPaymentRequest{Id: 999999})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("GRPCListPayments", func(t *testing.T) {
		res, err := grpcClient.ListPayments(context.Background(), &types.ListPaymentsRequest{
			Limit:  10,
			Offset: 0,
		})
		if err != nil {
			t.Fatalf("grpc list payments failed: %v", err)
		}
		if res == nil {
			t.Fatal("expected list response")
		}
	})

	t.Run("GRPCCancelNotFound", func(t *testing.T) {
		_, err := grpcClient.CancelPayment(context.Background(), &types.CancelPaymentRequest{Id: 999999, Reason: "e2e"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("HTTPGetNotFound", func(t *testing.T) {
		resp, body := client.doJSON(t, http.MethodGet, "/payments/"+strconv.FormatUint(999999, 10), nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", resp.StatusCode, string(body))
		}
	})
}
