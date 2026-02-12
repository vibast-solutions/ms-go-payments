package entity

import "time"

const (
	CallbackDeliveryNone    int32 = 0
	CallbackDeliveryPending int32 = 1
	CallbackDeliverySuccess int32 = 10
	CallbackDeliveryFailed  int32 = 20
)

type Payment struct {
	ID uint64

	RequestID     string
	CallerService string

	ResourceType string
	ResourceID   string
	CustomerRef  *string

	AmountCents int64
	Currency    string

	Status        int32
	PaymentMethod int32
	PaymentType   int32
	Provider      int32

	RecurringInterval      *string
	RecurringIntervalCount *int32

	ProviderPaymentID      *string
	ProviderSubscriptionID *string
	CheckoutURL            *string

	ProviderCallbackHash string
	ProviderCallbackURL  string

	StatusCallbackURL string

	RefundedCents   int64
	RefundableCents int64

	Metadata map[string]string

	CallbackDeliveryStatus   int32
	CallbackDeliveryAttempts int32
	CallbackDeliveryNextAt   *time.Time
	CallbackDeliveryLastErr  *string

	CreatedAt time.Time
	UpdatedAt time.Time
}
