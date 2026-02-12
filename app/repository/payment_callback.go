package repository

import (
	"context"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
)

type PaymentCallbackRepository struct {
	db DBTX
}

func NewPaymentCallbackRepository(db DBTX) *PaymentCallbackRepository {
	return &PaymentCallbackRepository{db: db}
}

func (r *PaymentCallbackRepository) Create(ctx context.Context, callback *entity.PaymentCallback) error {
	query := `
		INSERT INTO payment_callbacks (
			payment_id, provider, callback_hash, signature, payload_json, status, error, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		callback.PaymentID,
		callback.Provider,
		callback.CallbackHash,
		callback.Signature,
		callback.PayloadJSON,
		callback.Status,
		nullableStringValue(callback.Error),
		callback.CreatedAt,
		callback.UpdatedAt,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	callback.ID = uint64(id)

	return nil
}
