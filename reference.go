// reference.go — the return address: pulls the caller's reference and
// idempotency key off the request itself, so services do not have to annotate
// the context for them.

package wirelog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// resolveRef prefers an explicit context annotation and falls back to the
// request body, so a caller that sets WithRef always wins.
func resolveRef(ctx context.Context, reqBody []byte, fields []string) string {
	if ref := refFrom(ctx); ref != "" {
		return ref
	}
	return refFromBody(reqBody, fields)
}

// resolveIdempotencyKey prefers an explicit context annotation and falls back
// to the standard request headers.
func resolveIdempotencyKey(ctx context.Context, header http.Header) string {
	if key := idempotencyKeyFrom(ctx); key != "" {
		return key
	}
	return idempotencyKeyFromHeaders(header)
}

// defaultRefFields are the request body field names checked, in order, for the
// caller's own reference; extend per provider via WithExtraRefFields.
var defaultRefFields = []string{
	"reference", "merchant_transaction_id", "transaction_reference",
	"client_reference", "reference_id", "external_reference",
	"merchant_reference", "client_order_id", "order_reference",
}

// defaultIdempotencyHeaders are the request headers checked, in order, for an
// idempotency key.
var defaultIdempotencyHeaders = []string{
	"Idempotency-Key", "X-Idempotency-Key", "X-Anchor-Idempotent-Key",
}

// refFromBody returns the first matching field's string value from a JSON
// request body, or "" when the body is absent, unparseable, or has no match.
// Matching is canonicalised, so "merchantTransactionId" matches
// "merchant_transaction_id".
func refFromBody(body []byte, fields []string) string {
	if len(body) == 0 || len(fields) == 0 {
		return ""
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ""
	}
	canonical := make(map[string]any, len(decoded))
	for key, value := range decoded {
		canonical[canonicalFieldName(key)] = value
	}
	for _, field := range fields {
		value, ok := canonical[canonicalFieldName(field)]
		if !ok {
			continue
		}
		if text, isString := value.(string); isString && text != "" {
			return text
		}
	}
	return ""
}

// idempotencyKeyFromHeaders returns the first idempotency header present.
func idempotencyKeyFromHeaders(header http.Header) string {
	for _, name := range defaultIdempotencyHeaders {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

// canonicalFieldName lowercases and strips separators so snake_case, camelCase
// and kebab-case spellings of one name compare equal.
func canonicalFieldName(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}
