package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/provider"
	"github.com/vibast-solutions/ms-go-payments/app/repository"
	"github.com/vibast-solutions/ms-go-payments/app/types"
	"github.com/vibast-solutions/ms-go-payments/config"
)

const (
	defaultListLimit = int32(100)
	defaultBatchSize = int32(100)
)

type createPaymentRequest interface {
	GetRequestId() string
	GetCallerService() string
	GetResourceType() string
	GetResourceId() string
	GetCustomerRef() string
	GetAmountCents() int64
	GetCurrency() string
	GetPaymentMethod() types.PaymentMethod
	GetPaymentType() types.PaymentType
	GetProvider() types.ProviderType
	GetRecurringInterval() string
	GetRecurringIntervalCount() int32
	GetStatusCallbackUrl() string
	GetSuccessUrl() string
	GetCancelUrl() string
	GetMetadata() map[string]string
}

type listPaymentsRequest interface {
	GetRequestId() string
	GetCallerService() string
	GetResourceType() string
	GetResourceId() string
	GetHasStatus() bool
	GetStatus() types.PaymentStatus
	GetProvider() types.ProviderType
	GetLimit() int32
	GetOffset() int32
}

type cancelPaymentRequest interface {
	GetId() uint64
	GetReason() string
}

type paymentRepository interface {
	Create(ctx context.Context, payment *entity.Payment) error
	Update(ctx context.Context, payment *entity.Payment) error
	FindByID(ctx context.Context, id uint64) (*entity.Payment, error)
	FindByCallerRequestID(ctx context.Context, callerService, requestID string) (*entity.Payment, error)
	FindByCallbackHash(ctx context.Context, provider int32, callbackHash string) (*entity.Payment, error)
	List(ctx context.Context, filter repository.PaymentFilter) ([]*entity.Payment, error)
	ListDueCallbackDispatch(ctx context.Context, now time.Time, limit int32) ([]*entity.Payment, error)
	ListExpiredPending(ctx context.Context, cutoff time.Time, limit int32) ([]*entity.Payment, error)
	ListForReconcile(ctx context.Context, before time.Time, limit int32) ([]*entity.Payment, error)
}

type paymentEventRepository interface {
	Create(ctx context.Context, event *entity.PaymentEvent) error
}

type paymentCallbackRepository interface {
	Create(ctx context.Context, callback *entity.PaymentCallback) error
}

type PaymentService struct {
	paymentRepo  paymentRepository
	eventRepo    paymentEventRepository
	callbackRepo paymentCallbackRepository
	providerReg  *provider.Registry
	paymentsCfg  config.PaymentsConfig
	appAPIKey    string
	callbackHTTP *http.Client
}

func NewPaymentService(
	paymentRepo paymentRepository,
	eventRepo paymentEventRepository,
	callbackRepo paymentCallbackRepository,
	providerReg *provider.Registry,
	paymentsCfg config.PaymentsConfig,
	appAPIKey string,
) *PaymentService {
	timeout := paymentsCfg.CallbackHTTPTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &PaymentService{
		paymentRepo:  paymentRepo,
		eventRepo:    eventRepo,
		callbackRepo: callbackRepo,
		providerReg:  providerReg,
		paymentsCfg:  paymentsCfg,
		appAPIKey:    strings.TrimSpace(appAPIKey),
		callbackHTTP: &http.Client{Timeout: timeout},
	}
}

func (s *PaymentService) CreatePayment(ctx context.Context, req createPaymentRequest) (*entity.Payment, error) {
	requestID := strings.TrimSpace(req.GetRequestId())
	callerService := strings.TrimSpace(req.GetCallerService())
	if requestID == "" || callerService == "" {
		return nil, ErrInvalidRequest
	}

	existing, err := s.paymentRepo.FindByCallerRequestID(ctx, callerService, requestID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	providerCode := req.GetProvider()
	if providerCode == types.ProviderType_PROVIDER_TYPE_UNSPECIFIED {
		providerCode = types.ProviderType_PROVIDER_TYPE_STRIPE
	}

	providerClient, err := s.providerReg.Get(int32(providerCode))
	if err != nil {
		if errors.Is(err, provider.ErrProviderNotSupported) {
			return nil, ErrProviderUnsupported
		}
		return nil, err
	}

	callbackHash := uuid.NewString()
	customerRef := normalizeOptionalString(req.GetCustomerRef())
	metadata := cloneMetadata(req.GetMetadata())

	providerOutput, err := providerClient.CreatePayment(ctx, &provider.CreateInput{
		RequestID:              requestID,
		CallbackHash:           callbackHash,
		ResourceType:           strings.TrimSpace(req.GetResourceType()),
		ResourceID:             strings.TrimSpace(req.GetResourceId()),
		AmountCents:            req.GetAmountCents(),
		Currency:               strings.ToUpper(strings.TrimSpace(req.GetCurrency())),
		PaymentMethod:          int32(req.GetPaymentMethod()),
		PaymentType:            int32(req.GetPaymentType()),
		RecurringInterval:      strings.ToLower(strings.TrimSpace(req.GetRecurringInterval())),
		RecurringIntervalCount: req.GetRecurringIntervalCount(),
		CustomerRef:            customerRef,
		Metadata:               metadata,
		SuccessURL:             strings.TrimSpace(req.GetSuccessUrl()),
		CancelURL:              strings.TrimSpace(req.GetCancelUrl()),
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	payment := &entity.Payment{
		RequestID:              requestID,
		CallerService:          callerService,
		ResourceType:           strings.TrimSpace(req.GetResourceType()),
		ResourceID:             strings.TrimSpace(req.GetResourceId()),
		CustomerRef:            customerRef,
		AmountCents:            req.GetAmountCents(),
		Currency:               strings.ToUpper(strings.TrimSpace(req.GetCurrency())),
		Status:                 providerOutput.InitialStatus,
		PaymentMethod:          int32(req.GetPaymentMethod()),
		PaymentType:            int32(req.GetPaymentType()),
		Provider:               int32(providerCode),
		RecurringInterval:      normalizeOptionalString(strings.ToLower(strings.TrimSpace(req.GetRecurringInterval()))),
		RecurringIntervalCount: normalizeOptionalInt32(req.GetRecurringIntervalCount()),
		ProviderPaymentID:      providerOutput.ProviderPaymentID,
		ProviderSubscriptionID: providerOutput.ProviderSubscriptionID,
		CheckoutURL:            providerOutput.CheckoutURL,
		ProviderCallbackHash:   callbackHash,
		ProviderCallbackURL:    providerOutput.ProviderCallbackURL,
		StatusCallbackURL:      strings.TrimSpace(req.GetStatusCallbackUrl()),
		RefundedCents:          0,
		RefundableCents:        req.GetAmountCents(),
		Metadata:               metadata,
		CallbackDeliveryStatus: entity.CallbackDeliveryNone,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	if terminalStatus(payment.Status) {
		s.markForCallbackDelivery(payment, now)
	}

	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		if errors.Is(err, repository.ErrPaymentAlreadyExists) {
			return nil, ErrPaymentAlreadyExists
		}
		return nil, err
	}

	_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
		PaymentID: payment.ID,
		EventType: "payment_created",
		NewStatus: payment.Status,
		CreatedAt: now,
	})

	return payment, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, id uint64) (*entity.Payment, error) {
	payment, err := s.paymentRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	return payment, nil
}

func (s *PaymentService) ListPayments(ctx context.Context, req listPaymentsRequest) ([]*entity.Payment, error) {
	limit := req.GetLimit()
	if limit <= 0 {
		limit = defaultListLimit
	}

	filter := repository.PaymentFilter{
		RequestID:     strings.TrimSpace(req.GetRequestId()),
		CallerService: strings.TrimSpace(req.GetCallerService()),
		ResourceType:  strings.TrimSpace(req.GetResourceType()),
		ResourceID:    strings.TrimSpace(req.GetResourceId()),
		HasStatus:     req.GetHasStatus(),
		Status:        int32(req.GetStatus()),
		Provider:      int32(req.GetProvider()),
		Limit:         limit,
		Offset:        req.GetOffset(),
	}

	return s.paymentRepo.List(ctx, filter)
}

func (s *PaymentService) CancelPayment(ctx context.Context, req cancelPaymentRequest) (*entity.Payment, error) {
	payment, err := s.paymentRepo.FindByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}

	if payment.Status == int32(types.PaymentStatus_PAYMENT_STATUS_PAID) {
		return nil, fmt.Errorf("%w: paid payments cannot be canceled", ErrInvalidStatus)
	}

	now := time.Now().UTC()
	oldStatus := payment.Status
	payment.Status = int32(types.PaymentStatus_PAYMENT_STATUS_CANCELED)
	s.markForCallbackDelivery(payment, now)
	payment.UpdatedAt = now

	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		if errors.Is(err, repository.ErrPaymentNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
		PaymentID: payment.ID,
		EventType: "payment_canceled",
		OldStatus: &oldStatus,
		NewStatus: payment.Status,
		CreatedAt: now,
	})

	return payment, nil
}

func (s *PaymentService) markForCallbackDelivery(payment *entity.Payment, now time.Time) {
	payment.CallbackDeliveryStatus = entity.CallbackDeliveryPending
	payment.CallbackDeliveryAttempts = 0
	payment.CallbackDeliveryNextAt = &now
	payment.CallbackDeliveryLastErr = nil
}

func (s *PaymentService) batchSize() int32 {
	if s.paymentsCfg.JobBatchSize > 0 {
		return s.paymentsCfg.JobBatchSize
	}
	return defaultBatchSize
}

func terminalStatus(status int32) bool {
	switch status {
	case int32(types.PaymentStatus_PAYMENT_STATUS_PAID),
		int32(types.PaymentStatus_PAYMENT_STATUS_FAILED),
		int32(types.PaymentStatus_PAYMENT_STATUS_CANCELED),
		int32(types.PaymentStatus_PAYMENT_STATUS_EXPIRED):
		return true
	default:
		return false
	}
}

func normalizeOptionalString(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeOptionalInt32(v int32) *int32 {
	if v <= 0 {
		return nil
	}
	n := v
	return &n
}

func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
