package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestVerifyStripeSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	secret := "whsec_test"
	ts := time.Now().Unix()
	signed := fmt.Sprintf("%d.%s", ts, string(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))
	header := fmt.Sprintf("t=%d,v1=%s", ts, sig)

	if !verifyStripeSignature(payload, header, secret, 300) {
		t.Fatal("expected signature to validate")
	}
	if verifyStripeSignature(payload, header, "wrong-secret", 300) {
		t.Fatal("expected signature with wrong secret to fail")
	}
}

func TestJoinCallbackURL(t *testing.T) {
	joined := joinCallbackURL("https://example.com/webhooks/providers/stripe/", "hash123")
	if joined != "https://example.com/webhooks/providers/stripe/hash123" {
		t.Fatalf("unexpected callback URL: %s", joined)
	}

	if joinCallbackURL("", "hash123") != "" {
		t.Fatal("expected empty callback URL when base URL is empty")
	}
}
