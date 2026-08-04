// options.go — the depot's standing orders: instance-level options applied
// once in New.

package wirelog

import (
	"regexp"
	"time"
)

// Option customises a Wirelog instance at construction.
type Option func(*options)

// options holds instance settings; see defaultOptions for the FRD defaults.
type options struct {
	buffer        int
	batchSize     int
	flushInterval time.Duration
	logger        Logger
	autoMigrate   bool
	consumer      string
	table         string
}

// defaultOptions returns the FRD instance defaults applied before user options.
func defaultOptions() options {
	return options{
		buffer:        2048,
		batchSize:     100,
		flushInterval: 2 * time.Second,
		logger:        nopLogger{},
		table:         defaultTable,
	}
}

// tableNamePattern validates a destination table identifier. A table name
// cannot be a bind parameter, so it is interpolated into SQL — this pattern is
// the injection guard. The length bound keeps derived index names within
// Postgres's 63-character identifier limit.
var tableNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,44}$`)

// WithBuffer sets the enqueue channel capacity (default 2048); non-positive
// values keep the default.
func WithBuffer(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.buffer = n
		}
	}
}

// WithBatchSize sets the writer flush batch size (default 100). Non-positive
// values keep the default; values above maxBatchSize clamp so one INSERT
// never exceeds Postgres's bind-parameter limit.
func WithBatchSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.batchSize = min(n, maxBatchSize)
		}
	}
}

// WithFlushInterval sets the writer ticker interval (default 2s);
// non-positive values keep the default.
func WithFlushInterval(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.flushInterval = d
		}
	}
}

// WithLogger sets the insert-failure logger (default silent no-op); nil
// keeps the default.
func WithLogger(l Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithAutoMigrate toggles applying the embedded DDL in New (default false).
func WithAutoMigrate(b bool) Option {
	return func(o *options) { o.autoMigrate = b }
}

// WithDefaultConsumer stamps every record unless overridden — the lowest
// rung of the consumer precedence chain.
func WithDefaultConsumer(c string) Option {
	return func(o *options) { o.consumer = c }
}

// WithTable sets the destination table (default "provider_api_logs"), letting
// one database hold more than one wirelog table — e.g. a separate
// "inbound_api_logs" for inbound request capture. The name is validated as a
// safe SQL identifier; an invalid name is ignored so a misconfiguration keeps
// the default rather than yielding injectable SQL.
func WithTable(name string) Option {
	return func(o *options) {
		if tableNamePattern.MatchString(name) {
			o.table = name
		}
	}
}
