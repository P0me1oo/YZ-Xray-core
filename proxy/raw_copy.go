package proxy

import (
	"context"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/signal"
	"github.com/xtls/xray-core/features/stats"
)

type rawCopyState struct {
	handlers                  []buf.RawCopyHandler
	limiters                  []buf.RawCopyLimiter
	readCounter, writeCounter stats.Counter
	timer                     *signal.ActivityTimer
}

func newRawCopyState(writer buf.Writer, read, write stats.Counter, timer *signal.ActivityTimer) *rawCopyState {
	s := &rawCopyState{readCounter: read, writeCounter: write, timer: timer}
	for writer != nil {
		if handler, ok := writer.(buf.RawCopyHandler); ok {
			s.handlers = append(s.handlers, handler)
			if limiter, ok := writer.(buf.RawCopyLimiter); ok {
				s.limiters = append(s.limiters, limiter)
			}
		}
		upstream, ok := writer.(interface{ UnwrapWriter() buf.Writer })
		if !ok {
			break
		}
		writer = upstream.UnwrapWriter()
	}
	return s
}

func (s *rawCopyState) chunkSize() int {
	size := 64 * 1024
	for _, limiter := range s.limiters {
		if limit := limiter.RawCopyLimit(); limit > 0 && limit < size {
			size = limit
		}
	}
	return size
}

func (s *rawCopyState) before(ctx context.Context, n int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, handler := range s.handlers {
		if err := handler.BeforeRawCopy(ctx, n); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (s *rawCopyState) read(n int64) {
	if s.readCounter != nil {
		s.readCounter.Add(n)
	}
}

func (s *rawCopyState) written(n int64) {
	if s.writeCounter != nil {
		s.writeCounter.Add(n)
	}
	for _, handler := range s.handlers {
		handler.AfterRawCopy(n)
	}
	if s.timer != nil {
		s.timer.Update()
	}
}

func (s *rawCopyState) context(ctx context.Context) (context.Context, func()) {
	merged, cancel := context.WithCancel(ctx)
	var stops []func() bool
	for _, limiter := range s.limiters {
		if extra := limiter.RawCopyContext(); extra != nil {
			if extra.Err() != nil {
				cancel()
			}
			stops = append(stops, context.AfterFunc(extra, cancel))
		}
	}
	return merged, func() {
		for _, stop := range stops {
			stop()
		}
		cancel()
	}
}
