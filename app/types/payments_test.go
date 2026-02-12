package types

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNewCreatePaymentRequestFromContextUsesHeaderRequestID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("POST", "/payments", bytes.NewBufferString(`{"caller_service":"subscriptions-service","resource_type":"subscription","resource_id":"sub_1","amount_cents":1999,"currency":"usd","payment_method":1,"payment_type":1,"status_callback_url":"https://example.com/callback"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderXRequestID, "req-from-header")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	parsed, err := NewCreatePaymentRequestFromContext(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if parsed.GetRequestId() != "req-from-header" {
		t.Fatalf("expected header request id, got %q", parsed.GetRequestId())
	}
	if parsed.GetCurrency() != "USD" {
		t.Fatalf("expected upper-cased currency, got %q", parsed.GetCurrency())
	}
}

func TestCreatePaymentValidate(t *testing.T) {
	req := &CreatePaymentRequest{}
	if err := req.Validate(); err == nil {
		t.Fatal("expected request_id validation error")
	}

	req = &CreatePaymentRequest{
		RequestId:         "req-1",
		CallerService:     "subscriptions-service",
		ResourceType:      "subscription",
		ResourceId:        "sub-1",
		AmountCents:       999,
		Currency:          "USD",
		PaymentMethod:     PaymentMethod_PAYMENT_METHOD_HOSTED_CARD,
		PaymentType:       PaymentType_PAYMENT_TYPE_RECURRING,
		RecurringInterval: "month",
		StatusCallbackUrl: "https://example.com/callback",
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected recurring_interval_count validation error")
	}

	req.RecurringIntervalCount = 1
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid recurring request, got %v", err)
	}
}

func TestNewListPaymentsRequestFromContextAndValidate(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/payments?status=10&provider=stripe&limit=20&offset=3", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	parsed, err := NewListPaymentsRequestFromContext(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !parsed.GetHasStatus() || parsed.GetStatus() != PaymentStatus_PAYMENT_STATUS_PAID {
		t.Fatalf("unexpected status parse: %+v", parsed)
	}
	if parsed.GetProvider() != ProviderType_PROVIDER_TYPE_STRIPE {
		t.Fatalf("unexpected provider parse: %+v", parsed)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("expected valid list request, got %v", err)
	}
}

func TestListPaymentsValidateDefaultLimit(t *testing.T) {
	req := &ListPaymentsRequest{}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected zero-values request to apply default limit, got %v", err)
	}
	if req.GetLimit() != 100 {
		t.Fatalf("expected default limit 100, got %d", req.GetLimit())
	}
}

func TestNewCancelPaymentRequestFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("POST", "/payments/12/cancel", bytes.NewBufferString(`{"reason":" duplicate "}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues("12")

	parsed, err := NewCancelPaymentRequestFromContext(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if parsed.GetId() != 12 || parsed.GetReason() != "duplicate" {
		t.Fatalf("unexpected parsed cancel request: %+v", parsed)
	}
}

func TestNewHandleProviderCallbackRequestFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("POST", "/webhooks/providers/stripe/hash-1", bytes.NewBufferString(`{"payload":"{\"id\":\"evt_1\"}","signature":"sig-value"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderXRequestID, "callback-req-1")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("provider", "hash")
	ctx.SetParamValues("stripe", "hash-1")

	parsed, err := NewHandleProviderCallbackRequestFromContext(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if parsed.GetRequestId() != "callback-req-1" {
		t.Fatalf("expected callback request id, got %q", parsed.GetRequestId())
	}
	if parsed.GetProvider() != "stripe" || parsed.GetCallbackHash() != "hash-1" {
		t.Fatalf("unexpected callback route params: %+v", parsed)
	}
	if parsed.GetSignature() != "sig-value" {
		t.Fatalf("expected signature from body override, got %q", parsed.GetSignature())
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("expected valid callback request, got %v", err)
	}
}
