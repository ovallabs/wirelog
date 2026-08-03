package wirelog

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestClassifyInbound tables the inbound outcome mapping: 2xx/3xx success,
// 4xx client_error, 5xx server_error, and sub-200 (incl. 0) server_error.
func TestClassifyInbound(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{200, outcomeSuccess},
		{204, outcomeSuccess},
		{301, outcomeSuccess},
		{400, outcomeClientError},
		{404, outcomeClientError},
		{422, outcomeClientError},
		{499, outcomeClientError},
		{500, outcomeServerError},
		{503, outcomeServerError},
		{100, outcomeServerError},
		{0, outcomeServerError},
	}
	for _, tt := range tests {
		if got := classifyInbound(tt.status); got != tt.want {
			t.Errorf("classifyInbound(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// newInboundExchange builds a representative captured request for the record tests.
func newInboundExchange(ctx context.Context) InboundExchange {
	reqHeaders := http.Header{
		"Authorization": {"Bearer secret"},
		"Content-Type":  {"application/json"},
		"X-Trace":       {"t-1"},
	}
	respHeaders := http.Header{
		"Set-Cookie":   {"s=1"},
		"Content-Type": {"application/json"},
	}
	return InboundExchange{
		Ctx:             ctx,
		Method:          http.MethodPost,
		Route:           "/v1/wallet/:id/withdraw",
		Path:            "/v1/wallet/8f3a/withdraw",
		StatusCode:      201,
		Latency:         1500 * time.Millisecond,
		RequestHeaders:  reqHeaders,
		ResponseHeaders: respHeaders,
		RequestBody:     []byte(`{"pin":"1234","amount":100}`),
		ResponseBody:    []byte(`{"account_number":"0123456789","status":"ok"}`),
		RequestSize:     512,
		ResponseSize:    256,
		RemoteIP:        "10.0.0.7",
	}
}

// TestBuildInboundRecordFields verifies the full inbound column mapping with
// bodies on: service/consumer/operation/endpoint, masked headers and bodies,
// sizes, ref and idempotency resolution.
func TestBuildInboundRecordFields(t *testing.T) {
	c := newCapture(NewConfig("zobo-be", WithCaptureBodies(true), WithExtraMaskFields("pin", "account_number")), "ios")
	ctx := WithOperation(context.Background(), "wallet.withdraw")
	ctx = WithRef(ctx, "user-42")
	ctx = WithIdempotencyKey(ctx, "idem-1")

	rec := c.buildInboundRecord(newInboundExchange(ctx))

	if rec.provider != "zobo-be" {
		t.Errorf("provider = %q, want zobo-be", rec.provider)
	}
	if rec.consumer != "ios" {
		t.Errorf("consumer = %q, want ios (instance default)", rec.consumer)
	}
	if rec.operation != "wallet.withdraw" {
		t.Errorf("operation = %q, want wallet.withdraw", rec.operation)
	}
	if rec.endpoint != "/v1/wallet/:id/withdraw" {
		t.Errorf("endpoint = %q, want route template", rec.endpoint)
	}
	if rec.path != "/v1/wallet/8f3a/withdraw" {
		t.Errorf("path = %q, want raw path", rec.path)
	}
	if rec.method != http.MethodPost {
		t.Errorf("method = %q, want POST", rec.method)
	}
	if rec.statusCode != 201 || rec.outcome != outcomeSuccess {
		t.Errorf("status/outcome = %d/%q, want 201/success", rec.statusCode, rec.outcome)
	}
	if rec.latencyMS != 1500 {
		t.Errorf("latencyMS = %d, want 1500", rec.latencyMS)
	}
	if rec.requestSize != 512 || rec.responseSize != 256 {
		t.Errorf("sizes = %d/%d, want 512/256", rec.requestSize, rec.responseSize)
	}
	if rec.internalRef != "user-42" {
		t.Errorf("internalRef = %q, want user-42", rec.internalRef)
	}
	if rec.idempotencyKey != "idem-1" {
		t.Errorf("idempotencyKey = %q, want idem-1", rec.idempotencyKey)
	}
	if rec.requestHeaders["Authorization"] != maskedValue {
		t.Errorf("Authorization = %q, want masked", rec.requestHeaders["Authorization"])
	}
	if rec.responseHeaders["Set-Cookie"] != maskedValue {
		t.Errorf("Set-Cookie = %q, want masked", rec.responseHeaders["Set-Cookie"])
	}
	if rec.requestHeaders["X-Trace"] != "t-1" {
		t.Errorf("X-Trace = %q, want passthrough t-1", rec.requestHeaders["X-Trace"])
	}
	if got := string(rec.requestBody); !strings.Contains(got, `"pin":"`+maskedValue+`"`) {
		t.Errorf("request body pin not masked: %s", got)
	}
	if got := string(rec.responseBody); !strings.Contains(got, `"account_number":"`+maskedValue+`"`) {
		t.Errorf("response body account_number not masked: %s", got)
	}
}

// TestBuildInboundRecordError carries an explicit failure reason into the error
// column, e.g. a handler error on a 5xx.
func TestBuildInboundRecordError(t *testing.T) {
	c := newCapture(NewConfig("zobo-be"), "ios")
	x := newInboundExchange(context.Background())
	x.StatusCode = 500
	x.Error = "wallet: insufficient float"

	rec := c.buildInboundRecord(x)
	if rec.outcome != outcomeServerError {
		t.Errorf("outcome = %q, want server_error", rec.outcome)
	}
	if rec.callErr != "wallet: insufficient float" {
		t.Errorf("callErr = %q, want the handler error", rec.callErr)
	}
}

// TestBuildInboundRecordMetadataOnly is the privacy default: bodies off records
// sizes and masked headers but never body content.
func TestBuildInboundRecordMetadataOnly(t *testing.T) {
	c := newCapture(NewConfig("zobo-be"), "ios") // CaptureBodies defaults false
	rec := c.buildInboundRecord(newInboundExchange(context.Background()))

	if rec.requestBody != nil || rec.responseBody != nil {
		t.Error("bodies captured despite metadata-only default")
	}
	if rec.requestSize != 512 || rec.responseSize != 256 {
		t.Errorf("sizes = %d/%d, want 512/256 recorded with bodies off", rec.requestSize, rec.responseSize)
	}
	if rec.requestHeaders["Authorization"] != maskedValue {
		t.Error("headers must still be masked with bodies off")
	}
}

// TestBuildInboundRecordSkipBodyPaths records metadata and sizes but no body on
// a skip-listed path, even with capture on.
func TestBuildInboundRecordSkipBodyPaths(t *testing.T) {
	c := newCapture(NewConfig("zobo-be", WithCaptureBodies(true), WithExtraSkipBodyPaths("/auth")), "ios")
	x := newInboundExchange(context.Background())
	x.Route = "/v1/auth/login"
	x.Path = "/v1/auth/login"

	rec := c.buildInboundRecord(x)
	if rec.requestBody != nil || rec.responseBody != nil {
		t.Error("body captured on skip-listed path")
	}
	if rec.requestSize != 512 || rec.responseSize != 256 {
		t.Error("sizes must still be recorded on skip-listed path")
	}
}

// TestInboundEndpointFallback normalizes the raw path when no route template is set.
func TestInboundEndpointFallback(t *testing.T) {
	c := newCapture(NewConfig("zobo-be"), "")
	x := InboundExchange{Path: "/users/123"}
	if got := c.inboundEndpoint(x); got != "/users/{id}" {
		t.Errorf("inboundEndpoint fallback = %q, want /users/{id}", got)
	}
}

// TestInboundSizeFallback falls back to body length when no explicit size is given.
func TestInboundSizeFallback(t *testing.T) {
	if got := inboundSize(0, []byte("abcd")); got != 4 {
		t.Errorf("inboundSize(0, 4 bytes) = %d, want 4", got)
	}
	if got := inboundSize(99, []byte("abcd")); got != 99 {
		t.Errorf("inboundSize(99, ...) = %d, want explicit 99", got)
	}
}

// TestInboundCapturerExcludePaths short-circuits with no record enqueued.
func TestInboundCapturerExcludePaths(t *testing.T) {
	ch := make(chan record, 1)
	var dropped atomic.Int64
	cap := &InboundCapturer{
		capture: newCapture(NewConfig("zobo-be"), "ios"),
		ch:      ch, dropped: &dropped,
	}
	cap.capture.cfg.ExcludePaths = []string{"/health"}

	x := newInboundExchange(context.Background())
	x.Path = "/health"
	cap.Log(x)

	if len(ch) != 0 {
		t.Error("excluded path produced a record")
	}
	if dropped.Load() != 0 {
		t.Error("excluded path counted as a drop; it should produce no record at all")
	}
}

// TestInboundCapturerLogEnqueues puts a record on the channel for a normal request.
func TestInboundCapturerLogEnqueues(t *testing.T) {
	ch := make(chan record, 1)
	var dropped atomic.Int64
	cap := &InboundCapturer{capture: newCapture(NewConfig("zobo-be"), "ios"), ch: ch, dropped: &dropped}

	cap.Log(newInboundExchange(context.Background()))
	select {
	case rec := <-ch:
		if rec.provider != "zobo-be" || rec.outcome != outcomeSuccess {
			t.Errorf("enqueued record = %+v, want zobo-be/success", rec)
		}
	default:
		t.Fatal("no record enqueued")
	}
}

// TestInboundCapturerNonBlockingEnqueue drops and counts when the buffer is full
// rather than blocking the request.
func TestInboundCapturerNonBlockingEnqueue(t *testing.T) {
	ch := make(chan record) // unbuffered, no reader: every send would block
	var dropped atomic.Int64
	cap := &InboundCapturer{capture: newCapture(NewConfig("zobo-be"), "ios"), ch: ch, dropped: &dropped}

	done := make(chan struct{})
	go func() {
		cap.Log(newInboundExchange(context.Background()))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Log blocked on a full buffer")
	}
	if dropped.Load() != 1 {
		t.Errorf("Dropped = %d, want 1", dropped.Load())
	}
}

// TestInboundCapturerNilSafe confirms a capturer minted from a nil *Wirelog is a
// no-op that never panics.
func TestInboundCapturerNilSafe(t *testing.T) {
	var wl *Wirelog
	cap := wl.InboundCapturer(NewConfig("zobo-be"))
	// must not panic
	cap.Log(newInboundExchange(context.Background()))
}

// TestBuildInboundRecordDoesNotMutateHeaders confirms masking copies and never
// writes back to the caller's header maps.
func TestBuildInboundRecordDoesNotMutateHeaders(t *testing.T) {
	c := newCapture(NewConfig("zobo-be"), "ios")
	x := newInboundExchange(context.Background())
	_ = c.buildInboundRecord(x)
	if x.RequestHeaders.Get("Authorization") != "Bearer secret" {
		t.Error("source request header mutated by masking")
	}
}
