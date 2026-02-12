package provider

import "context"

type CreateInput struct {
	RequestID     string
	CallbackHash  string
	ResourceType  string
	ResourceID    string
	AmountCents   int64
	Currency      string
	PaymentMethod int32
	PaymentType   int32

	RecurringInterval      string
	RecurringIntervalCount int32

	CustomerRef *string
	Metadata    map[string]string

	SuccessURL string
	CancelURL  string
}

type CreateOutput struct {
	ProviderPaymentID      *string
	ProviderSubscriptionID *string
	CheckoutURL            *string
	ProviderCallbackURL    string
	InitialStatus          int32
}

type CallbackEvent struct {
	ProviderEventID        *string
	ProviderPaymentID      *string
	ProviderSubscriptionID *string
	EventType              string
	NewStatus              int32
}

type Provider interface {
	Code() int32
	CreatePayment(ctx context.Context, input *CreateInput) (*CreateOutput, error)
	VerifyAndParseCallback(ctx context.Context, payload []byte, signature string) (*CallbackEvent, error)
	GetPaymentStatus(ctx context.Context, providerPaymentID string) (int32, error)
}
