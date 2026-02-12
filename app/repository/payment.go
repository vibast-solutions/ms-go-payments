package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/entity"
)

var (
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrPaymentAlreadyExists = errors.New("payment already exists")
)

type PaymentFilter struct {
	RequestID     string
	CallerService string
	ResourceType  string
	ResourceID    string
	HasStatus     bool
	Status        int32
	Provider      int32
	Limit         int32
	Offset        int32
}

type PaymentRepository struct {
	db DBTX
}

func NewPaymentRepository(db DBTX) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(ctx context.Context, payment *entity.Payment) error {
	metadataJSON, err := serializeMetadata(payment.Metadata)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO payments (
			request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		payment.RequestID,
		payment.CallerService,
		payment.ResourceType,
		payment.ResourceID,
		nullableStringValue(payment.CustomerRef),
		payment.AmountCents,
		payment.Currency,
		payment.Status,
		payment.PaymentMethod,
		payment.PaymentType,
		payment.Provider,
		nullableStringValue(payment.RecurringInterval),
		nullableInt32Value(payment.RecurringIntervalCount),
		nullableStringValue(payment.ProviderPaymentID),
		nullableStringValue(payment.ProviderSubscriptionID),
		nullableStringValue(payment.CheckoutURL),
		payment.ProviderCallbackHash,
		payment.ProviderCallbackURL,
		payment.StatusCallbackURL,
		payment.RefundedCents,
		payment.RefundableCents,
		metadataJSON,
		payment.CallbackDeliveryStatus,
		payment.CallbackDeliveryAttempts,
		nullableTimeValue(payment.CallbackDeliveryNextAt),
		nullableStringValue(payment.CallbackDeliveryLastErr),
		payment.CreatedAt,
		payment.UpdatedAt,
	)
	if err != nil {
		if isDuplicateEntryError(err) {
			return ErrPaymentAlreadyExists
		}
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	payment.ID = uint64(id)
	return nil
}

func (r *PaymentRepository) Update(ctx context.Context, payment *entity.Payment) error {
	metadataJSON, err := serializeMetadata(payment.Metadata)
	if err != nil {
		return err
	}

	query := `
		UPDATE payments SET
			resource_type = ?,
			resource_id = ?,
			customer_ref = ?,
			amount_cents = ?,
			currency = ?,
			status = ?,
			payment_method = ?,
			payment_type = ?,
			provider = ?,
			recurring_interval = ?,
			recurring_interval_count = ?,
			provider_payment_id = ?,
			provider_subscription_id = ?,
			checkout_url = ?,
			provider_callback_url = ?,
			status_callback_url = ?,
			refunded_cents = ?,
			refundable_cents = ?,
			metadata_json = ?,
			callback_delivery_status = ?,
			callback_delivery_attempts = ?,
			callback_delivery_next_at = ?,
			callback_delivery_last_error = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		payment.ResourceType,
		payment.ResourceID,
		nullableStringValue(payment.CustomerRef),
		payment.AmountCents,
		payment.Currency,
		payment.Status,
		payment.PaymentMethod,
		payment.PaymentType,
		payment.Provider,
		nullableStringValue(payment.RecurringInterval),
		nullableInt32Value(payment.RecurringIntervalCount),
		nullableStringValue(payment.ProviderPaymentID),
		nullableStringValue(payment.ProviderSubscriptionID),
		nullableStringValue(payment.CheckoutURL),
		payment.ProviderCallbackURL,
		payment.StatusCallbackURL,
		payment.RefundedCents,
		payment.RefundableCents,
		metadataJSON,
		payment.CallbackDeliveryStatus,
		payment.CallbackDeliveryAttempts,
		nullableTimeValue(payment.CallbackDeliveryNextAt),
		nullableStringValue(payment.CallbackDeliveryLastErr),
		payment.UpdatedAt,
		payment.ID,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPaymentNotFound
	}

	return nil
}

func (r *PaymentRepository) FindByID(ctx context.Context, id uint64) (*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
		WHERE id = ?
	`

	payment := &entity.Payment{}
	if err := scanPayment(r.db.QueryRowContext(ctx, query, id), payment); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return payment, nil
}

func (r *PaymentRepository) FindByCallerRequestID(ctx context.Context, callerService, requestID string) (*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
		WHERE caller_service = ? AND request_id = ?
		LIMIT 1
	`

	payment := &entity.Payment{}
	if err := scanPayment(r.db.QueryRowContext(ctx, query, callerService, requestID), payment); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return payment, nil
}

func (r *PaymentRepository) FindByCallbackHash(ctx context.Context, provider int32, callbackHash string) (*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
		WHERE provider = ? AND provider_callback_hash = ?
		LIMIT 1
	`

	payment := &entity.Payment{}
	if err := scanPayment(r.db.QueryRowContext(ctx, query, provider, callbackHash), payment); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return payment, nil
}

func (r *PaymentRepository) List(ctx context.Context, filter PaymentFilter) ([]*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
	`

	conditions := make([]string, 0, 6)
	args := make([]interface{}, 0, 8)

	if strings.TrimSpace(filter.RequestID) != "" {
		conditions = append(conditions, "request_id = ?")
		args = append(args, filter.RequestID)
	}
	if strings.TrimSpace(filter.CallerService) != "" {
		conditions = append(conditions, "caller_service = ?")
		args = append(args, filter.CallerService)
	}
	if strings.TrimSpace(filter.ResourceType) != "" {
		conditions = append(conditions, "resource_type = ?")
		args = append(args, filter.ResourceType)
	}
	if strings.TrimSpace(filter.ResourceID) != "" {
		conditions = append(conditions, "resource_id = ?")
		args = append(args, filter.ResourceID)
	}
	if filter.HasStatus {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Provider > 0 {
		conditions = append(conditions, "provider = ?")
		args = append(args, filter.Provider)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]*entity.Payment, 0)
	for rows.Next() {
		item, err := scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return payments, nil
}

func (r *PaymentRepository) ListDueCallbackDispatch(ctx context.Context, now time.Time, limit int32) ([]*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
		WHERE callback_delivery_status = ?
		  AND callback_delivery_next_at IS NOT NULL
		  AND callback_delivery_next_at <= ?
		ORDER BY callback_delivery_next_at ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, entity.CallbackDeliveryPending, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]*entity.Payment, 0)
	for rows.Next() {
		item, err := scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return payments, nil
}

func (r *PaymentRepository) ListExpiredPending(ctx context.Context, cutoff time.Time, limit int32) ([]*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
		WHERE status IN (?, ?)
		  AND created_at <= ?
		ORDER BY created_at ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, 2, 3, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]*entity.Payment, 0)
	for rows.Next() {
		item, err := scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return payments, nil
}

func (r *PaymentRepository) ListForReconcile(ctx context.Context, before time.Time, limit int32) ([]*entity.Payment, error) {
	query := `
		SELECT id, request_id, caller_service, resource_type, resource_id, customer_ref,
			amount_cents, currency, status, payment_method, payment_type, provider,
			recurring_interval, recurring_interval_count,
			provider_payment_id, provider_subscription_id, checkout_url,
			provider_callback_hash, provider_callback_url, status_callback_url,
			refunded_cents, refundable_cents, metadata_json,
			callback_delivery_status, callback_delivery_attempts, callback_delivery_next_at, callback_delivery_last_error,
			created_at, updated_at
		FROM payments
		WHERE status IN (?, ?)
		  AND provider_payment_id IS NOT NULL
		  AND updated_at <= ?
		ORDER BY updated_at ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, 2, 3, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]*entity.Payment, 0)
	for rows.Next() {
		item, err := scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return payments, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanPayment(scan rowScanner, payment *entity.Payment) error {
	var customerRef sql.NullString
	var recurringInterval sql.NullString
	var recurringIntervalCount sql.NullInt32
	var providerPaymentID sql.NullString
	var providerSubscriptionID sql.NullString
	var checkoutURL sql.NullString
	var metadataJSON string
	var callbackNextAt sql.NullTime
	var callbackLastErr sql.NullString

	err := scan.Scan(
		&payment.ID,
		&payment.RequestID,
		&payment.CallerService,
		&payment.ResourceType,
		&payment.ResourceID,
		&customerRef,
		&payment.AmountCents,
		&payment.Currency,
		&payment.Status,
		&payment.PaymentMethod,
		&payment.PaymentType,
		&payment.Provider,
		&recurringInterval,
		&recurringIntervalCount,
		&providerPaymentID,
		&providerSubscriptionID,
		&checkoutURL,
		&payment.ProviderCallbackHash,
		&payment.ProviderCallbackURL,
		&payment.StatusCallbackURL,
		&payment.RefundedCents,
		&payment.RefundableCents,
		&metadataJSON,
		&payment.CallbackDeliveryStatus,
		&payment.CallbackDeliveryAttempts,
		&callbackNextAt,
		&callbackLastErr,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	if err != nil {
		return err
	}

	payment.CustomerRef = stringPtrFromNull(customerRef)
	payment.RecurringInterval = stringPtrFromNull(recurringInterval)
	payment.RecurringIntervalCount = int32PtrFromNull(recurringIntervalCount)
	payment.ProviderPaymentID = stringPtrFromNull(providerPaymentID)
	payment.ProviderSubscriptionID = stringPtrFromNull(providerSubscriptionID)
	payment.CheckoutURL = stringPtrFromNull(checkoutURL)
	payment.CallbackDeliveryNextAt = timePtrFromNull(callbackNextAt)
	payment.CallbackDeliveryLastErr = stringPtrFromNull(callbackLastErr)

	metadata, err := parseMetadata(metadataJSON)
	if err != nil {
		return err
	}
	payment.Metadata = metadata

	return nil
}

func scanPaymentFromRows(rows *sql.Rows) (*entity.Payment, error) {
	item := &entity.Payment{}
	if err := scanPayment(rows, item); err != nil {
		return nil, err
	}
	return item, nil
}
