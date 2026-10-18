package shadowsocks_2022

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/pipe"
)

type lifecycleWriter struct {
	buf.Writer
	closed atomic.Int32
}

func (w *lifecycleWriter) Interrupt() {
	w.closed.Add(1)
	common.Interrupt(w.Writer)
}

func lifecycleLink(t *testing.T) (*transport.Link, *pipe.Writer, *lifecycleWriter) {
	t.Helper()
	r, w := pipe.New()
	ur, uw := pipe.New()
	t.Cleanup(func() { common.Interrupt(r); common.Interrupt(ur) })
	tracked := &lifecycleWriter{Writer: uw}
	return &transport.Link{Reader: r, Writer: tracked}, w, tracked
}

// Keep the old bridge's disconnect and cancellation guarantees on the native transport.
func TestTransportTCPReleasesLink(t *testing.T) {
	for _, cancelContext := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "cancel"}[cancelContext], func(t *testing.T) {
			link, down, writer := lifecycleLink(t)
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				_ = TransportTCP(ctx, policy.SessionDefault(), buf.NewReader(server), buf.NewWriter(server), link)
				close(done)
			}()
			if cancelContext {
				cancel()
			} else {
				_ = client.Close()
				_ = down.Close()
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("transport did not exit")
			}
			if got := writer.closed.Load(); got != 1 {
				t.Fatalf("link release count = %d, want 1", got)
			}
		})
	}
}

func TestUDPSessionConcurrentClose(t *testing.T) {
	link, _, writer := lifecycleLink(t)
	s := &ServerUDPSession{}
	s.link.Store(link)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(s.Close)
	}
	wg.Wait()
	if got := writer.closed.Load(); got != 1 {
		t.Fatalf("link release count = %d, want 1", got)
	}
}
