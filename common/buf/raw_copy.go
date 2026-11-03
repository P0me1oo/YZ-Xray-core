package buf

import "context"

// RawCopyHandler preserves a writer wrapper's side effects when a connection
// is copied directly without WriteMultiBuffer. BeforeRawCopy runs before bytes
// are forwarded; AfterRawCopy receives only bytes actually written.
type RawCopyHandler interface {
	Writer
	UnwrapWriter() Writer
	BeforeRawCopy(context.Context, int) error
	AfterRawCopy(int64)
}

// RawCopyLimiter bounds each kernel transfer and supplies an additional
// cancellation context. A zero limit means no additional size bound.
type RawCopyLimiter interface {
	RawCopyHandler
	RawCopyLimit() int
	RawCopyContext() context.Context
}
