package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vibast-solutions/ms-go-payments/app/types"
)

type StripeConfig struct {
	SecretKey                 string
	WebhookSecret             string
	ProviderCallbackBaseURL   string
	SignatureToleranceSeconds int64
	HTTPTimeout               time.Duration
}

type StripeProvider struct {
	cfg    StripeConfig
	client *http.Client
}

func NewStripeProvider(cfg StripeConfig) *StripeProvider {
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tolerance := cfg.SignatureToleranceSeconds
	if tolerance <= 0 {
		tolerance = 300
	}
	cfg.SignatureToleranceSeconds = tolerance

	return &StripeProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *StripeProvider) Code() int32 {
	return int32(types.ProviderType_PROVIDER_TYPE_STRIPE)
}

func (p *StripeProvider) CreatePayment(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	if strings.TrimSpace(p.cfg.SecretKey) == "" {
		return nil, errors.New("stripe secret key is not configured")
	}

	callbackURL := joinCallbackURL(p.cfg.ProviderCallbackBaseURL, input.CallbackHash)
	if callbackURL == "" {
		return nil, errors.New("provider callback base url is not configured")
	}

	switch input.PaymentMethod {
	case int32(types.PaymentMethod_PAYMENT_METHOD_HOSTED_CARD):
		return p.createCheckoutSession(ctx, input, callbackURL)
	case int32(types.PaymentMethod_PAYMENT_METHOD_PAYMENT_LINK):
		return p.createPaymentLink(ctx, input, callbackURL)
	default:
		return nil, errors.New("unsupported payment method for stripe")
	}
}

func (p *StripeProvider) GetPaymentStatus(ctx context.Context, providerPaymentID string) (int32, error) {
	if strings.TrimSpace(providerPaymentID) == "" {
		return 0, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stripe.com/v1/checkout/sessions/"+url.PathEscape(providerPaymentID), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.SecretKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("stripe get checkout session failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload struct {
		Status        string `json:"status"`
		PaymentStatus string `json:"payment_status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, err
	}

	switch payload.Status {
	case "expired":
		return int32(types.PaymentStatus_PAYMENT_STATUS_EXPIRED), nil
	}

	switch payload.PaymentStatus {
	case "paid", "no_payment_required":
		return int32(types.PaymentStatus_PAYMENT_STATUS_PAID), nil
	case "unpaid":
		return int32(types.PaymentStatus_PAYMENT_STATUS_PENDING), nil
	default:
		return 0, nil
	}
}

func (p *StripeProvider) VerifyAndParseCallback(_ context.Context, payload []byte, signature string) (*CallbackEvent, error) {
	if strings.TrimSpace(p.cfg.WebhookSecret) == "" {
		return nil, errors.New("stripe webhook secret is not configured")
	}
	if !verifyStripeSignature(payload, signature, p.cfg.WebhookSecret, p.cfg.SignatureToleranceSeconds) {
		return nil, errors.New("invalid stripe signature")
	}

	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}

	result := &CallbackEvent{
		EventType: event.Type,
	}
	if strings.TrimSpace(event.ID) != "" {
		eventID := strings.TrimSpace(event.ID)
		result.ProviderEventID = &eventID
	}

	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		result.NewStatus = int32(types.PaymentStatus_PAYMENT_STATUS_PAID)
		assignCheckoutSessionFields(result, event.Data.Object)
	case "checkout.session.async_payment_failed":
		result.NewStatus = int32(types.PaymentStatus_PAYMENT_STATUS_FAILED)
		assignCheckoutSessionFields(result, event.Data.Object)
	case "checkout.session.expired":
		result.NewStatus = int32(types.PaymentStatus_PAYMENT_STATUS_EXPIRED)
		assignCheckoutSessionFields(result, event.Data.Object)
	case "invoice.paid":
		result.NewStatus = int32(types.PaymentStatus_PAYMENT_STATUS_PAID)
		assignInvoiceFields(result, event.Data.Object)
	case "invoice.payment_failed":
		result.NewStatus = int32(types.PaymentStatus_PAYMENT_STATUS_FAILED)
		assignInvoiceFields(result, event.Data.Object)
	case "customer.subscription.deleted":
		result.NewStatus = int32(types.PaymentStatus_PAYMENT_STATUS_CANCELED)
		assignSubscriptionFields(result, event.Data.Object)
	default:
		result.NewStatus = 0
	}

	return result, nil
}

func (p *StripeProvider) createCheckoutSession(ctx context.Context, input *CreateInput, callbackURL string) (*CreateOutput, error) {
	values := url.Values{}
	values.Set("line_items[0][quantity]", "1")
	values.Set("line_items[0][price_data][currency]", strings.ToLower(input.Currency))
	values.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(input.AmountCents, 10))
	values.Set("line_items[0][price_data][product_data][name]", buildProductName(input))

	if input.PaymentType == int32(types.PaymentType_PAYMENT_TYPE_RECURRING) {
		values.Set("mode", "subscription")
		values.Set("line_items[0][price_data][recurring][interval]", input.RecurringInterval)
		values.Set("line_items[0][price_data][recurring][interval_count]", strconv.FormatInt(int64(input.RecurringIntervalCount), 10))
	} else {
		values.Set("mode", "payment")
	}

	successURL := strings.TrimSpace(input.SuccessURL)
	cancelURL := strings.TrimSpace(input.CancelURL)
	if successURL == "" {
		successURL = callbackURL + "?state=success"
	}
	if cancelURL == "" {
		cancelURL = callbackURL + "?state=cancel"
	}
	values.Set("success_url", successURL)
	values.Set("cancel_url", cancelURL)
	values.Set("client_reference_id", input.RequestID)

	for k, v := range input.Metadata {
		values.Set("metadata["+k+"]", v)
	}
	values.Set("metadata[request_id]", input.RequestID)
	values.Set("metadata[callback_hash]", input.CallbackHash)

	body, err := p.postForm(ctx, "/v1/checkout/sessions", values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		ID           string      `json:"id"`
		URL          string      `json:"url"`
		Subscription interface{} `json:"subscription"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	result := &CreateOutput{
		ProviderCallbackURL: callbackURL,
		InitialStatus:       int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
	}
	if s := strings.TrimSpace(payload.ID); s != "" {
		result.ProviderPaymentID = &s
	}
	if s := strings.TrimSpace(payload.URL); s != "" {
		result.CheckoutURL = &s
	}
	if s := parseStringish(payload.Subscription); s != "" {
		result.ProviderSubscriptionID = &s
	}

	return result, nil
}

func (p *StripeProvider) createPaymentLink(ctx context.Context, input *CreateInput, callbackURL string) (*CreateOutput, error) {
	productValues := url.Values{}
	productValues.Set("name", buildProductName(input))
	productResp, err := p.postForm(ctx, "/v1/products", productValues)
	if err != nil {
		return nil, err
	}
	var product struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(productResp, &product); err != nil {
		return nil, err
	}
	productID := strings.TrimSpace(product.ID)
	if productID == "" {
		return nil, errors.New("stripe product id missing")
	}

	priceValues := url.Values{}
	priceValues.Set("currency", strings.ToLower(input.Currency))
	priceValues.Set("unit_amount", strconv.FormatInt(input.AmountCents, 10))
	priceValues.Set("product", productID)
	if input.PaymentType == int32(types.PaymentType_PAYMENT_TYPE_RECURRING) {
		priceValues.Set("recurring[interval]", input.RecurringInterval)
		priceValues.Set("recurring[interval_count]", strconv.FormatInt(int64(input.RecurringIntervalCount), 10))
	}
	priceResp, err := p.postForm(ctx, "/v1/prices", priceValues)
	if err != nil {
		return nil, err
	}
	var price struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(priceResp, &price); err != nil {
		return nil, err
	}
	priceID := strings.TrimSpace(price.ID)
	if priceID == "" {
		return nil, errors.New("stripe price id missing")
	}

	linkValues := url.Values{}
	linkValues.Set("line_items[0][price]", priceID)
	linkValues.Set("line_items[0][quantity]", "1")
	linkValues.Set("after_completion[type]", "redirect")
	linkValues.Set("after_completion[redirect][url]", callbackURL)
	for k, v := range input.Metadata {
		linkValues.Set("metadata["+k+"]", v)
	}
	linkValues.Set("metadata[request_id]", input.RequestID)
	linkValues.Set("metadata[callback_hash]", input.CallbackHash)

	linkResp, err := p.postForm(ctx, "/v1/payment_links", linkValues)
	if err != nil {
		return nil, err
	}
	var link struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(linkResp, &link); err != nil {
		return nil, err
	}

	result := &CreateOutput{
		ProviderCallbackURL: callbackURL,
		InitialStatus:       int32(types.PaymentStatus_PAYMENT_STATUS_PENDING),
	}
	if s := strings.TrimSpace(link.ID); s != "" {
		result.ProviderPaymentID = &s
	}
	if s := strings.TrimSpace(link.URL); s != "" {
		result.CheckoutURL = &s
	}

	return result, nil
}

func (p *StripeProvider) postForm(ctx context.Context, path string, values url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com"+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe request failed: path=%s status=%d body=%s", path, resp.StatusCode, string(body))
	}

	return body, nil
}

func buildProductName(input *CreateInput) string {
	name := strings.TrimSpace(input.ResourceType) + "-" + strings.TrimSpace(input.ResourceID)
	name = strings.TrimSpace(name)
	if name == "-" || name == "" {
		return "payment"
	}
	return name
}

func joinCallbackURL(baseURL, callbackHash string) string {
	baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	callbackHash = strings.TrimSpace(callbackHash)
	if baseURL == "" || callbackHash == "" {
		return ""
	}
	return baseURL + "/" + callbackHash
}

func verifyStripeSignature(payload []byte, signatureHeader string, webhookSecret string, toleranceSeconds int64) bool {
	signatureHeader = strings.TrimSpace(signatureHeader)
	if signatureHeader == "" || strings.TrimSpace(webhookSecret) == "" {
		return false
	}

	parts := strings.Split(signatureHeader, ",")
	var ts string
	v1 := make([]string, 0, 1)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "t=") {
			ts = strings.TrimSpace(strings.TrimPrefix(part, "t="))
		}
		if strings.HasPrefix(part, "v1=") {
			v1 = append(v1, strings.TrimSpace(strings.TrimPrefix(part, "v1=")))
		}
	}
	if ts == "" || len(v1) == 0 {
		return false
	}

	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	now := time.Now().Unix()
	if now-tsUnix > toleranceSeconds || tsUnix-now > toleranceSeconds {
		return false
	}

	signedPayload := []byte(ts + "." + string(payload))
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(signedPayload)
	expected := mac.Sum(nil)

	for _, sig := range v1 {
		candidate, err := hex.DecodeString(sig)
		if err != nil {
			continue
		}
		if hmac.Equal(candidate, expected) {
			return true
		}
	}

	return false
}

func assignCheckoutSessionFields(event *CallbackEvent, payload json.RawMessage) {
	var object struct {
		ID           string      `json:"id"`
		Subscription interface{} `json:"subscription"`
	}
	if json.Unmarshal(payload, &object) != nil {
		return
	}
	if s := strings.TrimSpace(object.ID); s != "" {
		event.ProviderPaymentID = &s
	}
	if s := parseStringish(object.Subscription); s != "" {
		event.ProviderSubscriptionID = &s
	}
}

func assignInvoiceFields(event *CallbackEvent, payload json.RawMessage) {
	var object struct {
		ID           string      `json:"id"`
		Subscription interface{} `json:"subscription"`
	}
	if json.Unmarshal(payload, &object) != nil {
		return
	}
	if s := strings.TrimSpace(object.ID); s != "" {
		event.ProviderPaymentID = &s
	}
	if s := parseStringish(object.Subscription); s != "" {
		event.ProviderSubscriptionID = &s
	}
}

func assignSubscriptionFields(event *CallbackEvent, payload json.RawMessage) {
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(payload, &object) != nil {
		return
	}
	if s := strings.TrimSpace(object.ID); s != "" {
		event.ProviderSubscriptionID = &s
	}
}

func parseStringish(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]interface{}:
		if raw, ok := t["id"]; ok {
			if s, ok := raw.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	case json.RawMessage:
		if len(t) == 0 {
			return ""
		}
		if t[0] == '"' {
			var s string
			if json.Unmarshal(t, &s) == nil {
				return strings.TrimSpace(s)
			}
		}
		var obj map[string]interface{}
		if json.Unmarshal(t, &obj) == nil {
			if raw, ok := obj["id"]; ok {
				if s, ok := raw.(string); ok {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func BuildForwardPayload(payload []byte, signature string) []byte {
	message := map[string]string{
		"payload":   string(payload),
		"signature": signature,
	}
	encoded, _ := json.Marshal(message)
	return bytes.TrimSpace(encoded)
}
