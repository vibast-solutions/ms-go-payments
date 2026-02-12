package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/provider"
	"github.com/vibast-solutions/ms-go-payments/app/repository"
	"github.com/vibast-solutions/ms-go-payments/app/service"
	"github.com/vibast-solutions/ms-go-payments/app/types"
	"github.com/vibast-solutions/ms-go-payments/config"
)

type controllerPaymentRepo struct {
	createFn                 func(ctx context.Context, payment *entity.Payment) error
	updateFn                 func(ctx context.Context, payment *entity.Payment) error
	findByIDFn               func(ctx context.Context, id uint64) (*entity.Payment, error)
	findByCallerRequestIDFn  func(ctx context.Context, callerService, requestID string) (*entity.Payment, error)
	findByCallbackHashFn     func(ctx context.Context, provider int32, callbackHash string) (*entity.Payment, error)
	listFn                   func(ctx context.Context, filter repository.PaymentFilter) ([]*entity.Payment, error)
	listDueCallbackDispatchFn func(ctx context.Context, now time.Time, limit int32) ([]*entity.Payment, error)
	listExpiredPendingFn     func(ctx context.Context, cutoff time.Time, limit int32) ([]*entity.Payment, error)
	listForReconcileFn       func(ctx context.Context, before time.Time, limit int32) ([]*entity.Payment, error)
}

func (r *controllerPaymentRepo) Create(ctx context.Context, payment *entity.Payment) error {
	if r.createFn != nil {
		return r.createFn(ctx, payment)
	}
	return nil
}

func (r *controllerPaymentRepo) Update(ctx context.Context, payment *entity.Payment) error {
	if r.updateFn != nil {
		return r.updateFn(ctx, payment)
	}
	return nil
}

func (r *controllerPaymentRepo) FindByID(ctx context.Context, id uint64) (*entity.Payment, error) {
	if r.findByIDFn != nil {
		return r.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (r *controllerPaymentRepo) FindByCallerRequestID(ctx context.Context, callerService, requestID string) (*entity.Payment, error) {
	if r.findByCallerRequestIDFn != nil {
		return r.findByCallerRequestIDFn(ctx, callerService, requestID)
	}
	return nil, nil
}

func (r *controllerPaymentRepo) FindByCallbackHash(ctx context.Context, provider int32, callbackHash string) (*entity.Payment, error) {
	if r.findByCallbackHashFn != nil {
		return r.findByCallbackHashFn(ctx, provider, callbackHash)
	}
	return nil, nil
}

func (r *controllerPaymentRepo) List(ctx context.Context, filter repository.PaymentFilter) ([]*entity.Payment, error) {
	if r.listFn != nil {
		return r.listFn(ctx, filter)
	}
	return []*entity.Payment{}, nil
}

func (r *controllerPaymentRepo) ListDueCallbackDispatch(ctx context.Context, now time.Time, limit int32) ([]*entity.Payment, error) {
	if r.listDueCallbackDispatchFn != nil {
		return r.listDueCallbackDispatchFn(ctx, now, limit)
	}
	return []*entity.Payment{}, nil
}

func (r *controllerPaymentRepo) ListExpiredPending(ctx context.Context, cutoff time.Time, limit int32) ([]*entity.Payment, error) {
	if r.listExpiredPendingFn != nil {
		return r.listExpiredPendingFn(ctx, cutoff, limit)
	}
	return []*entity.Payment{}, nil
}

func (r *controllerPaymentRepo) ListForReconcile(ctx context.Context, before time.Time, limit int32) ([]*entity.Payment, error) {
	if r.listForReconcileFn != nil {
		return r.listForReconcileFn(ctx, before, limit)
	}
	return []*entity.Payment{}, nil
}

type controllerEventRepo struct{}

func (r *controllerEventRepo) Create(context.Context, *entity.PaymentEvent) error {
	return nil
}

type controllerCallbackRepo struct{}

func (r *controllerCallbackRepo) Create(context.Context, *entity.PaymentCallback) error {
	return nil
}

type controllerProvider struct {
	createOutput *provider.CreateOutput
	createErr    error
	callbackErr  error
	callbackEvt  *provider.CallbackEvent
}

func (p *controllerProvider) Code() int32 {
	return int32(types.ProviderType_PROVIDER_TYPE_STRIPE)
}

func (p *controllerProvider) CreatePayment(context.Context, *provider.CreateInput) (*provider.CreateOutput, error) {
	if p.createErr != nil {
		return nil, p.createErr
	}
	if p.createOutput != nil {
		return p.createOutput, nil
	}
	url := "https://stripe.example/checkout/session"
	id := "cs_test_123"
	return &provider.CreateOutput{
		ProviderPaymentID:   &id,
		CheckoutURL:         &url,
		ProviderCallbackURL: "https://gateway.example/callback/hash",
		InitialStatus:       int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
	}, nil
}

func (p *controllerProvider) VerifyAndParseCallback(context.Context, []byte, string) (*provider.CallbackEvent, error) {
	if p.callbackErr != nil {
		return nil, p.callbackErr
	}
	if p.callbackEvt != nil {
		return p.callbackEvt, nil
	}
	return &provider.CallbackEvent{EventType: "checkout.session.completed", NewStatus: int32(types.PaymentStatus_PAYMENT_STATUS_PAID)}, nil
}

func (p *controllerProvider) GetPaymentStatus(context.Context, string) (int32, error) {
	return 0, nil
}

func newControllerForTest(repo *controllerPaymentRepo, p provider.Provider) *PaymentController {
	paymentService := service.NewPaymentService(
		repo,
		&controllerEventRepo{},
		&controllerCallbackRepo{},
		provider.NewRegistry(p),
		config.PaymentsConfig{CallbackMaxAttempts: 3, CallbackRetryInterval: time.Minute, PendingTimeout: time.Hour, ReconcileStaleAfter: time.Minute, JobBatchSize: 100},
		"payments-app-key",
	)
	return NewPaymentController(paymentService)
}

func TestCreatePaymentBadBody(t *testing.T) {
	ctrl := newControllerForTest(&controllerPaymentRepo{}, &controllerProvider{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString("{bad"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	if err := ctrl.CreatePayment(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreatePaymentSuccess(t *testing.T) {
	repo := &controllerPaymentRepo{createFn: func(_ context.Context, payment *entity.Payment) error {
		payment.ID = 22
		return nil
	}}
	ctrl := newControllerForTest(repo, &controllerProvider{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString(`{"request_id":"req-1","caller_service":"subscriptions-service","resource_type":"subscription","resource_id":"sub-1","amount_cents":1000,"currency":"USD","payment_method":1,"payment_type":1,"status_callback_url":"https://caller.example/callback"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderXRequestID, "req-1")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	_ = ctrl.CreatePayment(ctx)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload types.PaymentEnvelopeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if payload.GetPayment().GetId() != 22 {
		t.Fatalf("unexpected payment payload: %+v", payload.GetPayment())
	}
}

func TestGetPaymentNotFound(t *testing.T) {
	ctrl := newControllerForTest(&controllerPaymentRepo{findByIDFn: func(context.Context, uint64) (*entity.Payment, error) { return nil, nil }}, &controllerProvider{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/payments/9", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues("9")

	_ = ctrl.GetPayment(ctx)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListPaymentsSuccess(t *testing.T) {
	now := time.Now().UTC()
	ctrl := newControllerForTest(&controllerPaymentRepo{listFn: func(context.Context, repository.PaymentFilter) ([]*entity.Payment, error) {
		return []*entity.Payment{{
			ID:                  1,
			RequestID:           "req-1",
			CallerService:       "subscriptions-service",
			ResourceType:        "subscription",
			ResourceID:          "sub-1",
			AmountCents:         1000,
			Currency:            "USD",
			Status:              int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
			PaymentMethod:       int32(types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD),
			PaymentType:         int32(types.PaymentType_PAYMENT_TYPE_ONE_TIME),
			Provider:            int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
			ProviderCallbackHash: "hash-1",
			ProviderCallbackURL:  "https://gateway.example/callback/hash-1",
			StatusCallbackURL:    "https://caller.example/status",
			Metadata:            map[string]string{},
			CreatedAt:           now,
			UpdatedAt:           now,
		}}, nil
	}}, &controllerProvider{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/payments?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	_ = ctrl.ListPayments(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCancelPaymentNotFound(t *testing.T) {
	ctrl := newControllerForTest(&controllerPaymentRepo{findByIDFn: func(context.Context, uint64) (*entity.Payment, error) { return nil, nil }}, &controllerProvider{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/payments/3/cancel", bytes.NewBufferString(`{"reason":"duplicate"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues("3")

	_ = ctrl.CancelPayment(ctx)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleProviderCallbackRejected(t *testing.T) {
	ctrl := newControllerForTest(&controllerPaymentRepo{}, &controllerProvider{callbackErr: errors.New("invalid signature")})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/providers/stripe/hash-1", bytes.NewBufferString(`{"id":"evt_1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderXRequestID, "req-callback-1")
	req.Header.Set("Stripe-Signature", "sig")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("provider", "hash")
	ctx.SetParamValues("stripe", "hash-1")

	_ = ctrl.HandleProviderCallback(ctx)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
