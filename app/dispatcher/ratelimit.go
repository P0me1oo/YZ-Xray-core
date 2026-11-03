package dispatcher

import (
	"context"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"golang.org/x/time/rate"
)

type RateLimitWriter struct {
	Writer  buf.Writer
	Limiter *rate.Limiter
	Context context.Context
}

func (w *RateLimitWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if w.Limiter != nil {
		ctx := w.Context
		if ctx == nil {
			ctx = context.Background()
		}
		if err := w.Limiter.WaitN(ctx, int(mb.Len())); err != nil {
			buf.ReleaseMulti(mb)
			return err
		}
	}
	return w.Writer.WriteMultiBuffer(mb)
}

func (w *RateLimitWriter) Close() error { return common.Close(w.Writer) }
func (w *RateLimitWriter) Interrupt()   { common.Interrupt(w.Writer) }

func (w *RateLimitWriter) UnwrapWriter() buf.Writer        { return w.Writer }
func (w *RateLimitWriter) RawCopyContext() context.Context { return w.Context }
func (w *RateLimitWriter) RawCopyLimit() int {
	if w.Limiter == nil || w.Limiter.Limit() == rate.Inf {
		return 0
	}
	return max(1, w.Limiter.Burst())
}
func (w *RateLimitWriter) BeforeRawCopy(ctx context.Context, n int) error {
	if w.Limiter == nil {
		return nil
	}
	return w.Limiter.WaitN(ctx, n)
}
func (w *RateLimitWriter) AfterRawCopy(int64) {}
