package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/mapper"
	"github.com/vibast-solutions/ms-go-payments/app/types"
)

func (s *PaymentService) RunReconcileBatch(ctx context.Context) error {
	now := time.Now().UTC()
	before := now.Add(-s.paymentsCfg.ReconcileStaleAfter)
	items, err := s.paymentRepo.ListForReconcile(ctx, before, s.batchSize())
	if err != nil {
		return err
	}

	var firstErr error
	for _, payment := range items {
		if payment == nil || payment.ProviderPaymentID == nil || strings.TrimSpace(*payment.ProviderPaymentID) == "" {
			continue
		}

		providerClient, err := s.providerReg.Get(payment.Provider)
		if err != nil {
			firstErr = keepFirstErr(firstErr, err)
			continue
		}

		newStatus, err := providerClient.GetPaymentStatus(ctx, strings.TrimSpace(*payment.ProviderPaymentID))
		if err != nil {
			firstErr = keepFirstErr(firstErr, err)
			continue
		}
		if newStatus == 0 || newStatus == payment.Status {
			continue
		}

		oldStatus := payment.Status
		payment.Status = newStatus
		if terminalStatus(newStatus) {
			s.markForCallbackDelivery(payment, now)
		}
		payment.UpdatedAt = now

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			firstErr = keepFirstErr(firstErr, err)
			continue
		}

		_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
			PaymentID: payment.ID,
			EventType: "payment_reconciled",
			OldStatus: &oldStatus,
			NewStatus: newStatus,
			CreatedAt: now,
		})
	}

	return firstErr
}

func (s *PaymentService) RunDispatchCallbacksBatch(ctx context.Context) error {
	now := time.Now().UTC()
	items, err := s.paymentRepo.ListDueCallbackDispatch(ctx, now, s.batchSize())
	if err != nil {
		return err
	}

	var firstErr error
	for _, payment := range items {
		if payment == nil {
			continue
		}
		if err := s.dispatchCallback(ctx, payment, now); err != nil {
			firstErr = keepFirstErr(firstErr, err)
		}
	}

	return firstErr
}

func (s *PaymentService) RunExpirePendingBatch(ctx context.Context) error {
	now := time.Now().UTC()
	cutoff := now.Add(-s.paymentsCfg.PendingTimeout)
	items, err := s.paymentRepo.ListExpiredPending(ctx, cutoff, s.batchSize())
	if err != nil {
		return err
	}

	var firstErr error
	for _, payment := range items {
		if payment == nil {
			continue
		}
		if payment.Status == int32(types.PaymentStatus_PAYMENT_STATUS_EXPIRED) {
			continue
		}

		oldStatus := payment.Status
		payment.Status = int32(types.PaymentStatus_PAYMENT_STATUS_EXPIRED)
		s.markForCallbackDelivery(payment, now)
		payment.UpdatedAt = now

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			firstErr = keepFirstErr(firstErr, err)
			continue
		}

		_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
			PaymentID: payment.ID,
			EventType: "payment_expired",
			OldStatus: &oldStatus,
			NewStatus: payment.Status,
			CreatedAt: now,
		})
	}

	return firstErr
}

func (s *PaymentService) dispatchCallback(ctx context.Context, payment *entity.Payment, now time.Time) error {
	if strings.TrimSpace(payment.StatusCallbackURL) == "" {
		errMsg := "status_callback_url is empty"
		payment.CallbackDeliveryStatus = entity.CallbackDeliveryFailed
		payment.CallbackDeliveryNextAt = nil
		payment.CallbackDeliveryLastErr = &errMsg
		payment.UpdatedAt = now
		return s.paymentRepo.Update(ctx, payment)
	}

	payload := &types.PaymentEnvelopeResponse{Payment: mapper.PaymentToProto(payment)}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payment.StatusCallbackURL, bytes.NewReader(body))
	if err != nil {
		return s.recordDispatchFailure(ctx, payment, now, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", payment.RequestID)
	if s.appAPIKey != "" {
		req.Header.Set("X-API-Key", s.appAPIKey)
	}

	resp, err := s.callbackHTTP.Do(req)
	if err != nil {
		return s.recordDispatchFailure(ctx, payment, now, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.recordDispatchFailure(ctx, payment, now, fmt.Errorf("callback endpoint returned status=%d", resp.StatusCode))
	}

	payment.CallbackDeliveryStatus = entity.CallbackDeliverySuccess
	payment.CallbackDeliveryNextAt = nil
	payment.CallbackDeliveryLastErr = nil
	payment.UpdatedAt = now

	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		return err
	}

	_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
		PaymentID: payment.ID,
		EventType: "callback_dispatched",
		NewStatus: payment.Status,
		CreatedAt: now,
	})

	return nil
}

func (s *PaymentService) recordDispatchFailure(ctx context.Context, payment *entity.Payment, now time.Time, dispatchErr error) error {
	payment.CallbackDeliveryAttempts++
	trimmed := truncate(dispatchErr.Error(), 1024)
	payment.CallbackDeliveryLastErr = &trimmed

	maxAttempts := s.paymentsCfg.CallbackMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	if payment.CallbackDeliveryAttempts >= maxAttempts {
		payment.CallbackDeliveryStatus = entity.CallbackDeliveryFailed
		payment.CallbackDeliveryNextAt = nil
	} else {
		retryInterval := s.paymentsCfg.CallbackRetryInterval
		if retryInterval <= 0 {
			retryInterval = 5 * time.Minute
		}
		next := now.Add(retryInterval)
		payment.CallbackDeliveryStatus = entity.CallbackDeliveryPending
		payment.CallbackDeliveryNextAt = &next
	}
	payment.UpdatedAt = now

	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		return err
	}

	_ = s.eventRepo.Create(ctx, &entity.PaymentEvent{
		PaymentID: payment.ID,
		EventType: "callback_dispatch_failed",
		NewStatus: payment.Status,
		CreatedAt: now,
	})

	return dispatchErr
}

func keepFirstErr(current error, candidate error) error {
	if current != nil {
		return current
	}
	return candidate
}
