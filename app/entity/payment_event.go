package entity

import "time"

type PaymentEvent struct {
	ID uint64

	PaymentID uint64

	EventType string

	OldStatus *int32
	NewStatus int32

	ProviderEventID *string
	PayloadJSON     *string

	CreatedAt time.Time
}
