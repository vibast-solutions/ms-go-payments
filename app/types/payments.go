package types

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

func NewCreatePaymentRequestFromContext(ctx echo.Context) (*CreatePaymentRequest, error) {
	var body CreatePaymentRequest
	if err := ctx.Bind(&body); err != nil {
		return nil, err
	}

	body.RequestId = strings.TrimSpace(body.RequestId)
	if body.RequestId == "" {
		body.RequestId = strings.TrimSpace(ctx.Request().Header.Get(echo.HeaderXRequestID))
	}
	body.CallerService = strings.TrimSpace(body.CallerService)
	body.ResourceType = strings.TrimSpace(body.ResourceType)
	body.ResourceId = strings.TrimSpace(body.ResourceId)
	body.CustomerRef = strings.TrimSpace(body.CustomerRef)
	body.Currency = strings.ToUpper(strings.TrimSpace(body.Currency))
	body.RecurringInterval = strings.ToLower(strings.TrimSpace(body.RecurringInterval))
	body.StatusCallbackUrl = strings.TrimSpace(body.StatusCallbackUrl)
	body.SuccessUrl = strings.TrimSpace(body.SuccessUrl)
	body.CancelUrl = strings.TrimSpace(body.CancelUrl)

	return &body, nil
}

func (r *CreatePaymentRequest) Validate() error {
	if strings.TrimSpace(r.GetRequestId()) == "" {
		return errors.New("request_id is required")
	}
	if strings.TrimSpace(r.GetCallerService()) == "" {
		return errors.New("caller_service is required")
	}
	if strings.TrimSpace(r.GetResourceType()) == "" {
		return errors.New("resource_type is required")
	}
	if strings.TrimSpace(r.GetResourceId()) == "" {
		return errors.New("resource_id is required")
	}
	if r.GetAmountCents() <= 0 {
		return errors.New("amount_cents must be > 0")
	}
	if len(strings.TrimSpace(r.GetCurrency())) != 3 {
		return errors.New("currency must be 3 letters")
	}
	if r.GetPaymentMethod() != PaymentMethod_PAYMENT_METHOD_HOSTED_CARD && r.GetPaymentMethod() != PaymentMethod_PAYMENT_METHOD_PAYMENT_LINK {
		return errors.New("payment_method must be hosted_card or payment_link")
	}
	if r.GetPaymentType() != PaymentType_PAYMENT_TYPE_ONE_TIME && r.GetPaymentType() != PaymentType_PAYMENT_TYPE_RECURRING {
		return errors.New("payment_type must be one_time or recurring")
	}
	if r.GetProvider() != ProviderType_PROVIDER_TYPE_UNSPECIFIED && r.GetProvider() != ProviderType_PROVIDER_TYPE_STRIPE {
		return errors.New("provider is invalid")
	}
	if strings.TrimSpace(r.GetStatusCallbackUrl()) == "" {
		return errors.New("status_callback_url is required")
	}
	if r.GetPaymentType() == PaymentType_PAYMENT_TYPE_RECURRING {
		if r.GetRecurringInterval() != "day" && r.GetRecurringInterval() != "week" && r.GetRecurringInterval() != "month" && r.GetRecurringInterval() != "year" {
			return errors.New("recurring_interval must be day, week, month, or year")
		}
		if r.GetRecurringIntervalCount() <= 0 {
			return errors.New("recurring_interval_count must be > 0")
		}
	}

	return nil
}

func NewGetPaymentRequestFromContext(ctx echo.Context) (*GetPaymentRequest, error) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return nil, err
	}
	return &GetPaymentRequest{Id: id}, nil
}

func (r *GetPaymentRequest) Validate() error {
	if r.GetId() == 0 {
		return errors.New("invalid payment id")
	}
	return nil
}

func NewListPaymentsRequestFromContext(ctx echo.Context) (*ListPaymentsRequest, error) {
	req := &ListPaymentsRequest{
		RequestId:    strings.TrimSpace(ctx.QueryParam("request_id")),
		CallerService: strings.TrimSpace(ctx.QueryParam("caller_service")),
		ResourceType: strings.TrimSpace(ctx.QueryParam("resource_type")),
		ResourceId:   strings.TrimSpace(ctx.QueryParam("resource_id")),
		Limit:        100,
		Offset:       0,
	}

	statusRaw := strings.TrimSpace(ctx.QueryParam("status"))
	if statusRaw != "" {
		status, err := strconv.ParseInt(statusRaw, 10, 32)
		if err != nil {
			return nil, err
		}
		req.HasStatus = true
		req.Status = PaymentStatus(status)
	}

	providerRaw := strings.TrimSpace(strings.ToLower(ctx.QueryParam("provider")))
	if providerRaw != "" {
		switch providerRaw {
		case "1", "stripe":
			req.Provider = ProviderType_PROVIDER_TYPE_STRIPE
		default:
			return nil, errors.New("invalid provider")
		}
	}

	if limitRaw := strings.TrimSpace(ctx.QueryParam("limit")); limitRaw != "" {
		limit, err := strconv.ParseInt(limitRaw, 10, 32)
		if err != nil {
			return nil, err
		}
		req.Limit = int32(limit)
	}

	if offsetRaw := strings.TrimSpace(ctx.QueryParam("offset")); offsetRaw != "" {
		offset, err := strconv.ParseInt(offsetRaw, 10, 32)
		if err != nil {
			return nil, err
		}
		req.Offset = int32(offset)
	}

	return req, nil
}

func (r *ListPaymentsRequest) Validate() error {
	if r.Limit == 0 {
		r.Limit = 100
	}
	if r.GetLimit() <= 0 || r.GetLimit() > 500 {
		return errors.New("limit must be between 1 and 500")
	}
	if r.GetOffset() < 0 {
		return errors.New("offset must be >= 0")
	}
	if r.GetHasStatus() {
		if !isValidPaymentStatus(r.GetStatus()) {
			return errors.New("invalid status")
		}
	}
	if r.GetProvider() != ProviderType_PROVIDER_TYPE_UNSPECIFIED && r.GetProvider() != ProviderType_PROVIDER_TYPE_STRIPE {
		return errors.New("invalid provider")
	}
	return nil
}

func NewCancelPaymentRequestFromContext(ctx echo.Context) (*CancelPaymentRequest, error) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return nil, err
	}

	var body CancelPaymentRequest
	if err = ctx.Bind(&body); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	body.Id = id
	body.Reason = strings.TrimSpace(body.Reason)

	return &body, nil
}

func (r *CancelPaymentRequest) Validate() error {
	if r.GetId() == 0 {
		return errors.New("invalid payment id")
	}
	return nil
}

func NewHandleProviderCallbackRequestFromContext(ctx echo.Context) (*HandleProviderCallbackRequest, error) {
	provider := strings.TrimSpace(strings.ToLower(ctx.Param("provider")))
	hash := strings.TrimSpace(ctx.Param("hash"))
	requestID := strings.TrimSpace(ctx.Request().Header.Get(echo.HeaderXRequestID))
	signature := strings.TrimSpace(ctx.Request().Header.Get("Stripe-Signature"))
	if signature == "" {
		signature = strings.TrimSpace(ctx.Request().Header.Get("X-Provider-Signature"))
	}

	rawBody, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		return nil, err
	}

	req := &HandleProviderCallbackRequest{
		RequestId:    requestID,
		Provider:     provider,
		CallbackHash: hash,
		Signature:    signature,
		Payload:      string(rawBody),
	}

	var body struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	if len(rawBody) > 0 && json.Unmarshal(rawBody, &body) == nil {
		if strings.TrimSpace(body.Payload) != "" {
			req.Payload = body.Payload
		}
		if strings.TrimSpace(body.Signature) != "" {
			req.Signature = strings.TrimSpace(body.Signature)
		}
	}

	return req, nil
}

func (r *HandleProviderCallbackRequest) Validate() error {
	if strings.TrimSpace(r.GetRequestId()) == "" {
		return errors.New("request_id is required")
	}
	if strings.TrimSpace(r.GetProvider()) == "" {
		return errors.New("provider is required")
	}
	if strings.TrimSpace(r.GetCallbackHash()) == "" {
		return errors.New("callback hash is required")
	}
	if strings.TrimSpace(r.GetSignature()) == "" {
		return errors.New("provider signature is required")
	}
	if strings.TrimSpace(r.GetPayload()) == "" {
		return errors.New("payload is required")
	}
	return nil
}

func isValidPaymentStatus(status PaymentStatus) bool {
	switch status {
	case PaymentStatus_PAYMENT_STATUS_CREATED,
		PaymentStatus_PAYMENT_STATUS_PENDING,
		PaymentStatus_PAYMENT_STATUS_PROCESSING,
		PaymentStatus_PAYMENT_STATUS_PAID,
		PaymentStatus_PAYMENT_STATUS_FAILED,
		PaymentStatus_PAYMENT_STATUS_CANCELED,
		PaymentStatus_PAYMENT_STATUS_EXPIRED:
		return true
	default:
		return false
	}
}
