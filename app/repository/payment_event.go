package repository

import (
	"context"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
)

type PaymentEventRepository struct {
	db DBTX
}

func NewPaymentEventRepository(db DBTX) *PaymentEventRepository {
	return &PaymentEventRepository{db: db}
}

func (r *PaymentEventRepository) Create(ctx context.Context, event *entity.PaymentEvent) error {
	query := `
		INSERT INTO payment_events (
			payment_id, event_type, old_status, new_status, provider_event_id, payload_json, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		event.PaymentID,
		event.EventType,
		nullableInt32Value(event.OldStatus),
		event.NewStatus,
		nullableStringValue(event.ProviderEventID),
		nullableStringValue(event.PayloadJSON),
		event.CreatedAt,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	event.ID = uint64(id)

	return nil
}
