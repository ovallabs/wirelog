// outcome.go — the delivery stamp: classifies an outbound call's result as
// success, provider_error, timeout or network, and an inbound request's result
// as success, client_error or server_error.

package wirelog

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// outcome values stored in the outcome column. The first four classify outbound
// provider calls; the last two, with success, classify inbound requests.
const (
	outcomeSuccess       = "success"
	outcomeProviderError = "provider_error"
	outcomeTimeout       = "timeout"
	outcomeNetwork       = "network"
	outcomeClientError   = "client_error"
	outcomeServerError   = "server_error"
)

// classify maps a transport result to an outcome. errors.Is/As unwrap,
// so DeadlineExceeded inside a *url.Error still classifies as timeout.
func classify(resp *http.Response, err error) string {
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return outcomeTimeout
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return outcomeTimeout
		}
		return outcomeNetwork
	}
	if resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return outcomeSuccess
	}
	return outcomeProviderError
}

// classifyInbound maps a response status to an inbound outcome: 5xx is a
// server_error, 4xx a client_error, 2xx/3xx a success. A status below 200
// (including 0, when no response was written) counts as server_error, since the
// request was never served a normal reply.
func classifyInbound(status int) string {
	switch {
	case status >= 500:
		return outcomeServerError
	case status >= 400:
		return outcomeClientError
	case status >= 200:
		return outcomeSuccess
	default:
		return outcomeServerError
	}
}
