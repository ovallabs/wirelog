// config.go — a provider's standing instruction sheet: the per-provider
// capture configuration types, read-only once a transport is minted.

package wirelog

// Operation labels the business action behind a provider call, e.g. OperationPayout.
type Operation string

// Masker transforms the value of a matched JSON body field; field arrives lowercased.
// It applies to JSON body fields only — denied headers always become the mask constant.
type Masker func(field string, value any) any

// ResultExtractor reads a provider's own response envelope to decide whether a
// call failed, for providers that return 2xx and signal errors in the body.
// It sees the unmasked response body and runs only when the transport succeeded.
type ResultExtractor func(statusCode int, body []byte) (failed bool, message string)

// Config is per-provider capture configuration. It is read-only shared state
// once HTTPClient or WrapTransport mints a transport from it; never mutate it
// afterwards.
type Config struct {
	Provider       string
	Consumer       string
	CaptureBodies  bool
	MaxBodyBytes   int // default 16384
	MaskFields     []string
	DenyHeaders    []string
	Masker         Masker
	SkipBodyPaths  []string            // substring match on req.URL.Path only: metadata+sizes, never bodies
	ExcludePaths   []string            // substring match on req.URL.Path only: no record at all
	PathNormalizer  func(string) string // default DefaultNormalizer
	ResultExtractor ResultExtractor     // optional; classifies 2xx-with-error-body responses
}

// ConfigOption customises a Config minted by NewConfig.
type ConfigOption func(*Config)
