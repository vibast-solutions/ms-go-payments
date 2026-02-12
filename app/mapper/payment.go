package mapper

import (
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
	"github.com/vibast-solutions/ms-go-payments/app/types"
)

func PaymentToProto(item *entity.Payment) *types.Payment {
	if item == nil {
		return nil
	}

	return &types.Payment{
		Id:                      item.ID,
		RequestId:               item.RequestID,
		CallerService:           item.CallerService,
		ResourceType:            item.ResourceType,
		ResourceId:              item.ResourceID,
		CustomerRef:             derefString(item.CustomerRef),
		AmountCents:             item.AmountCents,
		Currency:                item.Currency,
		Status:                  types.PaymentStatus(item.Status),
		PaymentMethod:           types.PaymentMethod(item.PaymentMethod),
		PaymentType:             types.PaymentType(item.PaymentType),
		Provider:                types.ProviderType(item.Provider),
		RecurringInterval:       derefString(item.RecurringInterval),
		RecurringIntervalCount:  derefInt32(item.RecurringIntervalCount),
		ProviderPaymentId:       derefString(item.ProviderPaymentID),
		ProviderSubscriptionId:  derefString(item.ProviderSubscriptionID),
		CheckoutUrl:             derefString(item.CheckoutURL),
		ProviderCallbackHash:    item.ProviderCallbackHash,
		ProviderCallbackUrl:     item.ProviderCallbackURL,
		StatusCallbackUrl:       item.StatusCallbackURL,
		RefundedCents:           item.RefundedCents,
		RefundableCents:         item.RefundableCents,
		Metadata:                cloneMetadata(item.Metadata),
		CreatedAt:               item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:               item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func PaymentsToProto(items []*entity.Payment) []*types.Payment {
	result := make([]*types.Payment, 0, len(items))
	for _, item := range items {
		result = append(result, PaymentToProto(item))
	}
	return result
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefInt32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
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
