package wirelog

import (
	"context"
	"net/http"
	"testing"
)

// TestRefFromBody covers the shared field list and canonical matching.
func TestRefFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "magma merchant_transaction_id",
			body: `{"amount":555,"merchant_transaction_id":"BP_5204474f_1785153926"}`,
			want: "BP_5204474f_1785153926",
		},
		{
			name: "camelCase spelling still matches",
			body: `{"merchantTransactionId":"BP_camel"}`,
			want: "BP_camel",
		},
		{
			name: "plain reference",
			body: `{"reference":"REF-1","amount":10}`,
			want: "REF-1",
		},
		{
			name: "first field in list order wins",
			body: `{"client_reference":"second","reference":"first"}`,
			want: "first",
		},
		{
			name: "no matching field",
			body: `{"amount":10,"currency":"XOF"}`,
			want: "",
		},
		{
			name: "empty string value is not a reference",
			body: `{"reference":""}`,
			want: "",
		},
		{
			name: "non-string value ignored",
			body: `{"reference":12345}`,
			want: "",
		},
		{
			name: "unparseable body",
			body: `<html>nope</html>`,
			want: "",
		},
		{
			name: "empty body when capture is off",
			body: ``,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refFromBody([]byte(tt.body), defaultRefFields); got != tt.want {
				t.Errorf("refFromBody = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIdempotencyKeyFromHeaders covers the standard headers providers already send.
func TestIdempotencyKeyFromHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"standard", "Idempotency-Key", "idem-1"},
		{"x-prefixed", "X-Idempotency-Key", "idem-1"},
		{"anchor spelling", "X-Anchor-Idempotent-Key", "idem-1"},
		{"unrelated header", "X-Request-Id", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			h.Set(tt.header, "idem-1")
			if got := idempotencyKeyFromHeaders(h); got != tt.want {
				t.Errorf("idempotencyKeyFromHeaders = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolvePrefersContext checks an explicit annotation always beats
// inference, so existing callers do not change behaviour.
func TestResolvePrefersContext(t *testing.T) {
	body := []byte(`{"reference":"from-body"}`)
	header := http.Header{}
	header.Set("Idempotency-Key", "from-header")

	t.Run("context wins when set", func(t *testing.T) {
		ctx := WithRef(context.Background(), "from-ctx")
		ctx = WithIdempotencyKey(ctx, "idem-ctx")

		if got := resolveRef(ctx, body, defaultRefFields); got != "from-ctx" {
			t.Errorf("ref = %q, want the context annotation to win", got)
		}
		if got := resolveIdempotencyKey(ctx, header); got != "idem-ctx" {
			t.Errorf("idempotency = %q, want the context annotation to win", got)
		}
	})

	t.Run("falls back when context is empty", func(t *testing.T) {
		ctx := context.Background()

		if got := resolveRef(ctx, body, defaultRefFields); got != "from-body" {
			t.Errorf("ref = %q, want the body fallback", got)
		}
		if got := resolveIdempotencyKey(ctx, header); got != "from-header" {
			t.Errorf("idempotency = %q, want the header fallback", got)
		}
	})
}

// TestWithExtraRefFields checks a provider can add its own naming.
func TestWithExtraRefFields(t *testing.T) {
	cfg := NewConfig("odd", WithExtraRefFields("txnRef"))
	if got := refFromBody([]byte(`{"txnRef":"T-1"}`), cfg.RefFields); got != "T-1" {
		t.Errorf("refFromBody = %q, want T-1 from the provider's extra field", got)
	}
	// the shared list still applies
	if got := refFromBody([]byte(`{"reference":"R-1"}`), cfg.RefFields); got != "R-1" {
		t.Errorf("refFromBody = %q, want the shared list to still work", got)
	}
}

// TestCanonicalFieldName covers the spelling variants seen across providers.
func TestCanonicalFieldName(t *testing.T) {
	for _, name := range []string{
		"merchant_transaction_id", "merchantTransactionId",
		"Merchant-Transaction-Id", "MERCHANT_TRANSACTION_ID",
	} {
		if got := canonicalFieldName(name); got != "merchanttransactionid" {
			t.Errorf("canonicalFieldName(%q) = %q", name, got)
		}
	}
}
