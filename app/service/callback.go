package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/provider"
	"github.com/vibast-solutions/ms-go-payments/app/repository"
	"github.com/vibast-solutions/ms-go-payments/app/types"
)

const (
	paymentCallbackStatusProcessed int32 = 10
	paymentCallbackStatusRejected  int32 = 20
)

type handleProviderCallbackRequest interface {
	GetProvider() string
	GetCallbackHash() string
	GetSignature() string
	GetPayload() string
}

func (s *PaymentService) HandleProviderCallback(ctx context.Context, req handleProviderCallbackRequest) (*entity.Payment, error) {
	providerCode, err := parseProviderCode(req.GetProvider())
	if err != nil {
		if errors.Is(err, provider.ErrProviderNotSupported) {
			return nil, ErrProviderUnsupported
		}
		return nil, err
	}

	providerClient, err := s.providerReg.Get(providerCode)
	if err != nil {
		if errors.Is(err, provider.ErrProviderNotSupported) {
			return nil, ErrProviderUnsupported
		}
		return nil, err
	}

	payload := []byte(req.GetPayload())
	signature := strings.TrimSpace(req.GetSignature())
	parsedEvent, err := providerClient.VerifyAndParseCallback(ctx, payload, signature)
	if err != nil {
		s.persistRejectedCallback(ctx, nil, req, fmt.Sprintf("provider callback validation failed: %v", err))
		return nil, ErrCallbackRejected
	}
	if parsedEvent == nil {
		s.persistRejectedCallback(ctx, nil, req, "provider callback payload could not be parsed")
		return nil, ErrCallbackRejected
	}

	callbackHash := strings.TrimSpace(req.GetCallbackHash())
	payment, err := s.paymentRepo.FindByCallbackHash(ctx, providerCode, callbackHash)
	if err != nil {
		return nil, err
	}
	if payment == nil {
		s.persistRejectedCallback(ctx, nil, req, "payment not found for callback hash")
		return nil, ErrPaymentNotFound
	}

	now := time.Now().UTC()
	oldStatus := payment.Status

	if parsedEvent.ProviderPaymentID != nil {
		payment.ProviderPaymentID = parsedEvent.ProviderPaymentID
	}
	if parsedEvent.ProviderSubscriptionID != nil {
		payment.ProviderSubscriptionID = parsedEvent.ProviderSubscriptionID
	}
	if parsedEvent.NewStatus > 0 {
		payment.Status = parsedEvent.NewStatus
	}

	if payment.Status != oldStatus && terminalStatus(payment.Status) {
		s.markForCallbackDelivery(payment, now)
	}

	payment.UpdatedAt = now
	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		if errors.Is(err, repository.ErrPaymentNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	eventType := strings.TrimSpace(parsedEvent.EventType)
	if eventType == "" {
		eventType = "provider_callback"
	}

	oldStatusPtr := &oldStatus
	if oldStatus == payment.Status {
		oldStatusPtr = nil
	}

	payloadJSON := string(payload)
	_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
		PaymentID:        payment.ID,
		EventType:        eventType,
		OldStatus:        oldStatusPtr,
		NewStatus:        payment.Status,
		ProviderEventID:  parsedEvent.ProviderEventID,
		PayloadJSON:      &payloadJSON,
		CreatedAt:        now,
	})

	paymentID := payment.ID
	callbackErr := s.callbackRepo.Create(ctx, &entity.PaymentCallback{
		PaymentID:     &paymentID,
		Provider:      strings.ToLower(strings.TrimSpace(req.GetProvider())),
		CallbackHash:  callbackHash,
		Signature:     signature,
		PayloadJSON:   string(payload),
		Status:        paymentCallbackStatusProcessed,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if callbackErr != nil {
		return nil, callbackErr
	}

	return payment, nil
}

func (s *PaymentService) persistRejectedCallback(
	ctx context.Context,
	paymentID *uint64,
	req handleProviderCallbackRequest,
	reason string,
) {
	now := time.Now().UTC()
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "callback rejected"
	}
	trimmedErr := truncate(reason, 1024)
	_ = s.callbackRepo.Create(ctx, &entity.PaymentCallback{
		PaymentID:    paymentID,
		Provider:     strings.ToLower(strings.TrimSpace(req.GetProvider())),
		CallbackHash: strings.TrimSpace(req.GetCallbackHash()),
		Signature:    strings.TrimSpace(req.GetSignature()),
		PayloadJSON:  req.GetPayload(),
		Status:       paymentCallbackStatusRejected,
		Error:        &trimmedErr,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func parseProviderCode(providerRaw string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(providerRaw)) {
	case "stripe", "1":
		return int32(types.ProviderType_PROVIDER_TYPE_STRIPE), nil
	default:
		return 0, provider.ErrProviderNotSupported
	}
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
