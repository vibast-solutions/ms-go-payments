package entity

import "time"

type PaymentCallback struct {
	ID uint64

	PaymentID *uint64

	Provider     string
	CallbackHash string
	Signature    string
	PayloadJSON  string
	Status       int32
	Error        *string

	CreatedAt time.Time
	UpdatedAt time.Time
}
