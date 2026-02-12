package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/provider"
	"github.com/vibast-solutions/ms-go-payments/app/repository"
	"github.com/vibast-solutions/ms-go-payments/app/types"
	"github.com/vibast-solutions/ms-go-payments/config"
)

type servicePaymentRepo struct {
	payments map[uint64]*entity.Payment
	nextID   uint64
}

func newServicePaymentRepo() *servicePaymentRepo {
	return &servicePaymentRepo{
		payments: map[uint64]*entity.Payment{},
		nextID:   1,
	}
}

func (r *servicePaymentRepo) Create(_ context.Context, payment *entity.Payment) error {
	for _, item := range r.payments {
		if item.CallerService == payment.CallerService && item.RequestID == payment.RequestID {
			return repository.ErrPaymentAlreadyExists
		}
	}
	id := r.nextID
	r.nextID++
	copyItem := *payment
	copyItem.ID = id
	r.payments[id] = &copyItem
	payment.ID = id
	return nil
}

func (r *servicePaymentRepo) Update(_ context.Context, payment *entity.Payment) error {
	if _, ok := r.payments[payment.ID]; !ok {
		return repository.ErrPaymentNotFound
	}
	copyItem := *payment
	r.payments[payment.ID] = &copyItem
	return nil
}

func (r *servicePaymentRepo) FindByID(_ context.Context, id uint64) (*entity.Payment, error) {
	item, ok := r.payments[id]
	if !ok {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (r *servicePaymentRepo) FindByCallerRequestID(_ context.Context, callerService, requestID string) (*entity.Payment, error) {
	for _, item := range r.payments {
		if item.CallerService == callerService && item.RequestID == requestID {
			copyItem := *item
			return &copyItem, nil
		}
	}
	return nil, nil
}

func (r *servicePaymentRepo) FindByCallbackHash(_ context.Context, providerCode int32, callbackHash string) (*entity.Payment, error) {
	for _, item := range r.payments {
		if item.Provider == providerCode && item.ProviderCallbackHash == callbackHash {
			copyItem := *item
			return &copyItem, nil
		}
	}
	return nil, nil
}

func (r *servicePaymentRepo) List(_ context.Context, filter repository.PaymentFilter) ([]*entity.Payment, error) {
	items := make([]*entity.Payment, 0)
	for _, item := range r.payments {
		if filter.RequestID != "" && item.RequestID != filter.RequestID {
			continue
		}
		if filter.CallerService != "" && item.CallerService != filter.CallerService {
			continue
		}
		if filter.ResourceType != "" && item.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && item.ResourceID != filter.ResourceID {
			continue
		}
		if filter.HasStatus && item.Status != filter.Status {
			continue
		}
		if filter.Provider > 0 && item.Provider != filter.Provider {
			continue
		}
		copyItem := *item
		items = append(items, &copyItem)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })

	start := int(filter.Offset)
	if start > len(items) {
		return []*entity.Payment{}, nil
	}
	end := start + int(filter.Limit)
	if end > len(items) {
		end = len(items)
	}
	if filter.Limit <= 0 {
		return items, nil
	}
	return items[start:end], nil
}

func (r *servicePaymentRepo) ListDueCallbackDispatch(_ context.Context, now time.Time, limit int32) ([]*entity.Payment, error) {
	items := make([]*entity.Payment, 0)
	for _, item := range r.payments {
		if item.CallbackDeliveryStatus == entity.CallbackDeliveryPending && item.CallbackDeliveryNextAt != nil && !item.CallbackDeliveryNextAt.After(now) {
			copyItem := *item
			items = append(items, &copyItem)
		}
	}
	return limitItems(items, limit), nil
}

func (r *servicePaymentRepo) ListExpiredPending(_ context.Context, cutoff time.Time, limit int32) ([]*entity.Payment, error) {
	items := make([]*entity.Payment, 0)
	for _, item := range r.payments {
		if (item.Status == int32(types.PaymentStatus_PAYMENT_STATUS_PENDING) || item.Status == int32(types.PaymentStatus_PAYMENT_STATUS_PROCESSING)) && !item.CreatedAt.After(cutoff) {
			copyItem := *item
			items = append(items, &copyItem)
		}
	}
	return limitItems(items, limit), nil
}

func (r *servicePaymentRepo) ListForReconcile(_ context.Context, before time.Time, limit int32) ([]*entity.Payment, error) {
	items := make([]*entity.Payment, 0)
	for _, item := range r.payments {
		if (item.Status == int32(types.PaymentStatus_PAYMENT_STATUS_PENDING) || item.Status == int32(types.PaymentStatus_PAYMENT_STATUS_PROCESSING)) && item.ProviderPaymentID != nil && !item.UpdatedAt.After(before) {
			copyItem := *item
			items = append(items, &copyItem)
		}
	}
	return limitItems(items, limit), nil
}

func limitItems(items []*entity.Payment, limit int32) []*entity.Payment {
	if limit <= 0 || int(limit) >= len(items) {
		return items
	}
	return items[:limit]
}

type serviceEventRepo struct {
	events []*entity.PaymentEvent
}

func (r *serviceEventRepo) Create(_ context.Context, event *entity.PaymentEvent) error {
	copyItem := *event
	r.events = append(r.events, &copyItem)
	return nil
}

type serviceCallbackRepo struct {
	callbacks []*entity.PaymentCallback
}

func (r *serviceCallbackRepo) Create(_ context.Context, callback *entity.PaymentCallback) error {
	copyItem := *callback
	r.callbacks = append(r.callbacks, &copyItem)
	return nil
}

type serviceProvider struct {
	createOutput *provider.CreateOutput
	createErr    error
	callbackEvt  *provider.CallbackEvent
	callbackErr  error
	reconcile    int32
	reconcileErr error
}

func (p *serviceProvider) Code() int32 {
	return int32(types.ProviderType_PROVIDER_TYPE_STRIPE)
}

func (p *serviceProvider) CreatePayment(context.Context, *provider.CreateInput) (*provider.CreateOutput, error) {
	if p.createErr != nil {
		return nil, p.createErr
	}
	if p.createOutput != nil {
		return p.createOutput, nil
	}
	pid := "cs_test_123"
	url := "https://stripe.example/checkout/session"
	return &provider.CreateOutput{
		ProviderPaymentID:   &pid,
		CheckoutURL:         &url,
		ProviderCallbackURL: "https://gateway.example/callback/hash",
		InitialStatus:       int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
	}, nil
}

func (p *serviceProvider) VerifyAndParseCallback(context.Context, []byte, string) (*provider.CallbackEvent, error) {
	if p.callbackErr != nil {
		return nil, p.callbackErr
	}
	if p.callbackEvt != nil {
		return p.callbackEvt, nil
	}
	return &provider.CallbackEvent{EventType: "checkout.session.completed", NewStatus: int32(types.PaymentStatus_PAYMENT_STATUS_PAID)}, nil
}

func (p *serviceProvider) GetPaymentStatus(context.Context, string) (int32, error) {
	if p.reconcileErr != nil {
		return 0, p.reconcileErr
	}
	return p.reconcile, nil
}

func newPaymentServiceForTest(repo *servicePaymentRepo, eventRepo *serviceEventRepo, callbackRepo *serviceCallbackRepo, p provider.Provider) *PaymentService {
	return NewPaymentService(
		repo,
		eventRepo,
		callbackRepo,
		provider.NewRegistry(p),
		config.PaymentsConfig{
			CallbackMaxAttempts:   3,
			CallbackRetryInterval: time.Second,
			CallbackHTTPTimeout:   time.Second,
			PendingTimeout:        time.Minute,
			ReconcileStaleAfter:   time.Minute,
			JobBatchSize:          100,
		},
		"payments-app-key",
	)
}

func TestCreatePaymentIdempotentByRequestIDAndCallerService(t *testing.T) {
	repo := newServicePaymentRepo()
	eventRepo := &serviceEventRepo{}
	callbackRepo := &serviceCallbackRepo{}
	svc := newPaymentServiceForTest(repo, eventRepo, callbackRepo, &serviceProvider{})

	first, err := svc.CreatePayment(context.Background(), &types.CreatePaymentRequest{
		RequestId:         "req-1",
		CallerService:     "subscriptions-service",
		ResourceType:      "subscription",
		ResourceId:        "sub-1",
		AmountCents:       1000,
		Currency:          "USD",
		PaymentMethod:     types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD,
		PaymentType:       types.PaymentType_PAYMENT_TYPE_ONE_TIME,
		StatusCallbackUrl: "https://caller.example/callback",
	})
	if err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	second, err := svc.CreatePayment(context.Background(), &types.CreatePaymentRequest{
		RequestId:         "req-1",
		CallerService:     "subscriptions-service",
		ResourceType:      "subscription",
		ResourceId:        "sub-1",
		AmountCents:       1000,
		Currency:          "USD",
		PaymentMethod:     types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD,
		PaymentType:       types.PaymentType_PAYMENT_TYPE_ONE_TIME,
		StatusCallbackUrl: "https://caller.example/callback",
	})
	if err != nil {
		t.Fatalf("second create payment failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same payment id for idempotent request, first=%d second=%d", first.ID, second.ID)
	}
}

func TestCreatePaymentRequiresRequestIDAndCallerService(t *testing.T) {
	repo := newServicePaymentRepo()
	svc := newPaymentServiceForTest(repo, &serviceEventRepo{}, &serviceCallbackRepo{}, &serviceProvider{})

	_, err := svc.CreatePayment(context.Background(), &types.CreatePaymentRequest{
		ResourceType:      "subscription",
		ResourceId:        "sub-1",
		AmountCents:       1000,
		Currency:          "USD",
		PaymentMethod:     types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD,
		PaymentType:       types.PaymentType_PAYMENT_TYPE_ONE_TIME,
		StatusCallbackUrl: "https://caller.example/callback",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestCancelPaymentPaidIsInvalidStatus(t *testing.T) {
	repo := newServicePaymentRepo()
	repo.payments[1] = &entity.Payment{ID: 1, Status: int32(types.PaymentStatus_PAYMENT_STATUS_PAID)}
	svc := newPaymentServiceForTest(repo, &serviceEventRepo{}, &serviceCallbackRepo{}, &serviceProvider{})

	_, err := svc.CancelPayment(context.Background(), &types.CancelPaymentRequest{Id: 1, Reason: "duplicate"})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

func TestHandleProviderCallbackUpdatesStatusAndStoresCallback(t *testing.T) {
	repo := newServicePaymentRepo()
	now := time.Now().UTC().Add(-time.Hour)
	repo.payments[1] = &entity.Payment{
		ID:                   1,
		RequestID:            "req-1",
		CallerService:        "subscriptions-service",
		Status:               int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
		Provider:             int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
		ProviderCallbackHash: "hash-1",
		ProviderCallbackURL:  "https://gateway.example/callback/hash-1",
		StatusCallbackURL:    "https://caller.example/status",
		Metadata:             map[string]string{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	eventRepo := &serviceEventRepo{}
	callbackRepo := &serviceCallbackRepo{}
	svc := newPaymentServiceForTest(repo, eventRepo, callbackRepo, &serviceProvider{
		callbackEvt: &provider.CallbackEvent{
			EventType: "checkout.session.completed",
			NewStatus: int32(types.PaymentStatus_PAYMENT_STATUS_PAID),
		},
	})

	payment, err := svc.HandleProviderCallback(context.Background(), &types.HandleProviderCallbackRequest{
		RequestId:    "cb-1",
		Provider:     "stripe",
		CallbackHash: "hash-1",
		Signature:    "valid-signature",
		Payload:      `{"id":"evt_1"}`,
	})
	if err != nil {
		t.Fatalf("handle callback failed: %v", err)
	}
	if payment.Status != int32(types.PaymentStatus_PAYMENT_STATUS_PAID) {
		t.Fatalf("expected paid status, got %d", payment.Status)
	}
	if len(callbackRepo.callbacks) != 1 {
		t.Fatalf("expected callback record, got %d", len(callbackRepo.callbacks))
	}
	if callbackRepo.callbacks[0].Status != paymentCallbackStatusProcessed {
		t.Fatalf("expected processed callback status, got %d", callbackRepo.callbacks[0].Status)
	}
	if len(eventRepo.events) == 0 {
		t.Fatal("expected payment event to be recorded")
	}
}

func TestRunExpirePendingBatchMarksExpired(t *testing.T) {
	repo := newServicePaymentRepo()
	now := time.Now().UTC().Add(-2 * time.Hour)
	repo.payments[1] = &entity.Payment{
		ID:                   1,
		RequestID:            "req-1",
		CallerService:        "subscriptions-service",
		Status:               int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
		Provider:             int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
		ProviderCallbackHash: "hash-1",
		ProviderCallbackURL:  "https://gateway.example/callback/hash-1",
		StatusCallbackURL:    "https://caller.example/status",
		Metadata:             map[string]string{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	cfgSvc := NewPaymentService(
		repo,
		&serviceEventRepo{},
		&serviceCallbackRepo{},
		provider.NewRegistry(&serviceProvider{}),
		config.PaymentsConfig{PendingTimeout: time.Minute, CallbackRetryInterval: time.Second, CallbackMaxAttempts: 3, JobBatchSize: 100},
		"payments-app-key",
	)

	if err := cfgSvc.RunExpirePendingBatch(context.Background()); err != nil {
		t.Fatalf("run expire pending batch failed: %v", err)
	}

	updated, _ := repo.FindByID(context.Background(), 1)
	if updated.Status != int32(types.PaymentStatus_PAYMENT_STATUS_EXPIRED) {
		t.Fatalf("expected expired status, got %d", updated.Status)
	}
	if updated.CallbackDeliveryStatus != entity.CallbackDeliveryPending {
		t.Fatalf("expected callback delivery pending, got %d", updated.CallbackDeliveryStatus)
	}
}

func TestRunReconcileBatchUpdatesTerminalStatus(t *testing.T) {
	repo := newServicePaymentRepo()
	now := time.Now().UTC().Add(-2 * time.Hour)
	providerPaymentID := "cs_test_123"
	repo.payments[1] = &entity.Payment{
		ID:                   1,
		RequestID:            "req-1",
		CallerService:        "subscriptions-service",
		Status:               int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
		Provider:             int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
		ProviderPaymentID:    &providerPaymentID,
		ProviderCallbackHash: "hash-1",
		ProviderCallbackURL:  "https://gateway.example/callback/hash-1",
		StatusCallbackURL:    "https://caller.example/status",
		Metadata:             map[string]string{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	svc := NewPaymentService(
		repo,
		&serviceEventRepo{},
		&serviceCallbackRepo{},
		provider.NewRegistry(&serviceProvider{reconcile: int32(types.PaymentStatus_PAYMENT_STATUS_PAID)}),
		config.PaymentsConfig{ReconcileStaleAfter: time.Minute, CallbackRetryInterval: time.Second, CallbackMaxAttempts: 3, JobBatchSize: 100},
		"payments-app-key",
	)

	if err := svc.RunReconcileBatch(context.Background()); err != nil {
		t.Fatalf("run reconcile batch failed: %v", err)
	}

	updated, _ := repo.FindByID(context.Background(), 1)
	if updated.Status != int32(types.PaymentStatus_PAYMENT_STATUS_PAID) {
		t.Fatalf("expected paid status after reconcile, got %d", updated.Status)
	}
}

func TestRunDispatchCallbacksBatchSuccess(t *testing.T) {
	repo := newServicePaymentRepo()
	now := time.Now().UTC()
	nextAt := now.Add(-time.Second)
	repo.payments[1] = &entity.Payment{
		ID:                     1,
		RequestID:              "req-1",
		CallerService:          "subscriptions-service",
		Status:                 int32(types.PaymentStatus_PAYMENT_STATUS_PAID),
		Provider:               int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
		ProviderCallbackHash:   "hash-1",
		ProviderCallbackURL:    "https://gateway.example/callback/hash-1",
		StatusCallbackURL:      "http://localhost/callback",
		Metadata:               map[string]string{},
		CallbackDeliveryStatus: entity.CallbackDeliveryPending,
		CallbackDeliveryNextAt: &nextAt,
		CreatedAt:              now.Add(-time.Hour),
		UpdatedAt:              now.Add(-time.Hour),
	}

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") != "req-1" {
			t.Fatalf("expected callback request to include x-request-id=req-1, got %q", r.Header.Get("X-Request-ID"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer callbackServer.Close()

	repo.payments[1].StatusCallbackURL = callbackServer.URL

	svc := NewPaymentService(
		repo,
		&serviceEventRepo{},
		&serviceCallbackRepo{},
		provider.NewRegistry(&serviceProvider{}),
		config.PaymentsConfig{CallbackRetryInterval: time.Second, CallbackMaxAttempts: 3, JobBatchSize: 100},
		"payments-app-key",
	)

	if err := svc.RunDispatchCallbacksBatch(context.Background()); err != nil {
		t.Fatalf("run dispatch callbacks batch failed: %v", err)
	}

	updated, _ := repo.FindByID(context.Background(), 1)
	if updated.CallbackDeliveryStatus != entity.CallbackDeliverySuccess {
		t.Fatalf("expected callback delivery success, got %d", updated.CallbackDeliveryStatus)
	}
}

func TestRunDispatchCallbacksBatchFailureMarksFailed(t *testing.T) {
	repo := newServicePaymentRepo()
	now := time.Now().UTC()
	nextAt := now.Add(-time.Second)
	repo.payments[1] = &entity.Payment{
		ID:                     1,
		RequestID:              "req-1",
		CallerService:          "subscriptions-service",
		Status:                 int32(types.PaymentStatus_PAYMENT_STATUS_FAILED),
		Provider:               int32(types.ProviderType_PROVIDER_TYPE_STRIPE),
		ProviderCallbackHash:   "hash-1",
		ProviderCallbackURL:    "https://gateway.example/callback/hash-1",
		StatusCallbackURL:      "http://localhost/callback",
		Metadata:               map[string]string{},
		CallbackDeliveryStatus: entity.CallbackDeliveryPending,
		CallbackDeliveryNextAt: &nextAt,
		CreatedAt:              now.Add(-time.Hour),
		UpdatedAt:              now.Add(-time.Hour),
	}

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer callbackServer.Close()

	repo.payments[1].StatusCallbackURL = callbackServer.URL

	svc := NewPaymentService(
		repo,
		&serviceEventRepo{},
		&serviceCallbackRepo{},
		provider.NewRegistry(&serviceProvider{}),
		config.PaymentsConfig{CallbackRetryInterval: time.Second, CallbackMaxAttempts: 1, JobBatchSize: 100},
		"payments-app-key",
	)

	err := svc.RunDispatchCallbacksBatch(context.Background())
	if err == nil {
		t.Fatal("expected dispatch callbacks batch to return error when callback endpoint fails")
	}

	updated, _ := repo.FindByID(context.Background(), 1)
	if updated.CallbackDeliveryStatus != entity.CallbackDeliveryFailed {
		t.Fatalf("expected callback delivery failed, got %d", updated.CallbackDeliveryStatus)
	}
	if updated.CallbackDeliveryAttempts != 1 {
		t.Fatalf("expected callback delivery attempts=1, got %d", updated.CallbackDeliveryAttempts)
	}
}
