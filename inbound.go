// inbound.go — the receiving dock: records each request arriving from the app,
// reusing the same redaction desk (masking) and archive (writer) as the
// outbound window, but observed by a server-side middleware instead of a
// transport.

package wirelog

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// InboundExchange carries everything a server-side middleware observed for one
// inbound request. Unlike the internal outbound exchange it is exported, because
// the observer — a gin middleware in zobo-be / vban-be — lives in another
// package and fills these fields itself.
type InboundExchange struct {
	// Ctx carries the request context and any wirelog annotations the handler
	// chain attached (WithRef, WithOperation, WithConsumer, WithTags). A nil Ctx
	// is treated as context.Background().
	Ctx context.Context
	// Method is the HTTP method as received.
	Method string
	// Route is the matched route template, e.g. "/v1/wallet/:id". When set it is
	// used as the normalized endpoint; when empty the raw Path is normalized.
	Route string
	// Path is the raw request path as received.
	Path string
	// StatusCode is the response status written to the client; 0 when none was.
	StatusCode int
	// Latency is how long the handler took to produce the response.
	Latency time.Duration
	// RequestHeaders and ResponseHeaders are masked before persist; the source
	// maps are never mutated.
	RequestHeaders  http.Header
	ResponseHeaders http.Header
	// RequestBody and ResponseBody are the raw bodies. Leave them nil when body
	// capture is off for this path; sizes are still recorded from RequestSize /
	// ResponseSize.
	RequestBody  []byte
	ResponseBody []byte
	// RequestSize and ResponseSize are the byte sizes on the wire, always set by
	// the middleware (from Content-Length / bytes written) regardless of whether
	// body content is captured. Zero falls back to len(body).
	RequestSize  int64
	ResponseSize int64
	// RemoteIP is the client IP, if the middleware resolved one.
	RemoteIP string
	// Error is an optional failure reason for the record's error column, e.g.
	// gin's c.Errors.String() on a 5xx. Leave empty when the request succeeded.
	Error string
}

// InboundCapturer records inbound requests for one service Config onto the same
// writer as the outbound path. Mint one per service with Wirelog.InboundCapturer
// and call Log from a middleware. The zero value and a capturer minted from a
// nil *Wirelog are no-ops, so a wirelog init failure never breaks request
// serving.
type InboundCapturer struct {
	capture *capture
	ch      chan<- record
	dropped *atomic.Int64
}

// InboundCapturer mints a capturer for one service Config, normalizing cfg at
// mint via newCapture. For inbound, cfg.Provider names the service (e.g.
// "zobo-be") and cfg.Consumer the default client label. On a nil receiver it
// returns a no-op capturer, matching HTTPClient's degradation contract.
func (wl *Wirelog) InboundCapturer(cfg Config) *InboundCapturer {
	if wl == nil {
		return &InboundCapturer{}
	}
	return &InboundCapturer{
		capture: newCapture(cfg, wl.opts.consumer),
		ch:      wl.ch,
		dropped: &wl.dropped,
	}
}

// Log builds a masked record from x and enqueues it without blocking. Excluded
// paths produce no record. A full buffer, or a panicking Masker/PathNormalizer,
// drops and counts the record rather than blocking or failing the request being
// served. It is safe on a no-op capturer.
func (c *InboundCapturer) Log(x InboundExchange) {
	if c == nil || c.capture == nil || c.ch == nil {
		return
	}
	if matchAny(x.Path, c.capture.cfg.ExcludePaths) {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			c.dropped.Add(1)
		}
	}()
	rec := c.capture.buildInboundRecord(x)
	select {
	case c.ch <- rec:
	default:
		c.dropped.Add(1)
	}
}

// buildInboundRecord assembles the persisted record for one inbound request;
// masking happens here, before enqueue, exactly as on the outbound path. Bodies
// are dropped when capture is off or the path is skip-listed, but sizes are kept.
func (c *capture) buildInboundRecord(x InboundExchange) record {
	ctx := x.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	captureBodies := c.cfg.CaptureBodies && !matchAny(x.Path, c.cfg.SkipBodyPaths)
	reqBody, respBody := x.RequestBody, x.ResponseBody
	if !captureBodies {
		reqBody, respBody = nil, nil
	}
	return record{
		provider:        c.cfg.Provider,
		consumer:        resolveConsumer(ctx, c.cfg.Consumer, c.consumer),
		operation:       string(operationFrom(ctx)),
		endpoint:        c.inboundEndpoint(x),
		path:            x.Path,
		method:          x.Method,
		remoteIP:        x.RemoteIP,
		statusCode:      x.StatusCode,
		outcome:         classifyInbound(x.StatusCode),
		callErr:         x.Error,
		latencyMS:       x.Latency.Milliseconds(),
		requestSize:     inboundSize(x.RequestSize, x.RequestBody),
		responseSize:    inboundSize(x.ResponseSize, x.ResponseBody),
		internalRef:     resolveRef(ctx, reqBody, c.cfg.RefFields),
		idempotencyKey:  resolveIdempotencyKey(ctx, x.RequestHeaders),
		requestHeaders:  maskHeaders(x.RequestHeaders, c.deny),
		requestBody:     maskBody(reqBody, c.cfg.MaxBodyBytes, c.fields, c.cfg.Masker, x.RequestHeaders.Get("Content-Type")),
		responseHeaders: maskHeaders(x.ResponseHeaders, c.deny),
		responseBody:    maskBody(respBody, c.cfg.MaxBodyBytes, c.fields, c.cfg.Masker, x.ResponseHeaders.Get("Content-Type")),
		tags:            tagsFrom(ctx),
	}
}

// inboundEndpoint prefers the middleware's matched route template and falls back
// to normalizing the raw path when no template was supplied.
func (c *capture) inboundEndpoint(x InboundExchange) string {
	if x.Route != "" {
		return x.Route
	}
	return c.cfg.PathNormalizer(x.Path)
}

// inboundSize prefers the explicit wire size the middleware reported and falls
// back to the captured body length; 0 when neither is known.
func inboundSize(explicit int64, body []byte) int64 {
	if explicit > 0 {
		return explicit
	}
	return int64(len(body))
}
