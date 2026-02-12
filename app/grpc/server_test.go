package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/provider"
	"github.com/vibast-solutions/ms-go-payments/app/repository"
	"github.com/vibast-solutions/ms-go-payments/app/service"
	"github.com/vibast-solutions/ms-go-payments/app/types"
	"github.com/vibast-solutions/ms-go-payments/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcPaymentRepo struct {
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

func (r *grpcPaymentRepo) Create(ctx context.Context, payment *entity.Payment) error {
	if r.createFn != nil {
		return r.createFn(ctx, payment)
	}
	return nil
}

func (r *grpcPaymentRepo) Update(ctx context.Context, payment *entity.Payment) error {
	if r.updateFn != nil {
		return r.updateFn(ctx, payment)
	}
	return nil
}

func (r *grpcPaymentRepo) FindByID(ctx context.Context, id uint64) (*entity.Payment, error) {
	if r.findByIDFn != nil {
		return r.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (r *grpcPaymentRepo) FindByCallerRequestID(ctx context.Context, callerService, requestID string) (*entity.Payment, error) {
	if r.findByCallerRequestIDFn != nil {
		return r.findByCallerRequestIDFn(ctx, callerService, requestID)
	}
	return nil, nil
}

func (r *grpcPaymentRepo) FindByCallbackHash(ctx context.Context, provider int32, callbackHash string) (*entity.Payment, error) {
	if r.findByCallbackHashFn != nil {
		return r.findByCallbackHashFn(ctx, provider, callbackHash)
	}
	return nil, nil
}

func (r *grpcPaymentRepo) List(ctx context.Context, filter repository.PaymentFilter) ([]*entity.Payment, error) {
	if r.listFn != nil {
		return r.listFn(ctx, filter)
	}
	return []*entity.Payment{}, nil
}

func (r *grpcPaymentRepo) ListDueCallbackDispatch(ctx context.Context, now time.Time, limit int32) ([]*entity.Payment, error) {
	if r.listDueCallbackDispatchFn != nil {
		return r.listDueCallbackDispatchFn(ctx, now, limit)
	}
	return []*entity.Payment{}, nil
}

func (r *grpcPaymentRepo) ListExpiredPending(ctx context.Context, cutoff time.Time, limit int32) ([]*entity.Payment, error) {
	if r.listExpiredPendingFn != nil {
		return r.listExpiredPendingFn(ctx, cutoff, limit)
	}
	return []*entity.Payment{}, nil
}

func (r *grpcPaymentRepo) ListForReconcile(ctx context.Context, before time.Time, limit int32) ([]*entity.Payment, error) {
	if r.listForReconcileFn != nil {
		return r.listForReconcileFn(ctx, before, limit)
	}
	return []*entity.Payment{}, nil
}

type grpcEventRepo struct{}

func (r *grpcEventRepo) Create(context.Context, *entity.PaymentEvent) error {
	return nil
}

type grpcCallbackRepo struct{}

func (r *grpcCallbackRepo) Create(context.Context, *entity.PaymentCallback) error {
	return nil
}

type grpcProvider struct {
	createOutput *provider.CreateOutput
	createErr    error
	callbackErr  error
	callbackEvt  *provider.CallbackEvent
	status       int32
	statusErr    error
}

func (p *grpcProvider) Code() int32 {
	return int32(types.ProviderType_PROVIDER_TYPE_STRIPE)
}

func (p *grpcProvider) CreatePayment(context.Context, *provider.CreateInput) (*provider.CreateOutput, error) {
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
		ProviderCallbackURL: "https://gateway.example/payments/callback/hash",
		InitialStatus:       int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
	}, nil
}

func (p *grpcProvider) VerifyAndParseCallback(context.Context, []byte, string) (*provider.CallbackEvent, error) {
	if p.callbackErr != nil {
		return nil, p.callbackErr
	}
	if p.callbackEvt != nil {
		return p.callbackEvt, nil
	}
	return &provider.CallbackEvent{EventType: "checkout.session.completed", NewStatus: int32(types.PaymentStatus_PAYMENT_STATUS_PAID)}, nil
}

func (p *grpcProvider) GetPaymentStatus(context.Context, string) (int32, error) {
	if p.statusErr != nil {
		return 0, p.statusErr
	}
	return p.status, nil
}

func newGRPCServerForTest(repo *grpcPaymentRepo, p provider.Provider) *Server {
	paymentService := service.NewPaymentService(
		repo,
		&grpcEventRepo{},
		&grpcCallbackRepo{},
		provider.NewRegistry(p),
		config.PaymentsConfig{CallbackMaxAttempts: 3, CallbackRetryInterval: time.Minute, PendingTimeout: time.Hour, ReconcileStaleAfter: time.Minute, JobBatchSize: 100},
		"payments-app-key",
	)
	return NewServer(paymentService)
}

func TestCreatePaymentInvalidArgument(t *testing.T) {
	srv := newGRPCServerForTest(&grpcPaymentRepo{}, &grpcProvider{})

	_, err := srv.CreatePayment(context.Background(), &types.CreatePaymentRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestCreatePaymentSuccess(t *testing.T) {
	repo := &grpcPaymentRepo{
		createFn: func(_ context.Context, payment *entity.Payment) error {
			payment.ID = 77
			return nil
		},
	}
	srv := newGRPCServerForTest(repo, &grpcProvider{})

	resp, err := srv.CreatePayment(context.Background(), &types.CreatePaymentRequest{
		RequestId:         "req-1",
		CallerService:     "subscriptions-service",
		ResourceType:      "subscription",
		ResourceId:        "sub-1",
		AmountCents:       1999,
		Currency:          "USD",
		PaymentMethod:     types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD,
		PaymentType:       types.PaymentType_PAYMENT_TYPE_ONE_TIME,
		StatusCallbackUrl: "https://caller.example/callback",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.GetPayment().GetId() != 77 {
		t.Fatalf("expected id=77, got %+v", resp.GetPayment())
	}
}

func TestGetPaymentNotFound(t *testing.T) {
	repo := &grpcPaymentRepo{findByIDFn: func(context.Context, uint64) (*entity.Payment, error) { return nil, nil }}
	srv := newGRPCServerForTest(repo, &grpcProvider{})

	_, err := srv.GetPayment(context.Background(), &types.GetPaymentRequest{Id: 9})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestCancelPaymentPaidInvalidStatus(t *testing.T) {
	repo := &grpcPaymentRepo{findByIDFn: func(context.Context, uint64) (*entity.Payment, error) {
		return &entity.Payment{ID: 9, Status: int32(types.PaymentStatus_PAYMENT_STATUS_PAID)}, nil
	}}
	srv := newGRPCServerForTest(repo, &grpcProvider{})

	_, err := srv.CancelPayment(context.Background(), &types.CancelPaymentRequest{Id: 9, Reason: "duplicate"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestHandleProviderCallbackRejected(t *testing.T) {
	repo := &grpcPaymentRepo{}
	srv := newGRPCServerForTest(repo, &grpcProvider{callbackErr: errors.New("invalid signature")})

	_, err := srv.HandleProviderCallback(context.Background(), &types.HandleProviderCallbackRequest{
		RequestId:    "req-1",
		Provider:     "stripe",
		CallbackHash: "hash-1",
		Signature:    "sig",
		Payload:      `{"id":"evt_1"}`,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestListPaymentsSuccess(t *testing.T) {
	repo := &grpcPaymentRepo{listFn: func(context.Context, repository.PaymentFilter) ([]*entity.Payment, error) {
		return []*entity.Payment{{
			ID:                5,
			RequestID:         "req-1",
			CallerService:     "subscriptions-service",
			ResourceType:      "subscription",
			ResourceID:        "sub-1",
			AmountCents:       1000,
			Currency:          "USD",
			Status:            int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
			PaymentMethod:     int32(types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD),
			PaymentType:       int32(types.PaymentType_PAYMENT_TYPE_ONE_TIME),
			Provider:          int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
			ProviderCallbackHash: "hash-1",
			ProviderCallbackURL:  "https://gateway.example/callback/hash-1",
			StatusCallbackURL:    "https://caller.example/status",
			Metadata:          map[string]string{},
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		}}, nil
	}}
	srv := newGRPCServerForTest(repo, &grpcProvider{})

	resp, err := srv.ListPayments(context.Background(), &types.ListPaymentsRequest{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.GetPayments()) != 1 || resp.GetPayments()[0].GetId() != 5 {
		t.Fatalf("unexpected payments response: %+v", resp)
	}
}
